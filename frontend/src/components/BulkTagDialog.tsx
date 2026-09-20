/**
 * 複数ファイルのタグをまとめて編集するダイアログ。
 *
 * 追加と削除を別々に指定できるようにしているのは、選択したファイル間で
 * 既存のタグが揃っていないのが普通で、「置き換え」だけだと使いづらいため。
 */

import { useMemo, useState } from "react";
import { XMarkIcon } from "@heroicons/react/16/solid";
import { addTags, removeTags, type Entry, type TagEditResult } from "../api/taggo";
import { Button } from "./Button";
import { TagBadge } from "./TagBadge";
import { TagEditor } from "./TagEditor";

interface Props {
  entries: Entry[];
  onClose: () => void;
  onApplied: (results: TagEditResult[]) => void;
}

export function BulkTagDialog({ entries, onClose, onApplied }: Props) {
  const [toAdd, setToAdd] = useState<string[]>([]);
  const [toRemove, setToRemove] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [failures, setFailures] = useState<TagEditResult[]>([]);

  const writable = useMemo(() => entries.filter((e) => e.writable), [entries]);
  const readOnly = entries.length - writable.length;

  // 選択中のファイルに付いているタグを、使用件数の多い順に並べる。
  const existingTags = useMemo(() => {
    const counts = new Map<string, { tag: string; count: number }>();
    for (const entry of entries) {
      for (const tag of entry.tags) {
        const key = tag.toLowerCase();
        const found = counts.get(key);
        if (found) {
          found.count += 1;
        } else {
          counts.set(key, { tag, count: 1 });
        }
      }
    }
    return [...counts.values()].sort((a, b) => b.count - a.count || a.tag.localeCompare(b.tag));
  }, [entries]);

  const apply = async () => {
    setBusy(true);
    setFailures([]);
    try {
      const paths = writable.map((e) => e.path);
      const results: TagEditResult[] = [];

      // 先に削除してから追加する。同じタグを両方に指定した場合、追加が勝つ。
      if (toRemove.length > 0) {
        results.push(...(await removeTags(paths, toRemove)));
      }
      if (toAdd.length > 0) {
        results.push(...(await addTags(paths, toAdd)));
      }

      const failed = results.filter((r) => !r.ok);
      onApplied(results);
      if (failed.length > 0) {
        setFailures(failed);
        return;
      }
      onClose();
    } finally {
      setBusy(false);
    }
  };

  const canApply = writable.length > 0 && (toAdd.length > 0 || toRemove.length > 0) && !busy;

  return (
    <div className="fixed inset-0 z-[110] grid place-items-center p-8" role="dialog" aria-modal="true">
      <div className="absolute inset-0 bg-black/40 backdrop-blur-[2px]" onClick={onClose} />

      <div className="relative flex max-h-[88vh] w-160 max-w-full flex-col gap-4.5 overflow-y-auto rounded-xl border border-line bg-surface p-5.5 shadow-pop">
        <header>
          <h2 className="m-0 text-base font-semibold">タグの一括編集</h2>
          <p className="mt-1 mb-0 text-xs text-ink-muted">
            {entries.length} 件を選択中
            {readOnly > 0 && (
              <span className="text-danger">（うち {readOnly} 件は読み取り専用のため対象外）</span>
            )}
          </p>
        </header>

        <section>
          <h3 className="m-0 mb-2 text-xs font-semibold tracking-wide text-ink-muted">
            追加するタグ
          </h3>
          <TagEditor tags={toAdd} onChange={setToAdd} disabled={busy} />
        </section>

        <section>
          <h3 className="m-0 mb-2 text-xs font-semibold tracking-wide text-ink-muted">
            取り除くタグ
          </h3>
          <TagEditor
            tags={toRemove}
            onChange={setToRemove}
            disabled={busy}
            placeholder="外したいタグを入力（Enter で確定）"
          />
          {existingTags.length > 0 && (
            <div className="mt-2.5 flex flex-wrap items-center gap-1.5 text-xs text-ink-faint">
              <span>選択中のファイルに付いているタグ:</span>
              {existingTags.map(({ tag, count }) => (
                <button
                  key={tag}
                  type="button"
                  className="inline-flex items-center gap-1 rounded-full border border-line bg-sunken px-2.5 py-0.5 text-ink-muted enabled:hover:border-danger enabled:hover:text-danger disabled:cursor-not-allowed disabled:opacity-40"
                  disabled={busy || toRemove.includes(tag)}
                  onClick={() => setToRemove((prev) => [...prev, tag])}
                >
                  #{tag}
                  <span className="tabular-nums text-ink-faint">{count}</span>
                </button>
              ))}
            </div>
          )}
        </section>

        {failures.length > 0 && (
          <section className="rounded-lg bg-danger-soft px-3.5 py-3">
            <h3 className="m-0 mb-1.5 text-xs font-semibold text-danger">
              書き込めなかったファイル
            </h3>
            <ul className="m-0 flex list-none flex-col gap-1.5 p-0 text-xs">
              {failures.map((f) => (
                <li key={f.path}>
                  <code className="block truncate text-ink-muted">{f.path}</code>
                  <span>{f.error}</span>
                </li>
              ))}
            </ul>
          </section>
        )}

        <footer className="flex flex-wrap items-center gap-2">
          {toAdd.length > 0 && (
            <span className="inline-flex flex-wrap items-center gap-1 text-xs text-ink-faint">
              追加:
              {toAdd.map((t) => (
                <TagBadge key={t} tag={t} />
              ))}
            </span>
          )}
          <span className="flex-1" />
          <Button onClick={onClose} disabled={busy}>
            <XMarkIcon className="size-4" aria-hidden="true" />
            キャンセル
          </Button>
          <Button variant="primary" onClick={() => void apply()} disabled={!canApply}>
            {busy ? "書き込み中…" : `${writable.length} 件に適用`}
          </Button>
        </footer>
      </div>
    </div>
  );
}
