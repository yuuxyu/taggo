/**
 * 編集画面の Emacs 風のカーソル移動。
 *
 * - Ctrl + A / Ctrl + E … 行頭 / 行末（折り返して見えている行ではなく、改行で区切られた 1 行の端）
 * - Ctrl + P / Ctrl + N … 上の行 / 下の行
 * - Ctrl + B / Ctrl + F … 1 文字左 / 1 文字右
 *
 * Shift を足すと、動いた先までを選ぶ。既定の割り当て（Ctrl + A のすべて選択など）より先に効かせ、
 * WebView の既定の動作（Ctrl + P の印刷、Ctrl + N の新しいウィンドウなど）も止める。
 * 入力補完の候補が出ている間は、Ctrl + N / Ctrl + P で候補を選ぶ。
 */

import { moveCompletionSelection } from "@codemirror/autocomplete";
import {
  cursorCharBackward,
  cursorCharForward,
  cursorLineDown,
  cursorLineEnd,
  cursorLineStart,
  cursorLineUp,
  selectCharBackward,
  selectCharForward,
  selectLineDown,
  selectLineEnd,
  selectLineStart,
  selectLineUp,
} from "@codemirror/commands";
import { Prec } from "@codemirror/state";
import { keymap } from "@codemirror/view";

export function emacsKeys() {
  return [
    // 補完の候補が出ていなければ false を返し、下のカーソル移動に回る。
    Prec.highest(
      keymap.of([
        { key: "Ctrl-n", run: moveCompletionSelection(true) },
        { key: "Ctrl-p", run: moveCompletionSelection(false) },
      ]),
    ),
    Prec.high(
      keymap.of([
        { key: "Ctrl-a", run: cursorLineStart, shift: selectLineStart, preventDefault: true },
        { key: "Ctrl-e", run: cursorLineEnd, shift: selectLineEnd, preventDefault: true },
        { key: "Ctrl-p", run: cursorLineUp, shift: selectLineUp, preventDefault: true },
        { key: "Ctrl-n", run: cursorLineDown, shift: selectLineDown, preventDefault: true },
        { key: "Ctrl-b", run: cursorCharBackward, shift: selectCharBackward, preventDefault: true },
        { key: "Ctrl-f", run: cursorCharForward, shift: selectCharForward, preventDefault: true },
      ]),
    ),
  ];
}
