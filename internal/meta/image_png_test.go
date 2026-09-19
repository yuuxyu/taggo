package meta

import (
	"reflect"
	"testing"
)

func TestPNGTagRoundTrip(t *testing.T) {
	path := makeTestImage(t, ".png")
	h := pngHandler{}

	// eXIf チャンクを持たない PNG でも、読み取りはタグ 0 件で成功すること。
	got, err := h.Read(path)
	if err != nil {
		t.Fatalf("Exif 無し PNG の読み取りに失敗: %v", err)
	}
	if len(got.Tags) != 0 {
		t.Fatalf("タグ 0 件を期待したが %v", got.Tags)
	}
	if got.Image == nil || got.Image.Width != 8 || got.Image.Height != 4 {
		t.Fatalf("画像サイズ 8x4 を期待したが %+v", got.Image)
	}

	want := []string{"写真", "sunset"}
	if err := h.WriteTags(path, want); err != nil {
		t.Fatalf("タグ書き込みに失敗: %v", err)
	}

	got, err = h.Read(path)
	if err != nil {
		t.Fatalf("書き込み後の読み取りに失敗: %v", err)
	}
	if !reflect.DeepEqual(got.Tags, want) {
		t.Fatalf("書き戻したタグが一致しない: got %v, want %v", got.Tags, want)
	}

	// 書き換え後も PNG として正しくデコードできること（チャンク破壊の検出）。
	if w, hgt := imageDimensions(path); w != 8 || hgt != 4 {
		t.Fatalf("書き換え後の PNG が壊れている: %dx%d", w, hgt)
	}
}
