/**
 * 編集画面のライブプレビュー。テキストエディタと見た目を分けず、書いたその場で整えて見せる。
 *
 * ファイルの中身は書いたままの Markdown で、見た目だけを変える。見出しは大きく、太字は太く、
 * `[[タグ]]` やリンクはリンクの色にして、`##` や `**` のような記号は隠す。カーソルがある
 * 行（リンクや強調ならその要素）だけは記号を見せるので、そのまま書き直せる。
 * エディタからフォーカスが外れている間は、すべての記号を隠す。
 *
 * 行の中で完結する見た目は、画面に見えている範囲だけを ViewPlugin で作る。表・mermaid・
 * リンクカードのように行ごと置き換えるものは、CodeMirror の決まりで StateField から渡す。
 */

import { syntaxTree } from "@codemirror/language";
import { Prec, StateField, type EditorState, type Range } from "@codemirror/state";
import {
  Decoration,
  EditorView,
  keymap,
  ViewPlugin,
  type Command,
  type DecorationSet,
  type ViewUpdate,
} from "@codemirror/view";
import type { SyntaxNode } from "@lezer/common";
import { normalizeTag } from "../bracketTags";
import { EXTERNAL_LINK, EXTERNAL_LINK_RE, INTERNAL_LINK, MISSING_LINK, noteLink, resolveNotePath } from "../links";
import { resolveLocalPath } from "../notePath";
import {
  focusedField,
  followHref,
  frontMatterField,
  hostOf,
  refreshDecorations,
  setFocused,
  type EditorHost,
} from "./host";
import {
  BulletWidget,
  CheckboxWidget,
  CodeLabelWidget,
  ExternalMarkWidget,
  ImageWidget,
  LinkCardWidget,
  MermaidWidget,
  TableWidget,
} from "./widgets";

/** 見出しの行。prose（以前のプレビュー）に近い大きさにする。行の間隔は余白（padding）で取る。
 * 行の余白はエディタの土台（theme.ts）が 0 にしているので、! を付けて優先させる。 */
const HEADING_LINE: Record<number, string> = {
  1: "text-[2em] leading-tight font-semibold pt-[0.6em]! pb-[0.3em]!",
  2: "text-[1.6em] leading-tight font-semibold pt-[0.8em]! pb-[0.3em]! border-b border-line",
  3: "text-[1.3em] leading-snug font-semibold pt-[0.7em]! pb-[0.2em]!",
  4: "text-[1.1em] font-semibold pt-[0.6em]!",
  5: "font-semibold pt-[0.5em]!",
  6: "font-semibold text-ink-muted pt-[0.5em]!",
};
const FRONT_MATTER_LINE = "font-mono text-[0.8em] text-ink-faint";
const QUOTE_LINE = "border-l-4 border-line-strong pl-4! text-ink-muted";
const CODE_LINE = "bg-sunken px-4! font-mono text-[0.85em] leading-relaxed";
const CODE_FIRST_LINE = "rounded-t-lg pt-2!";
const CODE_LAST_LINE = "rounded-b-lg pb-2!";
const TABLE_LINE = "font-mono text-[0.85em]";
const HR_LINE =
  "bg-[linear-gradient(var(--color-line),var(--color-line))] bg-[length:100%_1px] bg-center bg-no-repeat";
const FAINT = Decoration.mark({ class: "text-ink-faint" });
const INLINE_CODE = Decoration.mark({ class: "rounded bg-sunken px-1 py-0.5 font-mono text-[0.85em]" });
const HIDDEN = Decoration.replace({});
const BULLET = Decoration.replace({ widget: new BulletWidget() });
const EXTERNAL_MARK = Decoration.widget({ widget: new ExternalMarkWidget(), side: 1 });

/** URL だけの行（リンクカードにする）。`<…>` で囲んだものも含む。 */
const BARE_URL_RE = /^<?(https?:\/\/[^\s<>]+)>?$/i;

