// Package imagefile は、Markdown の本文に埋め込まれた画像ファイルを見分ける。
//
// taggo は画像をタグ管理の対象にしない。画像を扱うのは、ノートの本文が
// 参照している画像をプレビューへ配信するときだけである。
package imagefile

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"strings"
)

// ファイルの中身と拡張子は、実際にはよく食い違う。
// ブラウザから保存した画像やメッセージアプリ経由のファイルは、
// 中身が PNG なのに名前が .jpg、といったことが珍しくない。
//
// 拡張子どおりの MIME タイプを返すと、ブラウザはその型を信じて描画を拒否し、
// 表示できない画像になってしまう。そのため先頭バイトから実際の形式を判定する。

// sniffLen は形式判定のために読む先頭バイト数。
// ftyp ボックスのブランドまで見るには 16 バイトあれば足りるが、
// SVG は先頭に空白や XML 宣言が入るため、余裕を持たせている。
const sniffLen = 512

// ContentType はファイルの先頭バイトから、ブラウザで表示できる画像の MIME タイプを判定する。
// 画像でないか、WebView で表示できない形式（HEIC・TIFF など）なら ("", false) を返す。
func ContentType(path string) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()

	head := make([]byte, sniffLen)
	n, err := io.ReadFull(f, head)
	if n == 0 && err != nil {
		return "", false
	}
	return sniffBytes(head[:n])
}

// sniffBytes は先頭バイト列から画像の MIME タイプを判定する。
func sniffBytes(head []byte) (string, bool) {
	switch {
	case bytes.HasPrefix(head, []byte{0xFF, 0xD8, 0xFF}):
		return "image/jpeg", true
	case bytes.HasPrefix(head, []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}):
		return "image/png", true
	case bytes.HasPrefix(head, []byte("GIF87a")), bytes.HasPrefix(head, []byte("GIF89a")):
		return "image/gif", true
	case bytes.HasPrefix(head, []byte("BM")):
		return "image/bmp", true
	case len(head) >= 12 && bytes.HasPrefix(head, []byte("RIFF")) && string(head[8:12]) == "WEBP":
		return "image/webp", true
	case isAVIF(head):
		return "image/avif", true
	case isSVG(head):
		return "image/svg+xml", true
	}
	return "", false
}

// isAVIF は ISO Base Media 形式（ftyp ボックスを持つ MP4 系）のうち、AVIF のブランドを見る。
// 同じ入れ物を使う HEIC や M4A は WebView で表示できないので、ここでは画像とみなさない。
func isAVIF(head []byte) bool {
	if len(head) < 12 || string(head[4:8]) != "ftyp" {
		return false
	}
	// 先頭 4 バイトはボックス長。ブランドはその後ろに 4 バイトで入る。
	switch string(head[8:12]) {
	case "avif", "avis":
		return true
	}

	// 互換ブランド一覧にしか AVIF のブランドが入っていないファイルもある。
	size := int(binary.BigEndian.Uint32(head[0:4]))
	end := min(size, len(head))
	for off := 16; off+4 <= end; off += 4 {
		switch string(head[off : off+4]) {
		case "avif", "avis":
			return true
		}
	}
	return false
}

// isSVG は先頭が XML 宣言か <svg> 要素で始まるかを見る。
func isSVG(head []byte) bool {
	// 先頭には空白や BOM が入りうるので、それらを落としてから判定する。
	trimmed := strings.TrimLeft(string(head), " \t\r\n"+string(rune(0xFEFF)))
	if strings.HasPrefix(trimmed, "<svg") {
		return true
	}
	if !strings.HasPrefix(trimmed, "<?xml") && !strings.HasPrefix(trimmed, "<!DOCTYPE svg") {
		return false
	}
	return strings.Contains(trimmed, "<svg")
}
