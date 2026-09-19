package meta

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// copyFixture はテストデータを一時ディレクトリへ複製し、書き換えテストが
// リポジトリ内のファイルを汚さないようにする。
func copyFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("テストデータの読み込みに失敗: %v", err)
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("テストデータの複製に失敗: %v", err)
	}
	return path
}

func TestWebPTagRoundTrip(t *testing.T) {
	path := copyFixture(t, "sample.webp")
	h := webpHandler{}

	got, err := h.Read(path)
	if err != nil {
		t.Fatalf("メタデータ無し WebP の読み取りに失敗: %v", err)
	}
	if len(got.Tags) != 0 {
		t.Fatalf("タグ 0 件を期待したが %v", got.Tags)
	}
	if got.Image == nil || got.Image.Width != 8 || got.Image.Height != 4 {
		t.Fatalf("画像サイズ 8x4 を期待したが %+v", got.Image)
	}

	want := []string{"イラスト", "webp"}
	if err := h.WriteTags(path, want); err != nil {
		t.Fatalf("タグ書き込みに失敗: %v", err)
	}

	// 単純フォーマットから拡張フォーマットへ変換され、VP8X が先頭に来ていること。
	f, err := readWebP(path)
	if err != nil {
		t.Fatalf("書き込み後の WebP 解析に失敗: %v", err)
	}
	if len(f.Chunks) == 0 || f.Chunks[0].ID != chunkVP8X {
		t.Fatalf("先頭チャンクが VP8X でない: %+v", chunkIDs(f))
	}
	if vp8x, _ := f.Find(chunkVP8X); vp8x.Data[0]&vp8xFlagXMP == 0 {
		t.Fatalf("VP8X の XMP フラグが立っていない: %08b", vp8x.Data[0])
	}

	got, err = h.Read(path)
	if err != nil {
		t.Fatalf("書き込み後の読み取りに失敗: %v", err)
	}
	if !reflect.DeepEqual(got.Tags, want) {
		t.Fatalf("書き戻したタグが一致しない: got %v, want %v", got.Tags, want)
	}

	// 書き換え後も WebP としてデコードできること。
	if w, hgt := imageDimensions(path); w != 8 || hgt != 4 {
		t.Fatalf("書き換え後の WebP が壊れている: %dx%d", w, hgt)
	}

	// タグを空にすると XMP チャンクごと消え、フラグも下りること。
	if err := h.WriteTags(path, nil); err != nil {
		t.Fatalf("タグ削除に失敗: %v", err)
	}
	f, err = readWebP(path)
	if err != nil {
		t.Fatalf("削除後の WebP 解析に失敗: %v", err)
	}
	if _, ok := f.Find(chunkXMP); ok {
		t.Fatalf("XMP チャンクが残っている: %+v", chunkIDs(f))
	}
	if vp8x, _ := f.Find(chunkVP8X); vp8x.Data[0]&vp8xFlagXMP != 0 {
		t.Fatalf("VP8X の XMP フラグが下りていない: %08b", vp8x.Data[0])
	}
}

func chunkIDs(f *riffFile) []string {
	out := make([]string, 0, len(f.Chunks))
	for _, c := range f.Chunks {
		out = append(out, c.ID)
	}
	return out
}