/**
 * `<…>` で囲まずに書いた URL（GFM の自動リンク）の終わりの位置を返す。
 *
 * GFM の決まりでは、URL の末尾の `_` と `~` は URL に含めない。ところが X のユーザー名
 * （https://x.com/name_）のように、末尾が `_` の URL はよくあり、落とすと別のページになる。
 * そこで URL のすぐ後ろに続く `_` と `~` は、強調や取り消し線の記号でなく、その後ろが
 * 空白か行末なら、URL に含める。
 */
function bareUrlEnd(state: EditorState, to: number): number {
  const doc = state.doc;
  const line = doc.lineAt(to);
  let end = to;
  while (end < line.to && /[_~]/.test(doc.sliceString(end, end + 1))) end++;
  if (end === to) return to;
  if (end < line.to && !/\s/.test(doc.sliceString(end, end + 1))) return to;
  const tree = syntaxTree(state);
  for (let pos = to; pos < end; pos++) {
    const name = tree.resolveInner(pos, 1).name;
    if (name === "EmphasisMark" || name === "StrikethroughMark") return to;
  }
  return end;
}

/** 記号を見せるか（カーソルがかかっているか）を調べる。 */
function revealer(state: EditorState) {
  const focused = state.field(focusedField);
  const ranges = state.selection.ranges;
  return {
    /** from〜to にカーソル（選択範囲）がかかっているか。 */
    range: (from: number, to: number) => focused && ranges.some((r) => r.from <= to && r.to >= from),
    /** pos の行にカーソルがあるか。 */
    line: (pos: number) => {
      if (!focused) return false;
      const line = state.doc.lineAt(pos);
      return ranges.some((r) => r.from <= line.to && r.to >= line.from);
    },
  };
}

function childrenOf(node: SyntaxNode): SyntaxNode[] {
  const out: SyntaxNode[] = [];
  for (let c = node.firstChild; c; c = c.nextSibling) out.push(c);
  return out;
}

/** リンクの行き先の前後に付いた `<` `>` を外す。 */
function cleanHref(raw: string): string {
  return raw.trim().replace(/^<(.*)>$/, "$1");
}

/**
 * リンクの文字に付ける見た目。hidden（記号を隠している）ならクリックで行き先を開けるよう印を付ける。
 * 記号を見せているときは、カーソルを置けるよう Ctrl を押しながらのクリックだけで開く。
 */
function linkMark(host: EditorHost, href: string, hidden: boolean) {
  const notePath = resolveNotePath(href, host.entry);
  let cls = "";
  let title = href;
  if (notePath !== null) {
    const missing = host.missingLinks.has(noteLink(href).toLowerCase());
    cls = missing ? MISSING_LINK : INTERNAL_LINK;
    title = missing ? `${notePath}（まだありません。クリックで開くと作成できます）` : notePath;
  } else if (EXTERNAL_LINK_RE.test(href)) {
    cls = EXTERNAL_LINK;
  } else {
    cls = "text-accent-ink";
  }
  if (!hidden) title += "（Ctrl + クリックで開く）";
  return Decoration.mark({
    class: `${cls} cursor-pointer`,
    attributes: { "data-href": href, title, ...(hidden ? { "data-follow": "1" } : {}) },
  });
}

/** `[[タグ]]` の文字に付ける見た目。ページがまだ無いタグは赤みがかった色にする。 */
function tagMark(host: EditorHost, tag: string, hidden: boolean) {
  const missing = host.missingTags.has(tag.toLowerCase());
  let title = missing
    ? `「${tag}」のページはまだありません。クリックで開くと作成できます`
    : `「${tag}」のページを開く`;
  if (!hidden) title += "（Ctrl + クリック）";
  return Decoration.mark({
    class: `${missing ? MISSING_LINK : INTERNAL_LINK} cursor-pointer`,
    attributes: { "data-tag": tag, title, ...(hidden ? { "data-follow": "1" } : {}) },
  });
}

