/**
 * タグ候補のドロップダウン。検索バーとタグ編集フォームで共通に使う。
 *
 * 入力欄の真下に重ね、キーボードで選んでいる候補を強調する。
 * 候補をクリックしたときに入力欄のフォーカスが外れないよう、
 * mousedown の既定動作は止めている。
 */

import type { TagSuggestion } from "../api/taggo";

interface Props {
  suggestions: TagSuggestion[];
  /** キーボードで選択中の位置。 */
  highlighted: number;
  onHighlight: (index: number) => void;
  onCommit: (tag: string) => void;
}

export function TagSuggestions({ suggestions, highlighted, onHighlight, onCommit }: Props) {
  if (suggestions.length === 0) return null;

  return (
    <ul
      role="listbox"
      className="absolute top-[calc(100%+4px)] right-0 left-0 z-30 max-h-80 overflow-y-auto rounded-lg border border-line bg-surface p-1 shadow-pop"
    >
      {suggestions.map((s, i) => (
        <li key={s.tag}>
          <button
            type="button"
            role="option"
            aria-selected={i === highlighted}
            className={`flex w-full items-center justify-between rounded-md px-2.5 py-1.5 text-left text-sm text-accent-ink ${
              i === highlighted ? "bg-accent-soft" : ""
            }`}
            onMouseEnter={() => onHighlight(i)}
            onMouseDown={(e) => e.preventDefault()}
            onClick={() => onCommit(s.tag)}
          >
            <span className="truncate">#{s.tag}</span>
            <span className="pl-2 text-xs tabular-nums text-ink-faint">{s.count}</span>
          </button>
        </li>
      ))}
    </ul>
  );
}
