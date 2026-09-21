package store

import (
	"sort"
	"strings"
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
