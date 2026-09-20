package meta

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
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

func TestSniffFormat(t *testing.T) {
	cases := []struct {
		name string
		head []byte
		want string
	}{
		{"jpeg", []byte{0xFF, 0xD8, 0xFF, 0xE0}, ".jpg"},
		{"png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, ".png"},
		{"gif", []byte("GIF89a...."), ".gif"},
		{"webp", []byte("RIFF\x00\x00\x00\x00WEBPVP8 "), ".webp"},
		{"wav", []byte("RIFF\x00\x00\x00\x00WAVEfmt "), ".wav"},
		{"flac", []byte("fLaC\x00\x00\x00\x22"), ".flac"},
		{"mp3 (ID3)", []byte("ID3\x04\x00\x00"), ".mp3"},
		{"mp3 (フレーム同期)", []byte{0xFF, 0xFB, 0x90, 0x00}, ".mp3"},
		{"bmp", []byte("BM\x00\x00\x00\x00"), ".bmp"},
		{"tiff", []byte("II*\x00\x08\x00\x00\x00"), ".tiff"},
		{"avif", []byte("\x00\x00\x00\x20ftypavif\x00\x00\x00\x00"), ".avif"},
		{"heic", []byte("\x00\x00\x00\x18ftypheic\x00\x00\x00\x00"), ".heic"},
		{"svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`), ".svg"},
		{"svg (XML 宣言付き)", []byte("<?xml version=\"1.0\"?>\n<svg></svg>"), ".svg"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// 拡張子はわざと中身と無関係なものにして、中身だけで判定していることを確かめる。
			path := writeBytes(t, "sample.dat", c.head)
			got, ok := SniffFormat(path)
			if !ok {
				t.Fatalf("判定できなかった")
			}
			if got != c.want {
				t.Fatalf("判定結果が違う: got %q, want %q", got, c.want)
			}
		})
	}
}

func TestSniffFormatUnknown(t *testing.T) {
	for _, c := range []struct {
		name string
		data []byte
	}{
		{"空ファイル", nil},
		{"ただのテキスト", []byte("これは画像ではありません")},
	} {
		t.Run(c.name, func(t *testing.T) {
			path := writeBytes(t, "sample.png", c.data)
			if got, ok := SniffFormat(path); ok {
				t.Fatalf("判定できないはずが %q を返した", got)
			}
		})
	}
}

// TestReadUsesContentNotExtension は、拡張子が中身と食い違っていても
// 正しいハンドラーでメタデータを読むことを確かめる。
func TestReadUsesContentNotExtension(t *testing.T) {
	// 中身は PNG、名前は .jpg。
	png := pngHandler{}
	src := makeTestImage(t, ".png")
	if err := png.WriteTags(src, []string{"偽装", "png"}); err != nil {
		t.Fatalf("タグ書き込みに失敗: %v", err)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	path := writeBytes(t, "実はpng.jpg", data)

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := Read(path, info)
	if err != nil {
		t.Fatalf("読み取りに失敗: %v", err)
	}

	if entry.Err != "" {
		t.Fatalf("拡張子の食い違いでエラーになった: %s", entry.Err)
	}
	if entry.Format != ".png" {
		t.Fatalf("実体の形式が .png と判定されていない: %q", entry.Format)
	}
	if entry.Ext != ".jpg" {
		t.Fatalf("表示用の拡張子は .jpg のままであるべき: %q", entry.Ext)
	}
	if want := []string{"png", "偽装"}; !reflect.DeepEqual(entry.Tags, want) {
		t.Fatalf("PNG として読めていない: got %v, want %v", entry.Tags, want)
	}
}

// TestWriteTagsUsesContentNotExtension は、拡張子を偽ったファイルへ
// 誤った形式で書き込んで壊さないことを確かめる。
func TestWriteTagsUsesContentNotExtension(t *testing.T) {
	png := pngHandler{}
	src := makeTestImage(t, ".png")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	path := writeBytes(t, "実はpng.jpg", data)

	if err := WriteTags(path, []string{"書き込み"}); err != nil {
		t.Fatalf("タグ書き込みに失敗: %v", err)
	}

	// 書き込み後も PNG のままであること。
	if format, ok := SniffFormat(path); !ok || format != ".png" {
		t.Fatalf("書き込みでファイル形式が壊れた: %q", format)
	}
	got, err := png.Read(path)
	if err != nil {
		t.Fatalf("書き込み後の読み取りに失敗: %v", err)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "書き込み" {
		t.Fatalf("書き戻したタグが一致しない: %v", got.Tags)
	}
}

// TestReadRecognizedButUnsupportedFormat は、判別はできるが扱えない形式を
// 「対応していない」と明示し、書き込み対象から外すことを確かめる。
func TestReadRecognizedButUnsupportedFormat(t *testing.T) {
	// 中身は HEIC、名前は .jpg。
	path := writeBytes(t, "実はheic.jpg", []byte("\x00\x00\x00\x18ftypheic\x00\x00\x00\x00mif1heic"))

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := Read(path, info)
	if err != nil {
		t.Fatalf("読み取りに失敗: %v", err)
	}
	if entry.Format != ".heic" {
		t.Fatalf("HEIC と判定されていない: %q", entry.Format)
	}
	if entry.Writable {
		t.Fatal("扱えない形式は書き込み不可であるべき")
	}
	if entry.Err == "" {
		t.Fatal("理由が記録されていない")
	}

	if err := WriteTags(path, []string{"x"}); err == nil {
		t.Fatal("扱えない形式への書き込みはエラーであるべき")
	}
}

// TestGIFIsRecognizedButUnsupported は、GIF・SVG・AAC/M4A 用の専用ハンドラーを
// 廃止したあとも、これらの形式が HEIC などと同じ「判定はできるが対応しない」
// 枠として扱われ、書き込みが拒否されることを確かめる。
func TestGIFIsRecognizedButUnsupported(t *testing.T) {
	// 中身は GIF、名前は .png。
	path := writeBytes(t, "実はgif.png", []byte("GIF89a"))

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := Read(path, info)
	if err != nil {
		t.Fatalf("読み取りに失敗: %v", err)
	}
	if entry.Format != ".gif" {
		t.Fatalf("GIF と判定されていない: %q", entry.Format)
	}
	if entry.Writable {
		t.Fatal("GIF は書き込み不可であるべき")
	}
	if entry.Err == "" {
		t.Fatal("理由が記録されていない")
	}

	if err := WriteTags(path, []string{"x"}); !errors.Is(err, ErrFormatReadOnly) {
		t.Fatalf("ErrFormatReadOnly を期待したが %v", err)
	}
}

func TestContentTypeUsesFormat(t *testing.T) {
	if got := ContentType(".avif"); got != "image/avif" {
		t.Fatalf("AVIF の MIME タイプが違う: %q", got)
	}
	if got := ContentType(".unknown"); got != "" {
		t.Fatalf("未知の形式には空を返すべき: %q", got)
	}
}
