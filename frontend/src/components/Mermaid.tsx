/**
 * mermaid のコードブロックを図として描くコンポーネント。
 *
 * mermaid は描画が重く初期化も 1 度で済むため、モジュールを遅延読み込みし、
 * 初期化はアプリ全体で 1 回だけ行う。
 */

import { useEffect, useId, useRef, useState } from "react";

let initialized = false;

/** mermaid 本体を読み込み、テーマを OS 設定に合わせて初期化する。 */
async function loadMermaid() {
  const mermaid = (await import("mermaid")).default;
  if (!initialized) {
    const dark = window.matchMedia("(prefers-color-scheme: dark)").matches;
    mermaid.initialize({
      startOnLoad: false,
      theme: dark ? "dark" : "neutral",
      securityLevel: "strict",
      fontFamily: "inherit",
    });
    initialized = true;
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

  useEffect(() => {
    let cancelled = false;
    setSvg(null);
    setError(null);

    void loadMermaid()
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
  }, [id, source]);

  if (error) {
    return (
      <pre className="mermaid-error">
        図を描画できませんでした: {error}
        {"\n\n"}
        {source}
      </pre>
    );
  }
  if (svg === null) {
    return <div className="mermaid-loading">図を描画中…</div>;
  }
  // mermaid が生成する SVG は自前で組み立てたものなので、そのまま埋め込む。
  return <div className="mermaid" ref={containerRef} dangerouslySetInnerHTML={{ __html: svg }} />;
}