/** 画面に見えている範囲の、行の中で完結する見た目を作る。 */
function buildInline(view: EditorView): DecorationSet {
  const { state } = view;
  const host = hostOf(state);
  if (!host) return Decoration.none;
  const doc = state.doc;
  const reveal = revealer(state);
  const fmEnd = state.field(frontMatterField);
  const out: Range<Decoration>[] = [];
  const lineClasses = new Map<number, Set<string>>();
  const seen = new Set<string>();

  const addLine = (pos: number, cls: string) => {
    const from = doc.lineAt(pos).from;
    let set = lineClasses.get(from);
    if (!set) lineClasses.set(from, (set = new Set()));
    set.add(cls);
  };
  // 改行をまたいで隠す（置き換える）ことは CodeMirror の決まりでできないので、行の中に限る。
  const sameLine = (from: number, to: number) => doc.lineAt(from).number === doc.lineAt(to).number;
  const hide = (from: number, to: number) => {
    if (to > from && sameLine(from, to)) out.push(HIDDEN.range(from, to));
  };
  /** 記号の後ろの空白も一緒に隠す範囲の終わり。 */
  const afterSpaces = (pos: number, limit: number) => {
    while (pos < limit && /[ \t]/.test(doc.sliceString(pos, pos + 1))) pos++;
    return pos;
  };

  for (const { from, to } of view.visibleRanges) {
    // Front Matter は中身を読まないので、薄く小さく見せるだけにする。
    for (let pos = from; pos < Math.min(to, fmEnd); ) {
      const line = doc.lineAt(pos);
      addLine(line.from, FRONT_MATTER_LINE);
      pos = line.to + 1;
    }

    syntaxTree(state).iterate({
      from,
      to,
      enter: (ref) => {
        if (ref.name === "Document") return;
        if (ref.from < fmEnd) return false;
        const key = `${ref.name}:${ref.from}`;
        if (seen.has(key)) return false;
        seen.add(key);
        const node = ref.node;

        switch (ref.name) {
          case "ATXHeading1":
          case "ATXHeading2":
          case "ATXHeading3":
          case "ATXHeading4":
          case "ATXHeading5":
          case "ATXHeading6": {
            const line = doc.lineAt(ref.from);
            addLine(ref.from, HEADING_LINE[Number(ref.name.slice(-1))]);
            if (reveal.line(ref.from)) return;
            node.getChildren("HeaderMark").forEach((mark, i) => {
              if (i === 0) {
                hide(mark.from, afterSpaces(mark.to, line.to));
              } else {
                // 末尾の閉じの # は、前の空白ごと隠す。
                let start = mark.from;
                while (start > line.from && /[ \t]/.test(doc.sliceString(start - 1, start))) start--;
                hide(start, mark.to);
              }
            });
            return;
          }

          case "SetextHeading1":
          case "SetextHeading2": {
            const level = ref.name === "SetextHeading1" ? 1 : 2;
            const underline = node.getChild("HeaderMark");
            for (let pos = ref.from; pos <= ref.to; ) {
              const line = doc.lineAt(pos);
              if (underline && line.from <= underline.from) {
                addLine(line.from, "text-ink-faint");
              } else {
                addLine(line.from, HEADING_LINE[level]);
              }
              pos = line.to + 1;
            }
            if (underline && !reveal.range(ref.from, ref.to)) hide(underline.from, underline.to);
            return;
          }

          case "Emphasis":
          case "StrongEmphasis":
          case "Strikethrough": {
            if (reveal.range(ref.from, ref.to)) return;
            for (const c of childrenOf(node)) {
              if (c.name === "EmphasisMark" || c.name === "StrikethroughMark") hide(c.from, c.to);
            }
            return;
          }

          case "InlineCode": {
            out.push(INLINE_CODE.range(ref.from, ref.to));
            if (!reveal.range(ref.from, ref.to)) {
              for (const c of node.getChildren("CodeMark")) hide(c.from, c.to);
            }
            return false;
          }

          case "BracketTag": {
            const [open, close] = node.getChildren("BracketTagMark");
            if (!open || !close) return false;
            const tag = normalizeTag(doc.sliceString(open.to, close.from));
            const hidden = !reveal.range(ref.from, ref.to);
            out.push(tagMark(host, tag, hidden).range(open.to, close.from));
            if (hidden) {
              hide(open.from, open.to);
              hide(close.from, close.to);
            }
            return false;
          }

          case "Link": {
            const marks = node.getChildren("LinkMark");
            const url = node.getChild("URL");
            // 参照形式のリンク（[文字][名前]）は行き先が別の場所にあるので、そのまま見せる。
            if (!url || marks.length < 2) return;
            const textFrom = marks[0].to;
            const textTo = marks[1].from;
            if (textTo <= textFrom) return;
            const href = cleanHref(doc.sliceString(url.from, url.to));
            const hidden = !reveal.range(ref.from, ref.to);
            out.push(linkMark(host, href, hidden).range(textFrom, textTo));
            if (hidden) {
              hide(ref.from, textFrom);
              hide(textTo, ref.to);
              if (EXTERNAL_LINK_RE.test(href)) out.push(EXTERNAL_MARK.range(textTo));
            } else {
              out.push(FAINT.range(url.from, url.to));
            }
            return;
          }

          case "Image": {
            const marks = node.getChildren("LinkMark");
            const url = node.getChild("URL");
            if (!url || marks.length < 2) return;
            const src = cleanHref(doc.sliceString(url.from, url.to));
            const alt = doc.sliceString(marks[0].to, marks[1].from);
            const widget = new ImageWidget(src, alt, resolveLocalPath(src, host.entry));
            if (reveal.range(ref.from, ref.to) || !sameLine(ref.from, ref.to)) {
              // 書き直している間も、画像はすぐ後ろに見せておく。
              out.push(Decoration.widget({ widget, side: 1 }).range(ref.to));
              return;
            }
            out.push(Decoration.replace({ widget }).range(ref.from, ref.to));
            return false;
          }

          case "Autolink":
          case "URL": {
            // リンクや画像の行き先の URL は、それぞれの側で扱う。
            const parent = node.parent?.name;
            if (ref.name === "URL" && (parent === "Link" || parent === "Image" || parent === "Autolink" || parent === "LinkReference")) {
              return false;
            }
            const url = ref.name === "Autolink" ? node.getChild("URL") : node;
            if (!url) return false;
            const urlTo = ref.name === "URL" ? bareUrlEnd(state, url.to) : url.to;
            const href = doc.sliceString(url.from, urlTo);
            const hidden = !reveal.range(ref.from, urlTo);
            out.push(linkMark(host, href, hidden).range(url.from, urlTo));
            if (hidden && ref.name === "Autolink") {
              for (const c of node.getChildren("LinkMark")) hide(c.from, c.to);
            }
            return false;
          }

          case "Blockquote": {
            for (let pos = ref.from; pos <= ref.to; ) {
              const line = doc.lineAt(pos);
              addLine(line.from, QUOTE_LINE);
              pos = line.to + 1;
            }
            return;
          }

          case "QuoteMark": {
            if (!reveal.line(ref.from)) hide(ref.from, afterSpaces(ref.to, doc.lineAt(ref.from).to));
            return;
          }

          case "ListMark": {
            if (reveal.line(ref.from)) return;
            const item = node.parent;
            if (item?.getChild("Task")) {
              // タスクはチェックボックスだけを見せる。
              hide(ref.from, afterSpaces(ref.to, doc.lineAt(ref.from).to));
            } else if (item?.parent?.name === "BulletList") {
              out.push(BULLET.range(ref.from, ref.to));
            }
            return;
          }

          case "TaskMarker": {
            if (reveal.line(ref.from)) return;
            const checked = /x/i.test(doc.sliceString(ref.from, ref.to));
            out.push(Decoration.replace({ widget: new CheckboxWidget(checked) }).range(ref.from, ref.to));
            return;
          }

          case "HorizontalRule": {
            if (reveal.line(ref.from)) return;
            addLine(ref.from, HR_LINE);
            hide(ref.from, ref.to);
            return;
          }

          case "FencedCode":
          case "CodeBlock": {
            const first = doc.lineAt(ref.from);
            const last = doc.lineAt(ref.to);
            // 長いコードブロックでも、見えている範囲の行だけに付ける。
            const start = Math.max(first.number, doc.lineAt(from).number);
            const end = Math.min(last.number, doc.lineAt(to).number);
            for (let n = start; n <= end; n++) {
              const line = doc.line(n);
              addLine(line.from, CODE_LINE);
              if (n === first.number) addLine(line.from, CODE_FIRST_LINE);
              if (n === last.number) addLine(line.from, CODE_LAST_LINE);
            }
            if (ref.name === "FencedCode" && !reveal.range(ref.from, ref.to)) {
              // フェンスの行は空けて余白にし、言語名だけを右端に小さく出す。
              const marks = node.getChildren("CodeMark");
              const info = node.getChild("CodeInfo");
              const lang = info ? doc.sliceString(info.from, info.to) : "";
              if (first.to > first.from) {
                out.push(Decoration.replace({ widget: new CodeLabelWidget(lang) }).range(first.from, first.to));
              }
              if (marks.length > 1) {
                const closing = doc.lineAt(marks[marks.length - 1].from);
                if (closing.number !== first.number) hide(closing.from, closing.to);
              }
            }
            return false;
          }

          case "Table": {
            // 描いた表に置き換えないとき（カーソルが中にある・引用や箇条書きの中）は、等幅で見せる。
            for (let pos = ref.from; pos <= ref.to; ) {
              const line = doc.lineAt(pos);
              addLine(line.from, TABLE_LINE);
              pos = line.to + 1;
            }
            return;
          }
        }
        return;
      },
    });
  }

  for (const [from, classes] of lineClasses) {
    out.push(Decoration.line({ class: [...classes].join(" ") }).range(from));
  }
  return Decoration.set(out, true);
}

