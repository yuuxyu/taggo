package search

import (
	"reflect"
	"testing"
)

func TestParseAndTerms(t *testing.T) {
	q := Parse(`#golang 設計 -#下書き`)
	if len(q.Terms) != 3 {
		t.Fatalf("条件 3 件を期待したが %d 件: %+v", len(q.Terms), q.Terms)
	}
	if got := q.Terms[0].Alternatives[0].Tag; got != "golang" {
		t.Fatalf("1 件目はタグ golang のはずが %+v", q.Terms[0])
	}
	if got := q.Terms[1].Alternatives[0].Text; got != "設計" {
		t.Fatalf("2 件目は本文検索 設計 のはずが %+v", q.Terms[1])
	}
	if !q.Terms[2].Negated || q.Terms[2].Alternatives[0].Tag != "下書き" {
		t.Fatalf("3 件目は NOT タグ 下書き のはずが %+v", q.Terms[2])
	}
}

func TestParseOrGroup(t *testing.T) {
	q := Parse(`#golang OR #rust 設計`)
	if len(q.Terms) != 2 {
		t.Fatalf("条件 2 件を期待したが %d 件: %+v", len(q.Terms), q.Terms)
	}
	if len(q.Terms[0].Alternatives) != 2 {
		t.Fatalf("OR グループに 2 件入るはずが %+v", q.Terms[0])
	}
	if q.Terms[0].Alternatives[1].Tag != "rust" {
		t.Fatalf("OR の 2 件目が rust でない: %+v", q.Terms[0])
	}
	if q.Terms[1].Alternatives[0].Text != "設計" {
		t.Fatalf("OR の後ろの語が独立した条件になっていない: %+v", q.Terms[1])
	}
}

func TestParseQuoted(t *testing.T) {
	q := Parse(`#"開発 メモ" "OR"`)
	if q.Terms[0].Alternatives[0].Tag != "開発 メモ" {
		t.Fatalf("引用符付きタグが 1 語になっていない: %+v", q.Terms[0])
	}
	if len(q.Terms) != 2 || q.Terms[1].Alternatives[0].Text != "OR" {
		t.Fatalf("引用符で囲んだ OR は検索語として扱うべき: %+v", q.Terms)
	}
}

func TestTrailingTagPrefix(t *testing.T) {
	cases := []struct {
		input  string
		prefix string
		ok     bool
	}{
		{"#", "", true},
		{"#go", "go", true},
		{"設計 #go", "go", true},
		{"-#go", "go", true},
		{"#go ", "", false},
		{"設計", "", false},
		{`#"開発`, "", false},
	}
	for _, c := range cases {
		prefix, ok := TrailingTagPrefix(c.input)
		if ok != c.ok || prefix != c.prefix {
			t.Errorf("TrailingTagPrefix(%q) = (%q, %v), want (%q, %v)", c.input, prefix, ok, c.prefix, c.ok)
		}
	}
}

func TestAppendTag(t *testing.T) {
	if got := AppendTag("", "golang"); got != "#golang " {
		t.Errorf("空の検索バーへの追加: %q", got)
	}
	if got := AppendTag("設計", "golang"); got != "設計 #golang " {
		t.Errorf("既存の語への追加: %q", got)
	}
	if got := AppendTag("#golang ", "golang"); got != "#golang " {
		t.Errorf("同じタグは重複追加しないはず: %q", got)
	}
	if got := AppendTag("", "開発 メモ"); got != `#"開発 メモ" ` {
		t.Errorf("空白を含むタグは引用符で囲むはず: %q", got)
	}
}

func TestReplaceTrailingTag(t *testing.T) {
	if got := ReplaceTrailingTag("設計 #go", "golang"); got != "設計 #golang " {
		t.Errorf("入力途中のタグ置換: %q", got)
	}
	if got := ReplaceTrailingTag("-#go", "golang"); got != "-#golang " {
		t.Errorf("NOT 付きタグの置換で - が失われた: %q", got)
	}
}

func TestQueryTags(t *testing.T) {
	q := Parse(`#a OR #b -#c d`)
	if want := []string{"a", "b"}; !reflect.DeepEqual(q.Tags(), want) {
		t.Fatalf("肯定条件のタグ一覧: got %v, want %v", q.Tags(), want)
	}
}
