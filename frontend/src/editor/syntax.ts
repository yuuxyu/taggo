/**
 * 編集画面の Markdown の構文。GFM に、タグを付ける `[[タグ]]` を足す。
 *
 * `[[タグ]]` を読む規則は Go 側（internal/meta の bodyTags）と合わせてあり、中身に角括弧と
 * 改行を含まないものだけをタグとする。コードの中はコードの構文が先に読むので、タグにならない。
 */

import { markdown, markdownLanguage } from "@codemirror/lang-markdown";
import { languages } from "@codemirror/language-data";
import { tags } from "@lezer/highlight";
import type { MarkdownConfig } from "@lezer/markdown";
import { normalizeTag } from "../bracketTags";

const OPEN_BRACKET = 91; // [
const CLOSE_BRACKET = 93; // ]
const NEWLINE = 10;

/** `[[タグ]]` を BracketTag（両端の `[[` `]]` は BracketTagMark）として読む。 */
const bracketTags: MarkdownConfig = {
  defineNodes: [
    { name: "BracketTag", style: tags.link },
    { name: "BracketTagMark", style: tags.processingInstruction },
  ],
  parseInline: [
    {
      name: "BracketTag",
      // ふつうのリンク（[…](…)）の開き括弧として読まれる前に取る。
      before: "Link",
      parse(cx, next, pos) {
        if (next !== OPEN_BRACKET || cx.char(pos + 1) !== OPEN_BRACKET) return -1;
        let end = pos + 2;
        for (; end < cx.end; end++) {
          const c = cx.char(end);
          if (c === OPEN_BRACKET || c === NEWLINE) return -1;
          if (c === CLOSE_BRACKET) break;
        }
        if (end + 1 >= cx.end || cx.char(end + 1) !== CLOSE_BRACKET) return -1;
        if (normalizeTag(cx.slice(pos + 2, end)) === "") return -1;
        return cx.addElement(
          cx.elt("BracketTag", pos, end + 2, [
            cx.elt("BracketTagMark", pos, pos + 2),
            cx.elt("BracketTagMark", end, end + 2),
          ]),
        );
      },
    },
  ],
};

/**
 * 編集画面の言語。コードブロックは言語名に合わせて色分けする（言語の定義は使うときに読み込む）。
 * 箇条書きの続きを Enter で補う既定のキー割り当ては、本文の補完（markup.ts）と並べて
 * 優先度を決めたいので、ここでは入れない。
 */
export function markdownSyntax() {
  return markdown({
    base: markdownLanguage,
    codeLanguages: languages,
    extensions: [bracketTags],
    addKeymap: false,
  });
}
