package meta

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// buildTestOgg はタグの読み書きを試すための Ogg Vorbis を組み立てる。
//
// 本物の符号化器は要らないので、音声パケットの中身は適当なバイト列にしている。
// ページを組み直す処理を試したいので、セットアップヘッダーは 1 ページに
// 収まらない長さにし、音声も複数ページに分けている。
func buildTestOgg(t *testing.T, comments []string) []byte {
	t.Helper()

	// 識別ヘッダー: 0x01"vorbis" + バージョン + チャンネル数 + 標本化周波数 + …
	ident := []byte("\x01vorbis")
	ident = binary.LittleEndian.AppendUint32(ident, 0)     // バージョン
	ident = append(ident, 2)                               // チャンネル数
	ident = binary.LittleEndian.AppendUint32(ident, 44100) // 標本化周波数
	ident = binary.LittleEndian.AppendUint32(ident, 0)     // 最大ビットレート
	ident = binary.LittleEndian.AppendUint32(ident, 128000)
	ident = binary.LittleEndian.AppendUint32(ident, 0)
	ident = append(ident, 0xB8, 0x01) // ブロックサイズと終端ビット

	comment := &vorbisComments{vendor: "taggo test", comments: comments}
	commentPacket := append([]byte("\x03vorbis"), comment.encode(true)...)

	// セットアップヘッダーは 1 ページ（最大 65025 バイト）に収まらない長さにする。
	setup := append([]byte("\x05vorbis"), bytes.Repeat([]byte{0x5A}, 70000)...)

	const serial = 0x74616767
	pages := packOggPages([][]byte{ident}, serial, 0, true, 0)
	pages = append(pages, packOggPages([][]byte{commentPacket, setup}, serial, uint32(len(pages)), false, 0)...)

	// 音声ページ。granule position は 1 秒ぶん（44100）まで進める。
	seq := uint32(len(pages))
	for i, granule := range []int64{22050, 44100} {
		audio := packOggPages([][]byte{bytes.Repeat([]byte{byte(i + 1)}, 3000)}, serial, seq, false, granule)
		if i == 1 {
			audio[len(audio)-1].flags |= 0x04 // 最終ページ
		}
		pages = append(pages, audio...)
		seq += uint32(len(audio))
	}

	var out []byte
	for _, p := range pages {
		out = append(out, p.encode()...)
	}
	return out
}

func writeTestOgg(t *testing.T, comments []string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sample.ogg")
	if err := os.WriteFile(path, buildTestOgg(t, comments), 0o644); err != nil {
		t.Fatalf("テスト用 Ogg の作成に失敗: %v", err)
	}
	return path
}

// oggPagesOf はファイルに含まれるページを順に返す。書き換えの検証に使う。
func oggPagesOf(t *testing.T, path string) []*oggPage {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("読み込みに失敗: %v", err)
	}
	defer f.Close()

	stream := newOggStream(f)
	var pages []*oggPage
	for {
		page, err := readOggPage(stream.src)
		if err != nil {
			return pages
		}
		pages = append(pages, page)
	}
}

// TestOggCRC は Ogg の CRC が既知の値と一致することを確かめる。
// 多項式は CRC-32 と同じだがビット反転を行わないため、
// 取り違えるとページがすべて壊れる。
func TestOggCRC(t *testing.T) {
	if got := oggCRC([]byte("123456789")); got != 0x89A1897F {
		t.Fatalf("CRC が既知の値と一致しない: %08X", got)
	}
}

