// Package meta は Markdown ファイルの Front Matter に埋め込まれたタグの読み書きを担う。
// 埋め込みメタデータが taggo における正（Single Source of Truth）であり、
// サイドカーファイルや影のデータベースは一切持たない。
package meta

import (
	"errors"
	"fmt"
	"os"

	"github.com/yuuxyu/taggo/internal/model"
)

// ErrUnsupported は Markdown 以外のファイルに対して返すエラー。
var ErrUnsupported = errors.New("Markdown ファイルではありません")

// Info は 1 ファイルから抽出しうる情報をまとめたもの。
type Info struct {
	Tags      []string
	Title     string
	Preview   string
	Thumbnail string
	Links     []string
	TagPage   string
}

// Read は path の Entry を組み立てる。
// メタデータ読み取りの失敗はエラーとして返さず Entry に記録する。
// 壊れたファイル 1 件のせいでグリッドから消えてしまわないようにするためで、
// エラーになるのは Markdown 以外のファイルを渡したときだけである。
func Read(path string, info os.FileInfo) (*model.Entry, error) {
	if !model.IsMarkdown(path) {
		return nil, fmt.Errorf("%w: %s", ErrUnsupported, path)
	}

	e := &model.Entry{
		Path:     path,
		Name:     info.Name(),
		Ext:      model.Ext(path),
		Size:     info.Size(),
		ModTime:  info.ModTime(),
		Tags:     []string{},
		Title:    info.Name(),
		Writable: writable(path, info),
	}

	got, err := markdownHandler{}.Read(path)
	if err != nil {
		e.Err = err.Error()
		return e, nil
	}

	e.Tags = model.NormalizeTags(got.Tags)
	if got.Title != "" {
		e.Title = got.Title
	}
	e.Preview = got.Preview
	e.Thumbnail = got.Thumbnail
	e.Links = got.Links
	e.TagPage = model.NormalizeTag(got.TagPage)
	return e, nil
}

// WriteTags は実ファイルへタグを書き込み、その成否を返す。
// 呼び出し側は、これが nil を返したときにのみ再インデックスすること。
func WriteTags(path string, tags []string) error {
	if !model.IsMarkdown(path) {
		return fmt.Errorf("%w: %s", ErrUnsupported, path)
	}
	return markdownHandler{}.WriteTags(path, model.NormalizeTags(tags))
}
