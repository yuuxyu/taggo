/**
 * ライブラリ（開いているフォルダの全エントリ）の状態管理。
 *
 * 検索は 1 文字入力ごとに走らせる。BuntDB はインメモリなので往復は速いが、
 * 連続入力のたびに全件を描き直すのは無駄なので、ごく短い間隔でまとめている。
 */

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  cancelLoadMore as cancelLoadMoreApi,
  cloudSyncHint,
  Events,
  loadMore as loadMoreApi,
  on,
  openFolder,
  getStatus,
  search,
  selectFolder,
  type Entry,
  type EntryChanged,
  type ScanDone,
  type ScanProgress,
  type SortOrder,
  type Status,
  type TagPageGroup,
} from "../api/taggo";

/** 入力が落ち着くまでの待ち時間（ミリ秒）。体感では即時に見える範囲に収める。 */
const SEARCH_DEBOUNCE_MS = 60;

export interface Notice {
  id: number;
  kind: "info" | "error";
  message: string;
}

/** 読み込み前に確認したいフォルダ。クラウド同期フォルダの中にあるときに立つ。 */
export interface PendingFolder {
  path: string;
  /** 同期サービスが Windows に登録した名前（「Dropbox」など）。 */
  service: string;
}

export interface Library {
  status: Status | null;
  progress: ScanProgress | null;
  entries: Entry[];
  total: number;
  /** 検索しているタグのタグページ。一覧（entries）とは別枠で、グリッドの上に見出しとして出す。 */
  tagPages: TagPageGroup[];
  query: string;
  sort: SortOrder;
  loading: boolean;
  notices: Notice[];
  setQuery: (q: string) => void;
  setSort: (s: SortOrder) => void;
  chooseFolder: () => Promise<void>;
  /** 確認待ちのフォルダ。null なら確認は要らない。 */
  pendingFolder: PendingFolder | null;
  /** 確認のうえ読み込む。 */
  confirmPendingFolder: () => Promise<void>;
  /** 確認をやめて、フォルダを開かない。 */
  cancelPendingFolder: () => void;
  reload: () => Promise<void>;
  notify: (kind: Notice["kind"], message: string) => void;
  dismissNotice: (id: number) => void;
  /** 単一エントリを差し替える。タグ編集後に一覧へ即反映するために使う。 */
  replaceEntry: (entry: Entry) => void;
  /** 上限で打ち切ったことを知らせるバナーを出しているか。 */
  loadMoreBannerOpen: boolean;
  setLoadMoreBannerOpen: (open: boolean) => void;
  /** 続きを読み込む。all なら残りをすべて読む。 */
  loadMore: (all: boolean) => Promise<void>;
  /** 続きの読み込みを取りやめる。 */
  cancelLoadMore: () => Promise<void>;
}

