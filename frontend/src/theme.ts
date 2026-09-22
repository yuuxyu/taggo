/**
 * 画面の配色の切り替え。
 *
 * 設定のテーマを <html data-theme="light|dark"> へ反映する。配色トークン（global.css）と
 * Tailwind の dark: バリアントは、OS の設定ではなくこの属性を見る。
 * 「OS に合わせる」のときは、OS の設定が変わるたびに属性を書き換えて追従する。
 */

import { useSyncExternalStore } from "react";
import type { Theme } from "./api/taggo";

export type ResolvedTheme = "light" | "dark";

const systemDark = window.matchMedia("(prefers-color-scheme: dark)");
let chosen: Theme = "system";
const listeners = new Set<() => void>();

function resolve(): ResolvedTheme {
  if (chosen !== "system") return chosen;
  return systemDark.matches ? "dark" : "light";
}

function update() {
  const next = resolve();
  const root = document.documentElement;
  if (root.dataset.theme === next) return;
  root.dataset.theme = next;
  for (const listener of listeners) listener();
}

systemDark.addEventListener("change", () => {
  if (chosen === "system") update();
});

/** 設定のテーマを画面へ反映する。 */
export function applyTheme(theme: Theme) {
  chosen = theme;
  update();
}

/** いま実際に使っている配色（「OS に合わせる」を解決したもの）を返す。 */
export function resolvedTheme(): ResolvedTheme {
  return resolve();
}

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

/** いま実際に使っている配色。切り替わると再描画される。mermaid のように自前で色を持つ部品が使う。 */
export function useResolvedTheme(): ResolvedTheme {
  return useSyncExternalStore(subscribe, resolvedTheme);
}
