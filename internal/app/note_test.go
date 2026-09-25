package app

import (
	"os"
	"path/filepath"
	"testing"
)

// まだ無いページのファイルを、ファイル名を見出しにして作ること。既にあれば書き換えないこと。
func TestPreparePageCreatesMissingNote(t *testing.T) {
	a, root := newTestApp(t, map[string]string{"a.md": "# A\n"})
	want := filepath.Join(root, "まだ無いノート.md")

	path, created, err := a.preparePage(want)
	if err != nil {
		t.Fatalf("作成に失敗: %v", err)
	}
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
	path, created, err = a.preparePage(want)
	if err != nil || created || path != want {
		t.Fatalf("既存ファイル: created=%v path=%q err=%v", created, path, err)
	}
	if got, _ := os.ReadFile(want); string(got) != "書きかけ\n" {
		t.Fatalf("既存ファイルを書き換えた: %q", got)
	}
}

// フォルダの直下の Markdown 以外には、ファイルを作らないこと。
func TestPreparePageRejects(t *testing.T) {
	a, root := newTestApp(t, map[string]string{"a.md": "# A\n"})

	cases := map[string]string{
		"フォルダの外":      filepath.Join(filepath.Dir(root), "outside.md"),
		"サブフォルダ":      filepath.Join(root, "sub", "b.md"),
		"Markdown 以外": filepath.Join(root, "script.bat"),
	}
	for name, path := range cases {
		if _, _, err := a.preparePage(path); err == nil {
			t.Errorf("%s: エラーにならない", name)
		}
		if _, err := os.Stat(path); err == nil {
			t.Errorf("%s: ファイルを作った", name)
		}
	}
}
