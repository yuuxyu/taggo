// Package meta は Markdown ファイルから、一覧と検索に使う情報（タグ・見出し・抜粋・
// サムネイル・ノート間のリンク）を読み取る。
//
// タグは本文に `[[タグ]]` と書かれたものだけを読む。ノート自身が正（Single Source of Truth）
// であり、taggo はノートを書き換えず、サイドカーファイルや影のデータベースも持たない。
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
	// Tags は本文に `[[タグ]]` と書かれたタグ。本文に出てくる順に並ぶ。
	Tags      []string
	Title     string
	Preview   string
	Thumbnail string
	Links     []string
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
		Path:    path,
		Name:    info.Name(),
		Ext:     model.Ext(path),
		Size:    info.Size(),
		ModTime: info.ModTime(),
		Tags:    []string{},
		Title:   info.Name(),
	}

	got, err := readMarkdown(path)
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
	return e, nil
}
