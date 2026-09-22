/**
 * mermaid のコードブロックを図として描くコンポーネント。
 *
 * mermaid は描画が重いため、モジュールを遅延読み込みする。初期化は配色が
 * 変わったときだけやり直し、テーマを切り替えると描いてある図も描き直す。
 */

import { useEffect, useId, useRef, useState } from "react";
import { useResolvedTheme, type ResolvedTheme } from "../theme";

/** 最後に初期化したときの配色。まだ初期化していなければ null。 */
let initializedTheme: ResolvedTheme | null = null;

/** mermaid 本体を読み込み、画面の配色に合わせて初期化する。 */
async function loadMermaid(theme: ResolvedTheme) {
  const mermaid = (await import("mermaid")).default;
  if (initializedTheme !== theme) {
    mermaid.initialize({
      startOnLoad: false,
      theme: theme === "dark" ? "dark" : "neutral",
      securityLevel: "strict",
      fontFamily: "inherit",
    });
    initializedTheme = theme;
  }
  return mermaid;
}

export function Mermaid({ source }: { source: string }) {
  const rawId = useId();
  // useId が返す値にはコロンが含まれ、CSS セレクタとして使えないので置き換える。
  const id = `mermaid-${rawId.replace(/:/g, "")}`;
  const [svg, setSvg] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const theme = useResolvedTheme();

  useEffect(() => {
    let cancelled = false;
    setSvg(null);
    setError(null);

    void loadMermaid(theme)
      .then((mermaid) => mermaid.render(id, source))
      .then(({ svg: rendered }) => {
        if (!cancelled) setSvg(rendered);
      })
      .catch((err) => {
        if (!cancelled) setError(String(err));
      });

    return () => {
      cancelled = true;
    };
  }, [id, source, theme]);

  if (error) {
    return (
      <pre className="overflow-x-auto rounded-lg bg-danger-soft p-3 text-xs text-danger">
        図を描画できませんでした: {error}
        {"\n\n"}
        {source}
      </pre>
    );
  }
  if (svg === null) {
    return (
      <div className="rounded-lg bg-sunken p-5 text-center text-sm text-ink-faint">図を描画中…</div>
    );
  }
  // mermaid が生成する SVG は自前で組み立てたものなので、そのまま埋め込む。
  return (
    <div
      className="my-5 flex justify-center overflow-x-auto rounded-lg border border-line bg-sunken p-3 [&_svg]:h-auto [&_svg]:max-w-full"
      ref={containerRef}
      dangerouslySetInnerHTML={{ __html: svg }}
    />
  );
}
