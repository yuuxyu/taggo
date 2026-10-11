/**
 * 取り返しのつく形で確認を取るためのダイアログ。
 *
 * いまは、上限を大きく超えて続きを読み込む前と、ページを削除する前の確認に使っている。
 * 既定の操作を「やめる」側に置き、勢いで読み込みや削除を始めてしまわないようにする。
 *
 * Esc でやめる。下のノートの画面や一覧も Esc や Alt + 左右、マウスの戻る／進むボタンで動くので、
 * キャプチャで先に受けて届かないようにする。
 */

import { useEffect } from "react";
import { ExclamationTriangleIcon } from "@heroicons/react/20/solid";
import { Button } from "./Button";

interface Props {
  title: string;
  /** 本文。改行したい場合は配列で渡す。 */
  lines: string[];
  confirmLabel: string;
  cancelLabel?: string;
  onConfirm: () => void;
  onCancel: () => void;
}

export function ConfirmDialog({
  title,
  lines,
  confirmLabel,
  cancelLabel = "やめる",
  onConfirm,
  onCancel,
}: Props) {
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.stopPropagation();
        e.preventDefault();
        onCancel();
        return;
      }
      if (e.altKey && (e.key === "ArrowLeft" || e.key === "ArrowRight")) e.stopPropagation();
    };
    const onMouse = (e: MouseEvent) => {
      if (e.button !== 3 && e.button !== 4) return;
      e.stopPropagation();
      e.preventDefault();
    };
    window.addEventListener("keydown", onKeyDown, { capture: true });
    window.addEventListener("mousedown", onMouse, { capture: true });
    window.addEventListener("mouseup", onMouse, { capture: true });
    return () => {
      window.removeEventListener("keydown", onKeyDown, { capture: true });
      window.removeEventListener("mousedown", onMouse, { capture: true });
      window.removeEventListener("mouseup", onMouse, { capture: true });
    };
  }, [onCancel]);

  return (
    <div className="fixed inset-0 z-[120] grid place-items-center p-8" role="dialog" aria-modal="true">
      <div className="absolute inset-0 bg-black/40 backdrop-blur-[2px]" onClick={onCancel} />

      <div className="relative flex w-140 max-w-full flex-col gap-4 rounded-xl border border-line bg-surface p-5.5 shadow-pop">
        <header className="flex items-start gap-3">
          <ExclamationTriangleIcon className="mt-0.5 size-5 shrink-0 text-danger" aria-hidden="true" />
          <h2 className="m-0 text-base font-semibold">{title}</h2>
        </header>

        <div className="flex flex-col gap-2 text-sm text-ink-muted">
          {lines.map((line) => (
            <p key={line} className="m-0">
              {line}
            </p>
          ))}
        </div>

        <footer className="flex items-center justify-end gap-2">
          <Button onClick={onCancel} autoFocus>
            {cancelLabel}
          </Button>
          <Button variant="primary" onClick={onConfirm}>
            {confirmLabel}
          </Button>
        </footer>
      </div>
    </div>
  );
}
