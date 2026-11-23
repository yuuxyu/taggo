package mcpserver

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/yuuxyu/taggo/internal/folderconf"
	"github.com/yuuxyu/taggo/internal/scan"
	"github.com/yuuxyu/taggo/internal/store"
	"github.com/yuuxyu/taggo/internal/watcher"
)

// defaultWaitTimeout は、走査が終わる前にツールが呼ばれたときに待つ時間の上限。
const defaultWaitTimeout = 20 * time.Second

// library は、MCP サーバーが読むフォルダの中身を、画面と同じ仕組み（走査・インメモリ DB・
// ウォッチャー）で持つ。画面の App から、Wails のイベントと「続きを読み込む」を除いたもの。
//
// 走査の上限を超えた分は読まない。続きを読むのは、利用者が設定で上限を上げたときだけにする。
type library struct {
	store *store.Store
	// ready は、走査が終わったとき（か、フォルダを読めないと分かったとき）に閉じる。
	ready chan struct{}
	// waitTimeout は、走査が終わる前に呼ばれたツールを待たせる時間の上限。
	waitTimeout time.Duration

	mu sync.Mutex
	// err はフォルダを読めない理由。nil なら読めている（か、読み込み中）。
	err error
	// done・found は走査の進み具合。
	done, found int
	// cursor は上限で打ち切ったときの再開位置。空なら全部読み込んでいる。
	cursor string
	// remaining は、上限で打ち切ったために読んでいないノートの数。
	remaining int
	cancel    context.CancelFunc
	watcher   *watcher.Watcher
}

func newLibrary() (*library, error) {
	s, err := store.Open()
	if err != nil {
		return nil, err
	}
	return &library{store: s, ready: make(chan struct{}), waitTimeout: defaultWaitTimeout}, nil
}

// fail は、フォルダを読めないことを記録する。以後のツールの呼び出しは、この理由を返す。
func (l *library) fail(err error) {
	l.mu.Lock()
	l.err = err
	l.mu.Unlock()
	close(l.ready)
}

// open は root の走査をバックグラウンドで始める。終わったらウォッチャーを動かす。
func (l *library) open(root string, limit int) {
	abs, err := filepath.Abs(root)
	if err != nil {
		l.fail(fmt.Errorf("フォルダのパスを解決できませんでした: %w", err))
		return
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		l.fail(fmt.Errorf("フォルダ「%s」が見つかりません。taggo の設定の「起動時に開くフォルダ」を確かめてから、"+
			"Claude Desktop を起動し直してください", abs))
		return
	}
	if err := l.store.Reset(abs); err != nil {
		l.fail(err)
		return
	}
	l.loadPins()

	ctx, cancel := context.WithCancel(context.Background())
	l.mu.Lock()
	l.cancel = cancel
	l.mu.Unlock()
	go l.run(ctx, abs, limit)
}

// run は走査を実行し、結果を DB へ入れてからウォッチャーを動かす。
func (l *library) run(ctx context.Context, root string, limit int) {
	result, err := scan.Scan(ctx, scan.Options{
		Root:  root,
		Limit: limit,
		OnProgress: func(done, found int) {
			l.mu.Lock()
			l.done, l.found = done, found
			l.mu.Unlock()
		},
	})
	if ctx.Err() != nil {
		return // 終了の途中
	}
	if err == nil {
		err = l.store.PutAll(result.Entries)
	}
	if err != nil {
		l.fail(fmt.Errorf("フォルダを読み込めませんでした: %w", err))
		return
	}

	l.mu.Lock()
	if result.LimitReached {
		l.cursor, l.remaining = result.Cursor, result.Remaining
	}
	l.mu.Unlock()
	close(l.ready)
	log.Printf("%s のノートを %d 件読み込みました", root, len(result.Entries))

	l.startWatcher(ctx, root)
}

// wait は、走査が終わるまで待つ。フォルダを読めないときはその理由を、待ちきれないときは
// 読み込み中であることをエラーとして返す。
func (l *library) wait(ctx context.Context) error {
	timer := time.NewTimer(l.waitTimeout)
	defer timer.Stop()
	select {
	case <-l.ready:
		l.mu.Lock()
		defer l.mu.Unlock()
		return l.err
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		l.mu.Lock()
		defer l.mu.Unlock()
		return fmt.Errorf("フォルダを読み込んでいます（%d / %d 件）。少し待ってからもう一度呼んでください", l.done, l.found)
	}
}

// notLoaded は、走査の上限で読んでいないノートの数を返す。
func (l *library) notLoaded() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.remaining
}

// loadPins は、フォルダの設定ファイル（.taggo.json）からピン留めを読む。読むだけで、書かない。
// 読めなくてもノートは扱えるので、ログに残すだけにする。
func (l *library) loadPins() {
	root := l.store.Root()
	cfg, err := folderconf.Load(root)
	if err != nil {
		log.Printf("ピン留めを読み込めませんでした: %v", err)
		return
	}
	paths := make([]string, len(cfg.Pinned))
	for i, name := range cfg.Pinned {
		paths[i] = filepath.Join(root, name)
	}
	l.store.SetPins(paths)
}

// startWatcher は root 直下の監視を始める。監視できなくても、読み込んだ分は使える。
func (l *library) startWatcher(ctx context.Context, root string) {
	if ctx.Err() != nil {
		return // 終了の途中
	}
	w, err := watcher.New(root)
	if err != nil {
		log.Printf("フォルダ監視を開始できませんでした: %v", err)
		return
	}
	l.mu.Lock()
	// 作っている間に close が呼ばれていれば、渡す先が無いので閉じる。
	if ctx.Err() != nil {
		l.mu.Unlock()
		w.Close()
		return
	}
	l.watcher = w
	l.mu.Unlock()

	go w.Run(ctx)
	go l.consumeChanges(w)
}

// consumeChanges は、ウォッチャーの通知を DB へ反映する。画面での保存や、ほかのアプリでの変更も、
// ここで拾う。
func (l *library) consumeChanges(w *watcher.Watcher) {
	for change := range w.Changes() {
		switch {
		case change.Config:
			l.loadPins()
		case change.Removed:
			if err := l.store.Delete(change.Path); err != nil {
				log.Printf("消えたノートを一覧から除けませんでした: %v", err)
			}
		case l.notLoadedYet(change.Path):
			// まだ読み込んでいない範囲のファイル。1 件だけ紛れ込ませない。
		default:
			if err := l.store.Put(change.Entry); err != nil {
				log.Printf("変わったノートを読み直せませんでした: %v", err)
			}
		}
	}
}

// notLoadedYet は、そのパスが、走査の上限でまだ読み込んでいない範囲にあるかを判定する。
func (l *library) notLoadedYet(path string) bool {
	l.mu.Lock()
	cursor := l.cursor
	l.mu.Unlock()
	if cursor == "" {
		return false
	}
	if _, ok := l.store.Get(path); ok {
		return false
	}
	rel, err := filepath.Rel(l.store.Root(), path)
	if err != nil {
		return false
	}
	return scan.IsAfter(rel, cursor)
}

// close は走査と監視を止め、DB を閉じる。
func (l *library) close() {
	// キャンセルはロックの中で行う。startWatcher は、ロックを取ってからキャンセルを確かめるので、
	// ここより後にウォッチャーを渡されることはない。
	l.mu.Lock()
	if l.cancel != nil {
		l.cancel()
	}
	w := l.watcher
	l.cancel, l.watcher = nil, nil
	l.mu.Unlock()
	if w != nil {
		w.Close()
	}
	l.store.Close()
}
