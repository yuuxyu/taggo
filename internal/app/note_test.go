package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareNoteCreatesMissingNote(t *testing.T) {
	a, root := newTestApp(t, map[string]string{
		"docs/a.md": "# A\n\n[まだ無い](./sub/まだ無いノート.md)\n",
	})
	from := filepath.Join(root, "docs", "a.md")

	path, created, err := a.prepareNote(from, "./sub/まだ無いノート.md")
	if err != nil {
		t.Fatalf("作成に失敗: %v", err)
	}
	want := filepath.Join(root, "docs", "sub", "まだ無いノート.md")
	if !created || path != want {
		t.Fatalf("created=%v path=%q, want true %q", created, path, want)
	}
	got, err := os.ReadFile(want)
	if err != nil {
		t.Fatalf("作ったファイルを読めない: %v", err)
	}
	if string(got) != "# まだ無いノート\n" {
		t.Fatalf("中身が違う: %q", got)
	}

	// 2 回目は既にあるので、作り直さずに同じパスを返す。
	if err := os.WriteFile(want, []byte("書きかけ\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path, created, err = a.prepareNote(from, "./sub/まだ無いノート.md")
	if err != nil || created || path != want {
		t.Fatalf("既存ファイル: created=%v path=%q err=%v", created, path, err)
	}
	if got, _ := os.ReadFile(want); string(got) != "書きかけ\n" {
		t.Fatalf("既存ファイルを書き換えた: %q", got)
	}
}

func TestPrepareNoteRootRelative(t *testing.T) {
	a, root := newTestApp(t, map[string]string{"docs/a.md": "# A\n"})

	path, created, err := a.prepareNote(filepath.Join(root, "docs", "a.md"), "/top.md")
	if err != nil || !created || path != filepath.Join(root, "top.md") {
		t.Fatalf("created=%v path=%q err=%v", created, path, err)
	}
}

func TestPrepareNoteRejects(t *testing.T) {
	a, root := newTestApp(t, map[string]string{"a.md": "# A\n"})
	from := filepath.Join(root, "a.md")

	cases := map[string]struct{ from, link string }{
		"フォルダの外":      {from, "../outside.md"},
		"Markdown 以外": {from, "./script.bat"},
		"未登録のリンク元":    {filepath.Join(root, "none.md"), "./b.md"},
	}
	for name, c := range cases {
		if _, _, err := a.prepareNote(c.from, c.link); err == nil {
			t.Errorf("%s: エラーにならない", name)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "outside.md")); err == nil {
		t.Fatal("フォルダの外にファイルを作った")
	}
}
