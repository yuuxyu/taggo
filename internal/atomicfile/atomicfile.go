// Package atomicfile は、ファイルを書きかけの状態で残さないように置き換える。
//
// 同じフォルダの一時ファイルへ書き切ってから、元のファイルへ名前を変えて重ねる。
// 途中で失敗しても、元のファイルは書き込む前のまま残る。
package atomicfile

import (
	"os"
	"path/filepath"
	"strings"
)

// tempMarker は一時ファイルの名前に入れる印。フォルダの監視は、この印の付いた
// ファイルを taggo 自身の書き込みとして無視する（IsTemp）。
const tempMarker = ".taggo-"

// Replace は data で path を置き換える。一時ファイルの名前は ".<元の名前>.taggo-XXXX" にする。
func Replace(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+strings.TrimPrefix(filepath.Base(path), ".")+tempMarker+"*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(tmpName) // 置き換えに成功していれば何もしない
	}()

	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// IsTemp は、ファイル名 name が Replace の書き込みに使う一時ファイルかを判定する。
func IsTemp(name string) bool {
	return strings.HasPrefix(name, ".") && strings.Contains(name, tempMarker)
}
