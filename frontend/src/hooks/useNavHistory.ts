/**
 * 画面の移動履歴。
 *
 * ブラウザの「戻る／進む」と同じ考え方で、見てきた画面を順に覚えておく。
 * 履歴に積むのは次の 2 種類。
 *  - ノート … 詳細プレビューで開いたノート
 *  - 一覧   … ある検索語での一覧（タグのクリックで始めた検索も含む）
 *
 * 履歴の先頭は必ず一覧にしておく。こうしておけば、ノートからは必ず「戻る」で一覧へ戻れ、
 * プレビューを閉じる操作は「手前の一覧へ移る」ことで表せる。
 * 検索バーへの入力は 1 文字ごとに積まず、今の一覧の検索語を書き換える。
 * 本文中の画像はノートではないので履歴には積まない。
 *
 * ノートへ戻ったときに読んでいた位置へ戻れるよう、ノートの項目はスクロール位置も持つ。
 */

import { useCallback, useRef, useState } from "react";

interface NoteItem {
  kind: "note";
  path: string;
  /** 履歴の一覧に出す名前。移動した時点のタイトルを覚えておく。 */
  title: string;
  /** その項目を離れたときのスクロール位置。 */
  scrollTop: number;
}

interface SearchItem {
  kind: "search";
  query: string;
  /** 履歴の一覧に出す名前。検索語から作る。 */
  title: string;
}

export type HistoryItem = NoteItem | SearchItem;

interface State {
  items: HistoryItem[];
  index: number;
  /**
   * 移動のたびに増える番号。同じ項目へ戻ったときにも、
   * 検索語やスクロール位置を合わせ直す合図として使う。
   */
  navId: number;
}

/** 覚えておく最大件数。古いものから捨てる。 */
const MAX_ITEMS = 100;

function searchItem(query: string): SearchItem {
  return { kind: "search", query, title: query === "" ? "すべてのノート" : `「${query}」の検索結果` };
}

function noteItem(path: string, title: string): NoteItem {
  return { kind: "note", path, title, scrollTop: 0 };
}

/**
 * 古いものを捨てて MAX_ITEMS 件に収める。先頭が一覧でなくなるときは、
 * 捨てた中で最後の一覧を先頭に残し、ノートから必ず一覧へ戻れるようにする。
 */
function trim(items: HistoryItem[], index: number): Pick<State, "items" | "index"> {
  const drop = items.length - MAX_ITEMS;
  if (drop <= 0) return { items, index };
  const kept = items.slice(drop);
  if (kept[0].kind === "search") return { items: kept, index: index - drop };
  const root = items.slice(0, drop).findLast((item) => item.kind === "search") ?? searchItem("");
  return { items: [root, ...kept.slice(1)], index: index - drop };
}

export interface NavHistory {
  current: HistoryItem;
  items: HistoryItem[];
  index: number;
  navId: number;
  /** ノートを開く。今より先の履歴は捨てて積む。 */
  pushNote: (path: string, title: string) => void;
  /** 一覧を新しい検索語で表示する。今より先の履歴は捨てて積む。 */
  pushSearch: (query: string) => void;
  /** 今の一覧の検索語を書き換える。検索バーへの入力では履歴を積まない。 */
  replaceSearch: (query: string) => void;
  /** 履歴の中の位置へ移る。戻る・進むもこれで表す。 */
  go: (index: number) => void;
  /** 今の位置から見て手前にある、いちばん近い一覧へ移る。プレビューを閉じる操作。 */
  backToList: () => void;
  /** 消えたノートを履歴から除く。見ていたノートなら、その手前の項目へ移る。 */
  remove: (path: string) => void;
  /** 履歴を捨てて、query の一覧だけから始め直す。フォルダを開き直したときに使う。 */
  reset: (query: string) => void;
  /** 表示中のスクロール位置を知らせる。離れるときに今の項目へ書き込む。 */
  reportScroll: (scrollTop: number) => void;
}

export function useNavHistory(): NavHistory {
  const [state, setState] = useState<State>({ items: [searchItem("")], index: 0, navId: 0 });
  // スクロールのたびに再描画しないよう、位置は ref に持っておき、離れるときだけ書き込む。
  const scrollTop = useRef(0);

  /**
   * 今の項目がノートなら、スクロール位置 top を書き込んだ items を返す。
   * 更新関数は開発時に 2 回呼ばれることがあるので、ref は更新関数の外で読んでおく。
   */
  const saveScroll = (s: State, top: number): HistoryItem[] =>
    s.items.map((item, i) => (i === s.index && item.kind === "note" ? { ...item, scrollTop: top } : item));

  /** 今の位置の後ろに item を積む。今より先の履歴は捨てる。 */
  const pushItem = (s: State, top: number, item: HistoryItem): State => {
    const items = [...saveScroll(s, top).slice(0, s.index + 1), item];
    return { ...trim(items, items.length - 1), navId: s.navId + 1 };
  };

  const pushNote = useCallback((path: string, title: string) => {
    const top = scrollTop.current;
    setState((s) => {
      const cur = s.items[s.index];
      if (cur.kind === "note" && cur.path === path) return s;
      return pushItem(s, top, noteItem(path, title));
    });
  }, []);

  const pushSearch = useCallback((query: string) => {
    const top = scrollTop.current;
    setState((s) => {
      const cur = s.items[s.index];
      if (cur.kind === "search" && cur.query === query) return s;
      return pushItem(s, top, searchItem(query));
    });
  }, []);

  const replaceSearch = useCallback((query: string) => {
    setState((s) => {
      const cur = s.items[s.index];
      if (cur.kind !== "search" || cur.query === query) return s;
      return { ...s, items: s.items.map((item, i) => (i === s.index ? searchItem(query) : item)) };
    });
  }, []);

  const go = useCallback((index: number) => {
    const top = scrollTop.current;
    setState((s) => {
      if (index === s.index || index < 0 || index >= s.items.length) return s;
      return { items: saveScroll(s, top), index, navId: s.navId + 1 };
    });
  }, []);

  const backToList = useCallback(() => {
    const top = scrollTop.current;
    setState((s) => {
      if (s.items[s.index].kind === "search") return s;
      const index = s.items.slice(0, s.index).findLastIndex((item) => item.kind === "search");
      return { items: saveScroll(s, top), index, navId: s.navId + 1 };
    });
  }, []);

  const remove = useCallback((path: string) => {
    setState((s) => {
      const removed = (item: HistoryItem) => item.kind === "note" && item.path === path;
      if (!s.items.some(removed)) return s;
      // 今の位置より手前（今の位置を含む）で残る項目のうち、最後のものへ移る。
      // 先頭は一覧なので、必ず 1 つは残る。
      const index = s.items.slice(0, s.index + 1).filter((item) => !removed(item)).length - 1;
      const moved = removed(s.items[s.index]);
      return {
        items: s.items.filter((item) => !removed(item)),
        index,
        navId: moved ? s.navId + 1 : s.navId,
      };
    });
  }, []);

  const reset = useCallback((query: string) => {
    setState((s) => ({ items: [searchItem(query)], index: 0, navId: s.navId + 1 }));
  }, []);

  const reportScroll = useCallback((top: number) => {
    scrollTop.current = top;
  }, []);

  return {
    current: state.items[state.index],
    items: state.items,
    index: state.index,
    navId: state.navId,
    pushNote,
    pushSearch,
    replaceSearch,
    go,
    backToList,
    remove,
    reset,
    reportScroll,
  };
}
