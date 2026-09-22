/**
 * Markdown プレビューの右側に並べる関連ページ。
 *
 * そのノートがリンクしているページと、そのノートへリンクしているページを
 * それぞれカードで並べる。リンクとして扱うのは、.md を指す Markdown の
 * リンクだけで、行き先はそのノートの位置を基準に決まる。
 *
 * 行き先がまだ存在しないリンクも、書きかけのメモでは珍しくないため、
 * 「まだ無いノート」として控えめに出す。
 *
 * リンクの無いノートでも右の列が空にならないよう、タグが重なるノートも数件並べる。
 *
 * タグページ（Front Matter の `tag:` でタグを説明していると宣言したノート）でも
 * 並べる欄は通常のノートと同じ。同じタグを宣言しているほかのノートがあれば、
 * 先頭に警告として出す。
 */

import { useEffect, useState } from "react";
import {
  ArrowUpRightIcon,
  ArrowUturnLeftIcon,
  BookOpenIcon,
  ExclamationTriangleIcon,
  MusicalNoteIcon,
  TagIcon,
} from "@heroicons/react/16/solid";
import {
  getRelatedPages,
  thumbURL,
  type Entry,
  type Related,
  type RelatedPage,
} from "../api/taggo";
import { TagBadge } from "./TagBadge";

interface CardProps {
  /** カードを開く。行き先が無いリンクでは path が空になる。 */
  onOpen: (page: RelatedPage) => void;
  onTagClick: (tag: string) => void;
}

interface Props extends CardProps {
  related: Related;
  /** 開いているノートがタグページなら、説明しているタグ。 */
  tagPage?: string;
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

function RelatedCard({ page, onOpen, onTagClick }: { page: RelatedPage } & CardProps) {
  // 行き先が無いリンクは、Go 側で path を省いて返す。
  const missing = !page.path;
  // タグページには画像も並ぶので、小さなサムネイルを添える。
  // 中身がクラウド上にしか無い画像は、取りに行くとダウンロードが始まるので出さない。
  const thumbnail = page.kind === "image" && !page.cloudOnly ? page.path : undefined;

  return (
    <li
      className={`rounded-lg border bg-surface transition focus-within:ring-3 focus-within:ring-accent-soft ${
        missing ? "border-dashed border-line text-ink-faint" : "border-line hover:border-accent hover:shadow-card"
      }`}
    >
      <button
        type="button"
        className="flex w-full flex-col overflow-hidden rounded-lg text-left focus-visible:outline-hidden"
        title={missing ? `「${page.target}」はまだ見つかりません` : page.relPath}
        onClick={() => onOpen(page)}
      >
        {thumbnail && (
          <img
            className="block h-24 w-full border-b border-line bg-sunken object-cover"
            src={thumbURL(thumbnail, 240)}
            alt=""
            loading="lazy"
            draggable={false}
          />
        )}
        <span className="flex w-full flex-col items-start gap-1 px-3 py-2.5">
          <span className="flex max-w-full items-start gap-1 text-sm leading-snug font-semibold break-words">
            {page.kind === "audio" && (
              <MusicalNoteIcon className="mt-0.5 size-3.5 shrink-0 text-ink-faint" aria-hidden="true" />
            )}
            {page.tagPage && (
              <BookOpenIcon className="mt-0.5 size-3.5 shrink-0 text-accent-ink" aria-label="タグページ" />
            )}
            <span className="line-clamp-2">{page.title}</span>
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
        </span>
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
  emptyText = "まだありません",
  onOpen,
  onTagClick,
}: {
  title: string;
  icon: React.ReactNode;
  pages: RelatedPage[];
  /** 1 件も無いときに出す文言。 */
  emptyText?: string;
} & CardProps) {
  return (
    <section>
      <h3 className="m-0 mb-2 flex items-center gap-1.5 text-xs font-semibold tracking-wide text-ink-muted">
        {icon}
        <span className="min-w-0 truncate">{title}</span>
        <span className="tabular-nums text-ink-faint">{pages.length}</span>
      </h3>
      {/* 右の列は常に出すので、リンクが無くても見出しは残し、無いことを控えめに示す。 */}
      {pages.length === 0 ? (
        <p className="m-0 text-xs text-ink-faint">{emptyText}</p>
      ) : (
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
      )}
    </section>
  );
}

export function RelatedPages({ related, tagPage, onOpen, onTagClick }: Props) {
  // 古いバックエンドの応答でも落ちないよう、欠けていれば空として扱う。
  const duplicates = related.duplicates ?? [];

  return (
    <div className="flex flex-col gap-5">
      {tagPage && duplicates.length > 0 && (
        <section className="rounded-lg bg-danger-soft px-3 py-2.5 text-xs text-danger">
          <p className="m-0 mb-2 flex items-start gap-1.5">
            <ExclamationTriangleIcon className="mt-px size-3.5 shrink-0" aria-hidden="true" />
            <span>
              ほかにも #{tagPage} をタグページとして宣言しているファイルがあります。
              どれか 1 つに絞ってください。
            </span>
          </p>
          <ul className="m-0 flex list-none flex-col gap-2 p-0">
            {duplicates.map((page) => (
              <RelatedCard key={page.path} page={page} onOpen={onOpen} onTagClick={onTagClick} />
            ))}
          </ul>
        </section>
      )}
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
      <Section
        title="同じタグのノート"
        icon={<TagIcon className="size-3.5" aria-hidden="true" />}
        pages={related.sameTag}
        emptyText="タグが重なるノートはありません"
        onOpen={onOpen}
        onTagClick={onTagClick}
      />
    </div>
  );
}
