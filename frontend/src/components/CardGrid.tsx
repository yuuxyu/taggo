/**
 * カードのグリッド。
 *
 * 大量ファイルを描画しても遅くならないよう、react-window の仮想スクロールで
 * 画面に入っている行だけを描く。列数は幅から計算し、ウィンドウ幅の変化に追従する。
 *
 * 先頭の別枠（ピン留めしたノートや、検索語と同じ名前のタグのページ）は、
 * 残りのカードと行を分けて並べる。別枠の最後の行が埋まらなくても、残りは次の行から始める。
 *
 * Ctrl + N / Ctrl + P で、フォーカスするカードを並び順の次 / 前へ移す。
 * カードの外（検索バーなど）にフォーカスがあるときの Ctrl + N は先頭のカードへ、
 * 先頭のカードでの Ctrl + P はカードの外（呼び出し側が決める。検索バー）へ移す。
 */

import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { Grid, useGridRef, type CellComponentProps } from "react-window";
import type { Entry } from "../api/taggo";
import { focusByKey } from "../keyFocus";
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

/** スクロールした先のカードが描かれるまで、フォーカスを移すのを試し直す回数（フレーム数）。 */
const FOCUS_RETRY_FRAMES = 10;

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
  /**
   * Ctrl + N / Ctrl + P でカードを移るか。一覧の上にノートの画面が重なっている間は、
   * そちらのキー操作（Ctrl + N で本文を書き始めるなど）を優先させるため false にする。
   */
  keyboardNav: boolean;
  /** 先頭のカードで Ctrl + P を押したときに呼ぶ。 */
  onExitTop: () => void;
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

/** indexAt の逆。エントリの番号から、それを置くセルの位置を求める。 */
function cellOf(index: number, p: Pick<CellProps, "head" | "headRows" | "columnCount">) {
  if (index < p.head) {
    return { rowIndex: Math.floor(index / p.columnCount), columnIndex: index % p.columnCount };
  }
  const rest = index - p.head;
  return {
    rowIndex: p.headRows + Math.floor(rest / p.columnCount),
    columnIndex: rest % p.columnCount,
  };
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
    // キー操作でカードを移るとき、フォーカスのあるカードの番号をここから読む。
    <div style={{ ...style, padding: CELL_PADDING }} data-card-index={index}>
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
  keyboardNav,
  onExitTop,
}: Props) {
  const containerRef = useRef<HTMLDivElement>(null);
  const gridRef = useGridRef(null);
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

  // キー操作でカードを移るときに使う、今の並べ方。
  const layout = useRef({ count: 0, head: 0, headRows: 0, columnCount: 1 });
  useLayoutEffect(() => {
    layout.current = { count: entries.length, head: headCount, headRows, columnCount };
  });

  // スクロールした先のカードが描かれるのを待って、フォーカスを移そうとしているカード。
  // キーを押し続けたときは、まだフォーカスの移っていないこのカードから数える。
  const pending = useRef<{ index: number; frame: number } | null>(null);
  const cancelPending = useCallback(() => {
    if (pending.current) cancelAnimationFrame(pending.current.frame);
    pending.current = null;
  }, []);
  useEffect(() => cancelPending, [cancelPending]);

  // Ctrl + N / Ctrl + P で、フォーカスするカードを並び順の次 / 前へ移す。入力欄の中でも効かせる。
  useEffect(() => {
    if (!keyboardNav) return;

    // 仮想スクロールでは画面の外のカードは描かれていないので、先にそのセルまでスクロールし、
    // 描かれてからフォーカスを移す。
    const focusCard = (index: number) => {
      const container = containerRef.current;
      if (!container) return;
      cancelPending();
      gridRef.current?.scrollToCell({ ...cellOf(index, layout.current), rowAlign: "smart" });
      let tries = 0;
      const attempt = () => {
        const card = container.querySelector<HTMLElement>(`[data-card-index="${index}"] > article`);
        if (card || ++tries > FOCUS_RETRY_FRAMES) {
          pending.current = null;
          if (card) focusByKey(card, { preventScroll: true });
          return;
        }
        pending.current = { index, frame: requestAnimationFrame(attempt) };
      };
      attempt();
    };

    const onKeyDown = (e: KeyboardEvent) => {
      const ctrl = e.ctrlKey && !e.altKey && !e.shiftKey && !e.metaKey;
      const key = e.key.toLowerCase();
      if (!ctrl || (key !== "n" && key !== "p") || e.isComposing) return;
      // ダイアログや履歴の一覧が開いている間は、そちらに任せる。
      if (document.querySelector('[role="dialog"][aria-modal="true"], [role="menu"]')) return;
      // WebView の既定の働き（Ctrl + N の新しいウィンドウ、Ctrl + P の印刷）は止める。
      e.preventDefault();
      const { count } = layout.current;
      if (count === 0) return;
      // フォーカスのあるカード（カードの中のピンやタグも含む）の番号。カードの外なら -1。
      const active = document.activeElement;
      const cell = active instanceof Element ? active.closest<HTMLElement>("[data-card-index]") : null;
      const focused = cell && containerRef.current?.contains(cell) ? Number(cell.dataset.cardIndex) : -1;
      const current = Math.min(pending.current?.index ?? focused, count);
      if (key === "n") {
        if (current + 1 < count) focusCard(current + 1);
        return;
      }
      if (current < 0) return;
      if (current === 0) {
        cancelPending();
        onExitTop();
        return;
      }
      focusCard(current - 1);
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [keyboardNav, onExitTop, gridRef, cancelPending]);

  return (
    <div className="h-full" style={{ padding: WRAPPER_PADDING }} ref={containerRef}>
      {width > 0 && (
        <Grid<CellProps>
          gridRef={gridRef}
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
