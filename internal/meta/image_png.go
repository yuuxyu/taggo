package meta

import (
	"bytes"
	"fmt"

	_ "image/png" // 画像サイズ取得のために PNG デコーダーを登録する

	pngstructure "github.com/dsoprea/go-png-image-structure/v2"
	"github.com/yuuxyu/taggo/internal/model"
)

func init() { Register(pngHandler{}) }

// pngHandler はタグを PNG の eXIf チャンク（中身は JPEG と同じ Exif IFD0 の XPKeywords）に格納する。
// 読み取り時には、古いツールがキーワードを置く tEXt/iTXt チャンクも見る。
type pngHandler struct{}

func (pngHandler) Kind() model.Kind     { return model.KindImage }
func (pngHandler) Extensions() []string { return []string{".png"} }

// pngKeywordChunkKeys は tEXt / iTXt でキーワードが入りうるキー名。
var pngKeywordChunkKeys = []string{"Keywords", "keywords", "XML:com.adobe.xmp"}

func (pngHandler) Read(path string) (Info, error) {
	cs, err := parsePNG(path)
	if err != nil {
		return Info{}, err
	}

	var (
		tags []string
		meta *model.ImageMeta
	)
	if rootIfd, _, err := cs.Exif(); err == nil {
		tags, meta = exifInfo(rootIfd)
	} else {
		meta = &model.ImageMeta{}
	}
	tags = append(tags, pngTextTags(cs)...)

	if meta.Width == 0 || meta.Height == 0 {
		meta.Width, meta.Height = imageDimensions(path)
	}
	return Info{Tags: tags, Image: meta}, nil
}

func (pngHandler) WriteTags(path string, tags []string) error {
	cs, err := parsePNG(path)
	if err != nil {
		return err
	}

	// PNG 側の ConstructExifBuilder は eXIf チャンクが無いとエラーになるので、
	// JPEG と違って空ビルダーの生成を自前で行う。
	rootIb, err := cs.ConstructExifBuilder()
	if err != nil {
		if rootIb, err = newRootExifBuilder(); err != nil {
			return fmt.Errorf("Exif の準備に失敗しました: %w", err)
		}
	}
	if err := setXPKeywords(rootIb, tags); err != nil {
		return err
	}
	if err := cs.SetExif(rootIb); err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := cs.WriteTo(&buf); err != nil {
		return fmt.Errorf("PNG の書き出しに失敗しました: %w", err)
	}
	return replaceFileBytes(path, buf.Bytes())
}

func parsePNG(path string) (*pngstructure.ChunkSlice, error) {
	mc, err := pngstructure.NewPngMediaParser().ParseFile(path)
	if err != nil {
		return nil, fmt.Errorf("PNG の解析に失敗しました: %w", err)
	}
	cs, ok := mc.(*pngstructure.ChunkSlice)
	if !ok {
		return nil, fmt.Errorf("PNG の解析に失敗しました: 想定外のメディアコンテキスト %T", mc)
	}
	return cs, nil
}

// pngTextTags は tEXt / iTXt チャンクからキーワードを拾う。
// チャンクのデータは "キー\x00値" という形式で、iTXt はさらに圧縮フラグなどが続く。
func pngTextTags(cs *pngstructure.ChunkSlice) []string {
	var out []string
	index := cs.Index()
	for _, typ := range []string{"tEXt", "iTXt"} {
		for _, chunk := range index[typ] {
			key, value, ok := bytes.Cut(chunk.Data, []byte{0})
			if !ok {
				continue
			}
			if !isPNGKeywordKey(string(key)) {
				continue
			}
			if typ == "iTXt" {
				// iTXt は値の前に「圧縮フラグ・圧縮方式・言語タグ・翻訳キー」が入る。
				// 圧縮されているものは taggo では扱わない。
				var okDecode bool
				value, okDecode = decodeITXtValue(value)
				if !okDecode {
					continue
				}
			}
			if string(key) == "XML:com.adobe.xmp" {
				out = append(out, xmpSubjects(value)...)
				continue
			}
			out = append(out, splitKeywordString(string(value))...)
		}
	}
	return out
}

func isPNGKeywordKey(key string) bool {
	for _, k := range pngKeywordChunkKeys {
		if key == k {
			return true
		}
	}
	return false
}

// decodeITXtValue は iTXt チャンクのヘッダー部を読み飛ばして本体テキストを返す。
// 圧縮フラグが立っている場合は、非対応として false を返す。
func decodeITXtValue(rest []byte) ([]byte, bool) {
	if len(rest) < 2 {
		return nil, false
	}
	compressed := rest[0] != 0
	rest = rest[2:] // 圧縮フラグと圧縮方式の 2 バイト
	if compressed {
		return nil, false
	}
	// 言語タグと翻訳済みキーが、それぞれ NUL 終端で続く。
	for range 2 {
		_, after, ok := bytes.Cut(rest, []byte{0})
		if !ok {
			return nil, false
		}
		rest = after
	}
	return rest, true
}
