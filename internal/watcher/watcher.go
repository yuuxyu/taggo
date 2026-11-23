// Package watcher はフォルダ直下の変更を監視し、インメモリ DB へ反映する。
//
// 要件どおり、ファイル側の変更が常に正になるよう、外部エディタや別アプリによる
// 書き換えも検知して該当レコードを同期・削除する。走査と同じく、サブフォルダの中は見ない。
// フォルダの設定ファイル（.taggo.json）の書き換えも知らせる。
package watcher

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/yuuxyu/taggo/internal/atomicfile"
	"github.com/yuuxyu/taggo/internal/folderconf"
	"github.com/yuuxyu/taggo/internal/meta"
	"github.com/yuuxyu/taggo/internal/model"
)

// debounceInterval は同一ファイルへの連続イベントをまとめる待ち時間。
// エディタの保存は「一時ファイル作成 → rename」のように複数イベントを生むため、
// 落ち着いてから 1 回だけ読み直す。
const debounceInterval = 300 * time.Millisecond

// Change は 1 件の変更通知。
type Change struct {
	// Path は変更のあったファイルの絶対パス。
	Path string
	// Entry は読み直した結果。削除された場合は nil。
	Entry *model.Entry
	// Removed は、そのパスがもう存在しない（または対象外になった）ことを表す。
	Removed bool
	// Config は、フォルダの設定ファイル（.taggo.json）が変わったことを表す。
	// このとき Entry と Removed は使わない。
	Config bool
}

// Watcher はフォルダ配下の変更を監視する。
type Watcher struct {
	root    string
	fsw     *fsnotify.Watcher
	changes chan Change

	// mu は pending と closed を守る。changes を閉じることと送ることも、これで 1 つずつにする。
	mu      sync.Mutex
	pending map[string]*time.Timer
	// closed は、Run が changes を閉じたこと。閉じたあとは送らない。
	closed bool

	// ignoreTemp は taggo 自身が書き込みに使う一時ファイルを無視するための判定。
	// 自分の書き込みに反応して読み直しても無駄なうえ、中途半端な内容を読みかねない。
	ignoreTemp func(name string) bool
}

// New は root 直下の監視を開始する。返された Watcher は Close で止める。
func New(root string) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	w := &Watcher{
		root:       root,
		fsw:        fsw,
		changes:    make(chan Change, 64),
		pending:    map[string]*time.Timer{},
		ignoreTemp: atomicfile.IsTemp,
	}
	if err := fsw.Add(root); err != nil {
		fsw.Close()
		return nil, err
	}
	return w, nil
}

// Changes は変更通知を受け取るチャネルを返す。Close すると閉じられる。
func (w *Watcher) Changes() <-chan Change { return w.changes }

// Run はイベントループを回す。ctx のキャンセルか Close で戻る。
func (w *Watcher) Run(ctx context.Context) {
	defer w.closeChanges()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			w.handle(event)
		case _, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
			// 監視側のエラーは個別ファイルの問題であることがほとんどで、
			// 監視全体を止める理由にはならないため読み捨てる。
		}
	}
}

// closeChanges は通知のチャネルを閉じ、待っている読み直しをやめる。
//
// 読み直しはタイマーで遅らせて走るので、止めたあとに待ち時間が過ぎることがある。
// タイマーを止めても、既に走り始めた読み直しは止まらないので、閉じたことを覚えておき、
// emit は閉じたチャネルへ送らない（送ると panic する）。
func (w *Watcher) closeChanges() {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, timer := range w.pending {
		timer.Stop()
	}
	w.pending = map[string]*time.Timer{}
	w.closed = true
	close(w.changes)
}

// Close は監視を終了する。
func (w *Watcher) Close() error {
	w.mu.Lock()
	for _, timer := range w.pending {
		timer.Stop()
	}
	w.pending = map[string]*time.Timer{}
	w.mu.Unlock()
	return w.fsw.Close()
}

// handle は 1 件の fsnotify イベントを処理する。
func (w *Watcher) handle(event fsnotify.Event) {
	name := event.Name
	// 監視しているのはフォルダ直下だけだが、念のため直下以外のパスは捨てる。
	if !strings.EqualFold(filepath.Dir(filepath.Clean(name)), filepath.Clean(w.root)) {
		return
	}
	if w.ignoreTemp(filepath.Base(name)) {
		return
	}
	if !model.IsMarkdown(name) && !folderconf.IsConfigFile(w.root, name) {
		return
	}
	w.schedule(name)
}

// schedule は、同一パスへの連続イベントをまとめて 1 回の読み直しにする。
func (w *Watcher) schedule(path string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return
	}
	if timer, ok := w.pending[path]; ok {
		timer.Reset(debounceInterval)
		return
	}
	w.pending[path] = time.AfterFunc(debounceInterval, func() {
		w.mu.Lock()
		delete(w.pending, path)
		w.mu.Unlock()
		w.emit(path)
	})
}

// emit はファイルを読み直して通知する。
func (w *Watcher) emit(path string) {
	change := Change{Path: path}

	info, err := os.Stat(path)
	switch {
	case folderconf.IsConfigFile(w.root, path):
		change.Config = true
	case err != nil || info.IsDir():
		change.Removed = true
	default:
		entry, err := meta.Read(path, info)
		if err != nil {
			change.Removed = true
		} else {
			if rel, err := filepath.Rel(w.root, path); err == nil {
				entry.RelPath = rel
			}
			change.Entry = entry
		}
	}

	// 読み直している間に止められたかもしれないので、閉じていないことを確かめてから送る。
	// 受け手が居なくなっている可能性もあるので、送信でブロックしない（ロックを握ったまま待たない）。
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	select {
	case w.changes <- change:
	default:
	}
}