const inlinePlugin = ViewPlugin.fromClass(
  class {
    decorations: DecorationSet;

    constructor(view: EditorView) {
      this.decorations = buildInline(view);
    }

    update(u: ViewUpdate) {
      if (
        u.docChanged ||
        u.viewportChanged ||
        u.selectionSet ||
        syntaxTree(u.startState) !== syntaxTree(u.state) ||
        u.transactions.some((tr) => tr.effects.some((e) => e.is(refreshDecorations) || e.is(setFocused)))
      ) {
        this.decorations = buildInline(u.view);
      }
    }
  },
  { decorations: (v) => v.decorations },
);

/** 行ごと描いた結果に置き換える候補。種類と、置き換える行の範囲と、描くのに要るもの。 */
type BlockCandidate =
  | { kind: "table"; from: number; to: number; source: string }
  | { kind: "mermaid"; from: number; to: number; source: string }
  | { kind: "linkCard"; from: number; to: number; url: string };

/**
 * 行ごと置き換える候補（表・mermaid のコードブロック・URL だけの行）を集める。
 * 対象は文書の直下にあるものだけで、引用や箇条書きの中のものは行の頭に記号が付くので置き換えない。
 * 本文が変わったときだけ集め直し、カーソルが動いたときは集めた候補から作り直す。
 */
