package meta

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"

	"github.com/yuuxyu/taggo/internal/model"
)

func init() { Register(oggHandler{}) }

// oggHandler は Ogg コンテナの中の Vorbis / Opus を扱う。
// タグは FLAC と同じ Vorbis Comment の KEYWORDS フィールドに入れるため、
// 他のタガーで付けたキーワードもそのまま読める。
//
// .ogg は入れ物の名前でしかなく、中身は Vorbis のことも Opus のこともある。
// どちらもコメントの形式は同じで、包み方だけが違う。
type oggHandler struct{}

func (oggHandler) Kind() model.Kind     { return model.KindAudio }
func (oggHandler) Extensions() []string { return []string{".ogg"} }

// oggCodec は Ogg に入っている符号化方式ごとの違いをまとめたもの。
type oggCodec struct {
	name string
	// headers はタグより前に置かれるヘッダーパケットの数。
	headers int
	// commentPrefix はコメントパケットの先頭に付く識別子。
	commentPrefix string
	// framing はコメントの末尾に終端ビットが要るかどうか（Vorbis だけ）。
	framing bool
	// granuleRate は granule position の単位（1 秒あたり）。
	granuleRate int
	// preSkip は再生時に読み飛ばす分（Opus だけ）。
	preSkip int64
}

// identifyOggCodec は先頭のヘッダーパケットから符号化方式を見分ける。
func identifyOggCodec(ident []byte) (oggCodec, error) {
	switch {
	case bytes.HasPrefix(ident, []byte("\x01vorbis")):
		// 0x01"vorbis" + バージョン(4) + チャンネル数(1) + 標本化周波数(4)
		if len(ident) < 16 {
			return oggCodec{}, fmt.Errorf("Vorbis のヘッダーが短すぎます")
		}
		return oggCodec{
			name:          "Vorbis",
			headers:       3, // 識別・コメント・セットアップ
			commentPrefix: "\x03vorbis",
			framing:       true,
			granuleRate:   int(binary.LittleEndian.Uint32(ident[12:])),
		}, nil

	case bytes.HasPrefix(ident, []byte("OpusHead")):
		// "OpusHead" + バージョン(1) + チャンネル数(1) + プリスキップ(2)
		if len(ident) < 12 {
			return oggCodec{}, fmt.Errorf("Opus のヘッダーが短すぎます")
		}
		return oggCodec{
			name:          "Opus",
			headers:       2, // 識別・コメント
			commentPrefix: "OpusTags",
			// Opus の granule position は常に 48kHz 換算。
			granuleRate: 48000,
			preSkip:     int64(binary.LittleEndian.Uint16(ident[10:])),
		}, nil
	}
	return oggCodec{}, fmt.Errorf("Ogg の中身が Vorbis / Opus ではないため、タグを扱えません")
}

// readOggHeaders はヘッダーパケットを読み、コメントを取り出す。
func readOggHeaders(r io.Reader) (*oggStream, oggCodec, [][]byte, *vorbisComments, error) {
	stream := newOggStream(r)

	ident, err := stream.nextPacket()
	if err != nil {
		return nil, oggCodec{}, nil, nil, fmt.Errorf("Ogg の解析に失敗しました: %w", err)
	}
	codec, err := identifyOggCodec(ident)
	if err != nil {
		return nil, oggCodec{}, nil, nil, err
	}

	packets := [][]byte{ident}
	for len(packets) < codec.headers {
		pkt, err := stream.nextPacket()
		if err != nil {
			return nil, oggCodec{}, nil, nil, fmt.Errorf("%s のヘッダーが揃っていません: %w", codec.name, err)
		}
		packets = append(packets, pkt)
	}

	comment := packets[1]
	if !bytes.HasPrefix(comment, []byte(codec.commentPrefix)) {
		return nil, oggCodec{}, nil, nil, fmt.Errorf("%s のコメントヘッダーが見つかりません", codec.name)
	}
	body := comment[len(codec.commentPrefix):]
	if codec.framing && len(body) > 0 {
		body = body[:len(body)-1] // 終端ビットは中身ではない
	}
	comments, err := parseVorbisComments(body)
	if err != nil {
		return nil, oggCodec{}, nil, nil, err
	}
	return stream, codec, packets, comments, nil
}

