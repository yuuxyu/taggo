package app

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/yuuxyu/taggo/internal/folderconf"
	"github.com/yuuxyu/taggo/internal/trash"
)

// ページを削除すると、ファイルと一覧のエントリとピン留めが無くなり、
// ほかのノートの参照はそのまま残ること。
func TestDeletePage(t *testing.T) {
	a, root := newTestApp(t, map[string]string{
		"page.md":  "# page\n\n本文。\n",
		"a.md":     "# A\n\n[[page]] と [page](./page.md)\n",
		"other.md": "# その他\n",
	})
	page := filepath.Join(root, "page.md")
	for _, name := range []string{"page.md", "other.md"} {
		if err := a.SetPinned(filepath.Join(root, name), true); err != nil {
			t.Fatalf("ピン留めに失敗: %v", err)
		}
	}

	result, err := a.DeletePage(page)
	if err != nil {
		t.Fatalf("削除に失敗: %v", err)
	}
	if result.Cancelled || result.Warning != "" {
		t.Fatalf("結果が想定外: %+v", result)
	}
	if _, err := os.Stat(page); !os.IsNotExist(err) {
		t.Fatalf("ファイルが残っている: %v", err)
	}
	if _, ok := a.store.Get(page); ok {
		t.Fatal("DB にエントリが残っている")
	}

	// ほかのノートは書き換えず、参照はまだ無いページを指すようになる。
	got, err := os.ReadFile(filepath.Join(root, "a.md"))
	if err != nil || string(got) != "# A\n\n[[page]] と [page](./page.md)\n" {
		t.Fatalf("ほかのノートが変わった: %q, %v", got, err)
	}
	if e, err := a.TagPage("page"); err != nil || !e.Missing {
		t.Fatalf("削除したページがまだ無いページにならない: %+v, %v", e, err)
	}

	// ピン留めからも外れ、ほかのピン留めは残る。
	cfg, err := folderconf.Load(root)
	if err != nil {
		t.Fatalf("設定ファイルを読めない: %v", err)
	}
	if want := []string{"other.md"}; !reflect.DeepEqual(cfg.Pinned, want) {
		t.Fatalf("ピン留め: got %v, want %v", cfg.Pinned, want)
	}
	if a.store.IsPinned(page) {
		t.Fatal("削除したページがピン留めのまま")
	}
}

// ごみ箱の無いドライブで完全に消すのをやめたら、何も変えないこと。
func TestDeletePageCancelled(t *testing.T) {
	a, root := newTestApp(t, map[string]string{"page.md": "# page\n"})
	page := filepath.Join(root, "page.md")
	a.trash = func(string) error { return trash.ErrCancelled }

	result, err := a.DeletePage(page)
	if err != nil || !result.Cancelled {
		t.Fatalf("やめたことにならない: %+v, %v", result, err)
	}
	if _, ok := a.store.Get(page); !ok {
		t.Fatal("やめたのに DB からエントリが消えた")
	}
}

// ごみ箱へ移せなければ、一覧からも除かないこと。
func TestDeletePageFailed(t *testing.T) {
	a, root := newTestApp(t, map[string]string{"page.md": "# page\n"})
	page := filepath.Join(root, "page.md")
	a.trash = func(string) error { return errors.New("移せない") }

	if _, err := a.DeletePage(page); err == nil {
		t.Fatal("ごみ箱へ移せないのにエラーにならない")
	}
	if _, ok := a.store.Get(page); !ok {
		t.Fatal("削除できなかったのに DB からエントリが消えた")
	}
}

// 一覧に無いページ（まだ無いページ）や、フォルダの外のファイルは削除しないこと。
func TestDeletePageRejects(t *testing.T) {
	a, root := newTestApp(t, map[string]string{
		"page.md":     "# page\n",
		"sub/deep.md": "# deep\n",
	})
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("# outside\n"), 0o644); err != nil {
		t.Fatalf("ファイル作成に失敗: %v", err)
	}
	for _, path := range []string{
		filepath.Join(root, "missing.md"),
		filepath.Join(root, "sub", "deep.md"),
		outside,
		filepath.Join(root, "page.txt"),
	} {
		if _, err := a.DeletePage(path); err == nil {
			t.Errorf("%s を削除できてしまった", path)
		}
	}
	for _, path := range []string{filepath.Join(root, "sub", "deep.md"), outside} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s が消えた: %v", path, err)
		}
	}
}
