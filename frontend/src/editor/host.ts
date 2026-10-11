/**
 * 編集画面（CodeMirror）と、それを置いている画面（DetailPanel）をつなぐもの。
 *
 * リンクをたどる・画像を開くといった操作は画面の側が受け持つ。CodeMirror の拡張や
 * ウィジェットは作り直さずに使い回すので、最新の受け口は ref に入れて渡し、
 * 使う時点で current を読む。
 */

import { Facet, StateEffect, StateField, type EditorState, type Text } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import { BrowserOpenURL } from "../../wailsjs/runtime/runtime";
import type { Entry } from "../api/taggo";
import { EXTERNAL_LINK_RE, noteLink, resolveNotePath } from "../links";

export interface EditorHost {
  /** 編集しているノート。相対パスの画像とリンクの基準になる。 */
  entry: Entry;
  /**
   * ノートへのリンクをたどる。link は書かれたパスをデコードしたもの。
   * path を省くと、行き先がまだ無いノートとして開く。
   */
  onFollowLink: (link: string, path?: string) => void;
  /** `[[タグ]]` のページを開く。 */
  onFollowTag: (tag: string) => void;
  /** 本文中の画像を画像ビューアで開く。 */
  onOpenImage: (path: string, alt?: string) => void;
  onError: (message: string) => void;
  /** 行き先がまだ無いリンク（小文字にしたもの）。 */
  missingLinks: ReadonlySet<string>;
  /** ページがまだ無いタグ（小文字にしたもの）。 */
  missingTags: ReadonlySet<string>;
  /**
   * missingLinks / missingTags が変わるたびに増やす番号。
   * HTML に描き直した表などは、これが変わったら描き直す。
   */
  stamp: number;
}

export interface HostRef {
  current: EditorHost;
}

/** 画面側の受け口。エディタの状態から引けるようにしておく。 */
export const hostFacet = Facet.define<HostRef, HostRef | null>({
  combine: (values) => values[0] ?? null,
});

export function hostOf(state: EditorState): EditorHost | null {
  return state.facet(hostFacet)?.current ?? null;
}

/** 行き先が無いリンクやタグが変わったので、見た目を作り直させる合図。 */
export const refreshDecorations = StateEffect.define<null>();

/** エディタにフォーカスがあるかどうか。フォーカスが無い間は、記号をすべて隠して読みやすくする。 */
export const setFocused = StateEffect.define<boolean>();

export const focusedField = StateField.define<boolean>({
  create: () => false,
  update(value, tr) {
    for (const e of tr.effects) if (e.is(setFocused)) value = e.value;
    return value;
  },
});

export const focusTracker = EditorView.focusChangeEffect.of((_, focusing) => setFocused.of(focusing));

/**
 * 先頭の「---」で囲んだブロック（ほかのツールの Front Matter）の終わり、
 * つまり本文の始まりの位置を返す。無ければ 0。
 * Go 側の meta.StripFrontMatter と同じく、閉じていない「---」はただの本文とみなす。
 */
export function frontMatterEnd(doc: Text): number {
  if (doc.lines < 2 || doc.line(1).text !== "---") return 0;
  for (let n = 2; n <= doc.lines; n++) {
    const text = doc.line(n).text;
    if (text === "---" || text === "...") return n < doc.lines ? doc.line(n + 1).from : doc.length;
  }
  return 0;
}

/** Front Matter の終わりの位置。本文が変わったときだけ数え直す。 */
export const frontMatterField = StateField.define<number>({
  create: (state) => frontMatterEnd(state.doc),
  update: (value, tr) => (tr.docChanged ? frontMatterEnd(tr.state.doc) : value),
});

/**
 * 本文に書かれたリンク href をたどる。ノートへのリンクはアプリの中で開き、
 * 外部リンクは既定のブラウザで開く。それ以外（ローカルのファイルなど）は何もしない。
 */
export function followHref(host: EditorHost, href: string) {
  const notePath = resolveNotePath(href, host.entry);
  if (notePath !== null) {
    const link = noteLink(href);
    host.onFollowLink(link, host.missingLinks.has(link.toLowerCase()) ? undefined : notePath);
    return;
  }
  if (EXTERNAL_LINK_RE.test(href)) BrowserOpenURL(href);
}
