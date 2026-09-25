// Package app はフロントエンドへ公開する API をまとめる。
// Wails はこのパッケージの公開メソッドを TypeScript のバインディングへ変換する。
package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/yuuxyu/taggo/internal/cloudfile"
	"github.com/yuuxyu/taggo/internal/linkcard"
	"github.com/yuuxyu/taggo/internal/meta"
	"github.com/yuuxyu/taggo/internal/model"
	"github.com/yuuxyu/taggo/internal/scan"
	"github.com/yuuxyu/taggo/internal/settings"
	"github.com/yuuxyu/taggo/internal/store"
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
	// EventPinsChanged はピン留めが変わったことを伝える。taggo からの操作のほか、
	// フォルダの設定ファイル（.taggo.json）が外で書き換えられたときにも送る。
	EventPinsChanged = "pins:changed"
)

// App はアプリ全体の状態を持つ。
type App struct {
	ctx      context.Context
	store    *store.Store
	links    *linkcard.Client
	settings *settings.Store

	// mu は maxEntries と watcher、scanCancel、cloudOnly、続きの読み込み位置、
	// startupWarnings を守る。フォルダの開き直しと進行中の走査キャンセルが競合しうるため。
	mu sync.Mutex
	// maxEntries は 1 回の読み込みで展開する件数の上限。
	// 設定の ScanLimit に従い、テストでは小さくして上限まわりを確かめる。
	maxEntries  int
	watcher     *watcher.Watcher
	watcherStop context.CancelFunc
	scanCancel  context.CancelFunc
	// loadingMore は、進行中の走査が続きの読み込み（LoadMore）かどうか。
	loadingMore bool
	// cloudOnly は読み込んだファイルのうち、中身がクラウド上にしか無いため
	// 開かなかったファイルの数。
	cloudOnly int
	// cursor は上限で打ち切ったときの再開位置（ルートからの相対パス）。
	// 空なら、フォルダの Markdown ファイルは全部読み込み済み。
	cursor string
	// remaining は、まだ読み込んでいない Markdown ファイルの数。
	remaining int
	// startupWarnings は、起動時に画面へ出せなかった警告。画面が用意できてから取りに来る。
	startupWarnings []string

	// pinMu はピン留めの読み書き（設定ファイルを読んで直して書く一連の操作）を 1 つずつにする。
	pinMu sync.Mutex
}

// New はアプリを組み立てる。インメモリ DB の初期化に失敗した場合のみエラーを返す。
func New(cfg *settings.Store) (*App, error) {
	s, err := store.Open()
	if err != nil {
		return nil, err
	}
	return &App{
		store:      s,
		links:      linkcard.New(),
		settings:   cfg,
		maxEntries: cfg.Get().ScanLimit,
	}, nil
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
	// MaxEntries は 1 回の読み込みで展開する件数の上限。
	// 「続きを読み込む」で増える件数の表示に使う。
	MaxEntries int `json:"maxEntries"`
	// Remaining は、上限で打ち切ったためにまだ読み込んでいない Markdown ファイルの数。
	// 0 より大きければ、LoadMore で続きを読み込める。
	Remaining int `json:"remaining"`
	// CloudOnly は、中身がクラウド上にしか無いため読み込まなかった件数。
	CloudOnly int `json:"cloudOnly"`
}

// ScanDone は走査完了イベントのペイロード。
type ScanDone struct {
	Status
	// LoadedMore は、続きの読み込み（LoadMore）の完了かどうか。
	LoadedMore bool `json:"loadedMore,omitempty"`
	// Added は今回の読み込みで一覧に加わった件数。
	Added int `json:"added,omitempty"`
	// Cancelled は、利用者の操作で続きの読み込みを取りやめたかどうか。
	Cancelled bool `json:"cancelled,omitempty"`
	// Warning は、読み込みは済んだが知らせておきたいこと（ピン留めの設定を読めなかったなど）。
	Warning string `json:"warning,omitempty"`
}

// ScanProgress は走査の進捗イベントのペイロード。
type ScanProgress struct {
	Done  int `json:"done"`
	Found int `json:"found"`
	// LoadingMore は、続きの読み込み（LoadMore）の進捗かどうか。
	LoadingMore bool `json:"loadingMore,omitempty"`
}

