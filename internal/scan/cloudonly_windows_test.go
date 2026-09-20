//go:build windows

package scan

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// fileAttributeOffline は、クラウド同期が「中身はここに無い」と示すために使う属性。
// 本物のプレースホルダーは作れないが、判定が見ているのは属性だけなので、
// これで走査の分岐をそのまま確かめられる。
const fileAttributeOffline = 0x00001000

func TestScanDoesNotReadCloudOnlyFiles(t *testing.T) {
	root := t.TempDir()

	// 中身にタグを書いておく。読んでしまえばタグが付くので、
	// 「タグが空のまま」であることが「開いていない」ことの証拠になる。
	body := []byte("---\ntags:\n  - よんだら付くタグ\n---\n\n# 本文\n")
	local := filepath.Join(root, "local.md")
	if err := os.WriteFile(local, body, 0o644); err != nil {
		t.Fatal(err)
	}
	cloud := filepath.Join(root, "cloud.md")
	if err := os.WriteFile(cloud, body, 0o644); err != nil {
		t.Fatal(err)
	}

	p, err := syscall.UTF16PtrFromString(cloud)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.SetFileAttributes(p, fileAttributeOffline); err != nil {
		t.Skipf("OFFLINE 属性を付けられない環境のため確認を飛ばす: %v", err)
	}

	got, err := Scan(context.Background(), Options{Root: root})
	if err != nil {
		t.Fatalf("走査に失敗: %v", err)
	}
	if len(got.Entries) != 2 {
		t.Fatalf("2 件とも一覧に出るはず: %d 件", len(got.Entries))
	}
	if got.CloudOnly != 1 {
		t.Fatalf("クラウド専用の件数が違う: got %d, want 1", got.CloudOnly)
	}

	for _, e := range got.Entries {
		switch e.Name {
		case "local.md":
			if e.CloudOnly {
				t.Fatal("ローカルのファイルをクラウド専用と誤判定した")
			}
			if len(e.Tags) != 1 {
				t.Fatalf("ローカルのファイルはタグを読めているはず: %v", e.Tags)
			}
		case "cloud.md":
			if !e.CloudOnly {
				t.Fatal("クラウド専用として扱われていない")
			}
			if len(e.Tags) != 0 {
				t.Fatalf("中身を読んでしまっている: %v", e.Tags)
			}
			if e.Writable {
				t.Fatal("取り込むまでは書き込み不可であるべき")
			}
			// 一覧に出すのに要る情報は、ファイルを開かずとも埋まっていること。
			if e.Size != int64(len(body)) {
				t.Fatalf("サイズが取れていない: %d", e.Size)
			}
			if e.RelPath != "cloud.md" {
				t.Fatalf("相対パスが違う: %q", e.RelPath)
			}
		default:
			t.Fatalf("想定外のエントリ: %s", e.Name)
		}
	}
}
