/**
 * 最上部に固定する検索バー。
 *
 * 要件に合わせて次の 2 点を満たす。
 *  - 起動時に自動でフォーカスが当たる
 *  - 1 文字入力ごとにインクリメンタル検索が走る（検索実行は呼び出し側）
 *
 * 検索は単純な全文検索で、タグを指定する記法や AND / OR のような条件式は持たない。
 * 入力がタグの名前と一致すれば、そのタグのページが結果の先頭に出る（Go 側で並べる）。
 */

import { useEffect, useRef } from "react";
import { MagnifyingGlassIcon, XMarkIcon } from "@heroicons/react/20/solid";

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

export function SearchBar({ value, onChange, total, entryCount, disabled, focusSignal }: Props) {
  const inputRef = useRef<HTMLInputElement>(null);

  // 起動時に検索バーへキャレットを置く。
  // フォルダの読み込みが終わるまで入力を無効にしていると、その間の focus() は
  // 効かずに終わってしまうため、無効が解けた時点でも当て直す。
  useEffect(() => {
    if (disabled) return;
    inputRef.current?.focus();
  }, [disabled, focusSignal]);

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
          placeholder="検索 — タグの名前と一致すれば、そのタグのページを先頭に出します"
          spellCheck={false}
          autoComplete="off"
          onChange={(e) => onChange(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Escape") onChange("");
          }}
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
    </div>
  );
}
