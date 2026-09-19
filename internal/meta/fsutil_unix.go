//go:build unix

package meta

import (
	"path/filepath"

	"golang.org/x/sys/unix"
)

// dirWritable は path を含むディレクトリが書き込み可能なら nil を返す。
// 一時ファイル作成 + rename によるアトミックな置き換えが実際に必要とするのはこの権限である。
func dirWritable(path string) error {
	return unix.Access(filepath.Dir(path), unix.W_OK)
}
