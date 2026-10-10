/**
 * taggo のメイン画面。
 *
 * 最上部の検索バーと、ノートを並べたカード型グリッドの 2 層だけ。
 * フォルダーツリーは持たない。ノートの詳細プレビューは画面遷移せずオーバーレイで開く。
 *
 * 一覧（検索結果）とプレビューで開いたノートは、1 本の履歴に順に積む。
 * プレビューでタグをクリックして始めた検索も履歴に残るので、「戻る」で元のノートへ戻れる。
 */

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  ExclamationTriangleIcon,
  FolderOpenIcon,
  InformationCircleIcon,
  XMarkIcon,
} from "@heroicons/react/20/solid";
import {
  createPage,
  Events,
  getEntry,
  on,
  setPinned,
  linkedPage,
  tagPage,
  takeStartupWarnings,
  type Entry,
  type EntryChanged,
  type EntryRenamed,
  type RenameResult,
  type Settings,
} from "./api/taggo";
import { Button } from "./components/Button";
import { CardGrid } from "./components/CardGrid";
import { ConfirmDialog } from "./components/ConfirmDialog";
import { DetailPanel } from "./components/DetailPanel";
import { HistoryNav } from "./components/HistoryNav";
import { LoadMoreBanner, NotLoadedHint } from "./components/LoadMore";
import { NewTagPageBar } from "./components/NewTagPageBar";
import { RenameDialog } from "./components/RenameDialog";
import { SearchBar } from "./components/SearchBar";
import { SettingsDialog } from "./components/SettingsDialog";
import { Toolbar } from "./components/Toolbar";
import { useLibrary } from "./hooks/useLibrary";
import { useNavHistory } from "./hooks/useNavHistory";
import { applyTheme } from "./theme";

/**
 * 読み込み後の合計がこれを超える「すべて読み込む」は、確認を挟む。
 * 上限は RAM の使いすぎを防ぐ安全弁なので、大きく超えるときは一度立ち止まってもらう。
 */
const LOAD_ALL_CONFIRM_THRESHOLD = 100_000;

interface Props {
  /** 起動時に読み込んだ設定。 */
  initialSettings: Settings;
}

