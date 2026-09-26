/**
 * グリッドに並ぶノートのカード 1 枚。
 * 上部に本文の抜粋を、その下にタイトルとタグ（本文の [[タグ]]）のバッジを並べる。
 * タグ欄に収まらなかったタグは「+N」で数を示し、タグ欄に乗ると全部を重ねて見せる。
 * 右上のピンで、一覧の先頭にピン留めする。
 */

import { memo, useLayoutEffect, useRef, useState } from "react";
import { MapPinIcon } from "@heroicons/react/16/solid";
import type { Entry } from "../api/taggo";
import { NotePreview } from "./NotePreview";
import { TagBadge } from "./TagBadge";

interface Props {
  entry: Entry;
  /** 一覧の先頭にピン留めしているか。 */
  pinned: boolean;
  onOpen: (entry: Entry) => void;
  onTagClick: (tag: string) => void;
  onTogglePin: (entry: Entry, pinned: boolean) => void;
}

/**
 * カード下部のタグ欄。高さは 4 行ぶんに決めてあり、収まるだけのタグを並べる。
 *
 * 何個収まるかはタグの長さとカードの幅で変わるので、描画してから実際にあふれているかを
 * 測り、あふれていれば 1 個ずつ減らして「+N」を添える。useLayoutEffect の中で
 * 描き直すので、画面に出る前に収まる数へ落ち着き、ちらつかない。
 *
 * 収まらなかったタグがあるときは、タグ欄に乗る（またはキーボードで「+N」に移る）と、
 * 全部のタグをカードの下側に重ねて見せる。
 */
function CardTags({ tags, onTagClick }: { tags: string[]; onTagClick: (tag: string) => void }) {
  const listRef = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(0);
  const [expanded, setExpanded] = useState(false);

  // 幅が変わると収まる数も変わるので、測り直しのきっかけにする。
  useLayoutEffect(() => {
    const el = listRef.current;
    if (!el) return;
    const observer = new ResizeObserver(([e]) => setWidth(Math.round(e.contentRect.width)));
    observer.observe(el);
    return () => observer.disconnect();
  }, []);

  // 収まる数は「どのタグを・どの幅で並べたか」ごとに測る。タグか幅が変われば、
  // いったん全部を並べるところからやり直す。
  const key = `${width}\u0000${tags.join("\u0000")}`;
  const [fit, setFit] = useState({ key, limit: tags.length });
  const limit = fit.key === key ? fit.limit : tags.length;
  const hidden = tags.length - limit;

  useLayoutEffect(() => {
    const el = listRef.current;
    if (!el || limit === 0 || el.scrollHeight <= el.clientHeight) return;
    // 欄の下端からはみ出していないバッジを数え、そこまでに一気に詰める。
    // 「+N」を足すと 1 個ぶん押し出されることがあるので、その分は次の測定で 1 個ずつ減らす。
    const bottom = el.getBoundingClientRect().bottom;
    let inside = 0;
    for (const child of Array.from(el.children).slice(0, limit)) {
      if (child.getBoundingClientRect().bottom > bottom) break;
      inside += 1;
    }
    setFit({ key, limit: Math.min(inside, limit - 1) });
  });

  // 隠れているタグが無くなったら、重ねて見せる必要も無い。
  const showAll = expanded && hidden > 0;

  return (
    <>
      <div
        ref={listRef}
        className="mt-auto flex max-h-21 flex-wrap content-start gap-1 overflow-hidden"
        onMouseEnter={() => setExpanded(true)}
      >
        {tags.slice(0, limit).map((tag) => (
          <TagBadge key={tag} tag={tag} onClick={onTagClick} />
        ))}
        {hidden > 0 && (
          <button
            type="button"
            className="rounded-full bg-sunken px-2 py-px text-xs tabular-nums text-ink-muted hover:text-ink"
            aria-label={`ほかに ${hidden} 個のタグ`}
            aria-expanded={showAll}
            title={tags.slice(limit).map((t) => `#${t}`).join(" ")}
            onFocus={() => setExpanded(true)}
            onClick={(e) => {
              e.stopPropagation();
              setExpanded((v) => !v);
            }}
          >
            +{hidden}
          </button>
        )}
      </div>

      {showAll && (
        <div
          className="absolute inset-x-0 bottom-0 z-3 flex max-h-[70%] flex-wrap content-start gap-1 overflow-y-auto border-t border-line bg-surface px-3 pt-2.5 pb-3 shadow-card"
          onMouseLeave={() => setExpanded(false)}
          onBlur={(e) => {
            if (!e.currentTarget.contains(e.relatedTarget as Node | null)) setExpanded(false);
          }}
          // 重ねた欄の余白をクリックしても、カードは開かない。
          onClick={(e) => e.stopPropagation()}
        >
          {tags.map((tag) => (
            <TagBadge key={tag} tag={tag} onClick={onTagClick} />
          ))}
        </div>
      )}
    </>
  );
}

export const Card = memo(function Card({
  entry,
  pinned,
  onOpen,
  onTagClick,
  onTogglePin,
}: Props) {
  // カードはクリックで開くだけでなく、キーボードだけでも到達・操作できるようにする。
  const handleKeyDown = (event: React.KeyboardEvent) => {
    if (event.key !== "Enter" && event.key !== " ") return;
    event.preventDefault();
    onOpen(entry);
  };

  return (
    <article
      className={`group relative flex h-full cursor-pointer flex-col overflow-hidden rounded-lg border bg-surface transition hover:-translate-y-px hover:shadow-card focus-visible:outline-hidden focus-visible:ring-3 focus-visible:ring-accent-soft ${
        entry.err ? "border-danger hover:border-line-strong" : "border-line hover:border-line-strong"
      }`}
      role="button"
      tabIndex={0}
      aria-label={`${entry.title} を開く`}
      onClick={() => onOpen(entry)}
      onKeyDown={handleKeyDown}
    >
      {/* ピン留めしているカードでは常に出し、それ以外はカードに乗ったときだけ出す。 */}
      <button
        type="button"
        aria-pressed={pinned}
        aria-label={pinned ? "ピン留めを外す" : "一覧の先頭にピン留め"}
        title={pinned ? "ピン留めを外す" : "一覧の先頭にピン留め"}
        className={`absolute top-2 right-2 z-2 grid size-6 place-items-center rounded-md transition group-hover:opacity-100 focus-visible:opacity-100 ${
          pinned ? "bg-accent text-white opacity-100" : "bg-black/35 text-white opacity-0"
        }`}
        onClick={(e) => {
          e.stopPropagation();
          onTogglePin(entry, !pinned);
        }}
      >
        <MapPinIcon className="size-3.5" />
      </button>

      <div className="relative h-35 shrink-0 overflow-hidden border-b border-line bg-sunken">
        <NotePreview note={entry} />
      </div>

      <div className="flex min-h-0 flex-1 flex-col gap-1.5 px-3 pt-2.5 pb-3">
        <h3 className="m-0 line-clamp-2 text-sm leading-snug font-semibold" title={entry.relPath}>
          {entry.title}
        </h3>

        {entry.err && <p className="m-0 line-clamp-2 text-xs text-danger">{entry.err}</p>}

        {entry.tags.length > 0 && <CardTags tags={entry.tags} onTagClick={onTagClick} />}
      </div>
    </article>
  );
});
