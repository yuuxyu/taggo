/**
 * ノートの本文に書かれたパスを、ファイルシステム上の絶対パスへ直す補助関数。
 * プレビューの画像・リンクと、カードのサムネイルで共有する。
 */

import type { Entry } from "./api/taggo";

/** パスの解決に使う、ノートの置き場所。 */
export type NoteLocation = Pick<Entry, "path" | "relPath">;

/** パスを区切り文字で分解する。先頭の空要素（POSIX の "/"）は残す。 */
function splitPath(path: string): string[] {
  return path.split(/[\\/]/);
}

/** パスから親フォルダを取り出す。 */
function dirOf(path: string): string {
  const i = Math.max(path.lastIndexOf("/"), path.lastIndexOf("\\"));
  return i < 0 ? "" : path.slice(0, i);
}

/**
 * 開いているフォルダ（走査ルート）を求める。
 * 絶対パスの末尾から、表示用の相対パスを取り除いたものがルートになる。
 */
function rootOf(entry: NoteLocation): string {
  if (entry.relPath !== "" && entry.path.endsWith(entry.relPath)) {
    return entry.path.slice(0, entry.path.length - entry.relPath.length).replace(/[\\/]+$/, "");
  }
  return dirOf(entry.path);
}

/**
 * 本文に書かれた相対パスを、ファイルシステム上の絶対パスへ直す。
 *
 * ブラウザは相対 URL をアプリのページ基準で解決してしまうので、そのままでは
 * ノートの隣に置いたファイルを指せない。Markdown ファイルのあるフォルダを
 * 基準に絶対パスへ組み立て直す。"/" 始まりは、ノートからの相対ではなく
 * 開いているフォルダ基準として扱う。
 *
 * http(s): や data: などスキーム付きの URL は外部を指しているので null を返す。
 * Windows のドライブ文字（C:\… や C:/…）はスキームに見えるがパスなので通す。
 */
export function resolveLocalPath(target: string, entry: NoteLocation): string | null {
  const driveLetter = /^[a-z]:[\\/]/i.test(target);
  if (!driveLetter && /^[a-z][a-z0-9+.-]*:/i.test(target)) return null;

  // 末尾のフラグメント（#…）はパスの一部ではないので落とす。
  const cleaned = target.replace(/#.*$/, "");
  let decoded = cleaned;
  try {
    decoded = decodeURIComponent(cleaned);
  } catch {
    // 壊れたエスケープはそのままのパスとして扱う。
  }

  const sep = entry.path.includes("\\") ? "\\" : "/";
  const base = driveLetter
    ? []
    : splitPath(/^[\\/]/.test(decoded) ? rootOf(entry) : dirOf(entry.path));

  const segments = [...base];
  for (const part of splitPath(decoded)) {
    if (part === "" || part === ".") continue;
    if (part === "..") {
      if (segments.length > 1) segments.pop();
      continue;
    }
    segments.push(part);
  }
  return segments.join(sep);
}
