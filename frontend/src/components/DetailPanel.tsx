/**
 * カードをクリックしたときに開く、ノートの画面。本文は常に編集できる（NoteEditor）。
 *
 * 要件どおり画面遷移はせず、同じ画面にオーバーレイとして重ねる。
 * ウィンドウ全体を使って本文を優先するレイアウトにしている。ヘッダーとタグは、
 * マウスを動かした間だけ本文の上に一時的にオーバーレイ表示し、動きが止まれば
 * しばらくして消える。
 *
 * ヘッダーのタグ（本文の [[タグ]]）はバッジで表示するだけで、付け外しは本文を書き換えて行う。
 *
 * ヘッダーの「ピン留め」で、このノートを一覧の先頭にピン留めする。
 * 「名前を変更」で、ファイル名と、このページを指す [[タグ]] とリンクをまとめて書き換える。
 *
 * まだ無いページ（ページの無いタグや、行き先の無いリンク）も、ファイルを作らずにここで開く。
 * 本文の代わりに「md ファイルを作成する」ボタンを出し、関連ページはふつうのノートと同じく並べる。
 * 作ると、そのまま本文を書き始められる。
 *
 * ヘッダー左端の「戻る／進む」で、リンクをたどる前のノートや、開く前の一覧へ戻れる。
 * ノートの画面を閉じるボタンは置かず、一覧へは「戻る」か Esc で戻る。
 * 履歴そのものと、Alt + 左右・Ctrl + [ / ]・マウスの戻る／進むボタンは App が受け持つ。
 *
 * フッターには本文の文字数を表示する。
 *
 * 本文中の画像をクリックすると、画像ビューアをこの上に重ねて開く。画像はノートでは
 * ないので履歴には積まず、ビューアを閉じればそのまま同じ位置の本文へ戻る。
 */

import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { DocumentPlusIcon, MapPinIcon, PencilIcon } from "@heroicons/react/20/solid";
import type { Entry } from "../api/taggo";
import type { NavHistory } from "../hooks/useNavHistory";
import { NoteEditor } from "../editor/NoteEditor";
import { Button } from "./Button";
import { HistoryNav } from "./HistoryNav";
import { ImagePreview } from "./ImagePreview";
import { missingLinksOf, missingTagsOf, RelatedPages, useRelatedPages } from "./RelatedPages";
import { TagBadge } from "./TagBadge";

