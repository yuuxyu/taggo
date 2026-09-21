package store

import (
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
		Kind:    model.KindMarkdown,
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

func TestSearchTagAnd(t *testing.T) {
	s := newTestStore(t)
	seed(t, s)

	r, err := s.Search(SearchOptions{Query: "#golang #設計"})
	if err != nil {
		t.Fatalf("検索に失敗: %v", err)
	}
	if got := paths(r); len(got) != 1 || got[0] != "a.md" {
		t.Fatalf("AND 検索の結果が想定外: %v", got)
	}
}

func TestSearchTagOr(t *testing.T) {
	s := newTestStore(t)
	seed(t, s)

	r, _ := s.Search(SearchOptions{Query: "#rust OR #下書き"})
	if r.Total != 2 {
		t.Fatalf("OR 検索の件数が想定外: %v", paths(r))
	}
}

func TestSearchTagNot(t *testing.T) {
	s := newTestStore(t)
	seed(t, s)

	r, _ := s.Search(SearchOptions{Query: "#golang -#下書き"})
	if got := paths(r); len(got) != 1 || got[0] != "a.md" {
		t.Fatalf("NOT 検索の結果が想定外: %v", got)
	}
}

func TestSearchTagIsExactNotPrefix(t *testing.T) {
	s := newTestStore(t)
	seed(t, s)

	// "#go" は "golang" を巻き込んではいけない。
	r, _ := s.Search(SearchOptions{Query: "#go"})
	if r.Total != 0 {
		t.Fatalf("タグ検索が前方一致になっている: %v", paths(r))
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

func TestTagsSuggestion(t *testing.T) {
	s := newTestStore(t)
	seed(t, s)

	all := s.Tags("", 0)
	if len(all) != 4 {
		t.Fatalf("タグ種類数が想定外: %+v", all)
	}
	// golang と 設計 が 2 件ずつで先頭に来る。
	if all[0].Count != 2 || all[1].Count != 2 {
		t.Fatalf("使用件数の多い順になっていない: %+v", all)
	}

	got := s.Tags("go", 0)
	if len(got) != 1 || got[0].Tag != "golang" {
		t.Fatalf("前方一致の絞り込みが効いていない: %+v", got)
	}
}

func TestDeleteUpdatesTagCounts(t *testing.T) {
	s := newTestStore(t)
	seed(t, s)

	if err := s.Delete("c.md"); err != nil {
		t.Fatalf("削除に失敗: %v", err)
	}
	if s.Count() != 2 {
		t.Fatalf("削除後の件数が想定外: %d", s.Count())
	}
	for _, tag := range s.Tags("", 0) {
		if tag.Tag == "下書き" {
			t.Fatalf("使われなくなったタグが候補に残っている: %+v", s.Tags("", 0))
		}
		if tag.Tag == "golang" && tag.Count != 1 {
			t.Fatalf("タグ件数が減っていない: %+v", tag)
		}
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
	for _, tag := range s.Tags("", 0) {
		if tag.Tag == "設計" && tag.Count != 1 {
			t.Fatalf("外したタグの件数が減っていない: %+v", tag)
		}
	}
}

func TestRelated(t *testing.T) {
	s := newTestStore(t)

	target := newEntry("目次.md", "目次", 1, nil)
	source := newEntry("memo.md", "メモ", 1, nil)
	source.Links = []string{"./目次.md", "./まだ無いノート.md"}
	if err := s.PutAll([]*model.Entry{target, source}); err != nil {
		t.Fatalf("投入に失敗: %v", err)
	}

	got := s.Related(target)
	if len(got.Incoming) != 1 || got.Incoming[0].Path != "memo.md" {
		t.Fatalf("バックリンクが取れていない: %+v", got.Incoming)
	}
	if len(got.Outgoing) != 0 {
		t.Fatalf("リンクしていないのに関連が出ている: %+v", got.Outgoing)
	}

	// リンク元から見ると、行き先のあるリンクと無いリンクが順番どおりに並ぶ。
	from := s.Related(source)
	if len(from.Outgoing) != 2 {
		t.Fatalf("リンク先の数が合わない: %+v", from.Outgoing)
	}
	if from.Outgoing[0].Path != "目次.md" {
		t.Fatalf("リンク先が引けていない: %+v", from.Outgoing[0])
	}
	if from.Outgoing[1].Path != "" || from.Outgoing[1].Title != "まだ無いノート" {
		t.Fatalf("行き先の無いリンクの扱いが違う: %+v", from.Outgoing[1])
	}

	// リンクを外したら、関連も消えること。
	source.Links = nil
	if err := s.Put(source); err != nil {
		t.Fatalf("更新に失敗: %v", err)
	}
	if got := s.Related(target); len(got.Incoming) != 0 {
		t.Fatalf("外したリンクが残っている: %+v", got.Incoming)
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
		Kind:    model.KindMarkdown,
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
	source := mdEntry(filepath.Join("root", "a", "note.md"), "フォルダ A のメモ", "./README.md")
	if err := s.PutAll([]*model.Entry{
		mdEntry(here, "A の説明"),
		mdEntry(elsewhere, "B の説明"),
		source,
	}); err != nil {
		t.Fatalf("投入に失敗: %v", err)
	}

	if got := s.Related(source).Outgoing; len(got) != 1 || got[0].Path != here {
		t.Fatalf("リンク先が同じフォルダの README になっていない: %+v", got)
	}

	if got := s.Related(mdEntry(here, "A の説明")).Incoming; len(got) != 1 {
		t.Fatalf("同じフォルダの README にリンク元が集まっていない: %+v", got)
	}
	if got := s.Related(mdEntry(elsewhere, "B の説明")).Incoming; len(got) != 0 {
		t.Fatalf("無関係なフォルダの README に関連が出ている: %+v", got)
	}
}

// 上の階層や別フォルダを指すパスも、書かれたとおりにたどれること。
func TestRelatedFollowsRelativePaths(t *testing.T) {
	s := newTestStore(t)

	target := filepath.Join("root", "b", "手順.md")
	source := mdEntry(filepath.Join("root", "a", "note.md"), "メモ", "../b/手順.md", "/b/手順.md")
	if err := s.Reset("root"); err != nil {
		t.Fatalf("リセットに失敗: %v", err)
	}
	if err := s.PutAll([]*model.Entry{mdEntry(target, "手順"), source}); err != nil {
		t.Fatalf("投入に失敗: %v", err)
	}

	// 2 通りの書き方で同じノートを指しているので、カードは 1 枚にまとまる。
	got := s.Related(source).Outgoing
	if len(got) != 1 || got[0].Path != target {
		t.Fatalf("相対パスのリンクをたどれていない: %+v", got)
	}
}

// 同じタグのノートは、重なるタグの多い順に並び、自分自身とリンクで出ているものは除くこと。
func TestRelatedSameTag(t *testing.T) {
	s := newTestStore(t)

	self := newEntry("self.md", "自分", 0, []string{"go", "設計"})
	self.Links = []string{"./linked.md"}
	both := newEntry("both.md", "両方", 3, []string{"Go", "設計"})
	older := newEntry("older.md", "古い", 2, []string{"go"})
	newer := newEntry("newer.md", "新しい", 1, []string{"go"})
	linked := newEntry("linked.md", "リンク先", 1, []string{"go"})
	other := newEntry("other.md", "無関係", 1, []string{"料理"})
	audio := newEntry("song.mp3", "曲", 1, []string{"go"})
	audio.Kind = model.KindAudio
	if err := s.PutAll([]*model.Entry{self, both, older, newer, linked, other, audio}); err != nil {
		t.Fatalf("投入に失敗: %v", err)
	}

	got := s.Related(self).SameTag
	var paths []string
	for _, p := range got {
		paths = append(paths, p.Path)
	}
	want := []string{"both.md", "newer.md", "older.md"}
	if strings.Join(paths, ",") != strings.Join(want, ",") {
		t.Fatalf("同じタグのノートが違う: got %v, want %v", paths, want)
	}

	// タグの無いノートでは何も出さない。空でも nil ではなく空スライスで返す。
	if got := s.Related(newEntry("none.md", "タグなし", 0, nil)).SameTag; got == nil || len(got) != 0 {
		t.Fatalf("タグが無いのに同じタグのノートが出ている: %+v", got)
	}
}
