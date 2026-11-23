package notefile

import (
	"os"
	"path/filepath"
	"testing"
)

// 扱えるのは、開いているフォルダの直下の Markdown だけであること。
func TestCheckPagePath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "notes")
	tests := []struct {
		path string
		ok   bool
	}{
		{filepath.Join(root, "a.md"), true},
		{filepath.Join(root, "A.Markdown"), true},
		{filepath.Join(root, "a.txt"), false},
		{filepath.Join(root, "sub", "a.md"), false},
		{filepath.Join(root, "..", "a.md"), false},
		{filepath.Join(filepath.Dir(root), "notes2", "a.md"), false},
	}
	for _, tt := range tests {
		if err := CheckPagePath(root, tt.path); (err == nil) != tt.ok {
			t.Errorf("CheckPagePath(%q) = %v, 扱えるか %v のはず", tt.path, err, tt.ok)
		}
	}
	if err := CheckPagePath("", filepath.Join(root, "a.md")); err == nil {
		t.Error("フォルダが開かれていないのに扱えることになった")
	}
}

// 上限を超えるファイルは読まないこと。
func TestReadLimited(t *testing.T) {
	dir := t.TempDir()
	small := filepath.Join(dir, "small.md")
	if err := os.WriteFile(small, []byte("# 小さい\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadLimited(small); err != nil || string(got) != "# 小さい\n" {
		t.Fatalf("読めない: %q, %v", got, err)
	}

	large := filepath.Join(dir, "large.md")
	if err := os.WriteFile(large, make([]byte, MaxBytes+1), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadLimited(large); err == nil {
		t.Fatal("上限を超えるファイルを読んだ")
	}
}

// 中身が同じなら同じ版、違えば違う版になること。
func TestVersion(t *testing.T) {
	if Version([]byte("a")) != Version([]byte("a")) {
		t.Fatal("同じ中身で版が違う")
	}
	if Version([]byte("a")) == Version([]byte("b")) {
		t.Fatal("違う中身で版が同じ")
	}
}

// BOM と改行の種類を覚え、書き戻すときに保つこと。
func TestTextKeepsBOMAndLineEndings(t *testing.T) {
	raw := append(append([]byte{}, BOM...), "# A\r\n\r\n本文\r\n"...)
	text := Decode(raw)
	if !text.BOM || text.EOL != "\r\n" || text.Body != "# A\r\n\r\n本文\r\n" {
		t.Fatalf("読み方が違う: %+v", text)
	}
	if got := text.Lines("a\nb\r\nc"); got != "a\r\nb\r\nc" {
		t.Fatalf("改行をそろえていない: %q", got)
	}
	text.Body += text.Lines("追記\n")
	if got := string(text.Encode()); got != string(BOM)+"# A\r\n\r\n本文\r\n追記\r\n" {
		t.Fatalf("書き戻し方が違う: %q", got)
	}

	plain := Decode([]byte("# B\n本文"))
	if plain.BOM || plain.EOL != "\n" || string(plain.Encode()) != "# B\n本文" {
		t.Fatalf("LF のノートの扱いが違う: %+v", plain)
	}
	if got := plain.Lines("a\r\nb"); got != "a\nb" {
		t.Fatalf("LF にそろえていない: %q", got)
	}
	if empty := Decode(nil); empty.EOL != "\n" || empty.Body != "" {
		t.Fatalf("空のノートの扱いが違う: %+v", empty)
	}
}
