package store

import (
	"strings"

	"github.com/tidwall/buntdb"
	"github.com/yuuxyu/taggo/internal/model"
)

// taggedLimit は、タグページの右の列に「このタグが付いたファイル」として出す件数の上限。
// 残りは、そのタグで絞り込んだ一覧で見てもらう。
const taggedLimit = 30

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
	if e.Kind != model.KindMarkdown || e.TagPage == "" {
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

// taggedWith は tag が付いたファイルを、種類を問わず更新日時の新しい順に返す。
// exclude に含まれるパスは除く。返すのは上限件数までで、total は上限を超えた分も含む件数。
//
// タグの逆引き索引は持っていないので、同じタグのノートと同じく全件をなめる。
func (s *Store) taggedWith(tag string, exclude map[string]struct{}) (pages []*model.Entry, total int) {
	pages = []*model.Entry{}
	_ = s.db.View(func(tx *buntdb.Tx) error {
		return tx.Descend(idxModTime, func(_, raw string) bool {
			e := decodeEntry(raw)
			if e == nil {
				return true
			}
			if _, skip := exclude[e.Path]; skip {
				return true
			}
			if !hasTag(e, tag) {
				return true
			}
			total++
			if len(pages) < taggedLimit {
				pages = append(pages, e)
			}
			return true
		})
	})
	return pages, total
}

// fillTagPage は、タグページについて「このタグが付いたファイル」と、
// 同じタグを宣言しているほかのノートを related へ入れる。
// 一覧に出したファイルは seen へ足し、同じタグのノートの欄と重ならないようにする。
func (s *Store) fillTagPage(entry *model.Entry, related *Related, seen map[string]struct{}) {
	key := tagPageKey(entry)
	if key == "" {
		return
	}

	for _, e := range s.tagPageEntries(key, entry.Path) {
		related.Duplicates = append(related.Duplicates, relatedPage("", e))
	}

	// 同じタグを宣言しているノートは重複の欄に出すので、ここでは数えない。
	exclude := map[string]struct{}{entry.Path: {}}
	for _, p := range related.Duplicates {
		exclude[p.Path] = struct{}{}
	}
	tagged, total := s.taggedWith(entry.TagPage, exclude)
	for _, e := range tagged {
		related.Tagged = append(related.Tagged, relatedPage("", e))
		seen[e.Path] = struct{}{}
	}
	related.TaggedTotal = total
}
