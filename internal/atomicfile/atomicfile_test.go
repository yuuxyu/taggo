package atomicfile

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "a.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func readString(t *testing.T, path string) string {
	t.Helper()
	got, err := ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(got)
}

// このパッケージの Open で読んでいる最中でも、置き換えはすぐに通ること。
// ウォッチャーが保存直後のノートを読み直すのと、次の保存が重なっても失敗しないように。
func TestReplaceWhileOpenShared(t *testing.T) {
	path := writeTemp(t, "old")
	f, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	start := time.Now()
	if err := Replace(path, []byte("new")); err != nil {
		t.Fatalf("読んでいる最中の置き換えに失敗: %v", err)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("待たずに通るはずが %v かかった", d)
	}
	if got := readString(t, path); got != "new" {
		t.Fatalf("中身が違う: %q", got)
	}
}

// ほかのプロセス（同期サービスなど）が一瞬だけ使っているときは、待ってやり直して通ること。
func TestReplaceRetriesWhileInUse(t *testing.T) {
	path := writeTemp(t, "old")
	// os.Open は名前の変更を許さずに開くので、ほかのアプリが使用中の状態を再現できる。
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	time.AfterFunc(300*time.Millisecond, func() { f.Close() })

	if err := Replace(path, []byte("new")); err != nil {
		t.Fatalf("使用中が終わったあとも失敗した: %v", err)
	}
	if got := readString(t, path); got != "new" {
		t.Fatalf("中身が違う: %q", got)
	}
}

// 使われ続けているときは、待っても置き換えられなかったと分かるエラーを返し、元の中身を残すこと。
func TestReplaceGivesUpWhenStillInUse(t *testing.T) {
	path := writeTemp(t, "old")
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	err = Replace(path, []byte("new"))
	if !errors.Is(err, ErrInUse) {
		t.Fatalf("ErrInUse になるはずが: %v", err)
	}
	f.Close()
	if got := readString(t, path); got != "old" {
		t.Fatalf("元の中身が残っていない: %q", got)
	}
	// 一時ファイルを残さない。
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatalf("一時ファイルが残った: %d 件", len(entries))
	}
}

// 無いファイルは、os.ReadFile と同じく fs.ErrNotExist で分かること。
func TestReadFileNotExist(t *testing.T) {
	_, err := ReadFile(filepath.Join(t.TempDir(), "無い.md"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("fs.ErrNotExist になるはずが: %v", err)
	}
}

// CreateNew は無いファイルだけを作り、既にあるファイルは上書きしないこと。一時ファイルも残さないこと。
func TestCreateNew(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.md")

	if err := CreateNew(path, []byte("# 新しい\n")); err != nil {
		t.Fatalf("作れない: %v", err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "# 新しい\n" {
		t.Fatalf("中身が違う: %q, %v", got, err)
	}

	// 大文字小文字だけが違う名前も、同じファイルとして断る（Windows の決まり）。
	err := CreateNew(filepath.Join(dir, "A.md"), []byte("# 上書き\n"))
	if !errors.Is(err, fs.ErrExist) {
		t.Fatalf("既にあるファイルで fs.ErrExist にならない: %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "# 新しい\n" {
		t.Fatalf("既にあるファイルを上書きした: %q", got)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("一時ファイルが残っている: %v", entries)
	}
}
