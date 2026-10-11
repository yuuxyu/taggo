/**
 * 編集画面の Emacs 風のキー操作（カーソル移動・削除・改行・行き先を開く）。
 *
 * - Ctrl + A / Ctrl + E … 行頭 / 行末（折り返して見えている行ではなく、改行で区切られた 1 行の端）
 * - Ctrl + P / Ctrl + N … 上の行 / 下の行
 * - Ctrl + B / Ctrl + F … 1 文字左 / 1 文字右
 * - Ctrl + H … Backspace と同じ（補った括弧の対や、箇条書きの記号もまとめて消す）
 * - Ctrl + M … Enter と同じ改行（閉じのフェンスや、箇条書きの記号の補いも同じ）
 * - Ctrl + O … カーソルのある `[[タグ]]` のページや、リンク・URL の行き先を開く
 *
 * Shift を足すと、動いた先までを選ぶ。既定の割り当て（Ctrl + A のすべて選択など）より先に効かせ、
 * WebView の既定の動作（Ctrl + P の印刷、Ctrl + N の新しいウィンドウなど）も止める。
 * 入力補完の候補が出ている間は、Ctrl + N / Ctrl + P で候補を選ぶ。
 */

import { deleteBracketPair, moveCompletionSelection } from "@codemirror/autocomplete";
import {
  cursorCharBackward,
  cursorCharForward,
  cursorLineDown,
  cursorLineEnd,
  cursorLineStart,
  cursorLineUp,
  deleteCharBackward,
  insertNewlineAndIndent,
  selectCharBackward,
  selectCharForward,
  selectLineDown,
  selectLineEnd,
  selectLineStart,
  selectLineUp,
} from "@codemirror/commands";
import { deleteMarkupBackward, insertNewlineContinueMarkup } from "@codemirror/lang-markdown";
import { Prec } from "@codemirror/state";
import { keymap, type Command } from "@codemirror/view";
import { followAtCursor } from "./livePreview";
import { closeCodeFence } from "./markup";

/**
 * Backspace と同じ消し方。補った括弧の対（[[]] など）、箇条書きや引用の記号、1 文字の順に試す。
 * Backspace に割り当てた働き（closeBrackets・markup.ts・既定の割り当て）と同じ並び。
 */
const backspace: Command = (view) =>
  deleteBracketPair(view) || deleteMarkupBackward(view) || deleteCharBackward(view);

/**
 * Enter と同じ改行。閉じのフェンスの補い、箇条書きや引用の記号の続き、ふつうの改行の順に試す。
 * Enter に割り当てた働き（markup.ts・既定の割り当て）と同じ並び。補完の候補を決める働きは持たない。
 */
const newline: Command = (view) =>
  closeCodeFence(view) || insertNewlineContinueMarkup(view) || insertNewlineAndIndent(view);

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
        { key: "Ctrl-h", run: backspace, preventDefault: true },
        { key: "Ctrl-m", run: newline, preventDefault: true },
        // カーソルの所に何も無くても、WebView の Ctrl + O（ファイルを開く）は止める。
        { key: "Ctrl-o", run: followAtCursor, preventDefault: true },
      ]),
    ),
  ];
}
