package store

import (
	"sort"
	"strings"

	"github.com/yuuxyu/taggo/internal/model"
)

// TagSuggestion は検索バーのオートコンプリートに出す 1 候補。
type TagSuggestion struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

// Tags は登録済みの全タグを、使用件数の多い順（同数ならタグ名順）で返す。
// prefix を指定すると、その前方一致に絞る。limit が 0 以下なら全件返す。
func (s *Store) Tags(prefix string, limit int) []TagSuggestion {
	s.mu.RLock()
	defer s.mu.RUnlock()

	needle := strings.ToLower(strings.TrimSpace(prefix))
	out := make([]TagSuggestion, 0, len(s.tagCounts))
	for key, count := range s.tagCounts {
		if needle != "" && !strings.HasPrefix(key, needle) {
			continue
		}
		out = append(out, TagSuggestion{Tag: s.tagSpelling[key], Count: count})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Tag < out[j].Tag
	})

	if limit > 0 && limit < len(out) {
		out = out[:limit]
	}
	return out
}

// Backlink は、あるノートを参照している Markdown ファイル 1 件の情報。
type Backlink struct {
	Path  string `json:"path"`
	Title string `json:"title"`
}

// Backlinks は path を WikiLink で参照している Markdown エントリを返す。
// WikiLink の参照先はファイル名（拡張子なし）かタイトルで書かれるため、
// その両方の表記でメモリ内グラフを引く。
func (s *Store) Backlinks(entry *model.Entry) []Backlink {
	if entry == nil {
		return nil
	}

	names := map[string]struct{}{}
	base := strings.TrimSuffix(entry.Name, entry.Ext)
	names[strings.ToLower(base)] = struct{}{}
	names[strings.ToLower(entry.Title)] = struct{}{}

	s.mu.RLock()
	sources := map[string]struct{}{}
	for name := range names {
		for path := range s.backlinks[name] {
			if path == entry.Path {
				continue // 自分自身への参照は表示しない
			}
			sources[path] = struct{}{}
		}
	}
	s.mu.RUnlock()

	out := make([]Backlink, 0, len(sources))
	for path := range sources {
		if e, ok := s.Get(path); ok {
			out = append(out, Backlink{Path: e.Path, Title: e.Title})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Title < out[j].Title })
	return out
}
