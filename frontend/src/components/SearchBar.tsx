/**
 * 最上部に固定する検索バー。
 *
 * 要件に合わせて次の 3 点を満たす。
 *  - 起動時に自動でフォーカスが当たる
 *  - 1 文字入力ごとにインクリメンタル検索が走る（検索実行は呼び出し側）
 *  - "#" を入力すると登録済みタグのオートコンプリートが出る
 */

import { useEffect, useMemo, useRef, useState } from "react";
import { MagnifyingGlassIcon, XMarkIcon } from "@heroicons/react/20/solid";
import { suggestTags, type TagSuggestion } from "../api/taggo";
import { TagSuggestions } from "./TagSuggestions";

interface Props {
  value: string;
  onChange: (next: string) => void;
  /** 絞り込み後の件数。総件数と合わせて表示する。 */
  total: number;
  entryCount: number;
  disabled?: boolean;
  /**
   * この値が変わるたびに検索バーへフォーカスを戻す。
   * オーバーレイを閉じた直後など、キー入力の行き先を検索バーへ返したい場面で使う。
   */
  focusSignal?: number;
}

/**
 * 入力末尾が「入力途中のタグ」かどうかを判定し、その接頭辞を返す。
 * Go 側の search.TrailingTagPrefix と同じ規則で、候補を出すかどうかを決める。
 */
function trailingTagPrefix(input: string): string | null {
  // 引用符が閉じていない間はタグ名の入力途中なので候補を出さない。
  if ((input.match(/"/g)?.length ?? 0) % 2 === 1) return null;

  const lastSpace = Math.max(input.lastIndexOf(" "), input.lastIndexOf("　"));
  let last = input.slice(lastSpace + 1);
  if (last.startsWith("-")) last = last.slice(1);
  if (!last.startsWith("#")) return null;
  return last.slice(1);
}

/** 入力途中のタグを、確定したタグで置き換える。 */
function replaceTrailingTag(input: string, tag: string): string {
  const lastSpace = Math.max(input.lastIndexOf(" "), input.lastIndexOf("　"));
  const head = input.slice(0, lastSpace + 1);
  const last = input.slice(lastSpace + 1);
  const negate = last.startsWith("-") ? "-" : "";
  const quoted = /\s/.test(tag) ? `"${tag}"` : tag;
  return `${head}${negate}#${quoted} `;
}

export function SearchBar({ value, onChange, total, entryCount, disabled, focusSignal }: Props) {
  const inputRef = useRef<HTMLInputElement>(null);
  const [suggestions, setSuggestions] = useState<TagSuggestion[]>([]);
  const [highlighted, setHighlighted] = useState(0);
  const [open, setOpen] = useState(false);

  const prefix = useMemo(() => trailingTagPrefix(value), [value]);

  // 起動時に検索バーへキャレットを置く。
  // フォルダの読み込みが終わるまで入力を無効にしていると、その間の focus() は
  // 効かずに終わってしまうため、無効が解けた時点でも当て直す。
  useEffect(() => {
    if (disabled) return;
    inputRef.current?.focus();
  }, [disabled, focusSignal]);

  // "#" 以降の入力に合わせて候補を引き直す。
  useEffect(() => {
    if (prefix === null) {
      setOpen(false);
      setSuggestions([]);
      return;
    }
    let cancelled = false;
    void suggestTags(prefix).then((got) => {
      if (cancelled) return;
      setSuggestions(got);
      setHighlighted(0);
      setOpen(got.length > 0);
    });
    return () => {
      cancelled = true;
    };
  }, [prefix]);

  const commit = (tag: string) => {
    onChange(replaceTrailingTag(value, tag));
    setOpen(false);
    inputRef.current?.focus();
  };

  const handleKeyDown = (event: React.KeyboardEvent<HTMLInputElement>) => {
    if (event.key === "Escape") {
      if (open) {
        setOpen(false);
      } else {
        onChange("");
      }
      return;
    }
    if (!open || suggestions.length === 0) return;

    switch (event.key) {
      case "ArrowDown":
        event.preventDefault();
        setHighlighted((i) => (i + 1) % suggestions.length);
        break;
      case "ArrowUp":
        event.preventDefault();
        setHighlighted((i) => (i - 1 + suggestions.length) % suggestions.length);
        break;
      case "Enter":
      case "Tab":
        event.preventDefault();
        commit(suggestions[highlighted].tag);
        break;
    }
  };

  return (
    <div className="relative z-20">
      <div className="flex items-center gap-2 rounded-lg border border-line bg-surface px-3.5 transition-colors focus-within:border-accent focus-within:ring-3 focus-within:ring-accent-soft">
        <MagnifyingGlassIcon className="size-5 shrink-0 text-ink-faint" aria-hidden="true" />
        <input
          ref={inputRef}
          type="text"
          className="min-w-0 flex-1 bg-transparent py-2.5 text-base outline-hidden placeholder:text-ink-faint"
          value={value}
          disabled={disabled}
          placeholder="検索 — #タグ で絞り込み / -#タグ で除外 / OR で候補を広げる"
          spellCheck={false}
          autoComplete="off"
          onChange={(e) => onChange(e.target.value)}
          onKeyDown={handleKeyDown}
          onBlur={() => window.setTimeout(() => setOpen(false), 120)}
        />
        {value !== "" && (
          <button
            type="button"
            className="shrink-0 rounded-full p-0.5 text-ink-faint hover:bg-sunken hover:text-ink"
            title="検索条件をクリア"
            aria-label="検索条件をクリア"
            onClick={() => {
              onChange("");
              inputRef.current?.focus();
            }}
          >
            <XMarkIcon className="size-4.5" />
          </button>
        )}
        <span className="shrink-0 text-xs tabular-nums text-ink-faint">
          {value === ""
            ? `${entryCount.toLocaleString()} 件`
            : `${total.toLocaleString()} / ${entryCount.toLocaleString()} 件`}
        </span>
      </div>

      {open && (
        <TagSuggestions
          suggestions={suggestions}
          highlighted={highlighted}
          onHighlight={setHighlighted}
          onCommit={commit}
        />
      )}
    </div>
  );
}
