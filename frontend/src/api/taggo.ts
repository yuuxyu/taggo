/**
 * Wails が生成したバインディングを、アプリ側から扱いやすい形に包む層。
 *
 * 生成コードを直接あちこちから呼ぶと、バインディングの形が変わったときに
 * 影響範囲が広がるため、入口をここ 1 か所に集約している。
 */

import * as Backend from "../../wailsjs/go/app/App";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import type { app, linkcard, model, store } from "../../wailsjs/go/models";

export type Entry = model.Entry;
export type Status = app.Status;
export type RelatedPage = store.RelatedPage;
export type Related = store.Related;
export type RelatedGroup = store.RelatedGroup;
export type SearchResult = store.Result;
/** リンクカードに出す内容。kind は "page" / "youtube" / "x" のいずれか。 */
export type LinkPreview = linkcard.Preview;

/** 一覧の並び順。Go 側の store.SortOrder と対応する。 */
export type SortOrder = "modified_desc" | "name_asc" | "relevance";

/** 起動時にどのフォルダを開くか。Go 側の settings.StartupMode と対応する。 */
export type StartupMode = "none" | "last" | "fixed";

/** 画面の配色。Go 側の settings.Theme と対応する。 */
export type Theme = "system" | "light" | "dark";

/**
 * アプリの設定（%APPDATA%\taggo\settings.json）。
 * 生成された型は値を string のまま持つので、取りうる値に絞った形で扱う。
 */
export interface Settings {
  version: number;
  startupMode: StartupMode;
  /** startupMode が "fixed" のときに開くフォルダ。 */
  startupFolder?: string;
  /** startupMode が "last" のときに開く、前回開いたフォルダ。Go 側が記録する。 */
  lastFolder?: string;
  /** 起動時の一覧の並び順。 */
  sort: SortOrder;
  /** 1 回の走査で展開する件数の上限。 */
  scanLimit: number;
  theme: Theme;
}

/** 走査の上限件数として選べる範囲。Go 側の settings.MinScanLimit / MaxScanLimit と合わせる。 */
export const SCAN_LIMIT_MIN = 1_000;
export const SCAN_LIMIT_MAX = 100_000;

/** 設定を読めなかったときの設定。Go 側の settings.Default と合わせる。 */
export const DEFAULT_SETTINGS: Settings = {
  version: 1,
  startupMode: "none",
  sort: "modified_desc",
  scanLimit: 20_000,
  theme: "system",
};

/** 走査の進捗イベントのペイロード。 */
export interface ScanProgress {
  done: number;
  found: number;
  /** 続きの読み込み（loadMore）の進捗かどうか。 */
  loadingMore?: boolean;
}

/** 走査完了イベントのペイロード。エラーや警告だけが届く場合もある。 */
export interface ScanDone {
  root?: string;
  entryCount?: number;
  tagCount?: number;
  maxEntries?: number;
  /** 上限で打ち切ったため、まだ読み込んでいない Markdown ファイルの数。 */
  remaining?: number;
  /** 続きの読み込み（loadMore）の完了かどうか。 */
  loadedMore?: boolean;
  /** 今回の読み込みで一覧に加わった件数。 */
  added?: number;
  /** 利用者の操作で続きの読み込みを取りやめたかどうか。 */
  cancelled?: boolean;
  error?: string;
  warning?: string;
}

/** ピン留めの変更イベントのペイロード。 */
export interface PinsChanged {
  /** 設定ファイル（.taggo.json）を読めなかったときの理由。 */
  warning?: string;
}

/** ファイル変更イベントのペイロード。 */
export interface EntryChanged {
  path: string;
  entry?: Entry;
  removed?: boolean;
}

export const Events = {
  scanProgress: "scan:progress",
  scanDone: "scan:done",
  entryChanged: "entry:changed",
  pinsChanged: "pins:changed",
} as const;

/**
 * フォルダ選択ダイアログを開く。キャンセル時は空文字が返る。
 * 選ぶだけで、読み込みは openFolder を呼ぶまで始まらない。
 */
export const selectFolder = (): Promise<string> => Backend.SelectFolder();

/**
 * クラウド上にだけあるファイルを取り込む。
 * この呼び出しで実際にダウンロードが発生するので、利用者の操作からのみ呼ぶこと。
 */
export const fetchCloudEntry = (path: string): Promise<Entry> => Backend.FetchCloudEntry(path);

/** 指定フォルダを走査して読み込む。走査自体は非同期に進む。 */
export const openFolder = (path: string): Promise<void> => Backend.OpenFolder(path);

