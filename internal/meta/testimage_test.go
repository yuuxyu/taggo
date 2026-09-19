package meta

import (
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// makeTestImage は指定拡張子の小さな単色画像を一時ディレクトリに作り、そのパスを返す。
func makeTestImage(t *testing.T, ext string) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 100, B: 50, A: 255})
		}
	}

	path := filepath.Join(t.TempDir(), "sample"+ext)
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("テスト画像の作成に失敗: %v", err)
	}
	defer f.Close()

	switch ext {
	case ".jpg", ".jpeg":
		err = jpeg.Encode(f, img, nil)
	case ".png":
		err = png.Encode(f, img)
	default:
		t.Fatalf("未対応のテスト拡張子: %s", ext)
	}
	if err != nil {
		t.Fatalf("テスト画像のエンコードに失敗: %v", err)
	}
	return path
}
