package meta

import (
	"fmt"
	"image"
	"io"
	"os"

	_ "image/jpeg" // 画像サイズ取得のために JPEG デコーダーを登録する

	jpegstructure "github.com/dsoprea/go-jpeg-image-structure/v2"
	"github.com/yuuxyu/taggo/internal/model"
)

func init() { Register(jpegHandler{}) }

// jpegHandler はタグを IFD0 の Exif XPKeywords に格納する。
// 読み取り時には XMP の dc:subject も見るので、他の写真管理ツールが書いたタグも拾える。
type jpegHandler struct{}

func (jpegHandler) Kind() model.Kind     { return model.KindImage }
func (jpegHandler) Extensions() []string { return []string{".jpg", ".jpeg"} }

func (jpegHandler) Read(path string) (Info, error) {
	sl, err := parseJPEG(path)
	if err != nil {
		return Info{}, err
	}

	var (
		tags []string
		meta *model.ImageMeta
	)
	if rootIfd, _, err := sl.Exif(); err == nil {
		tags, meta = exifInfo(rootIfd)
	} else {
		meta = &model.ImageMeta{}
	}

	if _, seg, err := sl.FindXmp(); err == nil && seg != nil {
		tags = append(tags, xmpSubjects(seg.Data)...)
	}

	if meta.Width == 0 || meta.Height == 0 {
		meta.Width, meta.Height = imageDimensions(path)
	}
	return Info{Tags: tags, Image: meta}, nil
}

func (jpegHandler) WriteTags(path string, tags []string) error {
	sl, err := parseJPEG(path)
	if err != nil {
		return err
	}

	// Exif ブロックを持たない JPEG に対しても ConstructExifBuilder は空のビルダーを返すので、
	// 「新規作成」の分岐をここに書く必要はない。
	rootIb, err := sl.ConstructExifBuilder()
	if err != nil {
		return fmt.Errorf("Exif の準備に失敗しました: %w", err)
	}
	if err := setXPKeywords(rootIb, tags); err != nil {
		return err
	}
	if err := sl.SetExif(rootIb); err != nil {
		return err
	}
	return replaceFile(path, func(w io.Writer) error { return sl.Write(w) })
}

func parseJPEG(path string) (*jpegstructure.SegmentList, error) {
	mc, err := jpegstructure.NewJpegMediaParser().ParseFile(path)
	if err != nil {
		return nil, fmt.Errorf("JPEG の解析に失敗しました: %w", err)
	}
	sl, ok := mc.(*jpegstructure.SegmentList)
	if !ok {
		return nil, fmt.Errorf("JPEG の解析に失敗しました: 想定外のメディアコンテキスト %T", mc)
	}
	return sl, nil
}

// imageDimensions は、コンテナにサイズのメタデータが無いときに
// 画像ヘッダーをデコードして幅と高さを求めるフォールバック。
func imageDimensions(path string) (int, int) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}
