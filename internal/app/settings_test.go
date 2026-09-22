package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yuuxyu/taggo/internal/settings"
	"github.com/yuuxyu/taggo/internal/store"
)

// newSettingsApp は、settingsPath の設定ファイルを使うアプリを組み立てる。
// 同じパスで組み立て直すと、アプリの再起動を模せる。
func newSettingsApp(t *testing.T, settingsPath string) *App {
	t.Helper()
	a, err := New(settings.Load(settingsPath))
	if err != nil {
		t.Fatalf("アプリの初期化に失敗: %v", err)
	}
	t.Cleanup(func() { a.Shutdown(context.Background()) })
	return a
}

// newNotesFolder は Markdown を 1 件だけ置いたフォルダを作る。
func newNotesFolder(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "note.md"), []byte("# note\n"), 0o644); err != nil {
		t.Fatalf("ファイル作成に失敗: %v", err)
	}
	return root
}

func TestStartupLastReopensFolderAfterRestart(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	root := newNotesFolder(t)

	a := newSettingsApp(t, settingsPath)
	if err := a.OpenFolder(root); err != nil {
		t.Fatal(err)
	}
	waitForIdle(t, a)

	// 「前回のフォルダ」へ切り替えた時点で開いているフォルダを覚える。
	s := a.GetSettings()
	s.StartupMode = settings.StartupLast
	saved, err := a.SaveSettings(s)
	if err != nil {
		t.Fatalf("設定の保存に失敗: %v", err)
	}
	if saved.LastFolder != root {
		t.Fatalf("開いているフォルダが前回のフォルダになっていない: %q", saved.LastFolder)
	}

	restarted := newSettingsApp(t, settingsPath)
	restarted.OpenStartupFolder("")
	waitForIdle(t, restarted)
	if got := restarted.Status().Root; got != root {
		t.Fatalf("再起動後に前回のフォルダが開かれない: %q", got)
	}
	if w := restarted.TakeStartupWarnings(); len(w) != 0 {
		t.Fatalf("警告が出た: %v", w)
	}
}

func TestStartupArgumentWinsOverSettings(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	fixed := newNotesFolder(t)
	arg := newNotesFolder(t)

	a := newSettingsApp(t, settingsPath)
	s := a.GetSettings()
	s.StartupMode = settings.StartupFixed
	s.StartupFolder = fixed
	if _, err := a.SaveSettings(s); err != nil {
		t.Fatal(err)
	}

	a.OpenStartupFolder(arg)
	waitForIdle(t, a)
	if got := a.Status().Root; got != arg {
		t.Fatalf("コマンドライン引数のフォルダが開かれない: %q", got)
	}
}

func TestStartupMissingFolderWarnsOnce(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	missing := filepath.Join(t.TempDir(), "外したドライブ")

	a := newSettingsApp(t, settingsPath)
	s := a.GetSettings()
	s.StartupMode = settings.StartupFixed
	s.StartupFolder = missing
	if _, err := a.SaveSettings(s); err != nil {
		t.Fatal(err)
	}

	a.OpenStartupFolder("")
	if got := a.Status().Root; got != "" {
		t.Fatalf("見つからないフォルダを開こうとした: %q", got)
	}
	warnings := a.TakeStartupWarnings()
	if len(warnings) != 1 || !strings.Contains(warnings[0], missing) {
		t.Fatalf("見つからなかったことが伝わらない: %v", warnings)
	}
	if again := a.TakeStartupWarnings(); len(again) != 0 {
		t.Fatalf("同じ警告が 2 度返った: %v", again)
	}
}

func TestSaveSettingsAppliesScanLimit(t *testing.T) {
	a := newSettingsApp(t, filepath.Join(t.TempDir(), "settings.json"))
	if got := a.Status().MaxEntries; got != store.MaxEntries {
		t.Fatalf("既定の上限になっていない: %d", got)
	}

	s := a.GetSettings()
	s.ScanLimit = 5000
	if _, err := a.SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	if got := a.Status().MaxEntries; got != 5000 {
		t.Fatalf("上限が反映されていない: %d", got)
	}
}

func TestSaveSettingsKeepsCurrentOnError(t *testing.T) {
	a := newSettingsApp(t, filepath.Join(t.TempDir(), "settings.json"))

	s := a.GetSettings()
	s.ScanLimit = 1
	got, err := a.SaveSettings(s)
	if err == nil {
		t.Fatal("範囲外の上限で保存できてしまった")
	}
	if got != settings.Default() || a.Status().MaxEntries != store.MaxEntries {
		t.Fatalf("保存に失敗したのに設定が変わった: %+v", got)
	}
}
