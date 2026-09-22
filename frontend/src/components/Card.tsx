/**
 * グリッドに並ぶノートのカード 1 枚。
 * 上部に本文の抜粋を、下部に埋め込みタグをバッジで並べる。
 */

import { memo, useState } from "react";
import { BookOpenIcon, CheckIcon, CloudIcon, LockClosedIcon, PlayIcon } from "@heroicons/react/16/solid";
import { imageURL, type Entry } from "../api/taggo";
import { resolveLocalPath } from "../notePath";
import { TagBadge } from "./TagBadge";

interface Props {
  entry: Entry;
  selected: boolean;
  /** 選択モード中は、カードのクリックが詳細表示ではなく選択の切り替えになる。 */
  selectionMode: boolean;
  onOpen: (entry: Entry) => void;
  onToggleSelect: (entry: Entry) => void;
  onTagClick: (tag: string) => void;
}

/** ファイルサイズを読みやすい単位へ変換する。 */
function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  const units = ["KB", "MB", "GB"];
  let value = bytes / 1024;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  return `${value.toFixed(value < 10 ? 1 : 0)} ${units[unit]}`;
}

function CardPreview({ entry }: { entry: Entry }) {
  // 中身がクラウド上にしか無いノートは本文を読んでいないので、抜粋の代わりに印を出す。
  if (entry.cloudOnly) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-1.5 p-3 text-center text-xs text-ink-faint">
        <CloudIcon className="size-6" aria-hidden="true" />
        クラウド上のみ
      </div>
    );
  }

  if (entry.thumbnail) {
    return <CardThumbnail key={entry.thumbnail} entry={entry} thumbnail={entry.thumbnail} />;
  }
  return <CardExcerpt entry={entry} />;
}

function CardExcerpt({ entry }: { entry: Entry }) {
  return (
    <div className="h-full overflow-hidden px-3.5 py-3">
      <p className="m-0 line-clamp-6 text-xs leading-relaxed text-ink-muted">
        {entry.preview || "（本文なし）"}
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
function CardThumbnail({ entry, thumbnail }: { entry: Entry; thumbnail: string }) {
  const [failed, setFailed] = useState(false);
  if (failed) return <CardExcerpt entry={entry} />;

  const localPath = resolveLocalPath(thumbnail, entry);
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

export const Card = memo(function Card({
  entry,
  selected,
  selectionMode,
  onOpen,
  onToggleSelect,
  onTagClick,
}: Props) {
  const handleClick = (event: React.MouseEvent) => {
    // 修飾キー付きのクリックは、選択モードに入っていなくても選択操作として扱う。
    if (selectionMode || event.metaKey || event.ctrlKey || event.shiftKey) {
      onToggleSelect(entry);
      return;
    }
    onOpen(entry);
  };

  // カードはクリックで開くだけでなく、キーボードだけでも到達・操作できるようにする。
  const handleKeyDown = (event: React.KeyboardEvent) => {
    if (event.key !== "Enter" && event.key !== " ") return;
    event.preventDefault();
    if (selectionMode || event.ctrlKey || event.metaKey) {
      onToggleSelect(entry);
      return;
    }
    onOpen(entry);
  };

  return (
    <article
      className={`group relative flex h-full cursor-pointer flex-col overflow-hidden rounded-lg border bg-surface transition hover:-translate-y-px hover:shadow-card focus-visible:outline-hidden focus-visible:ring-3 focus-visible:ring-accent-soft ${
        selected
          ? "border-accent ring-2 ring-accent-soft"
          : entry.err
            ? "border-danger hover:border-line-strong"
            : "border-line hover:border-line-strong"
      }`}
      role="button"
      tabIndex={0}
      aria-label={`${entry.title} を開く`}
      onClick={handleClick}
      onKeyDown={handleKeyDown}
    >
      <button
        type="button"
        role="checkbox"
        aria-checked={selected}
        aria-label={selected ? "選択を解除" : "選択に追加"}
        className={`absolute top-2 left-2 z-2 grid size-5 place-items-center rounded-md border transition group-hover:opacity-100 focus-visible:opacity-100 ${
          selected
            ? "border-accent bg-accent text-white opacity-100"
            : "border-white/55 bg-black/35 text-white"
        } ${selected || selectionMode ? "opacity-100" : "opacity-0"}`}
        onClick={(e) => {
          e.stopPropagation();
          onToggleSelect(entry);
        }}
      >
        {selected && <CheckIcon className="size-3.5" />}
      </button>

      <div className="relative h-35 shrink-0 overflow-hidden border-b border-line bg-sunken">
        <CardPreview entry={entry} />
      </div>

      <div className="flex min-h-0 flex-1 flex-col gap-1.5 px-3 pt-2.5 pb-3">
        <h3 className="m-0 line-clamp-2 text-sm leading-snug font-semibold" title={entry.relPath}>
          {entry.title}
        </h3>
        <div className="flex items-center gap-2 text-xs text-ink-faint">
          <span className="rounded-sm bg-sunken px-1.5 uppercase">{entry.ext.replace(".", "")}</span>
          <span className="tabular-nums">{formatSize(entry.size)}</span>
          {entry.tagPage && (
            <span
              className="inline-flex min-w-0 items-center gap-0.5 text-accent-ink"
              title={`#${entry.tagPage} のタグページ`}
            >
              <BookOpenIcon className="size-3 shrink-0" aria-hidden="true" />
              <span className="truncate">タグページ</span>
            </span>
          )}
          {entry.cloudOnly ? (
            <span
              className="inline-flex items-center gap-0.5 text-ink-muted"
              title="中身はまだダウンロードされていません"
            >
              <CloudIcon className="size-3" aria-hidden="true" />
              未ダウンロード
            </span>
          ) : (
            !entry.writable && (
              <span className="inline-flex items-center gap-0.5 text-danger" title="読み取り専用">
                <LockClosedIcon className="size-3" aria-hidden="true" />
                読み取り専用
              </span>
            )
          )}
        </div>

        {entry.err && <p className="m-0 line-clamp-2 text-xs text-danger">{entry.err}</p>}

        {entry.tags.length > 0 && (
          <div className="mt-auto flex max-h-11 flex-wrap content-start gap-1 overflow-hidden">
            {entry.tags.map((tag) => (
              <TagBadge key={tag} tag={tag} onClick={onTagClick} />
            ))}
          </div>
        )}
      </div>
    </article>
  );
});
