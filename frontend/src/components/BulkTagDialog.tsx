/**
 * 複数ファイルのタグをまとめて編集するダイアログ。
 *
 * 追加と削除を別々に指定できるようにしているのは、選択したファイル間で
 * 既存のタグが揃っていないのが普通で、「置き換え」だけだと使いづらいため。
 */

import { useMemo, useState } from "react";
import { addTags, removeTags, type Entry, type TagEditResult } from "../api/taggo";
import { TagBadge } from "./TagBadge";
import { TagEditor } from "./TagEditor";
import "./BulkTagDialog.css";

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
    <div className="bulk" role="dialog" aria-modal="true">
      <div className="bulk__backdrop" onClick={onClose} />

      <div className="bulk__panel">
        <header className="bulk__header">
          <h2>タグの一括編集</h2>
          <p>
            {entries.length} 件を選択中
            {readOnly > 0 && (
              <span className="bulk__readonly">（うち {readOnly} 件は読み取り専用のため対象外）</span>
            )}
          </p>
        </header>

        <section className="bulk__section">
          <h3>追加するタグ</h3>
          <TagEditor tags={toAdd} onChange={setToAdd} disabled={busy} />
        </section>

        <section className="bulk__section">
          <h3>取り除くタグ</h3>
          <TagEditor
            tags={toRemove}
            onChange={setToRemove}
            disabled={busy}
            placeholder="外したいタグを入力（Enter で確定）"
          />
          {existingTags.length > 0 && (
            <div className="bulk__existing">
              <span>選択中のファイルに付いているタグ:</span>
              {existingTags.map(({ tag, count }) => (
                <button
                  key={tag}
                  type="button"
                  className="bulk__existing-tag"
                  disabled={busy || toRemove.includes(tag)}
                  onClick={() => setToRemove((prev) => [...prev, tag])}
                >
                  #{tag}
                  <span className="bulk__existing-count">{count}</span>
                </button>
              ))}
            </div>
          )}
        </section>

        {failures.length > 0 && (
          <section className="bulk__failures">
            <h3>書き込めなかったファイル</h3>
            <ul>
              {failures.map((f) => (
                <li key={f.path}>
                  <code>{f.path}</code>
                  <span>{f.error}</span>
                </li>
              ))}
            </ul>
          </section>
        )}

        <footer className="bulk__footer">
          {toAdd.length > 0 && (
            <span className="bulk__summary">
              追加: {toAdd.map((t) => <TagBadge key={t} tag={t} />)}
            </span>
          )}
          <span className="bulk__spacer" />
          <button className="btn" type="button" onClick={onClose} disabled={busy}>
            キャンセル
          </button>
          <button className="btn btn--primary" type="button" onClick={() => void apply()} disabled={!canApply}>
            {busy ? "書き込み中…" : `${writable.length} 件に適用`}
          </button>
        </footer>
      </div>
    </div>
  );
}
