/**
 * 詳細プレビューの前後移動。
 *
 * 移動先は一覧の並び順そのままで、ファイルの種類は問わない。
 * 画像を見ながら次の Markdown へ、といった行き来がそのままできる。
 * 端では止める（先頭の「前へ」と末尾の「次へ」は押せない）。
 */

import { ChevronLeftIcon, ChevronRightIcon } from "@heroicons/react/20/solid";
import { Button } from "./Button";

interface Props {
  /** 一覧での現在位置（0 始まり）。負なら何も出さない。 */
  index: number;
  total: number;
  onNavigate: (direction: 1 | -1) => void;
  /** 画像の上に重ねるときは、背景が何色でも読めるよう白基調にする。 */
  onImage?: boolean;
}

export function Pager({ index, total, onNavigate, onImage }: Props) {
  if (index < 0) return null;

  return (
    <div className="flex flex-wrap items-center gap-1.5">
      <Button
        variant={onImage ? "hudGhost" : "ghost"}
        disabled={index <= 0}
        onClick={() => onNavigate(-1)}
      >
        <ChevronLeftIcon className="size-4" aria-hidden="true" />
        前へ
      </Button>
      <span
        className={`min-w-16 text-center text-xs tabular-nums ${
          onImage ? "text-white/70" : "text-ink-faint"
        }`}
      >
        {index + 1} / {total}
      </span>
      <Button
        variant={onImage ? "hudGhost" : "ghost"}
        disabled={index >= total - 1}
        onClick={() => onNavigate(1)}
      >
        次へ
        <ChevronRightIcon className="size-4" aria-hidden="true" />
      </Button>
    </div>
  );
}
