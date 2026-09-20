/**
 * Markdown のプレビュー。
 *
 * CommonMark / GFM に準拠してレンダリングし、コードハイライト・テーブル・mermaid を扱う。
 * Front Matter はバックエンド側で本文から切り離されているため、ここには届かない。
 *
 * 本文は react-markdown が生成する素の HTML なので、装飾は Tailwind の
 * typography プラグイン（prose）に任せ、配色だけをアプリのトークンへ差し替える。
 */

import { isValidElement, useEffect, useMemo, useState, type ReactNode } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import rehypeHighlight from "rehype-highlight";
import { fileURL, getBacklinks, getMarkdownSource, type Backlink, type Entry } from "../api/taggo";
import { Mermaid } from "./Mermaid";
import "highlight.js/styles/github.css";

interface Props {
  entry: Entry;
  /** WikiLink をたどるときに呼ぶ。解決できない場合は検索に落とす。 */
  onFollowLink: (target: string) => void;
}

/**
 * prose の配色をアプリのテーマトークンへ結び付ける。
 * トークン自体が OS のテーマに追従するので、prose-invert は要らない。
 */
const PROSE_COLORS = [
  "[--tw-prose-body:var(--color-ink)]",
  "[--tw-prose-headings:var(--color-ink)]",
  "[--tw-prose-bold:var(--color-ink)]",
  "[--tw-prose-links:var(--color-accent-ink)]",
  "[--tw-prose-counters:var(--color-ink-muted)]",
  "[--tw-prose-bullets:var(--color-line-strong)]",
  "[--tw-prose-hr:var(--color-line)]",
  "[--tw-prose-quotes:var(--color-ink-muted)]",
  "[--tw-prose-quote-borders:var(--color-line-strong)]",
  "[--tw-prose-captions:var(--color-ink-faint)]",
  "[--tw-prose-code:var(--color-ink)]",
  "[--tw-prose-pre-code:var(--color-ink)]",
  "[--tw-prose-pre-bg:var(--color-sunken)]",
  "[--tw-prose-th-borders:var(--color-line)]",
  "[--tw-prose-td-borders:var(--color-line)]",
].join(" ");

/**
 * これまでの見た目に合わせた上書き。
 * 引用の飾り引用符とインラインコードのバッククォートは出さず、
 * 見出し・表・コードブロックには罫線を入れる。
 */
const PROSE_TWEAKS = [
  "prose-headings:font-semibold",
  "prose-h2:border-b prose-h2:border-line prose-h2:pb-[0.3em]",
  "prose-blockquote:font-normal prose-blockquote:not-italic",
  "[&_blockquote_p]:before:content-none [&_blockquote_p]:after:content-none",
  "prose-code:rounded prose-code:bg-sunken prose-code:px-1 prose-code:py-0.5 prose-code:font-normal",
  "prose-code:before:content-none prose-code:after:content-none",
  "prose-pre:rounded-lg prose-pre:border prose-pre:border-line",
  "[&_pre_code]:bg-transparent [&_pre_code]:p-0",
  "prose-th:border prose-th:border-line prose-th:bg-sunken prose-th:px-3 prose-th:py-1.5",
  "prose-td:border prose-td:border-line prose-td:px-3 prose-td:py-1.5",
  "prose-img:rounded-md",
].join(" ");

/** 暗いテーマでも読めるよう、highlight.js（github テーマ）の配色を調整する。 */
const HLJS_DARK = [
  "dark:[&_.hljs]:text-ink dark:[&_.hljs]:bg-transparent",
  "dark:[&_.hljs-comment]:text-ink-faint dark:[&_.hljs-quote]:text-ink-faint",
  "dark:[&_.hljs-keyword]:text-[#d08fd0] dark:[&_.hljs-selector-tag]:text-[#d08fd0] dark:[&_.hljs-literal]:text-[#d08fd0]",
  "dark:[&_.hljs-string]:text-[#8fd3c3] dark:[&_.hljs-attr]:text-[#8fd3c3]",
  "dark:[&_.hljs-number]:text-[#e0b070] dark:[&_.hljs-built_in]:text-[#e0b070]",
  "dark:[&_.hljs-title]:text-[#83b4e8] dark:[&_.hljs-section]:text-[#83b4e8]",
].join(" ");

