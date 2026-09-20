/**
 * 中身がまだクラウド上にしかないファイルの案内。
 *
 * taggo はこの状態のファイルを一度も開かない。開けばその場でダウンロードが
 * 始まり、同期フォルダを開いただけで大量の通信が走りかねないためで、
 * 取り込むかどうかはここで利用者が決める。
 */

import { useState } from "react";
import { CloudArrowDownIcon } from "@heroicons/react/20/solid";
import { fetchCloudEntry, type Entry } from "../api/taggo";
import { Button } from "./Button";

interface Props {
  entry: Entry;
  /** 取り込めた最新の状態を一覧へ返す。 */
  onFetched: (entry: Entry) => void;
  onError: (message: string) => void;
}

export function CloudOnlyNotice({ entry, onFetched, onError }: Props) {
  const [busy, setBusy] = useState(false);

  const fetchNow = async () => {
    setBusy(true);
    try {
      onFetched(await fetchCloudEntry(entry.path));
    } catch (err) {
      onError(`ファイルを取り込めませんでした: ${String(err)}`);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="flex h-full flex-col items-center justify-center gap-3 p-10 text-center text-ink-muted">
      <CloudArrowDownIcon className="size-10 text-ink-faint" aria-hidden="true" />
      <p className="m-0 text-base font-semibold text-ink">このファイルはクラウド上にだけあります</p>
      <p className="m-0 max-w-120">
        中身はまだ手元にありません。taggo は勝手にダウンロードしないので、タグもプレビューも
        まだ読み取っていません。取り込むと、このファイルのダウンロードが始まります。
      </p>
      <Button variant="primary" onClick={() => void fetchNow()} disabled={busy}>
        <CloudArrowDownIcon className="size-4" aria-hidden="true" />
        {busy ? "取り込み中…" : "ダウンロードして読み込む"}
      </Button>
    </div>
  );
}
