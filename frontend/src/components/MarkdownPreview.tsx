/**
 * Markdown のプレビュー。
 *
 * CommonMark / GFM に準拠してレンダリングし、コードハイライト・テーブル・mermaid を扱う。
 * URL だけを書いた段落は、リンク先の OGP をもとにしたリンクカードにする。
 * Front Matter はバックエンド側で本文から切り離されているため、ここには届かない。
 *
 * 本文は react-markdown が生成する素の HTML なので、装飾は Tailwind の
 * typography プラグイン（prose）に任せ、配色だけをアプリのトークンへ差し替える。
 */

import { isValidElement, useEffect, useRef, useState, type ReactNode } from "react";
import ReactMarkdown, { type ExtraProps } from "react-markdown";
import remarkGfm from "remark-gfm";
import rehypeHighlight from "rehype-highlight";
import { BrowserOpenURL } from "../../wailsjs/runtime/runtime";
import { fileURL, getMarkdownSource, type Entry } from "../api/taggo";
import { LinkCard } from "./LinkCard";
import { Mermaid } from "./Mermaid";
import "highlight.js/styles/github.css";

interface Props {
  entry: Entry;
  /** ノートへのリンクをたどるときに呼ぶ。行き先が一覧に無ければ検索に落とす。 */
  onFollowLink: (target: string, path?: string) => void;
  /** 本文中の画像をクリックしたときに、その画像のパスを渡して呼ぶ。 */
  onOpenImage: (path: string) => void;
  /**
   * 本文を読み込んで描画し終えたときに呼ぶ。
   * 戻ってきたときのスクロール位置の復元は、本文の高さが決まってからでないとできない。
   */
  onLoaded?: () => void;
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
  // prose-lg の h1 は本文の 2.67 倍と大きすぎるので、ブラウザ標準の h1 と同じ 2 倍に抑える。
  "prose-h1:text-[2em]",
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
 * 本文に書かれた相対パスを、ファイルシステム上の絶対パスへ直す。
 *
 * ブラウザは相対 URL をアプリのページ基準で解決してしまうので、そのままでは
 * ノートの隣に置いたファイルを指せない。Markdown ファイルのあるフォルダを
 * 基準に絶対パスへ組み立て直す。"/" 始まりは、ノートからの相対ではなく
 * 開いているフォルダ基準として扱う。
 *
 * http(s): や data: などスキーム付きの URL は外部を指しているので null を返す。
 * Windows のドライブ文字（C:\… や C:/…）はスキームに見えるがパスなので通す。
 */
function resolveLocalPath(target: string, entry: Entry): string | null {
  const driveLetter = /^[a-z]:[\\/]/i.test(target);
  if (!driveLetter && /^[a-z][a-z0-9+.-]*:/i.test(target)) return null;

  // 末尾のフラグメント（#…）はパスの一部ではないので落とす。
  const cleaned = target.replace(/#.*$/, "");
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
  return segments.join(sep);
}

/** 既定のブラウザへ渡してよいリンク。ローカルファイルや任意のスキームは OS に開かせない。 */
const EXTERNAL_LINK_RE = /^(https?|mailto):/i;

/**
 * ノートへのリンクなら、その行き先の絶対パスを返す。
 * 対象は .md / .markdown を指す相対・絶対パスのリンクだけで、
 * 外部 URL や画像・その他のファイルへのリンクは対象にしない。
 */
function resolveNotePath(href: string | undefined, entry: Entry): string | null {
  if (!href || !/\.(md|markdown)(#.*)?$/i.test(href)) return null;
  return resolveLocalPath(href, entry);
}

/**
 * 本文中の画像。開いているフォルダの外や、taggo が取り込んでいない形式は
 * 配信されないため、読み込めなかったときは元の記述を添えて理由を示す。
 *
 * 本文から参照された画像は、taggo の配信 URL へ書き換えて読み込む。
 * フォルダ内の画像はクリックでその画像のプレビューへ移れる。外部 URL の画像は
 * taggo の一覧に無いので、クリックしても何もしない。
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
  onOpen: (path: string) => void;
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
      src={fileURL(localPath)}
      alt={alt ?? ""}
      title={title ?? "クリックで画像を開く"}
      loading="lazy"
      role="button"
      tabIndex={0}
      className="cursor-zoom-in"
      onError={() => setFailed(true)}
      onClick={(e) => {
        // リンクで囲まれた画像は、リンクの行き先のほうを優先する。
        if (e.currentTarget.closest("a")) return;
        onOpen(localPath);
      }}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          onOpen(localPath);
        }
      }}
    />
  );
}

/** 壊れたエスケープで例外にならない decodeURI。 */
function safeDecodeURI(s: string): string {
  try {
    return decodeURI(s);
  } catch {
    return s;
  }
}

/**
 * 段落が URL 1 つだけでできていれば、その URL を返す。リンクカードにする段落を見分ける。
 *
 * `https://…` をそのまま書いたもの（GFM の自動リンク）と `<https://…>` が対象で、
 * 文中のリンクや、表示名を付けた `[名前](https://…)` はリンクのまま残す。
 * GFM は日本語などを含む URL の href をエスケープし、`www.` 始まりには http:// を
 * 補うので、表示されている文字と href はそれを踏まえて比べる。
 */
function bareURL(node: ExtraProps["node"]): string | null {
  if (!node) return null;
  const children = node.children.filter((c) => !(c.type === "text" && c.value.trim() === ""));
  if (children.length !== 1) return null;
  const link = children[0];
  if (link.type !== "element" || link.tagName !== "a") return null;

  const href = link.properties.href;
  if (typeof href !== "string" || !/^https?:\/\//i.test(href)) return null;
  if (link.children.length !== 1 || link.children[0].type !== "text") return null;

  const text = link.children[0].value;
  const target = safeDecodeURI(href);
  return text === href || text === target || `http://${text}` === target ? href : null;
}

/** リンクの行き先が一覧に無いときに、検索へ落とすための言葉。拡張子なしのファイル名。 */
function noteLabel(href: string): string {
  const path = href.replace(/#.*$/, "");
  const name = path.slice(Math.max(path.lastIndexOf("/"), path.lastIndexOf("\\")) + 1);
  return name.replace(/\.(md|markdown)$/i, "");
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

export function MarkdownPreview({ entry, onFollowLink, onOpenImage, onLoaded }: Props) {
  const [source, setSource] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  // 最後に本文を読み込んだファイル。同じファイルの読み直しかどうかを見分ける。
  const loadedPath = useRef<string | null>(null);

  // 別のエディタで保存されると、フォルダ監視が更新日時とサイズの変わったエントリを
  // 届けてくる。それを合図に本文を読み直し、プレビューを最新の内容へ追従させる。
  const version = `${String(entry.modTime)}:${entry.size}`;

  useEffect(() => {
    let cancelled = false;
    // 同じファイルの読み直しでは、読み込み中の表示へ切り替えない。
    // 本文が一瞬消えてスクロール位置が先頭へ戻ってしまうのを避けるため。
    const reload = loadedPath.current === entry.path;
    loadedPath.current = entry.path;
    if (!reload) {
      setSource(null);
      setError(null);
    }

    void getMarkdownSource(entry.path)
      .then((text) => {
        if (cancelled) return;
        setSource(text);
        setError(null);
      })
      .catch((err) => {
        if (!cancelled) setError(String(err));
      });

    return () => {
      cancelled = true;
    };
  }, [entry.path, version]);

  // 描画し終えてから知らせる。effect はコミット後に走るので、本文の高さは決まっている。
  useEffect(() => {
    if (source !== null) onLoaded?.();
    // 本文が変わったときだけ知らせればよい。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [source]);

  if (error) {
    return <div className="py-6 text-danger">本文を読み込めませんでした: {error}</div>;
  }
  if (source === null) {
    return <div className="py-6 text-ink-muted">読み込み中…</div>;
  }

  return (
    <div
      // 全角文字は 1 文字がちょうど 1em なので、幅を 38em にすると 1 行に全角 38 文字が入る。
      // 文字の大きさを変えても 1 行の文字数が保たれるよう、幅は em で決める。
      className={`prose prose-lg max-w-[38em] break-words ${PROSE_COLORS} ${PROSE_TWEAKS} ${HLJS_DARK}`}
    >
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        rehypePlugins={[rehypeHighlight]}
        components={{
          p({ node, children, ...rest }) {
            const url = bareURL(node);
            const paragraph = <p {...rest}>{children}</p>;
            // 取得できなかったときは、ただのリンクの段落として出す。
            return url === null ? paragraph : <LinkCard url={url} fallback={paragraph} />;
          },
          a({ href, children, ...rest }) {
            // 他のノートへのリンクは、ブラウザに渡さずその場でプレビューを切り替える。
            const notePath = resolveNotePath(href, entry);
            if (notePath !== null && href !== undefined) {
              return (
                <a
                  href={href}
                  title={notePath}
                  onClick={(e) => {
                    e.preventDefault();
                    onFollowLink(noteLabel(href), notePath);
                  }}
                >
                  {children}
                </a>
              );
            }
            // 外部リンクは OS の既定のブラウザ（起動中ならその新しいタブ）で開く。
            // target="_blank" に任せると WebView2 が自前のウィンドウを開いてしまう。
            return (
              <a
                href={href}
                rel="noreferrer"
                {...rest}
                onClick={(e) => {
                  e.preventDefault();
                  if (href !== undefined && EXTERNAL_LINK_RE.test(href)) BrowserOpenURL(href);
                }}
              >
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
                onOpen={onOpenImage}
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
        {source}
      </ReactMarkdown>
    </div>
  );
}
