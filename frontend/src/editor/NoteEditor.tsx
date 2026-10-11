/**
 * ノートの編集画面。常に編集できる状態で開き、テキストエディタと見た目を分けない。
 *
 * カーソルが無いとき（一覧から開いた直後など）は、Enter / Ctrl + M / Ctrl + N で本文の先頭から書き始められる。
 *
 * 本文は Markdown のまま CodeMirror で編集し、見た目はライブプレビュー（livePreview.ts）が整える。
 * 書いた内容は、入力が止まって少し経ったら自動で保存する。ほかのノートへ移るとき・エディタから
 * フォーカスが外れたとき・ウィンドウを閉じるとき・名前を変える前にも、待たずに保存する。
 *
 * 保存の前に、読んだときからファイルがほかのアプリで書き換えられていないかを確かめる（Go 側）。
 * 書き換えられていれば保存せず、ほかでの変更を読み込むか、この内容で上書きするかを選んでもらう。
 * 書きかけでないときにほかで書き換えられたら、その内容をそのまま読み込む。
 */

import { useEffect, useRef, useState } from "react";
import { closeBrackets, closeBracketsKeymap } from "@codemirror/autocomplete";
import { defaultKeymap, history, historyKeymap, indentWithTab } from "@codemirror/commands";
import { Annotation, EditorState, Transaction, type Extension } from "@codemirror/state";
import { EditorView, keymap, placeholder } from "@codemirror/view";
import { readNote, saveNote, type Entry } from "../api/taggo";
import { Button } from "../components/Button";
import { completions } from "./completion";
import {
  focusedField,
  focusTracker,
  frontMatterField,
  hostFacet,
  refreshDecorations,
  type EditorHost,
  type HostRef,
} from "./host";
import { emacsKeys } from "./emacsKeys";
import { livePreview } from "./livePreview";
import { markupHelpers } from "./markup";
import { imagePaste, PENDING_IMAGE_RE } from "./pasteImage";
import { markdownSyntax } from "./syntax";
import { editorTheme } from "./theme";

/** 画面全体の「戻る／進む」（App）に使うので、編集画面では割り当てないキー。 */
const HISTORY_KEYS = new Set(["Alt-ArrowLeft", "Alt-ArrowRight", "Mod-[", "Mod-]"]);

/** Enter / Ctrl + M を、本文へカーソルを置くのではなく、それ自身の働きに使う要素。 */
const INTERACTIVE =
  "input, textarea, select, button, a[href], [contenteditable], [role='button'], [role='menuitem'], [role='checkbox']";

/** 入力が止まってから自動で保存するまでの時間。 */
const AUTOSAVE_MS = 1000;
/**
 * 保存に失敗したとき（同期サービスがファイルを使っていたなど）に、もう一度保存するまでの時間と回数。
 * 同じ知らせを何度も出さないよう、知らせるのは最初の失敗だけにする。
 */
const SAVE_RETRY_MS = 3000;
const SAVE_RETRY_MAX = 5;
/** 文字数を数え直すまでの時間。長いノートで入力のたびに数えないよう、少し待つ。 */
const COUNT_MS = 300;

interface Props {
  entry: Entry;
  /**
   * ノートへのリンクをたどる。link は書かれたパスをデコードしたもの。
   * path を省くと、行き先がまだ無いノートとして（ファイルは作らずに）開く。
   */
  onFollowLink: (link: string, path?: string) => void;
  /** `[[タグ]]` のページを開く。まだ無ければ、まだ無いページとして開く。 */
  onFollowTag: (tag: string) => void;
  /** 行き先がまだ無いリンク（小文字にしたもの）。本文では色を変えて示す。 */
  missingLinks: ReadonlySet<string>;
  /** ページがまだ無いタグ（小文字にしたもの）。本文の `[[タグ]]` の色を変えて示す。 */
  missingTags: ReadonlySet<string>;
  /** 本文中の画像を画像ビューアで開く。 */
  onOpenImage: (path: string, alt?: string) => void;
  onError: (message: string) => void;
  /** 本文を読み込んで表示し終えたときに呼ぶ。戻ってきたときのスクロール位置の復元に使う。 */
  onLoaded?: () => void;
  /** 本文（Front Matter を除く）が変わるたびに、少し間を置いて呼ぶ。文字数の表示に使う。 */
  onBodyChange?: (body: string) => void;
}

