/**
 * "#タグ" のバッジ。クリックするとそのタグが検索バーへ挿入される。
 */

import "./TagBadge.css";

interface Props {
  tag: string;
  onClick?: (tag: string) => void;
  onRemove?: (tag: string) => void;
  /** 見出し的に大きく出したい場面（詳細プレビューのヘッダー）で使う。 */
  size?: "sm" | "md";
}

export function TagBadge({ tag, onClick, onRemove, size = "sm" }: Props) {
  return (
    <span className={`tagbadge tagbadge--${size}`}>
      <button
        type="button"
        className="tagbadge__label"
        title={`#${tag} で絞り込む`}
        onClick={(e) => {
          e.stopPropagation();
          onClick?.(tag);
        }}
      >
        #{tag}
      </button>
      {onRemove && (
        <button
          type="button"
          className="tagbadge__remove"
          title={`#${tag} を外す`}
          onClick={(e) => {
            e.stopPropagation();
            onRemove(tag);
          }}
        >
          ×
        </button>
      )}
    </span>
  );
}
