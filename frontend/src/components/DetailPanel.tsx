/**
 * カードをクリックしたときに開く、ノートの詳細プレビュー。
 *
 * 要件どおり画面遷移はせず、同じ画面にオーバーレイとして重ねる。
 * ウィンドウ全体を使って本文を優先するレイアウトにしている。ヘッダーとタグは、
 * マウスを動かした間だけ本文の上に一時的にオーバーレイ表示し、動きが止まれば
 * しばらくして消える。
 *
 * タグ編集の入力欄はコンテンツの邪魔になるため常には出さず、タグは読み取り専用の
 * バッジで表示するだけにして、「タグを編集」ボタンを押したときだけ編集フォーム
 * （入力欄・候補・保存操作）を表示する。
 *
 * ヘッダー左端の「戻る／進む」で、リンクをたどる前のノートへ戻れる。
 * 履歴そのものは App が持ち、ここは操作と表示だけを受け持つ。
 *
 * 本文は編集しない方針なので、ヘッダーの「エディタで開く」から、拡張子に紐づいた
 * アプリ（既定のテキストエディタ）へファイルを渡す。保存された変更はウォッチャーが拾う。
 *
 * 本文中の画像をクリックすると、画像ビューアをこの上に重ねて開く。画像はノートでは
 * ないので履歴には積まず、ビューアを閉じればそのまま同じ位置の本文へ戻る。
 */

import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { BookOpenIcon, PencilSquareIcon, TagIcon, XMarkIcon } from "@heroicons/react/20/solid";
import { openInEditor, setTags, type Entry } from "../api/taggo";
import type { DetailHistory } from "../hooks/useDetailHistory";
import { Button } from "./Button";
import { CloudOnlyNotice } from "./CloudOnlyNotice";
import { HistoryNav } from "./HistoryNav";
import { ImagePreview } from "./ImagePreview";
import { MarkdownPreview } from "./MarkdownPreview";
import { Pager } from "./Pager";
import { RelatedPages, useRelatedPages } from "./RelatedPages";
import { TagBadge } from "./TagBadge";
import { TagEditor } from "./TagEditor";

interface Props {
  entry: Entry;
  onClose: () => void;
  onTagClick: (tag: string) => void;
  /** そのタグだけで一覧を絞り込み直す。タグページから、そのタグの一覧へ移るときに使う。 */
  onSearchTag: (tag: string) => void;
  /** リンクをたどる。実体のパスが分かっている場合は一緒に渡す。 */
  onFollowLink: (target: string, path?: string) => void;
  /** 保存後の最新状態を一覧へ返す。 */
  onEntryUpdated: (entry: Entry) => void;
  onError: (message: string) => void;
  /** 前後のノートへの移動。一覧の並び順で動く。 */
  onNavigate: (direction: 1 | -1) => void;
  /** 一覧における現在位置（0 始まり）。 */
  index: number;
  total: number;
  /** 移動の履歴。戻る／進むと、戻ったときのスクロール位置の復元に使う。 */
  history: DetailHistory;
}

/** UI オーバーレイを自動で隠すまでの無操作時間。 */
const OVERLAY_HIDE_MS = 2200;

/** 本文中の画像をクリックして開いた画像ビューアの対象。 */
interface OpenImage {
  path: string;
  alt?: string;
}

