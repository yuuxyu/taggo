/**
 * Markdown プレビューの本文の下に並べる関連ページ。
 *
 * タグごとに「そのタグのページと、そのタグを持つノート」を 1 つのグループにして、
 * カードのグリッドで並べる。タグは本文の [[タグ]] で付けたもの。
 * グループの先頭のカードがタグのページで、その後ろに、開いているノートと同じタグを
 * 多く持つノートほど先に並ぶ。前のグループに出したノートは後のグループには出さない。
 *
 * 一番上のグループは、開いているノートを指しているノート（リンク元）で、開いているノートが
 * 表すタグを持つノートと、Markdown のリンクでリンクしているノートの両方が入る。
 * 最後に、開いているノートが Markdown のリンクでリンクしているノート（リンク先）のグループを置く。
 *
 * 行き先がまだ存在しないもの（ページの無いタグや、行き先の無いリンク）も、
 * 書きかけのメモでは珍しくないため、「まだ無いノート」として控えめに出す。
 * クリックすると、ファイルは作らずに、まだ無いページとして開く。
 */

import { useEffect, useState } from "react";
import { LinkIcon } from "@heroicons/react/16/solid";
import {
  Events,
  getRelatedPages,
  on,
  type Entry,
  type EntryChanged,
  type Related,
  type RelatedGroup,
  type RelatedPage,
} from "../api/taggo";
import { NotePreview } from "./NotePreview";

interface CardProps {
  /** カードを開く。行き先がまだ無いものでは path が空になる。 */
  onOpen: (page: RelatedPage) => void;
}

interface GroupProps extends CardProps {
  /** タグで検索し直す。グループの見出しと「ほか N 件」から使う。 */
  onTagClick: (tag: string) => void;
}

interface Props extends GroupProps {
  related: Related;
}

/**
 * 関連ページを読み込む。中身がクラウド上にしか無いノートでは何も読まない。
 * 一覧が入れ替わってもリンク関係は変わらないので、開いているノートだけを見る。
 */
