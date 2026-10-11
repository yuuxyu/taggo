/**
 * ページの名前を変えるダイアログ。
 *
 * 名前を変えると、そのページを指す [[タグ]] と Markdown のリンクを、フォルダ直下の
 * すべてのノートで書き換える。ノートを書き換えるのは taggo ではこの操作だけなので、
 * いきなり書き換えずに 2 段階で進める。
 *  1. 新しい名前を入れる
 *  2. 書き換わるノートの一覧（とダウンロードが必要なノート）を見て、変更を確かめる
 */

import { useEffect, useRef, useState } from "react";
import { PencilIcon } from "@heroicons/react/20/solid";
import { planRename, renamePage, type Entry, type RenamePlan, type RenameResult } from "../api/taggo";
import { Button } from "./Button";

interface Props {
  entry: Entry;
  onClose: () => void;
  onRenamed: (result: RenameResult) => void;
}

/** 書き換えるノートの名前を並べる上限。超えた分は件数だけを出す。 */
const MAX_LISTED = 8;

export function RenameDialog({ entry, onClose, onRenamed }: Props) {
  const stem = entry.name.slice(0, entry.name.length - entry.ext.length);
  const [name, setName] = useState(stem);
  const [plan, setPlan] = useState<RenamePlan | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  // 開いたら名前を選んだ状態にして、そのまま打ち直せるようにする。
  useEffect(() => {
    inputRef.current?.select();
  }, []);

  // Esc で閉じる。下のノートの画面や一覧も Esc や Alt + 左右・Ctrl + [ / ]、マウスの戻る／進むボタンで
  // 動くので、キャプチャで先に受けて届かないようにする。書き換えの途中は閉じない。
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.stopPropagation();
        e.preventDefault();
        if (!busy) onClose();
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
  }, [busy, onClose]);

  const check = async () => {
    setBusy(true);
    setError("");
    try {
      setPlan(await planRename(entry.path, name));
    } catch (err) {
      setError(String(err));
    } finally {
      setBusy(false);
    }
  };

  const rename = async () => {
    setBusy(true);
    setError("");
    try {
      onRenamed(await renamePage(entry.path, name));
    } catch (err) {
      setError(String(err));
      setBusy(false);
    }
  };

  return (
    <div className="fixed inset-0 z-[120] grid place-items-center p-8" role="dialog" aria-modal="true">
      <div className="absolute inset-0 bg-black/40 backdrop-blur-[2px]" onClick={busy ? undefined : onClose} />

      <form
        className="relative flex w-140 max-w-full flex-col gap-4 rounded-xl border border-line bg-surface p-5.5 shadow-pop"
        onSubmit={(e) => {
          e.preventDefault();
          if (busy) return;
          void (plan ? rename() : check());
        }}
      >
        <header className="flex items-start gap-3">
          <PencilIcon className="mt-0.5 size-5 shrink-0 text-ink-faint" aria-hidden="true" />
          <h2 className="m-0 text-base font-semibold">
            {plan ? `「${plan.from}」を「${plan.to}」に変更しますか？` : "名前を変更"}
          </h2>
        </header>

        {plan ? (
          <PlanSummary plan={plan} />
        ) : (
          <label className="flex flex-col gap-1.5 text-sm">
            <span className="text-ink-muted">
              新しい名前（拡張子なし）。このページを指す [[タグ]] とリンクも書き換えます。
            </span>
            <input
              ref={inputRef}
              type="text"
              className="rounded-md border border-line bg-surface px-2.5 py-1.5 text-base text-ink outline-hidden focus:border-accent focus:ring-3 focus:ring-accent-soft"
              value={name}
              disabled={busy}
              spellCheck={false}
              autoComplete="off"
              onChange={(e) => setName(e.target.value)}
            />
          </label>
        )}

        {error && <p className="m-0 rounded-lg bg-danger-soft px-3.5 py-2.5 text-xs text-danger">{error}</p>}

        <footer className="flex items-center justify-end gap-2">
          {plan ? (
            <Button
              disabled={busy}
              onClick={() => {
                setPlan(null);
                setError("");
              }}
            >
              戻る
            </Button>
          ) : (
            <Button disabled={busy} onClick={onClose}>
              やめる
            </Button>
          )}
          <Button type="submit" variant="primary" disabled={busy || (!plan && name.trim() === "")}>
            {plan ? (busy ? "変更しています…" : "変更する") : busy ? "確かめています…" : "次へ"}
          </Button>
        </footer>
      </form>
    </div>
  );
}

/** 変更の見込み。書き換えるノートと、ダウンロードが必要なノートを並べる。 */
function PlanSummary({ plan }: { plan: RenamePlan }) {
  return (
    <div className="flex flex-col gap-3 text-sm text-ink-muted">
      <p className="m-0">ファイル名を「{plan.path.split(/[\\/]/).pop()}」に変えます。</p>
      <NameList
        title={
          plan.notes.length > 0
            ? `${plan.notes.length.toLocaleString()} 件のノートの [[タグ]] とリンクを書き換えます。`
            : "このページを指しているノートはありません。"
        }
        names={plan.notes}
      />
      {plan.skipped.length > 0 && (
        <NameList
          title={`大きすぎる ${plan.skipped.length.toLocaleString()} 件は書き換えません。`}
          names={plan.skipped}
        />
      )}
    </div>
  );
}

function NameList({ title, names }: { title: string; names: string[] }) {
  const shown = names.slice(0, MAX_LISTED);
  return (
    <div className="flex flex-col gap-1">
      <p className="m-0">{title}</p>
      {shown.length > 0 && (
        <ul className="m-0 flex list-none flex-col gap-0.5 rounded-md bg-sunken px-3 py-2 text-xs text-ink">
          {shown.map((name) => (
            <li key={name} className="truncate">
              {name}
            </li>
          ))}
          {names.length > shown.length && (
            <li className="text-ink-faint">ほか {(names.length - shown.length).toLocaleString()} 件</li>
          )}
        </ul>
      )}
    </div>
  );
}