/** パスを区切り文字で分解する。先頭の空要素（POSIX の "/"）は残す。 */
function splitPath(path: string): string[] {
  return path.split(/[\\/]/);
}

/** パスから親フォルダを取り出す。 */
function dirOf(path: string): string {
  const i = Math.max(path.lastIndexOf("/"), path.lastIndexOf("\\"));
  return i < 0 ? "" : path.slice(0, i);
}

/**
 * 開いているフォルダ（走査ルート）を求める。
 * 絶対パスの末尾から、表示用の相対パスを取り除いたものがルートになる。
 */
function rootOf(entry: Entry): string {
  if (entry.relPath !== "" && entry.path.endsWith(entry.relPath)) {
    return entry.path.slice(0, entry.path.length - entry.relPath.length).replace(/[\\/]+$/, "");
  }
  return dirOf(entry.path);
}

/**
 * 本文から参照された画像の src を、taggo の配信 URL へ書き換える。
 *
 * ブラウザは相対 URL をアプリのページ基準で解決してしまうので、そのままでは
 * ノートの隣に置いた画像を指せない。Markdown ファイルのあるフォルダを基準に
 * 絶対パスへ直したうえで、Go 側の配信エンドポイントへ渡す。
 * "/" 始まりは、ノートからの相対ではなく開いているフォルダ基準として扱う。
 *
 * http(s): や data: などスキーム付きの URL はそのまま通す。Windows の
 * ドライブ文字（C:\… や C:/…）はスキームに見えるがパスなので、そちらへ回す。
 */
