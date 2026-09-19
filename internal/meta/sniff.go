package meta

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
// 拡張子だけで扱いを決めると、メタデータを誤った形式として読み書きしてしまい、
// 最悪の場合ファイルを壊す。そのため taggo は先頭バイトから実際の形式を判定し、
// 判定できたときはそちらを優先する。

// sniffLen は形式判定のために読む先頭バイト数。
// ftyp ボックスのブランドまで見るには 16 バイトあれば足りるが、
// SVG は先頭に空白や XML 宣言が入るため、余裕を持たせている。
const sniffLen = 512

// SniffFormat はファイルの先頭バイトから実際の形式を判定し、
// ".png" のような拡張子表記で返す。判定できない場合は ("", false) を返す。
func SniffFormat(path string) (string, bool) {
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

// sniffBytes は先頭バイト列から形式を判定する。
func sniffBytes(head []byte) (string, bool) {
	switch {
	case len(head) >= 3 && bytes.HasPrefix(head, []byte{0xFF, 0xD8, 0xFF}):
		return ".jpg", true
	case bytes.HasPrefix(head, []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}):
		return ".png", true
	case bytes.HasPrefix(head, []byte("GIF87a")), bytes.HasPrefix(head, []byte("GIF89a")):
		return ".gif", true
	case bytes.HasPrefix(head, []byte("fLaC")):
		return ".flac", true
	case bytes.HasPrefix(head, []byte("ID3")):
		return ".mp3", true
	case bytes.HasPrefix(head, []byte("BM")):
		return ".bmp", true
	case bytes.HasPrefix(head, []byte("II*\x00")), bytes.HasPrefix(head, []byte("MM\x00*")):
		return ".tiff", true
	}

	if len(head) >= 12 && bytes.HasPrefix(head, []byte("RIFF")) {
		switch string(head[8:12]) {
		case "WEBP":
			return ".webp", true
		case "WAVE":
			return ".wav", true
		}
	}

	if ext, ok := sniffISOBMFF(head); ok {
		return ext, true
	}
	if isMP3FrameSync(head) {
		return ".mp3", true
	}
	if isSVG(head) {
		return ".svg", true
	}
	return "", false
}

// sniffISOBMFF は ISO Base Media 形式（ftyp ボックスを持つ MP4 系）のブランドを見る。
// HEIC / AVIF / M4A はいずれもこの形式の上に作られている。
func sniffISOBMFF(head []byte) (string, bool) {
	if len(head) < 12 || string(head[4:8]) != "ftyp" {
		return "", false
	}
	// 先頭 4 バイトはボックス長。ブランドはその後ろに 4 バイトで入る。
	brand := string(head[8:12])
	switch brand {
	case "avif", "avis":
		return ".avif", true
	case "heic", "heix", "hevc", "hevx", "heim", "heis", "mif1", "msf1":
		return ".heic", true
	case "M4A ", "mp42", "mp41", "isom", "iso2":
		return ".m4a", true
	}

	// 互換ブランド一覧にしか目的のブランドが入っていないファイルもある。
	size := int(binary.BigEndian.Uint32(head[0:4]))
	end := min(size, len(head))
	for off := 16; off+4 <= end; off += 4 {
		switch string(head[off : off+4]) {
		case "avif", "avis":
			return ".avif", true
		case "heic", "heix", "mif1", "msf1":
			return ".heic", true
		}
	}
	return "", false
}

// isMP3FrameSync は ID3 タグを持たない MP3 の、フレーム同期ワードを判定する。
func isMP3FrameSync(head []byte) bool {
	return len(head) >= 2 && head[0] == 0xFF && head[1]&0xE0 == 0xE0
}

// isSVG は先頭が XML 宣言か <svg> 要素で始まるかを見る。
func isSVG(head []byte) bool {
	// 先頭には空白や BOM が入りうるので、それらを落としてから判定する。
	trimmed := strings.TrimLeft(string(head), " \t\r\n\uFEFF")
	if strings.HasPrefix(trimmed, "<svg") {
		return true
	}
	if !strings.HasPrefix(trimmed, "<?xml") && !strings.HasPrefix(trimmed, "<!DOCTYPE svg") {
		return false
	}
	return strings.Contains(trimmed, "<svg")
}

// recognizedFormats は、判定はできるが taggo がタグ編集に対応していない形式。
// 拡張子を偽ったファイルを黙って壊さないよう、ここで明示的に区別する。
var recognizedFormats = map[string]string{
	".avif": "AVIF",
	".heic": "HEIC / HEIF",
	".bmp":  "BMP",
	".tiff": "TIFF",
}

// ContentType は形式に対応する MIME タイプを返す。
// 中身から判定した形式を渡すことで、ブラウザへ嘘の型を伝えずに済む。
func ContentType(format string) string {
	switch format {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	case ".avif":
		return "image/avif"
	case ".heic":
		return "image/heic"
	case ".bmp":
		return "image/bmp"
	case ".tiff":
		return "image/tiff"
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	case ".flac":
		return "audio/flac"
	case ".aac":
		return "audio/aac"
	case ".m4a":
		return "audio/mp4"
	case ".md", ".markdown":
		return "text/markdown; charset=utf-8"
	}
	return ""
}
