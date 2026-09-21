/**
 * Markdown プレビューの右側に並べる関連ページ。
 *
 * そのノートがリンクしているページと、そのノートへリンクしているページを
 * それぞれカードで並べる。リンクとして扱うのは、.md を指す Markdown の
 * リンクだけで、行き先はそのノートの位置を基準に決まる。
 *
 * 行き先がまだ存在しないリンクも、書きかけのメモでは珍しくないため、
 * 「まだ無いノート」として控えめに出す。
 */

import { useEffect, useState } from "react";
import { ArrowUpRightIcon, ArrowUturnLeftIcon } from "@heroicons/react/16/solid";
import { getRelatedPages, type Entry, type Related, type RelatedPage } from "../api/taggo";
import { TagBadge } from "./TagBadge";

interface Props {
  related: Related;
  /** カードを開く。行き先が無いリンクでは path が空になる。 */
  onOpen: (page: RelatedPage) => void;
  onTagClick: (tag: string) => void;
}

/**
 * 関連ページを読み込む。Markdown 以外では何も読まない。
 * 一覧が入れ替わってもリンク関係は変わらないので、開いているノートだけを見る。
 */
export function useRelatedPages(entry: Entry): Related | null {
  const [related, setRelated] = useState<Related | null>(null);

  useEffect(() => {
    if (entry.kind !== "markdown" || entry.cloudOnly) {
      setRelated(null);
      return;
    }
    let cancelled = false;
    setRelated(null);
    void getRelatedPages(entry.path).then((got) => {
      if (!cancelled) setRelated(got);
    });
    return () => {
      cancelled = true;
    };
    // タグを書き換えるとリンクの索引も張り直されるため、更新日時も見る。
  }, [entry.kind, entry.cloudOnly, entry.path, entry.modTime]);

  return related;
}

/** 関連ページが 1 件でもあるか。無いときはサイドを出さず 1 カラムに戻す。 */
export function hasRelatedPages(related: Related | null): boolean {
  return related !== null && related.outgoing.length + related.incoming.length > 0;
}

function RelatedCard({ page, onOpen, onTagClick }: { page: RelatedPage } & Omit<Props, "related">) {
  // 行き先が無いリンクは、Go 側で path を省いて返す。
  const missing = !page.path;

  return (
    <li
      className={`rounded-lg border bg-surface transition focus-within:ring-3 focus-within:ring-accent-soft ${
        missing ? "border-dashed border-line text-ink-faint" : "border-line hover:border-accent hover:shadow-card"
      }`}
    >
      <button
        type="button"
        className="flex w-full flex-col items-start gap-1 px-3 py-2.5 text-left focus-visible:outline-hidden"
        title={missing ? `「${page.target}」はまだ見つかりません` : page.relPath}
        onClick={() => onOpen(page)}
      >
        <span className="line-clamp-2 text-sm leading-snug font-semibold break-words">
          {page.title}
        </span>
        {missing ? (
          <span className="text-xs">まだ無いノート</span>
        ) : (
          <>
            <span className="w-full truncate text-xs text-ink-faint">{page.relPath}</span>
            {page.preview && (
              <span className="line-clamp-2 text-xs leading-relaxed text-ink-muted">
                {page.preview}
              </span>
            )}
          </>
        )}
      </button>
      {page.tags && page.tags.length > 0 && (
        <div className="flex flex-wrap gap-1 px-3 pb-2.5">
          {page.tags.slice(0, 4).map((tag) => (
            <TagBadge key={tag} tag={tag} onClick={onTagClick} />
          ))}
        </div>
      )}
    </li>
  );
}

function Section({
  title,
  icon,
  pages,
  onOpen,
  onTagClick,
}: {
  title: string;
  icon: React.ReactNode;
  pages: RelatedPage[];
} & Omit<Props, "related">) {
  if (pages.length === 0) return null;

  return (
    <section>
      <h3 className="m-0 mb-2 flex items-center gap-1.5 text-xs font-semibold tracking-wide text-ink-muted">
        {icon}
        {title}
        <span className="tabular-nums text-ink-faint">{pages.length}</span>
      </h3>
      <ul className="m-0 flex list-none flex-col gap-2 p-0">
        {pages.map((page) => (
          <RelatedCard
            key={`${page.path}\u0000${page.target}`}
            page={page}
            onOpen={onOpen}
            onTagClick={onTagClick}
          />
        ))}
      </ul>
    </section>
  );
}

export function RelatedPages({ related, onOpen, onTagClick }: Props) {
  return (
    <aside className="flex flex-col gap-5" aria-label="関連ページ">
      <Section
        title="このページからリンク"
        icon={<ArrowUpRightIcon className="size-3.5" aria-hidden="true" />}
        pages={related.outgoing}
        onOpen={onOpen}
        onTagClick={onTagClick}
      />
      <Section
        title="このページへのリンク"
        icon={<ArrowUturnLeftIcon className="size-3.5" aria-hidden="true" />}
        pages={related.incoming}
        onOpen={onOpen}
        onTagClick={onTagClick}
      />
    </aside>
  );
}
