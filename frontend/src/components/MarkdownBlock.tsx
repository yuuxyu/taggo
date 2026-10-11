/**
 * Markdown の一部を HTML として描く。
 *
 * 本文は編集画面（NoteEditor）がそのまま見せるが、表のようにテキストのままでは
 * 読みにくいものは、カーソルが外れている間だけここで描き直して見せる。
 * GFM に準拠してレンダリングし、`[[タグ]]` はそのタグのページへのリンクにする。
 *
 * 装飾は Tailwind の typography プラグイン（prose）に任せ、配色だけをアプリのトークンへ差し替える。
 */

import { useState } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { BrowserOpenURL } from "../../wailsjs/runtime/runtime";
import { imageURL, type Entry } from "../api/taggo";
import { remarkBracketTags } from "../bracketTags";
import {
  EXTERNAL_LINK,
  EXTERNAL_LINK_RE,
  INTERNAL_LINK,
  MISSING_LINK,
  noteLink,
  resolveNotePath,
} from "../links";
import { resolveLocalPath } from "../notePath";

export interface MarkdownBlockProps {
  /** 描く Markdown。 */
  source: string;
  /** 本文を書いているノート。相対パスの画像とリンクの基準になる。 */
  entry: Entry;
  /**
   * ノートへのリンクをたどるときに呼ぶ。link は書かれたパスをデコードしたもの。
   * path を省くと、行き先がまだ無いノートとして（ファイルは作らずに）開く。
   */
  onFollowLink: (link: string, path?: string) => void;
  /** 行き先がまだ無いリンク（小文字にしたもの）。色を変えて示す。 */
  missingLinks: ReadonlySet<string>;
  /** `[[タグ]]` をたどるときに呼ぶ。tag は正規化したタグ。 */
  onFollowTag: (tag: string) => void;
  /** ページがまだ無いタグ（小文字にしたもの）。`[[タグ]]` の色を変えて示す。 */
  missingTags: ReadonlySet<string>;
  /** 画像をクリックしたときに、その画像のパスと代替テキストを渡して呼ぶ。 */
  onOpenImage: (path: string, alt?: string) => void;
}

/**
 * prose の配色をアプリのテーマトークンへ結び付ける。
 * トークン自体がテーマに追従するので、prose-invert は要らない。
 */
const PROSE_COLORS = [
  "[--tw-prose-body:var(--color-ink)]",
  "[--tw-prose-headings:var(--color-ink)]",
  "[--tw-prose-bold:var(--color-ink)]",
  "[--tw-prose-links:var(--color-accent-ink)]",
  "[--tw-prose-code:var(--color-ink)]",
  "[--tw-prose-th-borders:var(--color-line)]",
  "[--tw-prose-td-borders:var(--color-line)]",
].join(" ");

/** 表には罫線を入れ、インラインコードはバッククォートを出さずに地の色で示す。 */
const PROSE_TWEAKS = [
  "prose-code:rounded prose-code:bg-sunken prose-code:px-1 prose-code:py-0.5 prose-code:font-normal",
  "prose-code:before:content-none prose-code:after:content-none",
  "prose-table:my-0",
  "prose-th:border prose-th:border-line prose-th:bg-sunken prose-th:px-3 prose-th:py-1.5",
  "prose-td:border prose-td:border-line prose-td:px-3 prose-td:py-1.5",
  "prose-img:my-0 prose-img:rounded-md",
].join(" ");

/**
 * 画像。開いているフォルダの外の画像や、表示できない形式のファイルは
 * 配信されないため、読み込めなかったときは元の記述を添えて理由を示す。
 * フォルダ内の画像はクリックで画像ビューアを開ける。
 */
function MarkdownImage({
  src,
  alt,
  title,
  entry,
  onOpen,
}: {
  src?: string;
  alt?: string;
  title?: string;
  entry: Entry;
  onOpen: (path: string, alt?: string) => void;
}) {
  const [failed, setFailed] = useState(false);
  const localPath = src ? resolveLocalPath(src, entry) : null;

  if (failed) {
    return (
      <span className="inline-block rounded-md bg-sunken px-2 py-1 text-xs text-ink-muted">
        画像を表示できません: {src}
      </span>
    );
  }
  if (localPath === null) {
    return <img src={src} alt={alt ?? ""} title={title} loading="lazy" onError={() => setFailed(true)} />;
  }
  return (
    <img
      src={imageURL(localPath)}
      alt={alt ?? ""}
      title={title ?? "クリックで画像を開く"}
      loading="lazy"
      className="cursor-zoom-in"
      onError={() => setFailed(true)}
      onClick={(e) => {
        // リンクで囲まれた画像は、リンクの行き先のほうを優先する。
        if (e.currentTarget.closest("a")) return;
        e.stopPropagation();
        onOpen(localPath, alt);
      }}
    />
  );
}

export function MarkdownBlock({
  source,
  entry,
  onFollowLink,
  missingLinks,
  onFollowTag,
  missingTags,
  onOpenImage,
}: MarkdownBlockProps) {
  return (
    <div className={`prose prose-lg max-w-none break-words ${PROSE_COLORS} ${PROSE_TWEAKS}`}>
      <ReactMarkdown
        remarkPlugins={[remarkGfm, remarkBracketTags]}
        components={{
          a({ node, href, children, ...rest }) {
            // [[タグ]] は、そのタグのページを開く。ページがまだ無ければ色を変える。
            const tag = node?.properties?.dataTag;
            if (typeof tag === "string") {
              const missing = missingTags.has(tag.toLowerCase());
              return (
                <a
                  href="#"
                  className={missing ? MISSING_LINK : INTERNAL_LINK}
                  title={missing ? `「${tag}」のページはまだありません` : `「${tag}」のページを開く`}
                  onClick={(e) => {
                    e.preventDefault();
                    e.stopPropagation();
                    onFollowTag(tag);
                  }}
                >
                  {children}
                </a>
              );
            }
            // 他のノートへのリンクは、ブラウザに渡さずその場でノートを切り替える。
            const notePath = resolveNotePath(href, entry);
            if (notePath !== null && href !== undefined) {
              const link = noteLink(href);
              const missing = missingLinks.has(link.toLowerCase());
              return (
                <a
                  href={href}
                  className={missing ? MISSING_LINK : INTERNAL_LINK}
                  title={missing ? `${notePath}（まだありません）` : notePath}
                  onClick={(e) => {
                    e.preventDefault();
                    e.stopPropagation();
                    onFollowLink(link, missing ? undefined : notePath);
                  }}
                >
                  {children}
                </a>
              );
            }
            // 外部リンクは OS の既定のブラウザで開く。
            // target="_blank" に任せると WebView2 が自前のウィンドウを開いてしまう。
            const external = href !== undefined && EXTERNAL_LINK_RE.test(href);
            return (
              <a
                href={href}
                rel="noreferrer"
                {...rest}
                className={external ? EXTERNAL_LINK : rest.className}
                // WebView にはステータスバーが無いので、行き先の URL はツールチップで見せる。
                title={rest.title ?? (external ? href : undefined)}
                onClick={(e) => {
                  e.preventDefault();
                  e.stopPropagation();
                  if (external) BrowserOpenURL(href);
                }}
              >
                {children}
                {external && <span className="ml-0.5 inline-block text-[0.8em]">↗</span>}
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
                onOpen={onOpenImage}
              />
            );
          },
        }}
      >
        {source}
      </ReactMarkdown>
    </div>
  );
}
