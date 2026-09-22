package store

import (
	"strings"

	"github.com/yuuxyu/taggo/internal/model"
)

// TagPageGroup は 1 つのタグについてのタグページ。
type TagPageGroup struct {
	// Tag はタグの表記。ファイルに付いている表記があればそれを、
	// 無ければタグページの `tag:` に書かれた表記を使う。
	Tag string `json:"tag"`
	// Pages はそのタグを `tag:` で宣言しているノート。パス順に並ぶ。
	// 本来は 1 件だが、複数のノートが同じタグを宣言していれば全部を返し、
	// 画面で重複を警告する。
	Pages []*model.Entry `json:"pages"`
}

// tagPageKey は、タグページを索引に載せるときのキーを返す。
// タグページでないか、クラウド上にだけあって中身を読めていないノートは空を返す。
func tagPageKey(e *model.Entry) string {
	if e.TagPage == "" {
		return ""
	}
	return strings.ToLower(e.TagPage)
}

// tagPageGroups は、検索しているタグのうちタグページがあるものについて、
// そのタグページを返す。大文字小文字だけが違うタグは 1 つにまとめる。
func (s *Store) tagPageGroups(tags []string) []TagPageGroup {
	groups := []TagPageGroup{}
	seen := map[string]struct{}{}
	for _, tag := range tags {
		key := strings.ToLower(tag)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}

		pages := s.tagPageEntries(key, "")
		if len(pages) == 0 {
			continue
		}
		s.mu.RLock()
		spelling, ok := s.tagSpelling[key]
		s.mu.RUnlock()
		if !ok {
			spelling = pages[0].TagPage
		}
		groups = append(groups, TagPageGroup{Tag: spelling, Pages: pages})
	}
	return groups
}

// tagPageEntries は、小文字のタグ key を宣言しているノートをパス順に返す。
// except に渡したパスは除く。
func (s *Store) tagPageEntries(key, except string) []*model.Entry {
	s.mu.RLock()
	paths := pathsExcept(s.tagPages[key], except)
	s.mu.RUnlock()

	out := make([]*model.Entry, 0, len(paths))
	for _, path := range paths {
		if e, ok := s.Get(path); ok {
			out = append(out, e)
		}
	}
	return out
}

// fillTagPage は、タグページについて、同じタグを宣言しているほかのノートを related へ入れる。
func (s *Store) fillTagPage(entry *model.Entry, related *Related) {
	key := tagPageKey(entry)
	if key == "" {
		return
	}
	for _, e := range s.tagPageEntries(key, entry.Path) {
		related.Duplicates = append(related.Duplicates, relatedPage("", e))
	}
}
