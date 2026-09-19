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
		"a.md":                "---\ntags: [golang]\n---\n\n# A\n",
		"sub/b.md":            "---\ntags: [設計]\n---\n\n# B\n",
		"sub/notes.txt":       "対象外の拡張子",
		"node_modules/dep.md": "---\ntags: [除外]\n---\n\n# Dep\n",
		".hidden/secret.md":   "---\ntags: [隠し]\n---\n\n# Secret\n",
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
	if len(paths) != 2 {
		t.Fatalf("対象は a.md と sub/b.md の 2 件のはず: %v", paths)
	}
	for _, p := range paths {
		if p == "node_modules/dep.md" || p == ".hidden/secret.md" {
			t.Fatalf("除外されるべきディレクトリが走査された: %v", paths)
		}
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
	if tags := found["sub/b.md"]; len(tags) != 1 || tags[0] != "設計" {
		t.Fatalf("sub/b.md のタグが読めていない: %v", tags)
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
