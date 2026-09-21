/**
 * Markdown 本文に URL だけを書いた段落を、リンク先の OGP をもとにしたカードとして描く。
 *
 * 取得は Go 側で行う（WebView からは CORS で他サイトの HTML を読めないため）。
 * YouTube は動画のサムネイルを大きく、X は投稿の本文を主役に、
 * それ以外のページはタイトル・説明・画像を横に並べる。
 *
 * 取得できなかったときは、カードにせず元の段落（ただのリンク）を出す。
 */

import { useEffect, useState, type ReactNode } from "react";
import { GlobeAltIcon, PlayIcon } from "@heroicons/react/20/solid";
import { BrowserOpenURL } from "../../wailsjs/runtime/runtime";
import { getLinkPreview, type LinkPreview } from "../api/taggo";

interface Props {
  url: string;
  /** 取得に失敗したときに代わりに出すもの。元の段落を渡す。 */
  fallback: ReactNode;
}

/**
 * 取得済みのプレビュー。本文を読み直すとカードも作り直されるので、
 * ここに覚えておいて、2 回目以降は読み込み中の表示を挟まずに出す。
 * 失敗は覚えない。一時的な失敗なら、次に開いたときにやり直せるように。
 */
const loaded = new Map<string, LinkPreview>();
const inflight = new Map<string, Promise<LinkPreview>>();

function loadPreview(url: string): Promise<LinkPreview> {
  let p = inflight.get(url);
  if (!p) {
    p = getLinkPreview(url)
      .then((preview) => {
        loaded.set(url, preview);
        return preview;
      })
      .finally(() => inflight.delete(url));
    inflight.set(url, p);
  }
  return p;
}

type State = { status: "loading" } | { status: "ok"; preview: LinkPreview } | { status: "error" };

function initialState(url: string): State {
  const preview = loaded.get(url);
  return preview ? { status: "ok", preview } : { status: "loading" };
}

/** カードの外枠。クリックで OS の既定のブラウザに開かせる。 */
function Frame({ url, className, children }: { url: string; className: string; children: ReactNode }) {
  return (
    <a
      href={url}
      title={url}
      rel="noreferrer"
      className={`not-prose my-5 flex overflow-hidden rounded-lg border border-line bg-surface text-ink no-underline transition hover:border-line-strong hover:bg-sunken focus-visible:ring-3 focus-visible:ring-accent-soft focus-visible:outline-hidden ${className}`}
      onClick={(e) => {
        // target="_blank" に任せると WebView2 が自前のウィンドウを開いてしまう。
        e.preventDefault();
        BrowserOpenURL(url);
      }}
    >
      {children}
    </a>
  );
}

/** カード下部のサイト名。 */
function Site({ name, extra }: { name?: string; extra?: string }) {
  return (
    <span className="flex min-w-0 items-center gap-1 text-xs text-ink-faint">
      <GlobeAltIcon className="size-3.5 shrink-0" aria-hidden="true" />
      <span className="truncate">{[name, extra].filter(Boolean).join(" · ")}</span>
    </span>
  );
}

/** 画像。読み込めなければ何も出さない。 */
function Thumbnail({ src, className }: { src: string; className: string }) {
  const [failed, setFailed] = useState(false);
  if (failed) return null;
  return (
    <img
      src={src}
      alt=""
      loading="lazy"
      referrerPolicy="no-referrer"
      className={`object-cover ${className}`}
      onError={() => setFailed(true)}
    />
  );
}

function YouTubeCard({ preview }: { preview: LinkPreview }) {
  return (
    <Frame url={preview.url} className="max-w-[28rem] flex-col">
      {preview.image && (
        <div className="relative aspect-video w-full bg-sunken">
          <Thumbnail src={preview.image} className="absolute inset-0 size-full" />
          <span className="absolute inset-0 grid place-items-center">
            <span className="grid size-12 place-items-center rounded-full bg-black/60 text-white">
              <PlayIcon className="ml-0.5 size-6" aria-hidden="true" />
            </span>
          </span>
        </div>
      )}
      <span className="flex flex-col gap-1 px-4 py-3">
        <span className="line-clamp-2 text-base font-semibold">{preview.title}</span>
        <Site name={preview.siteName} extra={preview.author} />
      </span>
    </Frame>
  );
}

function XCard({ preview }: { preview: LinkPreview }) {
  return (
    <Frame url={preview.url} className="flex-col gap-2 px-4 py-3">
      <span className="flex min-w-0 items-baseline gap-1.5">
        <span className="truncate text-sm font-semibold">{preview.title}</span>
        {preview.author && <span className="truncate text-xs text-ink-muted">{preview.author}</span>}
      </span>
      {preview.description && (
        <span className="line-clamp-6 text-sm leading-relaxed whitespace-pre-wrap">{preview.description}</span>
      )}
      <Site name={preview.siteName} />
    </Frame>
  );
}

function PageCard({ preview }: { preview: LinkPreview }) {
  return (
    <Frame url={preview.url} className="items-stretch">
      <span className="flex min-w-0 flex-1 flex-col gap-1 px-4 py-3">
        <span className="line-clamp-2 text-base font-semibold">{preview.title}</span>
        {preview.description && (
          <span className="line-clamp-2 text-sm leading-relaxed text-ink-muted">{preview.description}</span>
        )}
        <span className="mt-auto pt-1">
          <Site name={preview.siteName} />
        </span>
      </span>
      {preview.image && (
        // OGP 画像は 1.91:1 が標準。カードの高さに合わせて切り抜く。
        <Thumbnail
          src={preview.image}
          className="aspect-[1.91/1] w-[34%] max-w-60 shrink-0 self-center border-l border-line"
        />
      )}
    </Frame>
  );
}

export function LinkCard({ url, fallback }: Props) {
  const [state, setState] = useState<State>(() => initialState(url));

  useEffect(() => {
    setState(initialState(url));
    if (loaded.has(url)) return;

    let cancelled = false;
    loadPreview(url)
      .then((preview) => {
        if (!cancelled) setState({ status: "ok", preview });
      })
      .catch(() => {
        if (!cancelled) setState({ status: "error" });
      });
    return () => {
      cancelled = true;
    };
  }, [url]);

  if (state.status === "error") return <>{fallback}</>;
  if (state.status === "loading") {
    return (
      <Frame url={url} className="flex-col gap-1 px-4 py-3">
        <span className="text-sm text-ink-muted">リンク先を読み込み中…</span>
        <span className="truncate text-xs text-ink-faint">{url}</span>
      </Frame>
    );
  }

  const { preview } = state;
  switch (preview.kind) {
    case "youtube":
      return <YouTubeCard preview={preview} />;
    case "x":
      return <XCard preview={preview} />;
    default:
      return <PageCard preview={preview} />;
  }
}
