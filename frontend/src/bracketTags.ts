/**
 * 本文の `[[タグ]]` を、タグのページへのリンクに変える remark プラグイン。
 *
 * `[[タグ]]` はノートにタグを付ける唯一の書き方で、プレビューではそのタグのページ
 * （ファイル名がそのタグのノート）へのリンクとして見せる。
 * 抜き出す規則は Go 側（internal/meta の bodyTags）と合わせてあり、コードの中に
 * 書かれたものはリンクにしない（コードは text ではない別の種類のノードになる）。
 *
 * 作ったリンクの hast 要素には data-tag 属性でタグを持たせる。
 * プレビューの a 要素の描画で、これを見てふつうのリンクと見分ける。
 */

/** remark の構文木のノード。使う項目だけを持つ。 */
interface MdNode {
  type: string;
  value?: string;
  url?: string;
  children?: MdNode[];
  data?: { hProperties?: Record<string, string> };
}

/** `[[タグ]]`。中身に角括弧と改行は含めない。 */
const BRACKET_TAG_RE = /\[\[([^[\]\n]+)\]\]/g;

/** リンクの中はリンクにできない（入れ子のリンクになる）ので、中へは潜らない。 */
const SKIP_TYPES = new Set(["link", "linkReference"]);

/** タグの表記を Go 側の model.NormalizeTag と同じ規則で整える。 */
export function normalizeTag(raw: string): string {
  return raw.trim().replace(/^#/, "").trim().split(/\s+/).filter(Boolean).join(" ");
}

/** 1 つのテキストを、`[[タグ]]` の前後のテキストとリンクに分ける。`[[タグ]]` が無ければ null。 */
function splitText(value: string): MdNode[] | null {
  const out: MdNode[] = [];
  let last = 0;
  for (const m of value.matchAll(BRACKET_TAG_RE)) {
    const tag = normalizeTag(m[1]);
    if (tag === "") continue;
    const at = m.index ?? 0;
    if (at > last) out.push({ type: "text", value: value.slice(last, at) });
    out.push({
      type: "link",
      url: "",
      children: [{ type: "text", value: m[1].trim() }],
      data: { hProperties: { dataTag: tag } },
    });
    last = at + m[0].length;
  }
  if (out.length === 0) return null;
  if (last < value.length) out.push({ type: "text", value: value.slice(last) });
  return out;
}

function walk(node: MdNode) {
  if (!node.children) return;
  let changed = false;
  const next: MdNode[] = [];
  for (const child of node.children) {
    if (child.type === "text" && child.value !== undefined) {
      const parts = splitText(child.value);
      if (parts) {
        next.push(...parts);
        changed = true;
        continue;
      }
    } else if (!SKIP_TYPES.has(child.type)) {
      walk(child);
    }
    next.push(child);
  }
  if (changed) node.children = next;
}

export function remarkBracketTags() {
  return (tree: unknown) => walk(tree as MdNode);
}
