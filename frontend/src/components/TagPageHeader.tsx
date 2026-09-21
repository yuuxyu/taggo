/**
 * タグで検索しているときに、グリッドの上へ出すタグページの見出し。
 *
 * タグページは Front Matter の `tag:` でそのタグを説明していると宣言したノートで、
 * 一覧のカードには混ぜず、ここに別枠で出す。何のタグで絞っているのかと、
 * そのタグが何を表すのかを、一覧を見る前に読めるようにするため。
 *
 * 1 つのタグにタグページは 1 つのはずなので、複数のノートが同じタグを宣言していたら
 * 警告を添えて全部を並べる。どれが正しいかは taggo には決められないので、
 * 利用者に直してもらう。
 */

import { BookOpenIcon, ExclamationTriangleIcon } from "@heroicons/react/16/solid";
import type { Entry, TagPageGroup } from "../api/taggo";

interface Props {
  groups: TagPageGroup[];
  onOpen: (entry: Entry) => void;
}

function PageButton({ page, compact, onOpen }: { page: Entry; compact: boolean; onOpen: Props["onOpen"] }) {
  return (
    <button
      type="button"
      className="flex min-w-0 flex-1 flex-col items-start gap-0.5 rounded-md px-3 py-2 text-left transition hover:bg-sunken focus-visible:ring-3 focus-visible:ring-accent-soft focus-visible:outline-hidden"
      title={page.relPath}
      onClick={() => onOpen(page)}
    >
      <span className="line-clamp-1 text-sm font-semibold break-words text-ink">{page.title}</span>
      {!compact && page.preview && (
        <span className="line-clamp-2 text-xs leading-relaxed text-ink-muted">{page.preview}</span>
      )}
      <span className="w-full truncate text-xs text-ink-faint">{page.relPath}</span>
    </button>
  );
}

function Heading({ group, onOpen }: { group: TagPageGroup; onOpen: Props["onOpen"] }) {
  const duplicated = group.pages.length > 1;

  return (
    <section
      className={`flex min-w-0 flex-col rounded-lg border bg-surface py-1 ${
        duplicated ? "border-danger" : "border-line"
      }`}
      aria-label={`#${group.tag} のタグページ`}
    >
      <p className="m-0 flex items-center gap-1.5 px-3 pt-1.5 text-xs font-semibold text-accent-ink">
        <BookOpenIcon className="size-3.5" aria-hidden="true" />
        <span className="truncate">#{group.tag} のタグページ</span>
      </p>
      {duplicated && (
        <p className="m-0 flex items-start gap-1.5 px-3 pt-1 text-xs text-danger">
          <ExclamationTriangleIcon className="mt-px size-3.5 shrink-0" aria-hidden="true" />
          <span>
            {group.pages.length} つのファイルが #{group.tag} をタグページとして宣言しています。
            どれか 1 つに絞ってください。
          </span>
        </p>
      )}
      <div className={duplicated ? "flex flex-wrap" : "flex"}>
        {group.pages.map((page) => (
          <PageButton key={page.path} page={page} compact={duplicated} onOpen={onOpen} />
        ))}
      </div>
    </section>
  );
}

export function TagPageHeader({ groups, onOpen }: Props) {
  if (groups.length === 0) return null;
  return (
    <div className="grid grid-cols-[repeat(auto-fit,minmax(18rem,1fr))] gap-2.5 px-4.5 pt-3">
      {groups.map((group) => (
        <Heading key={group.tag.toLowerCase()} group={group} onOpen={onOpen} />
      ))}
    </div>
  );
}
