/**
 * ライブラリ（開いているフォルダの全エントリ）の状態管理。
 *
 * 検索は 1 文字入力ごとに走らせる。BuntDB はインメモリなので往復は速いが、
 * 連続入力のたびに全件を描き直すのは無駄なので、ごく短い間隔でまとめている。
 */

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Events,
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
} from "../api/taggo";

/** 入力が落ち着くまでの待ち時間（ミリ秒）。体感では即時に見える範囲に収める。 */
const SEARCH_DEBOUNCE_MS = 60;

export interface Notice {
  id: number;
  kind: "info" | "error";
  message: string;
}

export interface Library {
  status: Status | null;
  progress: ScanProgress | null;
  entries: Entry[];
  total: number;
  query: string;
  sort: SortOrder;
  loading: boolean;
  notices: Notice[];
  setQuery: (q: string) => void;
  setSort: (s: SortOrder) => void;
  chooseFolder: () => Promise<void>;
  reload: () => Promise<void>;
  notify: (kind: Notice["kind"], message: string) => void;
  dismissNotice: (id: number) => void;
  /** 単一エントリを差し替える。タグ編集後に一覧へ即反映するために使う。 */
  replaceEntry: (entry: Entry) => void;
}

export function useLibrary(): Library {
  const [status, setStatus] = useState<Status | null>(null);
  const [progress, setProgress] = useState<ScanProgress | null>(null);
  const [entries, setEntries] = useState<Entry[]>([]);
  const [total, setTotal] = useState(0);
  const [query, setQuery] = useState("");
  const [sort, setSort] = useState<SortOrder>("modified_desc");
  const [loading, setLoading] = useState(false);
  const [notices, setNotices] = useState<Notice[]>([]);

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
        notify("error", `フォルダの読み込みに失敗しました: ${done.error}`);
        return;
      }
      if (done.warning) {
        notify("error", done.warning);
      }
      if (done.limitReached && done.maxEntries) {
        notify(
          "error",
          `対象ファイルが上限 ${done.maxEntries.toLocaleString()} 件に達したため、以降は読み込んでいません。`,
        );
      }
      void getStatus().then(setStatus);
    });

    const offChanged = on<EntryChanged>(Events.entryChanged, (change) => {
      if (change.removed) {
        setEntries((prev) => prev.filter((e) => e.path !== change.path));
        setTotal((prev) => Math.max(0, prev - 1));
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
      void getStatus().then(setStatus);
    });

    return () => {
      offProgress();
      offDone();
      offChanged();
    };
  }, [notify]);

  // 走査完了後に検索結果へ反映する。件数が変わったタイミングで引き直す。
  const entryCount = status?.entryCount ?? 0;
  useEffect(() => {
    void runSearch(query, sort);
    // query / sort の変更は別の effect が拾うので、ここでは件数だけを見る。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [entryCount]);

  const chooseFolder = useCallback(async () => {
    try {
      setLoading(true);
      const dir = await selectFolder();
      if (!dir) {
        setLoading(false);
        return;
      }
      setEntries([]);
      setTotal(0);
    } catch (err) {
      setLoading(false);
      notify("error", `フォルダを開けませんでした: ${String(err)}`);
    }
  }, [notify]);

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
      query,
      sort,
      loading,
      notices,
      setQuery,
      setSort,
      chooseFolder,
      reload,
      notify,
      dismissNotice,
      replaceEntry,
    }),
    [
      status,
      progress,
      entries,
      total,
      query,
      sort,
      loading,
      notices,
      chooseFolder,
      reload,
      notify,
      dismissNotice,
      replaceEntry,
    ],
  );
}

/** openFolder を直接呼びたい場面（起動引数など）向けの再エクスポート。 */
export { openFolder };
