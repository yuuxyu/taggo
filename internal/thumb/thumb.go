// Package thumb はカードのグリッド表示に使うサムネイルを生成し、メモリ上に保持する。
//
// 生成結果はディスクに書かない。要件どおり taggo はファイルのメタデータ領域以外へ
// 何も書き込まないため、キャッシュもアプリ終了時に消える前提で持つ。
package thumb

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"sync"
	"time"

	_ "image/gif"  // GIF をデコード対象に加える
	_ "image/jpeg" // JPEG をデコード対象に加える
	_ "image/png"  // PNG をデコード対象に加える

	"golang.org/x/image/draw"

	// 拡張子を偽ったファイル（中身が BMP や TIFF の .png など）でも
	// サムネイルを出せるように、標準外の形式もデコード対象に含める。
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

const (
	// DefaultWidth はカード 1 枚あたりのサムネイル幅の既定値。
	// 高 DPI ディスプレイでも粗く見えないよう、カードの表示幅より大きめに取る。
	DefaultWidth = 480
	// maxWidth は要求できるサムネイル幅の上限。
	maxWidth = 1600
	// maxCacheEntries はメモリ上に保持するサムネイルの最大件数。
	// 要件が定める走査上限（2 万件）に対し、一度に画面へ出るのはごく一部なので、
	// 直近で使ったぶんだけ残せば足りる。
	maxCacheEntries = 512
	// maxSourceBytes を超える画像はサムネイル化せず、そのまま原寸を返す判断を呼び出し側に委ねる。
	maxSourceBytes = 64 << 20
)

// Cache は生成済みサムネイルのメモリ内キャッシュ。ゼロ値で使える。
type Cache struct {
	mu    sync.Mutex
	items map[string]*entry
	// order は挿入順。上限を超えたときに古いものから捨てるために持つ。
	order []string
}

type entry struct {
	data []byte
	// modTime は元ファイルの更新日時。ファイルが変わったら作り直すために保持する。
	modTime time.Time
	size    int64
}

// NewCache は空のキャッシュを返す。
func NewCache() *Cache {
	return &Cache{items: map[string]*entry{}}
}

// Get は path のサムネイルを JPEG バイト列で返す。
// 生成済みで元ファイルが変わっていなければキャッシュを返す。
func (c *Cache) Get(path string, width int) ([]byte, error) {
	if width <= 0 {
		width = DefaultWidth
	}
	width = min(width, maxWidth)

	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > maxSourceBytes {
		return nil, fmt.Errorf("画像が大きすぎてサムネイルを生成できません (%d バイト)", info.Size())
	}

	key := fmt.Sprintf("%s|%d", path, width)

	c.mu.Lock()
	if e, ok := c.items[key]; ok && e.modTime.Equal(info.ModTime()) && e.size == info.Size() {
		data := e.data
		c.mu.Unlock()
		return data, nil
	}
	c.mu.Unlock()

	data, err := generate(path, width)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.items == nil {
		c.items = map[string]*entry{}
	}
	if _, exists := c.items[key]; !exists {
		c.order = append(c.order, key)
	}
	c.items[key] = &entry{data: data, modTime: info.ModTime(), size: info.Size()}
	c.evictLocked()
	return data, nil
}

// Invalidate は path に紐づくサムネイルを捨てる。
// ファイルウォッチャーが変更を検知したときに呼ぶ。
func (c *Cache) Invalidate(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	kept := c.order[:0]
	for _, key := range c.order {
		if pathOfKey(key) == path {
			delete(c.items, key)
			continue
		}
		kept = append(kept, key)
	}
	c.order = kept
}

// Clear は全キャッシュを捨てる。フォルダを選び直したときに呼ぶ。
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = map[string]*entry{}
	c.order = nil
}

// evictLocked は上限を超えたぶんを古い順に捨てる。呼び出し元が c.mu を握っていること。
func (c *Cache) evictLocked() {
	for len(c.order) > maxCacheEntries {
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.items, oldest)
	}
}

// pathOfKey はキャッシュキーから元のファイルパスを取り出す。
func pathOfKey(key string) string {
	if i := lastIndexByte(key, '|'); i >= 0 {
		return key[:i]
	}
	return key
}

func lastIndexByte(s string, b byte) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == b {
			return i
		}
	}
	return -1
}

// generate は画像を読み込んで縮小し、JPEG として書き出す。
// 元画像が要求幅より小さい場合は拡大せず、そのままの解像度で書き出す。
func generate(path string, width int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	src, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("画像のデコードに失敗しました: %w", err)
	}

	bounds := src.Bounds()
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		return nil, fmt.Errorf("画像のサイズが不正です")
	}

	dstW := min(width, bounds.Dx())
	dstH := bounds.Dy() * dstW / bounds.Dx()
	if dstH < 1 {
		dstH = 1
	}

	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	// CatmullRom は縮小時の品質が高く、写真のサムネイルに向く。
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, bounds, draw.Over, nil)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 85}); err != nil {
		return nil, fmt.Errorf("サムネイルの書き出しに失敗しました: %w", err)
	}
	return buf.Bytes(), nil
}