export function useLibrary(): Library {
  const [status, setStatus] = useState<Status | null>(null);
  const [progress, setProgress] = useState<ScanProgress | null>(null);
  const [entries, setEntries] = useState<Entry[]>([]);
  const [total, setTotal] = useState(0);
  const [tagPages, setTagPages] = useState<TagPageGroup[]>([]);
  const [query, setQuery] = useState("");
  const [sort, setSort] = useState<SortOrder>("name_asc");
  const [loading, setLoading] = useState(false);
  const [notices, setNotices] = useState<Notice[]>([]);
  const [pendingFolder, setPendingFolder] = useState<PendingFolder | null>(null);
  const [loadMoreBannerOpen, setLoadMoreBannerOpen] = useState(false);
  // 走査が終わった回数。件数が前と同じでも、走査のたびに一覧を引き直すきっかけにする。
  const [scanSeq, setScanSeq] = useState(0);

  const noticeSeq = useRef(0);

  const notify = useCallback((kind: Notice["kind"], message: string) => {
    noticeSeq.current += 1;
    const notice = { id: noticeSeq.current, kind, message };
    setNotices((prev) => [...prev, notice]);
    // 情報通知は自動で消す。エラーは操作で閉じるまで残す。
    if (kind === "info") {
      window.setTimeout(() => {
        setNotices((prev) => prev.filter((n) => n.id !== notice.id));
      }, 4000);
    }
  }, []);

  const dismissNotice = useCallback((id: number) => {
    setNotices((prev) => prev.filter((n) => n.id !== id));
  }, []);

  const runSearch = useCallback(
    async (q: string, s: SortOrder) => {
      try {
        const result = await search(q, s);
        setEntries(result.entries ?? []);
        setTotal(result.total ?? 0);
        setTagPages(result.tagPages ?? []);
      } catch (err) {
        notify("error", `検索に失敗しました: ${String(err)}`);
      }
    },
    [notify],
  );

  const reload = useCallback(async () => {
    const next = await getStatus();
    setStatus(next);
    await runSearch(query, sort);
  }, [query, sort, runSearch]);

  // 検索バーの入力と並び順に追従して検索し直す。
  useEffect(() => {
    const timer = window.setTimeout(() => {
      void runSearch(query, sort);
    }, SEARCH_DEBOUNCE_MS);
    return () => window.clearTimeout(timer);
  }, [query, sort, runSearch]);

  // 起動時に、前回の状態（あれば）を読み込む。
  useEffect(() => {
    void getStatus().then(setStatus);
  }, []);

  // バックエンドからのイベントを購読する。
  useEffect(() => {
    const offProgress = on<ScanProgress>(Events.scanProgress, (p) => {
      setProgress(p);
    });

    const offDone = on<ScanDone>(Events.scanDone, (done) => {
      setProgress(null);
      setLoading(false);
      if (done.error) {
        const what = done.loadedMore ? "続きの読み込み" : "フォルダの読み込み";
        notify("error", `${what}に失敗しました: ${done.error}`);
        return;
      }
      if (done.warning) {
        notify("error", done.warning);
      }
      if (done.loadedMore) {
        if (done.cancelled) {
          notify("info", "続きの読み込みを取りやめました。");
        } else {
          notify("info", `${(done.added ?? 0).toLocaleString()} 件を追加で読み込みました。`);
        }
      } else {
        // クラウド上にだけあるファイルの件数はツールバーに常に出ているので、通知はしない。
        // 上限で打ち切ったことは、すぐ消える通知ではなく、閉じるまで残るバナーで伝える。
        setLoadMoreBannerOpen((done.remaining ?? 0) > 0);
      }
      void getStatus().then(setStatus);
      setScanSeq((n) => n + 1);
    });

    const offChanged = on<EntryChanged>(Events.entryChanged, (change) => {
      if (change.removed) {
        setEntries((prev) => prev.filter((e) => e.path !== change.path));
        setTotal((prev) => Math.max(0, prev - 1));
        setTagPages((prev) => replaceTagPage(prev, change.path, null));
        void getStatus().then(setStatus);
        return;
      }
      if (!change.entry) return;
      const updated = change.entry;
      setEntries((prev) => {
        const idx = prev.findIndex((e) => e.path === updated.path);
        if (idx < 0) return prev;
        const next = prev.slice();
        next[idx] = updated;
        return next;
      });
      setTagPages((prev) => replaceTagPage(prev, updated.path, updated));
      void getStatus().then(setStatus);
    });

    return () => {
      offProgress();
      offDone();
      offChanged();
    };
  }, [notify]);

  // 走査完了後と、件数が変わったタイミングで検索結果を引き直す。
  // 件数だけを見ると、同じフォルダを開き直したときに件数が変わらず、
  // 開く時点で空にした一覧がそのまま残ってしまうため、走査の完了も見る。
  const entryCount = status?.entryCount ?? 0;
  useEffect(() => {
    void runSearch(query, sort);
    // query / sort の変更は別の effect が拾うので、ここでは件数と走査の完了だけを見る。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [entryCount, scanSeq]);

  /** 読み込みを実際に始める。ここから先はファイルを開くので、通信が起きうる。 */
  const startOpen = useCallback(
    async (dir: string) => {
      try {
        setLoading(true);
        setEntries([]);
        setTotal(0);
        setTagPages([]);
        setLoadMoreBannerOpen(false);
        await openFolder(dir);
      } catch (err) {
        setLoading(false);
        notify("error", `フォルダを開けませんでした: ${String(err)}`);
        // 断られた場合、バックエンドは前のフォルダを開いたままなので、一覧をそれに戻す。
        void reload();
      }
    },
    [notify, reload],
  );

  // フォルダを選ぶところと読み込むところを分けてある。クラウド同期フォルダでは、
  // 走査そのものがダウンロードを誘発しうるため、読み込む前に確認を挟む。
  const chooseFolder = useCallback(async () => {
    try {
      const dir = await selectFolder();
      if (!dir) return;

      const service = await cloudSyncHint(dir);
      if (service !== "") {
        setPendingFolder({ path: dir, service });
        return;
      }
      await startOpen(dir);
    } catch (err) {
      notify("error", `フォルダを開けませんでした: ${String(err)}`);
    }
  }, [notify, startOpen]);

  const confirmPendingFolder = useCallback(async () => {
    const target = pendingFolder;
    setPendingFolder(null);
    if (target) await startOpen(target.path);
  }, [pendingFolder, startOpen]);

  const cancelPendingFolder = useCallback(() => setPendingFolder(null), []);

  // 続きの読み込み中も一覧は消さない。読み終わったら走査完了イベントで引き直す。
  const loadMore = useCallback(
    async (all: boolean) => {
      // 最初の進捗イベントが届くまでの間も、読み込み中だと分かるようにしておく。
      // 呼び出しの完了より先にイベントが届くことがあるので、呼ぶ前に立てる。
      setProgress({ done: 0, found: 0, loadingMore: true });
      try {
        await loadMoreApi(all);
      } catch (err) {
        setProgress(null);
        notify("error", `続きを読み込めませんでした: ${String(err)}`);
      }
    },
    [notify],
  );

  const cancelLoadMore = useCallback(async () => {
    await cancelLoadMoreApi();
  }, []);

  const replaceEntry = useCallback((entry: Entry) => {
    setEntries((prev) => {
      const idx = prev.findIndex((e) => e.path === entry.path);
      if (idx < 0) return prev;
      const next = prev.slice();
      next[idx] = entry;
      return next;
    });
  }, []);

  return useMemo(
    () => ({
      status,
      progress,
      entries,
      total,
      tagPages,
      query,
      sort,
      loading,
      notices,
      setQuery,
      setSort,
      chooseFolder,
      pendingFolder,
      confirmPendingFolder,
      cancelPendingFolder,
      reload,
      notify,
      dismissNotice,
      replaceEntry,
      loadMoreBannerOpen,
      setLoadMoreBannerOpen,
      loadMore,
      cancelLoadMore,
    }),
    [
      status,
      progress,
      entries,
      total,
      tagPages,
      query,
      sort,
      loading,
      notices,
      chooseFolder,
      pendingFolder,
      confirmPendingFolder,
      cancelPendingFolder,
      reload,
      notify,
      dismissNotice,
      replaceEntry,
      loadMoreBannerOpen,
      loadMore,
      cancelLoadMore,
    ],
  );
}

/**
 * 見出しに出しているタグページのうち、path のものを最新のエントリへ差し替える。
 * 消えたとき（entry が null）や、`tag:` の宣言が外れたり別のタグへ変わったりしたときは
 * 見出しから外す。ほかのタグを宣言し直したノートを見出しへ足すのは、次の検索に任せる。
 */
function replaceTagPage(groups: TagPageGroup[], path: string, entry: Entry | null): TagPageGroup[] {
  if (!groups.some((g) => g.pages.some((p) => p.path === path))) return groups;
  return groups
    .map((g) => ({
      ...g,
      pages: g.pages.flatMap((p) => {
        if (p.path !== path) return [p];
        const still = entry !== null && (entry.tagPage ?? "").toLowerCase() === g.tag.toLowerCase();
        return still ? [entry] : [];
      }),
    }))
    .filter((g) => g.pages.length > 0);
}

/** openFolder を直接呼びたい場面（起動引数など）向けの再エクスポート。 */
export { openFolder };
