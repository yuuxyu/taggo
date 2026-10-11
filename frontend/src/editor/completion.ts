/**
 * 編集中の入力補完。
 *
 * - `[[` と打つと（`]]` は自動で補われる）、使われているタグとページのあるタグを候補に出す。
 *   選ぶとタグを入れて `]]` の後ろへ進む。候補に無い新しいタグは、そのまま打てばよい。
 * - 行の頭で ``` と打つと、コードブロックの言語名を候補に出す。
 */

import {
  autocompletion,
  pickedCompletion,
  type Completion,
  type CompletionContext,
  type CompletionResult,
} from "@codemirror/autocomplete";
import { syntaxTree } from "@codemirror/language";
import type { EditorView } from "@codemirror/view";
import type { SyntaxNode } from "@lezer/common";
import { getTagNames } from "../api/taggo";
import { inCode } from "./markup";

/** タグの候補。打つたびに Go 側へ問い合わせないよう、しばらく覚えておく。 */
let cached: { at: number; names: Promise<string[]> } | null = null;
const CACHE_MS = 5000;

function tagNames(): Promise<string[]> {
  if (!cached || Date.now() - cached.at > CACHE_MS) {
    cached = { at: Date.now(), names: getTagNames().catch(() => []) };
  }
  return cached.names;
}

/** タグを入れ、後ろに `]]` が無ければ補って、その後ろへカーソルを進める。 */
function applyTag(view: EditorView, completion: Completion, from: number, to: number) {
  const hasClose = view.state.sliceDoc(to, to + 2) === "]]";
  const insert = completion.label + (hasClose ? "" : "]]");
  view.dispatch({
    changes: { from, to, insert },
    selection: { anchor: from + completion.label.length + 2 },
    annotations: pickedCompletion.of(completion),
    userEvent: "input.complete",
  });
}

async function tagSource(ctx: CompletionContext): Promise<CompletionResult | null> {
  const typed = ctx.matchBefore(/\[\[[^[\]\n]*/);
  if (!typed || inCode(ctx.state, ctx.pos)) return null;
  const names = await tagNames();
  if (ctx.aborted || names.length === 0) return null;
  return {
    from: typed.from + 2,
    options: names.map((name) => ({ label: name, type: "tag", apply: applyTag })),
    validFor: /^[^[\]\n]*$/,
  };
}

/** コードブロックの言語名の候補。よく使うものと、図を描く mermaid。 */
const CODE_LANGUAGES = [
  "mermaid",
  "go",
  "typescript",
  "tsx",
  "javascript",
  "python",
  "rust",
  "java",
  "c",
  "cpp",
  "csharp",
  "sql",
  "json",
  "yaml",
  "toml",
  "html",
  "css",
  "markdown",
  "sh",
  "bash",
  "powershell",
  "dockerfile",
  "diff",
  "text",
];

function codeFenceSource(ctx: CompletionContext): CompletionResult | null {
  const line = ctx.state.doc.lineAt(ctx.pos);
  const m = /^(\s*)(`{3,}|~{3,})([\w+#.-]*)$/.exec(line.text.slice(0, ctx.pos - line.from));
  if (!m) return null;
  // コードブロックの中の行（閉じのフェンスを含む）では出さない。開きのフェンスの行だけ。
  let block: SyntaxNode | null = syntaxTree(ctx.state).resolveInner(line.from + m[1].length, 1);
  while (block && block.name !== "FencedCode") block = block.parent;
  if (block && ctx.state.doc.lineAt(block.from).number !== line.number) return null;
  return {
    from: ctx.pos - m[3].length,
    options: CODE_LANGUAGES.map((label) => ({ label, type: "keyword" })),
    validFor: /^[\w+#.-]*$/,
  };
}

/** 入力補完一式。 */
export function completions() {
  return autocompletion({
    override: [tagSource, codeFenceSource],
    icons: false,
  });
}
