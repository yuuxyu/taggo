// Package model は taggo のバックエンド全体で共有するデータ型を定義する。
//
// taggo が一覧・検索の対象にするのは、開いたフォルダの直下にある
// Markdown のノートだけである。ノートのファイル名（拡張子を除く）は、そのまま
// 1 つのタグを表し、ノートには本文に `[[タグ]]` と書いてタグを付ける。
//
// タグの正（Single Source of Truth）は常にディスク上のファイル自身であり、
// ここで定義する型は、その正を BuntDB へ展開するためのメモリ内表現にすぎない。
package model

import (
	"path/filepath"
	"strings"
	"time"
)

// Entry は走査済みの Markdown ファイル 1 件を表す。BuntDB には Path をキーとした JSON として格納する。
type Entry struct {
	Path    string `json:"path"`    // 絶対パス。BuntDB のキーになる
	RelPath string `json:"relPath"` // 走査ルートからの相対パス。表示用
	Name    string `json:"name"`
	Ext     string `json:"ext"` // ファイル名の拡張子。小文字、先頭のドットを含む

	Size    int64     `json:"size"`
	ModTime time.Time `json:"modTime"`

	// Tags はノートに付いているタグ。本文に `[[タグ]]` と書かれたもの。正規化・ソート済み。
	Tags []string `json:"tags"`

	// Title は人間向けのラベル。最初の見出しか Front Matter の title で、
	// どちらも取れない場合はファイル名をそのまま使う。
	Title string `json:"title"`

	// Preview はカードに表示する短い本文抜粋。
	Preview string `json:"preview,omitempty"`

	// Thumbnail はカードに表示する画像。本文で最初に使われている画像の参照
	// （書かれたままのパスか http(s) の URL）か、最初に貼られた YouTube 動画の
	// サムネイル画像の URL が入る。どちらも無ければ空になる。
	Thumbnail string `json:"thumbnail,omitempty"`

	// Links は Markdown 本文から見つかった、他のノートへのリンク。書かれたままの
	// 相対パス（"./sub/impl.md" など）で、関連ページのグラフ構築に使う。
	Links []string `json:"links,omitempty"`

	// Err は致命的でないメタデータ読み取り失敗を記録する。
	// 読み取りに失敗したことを隠さずに、ファイル自体は一覧に表示するため。
	Err string `json:"err,omitempty"`

	// CloudOnly は、中身がクラウド上にしか無いファイル（Dropbox や OneDrive の
	// オンライン専用ファイル）であることを表す。true のとき taggo はその中身を
	// 一度も開いておらず、埋まっているのはファイル一覧から分かる情報だけになる。
	// 開けばダウンロードが始まるため、利用者が明示的に取り込むまでは触らない。
	CloudOnly bool `json:"cloudOnly,omitempty"`

	// Missing は、まだファイルの無いページであることを表す。ページの無いタグや、行き先の
	// 無いリンクをたどったときに、ファイルを作らずにページとして開くために使う。
	// BuntDB には載せない。
	Missing bool `json:"missing,omitempty"`
}

// NewCloudOnly は、中身を読まずに分かる情報だけでエントリを組み立てる。
// クラウド上にしか無いファイルを、ダウンロードを起こさずに一覧へ出すために使う。
func NewCloudOnly(path, name string, size int64, modTime time.Time) *Entry {
	return &Entry{
		Path:      path,
		Name:      name,
		Ext:       Ext(path),
		Size:      size,
		ModTime:   modTime,
		Tags:      []string{},
		Title:     name,
		CloudOnly: true,
	}
}

// NewMissing は、まだファイルの無いページ path のエントリを組み立てる。
// 名前はファイル名から決まり、タグもリンクも持たない。
func NewMissing(path string) *Entry {
	name := filepath.Base(path)
	return &Entry{
		Path:    path,
		RelPath: name,
		Name:    name,
		Ext:     Ext(path),
		Tags:    []string{},
		Title:   strings.TrimSuffix(name, filepath.Ext(name)),
		Missing: true,
	}
}

// NormalizeTag は生のタグ文字列を taggo の正規形に整える。
// 先頭の '#' と前後の空白を取り除き、内部の連続する空白は 1 つの半角空白にまとめる。
// 正規化した結果が空になる入力に対しては "" を返す。
func NormalizeTag(raw string) string {
	t := strings.TrimSpace(raw)
	t = strings.TrimPrefix(t, "#")
	t = strings.Join(strings.Fields(t), " ")
	return t
}

// NormalizeTags は全タグを正規化し、空を捨て、大文字小文字を無視して重複を除く。
// 重複時は最初に現れた表記を残す。同じタグ集合を持つファイルが必ず同じ JSON に
// シリアライズされるよう、結果はソートして返す。
func NormalizeTags(raw []string) []string {
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		t := NormalizeTag(r)
		if t == "" {
			continue
		}
		k := strings.ToLower(t)
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, t)
	}
	sortStrings(out)
	return out
}

func sortStrings(s []string) {
	// 典型的なケース（タグ数個）ではアロケーションが発生しないよう挿入ソートを使う。
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && strings.ToLower(s[j]) < strings.ToLower(s[j-1]); j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// IsMarkdown は、taggo が扱う Markdown ファイルの拡張子かを判定する。
// taggo がタグを管理するのは Markdown だけで、画像などは本文から参照されるだけのファイルとして扱う。
func IsMarkdown(path string) bool {
	switch Ext(path) {
	case ".md", ".markdown":
		return true
	}
	return false
}

// Ext はパスの拡張子を小文字にして返す。
func Ext(path string) string { return strings.ToLower(filepath.Ext(path)) }

// PageTag は、ノートのファイル名が表すタグを返す。拡張子を除いたファイル名を
// タグと同じ規則で正規化したもの。
func PageTag(path string) string {
	name := filepath.Base(path)
	return NormalizeTag(strings.TrimSuffix(name, filepath.Ext(name)))
}

// TagKey は、タグを大文字小文字を区別せずに突き合わせるためのキーを返す。
func TagKey(tag string) string { return strings.ToLower(NormalizeTag(tag)) }

// TagFileName は、タグ tag を表すページのファイル名（"タグ.md"）を返す。
// Windows のファイル名に使えない文字を含むタグや、予約されたデバイス名と同じタグは、
// ページにできないので ok に false を返す。
func TagFileName(tag string) (name string, ok bool) {
	t := NormalizeTag(tag)
	if t == "" || t == "." || t == ".." {
		return "", false
	}
	if strings.ContainsFunc(t, func(r rune) bool {
		return r < 0x20 || strings.ContainsRune(`<>:"/\|?*`, r)
	}) {
		return "", false
	}
	// Windows は末尾のドットと空白を黙って落とすので、別の名前のファイルになってしまう。
	if strings.HasSuffix(t, ".") {
		return "", false
	}
	stem, _, _ := strings.Cut(t, ".")
	if reservedNames[strings.ToUpper(strings.TrimSpace(stem))] {
		return "", false
	}
	return t + ".md", true
}

// reservedNames は Windows がデバイス名として予約しているファイル名。
// 拡張子を付けても（"CON.md" でも）ファイルとしては使えない。
var reservedNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
	"COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
	"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}
