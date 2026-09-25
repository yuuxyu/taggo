package scan

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/yuuxyu/taggo/internal/model"
)

// buildTree はテスト用のフォルダ構成を作る。
func buildTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	files := map[string]string{
		"a.md":      "# A\n\n[[golang]]\n",
		"b.md":      "# B\n\n[[設計]] と [[並行処理]] の話\n",
		"notes.txt": "対象外の拡張子",
		// 画像と音声は Markdown から参照されるだけで、一覧には載せない。
		"photo.png": "画像",
		"photo.jpg": "画像",
		"song.mp3":  "音声",
		"voice.wav": "音声",
		// サブフォルダの中は、Markdown でも読まない。
		"sub/c.md":            "# C\n\n[[サブ]]\n",
		"node_modules/dep.md": "# Dep\n\n[[除外]]\n",
		".hidden/secret.md":   "# Secret\n\n[[隠し]]\n",
	}
	for rel, content := range files {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("ディレクトリ作成に失敗: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("ファイル作成に失敗: %v", err)
		}
	}
	return root
}

func relPaths(entries []*model.Entry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, filepath.ToSlash(e.RelPath))
	}
	return out
}

func TestScanCollectsSupportedFiles(t *testing.T) {
	root := buildTree(t)

	got, err := Scan(context.Background(), Options{Root: root})
	if err != nil {
		t.Fatalf("走査に失敗: %v", err)
	}

	paths := relPaths(got.Entries)
	if len(paths) != 2 || paths[0] != "a.md" || paths[1] != "b.md" {
		t.Fatalf("対象は直下の a.md と b.md の 2 件のはず: %v", paths)
	}

	// 相対パスが走査ルート基準になっていること。
	for _, e := range got.Entries {
		if filepath.IsAbs(e.RelPath) {
			t.Fatalf("RelPath が絶対パスになっている: %q", e.RelPath)
		}
		if !filepath.IsAbs(e.Path) {
			t.Fatalf("Path が絶対パスでない: %q", e.Path)
		}
	}
}

func TestScanReadsTags(t *testing.T) {
	root := buildTree(t)

	got, _ := Scan(context.Background(), Options{Root: root})
	found := map[string][]string{}
	for _, e := range got.Entries {
		found[filepath.ToSlash(e.RelPath)] = e.Tags
	}
	if tags := found["a.md"]; len(tags) != 1 || tags[0] != "golang" {
		t.Fatalf("a.md のタグが読めていない: %v", tags)
	}
	if tags := found["b.md"]; len(tags) != 2 || tags[0] != "並行処理" || tags[1] != "設計" {
		t.Fatalf("b.md のタグが読めていない: %v", tags)
	}
}

func TestScanRespectsLimit(t *testing.T) {
	root := buildTree(t)

	got, err := Scan(context.Background(), Options{Root: root, Limit: 1})
	if err != nil {
		t.Fatalf("走査に失敗: %v", err)
	}
	if len(got.Entries) != 1 {
		t.Fatalf("上限 1 件が効いていない: %v", relPaths(got.Entries))
	}
	if !got.LimitReached {
		t.Fatal("LimitReached が立っていない")
	}
}

// buildFlatTree は走査順の確かめやすいフォルダ構成を作る。
// 読むのは直下の a.md → b.md → c.md → d.md の順で、サブフォルダ a と e の中は読まない。
func buildFlatTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, rel := range []string{"a/x.md", "a.md", "b.md", "c.md", "d.md", "e/y.md"} {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("ディレクトリ作成に失敗: %v", err)
		}
		if err := os.WriteFile(path, []byte("# "+rel+"\n"), 0o644); err != nil {
			t.Fatalf("ファイル作成に失敗: %v", err)
		}
	}
	return root
}