interface Props {
  entry: Entry;
  onClose: () => void;
  onTagClick: (tag: string) => void;
  /**
   * リンクをたどる。link は本文に書かれたパスを、フラグメントを除いてデコードしたもの。
   * 実体のパスが分かっている場合は一緒に渡す。行き先がまだ無ければ、まだ無いページとして開く。
   */
  onFollowLink: (link: string, path?: string) => void;
  /** タグのページ（ファイル名がそのタグのノート）を開く。まだ無ければ、まだ無いページとして開く。 */
  onFollowTag: (tag: string) => void;
  /** まだ無いページの md ファイルを作る。作ると、このページがそのまま編集画面になる。 */
  onCreatePage: (entry: Entry) => Promise<void>;
  /** 一覧の先頭にピン留めしているか。 */
  pinned: boolean;
  onTogglePin: (pinned: boolean) => void;
  /** 名前の変更ダイアログを開く。 */
  onRename: () => void;
  onError: (message: string) => void;
  /** 移動の履歴。戻る／進むと、戻ったときのスクロール位置の復元に使う。 */
  history: NavHistory;
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
  onFollowLink,
  onFollowTag,
  onCreatePage,
  pinned,
  onTogglePin,
  onRename,
  onError,
  history,
}: Props) {
  const { go: goHistory, reportScroll, navId } = history;
  // 本文中の画像から開いた画像ビューア。閉じれば null に戻り、本文がそのまま見える。
  const [image, setImage] = useState<OpenImage | null>(null);

  // 関連ページ。本文の下に、タグごとのグループにしてグリッドで並べる。
  const related = useRelatedPages(entry);
  const missingLinks = useMemo(() => missingLinksOf(related), [related]);
  const missingTags = useMemo(() => missingTagsOf(related), [related]);

  // 本文の文字数。本文を読み込むまでと、本文の無いページでは null。
  const [charCount, setCharCount] = useState<number | null>(null);

  // 別のノートへ移ったら、前のノートの画像は閉じ、文字数も数え直す。
  useEffect(() => {
    setImage(null);
    setCharCount(null);
  }, [entry.path]);
  const closeImage = useCallback(() => setImage(null), []);
  const openImage = useCallback((path: string, alt?: string) => setImage({ path, alt }), []);
  const countBody = useCallback((body: string) => setCharCount(countChars(body)), []);

  // マウスが動いた直後だけヘッダー／タグ UI を出し、止まればしばらくして隠す。
  // ただしヘッダーのボタンなどにフォーカスが残っている間は隠さない。
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

  // Esc で一覧へ戻る。画像ビューアを開いている間は、ビューアが先に受けて止める。
  // 本文の入力補完を閉じるなど、エディタが使った Esc では戻らない。
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape" && !e.defaultPrevented) onClose();
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [onClose]);

  const contentRef = useRef<HTMLDivElement>(null);

  // 開いたら、フォーカスをこの画面の中へ移す。一覧から開いた直後は後ろのカードに
  // フォーカスが残っていて、Enter などのキーを後ろの一覧が受けてしまうため。
  // 編集画面などに既にフォーカスがあれば、そのままにする。
  const rootRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const root = rootRef.current;
    if (root && !root.contains(document.activeElement)) root.focus({ preventScroll: true });
  }, [entry.path]);

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
  }, [entry.path, entry.missing]);

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
    pendingScroll.current = history.current.kind === "note" ? history.current.scrollTop : 0;
    const samePath = shownPath.current === entry.path;
    shownPath.current = entry.path;
    // 本文を読み込まないページ（まだ無い）は、待たずに合わせる。
    if (entry.missing || samePath) applyPendingScroll();
    // 移動（navId）ごとに一度だけ合わせる。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [navId]);


  // ヘッダー・フッターは半透明にして、下を流れる本文をうっすら見せる。
  // 文字が重なっても読めるよう背景はぼかし、罫線も地に合わせて薄める。
  const glassBar = "border-line/70 bg-surface/75 backdrop-blur-md backdrop-saturate-150";
  // ヘッダーの高さ＋ひと呼吸ぶん下から本文を始める。
  const contentTop = overlayHeight + 24;

  return (
    <div
      ref={rootRef}
      className="fixed inset-0 z-[100] outline-hidden"
      role="dialog"
      aria-modal="true"
      // フォーカスを受けられるようにするが、Tab でたどる順には入れない。
      tabIndex={-1}
    >
      <div className="relative size-full overflow-hidden bg-canvas">
        {/* ---- 本文 ---- */}
        <div ref={contentRef} className="size-full overflow-auto">
          <div
            // 関連ページのグリッドは本文より広く取り、カードを横に多く並べる。
            className="mx-auto flex min-h-full max-w-243 flex-col items-center px-5 pb-18"
            style={{ paddingTop: contentTop }}
          >
            {/* 本文の 1 行が長くなりすぎないよう、横幅は本文の最大幅
                （全角 38 文字 = 18px × 38 = 42.75rem = 171）で止める。 */}
            <div className="w-full max-w-171">
              {entry.missing ? (
                <MissingPageNotice entry={entry} onCreate={onCreatePage} />
              ) : (
                <NoteEditor
                  entry={entry}
                  onFollowLink={onFollowLink}
                  missingLinks={missingLinks}
                  onFollowTag={onFollowTag}
                  missingTags={missingTags}
                  onOpenImage={openImage}
                  onError={onError}
                  onLoaded={applyPendingScroll}
                  onBodyChange={countBody}
                />
              )}
            </div>
            {/* 関連ページは本文の下にグリッドで並べ、本文と一緒にスクロールする。
                読み込み中は出さず、「無い」表示が一瞬出るのを避ける。 */}
            {related && (
              <aside className="mt-14 w-full border-t border-line pt-6" aria-label="関連ページ">
                <RelatedPages
                  related={related}
                  onOpen={(page) => {
                    // 行き先があれば開く。まだ無ければ、リンクなら書かれた場所に、
                    // タグならフォルダの直下に作る。
                    if (page.path || page.target) {
                      onFollowLink(page.target ?? "", page.path);
                    } else if (page.tag) {
                      onFollowTag(page.tag);
                    }
                  }}
                  onTagClick={onTagClick}
                />
              </aside>
            )}
          </div>
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
            <HistoryNav items={history.items} index={history.index} onGo={goHistory} />
            <div className="min-w-0 flex-1">
              <h2 className="m-0 text-lg leading-snug break-words">{entry.title}</h2>
              <p
                className="mt-0.5 mb-0 truncate text-xs text-ink-faint"
                title={entry.path}
              >
                {entry.missing ? `${entry.relPath}（まだありません）` : entry.relPath}
              </p>
            </div>
            {/* まだ無いページは一覧に無いので、ピン留めも名前の変更もできない。 */}
            {!entry.missing && (
            <Button
              variant={pinned ? "primary" : "ghost"}
              aria-pressed={pinned}
              onClick={() => onTogglePin(!pinned)}
              title={pinned ? "ピン留めを外す" : "一覧の先頭にピン留め"}
            >
              <MapPinIcon className="size-4" aria-hidden="true" />
              {pinned ? "ピン留め中" : "ピン留め"}
            </Button>
            )}
            {!entry.missing && (
              <Button
                variant="ghost"
                onClick={onRename}
                title="ファイル名と、このページを指す [[タグ]] とリンクを書き換える"
              >
                <PencilIcon className="size-4" aria-hidden="true" />
                名前を変更
              </Button>
            )}
          </header>

          {/* タグは本文の [[タグ]] から読んだもの。付け外しは本文を書き換えて行う。 */}
          {!entry.missing && (
            <div className="flex flex-wrap items-center gap-2 px-5 pb-3">
              {entry.tags.length > 0 ? (
                entry.tags.map((tag) => <TagBadge key={tag} tag={tag} onClick={onTagClick} />)
              ) : (
                <span className="text-xs text-ink-faint">タグなし</span>
              )}
            </div>
          )}
        </div>

        {/* ---- 文字数のフッター ---- */}
        {charCount !== null && (
          <div
            className={`absolute inset-x-0 bottom-0 z-10 flex justify-end border-t ${glassBar} px-5 py-2.5 transition-opacity duration-300 ${
              overlayVisible ? "opacity-100" : "pointer-events-none opacity-0"
            }`}
            // ヘッダーと同じく、スクロールバーの手前で止める。
            style={{ right: scrollbarWidth }}
          >
            <span className="text-xs tabular-nums text-ink-faint">
              {charCount.toLocaleString()} 文字
            </span>
          </div>
        )}
      </div>

      {image && <ImagePreview path={image.path} alt={image.alt} onClose={closeImage} />}
    </div>
  );
}