func TestOggTagRoundTrip(t *testing.T) {
	h := oggHandler{}
	path := writeTestOgg(t, nil)

	got, err := h.Read(path)
	if err != nil {
		t.Fatalf("タグ無しファイルの読み取りに失敗: %v", err)
	}
	if len(got.Tags) != 0 {
		t.Fatalf("タグ 0 件を期待したが %v", got.Tags)
	}
	// 1 秒ぶんの granule position を入れてある。
	if d := got.Audio.DurationSec; d < 0.9 || d > 1.1 {
		t.Fatalf("再生時間が想定外: %v 秒", d)
	}

	want := []string{"BGM", "作業用"}
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
	if d := got.Audio.DurationSec; d < 0.9 || d > 1.1 {
		t.Fatalf("書き換えで再生時間が変わった: %v 秒", d)
	}

	if err := h.WriteTags(path, nil); err != nil {
		t.Fatalf("タグ削除に失敗: %v", err)
	}
	if got, err = h.Read(path); err != nil || len(got.Tags) != 0 {
		t.Fatalf("タグが消えていない: %v (%v)", got.Tags, err)
	}
}

// TestOggKeepsOtherComments は、曲名などのコメントを巻き込まずに
// キーワードだけを入れ替えることを確かめる。
func TestOggKeepsOtherComments(t *testing.T) {
	h := oggHandler{}
	path := writeTestOgg(t, []string{"TITLE=夕暮れ", "ARTIST=だれか", "KEYWORDS=古いタグ"})

	if err := h.WriteTags(path, []string{"新しいタグ"}); err != nil {
		t.Fatalf("タグ書き込みに失敗: %v", err)
	}
	got, err := h.Read(path)
	if err != nil {
		t.Fatalf("読み取りに失敗: %v", err)
	}
	if got.Audio.Title != "夕暮れ" || got.Audio.Artist != "だれか" {
		t.Fatalf("曲名・アーティストが失われた: %+v", got.Audio)
	}
	if !reflect.DeepEqual(got.Tags, []string{"新しいタグ"}) {
		t.Fatalf("古いタグが残っている: %v", got.Tags)
	}
}

// TestOggRepagesWithoutTouchingAudio は、書き換えてもページの体裁が保たれ、
// 音声のページが 1 バイトも変わらないことを確かめる。
// ページ番号が飛んだり CRC が合わなかったりすると、再生側は壊れたファイルとして扱う。
func TestOggRepagesWithoutTouchingAudio(t *testing.T) {
	h := oggHandler{}
	path := writeTestOgg(t, nil)

	audioBefore := audioPageBodies(t, path)
	if len(audioBefore) == 0 {
		t.Fatal("テストデータに音声ページが無い")
	}

	// ヘッダーの長さが変わるよう、長いタグを書き込む。
	if err := h.WriteTags(path, []string{"とても長いタグ名をわざと使ってページの長さを変える"}); err != nil {
		t.Fatalf("タグ書き込みに失敗: %v", err)
	}

	pages := oggPagesOf(t, path)
	for i, page := range pages {
		if page.seq != uint32(i) {
			t.Fatalf("%d 枚目のページ番号が飛んでいる: %d", i, page.seq)
		}
		// encode は CRC を計算し直すので、元のバイト列と一致すれば CRC も正しい。
		raw := page.encode()
		if got := binary.LittleEndian.Uint32(raw[22:]); got == 0 {
			t.Fatalf("%d 枚目の CRC が空", i)
		}
	}
	if !reflect.DeepEqual(audioPageBodies(t, path), audioBefore) {
		t.Fatal("音声ページの中身が書き換わっている")
	}
}

// audioPageBodies は granule position を持つページ（＝音声側）の中身を返す。
func audioPageBodies(t *testing.T, path string) [][]byte {
	t.Helper()
	var out [][]byte
	for _, page := range oggPagesOf(t, path) {
		if page.granule > 0 {
			out = append(out, page.body)
		}
	}
	return out
}

// TestOggRejectsOtherCodecs は、Vorbis でも Opus でもない Ogg を
// 黙って書き換えないことを確かめる。
func TestOggRejectsOtherCodecs(t *testing.T) {
	// Ogg FLAC の先頭パケット。
	head := append([]byte("\x7FFLAC"), bytes.Repeat([]byte{0}, 40)...)
	pages := packOggPages([][]byte{head}, 1, 0, true, 0)

	path := filepath.Join(t.TempDir(), "flac-in.ogg")
	if err := os.WriteFile(path, pages[0].encode(), 0o644); err != nil {
		t.Fatalf("テストデータの作成に失敗: %v", err)
	}

	h := oggHandler{}
	if _, err := h.Read(path); err == nil {
		t.Fatal("Vorbis / Opus 以外なのに読み取れてしまった")
	}
	before, _ := os.ReadFile(path)
	if err := h.WriteTags(path, []string{"x"}); err == nil {
		t.Fatal("Vorbis / Opus 以外なのに書き込めてしまった")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("書き込みを断ったのにファイルが変わっている")
	}
}