func TestScanCountsRemainingAfterLimit(t *testing.T) {
	root := buildFlatTree(t)

	got, err := Scan(context.Background(), Options{Root: root, Limit: 2, Workers: 1})
	if err != nil {
		t.Fatalf("走査に失敗: %v", err)
	}
	if paths := relPaths(got.Entries); len(paths) != 2 || paths[0] != "a.md" || paths[1] != "b.md" {
		t.Fatalf("走査順の先頭 2 件になっていない: %v", paths)
	}
	if !got.LimitReached || got.Remaining != 2 {
		t.Fatalf("残り件数が想定外: limitReached=%v remaining=%d", got.LimitReached, got.Remaining)
	}
	if filepath.ToSlash(got.Cursor) != "b.md" {
		t.Fatalf("再開位置が想定外: %q", got.Cursor)
	}
}

func TestScanResumesFromCursor(t *testing.T) {
	root := buildFlatTree(t)

	first, _ := Scan(context.Background(), Options{Root: root, Limit: 2, Workers: 1})
	second, err := Scan(context.Background(), Options{
		Root: root, Limit: 1, Workers: 1, StartAfter: first.Cursor,
	})
	if err != nil {
		t.Fatalf("走査に失敗: %v", err)
	}
	if paths := relPaths(second.Entries); len(paths) != 1 || paths[0] != "c.md" {
		t.Fatalf("続きの 1 件が想定外: %v", paths)
	}
	if second.Remaining != 1 {
		t.Fatalf("残り件数が想定外: %d", second.Remaining)
	}

	// 上限なしなら最後まで読み切り、残りは無くなる。
	rest, _ := Scan(context.Background(), Options{Root: root, Workers: 1, StartAfter: second.Cursor})
	if paths := relPaths(rest.Entries); len(paths) != 1 || paths[0] != "d.md" {
		t.Fatalf("残りの読み込みが想定外: %v", paths)
	}
	if rest.LimitReached || rest.Remaining != 0 {
		t.Fatalf("読み切ったのに残りがある: %+v", rest)
	}
}

func TestScanResumesAfterDeletedCursor(t *testing.T) {
	root := buildFlatTree(t)

	// 再開位置のファイルが消えていても、その位置から続きを読める。
	if err := os.Remove(filepath.Join(root, "b.md")); err != nil {
		t.Fatal(err)
	}
	got, _ := Scan(context.Background(), Options{Root: root, Workers: 1, StartAfter: "b.md"})
	if paths := relPaths(got.Entries); len(paths) != 2 || paths[0] != "c.md" || paths[1] != "d.md" {
		t.Fatalf("消えた再開位置からの続きが想定外: %v", paths)
	}
}

func TestIsAfterFollowsWalkOrder(t *testing.T) {
	cases := []struct {
		rel, cursor string
		want        bool
	}{
		{"b.md", "a.md", true},
		{"a.md", "b.md", false},
		{"a.md", "a.md", false},
		{"a.md.md", "a.md", true},
	}
	for _, c := range cases {
		if got := IsAfter(c.rel, c.cursor); got != c.want {
			t.Errorf("IsAfter(%q, %q) = %v, want %v", c.rel, c.cursor, got, c.want)
		}
	}
}

func TestScanReportsProgress(t *testing.T) {
	root := buildTree(t)

	var lastDone, lastFound int
	_, err := Scan(context.Background(), Options{
		Root:    root,
		Workers: 1, // 進捗の最終値を決定的にするため直列で走らせる
		OnProgress: func(done, found int) {
			lastDone, lastFound = done, found
		},
	})
	if err != nil {
		t.Fatalf("走査に失敗: %v", err)
	}
	if lastDone != 2 || lastFound != 2 {
		t.Fatalf("進捗通知が想定外: done=%d found=%d", lastDone, lastFound)
	}
}

func TestScanCancelled(t *testing.T) {
	root := buildTree(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Scan(ctx, Options{Root: root})
	if err == nil {
		t.Fatal("キャンセル済み context ではエラーを返すべき")
	}
}

func TestScanMissingRoot(t *testing.T) {
	_, err := Scan(context.Background(), Options{Root: filepath.Join(t.TempDir(), "存在しない")})
	if err == nil {
		t.Fatal("存在しないフォルダはエラーにすべき")
	}
}
