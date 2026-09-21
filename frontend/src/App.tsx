/**
 * taggo のメイン画面。
 *
 * 最上部の検索バーとカード型グリッドの 2 層だけ。
 * フォルダーツリーは持たない。詳細プレビューは画面遷移せずオーバーレイで開く。
 */

import { useCallback, useEffect, useMemo, useState } from "react";
import {
  ExclamationTriangleIcon,
  FolderOpenIcon,
  InformationCircleIcon,
  XMarkIcon,
} from "@heroicons/react/20/solid";
import {
  appendTagToQuery,
  Events,
  getEntry,
  on,
  type Entry,
  type EntryChanged,
  type TagEditResult,
} from "./api/taggo";
import { BulkTagDialog } from "./components/BulkTagDialog";
import { Button } from "./components/Button";
import { CardGrid } from "./components/CardGrid";
import { ConfirmDialog } from "./components/ConfirmDialog";
import { DetailPanel } from "./components/DetailPanel";
import { SearchBar } from "./components/SearchBar";
import { Toolbar } from "./components/Toolbar";
import { useDetailHistory } from "./hooks/useDetailHistory";
import { useLibrary } from "./hooks/useLibrary";

export default function App() {
  const library = useLibrary();
  const [selected, setSelected] = useState<Set<string>>(new Set());
  // 詳細プレビューで見ているファイルは、戻る／進むのための履歴として持つ。
  const history = useDetailHistory();
  const { start: startHistory, push: pushHistory, replace: replaceHistory, clear: clearHistory } =
    history;
  const detailPath = history.current?.path ?? null;
  // 一覧（今の検索結果）に無いのにプレビューで開いたファイル。本文中の画像や
  // リンク先は絞り込みの外にあることが多いので、一覧とは別に持っておく。
  const [outside, setOutside] = useState<ReadonlyMap<string, Entry>>(new Map());
  const [bulkOpen, setBulkOpen] = useState(false);
  // オーバーレイを閉じたあと、キー入力の行き先を検索バーへ戻すための合図。
  const [focusSignal, setFocusSignal] = useState(0);

  const { entries, query, setQuery, notify, replaceEntry } = library;

  // 一覧が入れ替わっても、開いているプレビューは最新のエントリを指し続ける。
  // 「前へ／次へ」で隣のファイルへ移れるよう、位置も一緒に持つ。
  const detailIndex = useMemo(
    () => (detailPath === null ? -1 : entries.findIndex((e) => e.path === detailPath)),
    [detailPath, entries],
  );
  const detailEntry =
    detailIndex >= 0 ? entries[detailIndex] : detailPath === null ? null : (outside.get(detailPath) ?? null);

  // 一覧の外で開いたファイルも、外部での変更や削除に追従させる。
  useEffect(
    () =>
      on<EntryChanged>(Events.entryChanged, (change) => {
        setOutside((prev) => {
          if (!prev.has(change.path)) return prev;
          const next = new Map(prev);
          if (change.removed || !change.entry) {
            next.delete(change.path);
          } else {
            next.set(change.path, change.entry);
          }
          return next;
        });
      }),
    [],
  );

  // 見ていたファイルが消えたら、プレビューを閉じる。
  useEffect(() => {
    if (detailPath !== null && detailEntry === null) clearHistory();
  }, [detailPath, detailEntry, clearHistory]);

  // タグの保存などで新しくなったエントリを、一覧と一覧の外の両方へ反映する。
  const updateEntry = useCallback(
    (entry: Entry) => {
      replaceEntry(entry);
      setOutside((prev) => (prev.has(entry.path) ? new Map(prev).set(entry.path, entry) : prev));
    },
    [replaceEntry],
  );

  /**
   * パスの分かっているファイルをプレビューで開き、履歴に積む。
   * 一覧に無ければ Go 側から取り寄せる。開けなければ false を返す。
   */
  const openPath = useCallback(
    async (path: string): Promise<boolean> => {
      // パスの大文字小文字は、Windows に合わせて区別しない。
      const wanted = path.toLowerCase();
      const inList = entries.find((e) => e.path.toLowerCase() === wanted);
      if (inList) {
        pushHistory(inList.path, inList.title);
        return true;
      }
      try {
        const entry = await getEntry(path);
        setOutside((prev) => new Map(prev).set(entry.path, entry));
        pushHistory(entry.path, entry.title);
        return true;
      } catch {
        return false;
      }
    },
    [entries, pushHistory],
  );

  // 詳細プレビューの前後移動。いまの検索結果・並び順のまま、種類を問わず隣へ動く。
  // 端では止める（ループしない）。めくるたびに履歴が伸びないよう、今の項目を置き換える。
  const navigate = useCallback(
    (direction: 1 | -1) => {
      if (detailIndex < 0) return;
      const next = detailIndex + direction;
      if (next < 0 || next >= entries.length) return;
      replaceHistory(entries[next].path, entries[next].title);
    },
    [detailIndex, entries, replaceHistory],
  );

  const selectedEntries = useMemo(
    () => entries.filter((e) => selected.has(e.path)),
    [entries, selected],
  );

  // オーバーレイを閉じる共通処理。閉じたあとは必ず検索バーへ戻す。
  const closeDetail = useCallback(() => {
    clearHistory();
    setOutside(new Map());
    setFocusSignal((n) => n + 1);
  }, [clearHistory]);

  const closeBulk = useCallback(() => {
    setBulkOpen(false);
    setFocusSignal((n) => n + 1);
  }, []);

  const toggleSelect = useCallback((entry: Entry) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(entry.path)) {
        next.delete(entry.path);
      } else {
        next.add(entry.path);
      }
      return next;
    });
  }, []);

  // タグバッジのクリックで、そのタグを検索バーへ差し込んで絞り込む。
  const handleTagClick = useCallback(
    (tag: string) => {
      void appendTagToQuery(query, tag).then((next) => {
        setQuery(next);
        clearHistory();
      });
    },
    [query, setQuery, clearHistory],
  );

  // ノート間のリンクをたどる。行き先のパスが分かっていればそれを開く。
  // 一覧の外にあっても、登録済みのファイルならそのまま開ける。
  // パスが分からないのは行き先のファイルがまだ無いときなので、名前で一覧から探す。
  // それでも見つからなければ、検索条件のほうを切り替える。
  const handleFollowLink = useCallback(
    async (target: string, path?: string) => {
      if (path !== undefined) {
        if (await openPath(path)) return;
      } else {
        const needle = target.toLowerCase();
        const found = entries.find((e) => {
          const base = e.name.replace(/\.[^.]+$/, "").toLowerCase();
          return base === needle || e.title.toLowerCase() === needle;
        });
        if (found) {
          pushHistory(found.path, found.title);
          return;
        }
      }
      // ファイル自体が無いのか、今の絞り込みから外れているだけなのかは
      // 一覧からは分からないので、どちらにも当てはまる言い方にする。
      setQuery(target);
      clearHistory();
      notify("info", `「${target}」が今の一覧に見つからないため、検索条件に切り替えました。`);
    },
    [entries, notify, setQuery, openPath, pushHistory, clearHistory],
  );

  // Markdown の本文中の画像をクリックしたら、その画像のプレビューへ移る。
  const handleOpenImage = useCallback(
    async (path: string) => {
      if (!(await openPath(path))) notify("error", `画像を開けませんでした: ${path}`);
    },
    [notify, openPath],
  );

  const handleBulkApplied = useCallback(
    (results: TagEditResult[]) => {
      for (const result of results) {
        if (result.ok && result.entry) replaceEntry(result.entry);
      }
      const failed = results.filter((r) => !r.ok).length;
      const ok = results.filter((r) => r.ok).length;
      if (failed === 0) {
        notify("info", `${ok} 件のファイルへタグを書き込みました。`);
      }
    },
    [notify, replaceEntry],
  );

  const hasFolder = (library.status?.root ?? "") !== "";

  return (
    <div className="flex h-full flex-col bg-canvas text-sm">
      <header className="sticky top-0 z-10 flex flex-col gap-2.5 border-b border-line bg-canvas px-4.5 pt-3.5 pb-3">
        <SearchBar
          value={query}
          onChange={setQuery}
          total={library.total}
          entryCount={library.status?.entryCount ?? 0}
          disabled={!hasFolder}
          focusSignal={focusSignal}
        />
        <Toolbar
          status={library.status}
          progress={library.progress}
          sort={library.sort}
          onSortChange={library.setSort}
          onChooseFolder={() => void library.chooseFolder()}
          selectedCount={selected.size}
          onClearSelection={() => setSelected(new Set())}
          onOpenBulkEditor={() => setBulkOpen(true)}
          onSelectAll={() => setSelected(new Set(entries.map((e) => e.path)))}
        />
      </header>

      {library.notices.length > 0 && (
        <ul className="m-0 flex list-none flex-col gap-1.5 px-4.5 pt-2.5">
          {library.notices.map((notice) => (
            <li
              key={notice.id}
              className={`flex items-start gap-3 rounded-md px-3 py-2 text-xs ${
                notice.kind === "error"
                  ? "bg-danger-soft text-danger"
                  : "bg-accent-soft text-accent-ink"
              }`}
            >
              {notice.kind === "error" ? (
                <ExclamationTriangleIcon className="mt-px size-4 shrink-0" aria-hidden="true" />
              ) : (
                <InformationCircleIcon className="mt-px size-4 shrink-0" aria-hidden="true" />
              )}
              <span className="flex-1">{notice.message}</span>
              <button
                type="button"
                className="shrink-0 opacity-60 hover:opacity-100"
                aria-label="この通知を閉じる"
                onClick={() => library.dismissNotice(notice.id)}
              >
                <XMarkIcon className="size-4" />
              </button>
            </li>
          ))}
        </ul>
      )}

      <main className="min-h-0 flex-1 bg-canvas">
        {!hasFolder ? (
          <div className="flex h-full flex-col items-center justify-center gap-2.5 p-10 text-center text-ink-muted">
            <FolderOpenIcon className="size-10 text-ink-faint" aria-hidden="true" />
            <p className="m-0 text-base font-semibold text-ink">
              フォルダを選ぶとタグ管理を始められます
            </p>
            <p className="m-0 max-w-105">
              選んだフォルダ配下の Markdown・画像・音声を読み込み、
              ファイルに埋め込まれたタグでそのまま検索できます。
              タグはファイル自身に書き込むので、taggo を使わなくなっても情報は残ります。
            </p>
            <Button variant="primary" onClick={() => void library.chooseFolder()}>
              <FolderOpenIcon className="size-4" aria-hidden="true" />
              フォルダを選択
            </Button>
          </div>
        ) : entries.length === 0 ? (
          <div className="flex h-full flex-col items-center justify-center gap-2.5 p-10 text-center text-ink-muted">
            <p className="m-0 text-base font-semibold text-ink">
              {library.progress ? "読み込み中です…" : "条件に合うファイルがありません"}
            </p>
            {!library.progress && query !== "" && (
              <p className="m-0 max-w-105">
                検索条件を緩めてみてください。
                <code className="rounded-sm bg-sunken px-1.5 py-px font-mono text-[0.9em]">
                  #タグ
                </code>
                は完全一致、
                <code className="rounded-sm bg-sunken px-1.5 py-px font-mono text-[0.9em]">
                  -#タグ
                </code>
                は除外、
                <code className="rounded-sm bg-sunken px-1.5 py-px font-mono text-[0.9em]">OR</code>
                でタグの候補を広げられます。
              </p>
            )}
          </div>
        ) : (
          <CardGrid
            entries={entries}
            selected={selected}
            selectionMode={selected.size > 0}
            onOpen={(entry) => startHistory(entry.path, entry.title)}
            onToggleSelect={toggleSelect}
            onTagClick={handleTagClick}
          />
        )}
      </main>

      {detailEntry && (
        <DetailPanel
          entry={detailEntry}
          onClose={closeDetail}
          onTagClick={handleTagClick}
          onFollowLink={(target, path) => void handleFollowLink(target, path)}
          onOpenImage={(path) => void handleOpenImage(path)}
          onEntryUpdated={updateEntry}
          onError={(message) => notify("error", message)}
          onNavigate={navigate}
          index={detailIndex}
          total={entries.length}
          history={history}
        />
      )}

      {bulkOpen && (
        <BulkTagDialog
          entries={selectedEntries}
          onClose={closeBulk}
          onApplied={handleBulkApplied}
        />
      )}

      {/* このダイアログが出るのは、Windows のクラウドファイルの仕組みに登録された
          同期フォルダだけ。そこではクラウド上にだけあるファイルを属性で見分けられるので、
          ダウンロードは起きないことを伝えたうえで、読み込むかを一度確認する。 */}
      {library.pendingFolder && (
        <ConfirmDialog
          title={`${library.pendingFolder.service} のフォルダを読み込みますか？`}
          lines={[
            library.pendingFolder.path,
            "クラウド上にだけあるファイルは中身を開かずに一覧へ出すので、読み込んでもダウンロードは始まりません。",
            "中身を見たいファイルは、カードを開いて個別に取り込めます。",
          ]}
          confirmLabel="読み込む"
          onConfirm={() => void library.confirmPendingFolder()}
          onCancel={library.cancelPendingFolder}
        />
      )}
    </div>
  );
}
