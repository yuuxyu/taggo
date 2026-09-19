package meta

import (
	"os"
	"reflect"
	"testing"
)

// audioRoundTrip は、音声ハンドラー共通のラウンドトリップ検証を行う。
// タグを書いて読み戻し、さらに空タグで消えることと、
// 音声データ本体のサイズが保たれていることを確かめる。
func audioRoundTrip(t *testing.T, h Handler, fixture string, want []string) {
	t.Helper()
	path := copyFixture(t, fixture)

	before, err := os.Stat(path)
	if err != nil {
		t.Fatalf("テストデータの stat に失敗: %v", err)
	}

	got, err := h.Read(path)
	if err != nil {
		t.Fatalf("タグ無しファイルの読み取りに失敗: %v", err)
	}
	if len(got.Tags) != 0 {
		t.Fatalf("タグ 0 件を期待したが %v", got.Tags)
	}
	if got.Audio == nil {
		t.Fatal("AudioMeta が nil")
	}

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

	// タグを消したあとのファイルサイズが元とかけ離れていたら、
	// 音声データ本体を壊した疑いがある。
	after, err := os.Stat(path)
	if err != nil {
		t.Fatalf("書き換え後の stat に失敗: %v", err)
	}
	if diff := after.Size() - before.Size(); diff > 4096 || diff < -4096 {
		t.Fatalf("ファイルサイズが大きく変化した: %d -> %d", before.Size(), after.Size())
	}
}

func TestMP3TagRoundTrip(t *testing.T) {
	audioRoundTrip(t, mp3Handler{}, "sample.mp3", []string{"BGM", "作業用"})
}

func TestFLACTagRoundTrip(t *testing.T) {
	audioRoundTrip(t, flacHandler{}, "sample.flac", []string{"hi-res", "録音"})
}

func TestWAVTagRoundTrip(t *testing.T) {
	audioRoundTrip(t, wavHandler{}, "sample.wav", []string{"効果音", "click"})
}

func TestWAVDurationAndPlayback(t *testing.T) {
	path := copyFixture(t, "sample.wav")
	h := wavHandler{}

	got, err := h.Read(path)
	if err != nil {
		t.Fatalf("WAV の読み取りに失敗: %v", err)
	}
	// テストデータは 1 秒のサイン波。
	if d := got.Audio.DurationSec; d < 0.9 || d > 1.1 {
		t.Fatalf("再生時間が想定外: %v 秒", d)
	}

	if err := h.WriteTags(path, []string{"test"}); err != nil {
		t.Fatalf("タグ書き込みに失敗: %v", err)
	}
	// 書き換え後も fmt / data チャンクが保たれ、再生時間が変わらないこと。
	got, err = h.Read(path)
	if err != nil {
		t.Fatalf("書き込み後の読み取りに失敗: %v", err)
	}
	if d := got.Audio.DurationSec; d < 0.9 || d > 1.1 {
		t.Fatalf("書き換えで再生時間が変化した: %v 秒", d)
	}
}

func TestReadOnlyFormatsRejectWrites(t *testing.T) {
	for _, h := range []Handler{gifHandler{}, svgHandler{}, aacHandler{}} {
		if err := h.WriteTags("dummy", []string{"x"}); err != ErrFormatReadOnly {
			t.Fatalf("%T は ErrFormatReadOnly を返すべきだが %v", h, err)
		}
	}
}

func TestMP3Duration(t *testing.T) {
	h := mp3Handler{}
	path := copyFixture(t, "sample.mp3")

	got, err := h.Read(path)
	if err != nil {
		t.Fatalf("MP3 の読み取りに失敗: %v", err)
	}
	// テストデータは 1 秒のサイン波。
	if d := got.Audio.DurationSec; d < 0.9 || d > 1.2 {
		t.Fatalf("再生時間が想定外: %v 秒", d)
	}

	// タグを書いてビット数が変わっても、再生時間は変わらないこと。
	if err := h.WriteTags(path, []string{"タグ", "追加"}); err != nil {
		t.Fatalf("タグ書き込みに失敗: %v", err)
	}
	got, err = h.Read(path)
	if err != nil {
		t.Fatalf("書き込み後の読み取りに失敗: %v", err)
	}
	if d := got.Audio.DurationSec; d < 0.9 || d > 1.2 {
		t.Fatalf("タグ書き込みで再生時間が変化した: %v 秒", d)
	}
}
