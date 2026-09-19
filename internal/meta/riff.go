package meta

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// RIFF 形式は WebP と WAV の両方が土台にしているため、チャンクの解析と
// 組み立てをここに共通化している。
//
// ファイル全体の構造は次のとおり。
//
//	"RIFF" <以降のサイズ:4> <フォームタイプ:4> <チャンク>...
//
// 各チャンクは次のとおりで、ペイロードが奇数長のときは 1 バイトのパディングが入る
// （パディングはサイズに含めない）。
//
//	<FourCC:4> <ペイロードサイズ:4> <ペイロード> [パディング:1]

// errNotRIFF は先頭が RIFF コンテナでないときに返す。
var errNotRIFF = errors.New("RIFF コンテナではありません")

// riffChunk は RIFF コンテナ内の 1 チャンク。
type riffChunk struct {
	ID   string
	Data []byte
}

// riffFile は解析済みの RIFF コンテナ。
type riffFile struct {
	FormType string // WebP なら "WEBP"、WAV なら "WAVE"
	Chunks   []riffChunk
}

// parseRIFF は RIFF コンテナを解析する。
// 宣言サイズが実データより大きい壊れたファイルもあるため、
// 読める範囲までを解析結果として返す。
func parseRIFF(data []byte) (*riffFile, error) {
	if len(data) < 12 || string(data[0:4]) != "RIFF" {
		return nil, errNotRIFF
	}

	declared := int(binary.LittleEndian.Uint32(data[4:8])) + 8
	end := min(declared, len(data))

	f := &riffFile{FormType: string(data[8:12])}
	for off := 12; off+8 <= end; {
		id := string(data[off : off+4])
		size := int(binary.LittleEndian.Uint32(data[off+4 : off+8]))
		off += 8
		if size < 0 || off+size > end {
			// 途中で切れているチャンクは、読める分だけ残して打ち切る。
			size = end - off
			if size <= 0 {
				break
			}
			f.Chunks = append(f.Chunks, riffChunk{ID: id, Data: data[off : off+size]})
			break
		}
		f.Chunks = append(f.Chunks, riffChunk{ID: id, Data: data[off : off+size]})
		off += size
		if size%2 == 1 {
			off++ // 奇数長チャンクのパディング
		}
	}
	return f, nil
}

// Bytes は RIFF コンテナをバイト列へ組み直す。サイズとパディングは計算し直す。
func (f *riffFile) Bytes() []byte {
	total := 4 // フォームタイプ
	for _, c := range f.Chunks {
		total += 8 + len(c.Data)
		if len(c.Data)%2 == 1 {
			total++
		}
	}

	out := make([]byte, 0, 8+total)
	out = append(out, "RIFF"...)
	out = binary.LittleEndian.AppendUint32(out, uint32(total))
	out = append(out, f.FormType...)
	for _, c := range f.Chunks {
		out = append(out, fourCC(c.ID)...)
		out = binary.LittleEndian.AppendUint32(out, uint32(len(c.Data)))
		out = append(out, c.Data...)
		if len(c.Data)%2 == 1 {
			out = append(out, 0)
		}
	}
	return out
}

// Find は指定 ID の最初のチャンクを返す。
func (f *riffFile) Find(id string) (*riffChunk, bool) {
	for i := range f.Chunks {
		if f.Chunks[i].ID == id {
			return &f.Chunks[i], true
		}
	}
	return nil, false
}

// Remove は指定 ID のチャンクをすべて取り除く。
func (f *riffFile) Remove(id string) {
	kept := f.Chunks[:0]
	for _, c := range f.Chunks {
		if c.ID != id {
			kept = append(kept, c)
		}
	}
	f.Chunks = kept
}

// Set は指定 ID のチャンクの内容を差し替える。無ければ末尾に追加する。
func (f *riffFile) Set(id string, data []byte) {
	if c, ok := f.Find(id); ok {
		c.Data = data
		return
	}
	f.Chunks = append(f.Chunks, riffChunk{ID: id, Data: data})
}

// fourCC は ID をちょうど 4 バイトに整える。短ければ空白で埋め、長ければ切り詰める。
func fourCC(id string) []byte {
	b := []byte("    ")
	copy(b, id)
	return b[:4]
}

// riffError はチャンク処理中のエラーにフォーマット名を添える。
func riffError(format string, err error) error {
	return fmt.Errorf("%s の RIFF チャンク処理に失敗しました: %w", format, err)
}
