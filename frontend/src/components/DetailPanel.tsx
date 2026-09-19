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
 */

import { useEffect, useRef, useState } from "react";
import { setTags, type Entry } from "../api/taggo";
import { AudioPreview } from "./AudioPreview";
import { ImagePreview } from "./ImagePreview";
import { MarkdownPreview } from "./MarkdownPreview";
import { TagBadge } from "./TagBadge";
import { TagEditor } from "./TagEditor";
import "./DetailPanel.css";

interface Props {
  entry: Entry;
  onClose: () => void;
  onTagClick: (tag: string) => void;
  onFollowLink: (target: string) => void;
  /** 保存後の最新状態を一覧へ返す。 */
  onEntryUpdated: (entry: Entry) => void;
  onError: (message: string) => void;
  /** 画像プレビューでの前後移動。画像以外を見ているときは呼ばれない。 */
  onNavigateImage: (direction: 1 | -1) => void;
  /** 画像一覧における現在位置（0 始まり）。画像でなければ -1。 */
  imageIndex: number;
  imageTotal: number;
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
  onNavigateImage,
  imageIndex,
  imageTotal,
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
  // 画像プレビュー中は左右キーで前後の画像へ移動する。
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
      if (!isImage) return;
      // タグ入力中の左右キーはキャレット移動に使うので奪わない。
      const active = document.activeElement;
      const typing = active instanceof HTMLInputElement || active instanceof HTMLTextAreaElement;
      if (typing) return;

      if (e.key === "ArrowLeft") {
        e.preventDefault();
        onNavigateImage(-1);
      } else if (e.key === "ArrowRight") {
        e.preventDefault();
        onNavigateImage(1);
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [onClose, isImage, onNavigateImage, tagsOpen]);

  const dirty =
    draft.length !== entry.tags.length || draft.some((t, i) => t !== entry.tags[i]);

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

  const headerAndTags = (
    <>
      <header className="detail__header">
        <div className="detail__heading">
          <h2 className="detail__title">{entry.title}</h2>
          <p className="detail__path" title={entry.path}>
            {entry.relPath}
          </p>
        </div>
        <button className="btn btn--ghost detail__close" type="button" onClick={onClose}>
          閉じる
        </button>
      </header>

      <div className="detail__tagbar" ref={tagBarRef}>
        {tagsOpen ? (
          <div className="detail__tagform">
            <TagEditor
              tags={draft}
              onChange={setDraft}
              disabled={!entry.writable || saving}
              placeholder={entry.writable ? "タグを追加（Enter で確定）" : "読み取り専用のため編集できません"}
            />
            <div className="detail__tagactions">
              {!entry.writable && (
                <span className="detail__warning">
                  このファイルは読み取り専用のため、タグを書き込めません。
                </span>
              )}
              <span className="detail__spacer" />
              <button
                className="btn"
                type="button"
                disabled={!dirty || saving}
                onClick={() => setDraft(entry.tags)}
              >
                変更を取り消す
              </button>
              <button
                className="btn btn--primary"
                type="button"
                disabled={!dirty || saving || !entry.writable}
                onClick={() => void save()}
              >
                {saving ? "保存中…" : "ファイルへ保存"}
              </button>
              <button className="btn btn--ghost" type="button" onClick={() => setTagsOpen(false)}>
                閉じる
              </button>
            </div>
          </div>
        ) : (
          <div className="detail__tagsummary">
            {entry.tags.length > 0 ? (
              entry.tags.map((tag) => <TagBadge key={tag} tag={tag} onClick={onTagClick} />)
            ) : (
              <span className="detail__tagempty">タグなし</span>
            )}
            <button
              className="btn btn--ghost detail__tagtoggle"
              type="button"
              aria-expanded={false}
              onClick={() => setTagsOpen(true)}
            >
              <span aria-hidden="true">🏷</span>
              タグを編集
            </button>
          </div>
        )}
      </div>
    </>
  );

  return (
    <div className={`detail${isImage ? " detail--image" : ""}`} role="dialog" aria-modal="true">
      <div className="detail__panel">
        <div className={`detail__stage detail__stage--${entry.kind}`}>
          {entry.kind === "image" && (
            <ImagePreview
              entry={entry}
              uiVisible={overlayVisible}
              onNavigate={onNavigateImage}
              imageIndex={imageIndex}
              imageTotal={imageTotal}
            />
          )}
          {entry.kind === "markdown" && (
            <div className="detail__stagepad">
              <MarkdownPreview entry={entry} onFollowLink={onFollowLink} />
            </div>
          )}
          {entry.kind === "audio" && (
            <div className="detail__stagepad">
              <AudioPreview entry={entry} />
            </div>
          )}
        </div>

        <div
          ref={overlayRef}
          className={`detail__overlay${overlayVisible ? " is-visible" : ""}`}
        >
          {headerAndTags}
        </div>
      </div>
    </div>
  );
}
