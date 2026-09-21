/**
 * カードをクリックしたときに開く詳細プレビュー。
 *
 * 要件どおり画面遷移はせず、同じ画面にオーバーレイとして重ねる。
 * 画像・Markdown・音声のすべてで、ウィンドウ全体を使ってプレビューを優先する
 * レイアウトに統一している。ヘッダーとタグは、マウスを動かした間だけ
 * コンテンツの上に一時的にオーバーレイ表示し、動きが止まればしばらくして消える。
 *
 * タグ編集の入力欄はコンテンツの邪魔になるため常には出さず、タグは読み取り専用の
 * バッジで表示するだけにして、「タグを編集」ボタンを押したときだけ編集フォーム
 * （入力欄・候補・保存操作）を表示する。
 *
 * ヘッダー左端の「戻る／進む」で、リンクや本文中の画像をたどる前の
 * ファイルへ戻れる。履歴そのものは App が持ち、ここは操作と表示だけを受け持つ。
 *
 * 画像の上では背景の色が予測できないため、ヘッダーとタグ UI は暗いグラデーション
 * ＋白文字に固定する（isImage で配色を切り替える）。
 */

import { useEffect, useLayoutEffect, useRef, useState, type CSSProperties } from "react";
import { TagIcon, XMarkIcon } from "@heroicons/react/20/solid";
import { setTags, type Entry } from "../api/taggo";
import type { DetailHistory } from "../hooks/useDetailHistory";
import { AudioPreview } from "./AudioPreview";
import { Button } from "./Button";
import { CloudOnlyNotice } from "./CloudOnlyNotice";
import { HistoryNav } from "./HistoryNav";
import { ImagePreview, WHEEL_COOLDOWN_MS } from "./ImagePreview";
import { MarkdownPreview } from "./MarkdownPreview";
import { Pager } from "./Pager";
import { RelatedPages, useRelatedPages } from "./RelatedPages";
import { TagBadge } from "./TagBadge";
import { TagEditor } from "./TagEditor";

interface Props {
  entry: Entry;
  onClose: () => void;
  onTagClick: (tag: string) => void;
  /** リンクをたどる。実体のパスが分かっている場合は一緒に渡す。 */
  onFollowLink: (target: string, path?: string) => void;
  /** Markdown の本文中の画像を開く。 */
  onOpenImage: (path: string) => void;
  /** 保存後の最新状態を一覧へ返す。 */
  onEntryUpdated: (entry: Entry) => void;
  onError: (message: string) => void;
  /** 前後のファイルへの移動。種類を問わず一覧の並び順で動く。 */
  onNavigate: (direction: 1 | -1) => void;
  /** 一覧における現在位置（0 始まり）。 */
  index: number;
  total: number;
  /** 移動の履歴。戻る／進むと、戻ったときのスクロール位置の復元に使う。 */
  history: DetailHistory;
}

/** UI オーバーレイを自動で隠すまでの無操作時間。 */
const OVERLAY_HIDE_MS = 2200;
/**
 * 端に着いた直後のホイールではページ送りしないための待ち時間。
 * 本文を読み進めた勢い（慣性スクロール）のまま次のファイルへ飛ばないようにする。
 */
const EDGE_SETTLE_MS = 300;

/**
 * target から container までのどこかに、direction の向きへまだスクロールできる
 * 要素があるかを返す。関連ページの欄など、内側のスクロールを優先するために使う。
 */
function canScrollFurther(target: EventTarget | null, container: HTMLElement, direction: 1 | -1) {
  for (let el = target instanceof Element ? target : null; el; el = el.parentElement) {
    if (el instanceof HTMLElement && el.scrollHeight > el.clientHeight + 1) {
      const overflowY = getComputedStyle(el).overflowY;
      if (el === container || overflowY === "auto" || overflowY === "scroll") {
        const more =
          direction === 1 ? el.scrollTop + el.clientHeight < el.scrollHeight - 1 : el.scrollTop > 0;
        if (more) return true;
      }
    }
    if (el === container) break;
  }
  return false;
}

