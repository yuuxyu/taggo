// Package notefile は、ノートのファイルを扱うときの決まり（扱ってよい場所・読み込む大きさの
// 上限・中身の版）をまとめる。画面へ公開する API（internal/app）と MCP サーバー
// （internal/mcpserver）が、同じ決まりでノートを読み書きするために使う。
package notefile

import (
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