/** 開いているノート 1 つぶんの、保存のための状態。ノートを読み込むたびに作り直す。 */
interface OpenNote {
  path: string;
  /** 最後に読んだ（保存した）ときのファイルの中身を表す値。保存のときに Go 側で照らし合わせる。 */
  baseVersion: string;
  /** 最後に読んだ（保存した）本文。これと違えば書きかけ。改行は \n にそろえてある。 */
  savedDoc: string;
  /** ファイルの改行。保存するときはこれに戻す。 */
  eol: "\n" | "\r\n";
  /** 最後に把握したエントリの更新日時とサイズ。これと違うエントリが届いたら、ほかでの変更を疑う。 */
  knownKey: string;
  /** 保存の途中か。途中に届いた変更の知らせは、保存が終わってから確かめる。 */
  saving: number;
  recheck: boolean;
  /** ほかのアプリでの変更とぶつかっているか。解決するまで自動では保存しない。 */
  conflict: boolean;
  /** 続けて保存に失敗した回数。失敗を知らせるのは最初の 1 回だけにし、決まった回数まではやり直す。 */
  failures: number;
}

/** ディスクから読み込んだ内容で本文を差し替える変更。保存のきっかけにしない。 */
const fromDisk = Annotation.define<boolean>();

/** 書きかけのノートを保存する関数。ウィンドウを閉じる前や名前を変える前に、まとめて呼ぶ。 */
const flushers = new Set<() => Promise<void>>();

/** 開いている編集画面の書きかけを、すべて保存し終えるまで待つ。 */
export async function flushAllEditors(): Promise<void> {
  await Promise.all([...flushers].map((flush) => flush()));
}

/** 次に開いたときにカーソルを置くノート（作ったばかりのノートなど）。 */
const focusRequests = new Set<string>();

/** path のノートを次に開いたとき、本文の末尾にカーソルを置いて書き始められるようにする。 */
export function requestEditorFocus(path: string) {
  focusRequests.add(path.toLowerCase());
}

function entryKey(entry: Pick<Entry, "modTime" | "size"> | undefined): string {
  return entry ? `${String(entry.modTime)}:${entry.size}` : "";
}

/** パスのファイル名の部分。 */
function fileName(path: string): string {
  return path.split(/[\\/]/).pop() ?? path;
}

/** CodeMirror と同じく、改行を \n にそろえる。 */
function normalizeEol(text: string): string {
  return text.replace(/\r\n?/g, "\n");
}

/** 本文を next に差し替える。変わったところだけを置き換えて、カーソルの位置をなるべく保つ。 */
function replaceDoc(view: EditorView, next: string) {
  const cur = view.state.doc.toString();
  let start = 0;
  while (start < cur.length && start < next.length && cur[start] === next[start]) start++;
  let endCur = cur.length;
  let endNext = next.length;
  while (endCur > start && endNext > start && cur[endCur - 1] === next[endNext - 1]) {
    endCur--;
    endNext--;
  }
  view.dispatch({
    changes: { from: start, to: endCur, insert: next.slice(start, endNext) },
    annotations: [fromDisk.of(true), Transaction.addToHistory.of(false)],
  });
}

/** 本文から Front Matter を除いた部分。 */
function bodyOf(state: EditorState): string {
  return state.doc.sliceString(state.field(frontMatterField));
}

