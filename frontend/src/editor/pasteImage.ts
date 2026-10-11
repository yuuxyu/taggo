/**
 * 編集画面への画像の貼り付けとドロップ。
 *
 * 画像は開いているフォルダの直下に日時の名前で保存し（Go 側の SaveImage）、カーソルの位置
 * （ドロップならドロップした位置）に `![](名前)` を入れる。保存を待つ間は仮の記述を入れておき、
 * 保存できたら差し替える。その間に本文を書き進めても、入れる位置がずれないようにするため。
 *
 * 貼り付けた中身に文字があれば、画像ではなく文字として貼る（表計算ソフトのセルのように、
 * 文字と一緒に画像も載ってくるものがあるため）。
 */

import { EditorView } from "@codemirror/view";
import { saveImage } from "../api/taggo";
import { hostOf } from "./host";

/** 取り出した画像ファイル。画像が無ければ空。 */
function imageFiles(data: DataTransfer | null): File[] {
  if (!data) return [];
  return [...data.files].filter((f) => f.type.startsWith("image/"));
}

/** ファイルの中身を Base64 にする。Wails のバインディングへバイト列を渡すため。 */
async function toBase64(file: File): Promise<string> {
  const bytes = new Uint8Array(await file.arrayBuffer());
  let binary = "";
  // 大きな画像で引数の数の上限を超えないよう、少しずつ文字にする。
  const chunk = 0x8000;
  for (let i = 0; i < bytes.length; i += chunk) {
    binary += String.fromCharCode(...bytes.subarray(i, i + chunk));
  }
  return btoa(binary);
}

let pendingSeq = 0;

/** 保存を待っている画像の仮の記述の、行き先の頭。 */
export const PENDING_IMAGE_PREFIX = "taggo-pending-";

/**
 * 保存を待っている画像の仮の記述。待つ間にノートを保存しても、ファイルには書かないよう取り除く。
 */
export const PENDING_IMAGE_RE = /!\[保存中…\]\(taggo-pending-\d+\)\n?/g;

/** 画像を保存して pos に入れる。 */
async function insertImages(view: EditorView, files: File[], pos: number) {
  const host = hostOf(view.state);
  if (!host) return;
  const notePath = host.entry.path;

  // 保存を待つ間の仮の記述。保存できたら、この文字列を探して差し替える。
  const placeholders = files.map(() => `![保存中…](${PENDING_IMAGE_PREFIX}${++pendingSeq})`);
  const insert = placeholders.join("\n");
  view.dispatch({
    changes: { from: pos, insert },
    selection: { anchor: pos + insert.length },
    userEvent: "input.paste",
  });

  await Promise.all(
    files.map(async (file, i) => {
      let replacement = "";
      try {
        const name = await saveImage(notePath, await toBase64(file));
        replacement = `![](${name})`;
      } catch (err) {
        hostOf(view.state)?.onError(`画像を保存できませんでした: ${String(err)}`);
      }
      // 保存を待つ間に別のノートへ移っていれば、何もしない（仮の記述は保存されない）。
      if (hostOf(view.state)?.entry.path !== notePath) return;
      const at = view.state.doc.toString().indexOf(placeholders[i]);
      if (at < 0) return;
      // 保存できなかったときは、仮の記述と、つなぎの改行を消す。
      let to = at + placeholders[i].length;
      if (replacement === "" && view.state.sliceDoc(to, to + 1) === "\n") to++;
      view.dispatch({ changes: { from: at, to, insert: replacement }, userEvent: "input.paste" });
    }),
  );
}

/** 画像の貼り付けとドロップを受ける拡張。 */
export function imagePaste() {
  return EditorView.domEventHandlers({
    paste(e, view) {
      const text = e.clipboardData?.getData("text/plain") ?? "";
      const files = imageFiles(e.clipboardData);
      if (files.length === 0 || text.trim() !== "") return false;
      e.preventDefault();
      void insertImages(view, files, view.state.selection.main.head);
      return true;
    },
    drop(e, view) {
      const files = imageFiles(e.dataTransfer);
      if (files.length === 0) return false;
      e.preventDefault();
      const pos = view.posAtCoords({ x: e.clientX, y: e.clientY }) ?? view.state.selection.main.head;
      void insertImages(view, files, pos);
      return true;
    },
  });
}