function collectBlocks(state: EditorState): BlockCandidate[] {
  const doc = state.doc;
  const fmEnd = state.field(frontMatterField);
  const out: BlockCandidate[] = [];

  for (let node = syntaxTree(state).topNode.firstChild; node; node = node.nextSibling) {
    if (node.from < fmEnd) continue;
    const first = doc.lineAt(node.from);
    const last = doc.lineAt(node.to);

    switch (node.name) {
      case "Table":
        if (first.from !== node.from) break;
        out.push({ kind: "table", from: first.from, to: last.to, source: doc.sliceString(first.from, last.to) });
        break;

      case "FencedCode": {
        // mermaid のコードブロックは、閉じているものだけを図にする。
        if (first.from !== node.from) break;
        const info = node.getChild("CodeInfo");
        const marks = node.getChildren("CodeMark");
        if (!info || doc.sliceString(info.from, info.to).toLowerCase() !== "mermaid" || marks.length < 2) break;
        const code = node.getChild("CodeText");
        const source = code ? doc.sliceString(code.from, code.to).trimEnd() : "";
        out.push({ kind: "mermaid", from: first.from, to: last.to, source });
        break;
      }

      case "Paragraph":
        // URL だけを書いた行は、段落の途中の行でもリンクカードにする。
        // 空行で区切らずに URL を続けて書いたり、すぐ次の行に文を書いたりできるように。
        for (let c = node.firstChild; c; c = c.nextSibling) {
          if (c.name !== "URL" && c.name !== "Autolink") continue;
          const line = doc.lineAt(c.from);
          const m = BARE_URL_RE.exec(line.text.trim());
          const to = c.name === "URL" ? bareUrlEnd(state, c.to) : c.to;
          if (!m || doc.sliceString(c.from, to) !== line.text.trim()) continue;
          out.push({ kind: "linkCard", from: line.from, to: line.to, url: m[1] });
        }
        break;
    }
  }
  return out;
}

