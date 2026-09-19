package meta

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/bogem/id3v2/v2"
	"github.com/yuuxyu/taggo/internal/model"
)

func init() { Register(mp3Handler{}) }

// mp3Handler はタグを ID3v2 の TXXX フレーム（説明文 "KEYWORDS"）に格納する。
// TXXX:KEYWORDS は exiftool や Mp3tag がキーワード欄として扱う場所であり、
// 曲名・アーティストなど既存のフレームを一切壊さずにタグだけを出し入れできる。
// 読み取り時には、他ツールが使う別名フレームとコメント欄も拾う。
type mp3Handler struct{}

func (mp3Handler) Kind() model.Kind     { return model.KindAudio }
func (mp3Handler) Extensions() []string { return []string{".mp3"} }

// id3KeywordDesc は taggo がタグを書き込む TXXX フレームの説明文。
const id3KeywordDesc = "KEYWORDS"

// id3KeywordAliases は読み取り時に受け付ける TXXX の説明文。大文字小文字は無視して比較する。
var id3KeywordAliases = []string{"KEYWORDS", "TAGS", "TAGGO"}

func (mp3Handler) Read(path string) (Info, error) {
	tag, err := id3v2.Open(path, id3v2.Options{Parse: true})
	if err != nil {
		return Info{}, fmt.Errorf("ID3 タグの読み取りに失敗しました: %w", err)
	}
	defer tag.Close()

	var tags []string
	for _, f := range tag.GetFrames(tag.CommonID("User defined text information frame")) {
		udtf, ok := f.(id3v2.UserDefinedTextFrame)
		if !ok || !isKeywordAlias(udtf.Description) {
			continue
		}
		tags = append(tags, splitKeywordString(udtf.Value)...)
	}

	audio := &model.AudioMeta{
		Title:  strings.TrimSpace(tag.Title()),
		Artist: strings.TrimSpace(tag.Artist()),
		Album:  strings.TrimSpace(tag.Album()),
		Genre:  strings.TrimSpace(tag.Genre()),
		Year:   strings.TrimSpace(tag.Year()),
	}
	audio.HasCoverArt = len(tag.GetFrames(tag.CommonID("Attached picture"))) > 0
	// ID3 タグの直後から音声データが始まる。その位置を渡して再生時間を概算する。
	audio.DurationSec = mp3Duration(path, int64(tag.Size()))

	return Info{Tags: tags, Title: audio.Title, Audio: audio}, nil
}

func (mp3Handler) WriteTags(path string, tags []string) error {
	tag, err := id3v2.Open(path, id3v2.Options{Parse: true})
	if err != nil {
		return fmt.Errorf("ID3 タグの読み取りに失敗しました: %w", err)
	}

	// 既存の TXXX からキーワード用フレームだけを取り除き、他の TXXX は残す。
	frameID := tag.CommonID("User defined text information frame")
	kept := make([]id3v2.Framer, 0, len(tag.GetFrames(frameID)))
	for _, f := range tag.GetFrames(frameID) {
		udtf, ok := f.(id3v2.UserDefinedTextFrame)
		if ok && isKeywordAlias(udtf.Description) {
			continue
		}
		kept = append(kept, f)
	}
	tag.DeleteFrames(frameID)
	for _, f := range kept {
		tag.AddFrame(frameID, f)
	}

	if len(tags) > 0 {
		tag.AddUserDefinedTextFrame(id3v2.UserDefinedTextFrame{
			Encoding:    id3v2.EncodingUTF8,
			Description: id3KeywordDesc,
			Value:       strings.Join(tags, xpKeywordSep),
		})
	}

	// id3v2.Tag.Save() は一時ファイルへ書き出してから rename する実装なので、
	// taggo が求める「書き込み途中で元ファイルを壊さない」性質をすでに満たしている。
	// Close() はファイルハンドルを解放するだけで、保存とは独立している。
	saveErr := tag.Save()
	if cerr := tag.Close(); cerr != nil && saveErr == nil {
		saveErr = cerr
	}
	if saveErr != nil {
		return fmt.Errorf("ID3 タグの保存に失敗しました: %w", saveErr)
	}
	return nil
}

func isKeywordAlias(desc string) bool {
	for _, a := range id3KeywordAliases {
		if strings.EqualFold(strings.TrimSpace(desc), a) {
			return true
		}
	}
	return false
}

// parseID3v2FromBytes はメモリ上の ID3v2 ブロックからキーワードを取り出す。
// WAV の "id3 " チャンクのように、ID3 が別コンテナに間借りしている場合に使う。
func parseID3v2FromBytes(raw []byte) ([]string, error) {
	tag, err := id3v2.ParseReader(bytes.NewReader(raw), id3v2.Options{Parse: true})
	if err != nil {
		return nil, err
	}
	var tags []string
	for _, f := range tag.GetFrames(tag.CommonID("User defined text information frame")) {
		udtf, ok := f.(id3v2.UserDefinedTextFrame)
		if !ok || !isKeywordAlias(udtf.Description) {
			continue
		}
		tags = append(tags, splitKeywordString(udtf.Value)...)
	}
	return tags, nil
}