/**
 * 本文の文字数を数える。改行は数えない。
 * 絵文字や結合文字が 2 文字以上に数えられないよう、見た目の 1 文字（書記素）ごとに数える。
 */
function countChars(source: string): number {
  const text = source.replace(/\r?\n/g, "");
  return [...new Intl.Segmenter("ja", { granularity: "grapheme" }).segment(text)].length;
}

/**
 * まだ無いページで、本文の代わりに出す案内。md ファイルを作ると、このページが
 * そのまま編集画面に切り替わる。
 */
function MissingPageNotice({ entry, onCreate }: { entry: Entry; onCreate: (entry: Entry) => Promise<void> }) {
  const [creating, setCreating] = useState(false);
  return (
    <div className="flex flex-col items-center gap-3 rounded-lg border border-dashed border-line px-6 py-10 text-center">
      <p className="m-0 text-base font-semibold text-ink">このノートはまだありません</p>
      <p className="m-0 text-sm text-ink-muted">
        「{entry.name}」を作ると、ここで本文を書けるようになります。
        下に、このページを指しているノートが並びます。
      </p>
      <Button
        variant="primary"
        disabled={creating}
        onClick={() => {
          setCreating(true);
          void onCreate(entry).finally(() => setCreating(false));
        }}
      >
        <DocumentPlusIcon className="size-4" aria-hidden="true" />
        md ファイルを作成する
      </Button>
    </div>
  );
}
