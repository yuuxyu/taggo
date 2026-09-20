// Package search は検索バーに入力された文字列を、BuntDB 上で評価できる
// 問い合わせへ変換する。
//
// 検索バーの文法は、空白区切りの語をすべて満たすもの（AND）を既定とする。
//
//	#tag          そのタグを持つ
//	-#tag         そのタグを持たない（NOT）
//	#a OR #b      いずれかのタグを持つ（OR グループ）
//	語            タイトル・ファイル名・本文抜粋・タグへの部分一致
//	-語           上記に部分一致しない（NOT）
//	"複数 語"      引用符で囲むと空白を含めて 1 語として扱う
package search

import (
	"strings"
	"unicode"
)

// Query は解析済みの検索条件。すべての条件を満たすエントリが結果になる。
type Query struct {
	// Terms は AND で結合される条件。各要素は OR グループになっており、
	// グループ内のいずれかを満たせばその条件を満たしたとみなす。
	Terms []OrGroup
	// Raw は元の入力。UI へそのまま返す用途に使う。
	Raw string
}

// OrGroup は OR で結ばれた条件の集まり。
type OrGroup struct {
	Alternatives []Term
	// Negated が true なら、グループのいずれにも一致しないことを要求する。
	Negated bool
}

// Term は 1 つの検索語。
type Term struct {
	// Tag が空でなければタグ完全一致（大文字小文字は無視）。
	Tag string
	// Text が空でなければ部分一致検索。
	Text string
}

// IsEmpty は、絞り込み条件が 1 つも無いことを報告する。
func (q Query) IsEmpty() bool { return len(q.Terms) == 0 }

// Tags は、この問い合わせが肯定的に要求しているタグを返す。
// 候補表示や、検索バーへタグを追加する際の重複判定に使う。
func (q Query) Tags() []string {
	var out []string
	for _, g := range q.Terms {
		if g.Negated {
			continue
		}
		for _, t := range g.Alternatives {
			if t.Tag != "" {
				out = append(out, t.Tag)
			}
		}
	}
	return out
}

// Parse は検索バーの入力を Query へ変換する。
// 文法上おかしな入力でもエラーにはせず、解釈できた範囲を条件として返す。
// 1 文字ごとのインクリメンタル検索では、入力途中の文字列が必ず通るためである。
func Parse(input string) Query {
	q := Query{Raw: input}
	tokens := tokenize(input)

	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if tok.isOperator {
			continue // 先頭や末尾に置かれた OR は無視する
		}

		group := OrGroup{Negated: tok.negated}
		group.Alternatives = append(group.Alternatives, tok.term())

		// 後続が "OR" なら、その次の語を同じグループへ取り込む。
		for i+2 < len(tokens) && tokens[i+1].isOperator && !tokens[i+2].isOperator {
			group.Alternatives = append(group.Alternatives, tokens[i+2].term())
			i += 2
		}
		q.Terms = append(q.Terms, group)
	}
	return q
}

// token は字句解析の中間表現。
type token struct {
	text       string
	isTag      bool
	negated    bool
	isOperator bool // "OR"
	quoted     bool
}

func (t token) term() Term {
	if t.isTag {
		return Term{Tag: t.text}
	}
	return Term{Text: t.text}
}

// tokenize は入力を語へ分解する。引用符で囲まれた部分は空白を含めて 1 語になる。
func tokenize(input string) []token {
	var (
		out      []token
		cur      strings.Builder
		inQuote  bool
		wasQuote bool // 直前に閉じた語が引用符で囲まれていたか
		negated  bool
		isTag    bool
		started  bool
	)

	flush := func() {
		if !started {
			return
		}
		text := cur.String()
		cur.Reset()
		started = false
		defer func() { negated, isTag, wasQuote = false, false, false }()

		if text == "" {
			// 「#」だけを入力した状態。タグ候補を出すために空タグとして残す。
			if isTag {
				out = append(out, token{isTag: true})
			}
			return
		}
		// 引用符で囲まれた OR は演算子ではなく、検索語としての "OR" を意味する。
		if !isTag && !wasQuote && strings.EqualFold(text, "OR") {
			out = append(out, token{isOperator: true})
			return
		}
		out = append(out, token{text: text, isTag: isTag, negated: negated, quoted: wasQuote})
	}

	for _, r := range input {
		switch {
		case r == '"':
			if inQuote {
				inQuote = false
				wasQuote = true
				flush()
			} else {
				inQuote = true
				started = true
			}
		case inQuote:
			cur.WriteRune(r)
		case unicode.IsSpace(r):
			flush()
		case r == '-' && !started:
			negated = true
			started = true
		case r == '#' && !isTag && cur.Len() == 0:
			isTag = true
			started = true
		default:
			cur.WriteRune(r)
			started = true
		}
	}
	flush()
	return out
}

// TrailingTagPrefix は、入力の末尾が入力途中のタグ（"#" で始まり空白で終わっていない）
// であればその接頭辞を返す。オートコンプリートの候補を出すかどうかの判定に使う。
func TrailingTagPrefix(input string) (string, bool) {
	// 末尾の語だけを見ればよい。引用符の中ならタグ入力ではない。
	if strings.Count(input, `"`)%2 == 1 {
		return "", false
	}
	idx := strings.LastIndexFunc(input, unicode.IsSpace)
	last := input[idx+1:]
	last = strings.TrimPrefix(last, "-")
	if !strings.HasPrefix(last, "#") {
		return "", false
	}
	return strings.TrimPrefix(last, "#"), true
}

// ReplaceTrailingTag は、入力途中のタグを確定したタグで置き換えた文字列を返す。
// オートコンプリートで候補を選んだときに使う。
func ReplaceTrailingTag(input, tag string) string {
	idx := strings.LastIndexFunc(input, unicode.IsSpace)
	head := input[:idx+1]
	last := input[idx+1:]
	prefix := ""
	if strings.HasPrefix(last, "-") {
		prefix = "-"
	}
	return head + prefix + "#" + quoteIfNeeded(tag) + " "
}

// AppendTag は、検索バーの文字列に tag を AND 条件として追加する。
// すでに同じタグが条件に入っている場合は何も足さない。
// カードのタグバッジをクリックしたときの挙動に使う。
func AppendTag(input, tag string) string {
	for _, existing := range Parse(input).Tags() {
		if strings.EqualFold(existing, tag) {
			return input
		}
	}
	trimmed := strings.TrimRight(input, " ")
	if trimmed != "" {
		trimmed += " "
	}
	return trimmed + "#" + quoteIfNeeded(tag) + " "
}

// quoteIfNeeded は、空白を含むタグを引用符で囲む。
func quoteIfNeeded(tag string) string {
	if strings.ContainsFunc(tag, unicode.IsSpace) {
		return `"` + tag + `"`
	}
	return tag
}
