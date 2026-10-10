package app

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yuuxyu/taggo/internal/folderconf"
)

// ページの名前を変えると、ファイル名と、そのページを指す参照がすべて新しい名前になること。
func TestRenamePage(t *testing.T) {
	a, root := newTestApp(t, map[string]string{
		"page.md":  "# page\n\n本文。[[golang]]\n",
		"a.md":     "# A\n\n[[page]] と [page](./page.md) と [詳しく](page.md#節)\n",
		"b.md":     "# B\n\n`[[page]]` は書き換えない。\n",
		"other.md": "# その他\n\n[[golang]]\n",
	})
	if err := a.SetPinned(filepath.Join(root, "page.md"), true); err != nil {
		t.Fatalf("ピン留めに失敗: %v", err)
	}

	plan, err := a.PlanRename(filepath.Join(root, "page.md"), " 新しい  名前 ")
	if err != nil {
		t.Fatalf("見込みの計算に失敗: %v", err)
	}
	if plan.From != "page" || plan.To != "新しい 名前" {
		t.Fatalf("名前が違う: %+v", plan)
	}
	if want := []string{"a.md", "page.md"}; !reflect.DeepEqual(plan.Notes, want) {
		t.Fatalf("書き換えるノート: got %v, want %v", plan.Notes, want)
	}
	// 見込みを出すだけでは、ファイルは変えない。
	if _, err := os.Stat(filepath.Join(root, "page.md")); err != nil {
		t.Fatalf("見込みの計算でファイルが変わった: %v", err)
	}

	result, err := a.RenamePage(filepath.Join(root, "page.md"), " 新しい  名前 ")
	if err != nil {
		t.Fatalf("名前の変更に失敗: %v", err)
	}
	newPath := filepath.Join(root, "新しい 名前.md")
	if result.Entry == nil || result.Entry.Path != newPath || result.Rewritten != 2 || len(result.Failed) != 0 {
		t.Fatalf("結果が想定外: %+v", result)
	}

	want := map[string]string{
		"新しい 名前.md": "# 新しい 名前\n\n本文。[[golang]]\n",
		"a.md":      "# A\n\n[[新しい 名前]] と [新しい 名前](./新しい%20名前.md) と [詳しく](新しい%20名前.md#節)\n",
		"b.md":      "# B\n\n`[[page]]` は書き換えない。\n",
		"other.md":  "# その他\n\n[[golang]]\n",
	}
	for name, content := range want {
		got, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("%s を読めない: %v", name, err)
		}
		if string(got) != content {
			t.Errorf("%s の中身が違う\n got: %q\nwant: %q", name, got, content)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "page.md")); !os.IsNotExist(err) {
		t.Fatalf("元のファイルが残っている: %v", err)
	}

	// 一覧（DB）も新しい名前に付け替わり、書き換えたノートのタグも新しくなる。
	if _, ok := a.store.Get(filepath.Join(root, "page.md")); ok {
		t.Fatal("DB に元のエントリが残っている")
	}
	if e, ok := a.store.Get(newPath); !ok || e.Title != "新しい 名前" {
		t.Fatalf("DB のエントリが想定外: %+v", e)
	}
	if e, _ := a.store.Get(filepath.Join(root, "a.md")); !reflect.DeepEqual(e.Tags, []string{"新しい 名前"}) {
		t.Fatalf("書き換えたノートのタグが古い: %v", e.Tags)
	}

	// ピン留めも新しいファイル名へ付け替わる。
	cfg, err := folderconf.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"新しい 名前.md"}; !reflect.DeepEqual(cfg.Pinned, want) {
		t.Fatalf("ピン留めが付け替わっていない: %v", cfg.Pinned)
	}
	if !a.store.IsPinned(newPath) {
		t.Fatal("DB のピン留めが付け替わっていない")
	}
}

// 大文字小文字だけを変えられること。
func TestRenamePageCaseOnly(t *testing.T) {
	a, root := newTestApp(t, map[string]string{
		"page.md": "# page\n",
		"a.md":    "[[page]]\n",
	})
	if _, err := a.RenamePage(filepath.Join(root, "page.md"), "Page"); err != nil {
		t.Fatalf("名前の変更に失敗: %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if !reflect.DeepEqual(names, []string{"Page.md", "a.md"}) {
		t.Fatalf("ファイル名が想定外: %v", names)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "a.md")); string(got) != "[[Page]]\n" {
		t.Fatalf("参照が書き換わっていない: %q", got)
	}
}

// 使えない名前や、既にある名前には変えないこと。
func TestRenamePageRejects(t *testing.T) {
	a, root := newTestApp(t, map[string]string{
		"page.md":        "# page\n",
		"exists.md":      "# exists\n",
		"other.markdown": "# other\n",
		"a.md":           "[[page]]\n",
	})
	path := filepath.Join(root, "page.md")
	cases := map[string]string{
		"空":              "  ",
		"同じ名前":           "page",
		"既にある":           "Exists",
		"拡張子違いで既にある":     "other",
		"ファイル名に使えない文字":   "a/b",
		"[[タグ]] に書けない文字": "a[b]",
	}
	for name, newName := range cases {
		if _, err := a.PlanRename(path, newName); err == nil {
			t.Errorf("%s: 見込みの計算でエラーにならない", name)
		}
		if _, err := a.RenamePage(path, newName); err == nil {
			t.Errorf("%s: エラーにならない", name)
		}
	}
	if got, _ := os.ReadFile(filepath.Join(root, "a.md")); !strings.Contains(string(got), "[[page]]") {
		t.Fatalf("断ったのに参照を書き換えた: %q", got)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("断ったのにファイルが変わった: %v", err)
	}
}
