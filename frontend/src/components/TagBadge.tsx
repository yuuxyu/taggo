/**
 * "#タグ" のバッジ。クリックするとそのタグで検索する（そのタグのページが先頭に出る）。
 */

import { XMarkIcon } from "@heroicons/react/16/solid";

interface Props {
  tag: string;
  onClick?: (tag: string) => void;
  onRemove?: (tag: string) => void;
  /** 見出し的に大きく出したい場面（タグ編集フォーム）で md を使う。 */
  size?: "sm" | "md";
}

const LABEL_SIZE = {
  sm: "px-2 py-px text-xs",
  md: "px-2.5 py-0.5 text-sm",
} as const;

export function TagBadge({ tag, onClick, onRemove, size = "sm" }: Props) {
  return (
    <span className="inline-flex max-w-full items-center rounded-full bg-accent-soft">
      <button
        type="button"
        className={`max-w-full truncate text-accent-ink hover:underline ${LABEL_SIZE[size]}`}
        title={`「${tag}」で検索`}
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
          className="pr-2 pl-0.5 text-accent-ink opacity-60 hover:opacity-100"
          title={`#${tag} を外す`}
          onClick={(e) => {
            e.stopPropagation();
            onRemove(tag);
          }}
        >
          <XMarkIcon className="size-3.5" />
        </button>
      )}
    </span>
  );
}
