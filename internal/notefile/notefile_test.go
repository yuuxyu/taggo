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