export function DetailPanel({
  entry,
  onClose,
  onTagClick,
  onFollowLink,
  onOpenImage,
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
  // 中身をまだ持っていないファイルは、画像であっても黒地のビューアにはしない。
  // 取り込むかどうかを尋ねる案内を、通常のレイアウトで出す。
  const isImage = entry.kind === "image" && !entry.cloudOnly;
  const isMarkdown = entry.kind === "markdown";

  // 関連ページ。Markdown は常に本文を左、関連ページを右の 2 カラムにする。
  // ノートを行き来してもレイアウトが跳ねないよう、リンクが 1 件も無くても右の列は残す。
  const related = useRelatedPages(entry);

  // 別のエントリに切り替わったら編集中の内容を捨て、編集フォームも閉じる。
  useEffect(() => {
    setDraft(entry.tags);
    setTagsOpen(false);
  }, [entry.path, entry.tags]);

  // マウスが動いた直後だけヘッダー／タグ UI を出し、止まればしばらくして隠す。
  // 画像・Markdown・音声のすべてで共通の挙動にしている。
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

  // ヘッダーはコンテンツの上に重なるので、Markdown と音声では本文の先頭が
  // 隠れないよう、実際のヘッダーの高さぶんだけ上に余白を取る。
  // （画像は全面表示が主役なので、あえて重ねたままにする。）
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
  // 左右キーで前後のファイルへ移動する。画像に限らず種類は問わない。
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

  // 音声でも、ホイールで前後のファイルへ移動する（画像は ImagePreview が扱う）。
  // Markdown は本文を読むスクロールと紛らわしいので、ホイールでは送らない。
  // 内側にスクロールできる欄があればそちらを優先し、端まで来ているときだけ送る。
  // タグ編集欄が出ている間は、編集中のファイルから離れないよう送らない。
  const contentRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const el = contentRef.current;
    if (!el || isImage || isMarkdown || tagsOpen) return;

    let cooling = false;
    let lastScroll = 0;
    const onScroll = () => {
      lastScroll = performance.now();
    };
    const onWheel = (e: WheelEvent) => {
      // Ctrl + ホイールはブラウザの拡大・縮小に任せる。
      if (e.ctrlKey || e.deltaY === 0) return;
      const direction = e.deltaY > 0 ? 1 : -1;
      if (canScrollFurther(e.target, el, direction)) return;
      e.preventDefault();
      if (cooling || performance.now() - lastScroll < EDGE_SETTLE_MS) return;
      cooling = true;
      window.setTimeout(() => {
        cooling = false;
      }, WHEEL_COOLDOWN_MS);
      onNavigate(direction);
    };
    // 内側の要素のスクロールも拾えるよう、scroll はキャプチャで受ける。
    el.addEventListener("scroll", onScroll, { capture: true, passive: true });
    el.addEventListener("wheel", onWheel, { passive: false });
    return () => {
      el.removeEventListener("scroll", onScroll, { capture: true });
      el.removeEventListener("wheel", onWheel);
    };
  }, [isImage, isMarkdown, tagsOpen, onNavigate]);

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
  }, [entry.path, entry.kind, entry.cloudOnly]);

  // 読んでいる位置を履歴へ知らせておき、戻ってきたときにその位置から再開する。
  useEffect(() => {
    const el = contentRef.current;
    if (!el) return;
    const onScroll = () => reportScroll(el.scrollTop);
    el.addEventListener("scroll", onScroll, { passive: true });
    return () => el.removeEventListener("scroll", onScroll);
  }, [reportScroll]);

  // 移動するたびに、その項目のスクロール位置へ合わせる（新しく開いたファイルなら先頭）。
  // Markdown は本文を読み込むまで高さが決まらないので、読み込み終わるのを待つ。
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
    if (!isMarkdown || entry.cloudOnly || samePath) applyPendingScroll();
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

  // 画像の上かどうかで、文字色と地味なボタンの見た目を切り替える。
  const plainVariant = isImage ? "hud" : "default";
  const ghostVariant = isImage ? "hudGhost" : "ghost";
  // Markdown と音声のヘッダー・フッターは半透明にして、下を流れる本文をうっすら見せる。
  // 文字が重なっても読めるよう背景はぼかし、罫線も地に合わせて薄める。
  const glassBar = "border-line/70 bg-surface/75 backdrop-blur-md backdrop-saturate-150";
  // ヘッダーの高さ＋ひと呼吸ぶん下から本文を始める。
  const contentTop = overlayHeight + 24;

  return (
    <div className="fixed inset-0 z-[100]" role="dialog" aria-modal="true">
      <div className="relative size-full overflow-hidden bg-canvas">
        {/* ---- コンテンツ本体（種別ごとに切り替え） ----
            画像は ImagePreview 側が独自にスクロールを持つため、
            ここで二重にスクロールコンテナを作らない。 */}
        <div
          ref={contentRef}
          className={isImage ? "size-full overflow-hidden bg-black" : "size-full overflow-auto"}
        >
          {entry.cloudOnly && (
            <div className="min-h-full" style={{ paddingTop: contentTop }}>
              <CloudOnlyNotice entry={entry} onFetched={onEntryUpdated} onError={onError} />
            </div>
          )}
          {!entry.cloudOnly && entry.kind === "image" && (
            <ImagePreview
              entry={entry}
              uiVisible={overlayVisible}
              onNavigate={onNavigate}
              wheelNavigation={!tagsOpen}
              index={index}
              total={total}
            />
          )}
          {!entry.cloudOnly && entry.kind === "markdown" && (
            <div
              // 幅は本文（全角 38 文字 = 18px × 38 = 42.75rem = 171）に左右の余白を足したもの。
              // さらに間隔（6）と関連ページの列（56）を足す。
              // 画面の半分ほどのウィンドウ（約 1000px）でも 2 カラムに収まるよう、
              // 横に並べるときは余白と間隔を詰め、60rem（960px）から横に並べる。
              // それより少し狭いだけなら、本文の列が縮んで 2 カラムのまま収まる。
              className="mx-auto min-h-full max-w-243 px-7 pb-18 min-[60rem]:px-5"
              style={{ paddingTop: contentTop }}
            >
              <div className="flex flex-col gap-8 min-[60rem]:flex-row min-[60rem]:items-start min-[60rem]:justify-center min-[60rem]:gap-6">
                {/* 本文の 1 行が長くなりすぎないよう、横幅は本文の最大幅（全角 38 文字）で止める。 */}
                <div className="min-w-0 flex-1 min-[60rem]:max-w-171">
                  <MarkdownPreview
                    entry={entry}
                    onFollowLink={onFollowLink}
                    onOpenImage={onOpenImage}
                    onLoaded={applyPendingScroll}
                  />
                </div>
                <div
                  // 本文が長くても関連ページが見えているよう、横に並ぶ幅では貼り付ける。
                  // ヘッダーに隠れない位置で止め、収まらないぶんはこの中だけでスクロールする。
                  className="w-full shrink-0 min-[60rem]:sticky min-[60rem]:top-(--related-top) min-[60rem]:max-h-[calc(100dvh-var(--related-top)-5rem)] min-[60rem]:w-56 min-[60rem]:overflow-auto"
                  style={{ "--related-top": `${contentTop}px` } as CSSProperties}
                >
                  {/* 読み込み中は空けておき、「無い」表示が一瞬出るのを避ける。 */}
                  {related && (
                    <RelatedPages
                      related={related}
                      onOpen={(page) => onFollowLink(page.title, page.path)}
                      onTagClick={onTagClick}
                    />
                  )}
                </div>
              </div>
            </div>
          )}
          {!entry.cloudOnly && entry.kind === "audio" && (
            <div className="mx-auto min-h-full max-w-205 px-7 pb-18" style={{ paddingTop: contentTop }}>
              <AudioPreview entry={entry} />
            </div>
          )}
        </div>

        {/* ---- ヘッダー／タグのオーバーレイ ---- */}
        <div
          ref={overlayRef}
          className={`absolute inset-x-0 top-0 z-10 transition-opacity duration-300 ${
            isImage
              ? "bg-linear-to-b from-black/70 to-transparent pb-7 text-white"
              : `border-b ${glassBar} pb-3`
          } ${overlayVisible ? "opacity-100" : "pointer-events-none opacity-0"}`}
          // スクロールバーを隠さないよう、右端はスクロールバーの手前で止める。
          style={{ right: scrollbarWidth }}
        >
          <header className="flex items-start gap-4 px-5 pt-4 pb-2.5">
            {historyLength > 1 && (
              <HistoryNav
                items={history.items}
                index={historyIndex}
                onGo={goHistory}
                variant={ghostVariant}
                onImage={isImage}
              />
            )}
            <div className="min-w-0 flex-1">
              <h2 className="m-0 text-lg leading-snug break-words">{entry.title}</h2>
              <p
                className={`mt-0.5 mb-0 truncate text-xs ${isImage ? "text-white/70" : "text-ink-faint"}`}
                title={entry.path}
              >
                {entry.relPath}
              </p>
            </div>
            <Button variant={ghostVariant} onClick={onClose} title="閉じる（Esc）">
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
                    <span className={isImage ? "text-[#ff8a72]" : "text-danger"}>
                      このファイルは読み取り専用のため、タグを書き込めません。
                    </span>
                  )}
                  <span className="flex-1" />
                  <Button
                    variant={plainVariant}
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
                  <Button variant={ghostVariant} onClick={() => setTagsOpen(false)}>
                    閉じる
                  </Button>
                </div>
              </div>
            ) : (
              <div className="flex flex-wrap items-center gap-2">
                {entry.tags.length > 0 ? (
                  entry.tags.map((tag) => <TagBadge key={tag} tag={tag} onClick={onTagClick} />)
                ) : (
                  <span className={`text-xs ${isImage ? "text-white/70" : "text-ink-faint"}`}>
                    タグなし
                  </span>
                )}
                <Button
                  variant={ghostVariant}
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

        {/* ---- 前後のファイルへの移動 ----
            画像は ImagePreview の HUD に倍率の操作と並べて出すので、
            ここでは Markdown と音声のぶんだけを下部に置く。
            左右と下の余白は画像の HUD と同じにしてあり、種類を切り替えても
            ボタンが同じ位置に出る。 */}
        {!isImage && (
          <div
            className={`absolute inset-x-0 bottom-0 z-10 flex justify-end border-t ${glassBar} px-5 pt-3 pb-4 transition-opacity duration-300 ${
              overlayVisible ? "opacity-100" : "pointer-events-none opacity-0"
            }`}
            // ヘッダーと同じく、スクロールバーの手前で止める。
            style={{ right: scrollbarWidth }}
          >
            <Pager index={index} total={total} onNavigate={onNavigate} />
          </div>
        )}
      </div>
    </div>
  );
}
