package meta

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"io"
	"regexp"
	"strings"
	"unicode/utf16"

	exif "github.com/dsoprea/go-exif/v3"
	exifcommon "github.com/dsoprea/go-exif/v3/common"
	"github.com/yuuxyu/taggo/internal/model"
)

// tagXPKeywords は Windows 系の「キーワード」Exif タグ（IFD0 0x9C9E）。
// セミコロン区切りの一覧を、末尾 NUL 付きの UTF-16LE バイト列として保持する。
// エクスプローラー・Lightroom をはじめ、多くの写真管理ツールが共通で参照する領域である。
const tagXPKeywords = "XPKeywords"

// tagIDXPKeywords は XPKeywords のタグ ID。削除時に ID 指定が必要になる。
const tagIDXPKeywords = 0x9c9e

// xpKeywordSep は Windows が XPKeywords 内で複数キーワードを連結する際の区切り文字。
const xpKeywordSep = "; "

// encodeXPKeywords は tags を、XPKeywords が要求する末尾 NUL 付き UTF-16LE バイト列に変換する。
func encodeXPKeywords(tags []string) []byte {
	s := strings.Join(tags, xpKeywordSep)
	units := utf16.Encode([]rune(s))
	units = append(units, 0) // XPKeywords は NUL 終端
	buf := make([]byte, 0, len(units)*2)
	for _, u := range units {
		buf = binary.LittleEndian.AppendUint16(buf, u)
	}
	return buf
}

// decodeXPKeywords は UTF-16LE バイト列をタグの一覧へ戻す。
func decodeXPKeywords(raw []byte) []string {
	if len(raw) < 2 {
		return nil
	}
	units := make([]uint16, 0, len(raw)/2)
	for i := 0; i+1 < len(raw); i += 2 {
		u := binary.LittleEndian.Uint16(raw[i : i+2])
		if u == 0 {
			break
		}
		units = append(units, u)
	}
	s := string(utf16.Decode(units))
	return splitKeywordString(s)
}

