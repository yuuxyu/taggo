/**
 * 検索バーの下に置くツールバー。
 * フォルダの選択、並び順の切り替え、読み込み状況の表示、一括編集の操作をまとめる。
 */

import { CheckIcon, FolderOpenIcon, TagIcon, XMarkIcon } from "@heroicons/react/20/solid";
import type { ScanProgress, SortOrder, Status } from "../api/taggo";
import { Button } from "./Button";

interface Props {
  status: Status | null;
  progress: ScanProgress | null;
  sort: SortOrder;
  onSortChange: (next: SortOrder) => void;
  onChooseFolder: () => void;
  /** 選択中のファイル数。0 なら一括編集バーは出さない。 */
  selectedCount: number;
  onClearSelection: () => void;
  onOpenBulkEditor: () => void;
  onSelectAll: () => void;
}

const SORT_LABELS: Record<SortOrder, string> = {
  modified_desc: "更新が新しい順",
  name_asc: "名前順",
  relevance: "関連度順",
};

/** パスを画面幅に収まる長さへ省略する。先頭ではなく中間を省く。 */
function shortenPath(path: string, max = 56): string {
  if (path.length <= max) return path;
  const head = Math.ceil((max - 1) / 2);
  const tail = Math.floor((max - 1) / 2);
  return `${path.slice(0, head)}…${path.slice(path.length - tail)}`;
}

export function Toolbar({
  status,
  progress,
  sort,
  onSortChange,
  onChooseFolder,
  selectedCount,
  onClearSelection,
  onOpenBulkEditor,
  onSelectAll,
}: Props) {
  const root = status?.root ?? "";

  return (
    <div className="flex flex-wrap items-center gap-2.5 text-sm">
      <Button onClick={onChooseFolder}>
        <FolderOpenIcon className="size-4" aria-hidden="true" />
        フォルダを選択
      </Button>

      {root !== "" && (
        <span className="truncate text-ink-muted" title={root}>
          {shortenPath(root)}
        </span>
      )}

      {progress && (
        <span className="rounded-full bg-accent-soft px-2.5 py-0.5 text-xs tabular-nums text-accent-ink">
          読み込み中 {progress.done.toLocaleString()} / {progress.found.toLocaleString()}
        </span>
      )}

      <span className="flex-1" />

      {selectedCount > 0 ? (
        <span className="flex items-center gap-2 text-ink-muted">
          <strong className="tabular-nums text-ink">{selectedCount}</strong> 件を選択中
          <Button variant="primary" onClick={onOpenBulkEditor}>
            <TagIcon className="size-4" aria-hidden="true" />
            タグを一括編集
          </Button>
          <Button variant="ghost" onClick={onClearSelection}>
            <XMarkIcon className="size-4" aria-hidden="true" />
            選択を解除
          </Button>
        </span>
      ) : (
        <Button variant="ghost" onClick={onSelectAll}>
          <CheckIcon className="size-4" aria-hidden="true" />
          表示中をすべて選択
        </Button>
      )}

      <label className="inline-flex items-center gap-1.5 text-ink-muted">
        並び順
        <select
          className="rounded-md border border-line bg-surface px-2 py-1 text-sm text-ink"
          value={sort}
          onChange={(e) => onSortChange(e.target.value as SortOrder)}
        >
          {(Object.keys(SORT_LABELS) as SortOrder[]).map((key) => (
            <option key={key} value={key}>
              {SORT_LABELS[key]}
            </option>
          ))}
        </select>
      </label>
    </div>
  );
}
