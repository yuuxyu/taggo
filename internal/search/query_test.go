package search

import (
	"reflect"
	"testing"
)

// 空白で区切った語を、小文字にそろえて取り出すこと。記号は特別扱いしない。
func TestParseWords(t *testing.T) {
	q := Parse("  Golang　設計 #下書き OR -除外 ")
	want := []string{"golang", "設計", "#下書き", "or", "-除外"}
	if !reflect.DeepEqual(q.Words, want) {
		t.Fatalf("語の分け方が違う: got %v, want %v", q.Words, want)
	}
}

// 入力全体をタグとして読んだキーを持つこと。空白の揺れと大文字小文字は吸収する。
func TestParseTagKey(t *testing.T) {
	cases := map[string]string{
		"Golang":      "golang",
		"  開発   メモ  ": "開発 メモ",
		"":            "",
	}
	for input, want := range cases {
		if got := Parse(input).TagKey; got != want {
			t.Errorf("Parse(%q).TagKey = %q, want %q", input, got, want)
		}
	}
}

func TestParseEmpty(t *testing.T) {
	for _, input := range []string{"", "   ", "\t"} {
		if !Parse(input).IsEmpty() {
			t.Errorf("Parse(%q) は空のはず", input)
		}
	}
	if Parse("a").IsEmpty() {
		t.Error(`Parse("a") は空ではないはず`)
	}
}
