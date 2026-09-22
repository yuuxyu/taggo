/**
 * アプリ共通のボタン。
 *
 * 同じ見た目のユーティリティを呼び出しごとに並べ直さずに済むよう、
 * 種類（variant）だけを選べるようにしている。
 *
 * hud と hudGhost は本文中の画像を開いたビューアの上に重ねるボタン。背景の画像が何色でも
 * 読めるよう、テーマの配色ではなく白基調に固定する。
 */

import type { ButtonHTMLAttributes } from "react";

export type ButtonVariant = "default" | "primary" | "ghost" | "hud" | "hudGhost";

const BASE =
  "inline-flex shrink-0 items-center gap-1.5 whitespace-nowrap rounded-md border px-3 py-1.5 " +
  "text-sm transition-colors disabled:cursor-not-allowed disabled:opacity-45";

const VARIANTS: Record<ButtonVariant, string> = {
  default: "border-line bg-surface hover:enabled:border-line-strong hover:enabled:bg-sunken",
  primary: "border-accent bg-accent text-white hover:enabled:border-accent-ink hover:enabled:bg-accent-ink",
  ghost: "border-transparent hover:enabled:bg-sunken",
  hud: "border-white/25 bg-white/10 text-white hover:enabled:bg-white/20",
  hudGhost: "border-transparent text-white hover:enabled:bg-white/15",
};

interface Props extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
}

export function Button({ variant = "default", className = "", type = "button", ...rest }: Props) {
  return <button type={type} className={`${BASE} ${VARIANTS[variant]} ${className}`} {...rest} />;
}