export function DetailPanel({
  entry,
  onClose,
  onTagClick,
  onSearchTag,
  onFollowLink,
  onEntryUpdated,
  onError,
  onNavigate,
  index,
  total,
  history,
}: Props) {
  const { go: goHistory, reportScroll, navId } = history;
  const historyIndex = history.index;
  const historyLength = history.items.length;
  const [draft, setDraft] = useState<string[]>(entry.tags);
  const [saving, setSaving] = useState(false);
  const [tagsOpen, setTagsOpen] = useState(false);
  // 本文中の画像から開いた画像ビューア。閉じれば null に戻り、本文がそのまま見える。
  const [image, setImage] = useState<OpenImage | null>(null);

  // 関連ページ。Markdown は常に本文の右脇に関連ページの欄を置く。
  // ノートを行き来してもレイアウトが跳ねないよう、リンクが 1 件も無くても右の列は残す。
  const related = useRelatedPages(entry);

  // 別のエントリに切り替わったら編集中の内容を捨て、編集フォームも閉じる。
  useEffect(() => {
    setDraft(entry.tags);
    setTagsOpen(false);
  }, [entry.path, entry.tags]);

  // 別のノートへ移ったら、前のノートの画像は閉じる。
  useEffect(() => setImage(null), [entry.path]);
  const closeImage = useCallback(() => setImage(null), []);

  // マウスが動いた直後だけヘッダー／タグ UI を出し、止まればしばらくして隠す。
  // ただしタグ編集欄などにフォーカスが残っている間は、入力中の欄が
  // 見えなくなってしまわないよう隠さない。
  const [overlayVisible, setOverlayVisible] = useState(true);
  const overlayRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    let hideTimer: number;
    const scheduleHide = () => {
      window.clearTimeout(hideTimer);
      hideTimer = window.setTimeout(() => {
        if (overlayRef.current?.contains(document.activeElement)) return;
        setOverlayVisible(false);
      }, OVERLAY_HIDE_MS);
    };
    const onMouseMove = () => {
      setOverlayVisible(true);
      scheduleHide();
    };
    setOverlayVisible(true);
    scheduleHide();
    window.addEventListener("mousemove", onMouseMove);
    // 編集欄からフォーカスが外れたら、改めて隠すタイマーを動かす。
    window.addEventListener("focusout", scheduleHide);
    return () => {
      window.removeEventListener("mousemove", onMouseMove);
      window.removeEventListener("focusout", scheduleHide);
      window.clearTimeout(hideTimer);
    };
  }, [entry.path]);

  // ヘッダーは本文の上に重なるので、本文の先頭が隠れないよう、
  // 実際のヘッダーの高さぶんだけ上に余白を取る。
  const [overlayHeight, setOverlayHeight] = useState(0);
  useLayoutEffect(() => {
    const el = overlayRef.current;
    if (!el) return;
    const observer = new ResizeObserver(() => setOverlayHeight(el.offsetHeight));
    observer.observe(el);
    setOverlayHeight(el.offsetHeight);
    return () => observer.disconnect();
  }, []);

  // タグ編集フォームの外側をクリックしたら閉じる。
  const tagBarRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!tagsOpen) return;
    const onPointerDown = (e: MouseEvent) => {
      if (tagBarRef.current && !tagBarRef.current.contains(e.target as Node)) {
        setTagsOpen(false);
      }
    };
    window.addEventListener("mousedown", onPointerDown);
    return () => window.removeEventListener("mousedown", onPointerDown);
  }, [tagsOpen]);

  // Esc はまずタグ編集フォームを閉じ、閉じていればプレビュー自体を閉じる。
  // 左右キーで前後のノートへ移動する。
  // 画像ビューアを開いている間は、これらのキーはビューアが先に受けて止める。
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        if (tagsOpen) {
          setTagsOpen(false);
        } else {
          onClose();
        }
        return;
      }
      // Alt + 左右は、ブラウザと同じく履歴の戻る／進む。入力欄の中でも効かせる。
      if (e.altKey && (e.key === "ArrowLeft" || e.key === "ArrowRight")) {
        e.preventDefault();
        goHistory(historyIndex + (e.key === "ArrowLeft" ? -1 : 1));
        return;
      }
      // タグ入力中の左右キーはキャレット移動に使うので奪わない。
      const active = document.activeElement;
      const typing = active instanceof HTMLInputElement || active instanceof HTMLTextAreaElement;
      if (typing) return;

      if (e.key === "ArrowLeft") {
        e.preventDefault();
        onNavigate(-1);
      } else if (e.key === "ArrowRight") {
        e.preventDefault();
        onNavigate(1);
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [onClose, onNavigate, tagsOpen, goHistory, historyIndex]);

  // マウスの戻る／進むボタン（ボタン 3 / 4）でも履歴を移動する。
  // WebView 自体のページ遷移に使われないよう、押した時点で既定の動作を止める。
  useEffect(() => {
    const onMouseDown = (e: MouseEvent) => {
      if (e.button === 3 || e.button === 4) e.preventDefault();
    };
    const onMouseUp = (e: MouseEvent) => {
      if (e.button !== 3 && e.button !== 4) return;
      e.preventDefault();
      goHistory(historyIndex + (e.button === 3 ? -1 : 1));
    };
    window.addEventListener("mousedown", onMouseDown);
    window.addEventListener("mouseup", onMouseUp);
    return () => {
      window.removeEventListener("mousedown", onMouseDown);
      window.removeEventListener("mouseup", onMouseUp);
    };
  }, [goHistory, historyIndex]);

  const contentRef = useRef<HTMLDivElement>(null);

  // ヘッダーがスクロールバーの上に重なると、どこまで読んだかが分かりにくい。
  // スクロールバーの幅を測って、ヘッダーの右端をその手前で止める。
  // スクロールバーは本文の長さやウィンドウの大きさで出たり消えたりするので、
  // スクロール領域とその中身の両方の大きさの変化を見て測り直す。
  const [scrollbarWidth, setScrollbarWidth] = useState(0);
  useLayoutEffect(() => {
    const el = contentRef.current;
    if (!el) return;
    const measure = () => setScrollbarWidth(el.offsetWidth - el.clientWidth);
    const observer = new ResizeObserver(measure);
    observer.observe(el);
    for (const child of el.children) observer.observe(child);
    measure();
    return () => observer.disconnect();
  }, [entry.path, entry.cloudOnly]);

  // 読んでいる位置を履歴へ知らせておき、戻ってきたときにその位置から再開する。
  useEffect(() => {
    const el = contentRef.current;
    if (!el) return;
    const onScroll = () => reportScroll(el.scrollTop);
    el.addEventListener("scroll", onScroll, { passive: true });
    return () => el.removeEventListener("scroll", onScroll);
  }, [reportScroll]);

  // 移動するたびに、その項目のスクロール位置へ合わせる（新しく開いたノートなら先頭）。
  // 本文を読み込むまで高さが決まらないので、読み込み終わるのを待つ。
  // 同じノートへ戻った場合は読み直しが起きないので、その場で合わせる。
  const pendingScroll = useRef<number | null>(null);
  const shownPath = useRef<string | null>(null);
  const applyPendingScroll = () => {
    const el = contentRef.current;
    if (!el || pendingScroll.current === null) return;
    el.scrollTop = pendingScroll.current;
    pendingScroll.current = null;
    reportScroll(el.scrollTop);
  };
  useLayoutEffect(() => {
    pendingScroll.current = history.current?.scrollTop ?? 0;
    const samePath = shownPath.current === entry.path;
    shownPath.current = entry.path;
    if (entry.cloudOnly || samePath) applyPendingScroll();
    // 移動（navId）ごとに一度だけ合わせる。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [navId]);

  const dirty = draft.length !== entry.tags.length || draft.some((t, i) => t !== entry.tags[i]);

  const save = async () => {
    setSaving(true);
    try {
      const result = await setTags(entry.path, draft);
      if (!result.ok) {
        onError(result.error ?? "タグの保存に失敗しました");
        setDraft(entry.tags);
        return;
      }
      if (result.entry) onEntryUpdated(result.entry);
    } catch (err) {
      onError(`タグの保存に失敗しました: ${String(err)}`);
    } finally {
      setSaving(false);
    }
  };

  const openEditor = async () => {
    try {
      await openInEditor(entry.path);
    } catch (err) {
      onError(String(err));
    }
  };

  // ヘッダー・フッターは半透明にして、下を流れる本文をうっすら見せる。
  // 文字が重なっても読めるよう背景はぼかし、罫線も地に合わせて薄める。
  const glassBar = "border-line/70 bg-surface/75 backdrop-blur-md backdrop-saturate-150";
  // ヘッダーの高さ＋ひと呼吸ぶん下から本文を始める。
  const contentTop = overlayHeight + 24;

  return (
    <div className="fixed inset-0 z-[100]" role="dialog" aria-modal="true">
      <div className="relative size-full overflow-hidden bg-canvas">
        {/* ---- 本文 ---- */}
        <div ref={contentRef} className="size-full overflow-auto">
          {entry.cloudOnly && (
            <div className="min-h-full" style={{ paddingTop: contentTop }}>
              <CloudOnlyNotice entry={entry} onFetched={onEntryUpdated} onError={onError} />
            </div>
          )}
          {!entry.cloudOnly && (
            <div
              // 幅は本文（全角 38 文字 = 18px × 38 = 42.75rem = 171）に左右の余白（5 × 2）、
              // 間隔（6）、関連ページの欄（56）を足したもの。
              className="mx-auto flex min-h-full max-w-243 items-start justify-center gap-6 px-5 pb-18"
              style={{ paddingTop: contentTop }}
            >
              {/* 本文の 1 行が長くなりすぎないよう、横幅は本文の最大幅（全角 38 文字）で止める。
                  ウィンドウが狭いときは本文の側が縮む。 */}
              <div className="min-w-0 max-w-171 flex-1">
                <MarkdownPreview
                  entry={entry}
                  onFollowLink={onFollowLink}
                  onOpenImage={(path, alt) => setImage({ path, alt })}
                  onLoaded={applyPendingScroll}
                />
              </div>
              {/* 関連ページは本文の脇に置き、本文と一緒にスクロールする。 */}
              <aside className="w-56 shrink-0" aria-label="関連ページ">
                {/* 読み込み中は空けておき、「無い」表示が一瞬出るのを避ける。 */}
                {related && (
                  <RelatedPages
                    related={related}
                    tagPage={entry.tagPage}
                    onOpen={(page) => onFollowLink(page.title, page.path)}
                    onTagClick={onTagClick}
                  />
                )}
              </aside>
            </div>
          )}
        </div>

        {/* ---- ヘッダー／タグのオーバーレイ ---- */}
        <div
          ref={overlayRef}
          className={`absolute inset-x-0 top-0 z-10 border-b ${glassBar} pb-3 transition-opacity duration-300 ${
            overlayVisible ? "opacity-100" : "pointer-events-none opacity-0"
          }`}
          // スクロールバーを隠さないよう、右端はスクロールバーの手前で止める。
          style={{ right: scrollbarWidth }}
        >
          <header className="flex items-start gap-4 px-5 pt-4 pb-2.5">
            {historyLength > 1 && (
              <HistoryNav items={history.items} index={historyIndex} onGo={goHistory} />
            )}
            <div className="min-w-0 flex-1">
              <h2 className="m-0 text-lg leading-snug break-words">{entry.title}</h2>
              {entry.tagPage && (
                <button
                  type="button"
                  className="mt-0.5 inline-flex max-w-full items-center gap-1 text-xs font-semibold text-accent-ink hover:underline"
                  title={`#${entry.tagPage} で絞り込む`}
                  onClick={() => onSearchTag(entry.tagPage!)}
                >
                  <BookOpenIcon className="size-3.5 shrink-0" aria-hidden="true" />
                  <span className="truncate">#{entry.tagPage} のタグページ</span>
                </button>
              )}
              <p
                className="mt-0.5 mb-0 truncate text-xs text-ink-faint"
                title={entry.path}
              >
                {entry.relPath}
              </p>
            </div>
            {/* クラウド上にだけあるファイルは、開くとダウンロードが始まるので出さない。 */}
            {!entry.cloudOnly && (
              <Button
                variant="ghost"
                onClick={() => void openEditor()}
                title="拡張子に紐づいたエディタで開く"
              >
                <PencilSquareIcon className="size-4" aria-hidden="true" />
                エディタで開く
              </Button>
            )}
            <Button variant="ghost" onClick={onClose} title="閉じる（Esc）">
              <XMarkIcon className="size-4" aria-hidden="true" />
              閉じる
            </Button>
          </header>

          <div className="px-5 pb-3" ref={tagBarRef}>
            {tagsOpen ? (
              <div className="flex flex-col gap-2">
                <TagEditor
                  tags={draft}
                  onChange={setDraft}
                  disabled={!entry.writable || saving}
                  placeholder={
                    entry.writable ? "タグを追加（Enter で確定）" : "読み取り専用のため編集できません"
                  }
                />
                <div className="flex flex-wrap items-center gap-2 text-xs">
                  {!entry.writable && (
                    <span className="text-danger">
                      このファイルは読み取り専用のため、タグを書き込めません。
                    </span>
                  )}
                  <span className="flex-1" />
                  <Button
                    variant="default"
                    disabled={!dirty || saving}
                    onClick={() => setDraft(entry.tags)}
                  >
                    変更を取り消す
                  </Button>
                  <Button
                    variant="primary"
                    disabled={!dirty || saving || !entry.writable}
                    onClick={() => void save()}
                  >
                    {saving ? "保存中…" : "ファイルへ保存"}
                  </Button>
                  <Button variant="ghost" onClick={() => setTagsOpen(false)}>
                    閉じる
                  </Button>
                </div>
              </div>
            ) : (
              <div className="flex flex-wrap items-center gap-2">
                {entry.tags.length > 0 ? (
                  entry.tags.map((tag) => <TagBadge key={tag} tag={tag} onClick={onTagClick} />)
                ) : (
                  <span className="text-xs text-ink-faint">
                    タグなし
                  </span>
                )}
                <Button
                  variant="ghost"
                  className="ml-auto"
                  aria-expanded={false}
                  onClick={() => setTagsOpen(true)}
                >
                  <TagIcon className="size-4" aria-hidden="true" />
                  タグを編集
                </Button>
              </div>
            )}
          </div>
        </div>

        {/* ---- 前後のノートへの移動 ---- */}
        <div
          className={`absolute inset-x-0 bottom-0 z-10 flex justify-end border-t ${glassBar} px-5 pt-3 pb-4 transition-opacity duration-300 ${
            overlayVisible ? "opacity-100" : "pointer-events-none opacity-0"
          }`}
          // ヘッダーと同じく、スクロールバーの手前で止める。
          style={{ right: scrollbarWidth }}
        >
          <Pager index={index} total={total} onNavigate={onNavigate} />
        </div>
      </div>

      {image && <ImagePreview path={image.path} alt={image.alt} onClose={closeImage} />}
    </div>
  );
}
