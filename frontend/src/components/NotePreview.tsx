/**
 * カードの上部に出すノートの中身。一覧のカードと、関連ページのカードで共有する。
 *
 * 本文で画像か YouTube 動画を使っていれば、その最初のもののサムネイルを、
 * そうでなければ本文の抜粋を出す。中身がクラウド上にしか無いノートは本文を
 * 読んでいないので、その印を出す。
 */

import { useState } from "react";
import { CloudIcon, PlayIcon } from "@heroicons/react/16/solid";
import { imageURL, type Entry } from "../api/taggo";
import { resolveLocalPath } from "../notePath";

/** 中身を出すのに要るノートの情報。 */
export type PreviewSource = Pick<Entry, "path" | "relPath" | "preview" | "thumbnail" | "cloudOnly">;

interface Props {
  note: PreviewSource;
  /** 抜粋を何行まで出すか。カードの大きさに合わせる。 */
  lines?: 3 | 6;
}

const LINE_CLAMP = { 3: "line-clamp-3", 6: "line-clamp-6" } as const;

export function NotePreview({ note, lines = 6 }: Props) {
  if (note.cloudOnly) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-1.5 p-3 text-center text-xs text-ink-faint">
        <CloudIcon className="size-6" aria-hidden="true" />
        クラウド上のみ
      </div>
    );
  }
  if (note.thumbnail) {
    return <Thumbnail key={note.thumbnail} note={note} thumbnail={note.thumbnail} lines={lines} />;
  }
  return <Excerpt note={note} lines={lines} />;
}

function Excerpt({ note, lines }: { note: PreviewSource; lines: 3 | 6 }) {
  return (
    <div className="h-full overflow-hidden px-3.5 py-3">
      <p className={`m-0 ${LINE_CLAMP[lines]} text-xs leading-relaxed text-ink-muted`}>
        {note.preview || "（本文なし）"}
      </p>
    </div>
  );
}

/** Go 側が YouTube 動画から組み立てたサムネイルの URL。 */
const YOUTUBE_THUMBNAIL_RE = /^https:\/\/i\.ytimg\.com\//;

/**
 * 本文で最初に使われている画像か、YouTube 動画のサムネイル。
 * フォルダ内の画像は taggo の配信 URL で読み込む。読み込めなければ
 * （クラウド上にしか無い・フォルダの外・画像でない など）本文の抜粋に戻す。
 */
function Thumbnail({ note, thumbnail, lines }: { note: PreviewSource; thumbnail: string; lines: 3 | 6 }) {
  const [failed, setFailed] = useState(false);
  if (failed) return <Excerpt note={note} lines={lines} />;

  const localPath = resolveLocalPath(thumbnail, note);
  const src = localPath === null ? thumbnail : imageURL(localPath);
  return (
    <>
      <img
        src={src}
        alt=""
        loading="lazy"
        decoding="async"
        draggable={false}
        className="size-full object-cover"
        onError={() => setFailed(true)}
      />
      {YOUTUBE_THUMBNAIL_RE.test(thumbnail) && (
        <span className="absolute inset-0 grid place-items-center" aria-hidden="true">
          <span className="grid size-9 place-items-center rounded-full bg-black/60 text-white">
            <PlayIcon className="ml-0.5 size-4.5" />
          </span>
        </span>
      )}
    </>
  );
}
