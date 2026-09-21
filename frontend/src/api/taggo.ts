/**
 * Wails が生成したバインディングを、アプリ側から扱いやすい形に包む層。
 *
 * 生成コードを直接あちこちから呼ぶと、バインディングの形が変わったときに
 * 影響範囲が広がるため、入口をここ 1 か所に集約している。
 */

import * as Backend from "../../wailsjs/go/app/App";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import type { app, model, store } from "../../wailsjs/go/models";

export type Entry = model.Entry;
export type Status = app.Status;
export type TagSuggestion = store.TagSuggestion;
export type TagEditResult = app.TagEditResult;
export type RelatedPage = store.RelatedPage;
export type Related = store.Related;
export type SearchResult = store.Result;

/** 一覧の並び順。Go 側の store.SortOrder と対応する。 */
export type SortOrder = "modified_desc" | "name_asc" | "relevance";

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
  /** 上限で打ち切ったため、まだ読み込んでいない対応ファイルの数。 */
  remaining?: number;
  /** 続きの読み込み（loadMore）の完了かどうか。 */
  loadedMore?: boolean;
  /** 今回の読み込みで一覧に加わった件数。 */
  added?: number;
  /** 利用者の操作で続きの読み込みを取りやめたかどうか。 */
  cancelled?: boolean;
  /** 中身がクラウド上にしか無いため、読み込まなかったファイルの数。 */
  cloudOnly?: number;
  error?: string;
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
} as const;

/**
 * フォルダ選択ダイアログを開く。キャンセル時は空文字が返る。
 * 選ぶだけで、読み込みは openFolder を呼ぶまで始まらない。
 */
export const selectFolder = (): Promise<string> => Backend.SelectFolder();

/**
 * そのフォルダがクラウド同期フォルダの中にあるか。該当すればサービス名が返る。
 * 走査でダウンロードが起きうる場所かを、読み込む前に確かめるために使う。
 */
export const cloudSyncHint = (path: string): Promise<string> => Backend.CloudSyncHint(path);

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

/** 検索バーの入力でエントリを絞り込む。 */
export const search = (query: string, sort: SortOrder): Promise<SearchResult> =>
  Backend.Search({ query, sort, offset: 0, limit: 0 });

/** オートコンプリート候補を取得する。prefix が空なら全タグ。 */
export const suggestTags = (prefix: string, limit = 30): Promise<TagSuggestion[]> =>
  Backend.Tags(prefix, limit);

/** 1 件のエントリを取得する。 */
export const getEntry = (path: string): Promise<Entry> => Backend.Entry(path);

/** そのノートの関連ページ（リンク先とリンク元）を取得する。 */
export const getRelatedPages = (path: string): Promise<Related> => Backend.RelatedPages(path);

/** Markdown の本文（Front Matter を除く）を取得する。 */
export const getMarkdownSource = (path: string): Promise<string> => Backend.MarkdownSource(path);

/** 1 ファイルのタグを置き換える。 */
export const setTags = (path: string, tags: string[]): Promise<TagEditResult> =>
  Backend.SetTags(path, tags);

/** 複数ファイルへ同じタグを追加する。 */
export const addTags = (paths: string[], tags: string[]): Promise<TagEditResult[]> =>
  Backend.AddTags(paths, tags);

/** 複数ファイルから指定タグを取り除く。 */
export const removeTags = (paths: string[], tags: string[]): Promise<TagEditResult[]> =>
  Backend.RemoveTags(paths, tags);

/** 検索バーの文字列へタグを AND 条件として足す。 */
export const appendTagToQuery = (query: string, tag: string): Promise<string> =>
  Backend.AppendTagToQuery(query, tag);

/** イベント購読。戻り値を呼ぶと購読を解除する。 */
export function on<T>(event: string, handler: (payload: T) => void): () => void {
  return EventsOn(event, handler as (...data: unknown[]) => void);
}

/**
 * 原寸ファイルの配信 URL。画像の拡大表示や音声再生に使う。
 * Go 側のアセットハンドラーが、開いているフォルダ配下の登録済みファイルだけを返す。
 */
export const fileURL = (path: string): string =>
  `/taggo/file?path=${encodeURIComponent(path)}`;

/** サムネイルの配信 URL。カードのグリッドで使う。 */
export const thumbURL = (path: string, width = 480): string =>
  `/taggo/thumb?path=${encodeURIComponent(path)}&w=${width}`;
