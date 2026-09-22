/**
 * 詳細プレビューの前後移動。
 *
 * 移動先は一覧の並び順そのままのノート。
 * 端では止める（先頭の「前へ」と末尾の「次へ」は押せない）。
 */

import { ChevronLeftIcon, ChevronRightIcon } from "@heroicons/react/20/solid";
import { Button } from "./Button";

interface Props {
  /** 一覧での現在位置（0 始まり）。負なら何も出さない。 */
  index: number;
  total: number;
  onNavigate: (direction: 1 | -1) => void;
}

export function Pager({ index, total, onNavigate }: Props) {
  if (index < 0) return null;

  return (
    <div className="flex flex-wrap items-center gap-1.5">
      <Button variant="ghost" disabled={index <= 0}
        onClick={() => onNavigate(-1)}
      >
        <ChevronLeftIcon className="size-4" aria-hidden="true" />
        前へ
      </Button>
      <span className="min-w-16 text-center text-xs tabular-nums text-ink-faint">
        {index + 1} / {total}
      </span>
      <Button variant="ghost" disabled={index >= total - 1}
        onClick={() => onNavigate(1)}
      >
        次へ
        <ChevronRightIcon className="size-4" aria-hidden="true" />
      </Button>
    </div>
  );
}
