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
 * 画像の上では背景の色が予測できないため、ヘッダーとタグ UI は暗いグラデーション
 * ＋白文字に固定する（isImage で配色を切り替える）。
 */

import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { TagIcon, XMarkIcon } from "@heroicons/react/20/solid";
import { setTags, type Entry } from "../api/taggo";
import { AudioPreview } from "./AudioPreview";
import { Button } from "./Button";
import { ImagePreview } from "./ImagePreview";
import { MarkdownPreview } from "./MarkdownPreview";
import { Pager } from "./Pager";
import { TagBadge } from "./TagBadge";
import { TagEditor } from "./TagEditor";

interface Props {
  entry: Entry;
  onClose: () => void;
  onTagClick: (tag: string) => void;
  onFollowLink: (target: string) => void;
  /** 保存後の最新状態を一覧へ返す。 */
  onEntryUpdated: (entry: Entry) => void;
  onError: (message: string) => void;
  /** 前後のファイルへの移動。種類を問わず一覧の並び順で動く。 */
  onNavigate: (direction: 1 | -1) => void;
  /** 一覧における現在位置（0 始まり）。 */
  index: number;
  total: number;
}

/** UI オーバーレイを自動で隠すまでの無操作時間。 */
const OVERLAY_HIDE_MS = 2200;

export function DetailPanel({
  entry,
  onClose,
  onTagClick,
  onFollowLink,
  onEntryUpdated,
  onError,
  onNavigate,
  index,
  total,
}: Props) {
  const [draft, setDraft] = useState<string[]>(entry.tags);
  const [saving, setSaving] = useState(false);
  const [tagsOpen, setTagsOpen] = useState(false);
  const isImage = entry.kind === "image";

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
  }, [onClose, onNavigate, tagsOpen]);

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
  // ヘッダーの高さ＋ひと呼吸ぶん下から本文を始める。
  const contentTop = overlayHeight + 24;

  return (
    <div className="fixed inset-0 z-[100]" role="dialog" aria-modal="true">
      <div className="relative size-full overflow-hidden bg-canvas">
        {/* ---- コンテンツ本体（種別ごとに切り替え） ----
            画像は ImagePreview 側が独自にスクロールを持つため、
            ここで二重にスクロールコンテナを作らない。 */}
        <div className={isImage ? "size-full overflow-hidden bg-black" : "size-full overflow-auto"}>
          {entry.kind === "image" && (
            <ImagePreview
              entry={entry}
              uiVisible={overlayVisible}
              onNavigate={onNavigate}
              index={index}
              total={total}
            />
          )}
          {entry.kind === "markdown" && (
            <div className="mx-auto min-h-full max-w-205 px-7 pb-18" style={{ paddingTop: contentTop }}>
              <MarkdownPreview entry={entry} onFollowLink={onFollowLink} />
            </div>
          )}
          {entry.kind === "audio" && (
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
              : "border-b border-line bg-surface pb-3"
          } ${overlayVisible ? "opacity-100" : "pointer-events-none opacity-0"}`}
        >
          <header className="flex items-start gap-4 px-5 pt-4 pb-2.5">
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
            className={`absolute inset-x-0 bottom-0 z-10 flex justify-end border-t border-line bg-surface px-5 pt-3 pb-4 transition-opacity duration-300 ${
              overlayVisible ? "opacity-100" : "pointer-events-none opacity-0"
            }`}
          >
            <Pager index={index} total={total} onNavigate={onNavigate} />
          </div>
        )}
      </div>
    </div>
  );
}
