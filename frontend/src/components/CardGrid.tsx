/**
 * カードのグリッド。
 *
 * 大量ファイルを描画しても遅くならないよう、react-window の仮想スクロールで
 * 画面に入っている行だけを描く。列数は幅から計算し、ウィンドウ幅の変化に追従する。
 */

import { useCallback, useLayoutEffect, useRef, useState, type CSSProperties } from "react";
import { Grid, type CellComponentProps } from "react-window";
import type { Entry } from "../api/taggo";
import { Card } from "./Card";
import "./CardGrid.css";

/** カード 1 枚の目標幅。実際の幅は、この値を下回らない範囲で列数から決まる。 */
const MIN_CARD_WIDTH = 236;
/** カードの高さ。均一にすることで行の高さ計算を単純に保つ。 */
const ROW_HEIGHT = 292;
/** カード同士の間隔。セルの内側に半分ずつ持たせる（CSS 側と共有する）。 */
const GAP = 14;
/** グリッド外周の余白。検索バーの左右余白と揃える。 */
const PADDING = 18;

/**
 * 縦スクロールバーが占める幅を一度だけ測る。
 *
 * ビューポートには scrollbar-gutter: stable を指定してあり、スクロールバーの
 * 有無にかかわらず常にこの幅が確保される。したがって外側の幅からこれを引いた
 * ものが、実際にカードを置ける幅になる。
 */
let scrollbarWidthCache: number | null = null;
function scrollbarWidth(): number {
  if (scrollbarWidthCache !== null) return scrollbarWidthCache;
  const probe = document.createElement("div");
  probe.style.cssText =
    "position:absolute;top:-9999px;width:100px;height:100px;overflow:scroll;visibility:hidden";
  document.body.appendChild(probe);
  scrollbarWidthCache = probe.offsetWidth - probe.clientWidth;
  probe.remove();
  return scrollbarWidthCache;
}

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
  // 外周の余白はこの要素の padding なので、いずれも内容領域の幅を見る。
  useLayoutEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const observer = new ResizeObserver(([entry]) => {
      setWidth(entry.contentRect.width);
    });
    observer.observe(el);
    const style = getComputedStyle(el);
    setWidth(el.clientWidth - parseFloat(style.paddingLeft) - parseFloat(style.paddingRight));
    return () => observer.disconnect();
  }, []);

  // 列はビューポートの内側を過不足なく分け合う。合計が 1px でも内側の幅を
  // 超えると横スクロールバーが出てしまうので、整数に切り捨てたうえで、
  // 余った端数を左の列から 1px ずつ配って合計を幅ちょうどに収める。
  const usable = Math.max(Math.floor(width - scrollbarWidth()), MIN_CARD_WIDTH + GAP);
  const columnCount = Math.max(1, Math.floor(usable / (MIN_CARD_WIDTH + GAP)));
  const rowCount = Math.ceil(entries.length / columnCount);
  const baseColumnWidth = Math.floor(usable / columnCount);
  const widerColumns = usable - baseColumnWidth * columnCount;
  const columnWidth = useCallback(
    (index: number) => baseColumnWidth + (index < widerColumns ? 1 : 0),
    [baseColumnWidth, widerColumns],
  );

  // 同じカードが並び替えで別のセルへ移っても状態を持ち越さないよう、パスをキーにする。
  const cellKey = useCallback(
    ({ columnIndex, rowIndex, data }: { columnIndex: number; rowIndex: number; data: CellProps }) =>
      data.entries[rowIndex * data.columnCount + columnIndex]?.path ?? `${rowIndex}:${columnIndex}`,
    [],
  );

  // 外周の余白と間隔は CSS 側でも使うので、算出の元になる値をそのまま渡す。
  const metrics = {
    "--cardgrid-gap": `${GAP}px`,
    "--cardgrid-edge": `${PADDING}px`,
  } as CSSProperties;

  return (
    <div className="cardgrid" ref={containerRef} style={metrics}>
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
        />
      )}
    </div>
  );
}
