/**
 * Markdown のプレビュー。
 *
 * CommonMark / GFM に準拠してレンダリングし、コードハイライト・テーブル・mermaid を扱う。
 * Front Matter はバックエンド側で本文から切り離されているため、ここには届かない。
 */

import { isValidElement, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import rehypeHighlight from "rehype-highlight";
import { getBacklinks, getMarkdownSource, type Backlink, type Entry } from "../api/taggo";
import { Mermaid } from "./Mermaid";
import "highlight.js/styles/github.css";
import "./MarkdownPreview.css";

interface Props {
  entry: Entry;
  /** WikiLink をたどるときに呼ぶ。解決できない場合は検索に落とす。 */
  onFollowLink: (target: string) => void;
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
  const containerRef = useRef<HTMLDivElement>(null);

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
    return <div className="mdpreview__error">本文を読み込めませんでした: {error}</div>;
  }
  if (source === null) {
    return <div className="mdpreview__loading">読み込み中…</div>;
  }

  return (
    <div className="mdpreview" ref={containerRef}>
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
                  className="mdpreview__wikilink"
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

      {backlinks.length > 0 && (
        <section className="mdpreview__backlinks">
          <h4>このノートを参照しているノート</h4>
          <ul>
            {backlinks.map((link) => (
              <li key={link.path}>
                <button type="button" onClick={() => onFollowLink(link.title)}>
                  {link.title}
                </button>
              </li>
            ))}
          </ul>
        </section>
      )}
    </div>
  );
}
