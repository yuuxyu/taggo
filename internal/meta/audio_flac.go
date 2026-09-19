package meta

import (
	"fmt"
	"io"
	"strings"

	flacvorbis "github.com/go-flac/flacvorbis/v2"
	flac "github.com/go-flac/go-flac/v2"
	"github.com/yuuxyu/taggo/internal/model"
)

func init() { Register(flacHandler{}) }

// flacHandler はタグを Vorbis Comment の KEYWORDS フィールドに格納する。
// Vorbis Comment の標準フィールドにキーワード欄は無いが、KEYWORDS は
// 各種タガーが事実上の標準として使っている名前である。
type flacHandler struct{}

func (flacHandler) Kind() model.Kind     { return model.KindAudio }
func (flacHandler) Extensions() []string { return []string{".flac"} }

// vorbisKeywordField は taggo がタグを書き込む Vorbis Comment のフィールド名。
const vorbisKeywordField = "KEYWORDS"

// vorbisKeywordAliases は読み取り時に受け付けるフィールド名。
var vorbisKeywordAliases = []string{"KEYWORDS", "TAGS"}

func (flacHandler) Read(path string) (Info, error) {
	f, err := flac.ParseFile(path)
	if err != nil {
		return Info{}, fmt.Errorf("FLAC の解析に失敗しました: %w", err)
	}
	defer closeFLAC(f)

	audio := &model.AudioMeta{}
	if info, err := f.GetStreamInfo(); err == nil && info.SampleRate > 0 {
		audio.DurationSec = float64(info.SampleCount) / float64(info.SampleRate)
	}

	cmt, _ := findVorbisComment(f)
	var tags []string
	if cmt != nil {
		for _, alias := range vorbisKeywordAliases {
			values, err := cmt.Get(alias)
			if err != nil {
				continue
			}
			for _, v := range values {
				tags = append(tags, splitKeywordString(v)...)
			}
		}
		audio.Title = firstVorbisValue(cmt, "TITLE")
		audio.Artist = firstVorbisValue(cmt, "ARTIST")
		audio.Album = firstVorbisValue(cmt, "ALBUM")
		audio.Genre = firstVorbisValue(cmt, "GENRE")
		audio.Year = firstVorbisValue(cmt, "DATE")
	}
	audio.HasCoverArt = hasFLACPicture(f)

	return Info{Tags: tags, Title: audio.Title, Audio: audio}, nil
}

func (flacHandler) WriteTags(path string, tags []string) error {
	f, err := flac.ParseFile(path)
	if err != nil {
		return fmt.Errorf("FLAC の解析に失敗しました: %w", err)
	}

	cmt, idx := findVorbisComment(f)
	if cmt == nil {
		cmt = flacvorbis.New()
		idx = -1
	}

	// キーワード系フィールドだけを落として、他のコメントはそのまま残す。
	kept := cmt.Comments[:0]
	for _, c := range cmt.Comments {
		name, _, ok := strings.Cut(c, "=")
		if ok && isVorbisKeywordAlias(name) {
			continue
		}
		kept = append(kept, c)
	}
	cmt.Comments = kept

	for _, t := range tags {
		if err := cmt.Add(vorbisKeywordField, t); err != nil {
			closeFLAC(f)
			return fmt.Errorf("Vorbis Comment への書き込みに失敗しました: %w", err)
		}
	}

	block := cmt.Marshal()
	if idx >= 0 {
		f.Meta[idx] = &block
	} else {
		f.Meta = append(f.Meta, &block)
	}

	// f.WriteTo は音声フレームを読み切った時点で元ファイルのハンドルを閉じるため、
	// 出力先は必ず別ファイルにする必要がある。replaceFile の一時ファイルがそれにあたる。
	return replaceFile(path, func(w io.Writer) error {
		if _, err := f.WriteTo(w); err != nil {
			return fmt.Errorf("FLAC の書き出しに失敗しました: %w", err)
		}
		return nil
	})
}

// findVorbisComment は Vorbis Comment ブロックと、その Meta 内での位置を返す。
// 見つからない場合は (nil, -1) を返す。
func findVorbisComment(f *flac.File) (*flacvorbis.MetaDataBlockVorbisComment, int) {
	for i, block := range f.Meta {
		if block.Type != flac.VorbisComment {
			continue
		}
		cmt, err := flacvorbis.ParseFromMetaDataBlock(*block)
		if err != nil {
			return nil, -1
		}
		return cmt, i
	}
	return nil, -1
}

func firstVorbisValue(cmt *flacvorbis.MetaDataBlockVorbisComment, key string) string {
	values, err := cmt.Get(key)
	if err != nil || len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}

func isVorbisKeywordAlias(name string) bool {
	for _, a := range vorbisKeywordAliases {
		if strings.EqualFold(strings.TrimSpace(name), a) {
			return true
		}
	}
	return false
}

func hasFLACPicture(f *flac.File) bool {
	for _, block := range f.Meta {
		if block.Type == flac.Picture {
			return true
		}
	}
	return false
}

// closeFLAC は音声フレーム側のハンドルを閉じる。読み取りだけで終わる経路で使う。
func closeFLAC(f *flac.File) {
	if c, ok := f.Frames.(io.Closer); ok {
		_ = c.Close()
	}
}
