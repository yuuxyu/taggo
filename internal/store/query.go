package store

import (
	"sort"
	"strings"

	"github.com/tidwall/buntdb"
	"github.com/yuuxyu/taggo/internal/model"
	"github.com/yuuxyu/taggo/internal/search"
)

// Result は 1 回の検索の結果。
type Result struct {
	// Entries は絞り込み後、指定された並び順に整列したエントリ。
	// 先頭の Head 件は並び順とは別枠で、その後ろに残りが並ぶ。
	Entries []*model.Entry `json:"entries"`
	// Total は絞り込み後の総件数。ページングしても全体件数が分かるようにする。
	Total int `json:"total"`
	// Head は Entries の先頭のうち、並び順とは別枠で先に並べた件数。
	// 検索語が無いときはピン留めしたノート、あるときは検索語と同じ名前のタグのページ。
	// 画面ではこの件数ぶんを 1 つのかたまりとして、残りと行を分けて並べる。
	Head int `json:"head"`
	// Pins はピン留めしているノートのうち、読み込み済みのもののパス。ピン留めした順に並ぶ。
	// 検索で絞り込んでいても、カードにピン留めの印を出すために全件を返す。
	Pins []string `json:"pins"`
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
//
// 検索語が無いときは、ピン留めしたノートをピン留めした順に先頭へ出す。
// 検索語があるときは、入力全体と同じ名前のタグのページを先頭へ出し、
// 残りは全文検索の結果を並び順どおりに並べる。
func (s *Store) Search(opts SearchOptions) (Result, error) {
	q := search.Parse(opts.Query)
	order := s.pinOrder()

	// 並び順に対応するインデックスを選ぶ。関連度順だけは絞り込み後に自前で並べる。
	// 名前順はタイトル（見出し）ではなくファイル名で並べる。
	index := idxModTime
	descending := true
	switch opts.Sort {
	case SortNameAsc:
		index, descending = idxRelPath, false
	case SortRelevance:
		index, descending = idxModTime, true
	}

	matched := make([]*model.Entry, 0, 64)
	var pinned, tagPages []*model.Entry
	visit := func(_, raw string) bool {
		e := decodeEntry(raw)
		if e == nil {
			return true
		}
		_, isPinned := order[strings.ToLower(e.Path)]
		if isPinned {
			pinned = append(pinned, e)
		}
		switch {
		case q.IsEmpty() && isPinned:
			// 先頭の別枠へ回す
		case !q.IsEmpty() && q.TagKey != "" && tagPageKey(e) == q.TagKey:
			tagPages = append(tagPages, e)
		case matches(e, q):
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
	sort.SliceStable(pinned, func(i, j int) bool {
		return order[strings.ToLower(pinned[i].Path)] < order[strings.ToLower(pinned[j].Path)]
	})
	pins := make([]string, len(pinned))
	for i, e := range pinned {
		pins[i] = e.Path
	}

	head := tagPages
	if q.IsEmpty() {
		head = pinned
	} else {
		sort.Slice(head, func(i, j int) bool { return head[i].Path < head[j].Path })
	}
	all := append(head[:len(head):len(head)], matched...)

	total := len(all)
	window := applyWindow(all, opts.Offset, opts.Limit)
	return Result{
		Entries: window,
		Total:   total,
		Head:    min(max(len(head)-max(opts.Offset, 0), 0), len(window)),
		Pins:    pins,
	}, nil
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

// matches は、検索語をすべて含むエントリかを判定する。
// 語はタイトル・ファイル名・本文の抜粋・タグから、大文字小文字を区別せずに探す。
func matches(e *model.Entry, q search.Query) bool {
	if q.IsEmpty() {
		return true
	}
	title := strings.ToLower(e.Title)
	name := strings.ToLower(e.Name)
	preview := strings.ToLower(e.Preview)
	for _, word := range q.Words {
		if strings.Contains(title, word) ||
			strings.Contains(name, word) ||
			strings.Contains(preview, word) ||
			containsFold(e.Tags, word) {
			continue
		}
		return false
	}
	return true
}

// sortByRelevance は関連度順に並べ替える。
// タイトル先頭一致を最上位、次にタイトル一致、タグ一致、本文一致の順とし、
// 同点なら更新日時の新しい順にする。
func sortByRelevance(entries []*model.Entry, q search.Query) {
	texts := q.Words
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
