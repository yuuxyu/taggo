package store

import (
	"github.com/yuuxyu/taggo/internal/model"
)

// tagPageKey は、そのノートがページとして表しているタグのキーを返す。
// ファイル名（拡張子を除く）がタグを表す。クラウド上にだけあって中身を
// 読めていないノートも、名前は分かっているのでタグのページになる。
func tagPageKey(e *model.Entry) string {
	return model.TagKey(model.PageTag(e.Path))
}

// FindNote は、path のノートを読み込んでいれば、その実際のパスを返す。無ければ空を返す。
// パスの大文字小文字と、.md と .markdown の違いは区別しない。
func (s *Store) FindNote(path string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return pickOne(s.notePaths[noteKey(path)])
}

// TagPagePath は、タグ tag を表すページ（ファイル名がそのタグのノート）のパスを返す。
// 読み込んだノートの中に無ければ空を返す。
func (s *Store) TagPagePath(tag string) string {
	key := model.TagKey(tag)
	if key == "" {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return pickOne(s.tagPages[key])
}
