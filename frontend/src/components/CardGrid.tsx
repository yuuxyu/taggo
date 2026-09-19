/**
 * カードのグリッド。
 *
 * 大量ファイルを描画しても遅くならないよう、react-window の仮想スクロールで
 * 画面に入っている行だけを描く。列数は幅から計算し、ウィンドウ幅の変化に追従する。
 */

import { useCallback, useLayoutEffect, useRef, useState } from "react";
import { Grid, type CellComponentProps } from "react-window";
import type { Entry } from "../api/taggo";
import { Card } from "./Card";
import "./CardGrid.css";

/** カード 1 枚の目標幅。実際の幅は、この値を下回らない範囲で列数から決まる。 */
const MIN_CARD_WIDTH = 236;
/** カードの高さ。均一にすることで行の高さ計算を単純に保つ。 */
const ROW_HEIGHT = 292;
/** カード同士の間隔。 */
const GAP = 14;
/** グリッド外周の余白。 */
const PADDING = 18;

interface Props {
  entries: Entry[];
  selected: Set<string>;
  selectionMode: boolean;
  onOpen: (entry: Entry) => void;
  onToggleSelect: (entry: Entry) => void;
  onTagClick: (tag: string) => void;
}

/** セルに渡す追加プロパティ。react-window が変更を検知して再描画する。 */
interface CellProps {
  entries: Entry[];
  columnCount: number;
  selected: Set<string>;
  selectionMode: boolean;
  onOpen: (entry: Entry) => void;
  onToggleSelect: (entry: Entry) => void;
  onTagClick: (tag: string) => void;
}

function Cell({
  columnIndex,
  rowIndex,
  style,
  entries,
  columnCount,
  selected,
  selectionMode,
  onOpen,
  onToggleSelect,
  onTagClick,
}: CellComponentProps<CellProps>) {
  const index = rowIndex * columnCount + columnIndex;
  const entry = entries[index];
  if (!entry) return null;

  return (
    <div style={style} className="cardgrid__cell">
      <Card
        entry={entry}
        selected={selected.has(entry.path)}
        selectionMode={selectionMode}
        onOpen={onOpen}
        onToggleSelect={onToggleSelect}
        onTagClick={onTagClick}
      />
    </div>
  );
}

export function CardGrid({
  entries,
  selected,
  selectionMode,
  onOpen,
  onToggleSelect,
  onTagClick,
}: Props) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(0);

  // 列数は実際の表示幅から決めるので、幅の変化を監視する。
  useLayoutEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const observer = new ResizeObserver(([entry]) => {
      setWidth(entry.contentRect.width);
    });
    observer.observe(el);
    setWidth(el.clientWidth);
    return () => observer.disconnect();
  }, []);

  const usable = Math.max(width - PADDING * 2, MIN_CARD_WIDTH);
  const columnCount = Math.max(1, Math.floor((usable + GAP) / (MIN_CARD_WIDTH + GAP)));
  const rowCount = Math.ceil(entries.length / columnCount);
  const columnWidth = (usable + GAP) / columnCount;

  // 同じカードが並び替えで別のセルへ移っても状態を持ち越さないよう、パスをキーにする。
  const cellKey = useCallback(
    ({ columnIndex, rowIndex, data }: { columnIndex: number; rowIndex: number; data: CellProps }) =>
      data.entries[rowIndex * data.columnCount + columnIndex]?.path ?? `${rowIndex}:${columnIndex}`,
    [],
  );

  return (
    <div className="cardgrid" ref={containerRef}>
      {width > 0 && (
        <Grid<CellProps>
          className="cardgrid__viewport"
          cellComponent={Cell}
          cellProps={{
            entries,
            columnCount,
            selected,
            selectionMode,
            onOpen,
            onToggleSelect,
            onTagClick,
          }}
          columnCount={columnCount}
          columnWidth={columnWidth}
          rowCount={rowCount}
          rowHeight={ROW_HEIGHT + GAP}
          rowKey={({ rowIndex }) => rowIndex}
          columnKey={cellKey}
          overscanCount={2}
          style={{ padding: PADDING }}
        />
      )}
    </div>
  );
}
