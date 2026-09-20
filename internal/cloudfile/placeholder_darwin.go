//go:build darwin

package cloudfile

import (
	"io/fs"
	"syscall"
)

// sfDataless は、中身がクラウド上にしか無いことを表す macOS のフラグ（SF_DATALESS）。
// iCloud Drive の「最適化」されたファイルや、同じ仕組みを使う同期クライアントで立つ。
const sfDataless = 0x40000000

// IsPlaceholder は、中身がクラウド上にしか無いファイルかどうかを返す。
// stat の結果だけを見るので、この判定でダウンロードが始まることはない。
func IsPlaceholder(info fs.FileInfo) bool {
	if info == nil {
		return false
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	return st.Flags&sfDataless != 0
}
