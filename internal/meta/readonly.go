package meta

import (
	"os"
	"regexp"

	_ "image/gif" // 画像サイズ取得のために GIF デコーダーを登録する

	"github.com/yuuxyu/taggo/internal/model"
)

func init() {
	Register(gifHandler{})
	Register(svgHandler{})
	Register(aacHandler{})
}

// 以下は「プレビューには対応するがタグ編集は行わない」フォーマット群。
// 要件どおり、編集要求はメモリ上だけ更新するのではなくエラーとして返す。

// gifHandler は GIF を表示専用として扱う。
// GIF のコメント拡張はタグ用の標準的な置き場所ではないため、書き込みは行わない。
type gifHandler struct{}

func (gifHandler) Kind() model.Kind     { return model.KindImage }
func (gifHandler) Extensions() []string { return []string{".gif"} }

func (gifHandler) Read(path string) (Info, error) {
	meta := &model.ImageMeta{}
	meta.Width, meta.Height = imageDimensions(path)
	return Info{Image: meta}, nil
}

func (gifHandler) WriteTags(string, []string) error { return ErrFormatReadOnly }

// svgHandler は SVG を表示専用として扱う。
// ただし SVG は XML なので、埋め込まれた XMP の dc:subject があれば読み取る。
type svgHandler struct{}

func (svgHandler) Kind() model.Kind     { return model.KindImage }
func (svgHandler) Extensions() []string { return []string{".svg"} }

// svgSizeRe は <svg> 要素の width / height 属性から数値部分を取り出す。
var svgSizeRe = regexp.MustCompile(`(?s)<svg\b[^>]*?\b(width|height)\s*=\s*["']\s*([0-9.]+)`)

// maxSVGSize を超える SVG は、メタデータ抽出のために全文を読み込むには大きすぎるとみなす。
const maxSVGSize = 4 << 20

func (svgHandler) Read(path string) (Info, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Info{}, err
	}
	if info.Size() > maxSVGSize {
		return Info{Image: &model.ImageMeta{}}, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Info{}, err
	}

	meta := &model.ImageMeta{}
	for _, m := range svgSizeRe.FindAllSubmatch(data, -1) {
		v := parseIntPrefix(string(m[2]))
		switch string(m[1]) {
		case "width":
			meta.Width = v
		case "height":
			meta.Height = v
		}
	}
	return Info{Tags: xmpSubjects(data), Image: meta}, nil
}

func (svgHandler) WriteTags(string, []string) error { return ErrFormatReadOnly }

// aacHandler は AAC / M4A を再生専用として扱う。
// MP4 コンテナのアトム書き換えは要件のスコープ外であり、読み取りも行わない。
type aacHandler struct{}

func (aacHandler) Kind() model.Kind     { return model.KindAudio }
func (aacHandler) Extensions() []string { return []string{".aac", ".m4a"} }

func (aacHandler) Read(string) (Info, error) {
	return Info{Audio: &model.AudioMeta{}}, nil
}

func (aacHandler) WriteTags(string, []string) error { return ErrFormatReadOnly }

// parseIntPrefix は文字列の先頭にある整数部分だけを読む。"120.5" は 120 になる。
func parseIntPrefix(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	return n
}
