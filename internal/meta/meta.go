// Package meta は各対応フォーマットのファイルに埋め込まれたタグの読み書きを担う。
// 埋め込みメタデータが taggo における正（Single Source of Truth）であり、
// サイドカーファイルや影のデータベースは一切持たない。
package meta

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/yuuxyu/taggo/internal/model"
)

// ErrFormatReadOnly は、拡張子と中身が食い違うファイルの中身が
// taggo の対応フォーマット外（AVIF・HEIC・BMP・TIFF・GIF・SVG・M4A など）
// だったときに WriteTags が返すエラー。
var ErrFormatReadOnly = errors.New("このファイル形式はタグ編集に対応していません")

// ErrUnsupported は taggo がまったく扱わない拡張子に対して返すエラー。
var ErrUnsupported = errors.New("対応していないファイル形式です")

// Info は 1 ファイルからハンドラーが抽出しうる情報をまとめたもの。
type Info struct {
	Tags    []string
	Title   string
	Preview string
	Links   []string
	TagPage string
	Image   *model.ImageMeta
	Audio   *model.AudioMeta
}

// Handler は 1 つのフォーマット系統について、埋め込みタグの読み書きを実装する。
type Handler interface {
	// Kind は、担当する拡張子が属するエントリ種別を返す。
	Kind() model.Kind
	// Extensions は、このハンドラーが担当する拡張子（小文字・ドット付き）を列挙する。
	Extensions() []string
	// Read はメタデータを抽出する。ファイルを変更してはならない。
	Read(path string) (Info, error)
	// WriteTags はファイルの埋め込みタグ集合を tags で置き換える。
	// tags は呼び出し側で正規化済みである。書き込めないハンドラーは
	// ファイルに一切触れずに ErrFormatReadOnly を返す。
	WriteTags(path string, tags []string) error
}

var (
	mu       sync.RWMutex
	handlers = map[string]Handler{}
)

// Register はハンドラーを、その担当拡張子に対して登録する。
// 重複登録はプログラミングミス以外にありえないため panic する。
func Register(h Handler) {
	mu.Lock()
	defer mu.Unlock()
	for _, ext := range h.Extensions() {
		if _, dup := handlers[ext]; dup {
			panic(fmt.Sprintf("meta: 拡張子 %q のハンドラーが重複して登録されました", ext))
		}
		handlers[ext] = h
	}
}

// HandlerFor は拡張子を担当するハンドラーを返す。
func HandlerFor(ext string) (Handler, bool) {
	mu.RLock()
	defer mu.RUnlock()
	h, ok := handlers[ext]
	return h, ok
}

// Extensions は taggo が走査対象とする全拡張子を返す。ファイルウォーカーが使う。
func Extensions() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(handlers))
	for ext := range handlers {
		out = append(out, ext)
	}
	return out
}

// resolve は、ファイルを扱うハンドラーと実際の形式を決める。
//
// 中身から形式を判別できた場合はそちらを優先する。拡張子を信じて
// 別形式として書き込むと、ファイルを壊しかねないためである。
// 判別できた形式に対応するハンドラーが無いときは、その形式名を添えて
// notSupported を返し、呼び出し側が理由を表示できるようにする。
func resolve(path string) (h Handler, format string, notSupported string) {
	ext := model.Ext(path)

	sniffed, ok := SniffFormat(path)
	if !ok {
		// 空ファイルや未知の形式。拡張子どおりに読んでみて、
		// 失敗すればその理由がそのままエントリに記録される。
		h, _ := HandlerFor(ext)
		return h, ext, ""
	}

	if h, ok := HandlerFor(sniffed); ok {
		return h, sniffed, ""
	}
	if name, known := recognizedFormats[sniffed]; known {
		return nil, sniffed, name
	}
	h, _ = HandlerFor(ext)
	return h, ext, ""
}

// Read は path の Entry を組み立てる。
// メタデータ読み取りの失敗はエラーとして返さず Entry に記録する。
// 壊れたファイル 1 件のせいでグリッドから消えてしまわないようにするためで、
// エラーになるのは未対応拡張子か stat 失敗のときだけである。
func Read(path string, info os.FileInfo) (*model.Entry, error) {
	ext := model.Ext(path)
	kind, supportedExt := model.KindForExt(ext)
	if !supportedExt {
		return nil, fmt.Errorf("%w: %s", ErrUnsupported, ext)
	}

	h, format, notSupported := resolve(path)

	e := &model.Entry{
		Path:     path,
		Name:     info.Name(),
		Ext:      ext,
		Format:   format,
		Kind:     kind,
		Size:     info.Size(),
		ModTime:  info.ModTime(),
		Tags:     []string{},
		Title:    info.Name(),
		Writable: writable(path, info),
	}
	if h != nil {
		e.Kind = h.Kind()
	}

	if notSupported != "" {
		// 中身は画像だが taggo が扱えない形式。一覧には出しつつ、
		// 書き込みは行わないことと理由をはっきり伝える。
		e.Writable = false
		e.Err = fmt.Sprintf("中身は %s 形式のため、タグの読み書きに対応していません", notSupported)
		return e, nil
	}
	if h == nil {
		e.Writable = false
		e.Err = fmt.Sprintf("%s 形式として読み取れませんでした", strings.TrimPrefix(ext, "."))
		return e, nil
	}

	got, err := h.Read(path)
	if err != nil {
		e.Err = err.Error()
		return e, nil
	}

	e.Tags = model.NormalizeTags(got.Tags)
	if got.Title != "" {
		e.Title = got.Title
	}
	e.Preview = got.Preview
	e.Links = got.Links
	e.TagPage = model.NormalizeTag(got.TagPage)
	e.Image = got.Image
	e.Audio = got.Audio
	return e, nil
}

// WriteTags は実ファイルへタグを書き込み、その成否を返す。
// 呼び出し側は、これが nil を返したときにのみ再インデックスすること。
//
// 書き込み先の形式も中身から判定する。拡張子を信じて別形式として
// 書き込むと、ファイルそのものを壊してしまうためである。
func WriteTags(path string, tags []string) error {
	h, format, notSupported := resolve(path)
	if notSupported != "" {
		return fmt.Errorf("%w: 中身は %s 形式です", ErrFormatReadOnly, notSupported)
	}
	if h == nil {
		return fmt.Errorf("%w: %s", ErrUnsupported, format)
	}
	return h.WriteTags(path, model.NormalizeTags(tags))
}
