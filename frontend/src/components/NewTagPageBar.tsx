/**
 * 検索語と同じ名前のタグのページがまだ無いときに、検索結果の上へ出す帯。
 *
 * 検索語と同じ名前のページがあれば結果の先頭に出るので、出ないときは
 * そのページがまだ無いということ。その場で md ファイルを作れるようにする。
 */

import { useState } from "react";
import { DocumentPlusIcon } from "@heroicons/react/20/solid";
import type { Entry } from "../api/taggo";
import { Button } from "./Button";

interface Props {
  /** まだ無いタグのページ（tagPage が返したもの）。 */
  page: Entry;
  /** md ファイルを作り、既定のエディタで開く。 */
  onCreate: (page: Entry) => Promise<void>;
}

export function NewTagPageBar({ page, onCreate }: Props) {
  const [creating, setCreating] = useState(false);
  return (
    <div className="mx-4.5 mt-2.5 flex flex-wrap items-center gap-x-3 gap-y-2 rounded-md border border-dashed border-line px-3 py-2 text-xs text-ink-muted">
      <span className="min-w-60 flex-1">
        「<strong className="text-ink">{page.title}</strong>」のページはまだありません。
        作ると、このタグのページとして一覧の先頭に出ます。
      </span>
      <Button
        variant="primary"
        disabled={creating}
        onClick={() => {
          setCreating(true);
          void onCreate(page).finally(() => setCreating(false));
        }}
      >
        <DocumentPlusIcon className="size-4" aria-hidden="true" />
        {page.name} を作成する
      </Button>
    </div>
  );
}
