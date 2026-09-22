import React from "react";
import { createRoot } from "react-dom/client";
import App from "./App";
import { DEFAULT_SETTINGS, getSettings, type Settings } from "./api/taggo";
import { applyTheme } from "./theme";
import "./styles/global.css";

const container = document.getElementById("root");
if (!container) {
  throw new Error("#root が見つかりません");
}

/** 設定を読む。読めなくても既定の設定で画面は出す。 */
async function loadSettings(): Promise<Settings> {
  try {
    return await getSettings();
  } catch (err) {
    console.error("設定を読み込めませんでした", err);
    return DEFAULT_SETTINGS;
  }
}

// 設定が届くまでの間も配色が決まっているよう、まず OS に合わせておく。
applyTheme("system");

// 起動時の並び順やテーマで描き直さずに済むよう、設定を読んでから描く。
void loadSettings().then((settings) => {
  applyTheme(settings.theme);
  createRoot(container).render(
    <React.StrictMode>
      <App initialSettings={settings} />
    </React.StrictMode>,
  );
});
