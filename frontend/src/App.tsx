/**
 * taggo のメイン画面。
 *
 * 画面構成は Scrapbox の考え方にならい、最上部の検索バーとカード型グリッドの 2 層だけで、
 * フォルダーツリーは持たない。詳細プレビューは画面遷移せずオーバーレイで開く。
 */

import { useCallback, useMemo, useState } from "react";
import { appendTagToQuery, type Entry, type TagEditResult } from "./api/taggo";
import { BulkTagDialog } from "./components/BulkTagDialog";
import { CardGrid } from "./components/CardGrid";
import { DetailPanel } from "./components/DetailPanel";
import { SearchBar } from "./components/SearchBar";
import { Toolbar } from "./components/Toolbar";
import { useLibrary } from "./hooks/useLibrary";
import "./App.css";

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
    <div className="app">
      <header className="app__header">
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
        <ul className="app__notices">
          {library.notices.map((notice) => (
            <li key={notice.id} className={`app__notice app__notice--${notice.kind}`}>
              <span>{notice.message}</span>
              <button type="button" onClick={() => library.dismissNotice(notice.id)}>
                ×
              </button>
            </li>
          ))}
        </ul>
      )}

      <main className="app__body">
        {!hasFolder ? (
          <div className="empty">
            <p className="empty__title">フォルダを選ぶとタグ管理を始められます</p>
            <p className="empty__hint">
              選んだフォルダ配下の Markdown・画像・音声を読み込み、
              ファイルに埋め込まれたタグでそのまま検索できます。
              タグはファイル自身に書き込むので、taggo を使わなくなっても情報は残ります。
            </p>
            <button className="btn btn--primary" type="button" onClick={() => void library.chooseFolder()}>
              フォルダを選択
            </button>
          </div>
        ) : entries.length === 0 ? (
          <div className="empty">
            <p className="empty__title">
              {library.progress ? "読み込み中です…" : "条件に合うファイルがありません"}
            </p>
            {!library.progress && query !== "" && (
              <p className="empty__hint">
                検索条件を緩めてみてください。<code>#タグ</code> は完全一致、
                <code>-#タグ</code> は除外、<code>OR</code> でタグの候補を広げられます。
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
