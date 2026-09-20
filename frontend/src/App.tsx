/**
 * taggo のメイン画面。
 *
 * 最上部の検索バーとカード型グリッドの 2 層だけ。
 * フォルダーツリーは持たない。詳細プレビューは画面遷移せずオーバーレイで開く。
 */

import { useCallback, useMemo, useState } from "react";
import {
  ExclamationTriangleIcon,
  FolderOpenIcon,
  InformationCircleIcon,
  XMarkIcon,
} from "@heroicons/react/20/solid";
import { appendTagToQuery, type Entry, type TagEditResult } from "./api/taggo";
import { BulkTagDialog } from "./components/BulkTagDialog";
import { Button } from "./components/Button";
import { CardGrid } from "./components/CardGrid";
import { DetailPanel } from "./components/DetailPanel";
import { SearchBar } from "./components/SearchBar";
import { Toolbar } from "./components/Toolbar";
import { useLibrary } from "./hooks/useLibrary";

export default function App() {
  const library = useLibrary();
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [detailPath, setDetailPath] = useState<string | null>(null);
  const [bulkOpen, setBulkOpen] = useState(false);
  // オーバーレイを閉じたあと、キー入力の行き先を検索バーへ戻すための合図。
  const [focusSignal, setFocusSignal] = useState(0);

  const { entries, query, setQuery, notify, replaceEntry } = library;

  // 一覧が入れ替わっても、開いているプレビューは最新のエントリを指し続ける。
  const detailEntry = useMemo(
    () => (detailPath === null ? null : entries.find((e) => e.path === detailPath) ?? null),
    [detailPath, entries],
  );

  // 画像の詳細プレビューは「前へ／次へ」で画像だけを順に辿れるようにする。
  // 現在の検索結果・並び順に対する画像だけの部分列として扱う。
  const imageEntries = useMemo(() => entries.filter((e) => e.kind === "image"), [entries]);
  const imageIndex = useMemo(
    () =>
      detailEntry?.kind === "image"
        ? imageEntries.findIndex((e) => e.path === detailEntry.path)
        : -1,
    [detailEntry, imageEntries],
  );
  // 端では止める（ループしない）。
  const navigateImage = useCallback(
    (direction: 1 | -1) => {
      if (imageIndex < 0) return;
      const next = imageIndex + direction;
      if (next < 0 || next >= imageEntries.length) return;
      setDetailPath(imageEntries[next].path);
    },
    [imageIndex, imageEntries],
  );

  const selectedEntries = useMemo(
    () => entries.filter((e) => selected.has(e.path)),
    [entries, selected],
  );

  // オーバーレイを閉じる共通処理。閉じたあとは必ず検索バーへ戻す。
  const closeDetail = useCallback(() => {
    setDetailPath(null);
    setFocusSignal((n) => n + 1);
  }, []);

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
        setDetailPath(null);
      });
    },
    [query, setQuery],
  );

  // WikiLink をたどる。同じ名前のノートが一覧にあればそれを開き、無ければ検索に落とす。
  const handleFollowLink = useCallback(
    (target: string) => {
      const needle = target.toLowerCase();
      const found = entries.find((e) => {
        const base = e.name.replace(/\.[^.]+$/, "").toLowerCase();
        return base === needle || e.title.toLowerCase() === needle;
      });
      if (found) {
        setDetailPath(found.path);
        return;
      }
      setQuery(target);
      setDetailPath(null);
      notify("info", `「${target}」に一致するノートが無かったため、検索条件に切り替えました。`);
    },
    [entries, notify, setQuery],
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
            onOpen={(entry) => setDetailPath(entry.path)}
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
          onFollowLink={handleFollowLink}
          onEntryUpdated={replaceEntry}
          onError={(message) => notify("error", message)}
          onNavigateImage={navigateImage}
          imageIndex={imageIndex}
          imageTotal={imageEntries.length}
        />
      )}

      {bulkOpen && (
        <BulkTagDialog
          entries={selectedEntries}
          onClose={closeBulk}
          onApplied={handleBulkApplied}
        />
      )}
    </div>
  );
}
