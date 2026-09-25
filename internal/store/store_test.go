package store

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yuuxyu/taggo/internal/model"
)

// newEntry はテスト用のエントリを組み立てる。更新日時は日数で指定する。
func newEntry(path, title string, daysAgo int, tags []string) *model.Entry {
	return &model.Entry{
		Path:    path,
		RelPath: path,
		Name:    path,
		Ext:     ".md",
		ModTime: time.Now().AddDate(0, 0, -daysAgo),
		Tags:    model.NormalizeTags(tags),
		Title:   title,
	}
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open()
	if err != nil {
		t.Fatalf("ストアの初期化に失敗: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func seed(t *testing.T, s *Store) {
	t.Helper()
	entries := []*model.Entry{
		newEntry("a.md", "Go の設計メモ", 1, []string{"golang", "設計"}),
		newEntry("b.md", "Rust の設計メモ", 2, []string{"rust", "設計"}),
		newEntry("c.md", "下書き", 3, []string{"golang", "下書き"}),
	}
	entries[0].Preview = "インターフェースの使い分けについて"
	if err := s.PutAll(entries); err != nil {
		t.Fatalf("投入に失敗: %v", err)
	}
}

func paths(r Result) []string {
	out := make([]string, 0, len(r.Entries))
	for _, e := range r.Entries {
		out = append(out, e.Path)
	}
	return out
}

// 空白で区切った語は、すべて含むものだけに絞り込むこと。
func TestSearchWordsAnd(t *testing.T) {
	s := newTestStore(t)
	seed(t, s)

	r, err := s.Search(SearchOptions{Query: "golang 設計"})
	if err != nil {
		t.Fatalf("検索に失敗: %v", err)
	}
	if got := paths(r); len(got) != 1 || got[0] != "a.md" {
		t.Fatalf("すべての語を含むものだけになっていない: %v", got)
	}
}

// "#" や "OR" や "-" は特別な意味を持たず、ただの文字として探すこと。
func TestSearchHasNoOperators(t *testing.T) {
	s := newTestStore(t)
	seed(t, s)

	for _, q := range []string{"#golang", "golang OR rust", "-下書き"} {
		r, _ := s.Search(SearchOptions{Query: q})
		if r.Total != 0 {
			t.Fatalf("%q が演算子として解釈されている: %v", q, paths(r))
		}
	}
}

// タグは部分一致でも拾うこと。
func TestSearchMatchesTagsPartially(t *testing.T) {
	s := newTestStore(t)
	seed(t, s)

	r, _ := s.Search(SearchOptions{Query: "lang"})
	if r.Total != 2 {
		t.Fatalf("タグへの部分一致が効いていない: %v", paths(r))
	}
}

func TestSearchFreeText(t *testing.T) {
	s := newTestStore(t)
	seed(t, s)

	r, _ := s.Search(SearchOptions{Query: "インターフェース"})
	if got := paths(r); len(got) != 1 || got[0] != "a.md" {
		t.Fatalf("本文抜粋への部分一致が効いていない: %v", got)
	}

	r, _ = s.Search(SearchOptions{Query: "設計メモ"})
	if r.Total != 2 {
		t.Fatalf("タイトルへの部分一致が効いていない: %v", paths(r))
	}
}

func TestSearchSortOrders(t *testing.T) {
	s := newTestStore(t)
	seed(t, s)

	r, _ := s.Search(SearchOptions{Sort: SortModifiedDesc})
	if got := paths(r); got[0] != "a.md" || got[2] != "c.md" {
		t.Fatalf("更新日時の降順になっていない: %v", got)
	}

	r, _ = s.Search(SearchOptions{Sort: SortNameAsc})
	if got := paths(r); got[0] != "a.md" || got[2] != "c.md" {
		t.Fatalf("パス昇順になっていない: %v", got)
	}
}

// TestSearchSortNameUsesPathNotTitle は、名前順が見出しやメタデータのタイトル
// ではなく実際のファイルパスで並ぶことを確かめる。フォルダ配下のファイルが
// 自然にまとまるようにするための挙動。
func TestSearchSortNameUsesPathNotTitle(t *testing.T) {
	s := newTestStore(t)
	entries := []*model.Entry{
		// パスは folder/b.md だが、タイトルは "A" 始まりでアルファベット順なら先頭に来る。
		newEntry("folder/b.md", "Aaa という見出し", 1, nil),
		// パスは folder/a.md だが、タイトルは "Z" 始まりでアルファベット順なら末尾に来る。
		newEntry("folder/a.md", "Zzz という見出し", 1, nil),
	}
	if err := s.PutAll(entries); err != nil {
		t.Fatalf("投入に失敗: %v", err)
	}

	r, _ := s.Search(SearchOptions{Sort: SortNameAsc})
	if got := paths(r); got[0] != "folder/a.md" || got[1] != "folder/b.md" {
		t.Fatalf("名前順がパスではなくタイトルで並んでいる: %v", got)
	}
}

func TestSearchWindow(t *testing.T) {
	s := newTestStore(t)
	seed(t, s)

	r, _ := s.Search(SearchOptions{Offset: 1, Limit: 1})
	if r.Total != 3 {
		t.Fatalf("Total は絞り込み後の総件数であるべき: %d", r.Total)
	}
	if len(r.Entries) != 1 || r.Entries[0].Path != "b.md" {
		t.Fatalf("offset/limit が効いていない: %v", paths(r))
	}
}

// tagCount は、そのタグの使用件数を索引から読む。
func tagCount(s *Store, tag string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.tagCounts[strings.ToLower(tag)]
}

func TestDeleteUpdatesTagCounts(t *testing.T) {
	s := newTestStore(t)
	seed(t, s)
	if s.TagCount() != 4 {
		t.Fatalf("タグの種類数が想定外: %d", s.TagCount())
	}

	if err := s.Delete("c.md"); err != nil {
		t.Fatalf("削除に失敗: %v", err)
	}
	if s.Count() != 2 {
		t.Fatalf("削除後の件数が想定外: %d", s.Count())
	}
	if s.TagCount() != 3 || tagCount(s, "下書き") != 0 {
		t.Fatalf("使われなくなったタグが残っている: %d 種類", s.TagCount())
	}
	if tagCount(s, "golang") != 1 {
		t.Fatalf("タグ件数が減っていない: %d", tagCount(s, "golang"))
	}
}

func TestPutReplacesDerivedData(t *testing.T) {
	s := newTestStore(t)
	seed(t, s)

	updated := newEntry("a.md", "Go の設計メモ", 1, []string{"golang"})
	if err := s.Put(updated); err != nil {
		t.Fatalf("更新に失敗: %v", err)
	}
	if s.Count() != 3 {
		t.Fatalf("更新でエントリ数が変わった: %d", s.Count())
	}
	if tagCount(s, "設計") != 1 {
		t.Fatalf("外したタグの件数が減っていない: %d", tagCount(s, "設計"))
	}
}

func TestResetClearsEverything(t *testing.T) {
	s := newTestStore(t)
	seed(t, s)

	if err := s.Reset("/new/root"); err != nil {
		t.Fatalf("リセットに失敗: %v", err)
	}
	if s.Count() != 0 || s.TagCount() != 0 {
		t.Fatalf("リセット後にデータが残っている: %d 件 / %d タグ", s.Count(), s.TagCount())
	}
	if s.Root() != "/new/root" {
		t.Fatalf("ルートが更新されていない: %q", s.Root())
	}

	// リセット後もインデックスが生きていて、検索が動くこと。
	seed(t, s)
	r, err := s.Search(SearchOptions{Sort: SortNameAsc})
	if err != nil {
		t.Fatalf("リセット後の検索に失敗: %v", err)
	}
	if r.Total != 3 {
		t.Fatalf("リセット後の再投入が反映されていない: %d", r.Total)
	}
}

// mdEntry はフォルダ階層のあるテスト用の Markdown エントリを組み立てる。
func mdEntry(path, title string, links ...string) *model.Entry {
	return &model.Entry{
		Path:    path,
		RelPath: path,
		Name:    filepath.Base(path),
		Ext:     ".md",
		ModTime: time.Now(),
		Tags:    []string{},
		Title:   title,
		Links:   links,
	}
}

// 同じ名前のノートが別のフォルダにあっても、関連ページが混ざらないこと。
// リンクはファイル名ではなく、リンク元から見たパスで解決する。
func TestRelatedResolvesLinkPaths(t *testing.T) {
	s := newTestStore(t)

	here := filepath.Join("root", "a", "README.md")
	elsewhere := filepath.Join("root", "b", "README.md")
	source := mdEntry(filepath.Join("root", "a", "note.md"), "フォルダ A のメモ", "./README.md", "../a/README.md")
	if err := s.PutAll([]*model.Entry{
		mdEntry(here, "A の説明"),
		mdEntry(elsewhere, "B の説明"),
		source,
	}); err != nil {
		t.Fatalf("投入に失敗: %v", err)
	}

	// 2 通りの書き方で同じノートを指しているので、カードは 1 枚にまとまる。
	if got := groupSummary(s.Related(source)); len(got) != 1 || got[0] != "リンク先 README.md" {
		t.Fatalf("リンク先が同じフォルダの README になっていない: %v", got)
	}
	if got := s.Related(mdEntry(here, "A の説明")).Groups; len(got) != 1 || got[0].Pages[0].Path != source.Path {
		t.Fatalf("同じフォルダの README にリンク元が集まっていない: %+v", got)
	}
	if got := s.Related(mdEntry(elsewhere, "B の説明")).Groups; len(got) != 0 {
		t.Fatalf("無関係なフォルダの README に関連が出ている: %+v", got)
	}
}

// rootEntry は開いたフォルダ "root" の直下にあるテスト用のエントリを組み立てる。
func rootEntry(name string, daysAgo int, tags []string, links ...string) *model.Entry {
	e := newEntry(filepath.Join("root", name), strings.TrimSuffix(name, ".md"), daysAgo, tags)
	e.Name = name
	e.RelPath = name
	e.Links = links
	return e
}

// groupSummary は関連ページのグループを、比べやすい 1 行ずつの文字列にする。
// 「タグ [先頭のカード] ノート, ノート (+あふれた数)」の形で、まだ無いものは ? を付ける。
func groupSummary(r Related) []string {
	name := func(p RelatedPage) string {
		if p.Path == "" {
			return "?" + p.Title
		}
		return filepath.Base(p.Path)
	}
	out := make([]string, 0, len(r.Groups))
	for _, g := range r.Groups {
		line := "#" + g.Tag
		if g.Tag == "" {
			line = "リンク先"
		}
		if g.Page != nil {
			line += " [" + name(*g.Page) + "]"
		}
		names := make([]string, len(g.Pages))
		for i, p := range g.Pages {
			names[i] = name(p)
		}
		line += " " + strings.Join(names, ",")
		if g.More > 0 {
			line += fmt.Sprintf(" (+%d)", g.More)
		}
		out = append(out, line)
	}
	return out
}

// 関連ページは「タグのページと、そのタグを持つノート」をタグごとにまとめ、
// 最後に Markdown のリンクでつながるノートをまとめること。
// 同じタグを多く持つノートほど先に来て、前のグループに出したノートは後に出さないこと。
func TestRelatedGroupsByTag(t *testing.T) {
	s := newTestStore(t)
	if err := s.Reset("root"); err != nil {
		t.Fatal(err)
	}
	self := rootEntry("self.md", 0, []string{"go", "設計", "a/b"}, "./linked.md", "./まだ無い.md", "./both.md")
	entries := []*model.Entry{
		self,
		rootEntry("go.md", 9, nil), // go のページ
		rootEntry("both.md", 5, []string{"go", "設計"}),           // 2 つ重なる
		rootEntry("newer.md", 1, []string{"go"}),                // 1 つ重なる・新しい
		rootEntry("older.md", 3, []string{"Go"}),                // 1 つ重なる・古い
		rootEntry("linked.md", 2, []string{"go"}),               // リンクもしているが、go のグループが先
		rootEntry("design.md", 1, []string{"設計"}),               // 設計のページはまだ無い
		rootEntry("slash.md", 1, []string{"a/b"}),               // ファイル名にできないタグ
		rootEntry("backlink.md", 1, nil, "./self.md"),           // リンク元
		rootEntry("other.md", 1, []string{"料理"}, "./design.md"), // 無関係
	}
	if err := s.PutAll(entries); err != nil {
		t.Fatalf("投入に失敗: %v", err)
	}

	got := s.Related(self)
	// 最初はこのノートを指しているノート（リンク元）。見出しはこのノートが表すタグ。
	// タグのグループはタグの名前順。ファイル名にできないタグ（a/b）は先頭のカードを持たない。
	// 最後のリンクのグループには、このノートからのリンク先だけが残る。
	want := []string{
		"#self backlink.md",
		"#a/b slash.md",
		"#go [go.md] both.md,newer.md,linked.md,older.md",
		"#設計 [?設計] design.md",
		"リンク先 ?まだ無い",
	}
	if strings.Join(groupSummary(got), "\n") != strings.Join(want, "\n") {
		t.Fatalf("グループが違う:\ngot:\n%s\nwant:\n%s", strings.Join(groupSummary(got), "\n"), strings.Join(want, "\n"))
	}
	if strings.Join(got.MissingTags, ",") != "設計" {
		t.Fatalf("ページの無いタグが違う: %v", got.MissingTags)
	}
	if strings.Join(got.MissingLinks, ",") != "./まだ無い.md" {
		t.Fatalf("行き先の無いリンクが違う: %v", got.MissingLinks)
	}
}

// タグのページでは、そのタグを持つノートと、Markdown のリンクでリンクしているノートが、
// どちらもリンク元として最初のグループに並ぶこと。先頭のカードは自分なので置かない。
func TestRelatedOwnTagGroup(t *testing.T) {
	s := newTestStore(t)
	if err := s.Reset("root"); err != nil {
		t.Fatal(err)
	}
	page := rootEntry("Golang.md", 9, []string{"言語"})
	if err := s.PutAll([]*model.Entry{
		page,
		rootEntry("a.md", 2, []string{"golang"}),
		rootEntry("b.md", 1, []string{"golang", "言語"}),
		rootEntry("言語.md", 1, nil),
		rootEntry("c.md", 1, []string{"言語"}),
		rootEntry("linking.md", 0, []string{"言語"}, "./Golang.md"), // Markdown のリンクで指している
	}); err != nil {
		t.Fatal(err)
	}

	// linking.md はタグ「言語」も持つが、リンク元として最初のグループに出たので、後には出さない。
	want := []string{
		"#golang linking.md,b.md,a.md",
		"#言語 [言語.md] c.md",
	}
	if got := groupSummary(s.Related(page)); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("グループが違う:\ngot:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// 1 つのグループに並べるノートには上限があり、超えた分は数だけを返すこと。
func TestRelatedGroupLimit(t *testing.T) {
	s := newTestStore(t)
	self := rootEntry("self.md", 0, []string{"多い"})
	entries := []*model.Entry{self}
	for i := range groupLimit + 6 {
		entries = append(entries, rootEntry(fmt.Sprintf("n%02d.md", i), 1, []string{"多い"}))
	}
	if err := s.PutAll(entries); err != nil {
		t.Fatal(err)
	}
	g := s.Related(self).Groups[0]
	if len(g.Pages) != groupLimit || g.More != 6 {
		t.Fatalf("上限が効いていない: %d 件 (+%d)", len(g.Pages), g.More)
	}

	// 関連の無いノートでは、グループは空（nil ではなく空スライス）。
	lonely := rootEntry("lonely.md", 0, nil)
	if got := s.Related(lonely).Groups; got == nil || len(got) != 0 {
		t.Fatalf("関連が無いのにグループがある: %+v", got)
	}
}

// 検索語がタグの名前と一致するときは、そのタグのページ（同じ名前のノート）が
// 並び順にかかわらず先頭に別枠で出ること。
func TestSearchPutsTagPageFirst(t *testing.T) {
	s := newTestStore(t)
	seed(t, s)
	// 古いうえに名前順でも後ろに来るページにして、並び順では先頭にならないようにする。
	page := newEntry("Golang.md", "Go 言語", 30, nil)
	if err := s.Put(page); err != nil {
		t.Fatalf("投入に失敗: %v", err)
	}

	for _, sort := range []SortOrder{SortModifiedDesc, SortNameAsc, SortRelevance} {
		r, err := s.Search(SearchOptions{Query: " golang ", Sort: sort})
		if err != nil {
			t.Fatalf("検索に失敗: %v", err)
		}
		got := paths(r)
		if len(got) != 3 || got[0] != "Golang.md" || r.Head != 1 {
			t.Fatalf("%s: タグのページが先頭に来ていない: %v (head=%d)", sort, got, r.Head)
		}
	}

	// 一部だけ一致しても先頭には出さない。
	r, _ := s.Search(SearchOptions{Query: "gola"})
	if r.Head != 0 {
		t.Fatalf("部分一致でタグのページが先頭に出ている: %v", paths(r))
	}
}

// 検索語が無いときは、ピン留めしたノートがピン留めした順に先頭へ並ぶこと。
// 読み込んでいないノートのピン留めは無視し、検索中はピン留めを先頭にしない。
func TestSearchPins(t *testing.T) {
	s := newTestStore(t)
	seed(t, s)
	s.SetPins([]string{"C.md", "missing.md", "b.md"})

	r, _ := s.Search(SearchOptions{Sort: SortModifiedDesc})
	if got := paths(r); strings.Join(got, ",") != "c.md,b.md,a.md" || r.Head != 2 {
		t.Fatalf("ピン留めが先頭に並んでいない: %v (head=%d)", got, r.Head)
	}
	if strings.Join(r.Pins, ",") != "c.md,b.md" {
		t.Fatalf("ピン留めの一覧が違う: %v", r.Pins)
	}

	r, _ = s.Search(SearchOptions{Query: "設計", Sort: SortModifiedDesc})
	if got := paths(r); strings.Join(got, ",") != "a.md,b.md" || r.Head != 0 {
		t.Fatalf("検索中にピン留めが先頭へ来ている: %v (head=%d)", got, r.Head)
	}
	if strings.Join(r.Pins, ",") != "c.md,b.md" {
		t.Fatalf("検索中もピン留めの一覧は全部返すはず: %v", r.Pins)
	}

	// ページングしたときの別枠の件数は、その範囲に入っている分だけ。
	r, _ = s.Search(SearchOptions{Sort: SortModifiedDesc, Offset: 1, Limit: 1})
	if got := paths(r); len(got) != 1 || got[0] != "b.md" || r.Head != 1 {
		t.Fatalf("ページングしたときの別枠が違う: %v (head=%d)", got, r.Head)
	}
}

// タグのページのパスを、タグの大文字小文字や空白の揺れを無視して引けること。
func TestTagPagePath(t *testing.T) {
	s := newTestStore(t)
	page := mdEntry(filepath.Join("root", "開発 メモ.md"), "開発メモ")
	if err := s.Put(page); err != nil {
		t.Fatal(err)
	}
	if got := s.TagPagePath("  開発   メモ "); got != page.Path {
		t.Fatalf("タグのページが引けない: %q", got)
	}
	if got := s.TagPagePath("開発"); got != "" {
		t.Fatalf("別のタグでページが引けてしまう: %q", got)
	}
	if err := s.Delete(page.Path); err != nil {
		t.Fatal(err)
	}
	if got := s.TagPagePath("開発 メモ"); got != "" {
		t.Fatalf("消したページが残っている: %q", got)
	}
}
