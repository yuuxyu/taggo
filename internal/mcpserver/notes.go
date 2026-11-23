package mcpserver

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/yuuxyu/taggo/internal/model"
	"github.com/yuuxyu/taggo/internal/notefile"
	"github.com/yuuxyu/taggo/internal/store"
)

// NoteSummary は、一覧で返すノート 1 件。
type NoteSummary struct {
	Name     string   `json:"name" jsonschema:"ノートのファイル名。ほかのツールの note にはこれを渡す"`
	Title    string   `json:"title" jsonschema:"最初の見出し。見出しが無ければファイル名"`
	Tags     []string `json:"tags" jsonschema:"本文に [[タグ]] と書かれたタグ"`
	Modified string   `json:"modified" jsonschema:"更新日時（RFC 3339）"`
	Excerpt  string   `json:"excerpt,omitempty" jsonschema:"本文の抜粋（240 文字まで）"`
	Pinned   bool     `json:"pinned,omitempty" jsonschema:"一覧の先頭にピン留めしている"`
}

// summarize はエントリを一覧の形にする。
func summarize(s *store.Store, e *model.Entry) NoteSummary {
	return NoteSummary{
		Name:     filepath.Base(e.Path),
		Title:    e.Title,
		Tags:     nonNil(e.Tags),
		Modified: formatTime(e.ModTime),
		Excerpt:  e.Preview,
		Pinned:   s.IsPinned(e.Path),
	}
}

// formatTime は日時を、タイムゾーン付きの RFC 3339 にする。
func formatTime(t time.Time) string { return t.Local().Format(time.RFC3339) }

// page は、ツールの引数 note が指すページ。
type page struct {
	// path はページのパス。まだ無いページなら、作るとしたらここになるパス。
	path string
	// entry は読み込み済みのノート。まだ無いページなら nil。
	entry *model.Entry
}

// name はページのファイル名。
func (p page) name() string { return filepath.Base(p.path) }

// asEntry は、まだ無いページも含めてエントリとして返す（関連の索引を引くため）。
func (p page) asEntry() *model.Entry {
	if p.entry != nil {
		return p.entry
	}
	return model.NewMissing(p.path)
}

// resolve は、ツールの引数 note（ノートのファイル名）が指すページを返す。
//
//   - 拡張子は省いてよい（"golang" は golang.md か golang.markdown）。
//   - 大文字小文字と空白の連なりは、タグと同じく区別しない。
//   - フォルダ直下のノートだけを扱う。パスの区切りを含む名前は断る。
//
// 読み込み済みのノートが無ければ、まだ無いページ（entry が nil）を返す。
func resolve(s *store.Store, note string) (page, error) {
	name := strings.TrimSpace(note)
	if name == "" {
		return page{}, fmt.Errorf("note にノートのファイル名を入れてください")
	}
	if strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		return page{}, fmt.Errorf("「%s」はファイル名ではありません。note にはフォルダを含まないファイル名（golang.md など）を渡してください。"+
			"taggo が扱うのはフォルダ直下のノートだけです", note)
	}
	if !model.IsMarkdown(name) {
		name += ".md"
	}
	root := s.Root()
	path := filepath.Join(root, name)
	if err := notefile.CheckPagePath(root, path); err != nil {
		return page{}, fmt.Errorf("「%s」は扱えません: %w", note, err)
	}

	// ファイル名（大文字小文字と .md / .markdown の違いを除く）で探し、無ければタグとして探す。
	found := s.FindNote(path)
	if found == "" {
		found = s.TagPagePath(model.PageTag(path))
	}
	if found != "" {
		if e, ok := s.Get(found); ok {
			return page{path: e.Path, entry: e}, nil
		}
	}
	// まだ無いページは、タグのページと同じ名前（空白の連なりをまとめたもの）にそろえる。
	if tagName, ok := model.TagFileName(model.PageTag(path)); ok && model.Ext(path) == ".md" {
		path = filepath.Join(root, tagName)
	}
	return page{path: path}, nil
}

// errNotFound は、ノートもタグも見つからないときのエラー。
func errNotFound(note string) error {
	return fmt.Errorf("「%s」というノートはありません。search_notes で探すか、list_tags でタグの表記を確かめてください", note)
}

// window は items に offset と limit を当て、続きがあれば次の offset を返す。
func window[T any](items []T, offset, limit int) ([]T, *int) {
	offset = max(offset, 0)
	if offset >= len(items) {
		return []T{}, nil
	}
	end := len(items)
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	var next *int
	if end < len(items) {
		next = &end
	}
	return items[offset:end], next
}

// nonNil は、nil のスライスを空のスライスにする。結果の JSON で null ではなく [] にするため。
func nonNil[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}

// pageName は、パスが空でなければそのファイル名を、空なら nil を返す。
func pageName(path string) *string {
	if path == "" {
		return nil
	}
	name := filepath.Base(path)
	return &name
}