export function NoteEditor({
  entry,
  onFollowLink,
  onFollowTag,
  missingLinks,
  missingTags,
  onOpenImage,
  onError,
  onLoaded,
  onBodyChange,
}: Props) {
  const parentRef = useRef<HTMLDivElement>(null);
  const viewRef = useRef<EditorView | null>(null);
  const noteRef = useRef<OpenNote | null>(null);
  const saveTimer = useRef<number | undefined>(undefined);
  const countTimer = useRef<number | undefined>(undefined);
  const [status, setStatus] = useState<"loading" | "ready" | "error">("loading");
  const [loadError, setLoadError] = useState<string | null>(null);
  const [conflict, setConflict] = useState(false);
  // 読み込んだあと、表示されてから本文の末尾にカーソルを置くか。
  const focusPending = useRef(false);

  // 画面側の受け口。CodeMirror の拡張は作り直さないので、描くたびに最新へ差し替える。
  const stamp = useRef(0);
  const hostRef = useRef<HostRef | null>(null);
  const host: EditorHost = {
    entry,
    onFollowLink,
    onFollowTag,
    onOpenImage,
    onError,
    missingLinks,
    missingTags,
    stamp: stamp.current,
  };
  if (hostRef.current) hostRef.current.current = host;
  else hostRef.current = { current: host };

  // 呼び出し側から渡される関数は描くたびに変わるので、最新のものを ref から呼ぶ。
  const callbacks = useRef({ onError, onLoaded, onBodyChange });
  callbacks.current = { onError, onLoaded, onBodyChange };

  /**
   * note の本文 text を保存する。force なら、ほかでの変更とぶつかっていても上書きする。
   * 保存は 1 つずつ順に行う（前の保存の結果の version を次の保存で使うため）。
   */
  const saveChain = useRef<Promise<void>>(Promise.resolve());
  const save = (note: OpenNote, text: string, force = false): Promise<void> => {
    const run = async () => {
      if (note.conflict && !force) return;
      if (text === note.savedDoc && !force) return;
      // 保存を待っている画像の仮の記述は、ファイルには書かない。
      const content = text.replace(PENDING_IMAGE_RE, "").replace(/\n/g, note.eol);
      note.saving++;
      try {
        const res = await saveNote(note.path, content, force ? "" : note.baseVersion);
        if (res.conflict) {
          note.conflict = true;
          if (noteRef.current === note) setConflict(true);
          return;
        }
        note.baseVersion = res.version;
        note.savedDoc = text;
        note.knownKey = entryKey(res.entry);
        note.failures = 0;
        if (force) {
          note.conflict = false;
          if (noteRef.current === note) setConflict(false);
        }
      } catch (err) {
        if (note.failures === 0) {
          callbacks.current.onError(
            `「${fileName(note.path)}」を保存できませんでした。少し後にもう一度保存します: ${String(err)}`,
          );
        }
        note.failures++;
        if (note.failures <= SAVE_RETRY_MAX) {
          window.setTimeout(() => {
            // 開いたままなら最新の本文で、ほかのノートへ移っていれば失敗したときの本文で保存し直す。
            if (noteRef.current === note) void flushRef.current();
            else void save(note, text);
          }, SAVE_RETRY_MS);
        }
      } finally {
        note.saving--;
      }
      if (note.saving === 0 && note.recheck) {
        note.recheck = false;
        void checkDisk(note);
      }
    };
    saveChain.current = saveChain.current.then(run, run);
    return saveChain.current;
  };

  /** 書きかけがあれば、待たずに保存する。 */
  const flush = (): Promise<void> => {
    window.clearTimeout(saveTimer.current);
    const note = noteRef.current;
    const view = viewRef.current;
    if (!note || !view) return saveChain.current;
    return save(note, view.state.doc.toString());
  };
  const flushRef = useRef(flush);
  flushRef.current = flush;

  /**
   * ファイルがほかのアプリで書き換えられていないかを確かめる。書き換えられていて、
   * 書きかけが無ければ読み込み、書きかけがあれば、ぶつかっていることを知らせる。
   */
  const checkDisk = async (note: OpenNote) => {
    if (note.saving > 0) {
      note.recheck = true;
      return;
    }
    let disk;
    try {
      disk = await readNote(note.path);
    } catch {
      return; // 消えたなら、一覧の側が閉じる
    }
    const view = viewRef.current;
    if (noteRef.current !== note || !view || disk.version === note.baseVersion) return;
    const dirty = view.state.doc.toString() !== note.savedDoc;
    if (dirty) {
      note.conflict = true;
      setConflict(true);
      return;
    }
    const text = normalizeEol(disk.content);
    note.baseVersion = disk.version;
    note.savedDoc = text;
    replaceDoc(view, text);
  };

  // エディタを 1 つだけ作り、ノートを開くたびに中身（EditorState）を差し替えて使い回す。
  const extensions = useRef<Extension[] | null>(null);
  if (!extensions.current) {
    extensions.current = [
      hostFacet.of(hostRef.current),
      focusedField,
      focusTracker,
      frontMatterField,
      markdownSyntax(),
      history(),
      markupHelpers(),
      // 括弧の対を補う。`[[` と打てば `[[]]` になり、タグの補完が出る。
      // 文章の中で邪魔になる引用符は補わない。
      closeBrackets(),
      EditorState.languageData.of(() => [{ closeBrackets: { brackets: ["(", "[", "{"] } }]),
      completions(),
      keymap.of([
        ...closeBracketsKeymap,
        // Alt + 左右と Ctrl + [ / ]（既定では字下げ）は、画面全体の「戻る／進む」に使う。
        ...defaultKeymap.filter((b) => !HISTORY_KEYS.has(b.key ?? "")),
        ...historyKeymap,
        indentWithTab,
      ]),
      EditorView.lineWrapping,
      placeholder("ここに本文を書きます。[[ でタグ、## で見出し、``` でコード"),
      EditorView.contentAttributes.of({ spellcheck: "false", "aria-label": "本文" }),
      livePreview(),
      // 上下の移動で表やリンクカードの行に入る割り当て（livePreview）を先に効かせるため、その後ろに置く。
      emacsKeys(),
      imagePaste(),
      editorTheme(),
      EditorView.updateListener.of((u) => {
        if (!u.docChanged) return;
        if (!u.transactions.some((tr) => tr.annotation(fromDisk))) {
          window.clearTimeout(saveTimer.current);
          saveTimer.current = window.setTimeout(() => void flushRef.current(), AUTOSAVE_MS);
        }
        window.clearTimeout(countTimer.current);
        countTimer.current = window.setTimeout(
          () => callbacks.current.onBodyChange?.(bodyOf(u.view.state)),
          COUNT_MS,
        );
      }),
      // フォーカスが外れたら（ウィンドウを切り替えたときも）、待たずに保存する。
      EditorView.domEventHandlers({
        blur: () => {
          void flushRef.current();
        },
      }),
    ];
  }

  useEffect(() => {
    const view = new EditorView({
      parent: parentRef.current!,
      state: EditorState.create({ extensions: [...extensions.current!, EditorState.readOnly.of(true)] }),
    });
    viewRef.current = view;
    const flusher = () => flushRef.current();
    flushers.add(flusher);
    return () => {
      flushers.delete(flusher);
      view.destroy();
      viewRef.current = null;
    };
  }, []);

  // ノートを開いたら中身を読み込む。離れるときは書きかけを保存する。
  useEffect(() => {
    const view = viewRef.current;
    if (!view) return;
    let cancelled = false;
    setStatus("loading");
    setLoadError(null);
    setConflict(false);
    noteRef.current = null;
    // 読み込むまでは、前のノートの中身を書き換えられないよう空にしておく。
    view.setState(EditorState.create({ extensions: [...extensions.current!, EditorState.readOnly.of(true)] }));

    void readNote(entry.path).then(
      (note) => {
        if (cancelled) return;
        const text = normalizeEol(note.content);
        noteRef.current = {
          path: entry.path,
          baseVersion: note.version,
          savedDoc: text,
          eol: note.content.includes("\r\n") ? "\r\n" : "\n",
          knownKey: entryKey(entry),
          saving: 0,
          recheck: false,
          conflict: false,
          failures: 0,
        };
        const wantFocus = focusRequests.delete(entry.path.toLowerCase());
        focusPending.current = wantFocus;
        view.setState(
          EditorState.create({
            doc: text,
            extensions: extensions.current!,
            selection: wantFocus ? { anchor: text.length } : undefined,
          }),
        );
        setStatus("ready");
        callbacks.current.onBodyChange?.(bodyOf(view.state));
      },
      (err) => {
        if (cancelled) return;
        setStatus("error");
        setLoadError(String(err));
      },
    );

    return () => {
      cancelled = true;
      window.clearTimeout(countTimer.current);
      const note = noteRef.current;
      if (note?.conflict && view.state.doc.toString() !== note.savedDoc) {
        callbacks.current.onError(
          `ほかのアプリでの変更とぶつかっていたため、「${fileName(note.path)}」の書きかけは保存しませんでした。`,
        );
      }
      // 離れる前の中身で保存する（このあと中身は次のノートに差し替わる）。
      void flushRef.current();
    };
    // 開いているノートが変わったときだけ読み直す。ほかでの変更は下の effect で拾う。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [entry.path]);

  // 表示されて行の高さが測られてから、スクロール位置を戻させ、カーソルを置く。
  useEffect(() => {
    if (status === "loading") return;
    let frame = requestAnimationFrame(() => {
      viewRef.current?.requestMeasure();
      frame = requestAnimationFrame(() => {
        callbacks.current.onLoaded?.();
        if (focusPending.current) {
          focusPending.current = false;
          viewRef.current?.focus();
        }
      });
    });
    return () => cancelAnimationFrame(frame);
  }, [status]);

  // カーソルが無いとき（一覧から開いた直後など）に Enter / Ctrl + M / Ctrl + N を押すと、本文の
  // 先頭にカーソルを置いて書き始められるようにする。ほかの画面（画像ビューア・ダイアログ・履歴の
  // 一覧）が重なっているときは、そちらの働きを優先する。Enter / Ctrl + M は、ボタンなどに
  // フォーカスがあるときもそちらを優先する（Ctrl + N はボタンの働きとぶつからないので本文へ移す）。
  useEffect(() => {
    if (status !== "ready") return;
    const onKeyDown = (e: KeyboardEvent) => {
      const view = viewRef.current;
      if (!view || view.hasFocus || e.defaultPrevented || e.isComposing) return;
      const enter = e.key === "Enter" && !e.ctrlKey && !e.altKey && !e.shiftKey && !e.metaKey;
      const ctrl = e.ctrlKey && !e.altKey && !e.shiftKey && !e.metaKey;
      const ctrlM = ctrl && e.key.toLowerCase() === "m";
      const ctrlN = ctrl && e.key.toLowerCase() === "n";
      if (!enter && !ctrlM && !ctrlN) return;
      // Ctrl + N は、WebView の既定の働き（新しいウィンドウ）をいつも止める。
      if (ctrlN) e.preventDefault();
      // 一覧から開いた直後は、後ろの一覧のカードにフォーカスが残っている。ノートの画面の
      // 外にあるフォーカスは、無いものとして扱う。
      const screen = view.dom.closest('[role="dialog"]');
      const active = document.activeElement;
      if (!ctrlN && active instanceof Element && screen?.contains(active) && active.closest(INTERACTIVE)) return;
      if (document.querySelectorAll('[role="dialog"][aria-modal="true"]').length > 1) return;
      if (document.querySelector('[role="menu"]')) return;
      e.preventDefault();
      view.dispatch({ selection: { anchor: 0 }, scrollIntoView: true });
      view.focus();
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [status]);

  // ほかのアプリでの保存は、フォルダの監視が更新日時とサイズの変わったエントリとして届けてくる。
  // 自分の保存で届いたものは、保存の結果で覚えた値と同じなので読み直さない。
  const key = entryKey(entry);
  useEffect(() => {
    const note = noteRef.current;
    if (!note || note.path !== entry.path || note.knownKey === key) return;
    note.knownKey = key;
    void checkDisk(note);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [entry.path, key]);

  // 行き先の無いリンクやタグが変わったら、色を付け直す。
  useEffect(() => {
    stamp.current++;
    if (hostRef.current) hostRef.current.current = { ...hostRef.current.current, stamp: stamp.current };
    viewRef.current?.dispatch({ effects: refreshDecorations.of(null) });
  }, [missingLinks, missingTags]);

  /** ほかでの変更を読み込み、書きかけを捨てる。 */
  const reloadFromDisk = async () => {
    const note = noteRef.current;
    const view = viewRef.current;
    if (!note || !view) return;
    try {
      const disk = await readNote(note.path);
      const text = normalizeEol(disk.content);
      note.baseVersion = disk.version;
      note.savedDoc = text;
      note.conflict = false;
      setConflict(false);
      replaceDoc(view, text);
    } catch (err) {
      onError(String(err));
    }
  };

  return (
    <div>
      {conflict && (
        <div
          role="alert"
          className="mb-4 flex flex-wrap items-center gap-3 rounded-lg bg-danger-soft px-4 py-3 text-sm text-danger"
        >
          <span className="min-w-0 flex-1">
            ほかのアプリでこのノートが書き換えられたため、保存を止めています。どちらの内容を残すか選んでください。
          </span>
          <Button variant="ghost" onClick={() => void reloadFromDisk()}>
            ほかでの変更を読み込む
          </Button>
          <Button
            variant="primary"
            onClick={() => {
              const note = noteRef.current;
              const view = viewRef.current;
              if (note && view) void save(note, view.state.doc.toString(), true);
            }}
          >
            この内容で上書き
          </Button>
        </div>
      )}
      {status === "loading" && <div className="py-6 text-ink-muted">読み込み中…</div>}
      {status === "error" && <div className="py-6 text-danger">本文を読み込めませんでした: {loadError}</div>}
      <div ref={parentRef} className={status === "ready" ? "" : "hidden"} />
    </div>
  );
}