// SelectFolder はフォルダ選択ダイアログを開き、選ばれたフォルダのパスを返す。
// キャンセルされた場合は空文字を返す。
//
// 選ぶことと読み込むことを分けてあるのは、クラウド同期フォルダのように
// 走査でダウンロードが発生しうる場所では、読み込む前に確認を挟みたいため。
// 実際の読み込みは呼び出し側が OpenFolder を呼んで始める。
func (a *App) SelectFolder() (string, error) {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Wiki として開くフォルダを選択",
	})
	if err != nil {
		return "", fmt.Errorf("フォルダ選択ダイアログを開けませんでした: %w", err)
	}
	return dir, nil
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

// OpenFolder は指定フォルダの直下を走査してインメモリ DB へ展開する。
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
	a.loadingMore = false
	a.cursor, a.remaining = "", 0
	limit := a.maxEntries
	a.mu.Unlock()

	a.stopWatcher()
	if err := a.store.Reset(abs); err != nil {
		return err
	}

	// 覚えられなくてもフォルダは開けるので、読み込みは止めない。
	if err := a.settings.RememberFolder(abs); err != nil {
		log.Printf("前回開いたフォルダを記録できませんでした: %v", err)
	}
	// ピン留めを読めなくても一覧は出せるので、知らせるだけにする。
	warning := a.loadPins(abs)

	go a.runScan(ctx, abs, limit, warning)
	return nil
}