export function useRelatedPages(entry: Entry): Related | null {
  const [related, setRelated] = useState<Related | null>(null);

  useEffect(() => {
    if (entry.cloudOnly) {
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
  }, [entry.cloudOnly, entry.path, entry.modTime]);

  // まだ無いノートへのリンクやタグがあるうちは、ほかのファイルが増えるたびに読み直す。
  // リンクから作ったノートを、開き直さなくても「ある」側へ移すため。
  const hasMissing = (related?.missingLinks?.length ?? 0) + (related?.missingTags?.length ?? 0) > 0;
  useEffect(() => {
    if (!hasMissing) return;
    let cancelled = false;
    const off = on<EntryChanged>(Events.entryChanged, (change) => {
      if (change.removed || change.path === entry.path) return;
      void getRelatedPages(entry.path).then((got) => {
        if (!cancelled) setRelated(got);
      });
    });
    return () => {
      cancelled = true;
      off();
    };
  }, [hasMissing, entry.path]);

  return related;
}

/**
 * 行き先がまだ無いリンクを、本文のリンクと突き合わせられる形で集める。
 * 大文字小文字は、Windows に合わせて区別しない。
 */
export function missingLinksOf(related: Related | null): ReadonlySet<string> {
  return new Set((related?.missingLinks ?? []).map((link) => link.toLowerCase()));
}

/** ページがまだ無いタグを、本文の [[タグ]] と突き合わせられる形（小文字）で集める。 */
export function missingTagsOf(related: Related | null): ReadonlySet<string> {
  return new Set((related?.missingTags ?? []).map((tag) => tag.toLowerCase()));
}

/** グリッドのカード 1 枚。head はグループの先頭に置くタグのページ。 */
function RelatedCard({ page, head, onOpen }: { page: RelatedPage; head?: boolean } & CardProps) {
  // 行き先がまだ無いものは、Go 側で path を省いて返す。
  const path = page.path;
  const name = page.target || page.tag || page.title;

  return (
    <li className="min-w-0">
      <button
        type="button"
        className={`flex h-44 w-full flex-col overflow-hidden rounded-lg border text-left transition focus-visible:ring-3 focus-visible:ring-accent-soft focus-visible:outline-hidden ${
          !path
            ? "border-dashed border-line bg-surface text-ink-faint hover:border-line-strong"
            : head
              ? "border-accent bg-surface hover:shadow-card"
              : "border-line bg-surface hover:border-line-strong hover:shadow-card"
        }`}
        title={
          path
            ? page.relPath
            : `「${name}」はまだありません。クリックで開くと、md ファイルを作成できます`
        }
        onClick={() => onOpen(page)}
      >
        <span className="relative block min-h-0 flex-1 overflow-hidden border-b border-line bg-sunken">
          {path ? (
            <NotePreview note={{ ...page, path, relPath: page.relPath ?? "" }} lines={3} />
          ) : (
            <span className="grid h-full place-items-center text-xs">まだ無いノート</span>
          )}
        </span>
        <span
          className={`line-clamp-2 shrink-0 px-2.5 py-2 text-xs leading-snug font-semibold break-words ${
            path ? "text-ink" : ""
          }`}
        >
          {page.title}
        </span>
      </button>
    </li>
  );
}

function Group({ group, onOpen, onTagClick }: { group: RelatedGroup } & GroupProps) {
  const tag = group.tag;
  const count = (group.page ? 1 : 0) + group.pages.length + group.more;

  return (
    <section aria-label={tag ? `#${tag} のノート` : "このノートからのリンク先"}>
      <h3 className="m-0 mb-2.5 flex items-center gap-1.5 text-sm font-semibold text-ink-muted">
        {tag ? (
          <button
            type="button"
            className="truncate text-accent-ink hover:underline"
            title={`「${tag}」で検索`}
            onClick={() => onTagClick(tag)}
          >
            #{tag}
          </button>
        ) : (
          <>
            <LinkIcon className="size-3.5" aria-hidden="true" />
            <span>リンク先</span>
          </>
        )}
        <span className="text-xs font-normal tabular-nums text-ink-faint">{count}</span>
      </h3>
      <ul className="m-0 grid list-none grid-cols-[repeat(auto-fill,minmax(10rem,1fr))] gap-3 p-0">
        {group.page && <RelatedCard page={group.page} head onOpen={onOpen} />}
        {group.pages.map((page) => (
          <RelatedCard key={`${page.path}\u0000${page.target}`} page={page} onOpen={onOpen} />
        ))}
        {group.more > 0 && (
          <li className="min-w-0">
            {tag ? (
              <button
                type="button"
                className="grid h-44 w-full place-items-center rounded-lg border border-line bg-sunken px-3 text-center text-xs text-ink-muted transition hover:border-line-strong hover:text-ink"
                title={`「${tag}」で検索して、すべてを一覧で見る`}
                onClick={() => onTagClick(tag)}
              >
                ほか {group.more.toLocaleString()} 件
              </button>
            ) : (
              <span className="grid h-44 place-items-center rounded-lg border border-line bg-sunken text-xs text-ink-muted">
                ほか {group.more.toLocaleString()} 件
              </span>
            )}
          </li>
        )}
      </ul>
    </section>
  );
}

export function RelatedPages({ related, onOpen, onTagClick }: Props) {
  // 古いバックエンドの応答でも落ちないよう、欠けていれば空として扱う。
  const groups = related.groups ?? [];
  if (groups.length === 0) {
    return <p className="m-0 text-xs text-ink-faint">タグやリンクでつながるノートはまだありません</p>;
  }
  return (
    <div className="flex flex-col gap-8">
      {groups.map((group) => (
        <Group key={group.tag ?? ""} group={group} onOpen={onOpen} onTagClick={onTagClick} />
      ))}
    </div>
  );
}