/** 集めた候補のうち、カーソルが外れているものを描いた結果に置き換える。 */
function buildBlocks(state: EditorState, candidates: readonly BlockCandidate[]): DecorationSet {
  const host = hostOf(state);
  if (!host) return Decoration.none;
  const reveal = revealer(state);
  const out: Range<Decoration>[] = [];
  for (const c of candidates) {
    if (reveal.range(c.from, c.to)) continue;
    const widget =
      c.kind === "table"
        ? new TableWidget(c.source, host.stamp)
        : c.kind === "mermaid"
          ? new MermaidWidget(c.source)
          : new LinkCardWidget(c.url);
    // 行ごとのブロックとして置き換える。行の中に置くと、カーソル用の要素がカードの下に
    // 空の行を作ってしまう。
    out.push(Decoration.replace({ widget, block: true }).range(c.from, c.to));
  }
  return Decoration.set(out, true);
}

interface Blocks {
  candidates: BlockCandidate[];
  decorations: DecorationSet;
}

const blockField = StateField.define<Blocks>({
  create(state) {
    const candidates = collectBlocks(state);
    return { candidates, decorations: buildBlocks(state, candidates) };
  },
  update(value, tr) {
    let { candidates } = value;
    const treeChanged = syntaxTree(tr.startState) !== syntaxTree(tr.state);
    if (tr.docChanged || treeChanged) candidates = collectBlocks(tr.state);
    if (
      candidates !== value.candidates ||
      tr.selection ||
      tr.effects.some((e) => e.is(refreshDecorations) || e.is(setFocused))
    ) {
      return { candidates, decorations: buildBlocks(tr.state, candidates) };
    }
    return value;
  },
  provide: (f) => EditorView.decorations.from(f, (v) => v.decorations),
});

/**
 * 上下キー（Ctrl + N / Ctrl + P も）で、描いた結果に置き換えている行（表・図・リンクカード）へ入れるようにする。
 * CodeMirror は行ごとのブロックを上下の移動で飛ばすので、隣がそうした行なら、その行の端へ
 * カーソルを置く。カーソルが入ると元の記述に戻るので、そのまま書き直せる。
 */
function enterBlock(forward: boolean): Command {
  return (view) => {
    const { state } = view;
    const sel = state.selection;
    if (sel.ranges.length !== 1 || !sel.main.empty) return false;
    const line = state.doc.lineAt(sel.main.head);
    if (forward ? line.number >= state.doc.lines : line.number <= 1) return false;
    const next = state.doc.line(line.number + (forward ? 1 : -1));
    const block = state
      .field(blockField)
      .candidates.find((c) => (forward ? c.from === next.from : c.to === next.to));
    if (!block) return false;
    view.dispatch({ selection: { anchor: forward ? block.from : next.from }, scrollIntoView: true, userEvent: "select" });
    return true;
  };
}

