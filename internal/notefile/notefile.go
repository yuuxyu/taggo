// Package notefile は、ノートのファイルを扱うときの決まり（扱ってよい場所・読み込む大きさの
// 上限・中身の版）をまとめる。画面へ公開する API（internal/app）と MCP サーバー
// （internal/mcpserver）が、同じ決まりでノートを読み書きするために使う。
package notefile

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/yuuxyu/taggo/internal/atomicfile"
	"github.com/yuuxyu/taggo/internal/model"
)

// MaxBytes は、読み書きする Markdown の大きさの上限。
// これを超えるファイルは読み込まず、保存もしない。名前の変更でも書き換えない。
const MaxBytes = 8 << 20

// CheckPagePath は、path を root のページのパスとして扱ってよいかを確かめる。
//
// 任意の場所のファイルを読み書きさせないよう、開いているフォルダの直下の
// .md / .markdown に限る。taggo が読むのはフォルダ直下のノートだけなので、
// サブフォルダに作っても一覧には出ない。
func CheckPagePath(root, path string) error {
	if root == "" {
		return errors.New("フォルダが開かれていません")
	}
	if !model.IsMarkdown(path) {
		return errors.New("Markdown ファイルではありません")
	}
	if !Within(root, path) {
		return errors.New("開いているフォルダの外のノートは扱いません")
	}
	if !strings.EqualFold(filepath.Dir(filepath.Clean(path)), filepath.Clean(root)) {
		return errors.New("サブフォルダのノートは扱いません。taggo が読むのはフォルダ直下のノートだけです")
	}
	return nil
}

// Within は path が root 配下（root 自身を含む）にあるかを判定する。
func Within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// ReadLimited は上限付きでファイルを読む。読んでいる間も、保存（atomicfile.Replace）の
// 置き換えを邪魔しないように開く。
func ReadLimited(path string) ([]byte, error) {
	f, err := atomicfile.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, MaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxBytes {
		return nil, fmt.Errorf("ファイルが大きすぎて開けません (%d バイト超)", MaxBytes)
	}
	return data, nil
}

// Version は、ファイルの中身を表す値を返す。保存の前に、読んだときから
// 書き換えられていないかを確かめるのに使う。
func Version(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// BOM はファイルの先頭に付くことのある UTF-8 の BOM。
var BOM = []byte{0xEF, 0xBB, 0xBF}

// Text は、ノートの中身を、先頭の BOM と改行の種類を覚えたまま文字列として扱うためのもの。
// 書き換えるときに、元のファイルの BOM と改行（LF / CRLF）を保つために使う。
type Text struct {
	// Body は BOM を除いた中身。改行は書かれたまま（CRLF なら \r\n のまま）。
	Body string
	// BOM は、元のファイルの先頭に BOM があったこと。
	BOM bool
	// EOL はこのノートの改行。最初の改行に合わせ、改行が無ければ LF とする。
	EOL string
}

// Decode はファイルの中身を Text にする。
func Decode(raw []byte) Text {
	t := Text{EOL: "\n"}
	if rest, ok := bytes.CutPrefix(raw, BOM); ok {
		t.BOM, raw = true, rest
	}
	t.Body = string(raw)
	if i := strings.IndexByte(t.Body, '\n'); i > 0 && t.Body[i-1] == '\r' {
		t.EOL = "\r\n"
	}
	return t
}

// Encode は Body を、元の BOM を付け直したファイルの中身にする。
func (t Text) Encode() []byte {
	if !t.BOM {
		return []byte(t.Body)
	}
	return append(append([]byte{}, BOM...), t.Body...)
}

// Lines は s の改行を、このノートの改行にそろえる。s の改行は LF でも CRLF でもよい。
func (t Text) Lines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	if t.EOL == "\r\n" {
		s = strings.ReplaceAll(s, "\n", "\r\n")
	}
	return s
}
