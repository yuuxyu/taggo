package app

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"

	"github.com/yuuxyu/taggo/internal/store"
)

// 保存した中身がファイルに書かれ、タグなどの一覧の情報もその場で新しくなること。
func TestSaveNoteWritesAndUpdatesEntry(t *testing.T) {
	a, root := newTestApp(t, map[string]string{"a.md": "# A\n\n[[古いタグ]]\n"})
	path := filepath.Join(root, "a.md")

	note, err := a.ReadNote(path)
	if err != nil {
		t.Fatal(err)
	}
	res, err := a.SaveNote(path, "# A\n\n[[新しいタグ]]\n", note.Version)
	if err != nil {
		t.Fatalf("保存に失敗: %v", err)
	}
	if res.Conflict || res.Entry == nil {
		t.Fatalf("保存の結果が想定外: %+v", res)
	}
	if got, _ := os.ReadFile(path); string(got) != "# A\n\n[[新しいタグ]]\n" {
		t.Fatalf("ファイルの中身が違う: %q", got)
	}
	if !slices.Equal(res.Entry.Tags, []string{"新しいタグ"}) {
		t.Fatalf("エントリのタグが新しくなっていない: %v", res.Entry.Tags)
	}
	// ウォッチャーを待たずに、検索にも反映されている。
	if r, _ := a.Search(store.SearchOptions{Query: "新しいタグ"}); r.Total != 1 {
		t.Fatalf("保存した内容で検索できない: %d 件", r.Total)
	}

	// 続けて保存するときは、前回の保存で受け取った Version を使う。
	again, err := a.SaveNote(path, "# A\n\n続き\n", res.Version)
	if err != nil || again.Conflict {
		t.Fatalf("続けての保存に失敗: %+v %v", again, err)
	}
}

