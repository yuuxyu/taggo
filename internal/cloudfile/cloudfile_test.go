package cloudfile

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLooksLikeSyncFolder(t *testing.T) {
	cases := []struct {
		name  string
		parts []string
		want  string
	}{
		{"Dropbox 直下", []string{"C:", "Users", "taro", "Dropbox", "notes"}, "Dropbox"},
		{"OneDrive の組織名付き", []string{"C:", "Users", "taro", "OneDrive - Contoso"}, "OneDrive"},
		{"Google ドライブ", []string{"G:", "My Drive", "notes"}, "Google ドライブ"},
		{"iCloud Drive", []string{"Users", "taro", "iCloud Drive", "notes"}, "iCloud Drive"},
		{"大文字小文字を問わない", []string{"home", "taro", "dropbox"}, "Dropbox"},
		{"ふつうのフォルダ", []string{"home", "taro", "notes"}, ""},
		// 部分一致で拾いすぎないこと。"boxes" のような名前を Box と見なさない。
		{"紛らわしい名前", []string{"home", "taro", "boxes"}, ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// OS ごとの区切り文字でつないで、そのまま判定に渡す。
			got := LooksLikeSyncFolder(strings.Join(c.parts, string(filepath.Separator)))
			if got != c.want {
				t.Fatalf("判定が違う: got %q, want %q", got, c.want)
			}
		})
	}
}
