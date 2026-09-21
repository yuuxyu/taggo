/**
 * 詳細プレビューのヘッダー左端に置く「戻る／進む」と履歴の一覧。
 *
 * ブラウザと同じ並びと操作にそろえ、覚えることを増やさない。
 *  - ← / →            … 1 つ戻る・進む（ホバーで行き先の名前が出る）
 *  - 時計のボタン      … これまでに見たファイルの一覧を開き、好きな位置へ飛ぶ
 *  - Alt + ← / →      … キーボードでの戻る・進む（DetailPanel 側で受ける）
 *  - マウスの戻る／進むボタン … 同上
 *
 * 一覧は新しいものを上に並べ、いま見ているものに印を付ける。
 * 一覧から飛んでも履歴は切り詰めないので、「進む」で元の位置へ戻れる。
 */

import { useEffect, useRef, useState } from "react";
import { ArrowLeftIcon, ArrowRightIcon, CheckIcon, ClockIcon } from "@heroicons/react/20/solid";
import type { HistoryItem } from "../hooks/useDetailHistory";
import { Button, type ButtonVariant } from "./Button";

interface Props {
  items: HistoryItem[];
  index: number;
  onGo: (index: number) => void;
  /** 画像の上では白基調のボタンにする。 */
  variant: ButtonVariant;
  /** 一覧の配色も画像の上かどうかで変える。 */
  onImage: boolean;
}

/** アイコンだけのボタンは、文字付きのボタンより左右の余白を詰める。 */
const ICON_ONLY = "px-2!";

export function HistoryNav({ items, index, onGo, variant, onImage }: Props) {
  const [menuOpen, setMenuOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);

  const prev = index > 0 ? items[index - 1] : null;
  const next = index >= 0 && index < items.length - 1 ? items[index + 1] : null;

  // 一覧の外をクリックするか Esc で閉じる。Esc はプレビュー自体を閉じる操作でもあるので、
  // 一覧が開いている間はキャプチャで先に受け、プレビューまで届かないようにする。
  useEffect(() => {
    if (!menuOpen) return;
    const onPointerDown = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) setMenuOpen(false);
    };
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key !== "Escape") return;
      e.stopPropagation();
      setMenuOpen(false);
    };
    window.addEventListener("mousedown", onPointerDown);
    window.addEventListener("keydown", onKeyDown, { capture: true });
    return () => {
      window.removeEventListener("mousedown", onPointerDown);
      window.removeEventListener("keydown", onKeyDown, { capture: true });
    };
  }, [menuOpen]);

  // 別のファイルへ移ったら一覧は閉じる。
  useEffect(() => setMenuOpen(false), [index, items]);

  return (
    <div className="-ml-2 flex shrink-0 items-center" ref={menuRef}>
      <Button
        variant={variant}
        className={ICON_ONLY}
        disabled={!prev}
        onClick={() => onGo(index - 1)}
        title={prev ? `戻る: ${prev.title}（Alt+←）` : "戻る（Alt+←）"}
        aria-label="戻る"
      >
        <ArrowLeftIcon className="size-4" aria-hidden="true" />
      </Button>
      <Button
        variant={variant}
        className={ICON_ONLY}
        disabled={!next}
        onClick={() => onGo(index + 1)}
        title={next ? `進む: ${next.title}（Alt+→）` : "進む（Alt+→）"}
        aria-label="進む"
      >
        <ArrowRightIcon className="size-4" aria-hidden="true" />
      </Button>
      <div className="relative">
        <Button
          variant={variant}
          className={ICON_ONLY}
          disabled={items.length < 2}
          onClick={() => setMenuOpen((open) => !open)}
          title="これまでに見たファイル"
          aria-label="履歴"
          aria-haspopup="menu"
          aria-expanded={menuOpen}
        >
          <ClockIcon className="size-4" aria-hidden="true" />
        </Button>
        {menuOpen && (
          <ul
            role="menu"
            className={`absolute top-full left-0 z-20 m-0 mt-1 flex max-h-[60vh] w-80 list-none flex-col overflow-auto rounded-lg border p-1 shadow-card ${
              onImage ? "border-white/20 bg-black/85 text-white" : "border-line bg-surface text-ink"
            }`}
          >
            {items
              .map((item, i) => ({ item, i }))
              .reverse()
              .map(({ item, i }) => {
                const current = i === index;
                return (
                  <li key={i} role="none">
                    <button
                      type="button"
                      role="menuitem"
                      aria-current={current ? "page" : undefined}
                      className={`flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm ${
                        onImage ? "hover:bg-white/15" : "hover:bg-sunken"
                      } ${current ? "font-semibold" : ""}`}
                      title={item.path}
                      onClick={() => {
                        setMenuOpen(false);
                        onGo(i);
                      }}
                    >
                      <span className="flex size-4 shrink-0 items-center justify-center">
                        {current && <CheckIcon className="size-4" aria-hidden="true" />}
                      </span>
                      <span className="truncate">{item.title}</span>
                    </button>
                  </li>
                );
              })}
          </ul>
        )}
      </div>
    </div>
  );
}
