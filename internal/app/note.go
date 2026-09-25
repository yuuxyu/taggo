package app

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/yuuxyu/taggo/internal/cloudfile"
	"github.com/yuuxyu/taggo/internal/model"
)

// LinkedPage は、fromPath のノートに書かれた Markdown のリンク link の行き先のページを返す。
// link は本文に書かれたパスを、フラグメントを除いてデコードしたもの。
//
// 行き先を読み込んでいればそのエントリを、まだ無ければファイルを作らずに
// 「まだ無いページ」（Missing）を返す。行き先は開いているフォルダの直下の
// .md / .markdown に限る。
func (a *App) LinkedPage(fromPath, link string) (*model.Entry, error) {
	from, ok := a.store.Get(fromPath)
	if !ok {
		return nil, fmt.Errorf("リンク元のノートが見つかりません: %s", fromPath)
	}
	path := a.store.LinkPath(link, from.Path)
	if err := a.checkPagePath(path); err != nil {
		return nil, fmt.Errorf("%w: %s", err, link)
	}
	return a.pageAt(path), nil
}

// TagPage は、タグ tag を表すページ（ファイル名がそのタグのノート）を返す。
// 読み込んでいればそのエントリを、まだ無ければファイルを作らずに
// 「まだ無いページ」（Missing）を返す。ファイル名にできないタグはページにできない。
func (a *App) TagPage(tag string) (*model.Entry, error) {
	root := a.store.Root()
	if root == "" {
		return nil, errors.New("フォルダが開かれていません")
	}
	if path := a.store.TagPagePath(tag); path != "" {
		if e, ok := a.store.Get(path); ok {
			return e, nil
		}
	}
	name, ok := model.TagFileName(tag)
	if !ok {
		return nil, fmt.Errorf("「%s」はファイル名に使えない文字を含むため、ページにできません", tag)
	}
	return a.pageAt(filepath.Join(root, name)), nil
}

// pageAt は path のページを返す。読み込んでいればそのエントリを、無ければ「まだ無いページ」を返す。
// パスの大文字小文字と、.md と .markdown の違いは区別しない。
func (a *App) pageAt(path string) *model.Entry {
	if found := a.store.FindNote(path); found != "" {
		if e, ok := a.store.Get(found); ok {
			return e
		}
	}
	return model.NewMissing(path)
}

// checkPagePath は、path をページのパスとして扱ってよいかを確かめる。
//
// フロントエンドから任意の場所のファイルを作らせないよう、開いているフォルダの
// 直下の .md / .markdown に限る。taggo が読むのはフォルダ直下のノートだけなので、
// サブフォルダに作っても一覧には出ない。
func (a *App) checkPagePath(path string) error {
	root := a.store.Root()
	if root == "" {
		return errors.New("フォルダが開かれていません")
	}
	if !model.IsMarkdown(path) {
		return errors.New("Markdown ファイルではありません")
	}
	if !within(root, path) {
		return errors.New("開いているフォルダの外のノートは扱いません")
	}
	if !strings.EqualFold(filepath.Dir(filepath.Clean(path)), filepath.Clean(root)) {
		return errors.New("サブフォルダのノートは扱いません。taggo が読むのはフォルダ直下のノートだけです")
	}
	return nil
}

// preparePage は、まだ無いページ path の Markdown ファイルを、エディタで開ける状態にする。
// 無ければ見出しだけのファイルとして作り、created を true にする。
//
// taggo は既存のファイルを書き換えないが、まだ無いノートを新しく作るのはその範囲の外にある。
// 既にあるファイルは上書きせず、そのまま返す。
func (a *App) preparePage(path string) (string, bool, error) {
	if err := a.checkPagePath(path); err != nil {
		return "", false, err
	}
	// 読み込みの上限で一覧に載っていないだけで、ファイル自体はあることもある。
	info, err := cloudfile.StatLocal(path)
	switch {
	case errors.Is(err, cloudfile.ErrCloudOnly):
		return "", false, fmt.Errorf("クラウド上にだけあるファイルのため開きません: %s", filepath.Base(path))
	case err == nil && info.IsDir():
		return "", false, fmt.Errorf("同じ名前のフォルダがあるため作れません: %s", filepath.Base(path))
	case err == nil:
		return path, false, nil
	case !errors.Is(err, fs.ErrNotExist):
		return "", false, err
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
