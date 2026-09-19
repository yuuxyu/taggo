package meta

import (
	"reflect"
	"testing"
)

func TestJPEGTagRoundTrip(t *testing.T) {
	path := makeTestImage(t, ".jpg")
	h := jpegHandler{}

	// Exif を持たない JPEG でも、読み取りはタグ 0 件で成功すること。
	got, err := h.Read(path)
	if err != nil {
		t.Fatalf("Exif 無し JPEG の読み取りに失敗: %v", err)
	}
	if len(got.Tags) != 0 {
		t.Fatalf("タグ 0 件を期待したが %v", got.Tags)
	}
	if got.Image == nil || got.Image.Width != 8 || got.Image.Height != 4 {
		t.Fatalf("画像サイズ 8x4 を期待したが %+v", got.Image)
	}

	want := []string{"golang", "開発メモ"}
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

	// 空のタグ集合を書くと XPKeywords ごと消えること。
	if err := h.WriteTags(path, nil); err != nil {
		t.Fatalf("タグ削除に失敗: %v", err)
	}
	got, err = h.Read(path)
	if err != nil {
		t.Fatalf("削除後の読み取りに失敗: %v", err)
	}
	if len(got.Tags) != 0 {
		t.Fatalf("タグが消えているはずが %v", got.Tags)
	}
}