const blockKeys = Prec.high(
  keymap.of([
    { key: "ArrowDown", run: enterBlock(true) },
    { key: "ArrowUp", run: enterBlock(false) },
    // Emacs 風の上下移動（emacsKeys.ts）でも同じように入れるようにする。
    { key: "Ctrl-n", run: enterBlock(true) },
    { key: "Ctrl-p", run: enterBlock(false) },
  ]),
);

/**
 * リンク・タグ・チェックボックスのクリック。記号を隠しているリンクとタグはクリックで、
 * 記号を見せているものは Ctrl を押しながらのクリックで行き先を開く。
 * カーソルを動かす前に受けたいので、click ではなく mousedown で受ける。
 */
const clickHandler = EditorView.domEventHandlers({
  mousedown(e, view) {
    if (e.button !== 0 || e.shiftKey || e.altKey) return false;
    const target = e.target as HTMLElement;

    if (target instanceof HTMLInputElement && target.dataset.task) {
      e.preventDefault();
      const pos = view.posAtDOM(target);
      const marker = view.state.sliceDoc(pos, pos + 3);
      if (/^\[[ xX]\]$/.test(marker)) {
        view.dispatch({
          changes: { from: pos + 1, to: pos + 2, insert: marker[1] === " " ? "x" : " " },
          userEvent: "input",
        });
      }
      return true;
    }

    const el = target.closest<HTMLElement>("[data-href], [data-tag]");
    if (!el || !view.contentDOM.contains(el)) return false;
    if (el.dataset.follow !== "1" && !(e.ctrlKey || e.metaKey)) return false;
    const host = hostOf(view.state);
    if (!host) return false;
    e.preventDefault();
    if (el.dataset.tag !== undefined) {
      host.onFollowTag(el.dataset.tag);
    } else if (el.dataset.href !== undefined) {
      followHref(host, el.dataset.href);
    }
    return true;
  },
});

/**
 * カーソルのある `[[タグ]]`・リンク・URL の行き先を開く（Ctrl + O）。
 * `[[タグ]]` はそのタグのページを、ノートへのリンクはそのノートを、外部リンクは既定のブラウザで開く。
 * カーソルの左右どちらに接していてもよい。何も無ければ何もしない。
 */
export const followAtCursor: Command = (view) => {
  const { state } = view;
  const host = hostOf(state);
  if (!host || state.selection.ranges.length !== 1) return false;
  const doc = state.doc;
  const pos = state.selection.main.head;
  const tree = syntaxTree(state);

  for (const side of [-1, 1] as const) {
    for (let n: SyntaxNode | null = tree.resolveInner(pos, side); n; n = n.parent) {
      switch (n.name) {
        case "BracketTag": {
          const [open, close] = n.getChildren("BracketTagMark");
          if (!open || !close) return false;
          host.onFollowTag(normalizeTag(doc.sliceString(open.to, close.from)));
          return true;
        }
        case "Link":
        case "Autolink": {
          const url = n.getChild("URL");
          if (!url) break;
          followHref(host, cleanHref(doc.sliceString(url.from, url.to)));
          return true;
        }
        case "URL": {
          // リンクの中の URL は、外側のリンクとして扱う。
          const parent = n.parent?.name;
          if (parent === "Link" || parent === "Autolink" || parent === "Image" || parent === "LinkReference") break;
          followHref(host, doc.sliceString(n.from, bareUrlEnd(state, n.to)));
          return true;
        }
      }
    }
  }

  // URL の末尾の `_` の上にカーソルがあるときは、構文木では URL の外になるので、行の中から探す。
  const line = doc.lineAt(pos);
  let found: string | null = null;
  tree.iterate({
    from: line.from,
    to: line.to,
    enter: (n) => {
      if (found !== null || n.name !== "URL" || n.node.parent?.name !== "Paragraph") return;
      const end = bareUrlEnd(state, n.to);
      if (n.from <= pos && pos <= end) found = doc.sliceString(n.from, end);
    },
  });
  if (found === null) return false;
  followHref(host, found);
  return true;
};

/** ライブプレビュー一式。 */
export function livePreview() {
  return [inlinePlugin, blockField, blockKeys, clickHandler];
}
