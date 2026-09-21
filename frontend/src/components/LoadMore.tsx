/**
 * 上限件数で打ち切ったフォルダの、続きを読み込むための UI。
 *
 * 上限は RAM を使いすぎないための安全弁なので、黙って全部は読まない。
 * 代わりに「どこまで読んだか」「あと何件あるか」を常に見せ、
 * 続きを読むかどうかは利用者に選んでもらう。
 */

import { ArrowDownTrayIcon, InformationCircleIcon, XMarkIcon } from "@heroicons/react/20/solid";
import type { Status } from "../api/taggo";
import { Button } from "./Button";

/** 1 回の「続きを読み込む」で増える件数。残りが上限より少なければ残り全部。 */
export function nextBatchSize(status: Status): number {
  return Math.min(status.maxEntries, status.remaining);
}

interface BannerProps {
  status: Status;
  onLoadMore: (all: boolean) => void;
  onDismiss: () => void;
}

/** 走査が上限で止まったことを伝え、続きの読み込みを促すバナー。閉じるまで残る。 */
export function LoadMoreBanner({ status, onLoadMore, onDismiss }: BannerProps) {
  const total = status.entryCount + status.remaining;
  const batch = nextBatchSize(status);

  return (
    <div className="mx-4.5 mt-2.5 flex flex-wrap items-center gap-x-3 gap-y-2 rounded-md bg-accent-soft px-3 py-2 text-xs text-accent-ink">
      <InformationCircleIcon className="size-4 shrink-0" aria-hidden="true" />
      <span className="min-w-60 flex-1">
        対応ファイル <strong className="tabular-nums">{total.toLocaleString()}</strong> 件のうち、
        パス順の先頭 <strong className="tabular-nums">{status.entryCount.toLocaleString()}</strong>{" "}
        件を読み込みました。残りの{" "}
        <strong className="tabular-nums">{status.remaining.toLocaleString()}</strong>{" "}
        件は一覧にも検索にも出ていません。
      </span>
      <span className="flex flex-wrap items-center gap-1.5">
        <Button variant="primary" onClick={() => onLoadMore(false)}>
          <ArrowDownTrayIcon className="size-4" aria-hidden="true" />
          続きを読み込む（+{batch.toLocaleString()} 件）
        </Button>
        {/* 残りが 1 回分に収まるなら、「すべて」は上のボタンと同じなので出さない。 */}
        {status.remaining > batch && (
          <Button onClick={() => onLoadMore(true)}>すべて読み込む</Button>
        )}
        <Button variant="ghost" onClick={onDismiss}>
          <XMarkIcon className="size-4" aria-hidden="true" />
          このままにする
        </Button>
      </span>
    </div>
  );
}

interface HintProps {
  remaining: number;
  onLoadMore: () => void;
  /** 読み込み中は押せないようにする。 */
  disabled?: boolean;
}

/**
 * 検索結果に、読み込んでいないファイルが含まれていないことを添える。
 * これが無いと「タグを付けたはずなのに出てこない」と誤解されやすい。
 */
export function NotLoadedHint({ remaining, onLoadMore, disabled }: HintProps) {
  return (
    <div className="flex flex-wrap items-center justify-center gap-x-3 gap-y-1.5 text-xs text-ink-muted">
      <span>
        読み込んでいない <strong className="tabular-nums">{remaining.toLocaleString()}</strong>{" "}
        件は検索の対象外です
      </span>
      <Button variant="ghost" onClick={onLoadMore} disabled={disabled}>
        <ArrowDownTrayIcon className="size-4" aria-hidden="true" />
        続きを読み込んで検索する
      </Button>
    </div>
  );
}
