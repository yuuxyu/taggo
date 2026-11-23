package store

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/yuuxyu/taggo/internal/model"
)

// names はエントリのファイル名を並べる。
func names(entries []*model.Entry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = filepath.Base(e.Path)
	}
	return out
}

func seedRoot(t *testing.T, s *Store, entries ...*model.Entry) {
	t.Helper()
	if err := s.Reset("root"); err != nil {
		t.Fatal(err)
	}
	if err := s.PutAll(entries); err != nil {
		t.Fatal(err)
	}
}

// すべてのノートを、更新日時の新しい順とファイル名の順で並べられること。
// ピン留めしていても、先頭に出さないこと。
func TestEntries(t *testing.T) {
	s := newTestStore(t)
	seedRoot(t, s,
		rootEntry("b.md", 1, nil),
		rootEntry("a.md", 2, nil),
		rootEntry("c.md", 0, nil),
	)
	s.SetPins([]string{filepath.Join("root", "a.md")})

	if got := names(s.Entries(SortModifiedDesc)); !slices.Equal(got, []string{"c.md", "b.md", "a.md"}) {
		t.Fatalf("更新日時の順になっていない: %v", got)
	}
	if got := names(s.Entries(SortNameAsc)); !slices.Equal(got, []string{"a.md", "b.md", "c.md"}) {
		t.Fatalf("ファイル名の順になっていない: %v", got)
	}
}

// タグを付けたノートだけを、表記の違いを区別せずに引けること。
func TestTaggedWith(t *testing.T) {
	s := newTestStore(t)
	seedRoot(t, s,
		rootEntry("a.md", 0, []string{"Go 言語"}),
		rootEntry("b.md", 0, []string{"go 言語", "設計"}),
		rootEntry("c.md", 0, []string{"設計"}),
		rootEntry("Go 言語.md", 0, nil), // タグのページ自身はタグを付けていない
	)

	if got := names(s.TaggedWith("go  言語")); !slices.Equal(got, []string{"a.md", "b.md"}) {
		t.Fatalf("タグを付けたノートが違う: %v", got)
	}
	if got := s.TaggedWith("無いタグ"); len(got) != 0 {
		t.Fatalf("使われていないタグでノートが返った: %v", names(got))
	}
}

// リンク元は、タグで指しているものとリンクで指しているものを合わせ、どちらで指しているかを返すこと。
// 並びは関連ページと同じく、重なるタグの多い順、同じなら更新日時の新しい順。
func TestBacklinks(t *testing.T) {
	s := newTestStore(t)
	page := rootEntry("Golang.md", 9, []string{"言語"})
	seedRoot(t, s,
		page,
		rootEntry("a.md", 2, []string{"golang"}),
		rootEntry("b.md", 1, []string{"golang", "言語"}),
		rootEntry("both.md", 3, []string{"golang"}, "./Golang.md"),
		rootEntry("linking.md", 0, nil, "Golang.md"),
		rootEntry("other.md", 0, []string{"言語"}),
	)

	got := s.Backlinks(page)
	var order []string
	for _, b := range got {
		order = append(order, filepath.Base(b.Entry.Path))
	}
	if want := []string{"b.md", "linking.md", "a.md", "both.md"}; !slices.Equal(order, want) {
		t.Fatalf("リンク元の並びが違う: %v（want %v）", order, want)
	}
	via := map[string][2]bool{}
	for _, b := range got {
		via[filepath.Base(b.Entry.Path)] = [2]bool{b.ByTag, b.ByLink}
	}
	if via["both.md"] != [2]bool{true, true} || via["a.md"] != [2]bool{true, false} || via["linking.md"] != [2]bool{false, true} {
		t.Fatalf("どちらで指しているかが違う: %v", via)
	}

	// まだ無いページも、そのページを指しているノートを引けること。
	missing := model.NewMissing(filepath.Join("root", "未整理.md"))
	if err := s.Put(rootEntry("draft.md", 0, []string{"未整理"})); err != nil {
		t.Fatal(err)
	}
	if got := s.Backlinks(missing); len(got) != 1 || filepath.Base(got[0].Entry.Path) != "draft.md" {
		t.Fatalf("まだ無いページのリンク元が違う: %+v", got)
	}
}

// タグを、件数とページの有無と一緒に返すこと。ページだけあって使われていないタグも含める。
func TestTagStats(t *testing.T) {
	s := newTestStore(t)
	seedRoot(t, s,
		rootEntry("a.md", 0, []string{"Golang", "設計"}),
		rootEntry("b.md", 0, []string{"golang"}),
		rootEntry("golang.md", 0, nil),
		rootEntry("孤立.md", 0, nil),
	)

	got := map[string]TagStat{}
	for _, st := range s.TagStats() {
		got[st.Tag] = st
	}
	if st := got["Golang"]; st.Count != 2 || filepath.Base(st.Page) != "golang.md" {
		t.Fatalf("golang の集計が違う: %+v（全体 %+v）", st, got)
	}
	if st := got["設計"]; st.Count != 1 || st.Page != "" {
		t.Fatalf("設計 の集計が違う: %+v", st)
	}
	if st, ok := got["孤立"]; !ok || st.Count != 0 || filepath.Base(st.Page) != "孤立.md" {
		t.Fatalf("ページだけのタグが含まれていない: %+v", st)
	}
	if first := s.TagStats()[0]; first.Tag != "Golang" {
		t.Fatalf("件数の多い順になっていない: %+v", first)
	}
}
