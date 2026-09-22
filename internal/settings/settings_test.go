package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yuuxyu/taggo/internal/store"
)

func TestLoadMissingFileUsesDefaultsWithoutCreatingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "taggo", "settings.json")

	st := Load(path)
	if warning := st.LoadWarning(); warning != "" {
		t.Fatalf("ファイルが無いだけで警告が出た: %s", warning)
	}
	if st.Get() != Default() {
		t.Fatalf("既定値になっていない: %+v", st.Get())
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("読み込んだだけで設定ファイルが作られた: %v", err)
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "taggo", "settings.json")
	st := Load(path)

	want := Settings{
		StartupMode:   StartupFixed,
		StartupFolder: `C:\notes`,
		Sort:          store.SortNameAsc,
		ScanLimit:     5000,
		Theme:         ThemeDark,
	}
	if err := st.Save(want); err != nil {
		t.Fatalf("保存に失敗: %v", err)
	}
	want.Version = CurrentVersion

	reloaded := Load(path)
	if warning := reloaded.LoadWarning(); warning != "" {
		t.Fatalf("保存したファイルを読んで警告が出た: %s", warning)
	}
	if reloaded.Get() != want {
		t.Fatalf("保存した設定が戻らない: got %+v, want %+v", reloaded.Get(), want)
	}

	// 一時ファイルが残っていないこと。
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("設定ファイル以外が残っている: %v", entries)
	}
}

func TestSaveRejectsInvalidSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	st := Load(path)

	cases := map[string]func(*Settings){
		"フォルダ未指定": func(s *Settings) { s.StartupMode = StartupFixed },
		"不明な並び順":  func(s *Settings) { s.Sort = "random" },
		"上限が小さい":  func(s *Settings) { s.ScanLimit = MinScanLimit - 1 },
		"上限が大きい":  func(s *Settings) { s.ScanLimit = MaxScanLimit + 1 },
		"不明なテーマ":  func(s *Settings) { s.Theme = "sepia" },
	}
	for name, mutate := range cases {
		s := Default()
		mutate(&s)
		if err := st.Save(s); err == nil {
			t.Errorf("%s: 保存できてしまった", name)
		}
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("不正な設定でファイルが作られた: %v", err)
	}
	if st.Get() != Default() {
		t.Fatalf("不正な設定で今の設定が変わった: %+v", st.Get())
	}
}

func TestLoadNormalizesUnknownValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	raw := `{"version": 9, "startupMode": "fixed", "sort": "random", "scanLimit": 5, "theme": "sepia", "future": true}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	st := Load(path)
	if warning := st.LoadWarning(); warning != "" {
		t.Fatalf("読める JSON で警告が出た: %s", warning)
	}
	if st.Get() != Default() {
		t.Fatalf("知らない値が既定値へ直っていない: %+v", st.Get())
	}
}

func TestLoadKeepsDefaultsForMissingFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"theme": "light"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	st := Load(path)
	want := Default()
	want.Theme = ThemeLight
	if st.Get() != want {
		t.Fatalf("書かれていない項目が既定値になっていない: %+v", st.Get())
	}
}

func TestLoadBrokenFileMovesItAside(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte("{壊れている"), 0o644); err != nil {
		t.Fatal(err)
	}

	st := Load(path)
	if warning := st.LoadWarning(); !strings.Contains(warning, "壊れていた") {
		t.Fatalf("壊れたファイルの警告が出ない: %q", warning)
	}
	if st.Get() != Default() {
		t.Fatalf("既定値で始まっていない: %+v", st.Get())
	}
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Fatalf("壊れたファイルが退避されていない: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("壊れたファイルが元の場所に残っている: %v", err)
	}
}

func TestRememberFolderOnlyWhenStartupLast(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	st := Load(path)

	// 既定（何も開かない）のままなら、開いたフォルダは記録しない。
	if err := st.RememberFolder(`C:\a`); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("前回のフォルダを使わない設定なのにファイルが作られた: %v", err)
	}

	s := Default()
	s.StartupMode = StartupLast
	if err := st.Save(s); err != nil {
		t.Fatal(err)
	}
	if err := st.RememberFolder(`C:\b`); err != nil {
		t.Fatal(err)
	}
	reloaded := Load(path)
	if got := reloaded.Get().StartupPath(); got != `C:\b` {
		t.Fatalf("前回のフォルダが記録されていない: %q", got)
	}
}

func TestSaveWithoutPathFails(t *testing.T) {
	st := Load("")
	if err := st.Save(Default()); err == nil {
		t.Fatal("保存先が無いのに保存できてしまった")
	}
}
