package store

import (
	"strings"

	"github.com/tidwall/buntdb"
	"github.com/yuuxyu/taggo/internal/model"
	"github.com/yuuxyu/taggo/internal/search"
)

// Result は 1 回の検索の結果。
type Result struct {
	// Entries は絞り込み後、指定された並び順に整列したエントリ。
	Entries []*model.Entry `json:"entries"`
	// Total は絞り込み後の総件数。ページングしても全体件数が分かるようにする。
	Total int `json:"total"`
}

// SearchOptions は検索の付帯条件。
type SearchOptions struct {
	Query  string    `json:"query"`
	Sort   SortOrder `json:"sort"`
	Offset int       `json:"offset"`
	// Limit が 0 以下なら全件返す。仮想スクロールは全件を受け取って
	// 描画側で間引くため、通常は 0 のまま使う。
	Limit int `json:"limit"`
}

// Search は検索バーの入力でエントリを絞り込み、指定順に並べて返す。
func (s *Store) Search(opts SearchOptions) (Result, error) {
	q := search.Parse(opts.Query)

	// 並び順に対応するインデックスを選ぶ。関連度順だけは絞り込み後に自前で並べる。
	index := idxModTime
	descending := true
	switch opts.Sort {
	case SortNameAsc:
		index, descending = idxTitle, false
	case SortRelevance:
		index, descending = idxModTime, true
	}

	matched := make([]*model.Entry, 0, 64)
	visit := func(_, raw string) bool {
		e := decodeEntry(raw)
		if e == nil {
			return true
		}
		if matches(e, q) {
			matched = append(matched, e)
		}
		return true
	}

	err := s.db.View(func(tx *buntdb.Tx) error {
		if descending {
			return tx.Descend(index, visit)
		}
		return tx.Ascend(index, visit)
	})
	if err != nil {
		return Result{}, err
	}

	if opts.Sort == SortRelevance {
		sortByRelevance(matched, q)
	}

	total := len(matched)
	matched = applyWindow(matched, opts.Offset, opts.Limit)
	return Result{Entries: matched, Total: total}, nil
}

// applyWindow は offset / limit を適用する。範囲外の指定は空結果として扱う。
func applyWindow(entries []*model.Entry, offset, limit int) []*model.Entry {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(entries) {
		return []*model.Entry{}
	}
	entries = entries[offset:]
	if limit > 0 && limit < len(entries) {
		entries = entries[:limit]
	}
	return entries
}

// matches は 1 エントリが問い合わせ全体（AND 結合）を満たすかを判定する。
func matches(e *model.Entry, q search.Query) bool {
	for _, group := range q.Terms {
		hit := false
		for _, term := range group.Alternatives {
			if matchTerm(e, term) {
				hit = true
				break
			}
		}
		if hit == group.Negated {
			// 肯定条件なのに一致しない、または否定条件なのに一致した。
			return false
		}
	}
	return true
}

// matchTerm は 1 つの検索語がエントリに一致するかを判定する。
func matchTerm(e *model.Entry, t search.Term) bool {
	if t.Tag != "" {
		return hasTag(e, t.Tag)
	}
	if t.Text == "" {
		return true
	}
	needle := strings.ToLower(t.Text)
	if strings.Contains(strings.ToLower(e.Title), needle) ||
		strings.Contains(strings.ToLower(e.Name), needle) ||
		strings.Contains(strings.ToLower(e.RelPath), needle) ||
		strings.Contains(strings.ToLower(e.Preview), needle) {
		return true
	}
	// タグは本文検索でも拾えたほうが直感に合う。
	for _, tag := range e.Tags {
		if strings.Contains(strings.ToLower(tag), needle) {
			return true
		}
	}
	return false
}

// hasTag はタグの完全一致（大文字小文字は無視）を判定する。
// 部分一致にすると "#go" が "#golang" を巻き込んでしまい、絞り込みとして使えなくなる。
func hasTag(e *model.Entry, tag string) bool {
	for _, t := range e.Tags {
		if strings.EqualFold(t, tag) {
			return true
		}
	}
	return false
}

// sortByRelevance は関連度順に並べ替える。
// タイトル先頭一致を最上位、次にタイトル一致、タグ一致、本文一致の順とし、
// 同点なら更新日時の新しい順にする。
func sortByRelevance(entries []*model.Entry, q search.Query) {
	texts := make([]string, 0, len(q.Terms))
	for _, g := range q.Terms {
		if g.Negated {
			continue
		}
		for _, t := range g.Alternatives {
			if t.Text != "" {
				texts = append(texts, strings.ToLower(t.Text))
			}
		}
	}
	if len(texts) == 0 {
		return // 本文検索語が無いなら、既に整っている更新日時順のままでよい
	}

	score := func(e *model.Entry) int {
		best := 0
		title := strings.ToLower(e.Title)
		preview := strings.ToLower(e.Preview)
		for _, needle := range texts {
			switch {
			case strings.HasPrefix(title, needle):
				best = max(best, 4)
			case strings.Contains(title, needle):
				best = max(best, 3)
			case containsFold(e.Tags, needle):
				best = max(best, 2)
			case strings.Contains(preview, needle):
				best = max(best, 1)
			}
		}
		return best
	}

	// 既に更新日時の降順に並んでいるので、安定ソートで同点の順序を保つ。
	stableSortDesc(entries, score)
}

func containsFold(list []string, needle string) bool {
	for _, v := range list {
		if strings.Contains(strings.ToLower(v), needle) {
			return true
		}
	}
	return false
}

// stableSortDesc は score の降順に安定ソートする。
func stableSortDesc(entries []*model.Entry, score func(*model.Entry) int) {
	scores := make([]int, len(entries))
	for i, e := range entries {
		scores[i] = score(e)
	}
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && scores[j] > scores[j-1]; j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
			scores[j], scores[j-1] = scores[j-1], scores[j]
		}
	}
}