// splitKeywordString は、各ツールが区別なく使う区切り文字でキーワード文字列を分割する。
// 区切りの直後に空白が入る書式（"a; b"）が一般的なため、各要素は前後の空白を落として返す。
func splitKeywordString(s string) []string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ';' || r == ',' || r == '\n' || r == '\r' || r == 0
	})
	out := parts[:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// exifInfo はパース済みの Exif ツリーから、taggo のタグと
// 画像メタデータパネルに出す項目を取り出す。
// タグが存在しないこと自体はエラーではない。ほとんどのファイルは一部しか持たない。
func exifInfo(rootIfd *exif.Ifd) (tags []string, meta *model.ImageMeta) {
	meta = &model.ImageMeta{}
	if rootIfd == nil {
		return nil, meta
	}

	if raw, ok := byteTag(rootIfd, tagXPKeywords); ok {
		tags = decodeXPKeywords(raw)
	}
	meta.Make = strings.TrimSpace(stringTag(rootIfd, "Make"))
	meta.Model = strings.TrimSpace(stringTag(rootIfd, "Model"))
	if d := stringTag(rootIfd, "DateTime"); d != "" {
		meta.Taken = d
	}

	if exifIfd, err := rootIfd.ChildWithIfdPath(exifcommon.IfdExifStandardIfdIdentity); err == nil && exifIfd != nil {
		if d := stringTag(exifIfd, "DateTimeOriginal"); d != "" {
			meta.Taken = d // 撮影日時のほうがファイル更新日時より信頼できる
		}
		meta.Lens = strings.TrimSpace(stringTag(exifIfd, "LensModel"))
		meta.Width = intTag(exifIfd, "PixelXDimension")
		meta.Height = intTag(exifIfd, "PixelYDimension")
	}
	if meta.Width == 0 {
		meta.Width = intTag(rootIfd, "ImageWidth")
		meta.Height = intTag(rootIfd, "ImageLength")
	}
	return tags, meta
}

func firstTagValue(ifd *exif.Ifd, name string) (any, bool) {
	results, err := ifd.FindTagWithName(name)
	if err != nil || len(results) == 0 {
		return nil, false
	}
	v, err := results[0].Value()
	if err != nil {
		return nil, false
	}
	return v, true
}

func stringTag(ifd *exif.Ifd, name string) string {
	v, ok := firstTagValue(ifd, name)
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return strings.TrimRight(s, "\x00 ")
}

func byteTag(ifd *exif.Ifd, name string) ([]byte, bool) {
	v, ok := firstTagValue(ifd, name)
	if !ok {
		return nil, false
	}
	b, ok := v.([]byte)
	return b, ok
}

func intTag(ifd *exif.Ifd, name string) int {
	v, ok := firstTagValue(ifd, name)
	if !ok {
		return 0
	}
	switch t := v.(type) {
	case []uint32:
		if len(t) > 0 {
			return int(t[0])
		}
	case []uint16:
		if len(t) > 0 {
			return int(t[0])
		}
	}
	return 0
}

// setXPKeywords は Exif ビルダーの IFD0 に tags を書き込む。
// Exif ブロックを持たないファイルの場合は IFD0 を新たに作る。
// tags が空のときはフィールドごと削除する。
func setXPKeywords(rootIb *exif.IfdBuilder, tags []string) error {
	ifd0, err := exif.GetOrCreateIbFromRootIb(rootIb, exifcommon.IfdStandardIfdIdentity.UnindexedString())
	if err != nil {
		return err
	}
	if len(tags) == 0 {
		// もともと設定されていなければ DeleteAll は「見つからない」を返すが、
		// それは望んだ最終状態そのものなので、エラーは意図的に無視する。
		_, _ = ifd0.DeleteAll(tagIDXPKeywords)
		return nil
	}
	return ifd0.SetStandardWithName(tagXPKeywords, encodeXPKeywords(tags))
}

// newRootExifBuilder は、まだ Exif ブロックを持たないファイル向けに空の IFD0 ビルダーを作る。
func newRootExifBuilder() (*exif.IfdBuilder, error) {
	im := exifcommon.NewIfdMapping()
	if err := exifcommon.LoadStandardIfds(im); err != nil {
		return nil, err
	}
	return exif.NewIfdBuilder(
		im,
		exif.NewTagIndex(),
		exifcommon.IfdStandardIfdIdentity,
		exifcommon.EncodeDefaultByteOrder,
	), nil
}

// --- XMP ---------------------------------------------------------------
//
// XMP の dc:subject バッグは XPKeywords のクロスプラットフォーム版にあたる。
// Lightroom や digiKam が書いたタグも見えるように taggo は読み取り側で対応し、
// Exif から得たタグとマージする。

var xmpPacketRe = regexp.MustCompile(`(?s)<x:xmpmeta.*?</x:xmpmeta>`)

// xmpSubjects は生の XMP パケットから dc:subject の項目を取り出す。
func xmpSubjects(packet []byte) []string {
	m := xmpPacketRe.Find(packet)
	if m == nil {
		m = packet
	}

	dec := xml.NewDecoder(bytes.NewReader(m))
	var (
		out       []string
		inSubject bool
		inLi      bool
		buf       strings.Builder
	)
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return out // 途中で壊れていても、そこまでに読めた分は返す
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch {
			case t.Name.Local == "subject" && isDublinCore(t.Name.Space):
				inSubject = true
			case inSubject && t.Name.Local == "li":
				inLi = true
				buf.Reset()
			}
		case xml.CharData:
			if inLi {
				buf.Write(t)
			}
		case xml.EndElement:
			switch {
			case t.Name.Local == "subject" && isDublinCore(t.Name.Space):
				inSubject = false
			case inLi && t.Name.Local == "li":
				inLi = false
				if s := strings.TrimSpace(buf.String()); s != "" {
					out = append(out, s)
				}
			}
		}
	}
	return out
}

func isDublinCore(space string) bool {
	return space == "" || strings.Contains(space, "purl.org/dc/elements")
}

// parseRawExif は、生の Exif ブロック（TIFF ヘッダーから始まるバイト列）を解析する。
// WebP の EXIF チャンクのように、コンテナ側がブロックをそのまま抱えている形式で使う。
func parseRawExif(raw []byte) (*exif.Ifd, error) {
	// 一部のツールは JPEG の APP1 と同じ "Exif\0\0" 接頭辞を付けたまま格納する。
	raw = bytes.TrimPrefix(raw, []byte("Exif\x00\x00"))

	im := exifcommon.NewIfdMapping()
	if err := exifcommon.LoadStandardIfds(im); err != nil {
		return nil, err
	}
	_, index, err := exif.Collect(im, exif.NewTagIndex(), raw)
	if err != nil {
		return nil, err
	}
	return index.RootIfd, nil
}
