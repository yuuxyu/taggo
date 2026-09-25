// Package search は検索バーに入力された文字列を、BuntDB 上で評価できる
// 問い合わせへ変換する。
//
// 検索は単純な全文検索とする。タグを指定する記法や、AND / OR / NOT のような
// 条件式は持たない。
//
//   - 空白で区切った語を、すべて含むノートが結果になる（大文字小文字は区別しない）。
//     語はタイトル・ファイル名・本文の抜粋・タグから探す。
//   - 入力全体がタグの名前と一致するときは、そのタグを表すページ
//     （ファイル名がそのタグのノート）を結果の先頭に出す。これは store 側で行う。
package search

import (
	"strings"

	"github.com/yuuxyu/taggo/internal/model"
)

// Query は解析済みの検索条件。
type Query struct {
	// Words は部分一致で探す語。小文字にそろえてある。すべてを含むものが結果になる。
	Words []string
	// TagKey は入力全体をタグとして読んだときのキー（model.TagKey）。
	// これと同じ名前のページを、結果の先頭に出すために使う。
	TagKey string
	// Raw は元の入力。
	Raw string
}

// IsEmpty は、絞り込み条件が 1 つも無いことを報告する。
func (q Query) IsEmpty() bool { return len(q.Words) == 0 }

// Parse は検索バーの入力を Query へ変換する。
// どんな入力でもエラーにはしない。1 文字ごとのインクリメンタル検索では、
// 入力途中の文字列が必ず通るためである。
func Parse(input string) Query {
	words := strings.Fields(strings.ToLower(input))
	return Query{
		Words:  words,
		TagKey: model.TagKey(input),
		Raw:    input,
	}
}