// TestOpusTagRoundTrip は Opus 入りの Ogg でもタグを読み書きできることを確かめる。
// Opus はコメントの終端ビットが無く、ヘッダーも 2 つしかない。
func TestOpusTagRoundTrip(t *testing.T) {
	ident := []byte("OpusHead")
	ident = append(ident, 1, 2)                            // バージョン・チャンネル数
	ident = binary.LittleEndian.AppendUint16(ident, 312)   // プリスキップ
	ident = binary.LittleEndian.AppendUint32(ident, 48000) // 入力の標本化周波数
	ident = append(ident, 0, 0, 0)                         // 出力ゲインとマッピング

	comment := &vorbisComments{vendor: "taggo test"}
	tags := append([]byte("OpusTags"), comment.encode(false)...)

	const serial = 0x6F707573
	pages := packOggPages([][]byte{ident}, serial, 0, true, 0)
	pages = append(pages, packOggPages([][]byte{tags}, serial, 1, false, 0)...)
	pages = append(pages, packOggPages([][]byte{bytes.Repeat([]byte{7}, 500)}, serial, uint32(len(pages)), false, 48000+312)...)

	var raw []byte
	for _, p := range pages {
		raw = append(raw, p.encode()...)
	}
	path := filepath.Join(t.TempDir(), "sample.ogg")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("テストデータの作成に失敗: %v", err)
	}

	h := oggHandler{}
	if err := h.WriteTags(path, []string{"opus", "声"}); err != nil {
		t.Fatalf("タグ書き込みに失敗: %v", err)
	}
	got, err := h.Read(path)
	if err != nil {
		t.Fatalf("読み取りに失敗: %v", err)
	}
	if !reflect.DeepEqual(got.Tags, []string{"opus", "声"}) {
		t.Fatalf("書き戻したタグが一致しない: %v", got.Tags)
	}
	// プリスキップを引いた 48000 サンプル = 1 秒。
	if d := got.Audio.DurationSec; d < 0.9 || d > 1.1 {
		t.Fatalf("再生時間が想定外: %v 秒", d)
	}
}

// TestPackOggPagesContinuationFlag は、ページ 255 セグメントの境目が
// ちょうどパケットの切れ目に来たとき、次のページへ誤って「続き」の印を
// 付けないことを確かめる。付けてしまうと再生側は先頭のパケットを捨てる。
func TestPackOggPagesContinuationFlag(t *testing.T) {
	// 254 個の 255 バイトと端数 100 バイトで、ちょうど 255 セグメント。
	exact := bytes.Repeat([]byte{1}, 254*255+100)
	pages := packOggPages([][]byte{exact, {2, 2, 2}}, 1, 0, true, 0)

	if len(pages) != 2 {
		t.Fatalf("ページ数が想定外: %d", len(pages))
	}
	if len(pages[0].lacing) != 255 {
		t.Fatalf("1 ページ目が 255 セグメントになっていない: %d", len(pages[0].lacing))
	}
	if pages[1].flags&oggFlagContinued != 0 {
		t.Fatal("パケットの切れ目なのに続きの印が付いている")
	}

	// 逆に、パケットの途中でページが切れたときは印が付くこと。
	split := bytes.Repeat([]byte{3}, 255*255+10)
	pages = packOggPages([][]byte{split}, 1, 0, true, 0)
	if len(pages) != 2 || pages[1].flags&oggFlagContinued == 0 {
		t.Fatalf("続きの印が付いていない: %d ページ / flags=%02X", len(pages), pages[len(pages)-1].flags)
	}
}
