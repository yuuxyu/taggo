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
	"github.com/yuuxyu/taggo/internal/cloudfile"
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

	mu      sync.Mutex
	pending map[string]*time.Timer

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
		ignoreTemp: isTaggoTempFile,
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
	defer close(w.changes)
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
	case cloudfile.IsPlaceholder(info):
		// 中身はまだクラウド上にしか無い。ここで読むとダウンロードが始まるので、
		// 一覧に出すのに要る情報だけでエントリを作り直す。
		entry := model.NewCloudOnly(path, info.Name(), info.Size(), info.ModTime())
		if rel, err := filepath.Rel(w.root, path); err == nil {
			entry.RelPath = rel
		}
		change.Entry = entry
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

	// 受け手が居なくなっている可能性があるので、送信でブロックしない。
	select {
	case w.changes <- change:
	default:
	}
}

// isTaggoTempFile は、taggo 自身の書き込み用一時ファイルかを判定する。
// folderconf パッケージの replaceFile は ".<元の名前>.taggo-XXXX" という名前を使う。
func isTaggoTempFile(name string) bool {
	return strings.HasPrefix(name, ".") && strings.Contains(name, ".taggo-")
}
