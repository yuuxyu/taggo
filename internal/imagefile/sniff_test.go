package imagefile

import (
	"os"
	"path/filepath"
	"testing"
)

// writeBytes はテスト用に、任意のバイト列を任意の名前で書き出す。
func writeBytes(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("テストファイルの作成に失敗: %v", err)
	}
	return path
}

func TestContentType(t *testing.T) {
	cases := []struct {
		name string
		head []byte
		want string
	}{
		{"jpeg", []byte{0xFF, 0xD8, 0xFF, 0xE0}, "image/jpeg"},
		{"png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, "image/png"},
		{"gif", []byte("GIF89a...."), "image/gif"},
		{"webp", []byte("RIFF\x00\x00\x00\x00WEBPVP8 "), "image/webp"},
		{"bmp", []byte("BM\x00\x00\x00\x00"), "image/bmp"},
		{"avif", []byte("\x00\x00\x00\x20ftypavif\x00\x00\x00\x00"), "image/avif"},
		{"avif (互換ブランド)", []byte("\x00\x00\x00\x18ftypmif1\x00\x00\x00\x00avif"), "image/avif"},
		{"svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`), "image/svg+xml"},
		{"svg (XML 宣言付き)", []byte("<?xml version=\"1.0\"?>\n<svg></svg>"), "image/svg+xml"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// 拡張子はわざと中身と無関係なものにして、中身だけで判定していることを確かめる。
			path := writeBytes(t, "sample.dat", c.head)
			got, ok := ContentType(path)
			if !ok {
				t.Fatalf("判定できなかった")
			}
			if got != c.want {
				t.Fatalf("判定結果が違う: got %q, want %q", got, c.want)
			}
		})
	}
}

// TestContentTypeRejectsNonImages は、画像でないものや WebView で表示できない形式を
// 画像として扱わないことを確かめる。
func TestContentTypeRejectsNonImages(t *testing.T) {
	for _, c := range []struct {
		name string
		data []byte
	}{
		{"空ファイル", nil},
		{"ただのテキスト", []byte("これは画像ではありません")},
		{"Markdown", []byte("---\ntags: [a]\n---\n# 見出し\n")},
		{"heic", []byte("\x00\x00\x00\x18ftypheic\x00\x00\x00\x00mif1heic")},
		{"tiff", []byte("II*\x00\x08\x00\x00\x00")},
		{"wav", []byte("RIFF\x00\x00\x00\x00WAVEfmt ")},
		{"mp3", []byte("ID3\x04\x00\x00")},
	} {
		t.Run(c.name, func(t *testing.T) {
			path := writeBytes(t, "sample.png", c.data)
			if got, ok := ContentType(path); ok {
				t.Fatalf("画像とみなさないはずが %q を返した", got)
			}
		})
	}
}

func TestContentTypeMissingFile(t *testing.T) {
	if _, ok := ContentType(filepath.Join(t.TempDir(), "無い.png")); ok {
		t.Fatal("存在しないファイルを画像とみなした")
	}
}
