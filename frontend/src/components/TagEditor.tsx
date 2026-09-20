/**
 * タグの編集フォーム。詳細プレビューのヘッダーと、一括編集ダイアログの両方で使う。
 *
 * 入力中は登録済みタグの候補を出し、表記ゆれが増えないようにする。
 */

import { useEffect, useRef, useState } from "react";
import { suggestTags, type TagSuggestion } from "../api/taggo";
import { TagBadge } from "./TagBadge";
import { TagSuggestions } from "./TagSuggestions";

interface Props {
  /** 現在のタグ。一括編集では「これから追加するタグ」を表す。 */
  tags: string[];
  onChange: (next: string[]) => void;
  disabled?: boolean;
  placeholder?: string;
}

export function TagEditor({ tags, onChange, disabled, placeholder }: Props) {
  const [draft, setDraft] = useState("");
  const [suggestions, setSuggestions] = useState<TagSuggestion[]>([]);
  const [highlighted, setHighlighted] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    const term = draft.trim();
    if (term === "") {
      setSuggestions([]);
      return;
    }
    let cancelled = false;
    void suggestTags(term, 8).then((got) => {
      if (cancelled) return;
      // すでに付いているタグは候補から外す。
      const remaining = got.filter(
        (s) => !tags.some((t) => t.toLowerCase() === s.tag.toLowerCase()),
      );
      setSuggestions(remaining);
      setHighlighted(0);
    });
    return () => {
      cancelled = true;
    };
  }, [draft, tags]);

  const add = (raw: string) => {
    const tag = raw.trim().replace(/^#/, "");
    if (tag === "") return;
    if (tags.some((t) => t.toLowerCase() === tag.toLowerCase())) {
      setDraft("");
      return;
    }
    onChange([...tags, tag]);
    setDraft("");
    setSuggestions([]);
  };

  const handleKeyDown = (event: React.KeyboardEvent<HTMLInputElement>) => {
    if (suggestions.length > 0) {
      if (event.key === "ArrowDown") {
        event.preventDefault();
        setHighlighted((i) => (i + 1) % suggestions.length);
        return;
      }
      if (event.key === "ArrowUp") {
        event.preventDefault();
        setHighlighted((i) => (i - 1 + suggestions.length) % suggestions.length);
        return;
      }
    }

    if (event.key === "Enter") {
      event.preventDefault();
      // 候補を選んでいればそれを、そうでなければ入力そのものを採用する。
      add(suggestions.length > 0 && draft !== "" ? suggestions[highlighted].tag : draft);
      return;
    }
    // 末尾でのバックスペースは直前のタグを外す。
    if (event.key === "Backspace" && draft === "" && tags.length > 0) {
      onChange(tags.slice(0, -1));
    }
  };

  return (
    <div className="relative">
      <div
        className={`flex flex-wrap items-center gap-1.5 rounded-lg border border-line px-2.5 py-2 focus-within:border-accent focus-within:ring-3 focus-within:ring-accent-soft ${
          disabled ? "cursor-not-allowed bg-sunken opacity-70" : "cursor-text bg-surface"
        }`}
        onClick={() => inputRef.current?.focus()}
      >
        {tags.map((tag) => (
          <TagBadge
            key={tag}
            tag={tag}
            size="md"
            onRemove={disabled ? undefined : (t) => onChange(tags.filter((x) => x !== t))}
          />
        ))}
        <input
          ref={inputRef}
          type="text"
          className="min-w-36 flex-1 bg-transparent py-0.5 text-sm outline-hidden placeholder:text-ink-faint"
          value={draft}
          disabled={disabled}
          placeholder={placeholder ?? "タグを追加（Enter で確定）"}
          spellCheck={false}
          autoComplete="off"
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={handleKeyDown}
        />
      </div>

      <TagSuggestions
        suggestions={suggestions}
        highlighted={highlighted}
        onHighlight={setHighlighted}
        onCommit={add}
      />
    </div>
  );
}
