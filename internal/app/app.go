// Package app はフロントエンドへ公開する API をまとめる。
// Wails はこのパッケージの公開メソッドを TypeScript のバインディングへ変換する。
package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/yuuxyu/taggo/internal/cloudfile"
	"github.com/yuuxyu/taggo/internal/meta"
	"github.com/yuuxyu/taggo/internal/model"
	"github.com/yuuxyu/taggo/internal/scan"
	"github.com/yuuxyu/taggo/internal/search"
	"github.com/yuuxyu/taggo/internal/store"
	"github.com/yuuxyu/taggo/internal/thumb"
	"github.com/yuuxyu/taggo/internal/watcher"
)

// フロントエンドへ送るイベント名。
const (
	// EventScanProgress は走査の進捗を伝える。
	EventScanProgress = "scan:progress"
	// EventScanDone は走査の完了を伝える。
	EventScanDone = "scan:done"
	// EventEntryChanged はウォッチャーが検知した変更を伝える。
	EventEntryChanged = "entry:changed"
)

// App はアプリ全体の状態を持つ。
type App struct {
	ctx    context.Context
	store  *store.Store
	thumbs *thumb.Cache

	// mu は watcher と scanCancel、cloudOnly を守る。フォルダの開き直しと
	// 進行中の走査キャンセルが競合しうるため。
	mu          sync.Mutex
	watcher     *watcher.Watcher
	watcherStop context.CancelFunc
	scanCancel  context.CancelFunc
	// cloudOnly は直近の走査で、中身がクラウド上にしか無いため
	// 読み込まなかったファイルの数。
	cloudOnly int
}

// New はアプリを組み立てる。インメモリ DB の初期化に失敗した場合のみエラーを返す。
func New() (*App, error) {
	s, err := store.Open()
	if err != nil {
		return nil, err
	}
	return &App{store: s, thumbs: thumb.NewCache()}, nil
}

// Startup は Wails の起動フックから呼ばれ、以降 runtime API を使えるようにする。
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
}

// Shutdown は Wails の終了フックから呼ばれる。
// インメモリ DB は破棄されるだけで、保存すべきものは何も無い。
func (a *App) Shutdown(context.Context) {
	a.stopWatcher()

	a.mu.Lock()
	if a.scanCancel != nil {
		a.scanCancel()
		a.scanCancel = nil
	}
	a.mu.Unlock()

	a.store.Close()
}

// ---- 以下、フロントエンドへ公開する API ----

// Status はフォルダの読み込み状況をまとめたもの。
type Status struct {
	Root       string `json:"root"`
	EntryCount int    `json:"entryCount"`
	TagCount   int    `json:"tagCount"`
	// Scanning は走査が進行中かどうか。
	Scanning bool `json:"scanning"`
	// LimitReached は上限件数に達して打ち切ったかどうか。
	LimitReached bool `json:"limitReached"`
	// MaxEntries は展開件数の上限。UI の注意書きに使う。
	MaxEntries int `json:"maxEntries"`
	// CloudOnly は、中身がクラウド上にしか無いため読み込まなかった件数。
	CloudOnly int `json:"cloudOnly"`
}

// ScanProgress は走査の進捗イベントのペイロード。
type ScanProgress struct {
	Done  int `json:"done"`
	Found int `json:"found"`
}

// SelectFolder はフォルダ選択ダイアログを開き、選ばれたフォルダのパスを返す。
// キャンセルされた場合は空文字を返す。
//
// 選ぶことと読み込むことを分けてあるのは、クラウド同期フォルダのように
// 走査でダウンロードが発生しうる場所では、読み込む前に確認を挟みたいため。
// 実際の読み込みは呼び出し側が OpenFolder を呼んで始める。
func (a *App) SelectFolder() (string, error) {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "タグ管理するフォルダを選択",
	})
	if err != nil {
		return "", fmt.Errorf("フォルダ選択ダイアログを開けませんでした: %w", err)
	}
	return dir, nil
}

// CloudSyncHint は、そのフォルダがクラウド同期フォルダの中にある場合にサービス名を返す。
// 該当しなければ空文字を返す。
//
// 中身がローカルに無いファイルは属性から見分けて開かずに済ませるが、
// 同期フォルダを読み込むこと自体を利用者が意識できるよう、走査を始める前の確認に使う。
func (a *App) CloudSyncHint(path string) string {
	return cloudfile.SyncRootProvider(path)
}

