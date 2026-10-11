package trash

import (
	"os"
	"path/filepath"
	"testing"
)

// 実際にごみ箱へ移すので、テストのたびにごみ箱へファイルが溜まらないよう、
// TAGGO_TEST_TRASH=1 のときだけ動かす。
func TestMove(t *testing.T) {
	if os.Getenv("TAGGO_TEST_TRASH") != "1" {
		t.Skip("ごみ箱へ移すテストは TAGGO_TEST_TRASH=1 のときだけ動かす")
	}
	path := filepath.Join(t.TempDir(), "taggo のごみ箱のテスト.md")
	if err := os.WriteFile(path, []byte("# テスト\n"), 0o644); err != nil {
		t.Fatalf("ファイルを作れない: %v", err)
	}
	if err := Move(path); err != nil {
		t.Fatalf("ごみ箱へ移せない: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("ファイルが残っている: %v", err)
	}
}

// 無いファイルは移せない。
func TestMoveMissing(t *testing.T) {
	if err := Move(filepath.Join(t.TempDir(), "無い.md")); err == nil {
		t.Fatal("無いファイルでエラーにならない")
	}
}
