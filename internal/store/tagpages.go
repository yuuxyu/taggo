package store

import (
	"sort"

	"github.com/yuuxyu/taggo/internal/model"
)

// tagPageKey は、そのノートがページとして表しているタグのキーを返す。
// ファイル名（拡張子を除く）がタグを表す。
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

// SameNote は、a と b が同じノートを指しているかを判定する。
// パスの大文字小文字と、.md と .markdown の違いは区別しない。
func SameNote(a, b string) bool {
	return noteKey(a) == noteKey(b)
}

// TagNames は、使われているタグと、ページのあるタグ（ノートのファイル名）の表記を返す。
// 本文に [[ と入力したときの補完の候補に使う。
//
// 大文字小文字と空白の連なりだけが違うものは 1 つにまとめ、本文で使われている表記を
// 優先する。使われている件数の多い順に並べ、同じなら名前の順にする。
func (s *Store) TagNames() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	spelling := make(map[string]string, len(s.tagCounts)+len(s.tagPages))
	for key, tag := range s.tagSpelling {
		spelling[key] = tag
	}
	for key, paths := range s.tagPages {
		if _, ok := spelling[key]; !ok {
			spelling[key] = model.PageTag(pickOne(paths))
		}
	}

	keys := make([]string, 0, len(spelling))
	for key := range spelling {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if ci, cj := s.tagCounts[keys[i]], s.tagCounts[keys[j]]; ci != cj {
			return ci > cj
		}
		return keys[i] < keys[j]
	})
	names := make([]string, len(keys))
	for i, key := range keys {
		names[i] = spelling[key]
	}
	return names
}