function resolveImageSrc(src: string | undefined, entry: Entry): string | undefined {
  if (!src) return src;

  const driveLetter = /^[a-z]:[\\/]/i.test(src);
  if (!driveLetter && /^[a-z][a-z0-9+.-]*:/i.test(src)) return src;

  // 末尾のフラグメント（#…）はパスの一部ではないので落とす。
  const cleaned = src.replace(/#.*$/, "");
  let decoded = cleaned;
  try {
    decoded = decodeURIComponent(cleaned);
  } catch {
    // 壊れたエスケープはそのままのパスとして扱う。
  }

  const sep = entry.path.includes("\\") ? "\\" : "/";
  const base = driveLetter
    ? []
    : splitPath(/^[\\/]/.test(decoded) ? rootOf(entry) : dirOf(entry.path));

  const segments = [...base];
  for (const part of splitPath(decoded)) {
    if (part === "" || part === ".") continue;
    if (part === "..") {
      if (segments.length > 1) segments.pop();
      continue;
    }
    segments.push(part);
  }
  return fileURL(segments.join(sep));
}

/**
 * 本文中の画像。開いているフォルダの外や、taggo が取り込んでいない形式は
 * 配信されないため、読み込めなかったときは元の記述を添えて理由を示す。
 */
function MarkdownImage({ src, alt, title, entry }: { src?: string; alt?: string; title?: string; entry: Entry }) {
  const [failed, setFailed] = useState(false);

  if (failed) {
    return (
      <span className="inline-block rounded-md bg-sunken px-2 py-1 text-xs text-ink-muted">
        画像を表示できません: {src}
      </span>
    );
  }
  return (
    <img
      src={resolveImageSrc(src, entry)}
      alt={alt ?? ""}
      title={title}
      loading="lazy"
      onError={() => setFailed(true)}
    />
  );
}

/**
 * [[WikiLink]] は Markdown の標準記法ではないので、レンダリング前に通常のリンクへ置き換える。
 * taggo 内部だけで完結させるため、スキームに taggo: を使う。
 */
function rewriteWikiLinks(source: string): string {
  return source.replace(/\[\[([^\][|]+)(?:\|([^\][]*))?\]\]/g, (_all, target: string, label?: string) => {
    const text = (label ?? target).trim();
    return `[${text}](taggo:${encodeURIComponent(target.trim())})`;
  });
}

/**
 * React の子要素を、そこに含まれる文字列だけに畳み込む。
 * ハイライト処理が入ると本文が要素へ分割されるため、mermaid のソースを
 * 取り出すには再帰的に文字列を集める必要がある。
 */
function toPlainText(node: ReactNode): string {
  if (node === null || node === undefined || typeof node === "boolean") return "";
  if (typeof node === "string" || typeof node === "number") return String(node);
  if (Array.isArray(node)) return node.map(toPlainText).join("");
  if (isValidElement<{ children?: ReactNode }>(node)) return toPlainText(node.props.children);
  return "";
}

export function MarkdownPreview({ entry, onFollowLink }: Props) {
  const [source, setSource] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [backlinks, setBacklinks] = useState<Backlink[]>([]);

  useEffect(() => {
    let cancelled = false;
    setSource(null);
    setError(null);

    void getMarkdownSource(entry.path)
      .then((text) => {
        if (!cancelled) setSource(text);
      })
      .catch((err) => {
        if (!cancelled) setError(String(err));
      });

    void getBacklinks(entry.path).then((got) => {
      if (!cancelled) setBacklinks(got ?? []);
    });

    return () => {
      cancelled = true;
    };
  }, [entry.path]);

  const prepared = useMemo(() => (source === null ? "" : rewriteWikiLinks(source)), [source]);

  if (error) {
    return <div className="py-6 text-danger">本文を読み込めませんでした: {error}</div>;
  }
  if (source === null) {
    return <div className="py-6 text-ink-muted">読み込み中…</div>;
  }

  return (
    <>
      <div
        className={`prose prose-sm max-w-none break-words ${PROSE_COLORS} ${PROSE_TWEAKS} ${HLJS_DARK}`}
      >
        <ReactMarkdown
          remarkPlugins={[remarkGfm]}
          rehypePlugins={[rehypeHighlight]}
          components={{
            a({ href, children, ...rest }) {
              if (href?.startsWith("taggo:")) {
                const target = decodeURIComponent(href.slice("taggo:".length));
                return (
                  <a
                    href={href}
                    className="rounded-sm bg-accent-soft px-0.5 no-underline"
                    onClick={(e) => {
                      e.preventDefault();
                      onFollowLink(target);
                    }}
                  >
                    {children}
                  </a>
                );
              }
              // 外部リンクは既定のブラウザへ委ねる。
              return (
                <a href={href} target="_blank" rel="noreferrer" {...rest}>
                  {children}
                </a>
              );
            },
            img({ src, alt, title }) {
              // 相対パスはページ基準ではなく、Markdown ファイルの位置を基準に解決する。
              return (
                <MarkdownImage
                  src={typeof src === "string" ? src : undefined}
                  alt={alt}
                  title={title}
                  entry={entry}
                />
              );
            },
            code({ className, children, ...rest }) {
              // mermaid のコードブロックは図として描く。
              // rehype-highlight が "hljs" などのクラスを足すため、完全一致では判定できない。
              if (className?.split(/\s+/).includes("language-mermaid")) {
                return <Mermaid source={toPlainText(children).trimEnd()} />;
              }
              return (
                <code className={className} {...rest}>
                  {children}
                </code>
              );
            },
          }}
        >
          {prepared}
        </ReactMarkdown>
      </div>

      {backlinks.length > 0 && (
        <section className="mt-10 border-t border-line pt-4">
          <h4 className="m-0 mb-2 text-xs font-semibold tracking-wide text-ink-muted">
            このノートを参照しているノート
          </h4>
          <ul className="flex list-none flex-wrap gap-2 p-0">
            {backlinks.map((link) => (
              <li key={link.path}>
                <button
                  type="button"
                  className="rounded-full border border-line bg-sunken px-2.5 py-0.5 text-xs hover:border-accent"
                  onClick={() => onFollowLink(link.title)}
                >
                  {link.title}
                </button>
              </li>
            ))}
          </ul>
        </section>
      )}
    </>
  );
}
