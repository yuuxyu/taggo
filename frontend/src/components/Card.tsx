/**
 * グリッドに並ぶカード 1 枚。
 * 種別ごとにプレビュー領域の見た目を変え、下部に埋め込みタグをバッジで並べる。
 */

import { memo, useState } from "react";
import { BookOpenIcon, CheckIcon, CloudIcon, LockClosedIcon } from "@heroicons/react/16/solid";
import { thumbURL, type Entry } from "../api/taggo";
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

/** 再生時間を m:ss 形式にする。 */
function formatDuration(seconds: number): string {
  const total = Math.round(seconds);
  const m = Math.floor(total / 60);
  const s = total % 60;
  return `${m}:${String(s).padStart(2, "0")}`;
}

/** 音声カードに出す簡易な波形イメージ。ファイルパスから決定的に形を作る。 */
function WaveformGlyph({ seed }: { seed: string }) {
  // 実際の波形はデコードしないと分からないため、カードでは擬似的な形を出し、
  // 実波形は詳細プレビューのプレイヤーで描く。
  let hash = 0;
  for (let i = 0; i < seed.length; i += 1) {
    hash = (hash * 31 + seed.charCodeAt(i)) >>> 0;
  }
  const bars = Array.from({ length: 28 }, (_, i) => {
    hash = (hash * 1103515245 + 12345) >>> 0;
    const height = 18 + ((hash >>> 8) % 64);
    return <rect key={i} x={i * 8} y={(100 - height) / 2} width={4} height={height} rx={2} />;
  });
  return (
    <svg
      className="h-16 w-full fill-accent opacity-55"
      viewBox="0 0 224 100"
      preserveAspectRatio="none"
      aria-hidden="true"
    >
      {bars}
    </svg>
  );
}

/**
 * 表示できない画像について、分かっている理由を返す。
 * 走査時のメタデータ読み取りで理由が判明していればそれを使い、
 * 判明していない場合でも、拡張子と中身が食い違っていることは伝える。
 */
function describeImageFailure(entry: Entry): string {
  if (entry.err) return entry.err;
  if (entry.format && entry.format !== entry.ext) {
    const actual = entry.format.replace(".", "").toUpperCase();
    return `中身は ${actual} 形式のため表示できません`;
  }
  return "画像を表示できません";
}

function CardPreview({ entry }: { entry: Entry }) {
  const [failed, setFailed] = useState(false);

  // 中身がクラウド上にしか無いファイルは、サムネイルを要求した時点で
  // ダウンロードが始まる。取り込むまでは取りに行かない。
  if (entry.cloudOnly) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-1.5 p-3 text-center text-xs text-ink-faint">
        <CloudIcon className="size-6" aria-hidden="true" />
        クラウド上のみ
      </div>
    );
  }

  if (entry.kind === "image") {
    if (failed) {
      return (
        <div className="grid h-full place-items-center p-3 text-center text-xs text-ink-faint">
          {describeImageFailure(entry)}
        </div>
      );
    }
    return (
      <img
        className="block size-full object-cover"
        src={thumbURL(entry.path)}
        alt={entry.title}
        loading="lazy"
        draggable={false}
        onError={() => setFailed(true)}
      />
    );
  }

  if (entry.kind === "audio") {
    return (
      <div className="flex h-full flex-col justify-center px-3.5 py-3">
        <WaveformGlyph seed={entry.path} />
        <div className="mt-2 flex justify-between gap-2.5 overflow-hidden text-xs whitespace-nowrap text-ink-faint">
          {entry.audio?.artist && <span className="truncate">{entry.audio.artist}</span>}
          {entry.audio?.durationSec ? (
            <span className="tabular-nums">{formatDuration(entry.audio.durationSec)}</span>
          ) : null}
        </div>
      </div>
    );
  }

  return (
    <div className="h-full overflow-hidden px-3.5 py-3">
      <p className="m-0 line-clamp-6 text-xs leading-relaxed text-ink-muted">
        {entry.preview || "（本文なし）"}
      </p>
    </div>
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
          {entry.format && entry.format !== entry.ext && (
            <span
              className="rounded-sm bg-accent-soft px-1.5 uppercase text-accent-ink"
              title={`中身は ${entry.format} 形式です`}
            >
              実体 {entry.format.replace(".", "")}
            </span>
          )}
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
