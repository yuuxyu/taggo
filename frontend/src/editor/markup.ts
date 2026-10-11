/**
 * 書いている途中の Markdown の記号を補う。
 *
 * - `**` と打つと閉じの `**` を補い、間にカーソルを置く。閉じの `**` の前で `*` を打つと、
 *   補った記号の上を進むだけにする。文字を選んで `*` を打つと、選んだ文字を囲む。
 * - 行の頭で ``` と打って Enter を押すと、閉じの ``` を補い、間の行にカーソルを置く。
 * - Ctrl + B / Ctrl + I で、選んだ文字を太字・斜体にする（もう一度押すと戻す）。
 * - 箇条書き・番号付きリスト・引用の行で Enter を押すと、次の行に同じ記号を補う。
 */

import { insertNewlineContinueMarkup, deleteMarkupBackward } from "@codemirror/lang-markdown";
import { syntaxTree } from "@codemirror/language";
import { EditorSelection, Prec, type EditorState } from "@codemirror/state";
import { EditorView, keymap, type Command } from "@codemirror/view";
import type { SyntaxNode } from "@lezer/common";

/** pos の前後 n 文字。文書の端を越える分は空になる。 */
function before(state: EditorState, pos: number, n = 1) {
  return state.sliceDoc(Math.max(0, pos - n), pos);
}
function after(state: EditorState, pos: number, n = 1) {
  return state.sliceDoc(pos, pos + n);
}

/** 空白でも `*` でもない文字か。 */
const isWordChar = (c: string) => c !== "" && c !== "*" && !/\s/.test(c);

/** カーソルがコードの中にあるか。コードの中では記号もタグも補わない。 */
export function inCode(state: EditorState, pos: number): boolean {
  for (let n: SyntaxNode | null = syntaxTree(state).resolveInner(pos, -1); n; n = n.parent) {
    if (n.name === "InlineCode" || n.name === "FencedCode" || n.name === "CodeBlock") return true;
  }
  return false;
}

/** `*` の入力を受けて、太字の記号を補う。補ったら true。 */
const starHandler = EditorView.inputHandler.of((view, from, to, text) => {
  if (text !== "*" || view.composing) return false;
  const { state } = view;
  const ranges = state.selection.ranges;
  if (ranges.length !== 1) return false;
  const sel = ranges[0];

  // 選んだ文字を囲む。選んだままにしておくので、もう一度 `*` を打てば太字になる。
  if (!sel.empty) {
    view.dispatch({
      changes: [
        { from: sel.from, insert: "*" },
        { from: sel.to, insert: "*" },
      ],
      selection: EditorSelection.range(sel.from + 1, sel.to + 1),
      userEvent: "input",
    });
    return true;
  }
  if (from !== to || inCode(state, from)) return false;

  const prev = before(state, from);
  const prev2 = before(state, from, 2).slice(0, -1);
  const next = after(state, from);

  // 閉じの `**` の手前で打った `*` は、記号の上を進むだけにする。
  // 「太字|**」→「太字*|*」→「太字**|」
  if (next === "*" && (isWordChar(prev) && after(state, from, 2) === "**" || prev === "*" && isWordChar(prev2))) {
    view.dispatch({ selection: { anchor: from + 1 }, userEvent: "select" });
    return true;
  }

  // `**` と打った（直前が `*` で、その前は `*` でない）ら、閉じの `**` を補う。
  if (prev === "*" && prev2 !== "*" && next !== "*") {
    view.dispatch({
      changes: { from, insert: "***" },
      selection: { anchor: from + 1 },
      userEvent: "input",
    });
    return true;
  }
  return false;
});

/**
 * 行末で Enter を押したとき、その行が閉じていないコードブロックの開きのフェンス（```lang）なら、
 * 閉じのフェンスを補って間の行にカーソルを置く。
 */
const closeCodeFence: Command = (view) => {
  const { state } = view;
  const sel = state.selection.main;
  if (state.selection.ranges.length !== 1 || !sel.empty) return false;
  const line = state.doc.lineAt(sel.head);
  if (sel.head !== line.to) return false;
  const m = /^(\s*)(`{3,}|~{3,})[^`~]*$/.exec(line.text);
  if (!m) return false;

  // その行で始まるコードブロックで、閉じのフェンスがまだ無いものに限る。
  let block = syntaxTree(state).resolveInner(line.from + m[1].length, 1);
  while (block.name !== "FencedCode" && block.parent) block = block.parent;
  if (block.name !== "FencedCode" || state.doc.lineAt(block.from).number !== line.number) return false;
  if (block.getChildren("CodeMark").length >= 2) return false;

  const indent = m[1];
  view.dispatch({
    changes: { from: sel.head, insert: `\n${indent}\n${indent}${m[2]}` },
    selection: { anchor: sel.head + 1 + indent.length },
    scrollIntoView: true,
    userEvent: "input",
  });
  return true;
};

/** 選んだ文字を marker で囲む。既に囲まれていれば外す。何も選んでいなければ記号の対を入れる。 */
function toggleWrap(marker: string): Command {
  return (view) => {
    const { state } = view;
    const n = marker.length;
    const tr = state.changeByRange((range) => {
      const wrapped =
        before(state, range.from, n) === marker && after(state, range.to, n) === marker && !range.empty;
      if (wrapped) {
        return {
          changes: [
            { from: range.from - n, to: range.from },
            { from: range.to, to: range.to + n },
          ],
          range: EditorSelection.range(range.from - n, range.to - n),
        };
      }
      return {
        changes: [
          { from: range.from, insert: marker },
          { from: range.to, insert: marker },
        ],
        range: EditorSelection.range(range.from + n, range.to + n),
      };
    });
    view.dispatch(state.update(tr, { scrollIntoView: true, userEvent: "input" }));
    return true;
  };
}

/** 記号の補完一式。既定のキー割り当てより先に効かせる。 */
export function markupHelpers() {
  return [
    starHandler,
    Prec.high(
      keymap.of([
        { key: "Enter", run: closeCodeFence },
        { key: "Enter", run: insertNewlineContinueMarkup },
        { key: "Backspace", run: deleteMarkupBackward },
        { key: "Mod-b", run: toggleWrap("**") },
        { key: "Mod-i", run: toggleWrap("*") },
      ]),
    ),
  ];
}
