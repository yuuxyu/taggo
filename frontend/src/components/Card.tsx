/**
 * グリッドに並ぶカード 1 枚。
 * 種別ごとにプレビュー領域の見た目を変え、下部に埋め込みタグをバッジで並べる。
 */

import { memo, useState } from "react";
import { thumbURL, type Entry } from "../api/taggo";
import { TagBadge } from "./TagBadge";
import "./Card.css";

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
    <svg className="card__waveform" viewBox="0 0 224 100" preserveAspectRatio="none" aria-hidden="true">
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

  if (entry.kind === "image") {
    if (failed) {
      return <div className="card__fallback">{describeImageFailure(entry)}</div>;
    }
    return (
      <img
        className="card__image"
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
      <div className="card__audio">
        <WaveformGlyph seed={entry.path} />
        <div className="card__audio-meta">
          {entry.audio?.artist && <span>{entry.audio.artist}</span>}
          {entry.audio?.durationSec ? <span>{formatDuration(entry.audio.durationSec)}</span> : null}
        </div>
      </div>
    );
  }

  return (
    <div className="card__markdown">
      <p className="card__excerpt">{entry.preview || "（本文なし）"}</p>
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
      className={`card${selected ? " is-selected" : ""}${entry.err ? " has-error" : ""}`}
      role="button"
      tabIndex={0}
      aria-label={`${entry.title} を開く`}
      onClick={handleClick}
      onKeyDown={handleKeyDown}
    >
      <button
        className="card__checkbox"
        type="button"
        role="checkbox"
        aria-checked={selected}
        aria-label={selected ? "選択を解除" : "選択に追加"}
        onClick={(e) => {
          e.stopPropagation();
          onToggleSelect(entry);
        }}
      >
        {selected ? "✓" : ""}
      </button>

      <div className="card__preview">
        <CardPreview entry={entry} />
      </div>

      <div className="card__body">
        <h3 className="card__title" title={entry.relPath}>
          {entry.title}
        </h3>
        <div className="card__meta">
          <span className="card__ext">{entry.ext.replace(".", "")}</span>
          {entry.format && entry.format !== entry.ext && (
            <span className="card__format" title={`中身は ${entry.format} 形式です`}>
              実体 {entry.format.replace(".", "")}
            </span>
          )}
          <span>{formatSize(entry.size)}</span>
          {!entry.writable && <span className="card__readonly">読み取り専用</span>}
        </div>

        {entry.err && <p className="card__error">{entry.err}</p>}

        {entry.tags.length > 0 && (
          <div className="card__tags">
            {entry.tags.map((tag) => (
              <TagBadge
                key={tag}
                tag={tag}
                onClick={(t) => {
                  onTagClick(t);
                }}
              />
            ))}
          </div>
        )}
      </div>
    </article>
  );
});
