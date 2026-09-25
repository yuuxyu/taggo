package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/yuuxyu/taggo/internal/folderconf"
)

// PinsChanged はピン留めの変更イベントのペイロード。
type PinsChanged struct {
	// Warning は、設定ファイルを読めなかったときの理由。
	Warning string `json:"warning,omitempty"`
}

// SetPinned は、一覧の先頭にノートをピン留めする（pinned が false なら外す）。
//
// ピン留めは、開いているフォルダの直下の設定ファイル（.taggo.json）に書き込む。
// 新しくピン留めしたノートは、ピン留めしたノートの最後に並ぶ。
func (a *App) SetPinned(path string, pinned bool) error {
	a.pinMu.Lock()
	defer a.pinMu.Unlock()

	root := a.store.Root()
	if root == "" {
		return fmt.Errorf("フォルダが開かれていません")
	}
	entry, ok := a.store.Get(path)
	if !ok {
		return fmt.Errorf("エントリが見つかりません: %s", path)
	}
	// 外で書き換えられているかもしれないので、手元の状態ではなくファイルを読み直して直す。
	cfg, err := folderconf.Load(root)
	if err != nil {
		return fmt.Errorf("ピン留めを変えられませんでした: %w", err)
	}

	name := filepath.Base(entry.Path)
	kept := make([]string, 0, len(cfg.Pinned)+1)
	for _, p := range cfg.Pinned {
		if !strings.EqualFold(p, name) {
			kept = append(kept, p)
		}
	}
	if pinned {
		// 既にピン留めしていれば、今の位置のまま動かさない。
		if len(kept) < len(cfg.Pinned) {
			return nil
		}
		kept = append(kept, name)
	} else if len(kept) == len(cfg.Pinned) {
		return nil // ピン留めしていない
	}

	cfg.Pinned = kept
	if err := folderconf.Save(root, cfg); err != nil {
		return err
	}
	a.store.SetPins(pinPaths(root, cfg.Pinned))
	a.emit(EventPinsChanged, PinsChanged{})
	return nil
}

// loadPins は root の設定ファイルからピン留めを読み込む。
// 読めなかったときはピン留めを空にして、その理由を返す。
func (a *App) loadPins(root string) (warning string) {
	cfg, err := folderconf.Load(root)
	if err != nil {
		a.store.SetPins(nil)
		return fmt.Sprintf("ピン留めを読み込めませんでした: %v", err)
	}
	a.store.SetPins(pinPaths(root, cfg.Pinned))
	return ""
}

// reloadPins は、設定ファイルが外で書き換えられたときにピン留めを読み直す。
// 書きかけで壊れているだけかもしれないので、読めなければ今のピン留めのまま知らせる。
func (a *App) reloadPins() {
	a.pinMu.Lock()
	defer a.pinMu.Unlock()

	root := a.store.Root()
	cfg, err := folderconf.Load(root)
	if err != nil {
		a.emit(EventPinsChanged, PinsChanged{Warning: fmt.Sprintf("ピン留めを読み込めませんでした: %v", err)})
		return
	}
	a.store.SetPins(pinPaths(root, cfg.Pinned))
	a.emit(EventPinsChanged, PinsChanged{})
}

// pinPaths は、設定ファイルに書かれたファイル名を root の中のパスへ直す。
func pinPaths(root string, names []string) []string {
	out := make([]string, len(names))
	for i, name := range names {
		out[i] = filepath.Join(root, name)
	}
	return out
}
