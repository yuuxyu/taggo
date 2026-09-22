/**
 * 設定画面。ツールバーの歯車ボタンから開く。
 *
 * 設定は %APPDATA%\taggo\settings.json に保存する。持つのは「消えても初期状態へ戻るだけ」の
 * アプリの好みに限り、ノートのデータは持たない。保存先は画面の下端に見せておく。
 */

import { useEffect, useState, type ReactNode } from "react";
import { FolderOpenIcon, XMarkIcon } from "@heroicons/react/16/solid";
import {
  getSettingsPath,
  saveSettings,
  SCAN_LIMIT_MAX,
  SCAN_LIMIT_MIN,
  selectFolder,
  type Settings,
  type SortOrder,
  type Theme,
} from "../api/taggo";
import { Button } from "./Button";
import { SORT_LABELS } from "./Toolbar";

interface Props {
  settings: Settings;
  /** 今開いているフォルダ。開いていなければ空文字。 */
  currentRoot: string;
  onClose: () => void;
  onSaved: (saved: Settings) => void;
}

const THEME_LABELS: Record<Theme, string> = {
  system: "OS に合わせる",
  light: "ライト",
  dark: "ダーク",
};

export function SettingsDialog({ settings, currentRoot, onClose, onSaved }: Props) {
  const [draft, setDraft] = useState<Settings>(settings);
  // 入力途中の空欄や桁の途中も受け付けられるよう、件数は文字列のまま持つ。
  const [scanLimitText, setScanLimitText] = useState(String(settings.scanLimit));
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [settingsPath, setSettingsPath] = useState("");

  useEffect(() => {
    void getSettingsPath().then(setSettingsPath);
  }, []);

  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape" && !busy) onClose();
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [busy, onClose]);

  const update = (patch: Partial<Settings>) => setDraft((prev) => ({ ...prev, ...patch }));

  const chooseStartupFolder = async () => {
    try {
      const dir = await selectFolder();
      if (dir) update({ startupMode: "fixed", startupFolder: dir });
    } catch (err) {
      setError(String(err));
    }
  };

  const scanLimit = Number(scanLimitText);
  const scanLimitValid =
    Number.isInteger(scanLimit) && scanLimit >= SCAN_LIMIT_MIN && scanLimit <= SCAN_LIMIT_MAX;
  const folderMissing = draft.startupMode === "fixed" && !draft.startupFolder;
  const canSave = scanLimitValid && !folderMissing && !busy;

  const save = async () => {
    setBusy(true);
    setError(null);
    try {
      onSaved(await saveSettings({ ...draft, scanLimit }));
    } catch (err) {
      setError(String(err));
    } finally {
      setBusy(false);
    }
  };

  // 「前回開いたフォルダ」を選んだときに、次の起動で何が開くかを添える。
  const lastFolderHint =
    settings.startupMode === "last"
      ? settings.lastFolder || "まだありません。次に開いたフォルダから覚えます。"
      : currentRoot || "次に開いたフォルダから覚えます。";

  return (
    <div className="fixed inset-0 z-[110] grid place-items-center p-8" role="dialog" aria-modal="true">
      <div className="absolute inset-0 bg-black/40 backdrop-blur-[2px]" onClick={busy ? undefined : onClose} />

      <form
        className="relative flex max-h-[88vh] w-160 max-w-full flex-col gap-5 overflow-y-auto rounded-xl border border-line bg-surface p-5.5 shadow-pop"
        onSubmit={(e) => {
          e.preventDefault();
          if (canSave) void save();
        }}
      >
        <header>
          <h2 className="m-0 text-base font-semibold">設定</h2>
        </header>

        <Section title="起動時に開くフォルダ">
          <div className="flex flex-col gap-2">
            <Choice
              name="startupMode"
              checked={draft.startupMode === "none"}
              onChange={() => update({ startupMode: "none" })}
              label="開かない"
            />
            <Choice
              name="startupMode"
              checked={draft.startupMode === "last"}
              onChange={() => update({ startupMode: "last" })}
              label="前回開いたフォルダ"
              hint={draft.startupMode === "last" ? lastFolderHint : undefined}
            />
            <Choice
              name="startupMode"
              checked={draft.startupMode === "fixed"}
              onChange={() => update({ startupMode: "fixed" })}
              label="指定したフォルダ"
            />
            <div className="flex items-center gap-2 pl-6">
              <code
                className="min-w-0 flex-1 truncate rounded-md border border-line bg-sunken px-2 py-1.5 font-mono text-xs text-ink-muted"
                title={draft.startupFolder}
              >
                {draft.startupFolder || "未選択"}
              </code>
              <Button onClick={() => void chooseStartupFolder()} disabled={busy}>
                <FolderOpenIcon className="size-4" aria-hidden="true" />
                選択…
              </Button>
            </div>
          </div>
          <p className="m-0 mt-2 text-xs text-ink-faint">
            コマンドラインでフォルダを指定して起動したときは、そちらを優先します。
          </p>
        </Section>

        <Section title="一覧の既定の並び順">
          <select
            className="rounded-md border border-line bg-surface px-2 py-1 text-sm text-ink"
            value={draft.sort}
            onChange={(e) => update({ sort: e.target.value as SortOrder })}
          >
            {(Object.keys(SORT_LABELS) as SortOrder[]).map((key) => (
              <option key={key} value={key}>
                {SORT_LABELS[key]}
              </option>
            ))}
          </select>
          <p className="m-0 mt-2 text-xs text-ink-faint">
            起動したときの並び順です。ツールバーで切り替えた並び順は保存しません。
          </p>
        </Section>

        <Section title="1 回の走査の上限件数">
          <label className="inline-flex items-center gap-2 text-sm">
            <input
              type="number"
              inputMode="numeric"
              min={SCAN_LIMIT_MIN}
              max={SCAN_LIMIT_MAX}
              step={1000}
              className={`w-32 rounded-md border bg-surface px-2 py-1 text-right text-sm tabular-nums text-ink ${
                scanLimitValid ? "border-line" : "border-danger"
              }`}
              value={scanLimitText}
              onChange={(e) => setScanLimitText(e.target.value)}
            />
            件
          </label>
          <p className={`m-0 mt-2 text-xs ${scanLimitValid ? "text-ink-faint" : "text-danger"}`}>
            {SCAN_LIMIT_MIN.toLocaleString()}〜{SCAN_LIMIT_MAX.toLocaleString()} 件で指定します。
            多いほどメモリを使います。次にフォルダを開いたときと、続きを読み込むときから反映します。
          </p>
        </Section>

        <Section title="テーマ">
          <div className="flex flex-wrap gap-4">
            {(Object.keys(THEME_LABELS) as Theme[]).map((key) => (
              <Choice
                key={key}
                name="theme"
                checked={draft.theme === key}
                onChange={() => update({ theme: key })}
                label={THEME_LABELS[key]}
              />
            ))}
          </div>
        </Section>

        {error && (
          <p className="m-0 rounded-lg bg-danger-soft px-3.5 py-2.5 text-xs text-danger">{error}</p>
        )}

        <footer className="flex flex-wrap items-center gap-2">
          <span className="min-w-0 flex-1 truncate text-xs text-ink-faint" title={settingsPath}>
            {settingsPath && <>保存先: {settingsPath}</>}
          </span>
          <Button onClick={onClose} disabled={busy}>
            <XMarkIcon className="size-4" aria-hidden="true" />
            キャンセル
          </Button>
          <Button type="submit" variant="primary" disabled={!canSave}>
            {busy ? "保存中…" : "保存"}
          </Button>
        </footer>
      </form>
    </div>
  );
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section>
      <h3 className="m-0 mb-2 text-xs font-semibold tracking-wide text-ink-muted">{title}</h3>
      {children}
    </section>
  );
}

interface ChoiceProps {
  name: string;
  checked: boolean;
  onChange: () => void;
  label: string;
  /** 選択肢の下に添える補足。 */
  hint?: string;
}

function Choice({ name, checked, onChange, label, hint }: ChoiceProps) {
  return (
    <label className="flex items-start gap-2 text-sm">
      <input
        type="radio"
        name={name}
        className="mt-0.5 accent-accent"
        checked={checked}
        onChange={onChange}
      />
      <span className="flex min-w-0 flex-col">
        {label}
        {hint && (
          <span className="truncate font-mono text-xs text-ink-faint" title={hint}>
            {hint}
          </span>
        )}
      </span>
    </label>
  );
}
