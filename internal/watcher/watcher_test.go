package watcher

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// waitChange は通知を 1 件待つ。時間内に来なければテストを失敗させる。
func waitChange(t *testing.T, w *Watcher) Change {
	t.Helper()
	select {
	case c := <-w.Changes():
		return c
	case <-time.After(3 * time.Second):
		t.Fatal("変更通知が届かなかった")
		return Change{}
	}
}

func startWatcher(t *testing.T, root string) *Watcher {
	t.Helper()
	w, err := New(root)
	if err != nil {
		t.Fatalf("監視の開始に失敗: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go w.Run(ctx)
	t.Cleanup(func() {
		cancel()
		w.Close()
	})
	return w
}

func TestWatcherDetectsCreateAndUpdate(t *testing.T) {
	root := t.TempDir()
	w := startWatcher(t, root)

	path := filepath.Join(root, "note.md")
	if err := os.WriteFile(path, []byte("# 新規\n\n[[作成]]\n"), 0o644); err != nil {
		t.Fatalf("ファイル作成に失敗: %v", err)
	}

	c := waitChange(t, w)
	if c.Removed || c.Entry == nil {
		t.Fatalf("作成が検知されていない: %+v", c)
	}
	if len(c.Entry.Tags) != 1 || c.Entry.Tags[0] != "作成" {
		t.Fatalf("作成時のタグが読めていない: %v", c.Entry.Tags)
	}

	if err := os.WriteFile(path, []byte("# 更新後\n\n[[更新]]\n"), 0o644); err != nil {
		t.Fatalf("ファイル更新に失敗: %v", err)
	}
	c = waitChange(t, w)
	if c.Entry == nil || c.Entry.Tags[0] != "更新" {
		t.Fatalf("更新が反映されていない: %+v", c)
	}
}

func TestWatcherDetectsRemove(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.md")
	if err := os.WriteFile(path, []byte("# 消される\n"), 0o644); err != nil {
		t.Fatalf("ファイル作成に失敗: %v", err)
	}

	w := startWatcher(t, root)
	if err := os.Remove(path); err != nil {
		t.Fatalf("ファイル削除に失敗: %v", err)
	}

	c := waitChange(t, w)
	if !c.Removed || c.Path != path {
		t.Fatalf("削除が検知されていない: %+v", c)
	}
}

// サブフォルダの中の変更は、走査と同じく対象にしないこと。
func TestWatcherIgnoresSubdirectories(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatalf("ディレクトリ作成に失敗: %v", err)
	}
	w := startWatcher(t, root)

	if err := os.WriteFile(filepath.Join(sub, "deep.md"), []byte("# 深い\n"), 0o644); err != nil {
		t.Fatalf("ファイル作成に失敗: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "new"), 0o755); err != nil {
		t.Fatalf("ディレクトリ作成に失敗: %v", err)
	}

	select {
	case c := <-w.Changes():
		t.Fatalf("サブフォルダの変更が通知された: %+v", c)
	case <-time.After(700 * time.Millisecond):
		// 想定どおり無通知。
	}
}

// フォルダの設定ファイル（.taggo.json）の書き換えは、設定の変更として通知されること。
func TestWatcherReportsConfigChanges(t *testing.T) {
	root := t.TempDir()
	w := startWatcher(t, root)

	path := filepath.Join(root, ".taggo.json")
	if err := os.WriteFile(path, []byte(`{"pinned":["a.md"]}`), 0o644); err != nil {
		t.Fatalf("ファイル作成に失敗: %v", err)
	}
	c := waitChange(t, w)
	if !c.Config || c.Entry != nil || c.Removed {
		t.Fatalf("設定の変更として通知されていない: %+v", c)
	}
}

func TestWatcherIgnoresUnsupportedAndTempFiles(t *testing.T) {
	root := t.TempDir()
	w := startWatcher(t, root)

	// 対象外の拡張子と、taggo 自身の一時ファイルは通知されない。
	if err := os.WriteFile(filepath.Join(root, "memo.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("ファイル作成に失敗: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".note.md.taggo-123"), []byte("x"), 0o644); err != nil {
		t.Fatalf("ファイル作成に失敗: %v", err)
	}

	select {
	case c := <-w.Changes():
		t.Fatalf("通知されるべきでない変更が届いた: %+v", c)
	case <-time.After(700 * time.Millisecond):
		// 想定どおり無通知。
	}
}
