package meta

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yuuxyu/taggo/internal/notefile"
)

// withBOM は、先頭に BOM を付けた中身を作る。
func withBOM(s string) []byte { return append(append([]byte{}, notefile.BOM...), s...) }

// 先頭に BOM があっても、1 行目の見出しを見出しとして読み、抜粋にも BOM を入れないこと。
// メモ帳などで保存したノートには BOM が付いていることがある。
func TestParseIgnoresBOM(t *testing.T) {
	got := Parse(withBOM("# 見出し\r\n\r\n[[golang]] の本文\r\n"))
	if got.Title != "見出し" {
		t.Fatalf("BOM の後ろの見出しを読めていない: %q", got.Title)
	}
	if strings.HasPrefix(got.Preview, string(notefile.BOM)) || !strings.Contains(got.Preview, "golang の本文") {
		t.Fatalf("抜粋が違う: %q", got.Preview)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "golang" {
		t.Fatalf("タグが違う: %v", got.Tags)
	}
}

// 先頭に BOM があっても、Front Matter を Front Matter として見分けること。
func TestParseIgnoresBOMBeforeFrontMatter(t *testing.T) {
	got := Parse(withBOM("---\ntitle: 使わない\n---\n# 見出し\n\n本文\n"))
	if got.Title != "見出し" || strings.Contains(got.Preview, "使わない") {
		t.Fatalf("BOM の後ろの Front Matter を見分けていない: %+v", got)
	}
}

// ファイルから読むときも、BOM の後ろの見出しをタイトルにすること。
func TestReadUsesHeadingAfterBOM(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notepad.md")
	if err := os.WriteFile(path, withBOM("# メモ帳で書いた\r\n本文\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	e, err := Read(path, info)
	if err != nil {
		t.Fatal(err)
	}
	if e.Title != "メモ帳で書いた" {
		t.Fatalf("タイトルがファイル名のまま: %q", e.Title)
	}
}

// 名前を変えるとき、BOM の後ろの 1 行目の見出しも書き換え、BOM は残すこと。
func TestRenameRefsRewriteKeepsBOM(t *testing.T) {
	r := RenameRefs{OldName: "page", NewName: "newpage", LinksTo: linksToPage, Self: true}

	got, changed := r.Rewrite(withBOM("# page\r\n\r\n[[page]] と本文\r\n"))
	if want := withBOM("# newpage\r\n\r\n[[newpage]] と本文\r\n"); !changed || string(got) != string(want) {
		t.Fatalf("書き換えが違う:\n%q\nwant\n%q", got, want)
	}

	// Front Matter の中は、BOM があっても書き換えない。
	in := withBOM("---\nalias: [[page]]\n---\n# page\n")
	got, _ = r.Rewrite(in)
	if want := withBOM("---\nalias: [[page]]\n---\n# newpage\n"); string(got) != string(want) {
		t.Fatalf("Front Matter の扱いが違う:\n%q\nwant\n%q", got, want)
	}
}
