/**
 * カードのグリッド。
 *
 * 大量ファイルを描画しても遅くならないよう、react-window の仮想スクロールで
 * 画面に入っている行だけを描く。列数は幅から計算し、ウィンドウ幅の変化に追従する。
 *
 * 先頭の別枠（ピン留めしたノートや、検索語と同じ名前のタグのページ）は、
 * 残りのカードと行を分けて並べる。別枠の最後の行が埋まらなくても、残りは次の行から始める。
 */

import { useCallback, useLayoutEffect, useRef, useState } from "react";
import { Grid, type CellComponentProps } from "react-window";
import type { Entry } from "../api/taggo";
import { Card } from "./Card";

/** カード 1 枚の目標幅。実際の幅は、この値を下回らない範囲で列数から決まる。 */
const MIN_CARD_WIDTH = 236;
/** カードの高さ。均一にすることで行の高さ計算を単純に保つ。 */
const ROW_HEIGHT = 296;
/** カード同士の間隔。セルの内側に半分ずつ持たせる。 */
const GAP = 14;
/** グリッド外周の余白。検索バーの左右余白と揃える。 */
const PADDING = 18;

/**
 * react-window はセルを position: absolute で置く。絶対配置の基準はスクロール
 * コンテナの「パディングボックス」なので、コンテナ側の padding はセルの位置に
 * 反映されない（左と上だけ余白が消え、右と下に余る）。そのため外周の余白は
 * ラッパーが持ち、スクロール領域そのものには padding を付けない。
 *
 * 間隔の半分はセルが内側に持つので、ラッパーはその差分だけを受け持つ。
 */
const WRAPPER_PADDING = PADDING - GAP / 2;
const CELL_PADDING = GAP / 2;

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
  /** entries の先頭のうち、残りと行を分けて並べる件数。 */
  head: number;
  /** ピン留めしているノートのパス。 */
  pins: ReadonlySet<string>;
  onOpen: (entry: Entry) => void;
  onTagClick: (tag: string) => void;
  onTogglePin: (entry: Entry, pinned: boolean) => void;
}

/** セルに渡す追加プロパティ。react-window が変更を検知して再描画する。 */
interface CellProps {
  entries: Entry[];
  head: number;
  headRows: number;
  columnCount: number;
  pins: ReadonlySet<string>;
  onOpen: (entry: Entry) => void;
  onTagClick: (tag: string) => void;
  onTogglePin: (entry: Entry, pinned: boolean) => void;
}

/**
 * セルの位置から、そこに置くエントリの番号を求める。置くものが無ければ -1。
 * 先頭の別枠は headRows 行を占め、残りはその次の行の左端から並ぶ。
 */
function indexAt(rowIndex: number, columnIndex: number, p: Pick<CellProps, "head" | "headRows" | "columnCount">): number {
  if (rowIndex < p.headRows) {
    const i = rowIndex * p.columnCount + columnIndex;
    return i < p.head ? i : -1;
  }
  return p.head + (rowIndex - p.headRows) * p.columnCount + columnIndex;
}

function Cell({
  columnIndex,
  rowIndex,
  style,
  entries,
  head,
  headRows,
  columnCount,
  pins,
  onOpen,
  onTagClick,
  onTogglePin,
}: CellComponentProps<CellProps>) {
  const index = indexAt(rowIndex, columnIndex, { head, headRows, columnCount });
  const entry = index < 0 ? undefined : entries[index];
  if (!entry) return null;

  return (
    <div style={{ ...style, padding: CELL_PADDING }}>
      <Card
        entry={entry}
        pinned={pins.has(entry.path)}
        onOpen={onOpen}
        onTagClick={onTagClick}
        onTogglePin={onTogglePin}
      />
    </div>
  );
}

export function CardGrid({
  entries,
  head,
  pins,
  onOpen,
  onTagClick,
  onTogglePin,
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
  const headCount = Math.min(head, entries.length);
  const headRows = Math.ceil(headCount / columnCount);
  const rowCount = headRows + Math.ceil((entries.length - headCount) / columnCount);
  const baseColumnWidth = Math.floor(usable / columnCount);
  const widerColumns = usable - baseColumnWidth * columnCount;
  const columnWidth = useCallback(
    (index: number) => baseColumnWidth + (index < widerColumns ? 1 : 0),
    [baseColumnWidth, widerColumns],
  );

  // 同じカードが並び替えで別のセルへ移っても状態を持ち越さないよう、パスをキーにする。
  const cellKey = useCallback(
    ({ columnIndex, rowIndex, data }: { columnIndex: number; rowIndex: number; data: CellProps }) => {
      const index = indexAt(rowIndex, columnIndex, data);
      return (index < 0 ? undefined : data.entries[index]?.path) ?? `${rowIndex}:${columnIndex}`;
    },
    [],
  );

  return (
    <div className="h-full" style={{ padding: WRAPPER_PADDING }} ref={containerRef}>
      {width > 0 && (
        <Grid<CellProps>
          className="[scrollbar-gutter:stable]"
          cellComponent={Cell}
          cellProps={{
            entries,
            head: headCount,
            headRows,
            columnCount,
            pins,
            onOpen,
            onTagClick,
            onTogglePin,
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
