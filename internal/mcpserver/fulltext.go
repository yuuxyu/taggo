package mcpserver

import (
	"bytes"
	"context"
	"runtime"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/yuuxyu/taggo/internal/meta"
	"github.com/yuuxyu/taggo/internal/model"
	"github.com/yuuxyu/taggo/internal/notefile"
	"github.com/yuuxyu/taggo/internal/search"
	"github.com/yuuxyu/taggo/internal/store"
)

const (
	// maxLineMatches は、1 つのノートについて返す、語が見つかった行の数の上限。
	maxLineMatches = 3
	// snippetRunes は、見つかった行を切り詰める長さ（文字数）。
	snippetRunes = 120
	// snippetLead は、行を切り詰めるときに、語の前に残す文字数。
	snippetLead = 40
)

// lineMatch は、語が見つかった行 1 つ。
type lineMatch struct {
	Line int    `json:"line" jsonschema:"ファイルの行番号（1 から）"`
	Text string `json:"text" jsonschema:"見つかった行（長ければ語の前後で切り詰める）"`
}

// fullTextHit は、全文検索で見つかったノート 1 件。
type fullTextHit struct {
	entry   *model.Entry
	score   int
	matches []lineMatch
	// head は、検索語と同じ名前のタグのページであること。結果の先頭に出す。
	head bool
}

// fullTextSearch は、語をすべて含むノートを、本文全体から探す。
//
// 語の読み方（search.Parse）と並び順は、画面の検索の関連度順と同じにする。ただし画面が
// 本文の抜粋（240 文字）までしか見ないのに対し、ここでは本文全体を探す。本文は Front Matter を
// 除いて探し、コードブロックの中も探す（コードを探したいこともあるため）。
//
// 本文はメモリに持たず、呼ばれるたびに読み込み済みのノートを並列で読む。
func fullTextSearch(ctx context.Context, s *store.Store, query string) ([]fullTextHit, error) {
	q := search.Parse(query)
	entries := s.Entries(store.SortModifiedDesc)

	hits := make([]*fullTextHit, len(entries))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range runtime.NumCPU() {
		wg.Go(func() {
			for i := range jobs {
				hits[i] = matchNote(entries[i], q)
			}
		})
	}
	var err error
send:
	for i := range entries {
		select {
		case jobs <- i:
		case <-ctx.Done():
			err = ctx.Err()
			break send
		}
	}
	close(jobs)
	wg.Wait()
	if err != nil {
		return nil, err
	}

	out := make([]fullTextHit, 0, 16)
	for _, h := range hits {
		if h != nil {
			out = append(out, *h)
		}
	}
	// 更新日時の新しい順に並んでいるので、安定ソートで同点の順を保つ。
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].head != out[j].head {
			return out[i].head
		}
		if out[i].head {
			return out[i].entry.Path < out[j].entry.Path
		}
		return out[i].score > out[j].score
	})
	return out, nil
}

// matchNote は、1 つのノートが語をすべて含むかを確かめ、含めば見つかった行と点数を返す。
// 含まなければ nil を返す。
//
// 点数は画面の関連度順と同じで、タイトルの先頭一致 4、タイトル 3、タグ 2、本文 1 のうち、
// 語ごとの最も高いものを使う。
func matchNote(e *model.Entry, q search.Query) *fullTextHit {
	head := q.TagKey != "" && model.TagKey(model.PageTag(e.Path)) == q.TagKey
	title := strings.ToLower(e.Title)
	name := strings.ToLower(e.Name)

	body, lineOffset := readBody(e.Path)
	lowerBody := strings.ToLower(body)

	score := 0
	for _, word := range q.Words {
		switch {
		case strings.HasPrefix(title, word):
			score = max(score, 4)
		case strings.Contains(title, word):
			score = max(score, 3)
		case containsFold(e.Tags, word):
			score = max(score, 2)
		case strings.Contains(lowerBody, word), strings.Contains(name, word):
			score = max(score, 1)
		default:
			if !head {
				return nil
			}
		}
	}
	return &fullTextHit{
		entry:   e,
		score:   score,
		matches: matchLines(body, lineOffset, q.Words),
		head:    head,
	}
}

// readBody は、ノートの本文を、先頭の BOM と Front Matter を除いて読む。lineOffset は、
// 除いた Front Matter の行数（見つかった行の番号を、ファイルの行番号にそろえるため）。
// 読めなければ空を返す（タイトルとタグだけで探す）。
func readBody(path string) (body string, lineOffset int) {
	raw, err := notefile.ReadLimited(path)
	if err != nil {
		return "", 0
	}
	raw = bytes.TrimPrefix(raw, notefile.BOM)
	stripped := meta.StripFrontMatter(raw)
	lineOffset = bytes.Count(raw[:len(raw)-len(stripped)], []byte("\n"))
	return string(stripped), lineOffset
}

// matchLines は、語のどれかを含む行を、先頭から maxLineMatches 行まで返す。
func matchLines(body string, lineOffset int, words []string) []lineMatch {
	out := []lineMatch{}
	if len(words) == 0 {
		return out
	}
	n := lineOffset
	for line := range strings.SplitSeq(body, "\n") {
		n++
		line = strings.TrimRight(line, "\r")
		if at := indexWords(line, words); at >= 0 {
			out = append(out, lineMatch{Line: n, Text: snippet(line, at)})
			if len(out) == maxLineMatches {
				break
			}
		}
	}
	return out
}

// indexWords は、行の中で語のどれかが最初に現れる位置（文字数）を返す。無ければ -1。
// 大文字小文字は区別しない。文字ごとに小文字にするので、位置は元の行の文字数とそろう。
func indexWords(line string, words []string) int {
	lower := []rune(line)
	for i, r := range lower {
		lower[i] = unicode.ToLower(r)
	}
	lowerLine := string(lower)
	best := -1
	for _, word := range words {
		if at := strings.Index(lowerLine, word); at >= 0 {
			runeAt := utf8.RuneCountInString(lowerLine[:at])
			if best < 0 || runeAt < best {
				best = runeAt
			}
		}
	}
	return best
}

// snippet は行を、位置 at の語が入るように snippetRunes 文字ほどに切り詰める。
func snippet(line string, at int) string {
	r := []rune(strings.TrimSpace(line))
	// 前の空白を除いた分、位置もずらす。
	at -= utf8.RuneCountInString(line) - utf8.RuneCountInString(strings.TrimLeftFunc(line, unicode.IsSpace))
	if len(r) <= snippetRunes {
		return string(r)
	}
	start := min(max(at-snippetLead, 0), len(r)-snippetRunes)
	end := start + snippetRunes
	text := string(r[start:end])
	if start > 0 {
		text = "…" + text
	}
	if end < len(r) {
		text += "…"
	}
	return text
}

func containsFold(list []string, needle string) bool {
	for _, v := range list {
		if strings.Contains(strings.ToLower(v), needle) {
			return true
		}
	}
	return false
}
