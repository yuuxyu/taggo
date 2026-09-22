package app

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/yuuxyu/taggo/internal/cloudfile"
)

// prepareNote は、ノートに書かれたリンクの行き先を、エディタで開ける状態にする。
// 行き先がまだ無ければ、見出しだけの Markdown ファイルとして作る。
// 作ったときは created が true になる。
//
// taggo は既存のファイルの本文を書き換えないが、まだ無いノートを新しく作るのは
// その範囲の外にある。既にあるファイルは上書きせず、そのまま返す。
//
// フロントエンドから任意の場所へファイルを作らせないよう、リンク元は読み込み済みの
// ノートに限り、行き先は開いているフォルダの中の .md / .markdown に限る。
func (a *App) prepareNote(fromPath, link string) (path string, created bool, err error) {
	from, ok := a.store.Get(fromPath)
	if !ok {
		return "", false, fmt.Errorf("リンク元のノートが見つかりません: %s", fromPath)
	}
	root := a.store.Root()
	if root == "" {
		return "", false, errors.New("フォルダが開かれていません")
	}

	path = a.store.LinkPath(link, from.Path)
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".md" && ext != ".markdown" {
		return "", false, fmt.Errorf("Markdown ファイルではないため作りません: %s", link)
	}
	if !within(root, path) {
		return "", false, fmt.Errorf("開いているフォルダの外にはノートを作りません: %s", link)
	}

	// 読み込みの上限で一覧に載っていないだけで、ファイル自体はあることもある。
	info, err := cloudfile.StatLocal(path)
	switch {
	case errors.Is(err, cloudfile.ErrCloudOnly):
		return "", false, fmt.Errorf("クラウド上にだけあるファイルのため開きません: %s", filepath.Base(path))
	case err == nil && info.IsDir():
		return "", false, fmt.Errorf("同じ名前のフォルダがあるため作れません: %s", link)
	case err == nil:
		return path, false, nil
	case !errors.Is(err, fs.ErrNotExist):
		return "", false, err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", false, fmt.Errorf("フォルダを作れませんでした: %w", err)
	}
	// O_EXCL で、確かめてから作るまでの間に現れたファイルを上書きしない。
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return path, false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("ノートを作れませんでした: %w", err)
	}
	// 一覧でリンクと同じ名前に見えるよう、ファイル名を見出しにしておく。
	title := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if _, err := fmt.Fprintf(f, "# %s\n", title); err != nil {
		f.Close()
		return "", false, fmt.Errorf("ノートを書き込めませんでした: %w", err)
	}
	if err := f.Close(); err != nil {
		return "", false, fmt.Errorf("ノートを書き込めませんでした: %w", err)
	}
	return path, true, nil
}