// runScan は走査を実行し、完了後にウォッチャーを開始する。
// warning は走査の前に分かっていた警告で、走査の完了と一緒に知らせる。
func (a *App) runScan(ctx context.Context, root string, limit int, warning string) {
	result, err := scan.Scan(ctx, scan.Options{
		Root:  root,
		Limit: limit,
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
	a.setCursor(result)
	a.mu.Unlock()

	a.startWatcher(root)
	a.emit(EventScanDone, ScanDone{Status: a.Status(), Added: len(result.Entries), Warning: warning})
}

// setCursor は走査結果から、続きの読み込み位置と残り件数を覚える。
// 呼び出し元が a.mu を握っていること。
func (a *App) setCursor(result scan.Result) {
	if result.LimitReached {
		a.cursor, a.remaining = result.Cursor, result.Remaining
	} else {
		a.cursor, a.remaining = "", 0
	}
}

// LoadMore は、上限で打ち切ったフォルダの続きを読み込む。
// all が false なら上限と同じ件数だけ、true なら残りをすべて読み込む。
//
// 読み込みはバックグラウンドで進み、その間も今の一覧はそのまま使える。
// 読み込んだ分は最後にまとめて DB へ入れるので、検索結果が途中で欠けることはない。
func (a *App) LoadMore(all bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.scanCancel != nil {
		return errors.New("読み込みの途中です。終わってからもう一度お試しください")
	}
	if a.cursor == "" {
		return errors.New("読み込んでいないファイルはありません")
	}

	limit := a.maxEntries
	if all {
		limit = 0
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.scanCancel = cancel
	a.loadingMore = true
	go a.runLoadMore(ctx, a.store.Root(), a.cursor, limit)
	return nil
}

// CancelLoadMore は続きの読み込みを取りやめる。読み込み途中の分は捨て、
// 再開位置は変えないので、あとで同じところからやり直せる。
func (a *App) CancelLoadMore() {
	a.mu.Lock()
	cancel := a.scanCancel
	if !a.loadingMore || cancel == nil {
		a.mu.Unlock()
		return
	}
	a.scanCancel = nil
	a.loadingMore = false
	a.mu.Unlock()

	cancel()
	a.emit(EventScanDone, ScanDone{Status: a.Status(), LoadedMore: true, Cancelled: true})
}

// runLoadMore は再開位置から続きを走査し、今の DB へ追加する。
func (a *App) runLoadMore(ctx context.Context, root, cursor string, limit int) {
	result, err := scan.Scan(ctx, scan.Options{
		Root:       root,
		Limit:      limit,
		StartAfter: cursor,
		OnProgress: func(done, found int) {
			a.emit(EventScanProgress, ScanProgress{Done: done, Found: found, LoadingMore: true})
		},
	})
	if ctx.Err() != nil {
		return // 取りやめたか、別のフォルダが選ばれた
	}
	if err == nil {
		err = a.store.PutAll(result.Entries)
	}

	a.mu.Lock()
	a.scanCancel = nil
	a.loadingMore = false
	if err == nil {
		a.cloudOnly += result.CloudOnly
		a.setCursor(result)
	}
	a.mu.Unlock()

	if err != nil {
		a.emit(EventScanDone, map[string]any{"error": err.Error(), "loadedMore": true})
		return
	}
	a.emit(EventScanDone, ScanDone{Status: a.Status(), LoadedMore: true, Added: len(result.Entries)})
}

// notLoadedYet は、そのパスが続きをまだ読み込んでいない範囲にあるかを判定する。
// ウォッチャーの通知でそうしたファイルが 1 件だけ一覧へ紛れ込むのを防ぐ。
func (a *App) notLoadedYet(path string) bool {
	a.mu.Lock()
	cursor := a.cursor
	a.mu.Unlock()
	if cursor == "" {
		return false
	}
	if _, ok := a.store.Get(path); ok {
		return false
	}
	rel, err := filepath.Rel(a.store.Root(), path)
	if err != nil {
		return false
	}
	return scan.IsAfter(rel, cursor)
}

// Status は現在の読み込み状況を返す。
func (a *App) Status() Status {
	a.mu.Lock()
	scanning := a.scanCancel != nil
	cloudOnly := a.cloudOnly
	remaining := a.remaining
	maxEntries := a.maxEntries
	a.mu.Unlock()

	return Status{
		Root:       a.store.Root(),
		EntryCount: a.store.Count(),
		TagCount:   a.store.TagCount(),
		Scanning:   scanning,
		MaxEntries: maxEntries,
		Remaining:  remaining,
		CloudOnly:  cloudOnly,
	}
}

// Search は検索バーの入力でエントリを絞り込む。
func (a *App) Search(opts store.SearchOptions) (store.Result, error) {
	return a.store.Search(opts)
}

// Entry は 1 件のエントリを返す。詳細プレビューを開くときに使う。
func (a *App) Entry(path string) (*model.Entry, error) {
	e, ok := a.store.Get(path)
	if !ok {
		return nil, fmt.Errorf("エントリが見つかりません: %s", path)
	}
	return e, nil
}

// RelatedPages は、そのノートの関連ページをグループに分けて返す。プレビューの本文の下に並べる。
//
// まだ無いページ（LinkedPage や TagPage が返したもの）でも、そのページを指しているノートや、
// そのページが表すタグを持つノートを返す。
func (a *App) RelatedPages(path string) store.Related {
	if e, ok := a.store.Get(path); ok {
		return a.store.Related(e)
	}
	if a.checkPagePath(path) != nil {
		return store.EmptyRelated()
	}
	return a.store.Related(a.pageAt(path))
}

// MarkdownSource は Markdown ファイルの本文を返す。先頭に「---」で囲んだブロック
// （ほかのツールの Front Matter）があれば、本文からは外す。
func (a *App) MarkdownSource(path string) (string, error) {
	e, ok := a.store.Get(path)
	if !ok {
		return "", fmt.Errorf("エントリが見つかりません: %s", path)
	}
	if e.CloudOnly {
		return "", fmt.Errorf("クラウド上にだけあるファイルのため読み込みません: %s", e.Name)
	}
	if err := a.ensureLocal(e); err != nil {
		return "", err
	}
	return readMarkdownBody(path)
}

// LinkPreview は、Markdown 本文に URL だけを書いた段落をリンクカードにするための
// タイトルや画像を、リンク先から取得して返す。
//
// 「勝手に通信しない方針」の例外で、ノートを開いただけで外部へ通信する。
// 本文の外部画像を開いた時点で読み込むのと同じく、利用者が本文に書いた URL だけが対象になる。
func (a *App) LinkPreview(url string) (*linkcard.Preview, error) {
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return a.links.Fetch(ctx, url)
}

// emit はフロントエンドへイベントを送る。起動前は何もしない。
func (a *App) emit(name string, payload any) {
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, name, payload)
}

// startWatcher は root 直下の監視を始める。
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
		if change.Config {
			a.reloadPins()
			continue
		}
		if change.Removed {
			prev, had := a.store.Get(change.Path)
			if !had {
				continue // 読み込んでいないファイルが消えただけ
			}
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
		if a.notLoadedYet(change.Path) {
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

// readMarkdownBody は、先頭の「---」で囲んだブロックを除いた本文を読む。
func readMarkdownBody(path string) (string, error) {
	raw, err := readFileLimited(path)
	if err != nil {
		return "", err
	}
	return string(meta.StripFrontMatter(raw)), nil
}
