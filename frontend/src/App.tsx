/**
 * taggo のメイン画面。
 *
 * 最上部の検索バーと、ノートを並べたカード型グリッドの 2 層だけ。
 * フォルダーツリーは持たない。ノートの詳細プレビューは画面遷移せずオーバーレイで開く。
 */

import { useCallback, useEffect, useMemo, useState } from "react";
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
  type Settings,
} from "./api/taggo";
import { Button } from "./components/Button";
import { CardGrid } from "./components/CardGrid";
import { ConfirmDialog } from "./components/ConfirmDialog";
import { DetailPanel } from "./components/DetailPanel";
import { LoadMoreBanner, NotLoadedHint } from "./components/LoadMore";
import { SearchBar } from "./components/SearchBar";
import { SettingsDialog } from "./components/SettingsDialog";
import { Toolbar } from "./components/Toolbar";
import { useDetailHistory } from "./hooks/useDetailHistory";
import { useLibrary } from "./hooks/useLibrary";
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
  // 詳細プレビューで見ているノートは、戻る／進むのための履歴として持つ。
  const history = useDetailHistory();
  const { start: startHistory, push: pushHistory, replace: replaceHistory, clear: clearHistory } =
    history;
  const detailPath = history.current?.path ?? null;
  // 一覧（今の検索結果）に無いのにプレビューで開いたノート。リンク先や
  // タグのページは絞り込みの外にあることが多いので、一覧とは別に持っておく。
  const [outside, setOutside] = useState<ReadonlyMap<string, Entry>>(new Map());
  // 「すべて読み込む」の確認ダイアログを出しているか。
  const [confirmLoadAll, setConfirmLoadAll] = useState(false);
  // オーバーレイを閉じたあと、キー入力の行き先を検索バーへ戻すための合図。
  const [focusSignal, setFocusSignal] = useState(0);

  const { entries, query, setQuery, notify, replaceEntry } = library;

  // 起動時に開けなかったフォルダなど、画面の準備ができる前に起きたことを知らせる。
  useEffect(() => {
    void takeStartupWarnings().then((warnings) => {
      for (const warning of warnings ?? []) notify("error", warning);
    });
  }, [notify]);

  // 一覧が入れ替わっても、開いているプレビューは最新のエントリを指し続ける。
  // 「前へ／次へ」で隣のノートへ移れるよう、位置も一緒に持つ。
  const detailIndex = useMemo(
    () => (detailPath === null ? -1 : entries.findIndex((e) => e.path === detailPath)),
    [detailPath, entries],
  );
  const detailEntry =
    detailIndex >= 0 ? entries[detailIndex] : detailPath === null ? null : (outside.get(detailPath) ?? null);

  // 一覧の外で開いたノートも、外部での変更や削除に追従させる。
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

  // 見ていたノートが消えたら、プレビューを閉じる。
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
   * 取り寄せたページ（まだ無いページを含む）をプレビューで開き、履歴に積む。
   * 一覧に載っていればその項目を、無ければ一覧の外として持って開く。
   */
  const openEntry = useCallback(
    (entry: Entry) => {
      // パスの大文字小文字は、Windows に合わせて区別しない。
      const wanted = entry.path.toLowerCase();
      const inList = entries.find((e) => e.path.toLowerCase() === wanted);
      if (inList) {
        pushHistory(inList.path, inList.title);
        return;
      }
      setOutside((prev) => new Map(prev).set(entry.path, entry));
      pushHistory(entry.path, entry.title);
    },
    [entries, pushHistory],
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

  // 詳細プレビューの前後移動。いまの検索結果・並び順のまま隣のノートへ動く。
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

  // オーバーレイを閉じる共通処理。閉じたあとは必ず検索バーへ戻す。
  const closeDetail = useCallback(() => {
    clearHistory();
    setOutside(new Map());
    setFocusSignal((n) => n + 1);
  }, [clearHistory]);

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
  const handleTagClick = useCallback(
    (tag: string) => {
      setQuery(tag);
      clearHistory();
    },
    [setQuery, clearHistory],
  );

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

  const hasFolder = (library.status?.root ?? "") !== "";
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
                onOpen={(entry) => startHistory(entry.path, entry.title)}
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
          onClose={closeDetail}
          onTagClick={handleTagClick}
          onFollowLink={(link, path) => void handleFollowLink(detailEntry.path, link, path)}
          onFollowTag={(tag) => void handleFollowTag(tag)}
          onCreatePage={handleCreatePage}
          pinned={library.pins.has(detailEntry.path)}
          onTogglePin={(pinned) => handleTogglePin(detailEntry, pinned)}
          onEntryUpdated={updateEntry}
          onError={(message) => notify("error", message)}
          onNavigate={navigate}
          index={detailIndex}
          total={entries.length}
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
