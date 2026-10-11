/**
 * 本文に書かれたリンクの行き先を見分ける補助関数。
 * 編集画面の本文と、表のように HTML へ描き直す部分（MarkdownBlock）で共有する。
 */

import type { Entry } from "./api/taggo";
import { resolveLocalPath } from "./notePath";

/** 既定のブラウザへ渡してよいリンク。ローカルファイルや任意のスキームは OS に開かせない。 */
export const EXTERNAL_LINK_RE = /^(https?|mailto):/i;

/**
 * ノートへのリンクなら、その行き先の絶対パスを返す。
 * 対象は .md / .markdown を指す相対・絶対パスのリンクだけで、
 * 外部 URL や画像・その他のファイルへのリンクは対象にしない。
 */
export function resolveNotePath(href: string | undefined, entry: Entry): string | null {
  if (!href || !/\.(md|markdown)(#.*)?$/i.test(href)) return null;
  return resolveLocalPath(href, entry);
}

/**
 * 本文のリンクを、Go 側がノート間のリンクとして持つ形へ直す。
 * フラグメント（#…）を除き、エスケープを戻す。
 */
export function noteLink(href: string): string {
  const path = href.replace(/#.*$/, "");
  try {
    return decodeURIComponent(path);
  } catch {
    return path;
  }
}

/** 壊れたエスケープで例外にならない decodeURI。 */
export function safeDecodeURI(s: string): string {
  try {
    return decodeURI(s);
  } catch {
    return s;
  }
}

/**
 * アプリの中でたどるリンク（`[[タグ]]` とノートへのリンク）の見た目。
 * 外部リンクと見分けられるよう、下線はホバーしたときだけ出す。
 */
export const INTERNAL_LINK = "text-accent-ink no-underline hover:underline";

/** 行き先がまだ無いページへのリンクの見た目。赤みがかった色で、点線の下線を常に出す。 */
export const MISSING_LINK = "text-danger underline decoration-dashed";

/** 既定のブラウザで開く外部リンクの見た目。行き先の記号「↗」は呼び出し側で添える。 */
export const EXTERNAL_LINK = "text-accent-ink underline";
