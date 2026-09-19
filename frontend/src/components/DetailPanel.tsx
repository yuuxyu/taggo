/**
 * カードをクリックしたときに開く詳細プレビュー。
 *
 * 要件どおり画面遷移はせず、同じ画面にオーバーレイとして重ねる。
 * ヘッダーにタグ編集フォームを置き、プレビュー本体は種別ごとに切り替える。
 */

import { useEffect, useState } from "react";
import { setTags, type Entry } from "../api/taggo";
import { AudioPreview } from "./AudioPreview";
import { ImagePreview } from "./ImagePreview";
import { MarkdownPreview } from "./MarkdownPreview";
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
}

export function DetailPanel({
  entry,
  onClose,
  onTagClick,
  onFollowLink,
  onEntryUpdated,
  onError,
}: Props) {
  const [draft, setDraft] = useState<string[]>(entry.tags);
  const [saving, setSaving] = useState(false);

  // 別のエントリに切り替わったら編集中の内容を捨てる。
  useEffect(() => {
    setDraft(entry.tags);
  }, [entry.path, entry.tags]);

  // Esc で閉じる。
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [onClose]);

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

  return (
    <div className="detail" role="dialog" aria-modal="true">
      <div className="detail__backdrop" onClick={onClose} />

      <div className="detail__panel">
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

        <div className="detail__tags">
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
            {entry.tags.length > 0 && (
              <span className="detail__currenttags">
                現在のタグ:{" "}
                {entry.tags.map((tag) => (
                  <button
                    key={tag}
                    type="button"
                    className="detail__tagjump"
                    onClick={() => onTagClick(tag)}
                  >
                    #{tag}
                  </button>
                ))}
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
          </div>
        </div>

        <div className="detail__content">
          {entry.kind === "markdown" && (
            <MarkdownPreview entry={entry} onFollowLink={onFollowLink} />
          )}
          {entry.kind === "image" && <ImagePreview entry={entry} />}
          {entry.kind === "audio" && <AudioPreview entry={entry} />}
        </div>
      </div>
    </div>
  );
}