func (oggHandler) Read(path string) (Info, error) {
	f, err := os.Open(path)
	if err != nil {
		return Info{}, err
	}
	defer f.Close()

	stream, codec, _, comments, err := readOggHeaders(f)
	if err != nil {
		return Info{}, err
	}

	audio := &model.AudioMeta{
		Title:  comments.first("TITLE"),
		Artist: comments.first("ARTIST"),
		Album:  comments.first("ALBUM"),
		Genre:  comments.first("GENRE"),
		Year:   comments.first("DATE"),
	}
	// 画像は METADATA_BLOCK_PICTURE（FLAC と同じ構造を base64 で入れたもの）に置かれる。
	audio.HasCoverArt = len(comments.get("METADATA_BLOCK_PICTURE")) > 0 ||
		len(comments.get("COVERART")) > 0

	if granule, ok := lastOggGranule(f, stream.serial); ok && codec.granuleRate > 0 {
		if samples := granule - codec.preSkip; samples > 0 {
			audio.DurationSec = float64(samples) / float64(codec.granuleRate)
		}
	}

	return Info{Tags: comments.keywords(), Title: audio.Title, Audio: audio}, nil
}

func (oggHandler) WriteTags(path string, tags []string) error {
	src, err := os.Open(path)
	if err != nil {
		return err
	}
	defer src.Close()

	stream, codec, packets, comments, err := readOggHeaders(src)
	if err != nil {
		return err
	}
	if stream.multiplexed {
		return fmt.Errorf("%w: 複数のストリームを含む Ogg です", ErrFormatReadOnly)
	}
	if !stream.headerPagesDone() {
		// ヘッダーと音声が同じページに同居している異例の作り。
		// ページを組み直すと音声側を壊しかねないので触らない。
		return fmt.Errorf("%w: ヘッダーの区切りが通常と異なる Ogg です", ErrFormatReadOnly)
	}

	comments.setKeywords(tags)
	packets[1] = append([]byte(codec.commentPrefix), comments.encode(codec.framing)...)

	// 識別ヘッダーは 1 ページ目に単独で置く決まりなので、そこだけ分けて詰める。
	pages := packOggPages(packets[:1], stream.serial, 0, true, 0)
	pages = append(pages, packOggPages(packets[1:], stream.serial, uint32(len(pages)), false, 0)...)

	return replaceFile(path, func(w io.Writer) error {
		for _, page := range pages {
			if _, err := w.Write(page.encode()); err != nil {
				return err
			}
		}
		// 音声のページは中身をそのまま引き継ぎ、ページ番号だけ振り直す。
		// ヘッダーの長さが変わるとページ数も変わるためで、番号が飛ぶと
		// 再生側は音が抜けたものとして扱ってしまう。
		return copyOggPages(w, stream, src, uint32(len(pages)))
	})
}

// copyOggPages は残りのページを、ページ番号だけ振り直して書き写す。
// 読み終えたら元ファイルを閉じる。Windows では開いたままのファイルを
// 置き換えられず、replaceFile の rename が失敗するため。
func copyOggPages(w io.Writer, stream *oggStream, src *os.File, seq uint32) error {
	for {
		page, err := readOggPage(stream.src)
		if err == io.EOF {
			return src.Close()
		}
		if err != nil {
			return err
		}
		page.seq = seq
		seq++
		if _, err := w.Write(page.encode()); err != nil {
			return err
		}
	}
}

// oggTailBytes は末尾のページを探すために読む長さ。
// ページ 1 枚は最大でも 27 + 255 + 255*255 バイトなので、これで必ず収まる。
const oggTailBytes = 128 << 10

// lastOggGranule は指定した論理ストリームの最後の granule position を返す。
// 再生時間はこの値から求める。先頭から全ページを読むと大きなファイルで
// 無駄が大きいため、末尾だけを見てページの切れ目を探す。
func lastOggGranule(f *os.File, serial uint32) (int64, bool) {
	info, err := f.Stat()
	if err != nil {
		return 0, false
	}
	size := info.Size()
	offset := max(size-oggTailBytes, 0)

	tail := make([]byte, size-offset)
	if _, err := f.ReadAt(tail, offset); err != nil && err != io.EOF {
		return 0, false
	}

	// 末尾側から capture pattern を探し、CRC が合ったものだけを信じる。
	// 音声データの中にも "OggS" と並ぶ箇所はありうるため。
	for i := len(tail) - oggHeaderBytes; i >= 0; i-- {
		if string(tail[i:i+4]) != oggCapture || tail[i+4] != 0 {
			continue
		}
		if binary.LittleEndian.Uint32(tail[i+14:]) != serial {
			continue
		}
		segments := int(tail[i+26])
		if i+oggHeaderBytes+segments > len(tail) {
			continue
		}
		body := 0
		for _, l := range tail[i+oggHeaderBytes : i+oggHeaderBytes+segments] {
			body += int(l)
		}
		end := i + oggHeaderBytes + segments + body
		if end > len(tail) {
			continue
		}

		page := make([]byte, end-i)
		copy(page, tail[i:end])
		want := binary.LittleEndian.Uint32(page[22:])
		binary.LittleEndian.PutUint32(page[22:], 0)
		if oggCRC(page) != want {
			continue
		}
		return int64(binary.LittleEndian.Uint64(tail[i+6:])), true
	}
	return 0, false
}
