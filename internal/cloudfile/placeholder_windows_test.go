//go:build windows

package cloudfile

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// markOffline は、クラウド同期が立てるのと同じ「中身がここに無い」印を付ける。
// 本物のプレースホルダーは作れないが、判定が見ているのは属性だけなので、
// これで検出経路をそのまま確かめられる。
func markOffline(t *testing.T, path string) {
	t.Helper()
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatalf("パスを変換できない: %v", err)
	}
	if err := syscall.SetFileAttributes(p, fileAttributeOffline); err != nil {
		t.Skipf("OFFLINE 属性を付けられない環境のため確認を飛ばす: %v", err)
	}
}

func TestIsPlaceholder(t *testing.T) {
	dir := t.TempDir()

	normal := filepath.Join(dir, "local.png")
	if err := os.WriteFile(normal, []byte("dummy"), 0o644); err != nil {
		t.Fatal(err)
	}
	offline := filepath.Join(dir, "cloud.png")
	if err := os.WriteFile(offline, []byte("dummy"), 0o644); err != nil {
		t.Fatal(err)
	}
	markOffline(t, offline)

	infoNormal, err := os.Stat(normal)
	if err != nil {
		t.Fatal(err)
	}
	if IsPlaceholder(infoNormal) {
		t.Fatal("ローカルにあるファイルをクラウド専用と誤判定した")
	}

	infoOffline, err := os.Stat(offline)
	if err != nil {
		t.Fatal(err)
	}
	if !IsPlaceholder(infoOffline) {
		t.Fatal("OFFLINE 属性のファイルを検出できなかった")
	}
}

func TestIsPlaceholderNil(t *testing.T) {
	if IsPlaceholder(nil) {
		t.Fatal("nil を渡したらクラウド専用と見なしてはいけない")
	}
}