// FetchCloudEntry はクラウド上にだけあるファイルを、利用者の明示操作で取り込む。
//
// ここで初めてファイルを開くため、クラウドからのダウンロードが発生する。
// 自動では決して呼ばず、画面上の操作から呼ぶこと。
func (a *App) FetchCloudEntry(path string) (*model.Entry, error) {
	entry, ok := a.store.Get(path)
	if !ok {
		return nil, fmt.Errorf("エントリが見つかりません: %s", path)
	}
	if !entry.CloudOnly {
		return entry, nil // すでに取り込み済み
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("ファイルを読めませんでした: %w", err)
	}
	updated, err := meta.Read(path, info)
	if err != nil {
		return nil, fmt.Errorf("メタデータを読めませんでした: %w", err)
	}
	updated.RelPath = entry.RelPath

	if err := a.putEntry(updated); err != nil {
		return nil, err
	}
	a.emit(EventEntryChanged, map[string]any{"path": path, "entry": updated})
	return updated, nil
}

// ensureLocal は、これから中身を開くファイルが今もローカルにあるかを確かめる。
//
// 走査のあとで同期サービスがファイルを「オンラインのみ」へ戻しても、変わるのは属性だけで
// 更新日時は変わらないため、ウォッチャーでは気付けない。DB 上はローカルのままなので、
// 確かめずに開くとその場でダウンロードが始まってしまう。属性の問い合わせはファイルを
// 開かずに済むので、開く直前に毎回確かめる。
//
// クラウド上にしか無くなっていたら、DB と画面をその状態へ直したうえでエラーを返す。
func (a *App) ensureLocal(entry *model.Entry) error {
	info, err := cloudfile.StatLocal(entry.Path)
	if !errors.Is(err, cloudfile.ErrCloudOnly) {
		return err
	}

	cloud := model.NewCloudOnly(entry.Path, info.Name(), info.Size(), info.ModTime())
	cloud.RelPath = entry.RelPath
	a.thumbs.Invalidate(entry.Path)
	if err := a.putEntry(cloud); err == nil {
		a.emit(EventEntryChanged, map[string]any{"path": entry.Path, "entry": cloud})
	}
	return fmt.Errorf("クラウド上にだけあるファイルに戻っていたため開きません: %s", entry.Name)
}

// putEntry はエントリを DB へ入れ、クラウド上にだけある件数を前の状態との差で数え直す。
// 走査のあとでローカルとクラウドの間を行き来したファイルも、件数に正しく反映するため。
func (a *App) putEntry(e *model.Entry) error {
	prev, had := a.store.Get(e.Path)
	if err := a.store.Put(e); err != nil {
		return err
	}
	a.adjustCloudOnly(had && prev.CloudOnly, e.CloudOnly)
	return nil
}

// adjustCloudOnly は、1 件のエントリがクラウド上にだけあるかどうかの変化を件数へ反映する。
func (a *App) adjustCloudOnly(was, now bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	switch {
	case !was && now:
		a.cloudOnly++
	case was && !now && a.cloudOnly > 0:
		a.cloudOnly--
	}
}

// OpenFolder は指定フォルダを走査してインメモリ DB へ展開する。
// 走査はバックグラウンドで進み、進捗はイベントで通知する。
func (a *App) OpenFolder(root string) error {
	abs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("フォルダのパスを解決できませんでした: %w", err)
	}
	// 走査を始めると中身の読み取りでダウンロードが始まってしまうので、
	// 開いているフォルダの状態を捨てる前に断る。
	if cloudfile.IsGoogleDriveStreaming(abs) {
		return fmt.Errorf("Google ドライブのストリーミング用ドライブは対象外です。" +
			"クラウド上にだけあるファイルを見分けられず、読み込むとダウンロードが始まるためです。" +
			"Google ドライブの設定で「ファイルをミラーリング」にしたフォルダなら読み込めます")
	}

	a.mu.Lock()
	if a.scanCancel != nil {
		a.scanCancel() // 前の走査が残っていれば止める
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.scanCancel = cancel
	a.mu.Unlock()

	a.stopWatcher()
	a.thumbs.Clear()
	if err := a.store.Reset(abs); err != nil {
		return err
	}

	go a.runScan(ctx, abs)
	return nil
}

// runScan は走査を実行し、完了後にウォッチャーを開始する。
func (a *App) runScan(ctx context.Context, root string) {
	result, err := scan.Scan(ctx, scan.Options{
		Root:  root,
		Limit: store.MaxEntries,
		OnProgress: func(done, found int) {
			a.emit(EventScanProgress, ScanProgress{Done: done, Found: found})
		},
	})

	if ctx.Err() != nil {
		return // 新しいフォルダが選ばれたので、この走査結果は捨てる
	}
	if err != nil {
		a.emit(EventScanDone, map[string]any{"error": err.Error()})
		return
	}

	if err := a.store.PutAll(result.Entries); err != nil {
		a.emit(EventScanDone, map[string]any{"error": err.Error()})
		return
	}

	a.mu.Lock()
	a.scanCancel = nil
	a.cloudOnly = result.CloudOnly
	a.mu.Unlock()

	a.startWatcher(root)
	a.emit(EventScanDone, Status{
		Root:         root,
		EntryCount:   len(result.Entries),
		TagCount:     a.store.TagCount(),
		LimitReached: result.LimitReached,
		MaxEntries:   store.MaxEntries,
		CloudOnly:    result.CloudOnly,
	})
}

