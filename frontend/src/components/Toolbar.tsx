/**
 * 検索バーの下に置くツールバー。
 * フォルダの選択、並び順の切り替え、読み込み状況の表示、一括編集の操作をまとめる。
 */

import type { SortOrder, Status } from "../api/taggo";
import type { ScanProgress } from "../api/taggo";
import "./Toolbar.css";

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
    <div className="toolbar">
      <button className="btn" type="button" onClick={onChooseFolder}>
        <span aria-hidden="true">🗀</span>
        フォルダを選択
      </button>

      {root !== "" && (
        <span className="toolbar__root" title={root}>
          {shortenPath(root)}
        </span>
      )}

      {progress && (
        <span className="toolbar__progress">
          読み込み中 {progress.done.toLocaleString()} / {progress.found.toLocaleString()}
        </span>
      )}

      <span className="toolbar__spacer" />

      {selectedCount > 0 ? (
        <span className="toolbar__selection">
          <strong>{selectedCount}</strong> 件を選択中
          <button className="btn btn--primary" type="button" onClick={onOpenBulkEditor}>
            タグを一括編集
          </button>
          <button className="btn btn--ghost" type="button" onClick={onClearSelection}>
            選択を解除
          </button>
        </span>
      ) : (
        <button className="btn btn--ghost" type="button" onClick={onSelectAll}>
          表示中をすべて選択
        </button>
      )}

      <label className="toolbar__sort">
        並び順
        <select value={sort} onChange={(e) => onSortChange(e.target.value as SortOrder)}>
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
