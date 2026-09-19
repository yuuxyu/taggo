package thumb

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeTestPNG は指定サイズのテスト画像を作る。
func writeTestPNG(t *testing.T, dir string, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 128, A: 255})
		}
	}
	path := filepath.Join(dir, "sample.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("テスト画像の作成に失敗: %v", err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("テスト画像のエンコードに失敗: %v", err)
	}
	return path
}

func decodeSize(t *testing.T, data []byte) (int, int) {
	t.Helper()
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("サムネイルのデコードに失敗: %v", err)
	}
	return cfg.Width, cfg.Height
}

func TestGenerateScalesDown(t *testing.T) {
	path := writeTestPNG(t, t.TempDir(), 800, 400)
	c := NewCache()

	data, err := c.Get(path, 200)
	if err != nil {
		t.Fatalf("サムネイル生成に失敗: %v", err)
	}
	w, h := decodeSize(t, data)
	if w != 200 || h != 100 {
		t.Fatalf("縦横比を保った縮小になっていない: %dx%d", w, h)
	}
}

func TestGenerateDoesNotUpscale(t *testing.T) {
	path := writeTestPNG(t, t.TempDir(), 100, 50)
	c := NewCache()

	data, err := c.Get(path, 800)
	if err != nil {
		t.Fatalf("サムネイル生成に失敗: %v", err)
	}
	w, h := decodeSize(t, data)
	if w != 100 || h != 50 {
		t.Fatalf("元画像より拡大されている: %dx%d", w, h)
	}
}

func TestCacheRegeneratesAfterFileChange(t *testing.T) {
	dir := t.TempDir()
	path := writeTestPNG(t, dir, 400, 400)
	c := NewCache()

	first, err := c.Get(path, 100)
	if err != nil {
		t.Fatalf("サムネイル生成に失敗: %v", err)
	}

	// 更新日時の差が出るよう、少し待ってから別サイズの画像で上書きする。
	time.Sleep(10 * time.Millisecond)
	writeTestPNG(t, dir, 400, 200)
	if err := os.Chtimes(path, time.Now(), time.Now()); err != nil {
		t.Fatalf("更新日時の変更に失敗: %v", err)
	}

	second, err := c.Get(path, 100)
	if err != nil {
		t.Fatalf("再生成に失敗: %v", err)
	}
	_, h1 := decodeSize(t, first)
	_, h2 := decodeSize(t, second)
	if h1 == h2 {
		t.Fatalf("ファイル変更後も古いサムネイルが返っている: %d == %d", h1, h2)
	}
}

func TestInvalidateAndClear(t *testing.T) {
	path := writeTestPNG(t, t.TempDir(), 200, 200)
	c := NewCache()

	if _, err := c.Get(path, 100); err != nil {
		t.Fatalf("サムネイル生成に失敗: %v", err)
	}
	if len(c.order) != 1 {
		t.Fatalf("キャッシュに載っていない: %v", c.order)
	}

	c.Invalidate(path)
	if len(c.order) != 0 || len(c.items) != 0 {
		t.Fatalf("Invalidate でキャッシュが消えていない: %v", c.order)
	}

	if _, err := c.Get(path, 100); err != nil {
		t.Fatalf("再生成に失敗: %v", err)
	}
	c.Clear()
	if len(c.items) != 0 {
		t.Fatalf("Clear でキャッシュが消えていない: %v", c.items)
	}
}

func TestGetRejectsNonImage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(path, []byte("# 画像ではない"), 0o644); err != nil {
		t.Fatalf("ファイル作成に失敗: %v", err)
	}
	if _, err := NewCache().Get(path, 100); err == nil {
		t.Fatal("画像でないファイルはエラーにすべき")
	}
}