export default function App({ initialSettings }: Props) {
  const [settings, setSettings] = useState(initialSettings);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const library = useLibrary(initialSettings.sort);
  // 見てきた一覧とノートは、戻る／進むのための履歴として持つ。
  const history = useNavHistory();
  const {
    current,
    navId,
    go: goHistory,
    pushNote,
    pushSearch,
    replaceSearch,
    backToList,
    remove: removeFromHistory,
    renamePath: renameHistoryPath,
    reset: resetHistory,
  } = history;
  const historyIndex = history.index;
  const detailPath = current.kind === "note" ? current.path : null;
  // 履歴にあるノートのエントリ。リンク先やタグのページは絞り込みの外にあることが多く、
  // 一覧から開いたノートも、別の検索をすると一覧から外れる。戻ったときにも開けるよう、
  // 一覧とは別に持っておく。
  const [visited, setVisited] = useState<ReadonlyMap<string, Entry>>(new Map());
  // 「すべて読み込む」の確認ダイアログを出しているか。
  const [confirmLoadAll, setConfirmLoadAll] = useState(false);
  // 名前の変更ダイアログで名前を変えようとしているページ。
  const [renaming, setRenaming] = useState<Entry | null>(null);
  // オーバーレイを閉じたあと、キー入力の行き先を検索バーへ戻すための合図。
  const [focusSignal, setFocusSignal] = useState(0);

  const { entries, query, setQuery, notify, replaceEntry } = library;

  // 履歴で一覧へ移ったら、その一覧の検索語で引き直し、キー入力の行き先を検索バーへ戻す。
  // 検索バーへの入力は履歴の検索語を書き換えるだけで移動ではないので、ここは通らない。
  useEffect(() => {
    if (current.kind !== "search") return;
    setQuery(current.query);
    setFocusSignal((n) => n + 1);
    // 移動（navId）ごとに一度だけ合わせる。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [navId]);

  // 別のフォルダを開いたら、前のフォルダのノートを指す履歴は捨てる。
  const root = library.status?.root ?? "";
  const shownRoot = useRef(root);
  useEffect(() => {
    if (shownRoot.current === root) return;
    shownRoot.current = root;
    resetHistory(query);
    // 開き直した時点の検索語を使うだけなので、検索語の変化では捨てない。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [root, resetHistory]);

  // 履歴から外れたノートのエントリは、持っておく必要がない。
  useEffect(() => {
    const paths = new Set(history.items.flatMap((item) => (item.kind === "note" ? [item.path] : [])));
    setVisited((prev) =>
      [...prev.keys()].every((path) => paths.has(path))
        ? prev
        : new Map([...prev].filter(([path]) => paths.has(path))),
    );
  }, [history.items]);

  // 起動時に開けなかったフォルダなど、画面の準備ができる前に起きたことを知らせる。
  useEffect(() => {
    void takeStartupWarnings().then((warnings) => {
      for (const warning of warnings ?? []) notify("error", warning);
    });
  }, [notify]);

  // 一覧が入れ替わっても、開いているプレビューは最新のエントリを指し続ける。
  const detailEntry = useMemo(
    () =>
      detailPath === null
        ? null
        : (entries.find((e) => e.path === detailPath) ?? visited.get(detailPath) ?? null),
    [detailPath, entries, visited],
  );

  // 一覧に無いノートも、外部での変更や削除に追従させる。
  useEffect(
    () =>
      on<EntryChanged>(Events.entryChanged, (change) => {
        setVisited((prev) => {
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

  // ページの名前を変えたら、履歴と手元のエントリを新しいパスへ付け替える。
  // 古いパスは消えたものとして扱わないので、見ていたページはそのまま開いたままになる。
  useEffect(
    () =>
      on<EntryRenamed>(Events.entryRenamed, ({ from, to, entry }) => {
        setVisited((prev) => {
          const next = new Map(prev);
          next.delete(from);
          return next.set(to, entry);
        });
        renameHistoryPath(from, to, entry.title);
      }),
    [renameHistoryPath],
  );

  // 見ていたノートが消えたら、履歴から除いて手前の画面へ戻る。
  useEffect(() => {
    if (detailPath !== null && detailEntry === null) removeFromHistory(detailPath);
  }, [detailPath, detailEntry, removeFromHistory]);

  // タグの保存などで新しくなったエントリを、一覧と一覧の外の両方へ反映する。
  const updateEntry = useCallback(
    (entry: Entry) => {
      replaceEntry(entry);
      setVisited((prev) => (prev.has(entry.path) ? new Map(prev).set(entry.path, entry) : prev));
    },
    [replaceEntry],
  );

  /**
   * ページ（まだ無いページを含む）をプレビューで開き、履歴に積む。
   * 一覧に載っていればその項目を開く。どちらでも、戻ってきたときに開けるよう持っておく。
   */
  const openEntry = useCallback(
    (entry: Entry) => {
      // パスの大文字小文字は、Windows に合わせて区別しない。
      const wanted = entry.path.toLowerCase();
      const page = entries.find((e) => e.path.toLowerCase() === wanted) ?? entry;
      setVisited((prev) => new Map(prev).set(page.path, page));
      pushNote(page.path, page.title);
    },
    [entries, pushNote],
  );

  /**
   * パスの分かっているノートをプレビューで開き、履歴に積む。
   * 一覧に無ければ Go 側から取り寄せる。開けなければ false を返す。
   */
  const openPath = useCallback(
    async (path: string): Promise<boolean> => {
      // パスの大文字小文字は、Windows に合わせて区別しない。
      const wanted = path.toLowerCase();
      const inList = entries.find((e) => e.path.toLowerCase() === wanted);
      if (inList) {
        openEntry(inList);
        return true;
      }
      try {
        openEntry(await getEntry(path));
        return true;
      } catch {
        return false;
      }
    },
    [entries, openEntry],
  );

  const closeSettings = useCallback(() => {
    setSettingsOpen(false);
    setFocusSignal((n) => n + 1);
  }, []);

  const { setSort, reload } = library;
  const handleSettingsSaved = useCallback(
    (saved: Settings) => {
      applyTheme(saved.theme);
      // 既定の並び順を変えたときは、今の一覧もその並び順にする。
      if (saved.sort !== settings.sort) setSort(saved.sort);
      // 「続きを読み込む」で増える件数などの表示を、新しい上限にする。
      if (saved.scanLimit !== settings.scanLimit) void reload();
      setSettings(saved);
      closeSettings();
      notify("info", "設定を保存しました。");
    },
    [settings, setSort, reload, closeSettings, notify],
  );

  // タグバッジのクリックで、そのタグで検索し直す。タグの名前と一致するので、
  // そのタグのページが先頭に、タグを付けたノートがその後ろに並ぶ。
  // 新しい一覧として履歴に積むので、「戻る」でクリックした画面へ戻れる。
  const handleTagClick = useCallback((tag: string) => pushSearch(tag), [pushSearch]);

  // 検索バーへの入力は、今の一覧の検索語を書き換える（1 文字ごとに履歴を積まない）。
  const handleQueryChange = useCallback(
    (q: string) => {
      setQuery(q);
      replaceSearch(q);
    },
    [setQuery, replaceSearch],
  );

  // Alt + 左右とマウスの戻る／進むボタン（ボタン 3 / 4）で、ブラウザと同じく履歴を移動する。
  // 一覧でもプレビューでも、入力欄の中でも効かせる。ダイアログを開いている間は動かさない。
  // 画像ビューアを開いている間は、ビューアが先に受けて止める。
  const historyBlocked = settingsOpen || confirmLoadAll || renaming !== null;
  useEffect(() => {
    const move = (delta: number) => {
      if (!historyBlocked) goHistory(historyIndex + delta);
    };
    const onKeyDown = (e: KeyboardEvent) => {
      if (!e.altKey || (e.key !== "ArrowLeft" && e.key !== "ArrowRight")) return;
      e.preventDefault();
      move(e.key === "ArrowLeft" ? -1 : 1);
    };
    // WebView 自体のページ遷移に使われないよう、押した時点で既定の動作を止める。
    const onMouseDown = (e: MouseEvent) => {
      if (e.button === 3 || e.button === 4) e.preventDefault();
    };
    const onMouseUp = (e: MouseEvent) => {
      if (e.button !== 3 && e.button !== 4) return;
      e.preventDefault();
      move(e.button === 3 ? -1 : 1);
    };
    window.addEventListener("keydown", onKeyDown);
    window.addEventListener("mousedown", onMouseDown);
    window.addEventListener("mouseup", onMouseUp);
    return () => {
      window.removeEventListener("keydown", onKeyDown);
      window.removeEventListener("mousedown", onMouseDown);
      window.removeEventListener("mouseup", onMouseUp);
    };
  }, [goHistory, historyIndex, historyBlocked]);

  // ピン留めを切り替える。一覧はピン留めの変更イベントで引き直す。
  const handleTogglePin = useCallback(
    (entry: Entry, pinned: boolean) => {
      void setPinned(entry.path, pinned).catch((err) => notify("error", String(err)));
    },
    [notify],
  );

  // ノート間のリンクをたどる。行き先のパスが分かっていればそれを開く。
  // 一覧の外にあっても、登録済みのノートならそのまま開ける。行き先がまだ無ければ、
  // ファイルは作らずに「まだ無いページ」として開く（作るのはそのページのボタンから）。
  const handleFollowLink = useCallback(
    async (from: string, link: string, path?: string) => {
      if (path !== undefined && (await openPath(path))) return;
      try {
        openEntry(await linkedPage(from, link));
      } catch (err) {
        notify("error", String(err));
      }
    },
    [notify, openPath, openEntry],
  );

  // 本文の [[タグ]] や、関連ページのタグのページをたどる。
  // ページがあればそれを、無ければ「まだ無いページ」として開く。
  const handleFollowTag = useCallback(
    async (tag: string) => {
      try {
        openEntry(await tagPage(tag));
      } catch (err) {
        notify("error", String(err));
      }
    },
    [notify, openEntry],
  );

  // まだ無いページの md ファイルを作って、既定のエディタで開く。
  // 作ったファイルはフォルダの監視が拾い、開いているページがふつうのノートに切り替わる。
  const handleCreatePage = useCallback(
    async (entry: Entry) => {
      try {
        const created = await createPage(entry.path);
        notify(
          "info",
          created
            ? `「${entry.name}」を作成して、エディタで開きました。`
            : `「${entry.name}」は既にあるため、エディタで開きました。`,
        );
      } catch (err) {
        notify("error", String(err));
      }
    },
    [notify],
  );

  const closeRename = useCallback(() => setRenaming(null), []);

  // 名前を変え終えたら結果を知らせる。一覧と開いているプレビューは、名前の変更イベントと
  // 書き換えたノートの変更イベントで新しくなる。
  const handleRenamed = useCallback(
    (before: Entry, result: RenameResult) => {
      setRenaming(null);
      const title = (entry: Entry) => entry.name.slice(0, entry.name.length - entry.ext.length);
      const after = result.entry ? `「${title(result.entry)}」` : "新しい名前";
      notify(
        "info",
        `「${title(before)}」を${after}に変更しました（書き換えたノート ${result.rewritten.toLocaleString()} 件）。`,
      );
      if (result.failed.length > 0) {
        notify("error", `書き換えられなかったノートがあります: ${result.failed.join("、")}`);
      }
      if (result.skipped.length > 0) {
        notify("error", `大きすぎるため書き換えなかったノートがあります: ${result.skipped.join("、")}`);
      }
      if (result.warning) notify("error", result.warning);
    },
    [notify],
  );

  const hasFolder = (library.status?.root ?? "") !== "";

  // 検索語と同じ名前のタグのページが結果の先頭に無ければ、まだ無いページとして取り寄せ、
  // 作成を促す帯を出す。ファイル名にできない検索語では取り寄せに失敗するので、何も出さない。
  // 取り寄せは検索語が変わるたびに走るので、どの検索語に対するものかも一緒に持つ。
  const { resultQuery, head } = library;
  const lacksTagPage = hasFolder && resultQuery.trim() !== "" && head === 0;
  const [newTagPage, setNewTagPage] = useState<{ query: string; page: Entry } | null>(null);
  useEffect(() => {
    if (!lacksTagPage) return;
    let stale = false;
    tagPage(resultQuery).then(
      (page) => {
        if (!stale) setNewTagPage(page.missing ? { query: resultQuery, page } : null);
      },
      () => {
        if (!stale) setNewTagPage(null);
      },
    );
    return () => {
      stale = true;
    };
  }, [lacksTagPage, resultQuery]);
  const shownNewTagPage =
    lacksTagPage && newTagPage?.query === resultQuery ? newTagPage.page : null;

  const remaining = library.status?.remaining ?? 0;
  // 読み込み中は重ねて始められないので、続きを読む操作は出さないか押せなくする。
  const busy = library.progress !== null;

  const { loadMore, setLoadMoreBannerOpen } = library;
  const requestLoadMore = useCallback(
    (all: boolean) => {
      const total = (library.status?.entryCount ?? 0) + remaining;
      if (all && total > LOAD_ALL_CONFIRM_THRESHOLD) {
        setConfirmLoadAll(true);
        return;
      }
      setLoadMoreBannerOpen(false);
      void loadMore(all);
    },
    [library.status?.entryCount, remaining, loadMore, setLoadMoreBannerOpen],
  );

  return (
    <div className="flex h-full flex-col bg-canvas text-sm">
      <header className="sticky top-0 z-10 flex flex-col gap-2.5 border-b border-line bg-canvas px-4.5 pt-3.5 pb-3">
        <div className="flex items-center gap-2">
          {/* 履歴の一覧が検索バーの下に隠れないよう、検索バーより手前に重ねる。 */}
          <div className="relative z-30">
            <HistoryNav items={history.items} index={historyIndex} onGo={goHistory} />
          </div>
          <div className="min-w-0 flex-1">
            <SearchBar
              value={query}
              onChange={handleQueryChange}
              total={library.total}
              entryCount={library.status?.entryCount ?? 0}
              disabled={!hasFolder}
              focusSignal={focusSignal}
            />
          </div>
        </div>
        <Toolbar
          status={library.status}
          progress={library.progress}
          sort={library.sort}
          onSortChange={library.setSort}
          onChooseFolder={() => void library.chooseFolder()}
          onShowLoadMore={() => setLoadMoreBannerOpen(true)}
          onCancelLoadMore={() => void library.cancelLoadMore()}
          onOpenSettings={() => setSettingsOpen(true)}
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

      {library.status && remaining > 0 && library.loadMoreBannerOpen && !busy && (
        <LoadMoreBanner
          status={library.status}
          onLoadMore={requestLoadMore}
          onDismiss={() => setLoadMoreBannerOpen(false)}
        />
      )}

      {shownNewTagPage && <NewTagPageBar page={shownNewTagPage} onCreate={handleCreatePage} />}

      <main className="flex min-h-0 flex-1 flex-col bg-canvas">
        {!hasFolder ? (
          <div className="flex h-full flex-col items-center justify-center gap-2.5 p-10 text-center text-ink-muted">
            <FolderOpenIcon className="size-10 text-ink-faint" aria-hidden="true" />
            <p className="m-0 text-base font-semibold text-ink">
              フォルダを選ぶと、そこが Wiki になります
            </p>
            <p className="m-0 max-w-105">
              選んだフォルダの直下にある Markdown をノートとして読み込み、
              タグとリンクでたどれるようにします。ノートのファイル名は、そのままタグになります。
              タグは本文に [[タグ]] と書くので、taggo を使わなくなっても情報は残ります。
            </p>
            <Button variant="primary" onClick={() => void library.chooseFolder()}>
              <FolderOpenIcon className="size-4" aria-hidden="true" />
              フォルダを選択
            </Button>
          </div>
        ) : entries.length === 0 ? (
          <div className="flex min-h-0 flex-1 flex-col items-center justify-center gap-2.5 p-10 text-center text-ink-muted">
            <p className="m-0 text-base font-semibold text-ink">
              {library.progress ? "読み込み中です…" : "条件に合うノートがありません"}
            </p>
            {!library.progress && query !== "" && (
              <p className="m-0 max-w-105">
                空白で区切った語は、すべてを含むノートだけに絞り込みます。語を減らしてみてください。
              </p>
            )}
            {!library.progress && remaining > 0 && (
              <NotLoadedHint remaining={remaining} onLoadMore={() => requestLoadMore(false)} />
            )}
          </div>
        ) : (
          <>
            <div className="min-h-0 flex-1">
              <CardGrid
                entries={entries}
                head={library.head}
                pins={library.pins}
                onOpen={openEntry}
                onTagClick={handleTagClick}
                onTogglePin={handleTogglePin}
              />
            </div>
            {/* 絞り込んでいるときは、読み込んでいない分が結果に入っていないことを
                常に見える位置で伝える。グリッドの末尾に置くと、2 万件を
                スクロールしきるまで気付けないため、下端に固定する。 */}
            {query !== "" && remaining > 0 && (
              <div className="border-t border-line px-4.5 py-1.5">
                <NotLoadedHint
                  remaining={remaining}
                  onLoadMore={() => requestLoadMore(false)}
                  disabled={busy}
                />
              </div>
            )}
          </>
        )}
      </main>

      {detailEntry && (
        <DetailPanel
          entry={detailEntry}
          onClose={backToList}
          onTagClick={handleTagClick}
          onFollowLink={(link, path) => void handleFollowLink(detailEntry.path, link, path)}
          onFollowTag={(tag) => void handleFollowTag(tag)}
          onCreatePage={handleCreatePage}
          pinned={library.pins.has(detailEntry.path)}
          onTogglePin={(pinned) => handleTogglePin(detailEntry, pinned)}
          onRename={() => setRenaming(detailEntry)}
          onEntryUpdated={updateEntry}
          onError={(message) => notify("error", message)}
          history={history}
        />
      )}

      {confirmLoadAll && library.status && (
        <ConfirmDialog
          title={`残りの ${remaining.toLocaleString()} 件をすべて読み込みますか？`}
          lines={[
            `読み込み後は合計 ${(library.status.entryCount + remaining).toLocaleString()} 件になり、` +
              "メモリを多く使います。読み込み中も一覧は使えます。",
            `少しずつ読みたい場合は「続きを読み込む」で ${library.status.maxEntries.toLocaleString()} 件ずつ読めます。`,
          ]}
          confirmLabel="すべて読み込む"
          onConfirm={() => {
            setConfirmLoadAll(false);
            setLoadMoreBannerOpen(false);
            void loadMore(true);
          }}
          onCancel={() => setConfirmLoadAll(false)}
        />
      )}

      {renaming && (
        <RenameDialog
          entry={renaming}
          onClose={closeRename}
          onRenamed={(result) => handleRenamed(renaming, result)}
        />
      )}

      {settingsOpen && (
        <SettingsDialog
          settings={settings}
          currentRoot={library.status?.root ?? ""}
          onClose={closeSettings}
          onSaved={handleSettingsSaved}
        />
      )}

    </div>
  );
}
