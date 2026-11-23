package store

import (
	"sort"
	"strings"

	"github.com/tidwall/buntdb"
	"github.com/yuuxyu/taggo/internal/model"
)

// Entries は、読み込んだすべてのノートを order の順で返す。
// SortRelevance は検索語が無いと決まらないので、SortModifiedDesc と同じに扱う。
// 検索と違い、ピン留めしたノートを先頭に出すことはしない。
func (s *Store) Entries(order SortOrder) []*model.Entry {
	index, descending := idxModTime, true
	if order == SortNameAsc {
		index, descending = idxRelPath, false
	}
	out := make([]*model.Entry, 0, 64)
	visit := func(_, raw string) bool {
		if e := decodeEntry(raw); e != nil {
			out = append(out, e)
		}
		return true
	}
	_ = s.db.View(func(tx *buntdb.Tx) error {
		if descending {
			return tx.Descend(index, visit)
		}
		return tx.Ascend(index, visit)
	})
	return out
}

// TaggedWith は、本文に [[tag]] と書いてタグ tag を付けたノートを、パスの順で返す。
// タグの大文字小文字と空白の連なりは区別しない。
func (s *Store) TaggedWith(tag string) []*model.Entry {
	s.mu.RLock()
	paths := sortedPaths(s.tagged[model.TagKey(tag)])
	s.mu.RUnlock()
	return s.getAll(paths)
}

// Backlink は、あるノートを指しているノート 1 件。
type Backlink struct {
	Entry *model.Entry
	// ByTag は、指されているノートが表すタグ（golang.md なら golang）を、本文に [[ ]] で書いていること。
	ByTag bool
	// ByLink は、Markdown のリンクでリンクしていること。
	ByLink bool
}

// Backlinks は entry のノートを指しているノート（リンク元）を返す。entry は
// まだ無いページ（model.NewMissing）でもよい。
//
// 中身は関連ページのリンク元のグループ（Related の最初のグループ）と同じで、並び順も同じ
// （entry と重なるタグの多い順、同じなら更新日時の新しい順）。関連ページと違い、件数の上限は無い。
func (s *Store) Backlinks(entry *model.Entry) []Backlink {
	s.mu.RLock()
	byTag := s.tagged[tagPageKey(entry)]
	byLink := s.backlinks[noteKey(entry.Path)]
	paths := sortedPaths(unionPaths(byTag, byLink))
	vias := make(map[string]Backlink, len(paths))
	for _, path := range paths {
		_, tagged := byTag[path]
		_, linked := byLink[path]
		vias[path] = Backlink{ByTag: tagged, ByLink: linked}
	}
	s.mu.RUnlock()

	mine := make(map[string]struct{}, len(entry.Tags))
	for _, tag := range entry.Tags {
		mine[strings.ToLower(tag)] = struct{}{}
	}
	type candidate struct {
		link   Backlink
		shared int
	}
	found := make([]candidate, 0, len(paths))
	for _, path := range paths {
		if SameNote(path, entry.Path) {
			continue // 自分自身を指しているだけ
		}
		e, ok := s.Get(path)
		if !ok {
			continue
		}
		shared := 0
		for _, tag := range e.Tags {
			if _, ok := mine[strings.ToLower(tag)]; ok {
				shared++
			}
		}
		link := vias[path]
		link.Entry = e
		found = append(found, candidate{link: link, shared: shared})
	}
	sort.SliceStable(found, func(i, j int) bool {
		a, b := found[i], found[j]
		if a.shared != b.shared {
			return a.shared > b.shared
		}
		if !a.link.Entry.ModTime.Equal(b.link.Entry.ModTime) {
			return a.link.Entry.ModTime.After(b.link.Entry.ModTime)
		}
		return a.link.Entry.Path < b.link.Entry.Path
	})

	out := make([]Backlink, len(found))
	for i, c := range found {
		out[i] = c.link
	}
	return out
}

// TagStat は 1 つのタグの使われ方。
type TagStat struct {
	// Tag はタグの表記。本文で使われている表記を優先し、使われていなければページのファイル名の表記。
	Tag string
	// Count は、そのタグを付けたノートの数。ページはあるが誰も使っていなければ 0。
	Count int
	// Page はタグのページのパス。まだ無ければ空。
	Page string
}

// TagStats は、使われているタグと、ページのあるタグを返す。
// 並びと表記のまとめ方は TagNames と同じ（使われている件数の多い順、同じなら名前の順）。
func (s *Store) TagStats() []TagStat {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := make(map[string]*TagStat, len(s.tagCounts)+len(s.tagPages))
	for key, tag := range s.tagSpelling {
		stats[key] = &TagStat{Tag: tag, Count: s.tagCounts[key]}
	}
	for key, paths := range s.tagPages {
		page := pickOne(paths)
		if st, ok := stats[key]; ok {
			st.Page = page
			continue
		}
		stats[key] = &TagStat{Tag: model.PageTag(page), Page: page}
	}

	keys := make([]string, 0, len(stats))
	for key := range stats {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if ci, cj := stats[keys[i]].Count, stats[keys[j]].Count; ci != cj {
			return ci > cj
		}
		return keys[i] < keys[j]
	})
	out := make([]TagStat, len(keys))
	for i, key := range keys {
		out[i] = *stats[key]
	}
	return out
}

// getAll は paths のエントリを、読み込んでいるものだけ順に返す。
func (s *Store) getAll(paths []string) []*model.Entry {
	out := make([]*model.Entry, 0, len(paths))
	for _, path := range paths {
		if e, ok := s.Get(path); ok {
			out = append(out, e)
		}
	}
	return out
}