// 読んだあとでほかのアプリが書き換えていたら、上書きせずに Conflict を返すこと。
// Version を空にすれば（利用者が上書きを選べば）上書きすること。
func TestSaveNoteDetectsConflict(t *testing.T) {
	a, root := newTestApp(t, map[string]string{"a.md": "# A\n"})
	path := filepath.Join(root, "a.md")

	note, err := a.ReadNote(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# ほかのエディタでの変更\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := a.SaveNote(path, "# taggo での変更\n", note.Version)
	if err != nil {
		t.Fatalf("保存でエラー: %v", err)
	}
	if !res.Conflict {
		t.Fatal("ほかでの変更に気付かずに保存した")
	}
	if got, _ := os.ReadFile(path); string(got) != "# ほかのエディタでの変更\n" {
		t.Fatalf("ほかでの変更を上書きした: %q", got)
	}

	res, err = a.SaveNote(path, "# taggo での変更\n", "")
	if err != nil || res.Conflict {
		t.Fatalf("上書きに失敗: %+v %v", res, err)
	}
	if got, _ := os.ReadFile(path); string(got) != "# taggo での変更\n" {
		t.Fatalf("上書きされていない: %q", got)
	}
}

// 元のファイルの先頭に BOM があれば、編集画面には渡さず、保存で付け直すこと。
func TestSaveNoteKeepsBOM(t *testing.T) {
	a, root := newTestApp(t, map[string]string{"a.md": "\xEF\xBB\xBF# A\n"})
	path := filepath.Join(root, "a.md")

	note, err := a.ReadNote(path)
	if err != nil {
		t.Fatal(err)
	}
	if note.Content != "# A\n" {
		t.Fatalf("BOM が編集画面に渡っている: %q", note.Content)
	}
	if _, err := a.SaveNote(path, "# B\n", note.Version); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "\xEF\xBB\xBF# B\n" {
		t.Fatalf("BOM が付け直されていない: %q", got)
	}
}

// 一覧に無いノートや、フォルダの外のファイルには保存しないこと。
func TestSaveNoteRejects(t *testing.T) {
	a, root := newTestApp(t, map[string]string{"a.md": "# A\n"})

	cases := map[string]string{
		"一覧に無いノート":    filepath.Join(root, "無い.md"),
		"フォルダの外":      filepath.Join(filepath.Dir(root), "outside.md"),
		"Markdown 以外": filepath.Join(root, "script.bat"),
	}
	for name, path := range cases {
		if _, err := a.SaveNote(path, "# 書き込み\n", ""); err == nil {
			t.Errorf("%s: エラーにならない", name)
		}
		if _, err := os.Stat(path); err == nil {
			t.Errorf("%s: ファイルを作った", name)
		}
	}
}

// 貼り付けた画像を、日時の名前でフォルダの直下に保存すること。同じ名前があれば番号を付けること。
func TestSaveImage(t *testing.T) {
	a, root := newTestApp(t, map[string]string{"a.md": "# A\n"})
	note := filepath.Join(root, "a.md")
	data := base64.StdEncoding.EncodeToString([]byte(pngHead))

	first, err := a.SaveImage(note, data)
	if err != nil {
		t.Fatalf("保存に失敗: %v", err)
	}
	if !regexp.MustCompile(`^\d{8}-\d{6}\.png$`).MatchString(first) {
		t.Fatalf("名前が想定外: %q", first)
	}
	if got, _ := os.ReadFile(filepath.Join(root, first)); string(got) != pngHead {
		t.Fatalf("中身が違う: %q", got)
	}

	// 同じ秒に貼り付けても、前の画像を上書きしない。
	name, err := saveNewFile(root, first[:len(first)-len(".png")], ".png", []byte(pngHead))
	if err != nil {
		t.Fatal(err)
	}
	if name == first || !regexp.MustCompile(`-2\.png$`).MatchString(name) {
		t.Fatalf("同じ名前のときの名前が想定外: %q", name)
	}
}

// 画像でないデータは保存しないこと。
func TestSaveImageRejectsNonImage(t *testing.T) {
	a, root := newTestApp(t, map[string]string{"a.md": "# A\n"})
	note := filepath.Join(root, "a.md")

	if _, err := a.SaveImage(note, base64.StdEncoding.EncodeToString([]byte("MZ 実行ファイル"))); err == nil {
		t.Fatal("画像でないデータを保存した")
	}
	if _, err := a.SaveImage(note, "Base64 ではない"); err == nil {
		t.Fatal("壊れたデータを保存した")
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 1 {
		t.Fatalf("ファイルが増えた: %d 件", len(entries))
	}
}

// まだ無いページを作ると、その場で一覧に載ること。既にあるファイルは書き換えないこと。
func TestCreatePage(t *testing.T) {
	a, root := newTestApp(t, map[string]string{"a.md": "# A\n"})
	path := filepath.Join(root, "新しいページ.md")

	res, err := a.CreatePage(path)
	if err != nil {
		t.Fatalf("作成に失敗: %v", err)
	}
	if !res.Created || res.Entry == nil || res.Entry.Title != "新しいページ" {
		t.Fatalf("作成の結果が想定外: %+v", res)
	}
	if _, ok := a.store.Get(path); !ok {
		t.Fatal("作ったページが一覧に載っていない")
	}

	again, err := a.CreatePage(path)
	if err != nil || again.Created {
		t.Fatalf("既にあるページ: %+v %v", again, err)
	}
}

// 補完の候補には、使われているタグとページのあるタグを、使われている件数の多い順に並べること。
func TestTagNames(t *testing.T) {
	a, _ := newTestApp(t, map[string]string{
		"a.md":   "# A\n\n[[Go]] [[設計]]\n",
		"b.md":   "# B\n\n[[go]]\n",
		"ページ.md": "# ページ\n",
	})

	got := a.TagNames()
	// 大文字小文字だけ違うものは 1 つにまとめる。
	want := []string{"Go", "設計", "a", "b", "ページ"}
	if got[0] != "Go" || len(got) != len(want) {
		t.Fatalf("候補が想定外: %v", got)
	}
	for _, name := range want {
		if !slices.Contains(got, name) {
			t.Fatalf("候補に %q が無い: %v", name, got)
		}
	}
}
