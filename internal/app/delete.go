package app

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/yuuxyu/taggo/internal/folderconf"
	"github.com/yuuxyu/taggo/internal/model"
	"github.com/yuuxyu/taggo/internal/trash"
)

// DeleteResult はページを削除した結果。
type DeleteResult struct {
	// Cancelled は、ごみ箱の無いドライブで、完全に消す確認を利用者がやめたこと。
	// ファイルは何も変わっていない。
	Cancelled bool `json:"cancelled,omitempty"`
	// Warning は、削除はできたが知らせておきたいこと（ピン留めを外せなかったなど）。
	Warning string `json:"warning,omitempty"`
}

// DeletePage は、path のページのファイルをごみ箱へ移し、一覧から除く。
//
// ほかのノートにある、このページを指す [[タグ]] とリンクは書き換えない。
// それらは、まだ無いページを指すようになる。ピン留めしていれば外す。
//
// 利用者が確かめたうえで呼ぶこと。ごみ箱へ移すので、元に戻すときはごみ箱から戻す。
func (a *App) DeletePage(path string) (*DeleteResult, error) {
	a.writeMu.Lock()
	defer a.writeMu.Unlock()

	if err := a.checkPagePath(path); err != nil {
		return nil, err
	}
	a.mu.Lock()
	scanning := a.scanCancel != nil
	a.mu.Unlock()
	// 読み込み中に消すと、消す前に読んだ分が読み込みの終わりに一覧へ戻ってしまう。
	if scanning {
		return nil, errors.New("読み込みの途中です。終わってからもう一度お試しください")
	}
	entry, ok := a.store.Get(path)
	if !ok {
		return nil, fmt.Errorf("エントリが見つかりません: %s", path)
	}

	if err := a.trash(entry.Path); err != nil {
		if errors.Is(err, trash.ErrCancelled) {
			return &DeleteResult{Cancelled: true}, nil
		}
		return nil, fmt.Errorf("「%s」を削除できませんでした: %w", filepath.Base(entry.Path), err)
	}

	result := &DeleteResult{Warning: a.unpinDeleted(entry)}
	// 一覧（DB）からはウォッチャーを待たずに除く。ウォッチャーは、DB に無いファイルが
	// 消えた知らせを読み飛ばす。
	if err := a.store.Delete(entry.Path); err != nil {
		return nil, err
	}
	a.emit(EventEntryChanged, map[string]any{"path": entry.Path, "removed": true})
	return result, nil
}

// unpinDeleted は、削除したページをピン留めしていれば、設定ファイルから外す。
// 外せなかったときは、その理由を返す。
func (a *App) unpinDeleted(entry *model.Entry) string {
	if !a.store.IsPinned(entry.Path) {
		return ""
	}
	a.pinMu.Lock()
	defer a.pinMu.Unlock()

	root := a.store.Root()
	cfg, err := folderconf.Load(root)
	if err == nil {
		name := filepath.Base(entry.Path)
		kept := make([]string, 0, len(cfg.Pinned))
		for _, p := range cfg.Pinned {
			if !strings.EqualFold(p, name) {
				kept = append(kept, p)
			}
		}
		cfg.Pinned = kept
		err = folderconf.Save(root, cfg)
	}
	if err != nil {
		return fmt.Sprintf("ピン留めを外せませんでした: %v", err)
	}
	a.store.SetPins(pinPaths(root, cfg.Pinned))
	a.emit(EventPinsChanged, PinsChanged{})
	return ""
}
