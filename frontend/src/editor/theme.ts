/**
 * 編集画面の見た目。
 *
 * 本文の文字や見出しの装飾は、ライブプレビュー（livePreview.ts）が Tailwind のクラスで付ける。
 * ここには、CodeMirror が自前で描く部分（エディタの枠・カーソル・補完の一覧）と、
 * 構文ごとの文字の色（コードブロックの色分けなど）だけを置く。色はアプリの配色トークンを使う。
 */

import { HighlightStyle, syntaxHighlighting } from "@codemirror/language";
import { EditorView } from "@codemirror/view";
import { tags as t } from "@lezer/highlight";

const baseTheme = EditorView.theme({
  "&": {
    // 以前のプレビュー（prose-lg）と同じ文字の大きさと行の高さ。
    fontSize: "1.125rem",
    color: "var(--color-ink)",
    backgroundColor: "transparent",
  },
  "&.cm-focused": { outline: "none" },
  ".cm-scroller": {
    fontFamily: "var(--font-sans)",
    lineHeight: "1.78",
    overflow: "visible",
  },
  ".cm-content": {
    padding: "0 0 4rem",
    caretColor: "var(--color-ink)",
  },
  ".cm-line": { padding: "0" },
  ".cm-placeholder": { color: "var(--color-ink-faint)" },
  // 補完の一覧。ノートの画面より手前に重ねる。
  ".cm-tooltip": {
    zIndex: "200",
    border: "1px solid var(--color-line)",
    borderRadius: "0.5rem",
    backgroundColor: "var(--color-surface)",
    color: "var(--color-ink)",
    boxShadow: "var(--shadow-pop)",
    overflow: "hidden",
  },
  ".cm-tooltip.cm-tooltip-autocomplete > ul": {
    fontFamily: "var(--font-sans)",
    fontSize: "0.875rem",
    maxHeight: "16rem",
  },
  ".cm-tooltip.cm-tooltip-autocomplete > ul > li": { padding: "0.25rem 0.75rem" },
  ".cm-tooltip.cm-tooltip-autocomplete > ul > li[aria-selected]": {
    backgroundColor: "var(--color-accent-soft)",
    color: "var(--color-accent-ink)",
  },
  ".cm-completionMatchedText": { textDecoration: "none", fontWeight: "600" },
});

/**
 * 構文ごとの文字の装飾。Markdown の強調などと、コードブロックの色分け。
 * コードの色は以前のプレビュー（highlight.js の github 風の配色）に合わせ、暗いテーマでも読めるようにする。
 */
const highlightStyle = HighlightStyle.define([
  // Markdown
  { tag: t.heading, class: "font-semibold" },
  { tag: t.emphasis, class: "italic" },
  { tag: t.strong, class: "font-bold" },
  { tag: t.strikethrough, class: "line-through" },
  { tag: t.monospace, class: "font-mono" },
  { tag: t.quote, class: "text-ink-muted" },
  { tag: [t.processingInstruction, t.labelName, t.contentSeparator], class: "text-ink-faint" },
  // コード
  { tag: [t.comment, t.meta], class: "text-ink-faint italic" },
  { tag: [t.keyword, t.operatorKeyword, t.modifier, t.controlKeyword], class: "text-[#cf222e] dark:text-[#d08fd0]" },
  { tag: [t.string, t.special(t.string), t.regexp, t.attributeValue], class: "text-[#0a3069] dark:text-[#8fd3c3]" },
  { tag: [t.number, t.bool, t.null, t.atom, t.literal], class: "text-[#0550ae] dark:text-[#e0b070]" },
  {
    tag: [t.function(t.variableName), t.function(t.propertyName), t.definition(t.function(t.variableName))],
    class: "text-[#8250df] dark:text-[#83b4e8]",
  },
  { tag: [t.typeName, t.className, t.namespace], class: "text-[#953800] dark:text-[#e0b070]" },
  { tag: [t.tagName, t.attributeName, t.propertyName], class: "text-[#116329] dark:text-[#8fd3c3]" },
  { tag: t.invalid, class: "text-danger" },
]);

/** 編集画面の見た目一式。 */
export function editorTheme() {
  return [baseTheme, syntaxHighlighting(highlightStyle)];
}
