package meta

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"strings"

	"github.com/yuuxyu/taggo/internal/model"
)

func init() { Register(wavHandler{}) }

// wavHandler はタグを RIFF の LIST/INFO チャンク内 "IKEY"（Keywords）サブチャンクに格納する。
// IKEY は RIFF INFO の規格上まさにキーワード用のフィールドであり、
// 独自チャンクを増やさずに済む。読み取り時には "id3 " チャンクも見る。
type wavHandler struct{}

func (wavHandler) Kind() model.Kind     { return model.KindAudio }
func (wavHandler) Extensions() []string { return []string{".wav"} }

const (
	wavFormType   = "WAVE"
	chunkLIST     = "LIST"
	chunkFmt      = "fmt "
	listTypeINFO  = "INFO"
	infoKeywords  = "IKEY"
	infoName      = "INAM"
	infoArtist    = "IART"
	infoProduct   = "IPRD"
	infoGenre     = "IGNR"
	infoCreatedAt = "ICRD"
)

func (wavHandler) Read(path string) (Info, error) {
	f, err := readWAV(path)
	if err != nil {
		return Info{}, err
	}

	fields := wavInfoFields(f)
	audio := &model.AudioMeta{
		Title:       fields[infoName],
		Artist:      fields[infoArtist],
		Album:       fields[infoProduct],
		Genre:       fields[infoGenre],
		Year:        fields[infoCreatedAt],
		DurationSec: wavDuration(f),
	}

	tags := splitKeywordString(fields[infoKeywords])
	if c, ok := f.Find("id3 "); ok {
		tags = append(tags, id3ChunkKeywords(c.Data)...)
	}
	if c, ok := f.Find("ID3 "); ok {
		tags = append(tags, id3ChunkKeywords(c.Data)...)
	}

	return Info{Tags: tags, Title: audio.Title, Audio: audio}, nil
}

func (wavHandler) WriteTags(path string, tags []string) error {
	f, err := readWAV(path)
	if err != nil {
		return err
	}

	fields := wavInfoFields(f)
	if len(tags) == 0 {
		delete(fields, infoKeywords)
	} else {
		fields[infoKeywords] = strings.Join(tags, xpKeywordSep)
	}

	setWAVInfoList(f, fields)
	return replaceFileBytes(path, f.Bytes())
}

func readWAV(path string) (*riffFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f, err := parseRIFF(data)
	if err != nil {
		return nil, riffError("WAV", err)
	}
	if f.FormType != wavFormType {
		return nil, fmt.Errorf("WAV ではありません: フォームタイプ %q", f.FormType)
	}
	return f, nil
}

// wavInfoFields は LIST/INFO チャンクを、サブチャンク ID をキーとするマップへ展開する。
func wavInfoFields(f *riffFile) map[string]string {
	out := map[string]string{}
	for _, c := range f.Chunks {
		if c.ID != chunkLIST || len(c.Data) < 4 || string(c.Data[0:4]) != listTypeINFO {
			continue
		}
		for _, sub := range parseSubChunks(c.Data[4:]) {
			out[sub.ID] = strings.TrimRight(string(sub.Data), "\x00 ")
		}
	}
	return out
}

// setWAVInfoList は LIST/INFO チャンクを fields の内容で作り直す。
// WAV の INFO チャンクは fmt チャンクより前に置かない慣習なので、末尾に配置する。
func setWAVInfoList(f *riffFile, fields map[string]string) {
	// 既存の LIST/INFO をいったん全部外す。LIST でも INFO 以外（adtl など）は残す。
	kept := make([]riffChunk, 0, len(f.Chunks))
	for _, c := range f.Chunks {
		if c.ID == chunkLIST && len(c.Data) >= 4 && string(c.Data[0:4]) == listTypeINFO {
			continue
		}
		kept = append(kept, c)
	}
	f.Chunks = kept

	if len(fields) == 0 {
		return
	}

	// 出力順を安定させるため、サブチャンク ID の辞書順で書き出す。
	keys := make([]string, 0, len(fields))
	for k := range fields {
		if fields[k] != "" {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return
	}
	sortASCII(keys)

	var buf bytes.Buffer
	buf.WriteString(listTypeINFO)
	for _, k := range keys {
		// RIFF INFO の値は NUL 終端の文字列として書く。
		value := append([]byte(fields[k]), 0)
		buf.Write(fourCC(k))
		_ = binary.Write(&buf, binary.LittleEndian, uint32(len(value)))
		buf.Write(value)
		if len(value)%2 == 1 {
			buf.WriteByte(0) // サブチャンクにもパディングが必要
		}
	}
	f.Chunks = append(f.Chunks, riffChunk{ID: chunkLIST, Data: buf.Bytes()})
}

// parseSubChunks は LIST チャンクの中身を、通常のチャンク列として読む。
func parseSubChunks(data []byte) []riffChunk {
	var out []riffChunk
	for off := 0; off+8 <= len(data); {
		id := string(data[off : off+4])
		size := int(binary.LittleEndian.Uint32(data[off+4 : off+8]))
		off += 8
		if size < 0 || off+size > len(data) {
			break
		}
		out = append(out, riffChunk{ID: id, Data: data[off : off+size]})
		off += size
		if size%2 == 1 {
			off++
		}
	}
	return out
}

// wavDuration は fmt チャンクと data チャンクのサイズから再生時間を概算する。
// 非圧縮 PCM を前提にした概算で、可変ビットレート形式では正確にならない。
func wavDuration(f *riffFile) float64 {
	fmtChunk, ok := f.Find(chunkFmt)
	if !ok || len(fmtChunk.Data) < 16 {
		return 0
	}
	byteRate := binary.LittleEndian.Uint32(fmtChunk.Data[8:12])
	if byteRate == 0 {
		return 0
	}
	data, ok := f.Find("data")
	if !ok {
		return 0
	}
	return float64(len(data.Data)) / float64(byteRate)
}

// id3ChunkKeywords は WAV に埋め込まれた ID3v2 ブロックから TXXX のキーワードを拾う。
// 解析できない場合は黙って空を返す。ID3 は WAV では補助的な置き場所に過ぎないため。
func id3ChunkKeywords(data []byte) []string {
	tag, err := parseID3v2FromBytes(data)
	if err != nil {
		return nil
	}
	return tag
}

func sortASCII(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