// Status は現在の読み込み状況を返す。
func (a *App) Status() Status {
	a.mu.Lock()
	scanning := a.scanCancel != nil
	cloudOnly := a.cloudOnly
	a.mu.Unlock()

	return Status{
		Root:       a.store.Root(),
		EntryCount: a.store.Count(),
		TagCount:   a.store.TagCount(),
		Scanning:   scanning,
		MaxEntries: store.MaxEntries,
		CloudOnly:  cloudOnly,
	}
}

// Search は検索バーの入力でエントリを絞り込む。
func (a *App) Search(opts store.SearchOptions) (store.Result, error) {
	return a.store.Search(opts)
}

// Tags は検索バーのオートコンプリート候補を返す。
func (a *App) Tags(prefix string, limit int) []store.TagSuggestion {
	return a.store.Tags(prefix, limit)
}

// Entry は 1 件のエントリを返す。詳細プレビューを開くときに使う。
func (a *App) Entry(path string) (*model.Entry, error) {
	e, ok := a.store.Get(path)
	if !ok {
		return nil, fmt.Errorf("エントリが見つかりません: %s", path)
	}
	return e, nil
}

// RelatedPages は、そのノートがリンクしているページと、そのノートへ
// リンクしているページ、タグが重なるノートを返す。Markdown プレビューの右側に並べる。
func (a *App) RelatedPages(path string) store.Related {
	e, ok := a.store.Get(path)
	if !ok {
		return store.EmptyRelated()
	}
	return a.store.Related(e)
}

// MarkdownSource は Markdown ファイルの本文（Front Matter を除いた部分）を返す。
// Front Matter のタグはヘッダーにバッジとして別途表示するため、本文からは切り離す。
func (a *App) MarkdownSource(path string) (string, error) {
	e, ok := a.store.Get(path)
	if !ok {
		return "", fmt.Errorf("エントリが見つかりません: %s", path)
	}
	if e.Kind != model.KindMarkdown {
		return "", fmt.Errorf("Markdown ファイルではありません: %s", path)
	}
	if e.CloudOnly {
		return "", fmt.Errorf("クラウド上にだけあるファイルのため読み込みません: %s", e.Name)
	}
	if err := a.ensureLocal(e); err != nil {
		return "", err
	}
	return readMarkdownBody(path)
}

// AppendTagToQuery は検索バーの文字列にタグを AND 条件として足したものを返す。
// カードのタグバッジをクリックしたときに使う。
func (a *App) AppendTagToQuery(query, tag string) string {
	return search.AppendTag(query, tag)
}

// emit はフロントエンドへイベントを送る。起動前は何もしない。
func (a *App) emit(name string, payload any) {
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, name, payload)
}

// startWatcher は root 配下の監視を始める。
func (a *App) startWatcher(root string) {
	w, err := watcher.New(root)
	if err != nil {
		// 監視が使えなくても検索と編集は成立するので、起動自体は続ける。
		a.emit(EventScanDone, map[string]any{
			"warning": fmt.Sprintf("フォルダ監視を開始できませんでした: %v", err),
		})
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	a.mu.Lock()
	a.watcher = w
	a.watcherStop = cancel
	a.mu.Unlock()

	go w.Run(ctx)
	go a.consumeChanges(w)
}

// consumeChanges はウォッチャーの通知を DB へ反映し、フロントエンドへ転送する。
func (a *App) consumeChanges(w *watcher.Watcher) {
	for change := range w.Changes() {
		a.thumbs.Invalidate(change.Path)

		if change.Removed {
			prev, had := a.store.Get(change.Path)
			if err := a.store.Delete(change.Path); err != nil {
				continue
			}
			a.adjustCloudOnly(had && prev.CloudOnly, false)
			a.emit(EventEntryChanged, map[string]any{
				"path":    change.Path,
				"removed": true,
			})
			continue
		}
		if err := a.putEntry(change.Entry); err != nil {
			continue
		}
		a.emit(EventEntryChanged, map[string]any{
			"path":  change.Path,
			"entry": change.Entry,
		})
	}
}

// stopWatcher は監視を止める。監視していなければ何もしない。
func (a *App) stopWatcher() {
	a.mu.Lock()
	w, stop := a.watcher, a.watcherStop
	a.watcher, a.watcherStop = nil, nil
	a.mu.Unlock()

	if stop != nil {
		stop()
	}
	if w != nil {
		w.Close()
	}
}

// readMarkdownBody は Front Matter を除いた本文を読む。
func readMarkdownBody(path string) (string, error) {
	raw, err := readFileLimited(path)
	if err != nil {
		return "", err
	}
	_, body, err := meta.SplitFrontMatter(raw)
	if err != nil {
		// Front Matter が壊れている場合は、全文をそのまま本文として見せる。
		return string(raw), nil
	}
	return string(body), nil
}
