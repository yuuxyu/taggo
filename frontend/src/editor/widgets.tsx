/**
 * 編集画面で、Markdown の記述の代わりに見せる部品（ウィジェット）。
 *
 * カーソルが外れている間だけ、画像の記述は画像に、`- [ ]` はチェックボックスに、
 * URL だけの行はリンクカードに、表と mermaid のコードブロックは描いた結果に置き換える。
 * カーソルが入ると元の記述に戻るので、そのまま書き直せる。
 */

import type { ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import type { EditorView } from "@codemirror/view";
import { WidgetType } from "@codemirror/view";
import { BrowserOpenURL } from "../../wailsjs/runtime/runtime";
import { imageURL } from "../api/taggo";
import { LinkCard } from "../components/LinkCard";
import { MarkdownBlock } from "../components/MarkdownBlock";
import { Mermaid } from "../components/Mermaid";
import { hostOf, type EditorHost } from "./host";
import { PENDING_IMAGE_PREFIX } from "./pasteImage";

/** 本文中の画像。フォルダ内の画像はクリックで画像ビューアを開く。 */
export class ImageWidget extends WidgetType {
  /**
   * @param src 本文に書かれた画像の参照
   * @param localPath フォルダ内の画像なら、その絶対パス。外部 URL の画像なら null
   */
  constructor(
    readonly src: string,
    readonly alt: string,
    readonly localPath: string | null,
  ) {
    super();
  }

  eq(other: ImageWidget) {
    return other.src === this.src && other.alt === this.alt && other.localPath === this.localPath;
  }

  toDOM(view: EditorView) {
    const wrap = document.createElement("span");
    wrap.className = "inline-block max-w-full align-bottom";
    if (this.src.startsWith(PENDING_IMAGE_PREFIX)) {
      wrap.className = "inline-block rounded-md bg-sunken px-2 py-1 text-xs text-ink-muted";
      wrap.textContent = "画像を保存しています…";
      return wrap;
    }
    const img = document.createElement("img");
    img.src = this.localPath === null ? this.src : imageURL(this.localPath);
    img.alt = this.alt;
    img.loading = "lazy";
    img.className = "my-1 inline-block max-w-full rounded-md";
    if (this.localPath !== null) {
      img.title = "クリックで画像を開く";
      img.classList.add("cursor-zoom-in");
      const path = this.localPath;
      img.addEventListener("mousedown", (e) => {
        if (e.button !== 0) return;
        e.preventDefault();
        hostOf(view.state)?.onOpenImage(path, this.alt || undefined);
      });
    }
    // 読み込めて大きさが決まったら、行の高さを測り直させる。
    img.addEventListener("load", () => view.requestMeasure());
    img.addEventListener("error", () => {
      wrap.replaceChildren(
        Object.assign(document.createElement("span"), {
          className: "inline-block rounded-md bg-sunken px-2 py-1 text-xs text-ink-muted",
          textContent: `画像を表示できません: ${this.src}`,
        }),
      );
      view.requestMeasure();
    });
    wrap.append(img);
    return wrap;
  }
}

/** 箇条書きの記号（- * +）の代わりに出す黒丸。 */
export class BulletWidget extends WidgetType {
  eq() {
    return true;
  }

  toDOM() {
    const span = document.createElement("span");
    span.className = "inline-block w-[1ch] text-center text-ink-faint";
    span.textContent = "•";
    return span;
  }
}

/** タスクの `[ ]` / `[x]` の代わりに出すチェックボックス。クリックで本文の印を書き換える。 */
export class CheckboxWidget extends WidgetType {
  constructor(readonly checked: boolean) {
    super();
  }

  eq(other: CheckboxWidget) {
    return other.checked === this.checked;
  }

  toDOM() {
    const input = document.createElement("input");
    input.type = "checkbox";
    input.checked = this.checked;
    input.dataset.task = "1";
    input.className = "mr-1 size-[0.9em] cursor-pointer align-middle accent-accent";
    return input;
  }

  // クリックはエディタ側（livePreview の mousedown）で受けて本文を書き換える。
  ignoreEvent() {
    return false;
  }
}

/** コードブロックの開きのフェンス（```go）の代わりに、右端へ言語名だけを小さく出す。 */
export class CodeLabelWidget extends WidgetType {
  constructor(readonly lang: string) {
    super();
  }

  eq(other: CodeLabelWidget) {
    return other.lang === this.lang;
  }

  toDOM() {
    const span = document.createElement("span");
    span.className = "float-right font-sans text-xs text-ink-faint select-none";
    span.textContent = this.lang;
    return span;
  }
}

/** 既定のブラウザで開く外部リンクの後ろに添える「↗」。 */
export class ExternalMarkWidget extends WidgetType {
  eq() {
    return true;
  }

  toDOM() {
    const span = document.createElement("span");
    span.className = "ml-0.5 inline-block text-[0.8em] text-accent-ink";
    span.textContent = "↗";
    return span;
  }
}

/**
 * React の部品を描くウィジェット。リンクカードや表のように、既存の部品をそのまま使うときの土台。
 * 描いた部品はウィジェットが外されたときに片付ける。
 */
abstract class ReactWidget extends WidgetType {
  private static mounted = new WeakMap<HTMLElement, { root: Root; observer: ResizeObserver }>();

  /** 外枠の要素のクラス。 */
  protected abstract className: string;
  protected abstract render(view: EditorView, host: EditorHost): ReactNode;

  toDOM(view: EditorView) {
    const dom = document.createElement("div");
    dom.className = this.className;
    const host = hostOf(view.state);
    if (!host) return dom;
    const root = createRoot(dom);
    root.render(this.render(view, host));
    // 描き終えて大きさが決まったら（リンクカードや図は読み込みに時間がかかる）、高さを測り直させる。
    const observer = new ResizeObserver(() => view.requestMeasure());
    observer.observe(dom);
    ReactWidget.mounted.set(dom, { root, observer });
    return dom;
  }

  destroy(dom: HTMLElement) {
    const mounted = ReactWidget.mounted.get(dom);
    if (!mounted) return;
    ReactWidget.mounted.delete(dom);
    mounted.observer.disconnect();
    // エディタの描き直しの途中で React の描画を止めないよう、少し遅らせて片付ける。
    setTimeout(() => mounted.root.unmount());
  }

  ignoreEvent() {
    return true;
  }
}

/** ウィジェットの中をクリックしたときに、その記述の先頭へカーソルを置いて書き直せるようにする。 */
function editAt(view: EditorView, dom: Element) {
  const pos = view.posAtDOM(dom);
  view.dispatch({ selection: { anchor: pos } });
  view.focus();
}

/** URL だけを書いた行の代わりに出すリンクカード。カードの外側をクリックすると URL を書き直せる。 */
export class LinkCardWidget extends ReactWidget {
  protected className = "flow-root py-1";

  constructor(readonly url: string) {
    super();
  }

  eq(other: LinkCardWidget) {
    return other.url === this.url;
  }

  protected render(view: EditorView) {
    return (
      <div
        onMouseDown={(e) => {
          if ((e.target as Element).closest("a")) return;
          e.preventDefault();
          editAt(view, e.currentTarget);
        }}
      >
        <LinkCard
          url={this.url}
          fallback={
            <a
              href={this.url}
              className="text-accent-ink underline"
              onClick={(e) => {
                e.preventDefault();
                BrowserOpenURL(this.url);
              }}
            >
              {this.url}
              <span className="ml-0.5 inline-block text-[0.8em]">↗</span>
            </a>
          }
        />
      </div>
    );
  }
}

/** 表の代わりに出す、描いた表。クリックすると表の記述へ戻って書き直せる。 */
export class TableWidget extends ReactWidget {
  protected className = "flow-root py-2";

  constructor(
    readonly source: string,
    readonly stamp: number,
  ) {
    super();
  }

  eq(other: TableWidget) {
    return other.source === this.source && other.stamp === this.stamp;
  }

  protected render(view: EditorView, host: EditorHost) {
    return (
      <div
        className="cursor-text overflow-x-auto"
        title="クリックで表を書き直す"
        onMouseDown={(e) => {
          // 表の中のリンクと画像は、それぞれの行き先を開く。
          if ((e.target as Element).closest("a, img")) return;
          e.preventDefault();
          editAt(view, e.currentTarget);
        }}
      >
        <MarkdownBlock
          source={this.source}
          entry={host.entry}
          onFollowLink={(link, path) => hostOf(view.state)?.onFollowLink(link, path)}
          missingLinks={host.missingLinks}
          onFollowTag={(tag) => hostOf(view.state)?.onFollowTag(tag)}
          missingTags={host.missingTags}
          onOpenImage={(path, alt) => hostOf(view.state)?.onOpenImage(path, alt)}
        />
      </div>
    );
  }
}

/** mermaid のコードブロックの代わりに出す図。クリックするとコードへ戻って書き直せる。 */
export class MermaidWidget extends ReactWidget {
  protected className = "flow-root py-1";

  constructor(readonly source: string) {
    super();
  }

  eq(other: MermaidWidget) {
    return other.source === this.source;
  }

  protected render(view: EditorView) {
    return (
      <div
        className="cursor-text"
        title="クリックで図のコードを書き直す"
        onMouseDown={(e) => {
          e.preventDefault();
          editAt(view, e.currentTarget);
        }}
      >
        <Mermaid source={this.source} />
      </div>
    );
  }
}
