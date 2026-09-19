package meta

import (
	"fmt"
	"os"

	_ "golang.org/x/image/webp" // 画像サイズ取得のために WebP デコーダーを登録する

	"github.com/yuuxyu/taggo/internal/model"
)

func init() { Register(webpHandler{}) }

// webpHandler はタグを WebP の "XMP " チャンク（dc:subject）に格納する。
// WebP のメタデータは拡張フォーマット（VP8X）でしか持てないため、
// 単純フォーマットのファイルに書き込む場合は VP8X チャンクを補って拡張形式へ変換する。
// 読み取り時には、他ツールが書いた "EXIF" チャンクも見る。
type webpHandler struct{}

func (webpHandler) Kind() model.Kind     { return model.KindImage }
func (webpHandler) Extensions() []string { return []string{".webp"} }

const (
	webpFormType  = "WEBP"
	chunkVP8X     = "VP8X"
	chunkXMP      = "XMP "
	chunkEXIF     = "EXIF"
	chunkICCP     = "ICCP"
	chunkANIM     = "ANIM"
	vp8xChunkSize = 10
)

// VP8X のフラグバイト。どの補助チャンクが存在するかを示す。
const (
	vp8xFlagICC   byte = 0x20
	vp8xFlagAlpha byte = 0x10
	vp8xFlagEXIF  byte = 0x08
	vp8xFlagXMP   byte = 0x04
	vp8xFlagANIM  byte = 0x02
)

func (webpHandler) Read(path string) (Info, error) {
	f, err := readWebP(path)
	if err != nil {
		return Info{}, err
	}

	var tags []string
	if c, ok := f.Find(chunkXMP); ok {
		tags = append(tags, xmpSubjects(c.Data)...)
	}
	if c, ok := f.Find(chunkEXIF); ok {
		if rootIfd, err := parseRawExif(c.Data); err == nil {
			exifTags, _ := exifInfo(rootIfd)
			tags = append(tags, exifTags...)
		}
	}

	meta := &model.ImageMeta{}
	meta.Width, meta.Height = imageDimensions(path)
	return Info{Tags: tags, Image: meta}, nil
}

func (webpHandler) WriteTags(path string, tags []string) error {
	f, err := readWebP(path)
	if err != nil {
		return err
	}

	var existing []byte
	if c, ok := f.Find(chunkXMP); ok {
		existing = c.Data
	}
	packet := buildXMPPacket(existing, tags)

	if err := ensureVP8X(f, path); err != nil {
		return err
	}

	if len(tags) == 0 {
		f.Remove(chunkXMP)
	} else {
		f.Set(chunkXMP, packet)
	}
	if err := reorderWebPChunks(f); err != nil {
		return err
	}
	syncVP8XFlags(f)

	return replaceFileBytes(path, f.Bytes())
}

func readWebP(path string) (*riffFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f, err := parseRIFF(data)
	if err != nil {
		return nil, riffError("WebP", err)
	}
	if f.FormType != webpFormType {
		return nil, fmt.Errorf("WebP ではありません: フォームタイプ %q", f.FormType)
	}
	return f, nil
}

// ensureVP8X は、メタデータを保持できるよう拡張フォーマットのヘッダーチャンクを用意する。
// 単純フォーマットのファイルには、実際の画像サイズから VP8X を組み立てて先頭に挿入する。
func ensureVP8X(f *riffFile, path string) error {
	if _, ok := f.Find(chunkVP8X); ok {
		return nil
	}

	w, h := imageDimensions(path)
	if w <= 0 || h <= 0 {
		return fmt.Errorf("WebP の画像サイズを取得できないため拡張フォーマットへ変換できません")
	}
	if w > 1<<24 || h > 1<<24 {
		return fmt.Errorf("WebP の画像サイズが VP8X の上限を超えています (%dx%d)", w, h)
	}

	payload := make([]byte, vp8xChunkSize)
	// payload[0] はフラグ、payload[1:4] は予約領域で 0 のまま。
	// 幅・高さは「実値 - 1」を 24bit リトルエンディアンで格納する。
	putUint24LE(payload[4:7], uint32(w-1))
	putUint24LE(payload[7:10], uint32(h-1))

	f.Chunks = append([]riffChunk{{ID: chunkVP8X, Data: payload}}, f.Chunks...)
	return nil
}

// reorderWebPChunks は拡張フォーマットが定めるチャンク順へ並べ替える。
// VP8X → ICCP → ANIM → 画像データ → EXIF → XMP の順でなければ
// デコーダーによっては読めなくなる。
func reorderWebPChunks(f *riffFile) error {
	order := map[string]int{
		chunkVP8X: 0,
		chunkICCP: 1,
		chunkANIM: 2,
		chunkEXIF: 4,
		chunkXMP:  5,
	}
	rank := func(id string) int {
		if r, ok := order[id]; ok {
			return r
		}
		return 3 // 画像データ本体（VP8 / VP8L / ALPH / ANMF）
	}

	// 同じ順位のチャンク同士は元の並びを保ちたいので安定ソートにする。
	sorted := make([]riffChunk, 0, len(f.Chunks))
	for r := range 6 {
		for _, c := range f.Chunks {
			if rank(c.ID) == r {
				sorted = append(sorted, c)
			}
		}
	}
	if len(sorted) != len(f.Chunks) {
		return fmt.Errorf("WebP のチャンク並べ替えで件数が合いません (%d -> %d)", len(f.Chunks), len(sorted))
	}
	f.Chunks = sorted
	return nil
}

// syncVP8XFlags は、実際に存在する補助チャンクに合わせて VP8X のフラグを更新する。
func syncVP8XFlags(f *riffFile) {
	vp8x, ok := f.Find(chunkVP8X)
	if !ok || len(vp8x.Data) < vp8xChunkSize {
		return
	}
	flags := vp8x.Data[0]
	for _, pair := range []struct {
		id   string
		flag byte
	}{
		{chunkICCP, vp8xFlagICC},
		{chunkEXIF, vp8xFlagEXIF},
		{chunkXMP, vp8xFlagXMP},
		{chunkANIM, vp8xFlagANIM},
	} {
		if _, present := f.Find(pair.id); present {
			flags |= pair.flag
		} else {
			flags &^= pair.flag
		}
	}
	// アルファの有無は画像データ側の性質なので、既存のビットをそのまま保つ。
	_ = vp8xFlagAlpha
	vp8x.Data[0] = flags
}
func putUint24LE(dst []byte, v uint32) {
	dst[0] = byte(v)
	dst[1] = byte(v >> 8)
	dst[2] = byte(v >> 16)
}
