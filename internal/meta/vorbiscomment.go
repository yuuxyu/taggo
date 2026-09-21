package meta

import (
	"encoding/binary"
	"errors"
	"strings"
)

// Vorbis Comment のパケット本体（ベンダー文字列＋"NAME=値" の並び）の読み書き。
//
// FLAC ではメタデータブロックとして入るため go-flac 側に任せているが、
// Ogg では同じ中身がヘッダーパケットとして入る。こちらは自前で組み立てる。

// vorbisComments は Vorbis Comment の中身。
type vorbisComments struct {
	vendor   string
	comments []string // "NAME=値" の形のまま保持する
}

// parseVorbisComments はコメント領域を読み取る。終端ビットは読み飛ばす。
func parseVorbisComments(b []byte) (*vorbisComments, error) {
	read := func(n int) ([]byte, error) {
		if len(b) < n {
			return nil, errors.New("Vorbis Comment が途中で切れています")
		}
		out := b[:n]
		b = b[n:]
		return out, nil
	}
	readString := func() (string, error) {
		head, err := read(4)
		if err != nil {
			return "", err
		}
		size := binary.LittleEndian.Uint32(head)
		// 壊れた長さで巨大な確保をしないよう、残りの長さで頭打ちにする。
		if int64(size) > int64(len(b)) {
			return "", errors.New("Vorbis Comment の長さが壊れています")
		}
		body, err := read(int(size))
		if err != nil {
			return "", err
		}
		return string(body), nil
	}

	vendor, err := readString()
	if err != nil {
		return nil, err
	}
	head, err := read(4)
	if err != nil {
		return nil, err
	}
	count := binary.LittleEndian.Uint32(head)
	if int64(count) > int64(len(b)) {
		return nil, errors.New("Vorbis Comment の件数が壊れています")
	}

	out := &vorbisComments{vendor: vendor}
	for range count {
		c, err := readString()
		if err != nil {
			return nil, err
		}
		out.comments = append(out.comments, c)
	}
	return out, nil
}

// encode はコメント領域を組み立てる。framing が真なら終端ビットを足す
// （Vorbis のヘッダーパケットに必要。Opus には無い）。
func (v *vorbisComments) encode(framing bool) []byte {
	var out []byte
	appendString := func(s string) {
		out = binary.LittleEndian.AppendUint32(out, uint32(len(s)))
		out = append(out, s...)
	}

	appendString(v.vendor)
	out = binary.LittleEndian.AppendUint32(out, uint32(len(v.comments)))
	for _, c := range v.comments {
		appendString(c)
	}
	if framing {
		out = append(out, 1)
	}
	return out
}

// get は名前に対応する値を順に返す。名前の大文字小文字は区別しない。
func (v *vorbisComments) get(name string) []string {
	var out []string
	for _, c := range v.comments {
		key, value, ok := strings.Cut(c, "=")
		if ok && strings.EqualFold(strings.TrimSpace(key), name) {
			out = append(out, value)
		}
	}
	return out
}

// first は名前に対応する最初の値を返す。
func (v *vorbisComments) first(name string) string {
	if values := v.get(name); len(values) > 0 {
		return strings.TrimSpace(values[0])
	}
	return ""
}

// setKeywords はキーワード系のフィールドを tags で置き換える。
// それ以外のコメント（曲名やアーティストなど）はそのまま残す。
func (v *vorbisComments) setKeywords(tags []string) {
	kept := v.comments[:0]
	for _, c := range v.comments {
		name, _, ok := strings.Cut(c, "=")
		if ok && isVorbisKeywordAlias(name) {
			continue
		}
		kept = append(kept, c)
	}
	v.comments = kept

	for _, t := range tags {
		v.comments = append(v.comments, vorbisKeywordField+"="+t)
	}
}

// keywords はキーワード系フィールドから取り出したタグを返す。
func (v *vorbisComments) keywords() []string {
	var tags []string
	for _, alias := range vorbisKeywordAliases {
		for _, value := range v.get(alias) {
			tags = append(tags, splitKeywordString(value)...)
		}
	}
	return tags
}
