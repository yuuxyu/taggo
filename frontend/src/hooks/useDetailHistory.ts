/**
 * 詳細プレビューの移動履歴。
 *
 * ブラウザの「戻る／進む」と同じ考え方で、プレビューを開いてから閉じるまでの間に
 * 見たノートを順に覚えておく。一覧のカードから開くと履歴は新しく始まり、
 * プレビューを閉じると捨てる。本文中の画像はノートではないので履歴には積まない。
 *
 * 戻ったときに読んでいた位置へ戻れるよう、各項目はスクロール位置も持つ。
 */

import { useCallback, useRef, useState } from "react";

export interface HistoryItem {
  path: string;
  /** 履歴の一覧に出す名前。移動した時点のタイトルを覚えておく。 */
  title: string;
  /** その項目を離れたときのスクロール位置。 */
  scrollTop: number;
}

interface State {
  items: HistoryItem[];
  index: number;
  /**
   * 移動のたびに増える番号。同じファイルへ戻ったときにも、
   * スクロール位置の復元をやり直す合図として使う。
   */
  navId: number;
}

const EMPTY: State = { items: [], index: -1, navId: 0 };

/** 1 回のプレビューで覚えておく最大件数。古いものから捨てる。 */
const MAX_ITEMS = 100;

export interface DetailHistory {
  current: HistoryItem | null;
  items: HistoryItem[];
  index: number;
  navId: number;
  canBack: boolean;
  canForward: boolean;
  /** 一覧から開く。履歴は新しく始まる。 */
  start: (path: string, title: string) => void;
  /** リンクなどをたどる。今より先の履歴は捨てて積む。 */
  push: (path: string, title: string) => void;
  /** 履歴の中の位置へ移る。戻る・進むもこれで表す。 */
  go: (index: number) => void;
  /** プレビューを閉じる。履歴も捨てる。 */
  clear: () => void;
  /** 表示中のスクロール位置を知らせる。離れるときに今の項目へ書き込む。 */
  reportScroll: (scrollTop: number) => void;
}

export function useDetailHistory(): DetailHistory {
  const [state, setState] = useState<State>(EMPTY);
  // スクロールのたびに再描画しないよう、位置は ref に持っておき、離れるときだけ書き込む。
  const scrollTop = useRef(0);

  /**
   * 今の項目へスクロール位置 top を書き込んだ items を返す。
   * 更新関数は開発時に 2 回呼ばれることがあるので、ref は更新関数の外で読んでおく。
   */
  const saveScroll = (s: State, top: number): HistoryItem[] =>
    s.items.map((item, i) => (i === s.index ? { ...item, scrollTop: top } : item));

  const start = useCallback((path: string, title: string) => {
    setState((s) => ({ items: [{ path, title, scrollTop: 0 }], index: 0, navId: s.navId + 1 }));
  }, []);

  const push = useCallback((path: string, title: string) => {
    const top = scrollTop.current;
    setState((s) => {
      if (s.items[s.index]?.path === path) return s;
      const items = [...saveScroll(s, top).slice(0, s.index + 1), { path, title, scrollTop: 0 }].slice(
        -MAX_ITEMS,
      );
      return { items, index: items.length - 1, navId: s.navId + 1 };
    });
  }, []);

  const go = useCallback((index: number) => {
    const top = scrollTop.current;
    setState((s) => {
      if (index === s.index || index < 0 || index >= s.items.length) return s;
      return { items: saveScroll(s, top), index, navId: s.navId + 1 };
    });
  }, []);

  const clear = useCallback(() => {
    setState((s) => ({ ...EMPTY, navId: s.navId + 1 }));
  }, []);

  const reportScroll = useCallback((top: number) => {
    scrollTop.current = top;
  }, []);

  return {
    current: state.items[state.index] ?? null,
    items: state.items,
    index: state.index,
    navId: state.navId,
    canBack: state.index > 0,
    canForward: state.index >= 0 && state.index < state.items.length - 1,
    start,
    push,
    go,
    clear,
    reportScroll,
  };
}