/**
 * 上限で打ち切ったフォルダの続きを読み込む。all なら残りをすべて、
 * そうでなければ上限と同じ件数だけ読む。読み込み自体は非同期に進む。
 */
export const loadMore = (all: boolean): Promise<void> => Backend.LoadMore(all);

/** 続きの読み込みを取りやめる。読み込み途中の分は捨てられる。 */
export const cancelLoadMore = (): Promise<void> => Backend.CancelLoadMore();

/** 現在の読み込み状況を取得する。 */
export const getStatus = (): Promise<Status> => Backend.Status();

/**
 * 検索バーの入力でエントリを絞り込む。空白で区切った語をすべて含むものが結果になる。
 * 先頭の head 件は別枠（検索語が無ければピン留め、あれば検索語と同じ名前のタグのページ）。
 */
export const search = (query: string, sort: SortOrder): Promise<SearchResult> =>
  Backend.Search({ query, sort, offset: 0, limit: 0 });

/** 1 件のエントリを取得する。 */
export const getEntry = (path: string): Promise<Entry> => Backend.Entry(path);

/**
 * from のノートに書かれた Markdown のリンク link の行き先のページを取得する。
 * link は本文に書かれたパスを、フラグメントを除いてデコードしたもの。
 * 行き先がまだ無ければ、ファイルを作らずに「まだ無いページ」（missing）が返る。
 */
export const linkedPage = (from: string, link: string): Promise<Entry> =>
  Backend.LinkedPage(from, link);

/**
 * タグを表すページ（ファイル名がそのタグのノート）を取得する。
 * まだ無ければ、ファイルを作らずに「まだ無いページ」（missing）が返る。
 */
export const tagPage = (tag: string): Promise<Entry> => Backend.TagPage(tag);

/** そのノートの関連ページを取得する。まだ無いページでも、そのページを指しているノートが返る。 */
export const getRelatedPages = (path: string): Promise<Related> => Backend.RelatedPages(path);

/** Markdown の本文（先頭の「---」で囲んだブロックは除く）を取得する。 */
export const getMarkdownSource = (path: string): Promise<string> => Backend.MarkdownSource(path);

/**
 * 本文に書かれた URL のリンク先から、リンクカードに出すタイトルや画像を取得する。
 * 外部への通信が発生する。取得できなければ reject される。
 */
export const getLinkPreview = (url: string): Promise<LinkPreview> => Backend.LinkPreview(url);

/** Markdown ファイルを、拡張子に紐づいたアプリ（既定のテキストエディタ）で開く。 */
export const openInEditor = (path: string): Promise<void> => Backend.OpenInEditor(path);

/**
 * まだ無いページ（linkedPage や tagPage が返したもの）の md ファイルを作り、既定のエディタで開く。
 * 新しく作ったときは true、既にあったファイルを開いただけなら false になる。
 */
export const createPage = (path: string): Promise<boolean> => Backend.CreatePage(path);

/** 一覧の先頭にノートをピン留めする（pinned が false なら外す）。フォルダの .taggo.json に保存される。 */
export const setPinned = (path: string, pinned: boolean): Promise<void> =>
  Backend.SetPinned(path, pinned);

/** 今の設定を取得する。 */
export const getSettings = (): Promise<Settings> => Backend.GetSettings() as Promise<Settings>;

/** 設定を保存して今のアプリへ反映する。保存した設定が返る。値が正しくなければ reject される。 */
export const saveSettings = (next: Settings): Promise<Settings> =>
  Backend.SaveSettings(next) as Promise<Settings>;

/** 設定ファイルの置き場所を取得する。 */
export const getSettingsPath = (): Promise<string> => Backend.SettingsPath();

/** 起動時に出た警告（開けなかったフォルダなど）を受け取る。受け取った警告は二度と返らない。 */
export const takeStartupWarnings = (): Promise<string[]> => Backend.TakeStartupWarnings();

/** イベント購読。戻り値を呼ぶと購読を解除する。 */
export function on<T>(event: string, handler: (payload: T) => void): () => void {
  return EventsOn(event, handler as (...data: unknown[]) => void);
}

/**
 * Markdown の本文に埋め込まれた画像の配信 URL。
 * Go 側のアセットハンドラーが、開いているフォルダ配下にある表示できる画像だけを返す。
 */
export const imageURL = (path: string): string =>
  `/taggo/image?path=${encodeURIComponent(path)}`;
