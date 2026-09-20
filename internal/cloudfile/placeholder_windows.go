//go:build windows

package cloudfile

import (
	"io/fs"
	"syscall"
)

// クラウド同期が「中身はまだローカルに無い」と示すために立てるファイル属性。
//
//   - OFFLINE               … 古くからある印。Dropbox の一部の方式などで立つ
//   - RECALL_ON_OPEN        … 開いた時点で呼び戻しが走る
//   - RECALL_ON_DATA_ACCESS … 中身を読んだ時点で呼び戻しが走る（OneDrive の
//     ファイル オンデマンド、現行の Dropbox など）
//
// syscall に定数が無いものがあるため、ここでまとめて定義する。
const (
	fileAttributeOffline            = 0x00001000
	fileAttributeRecallOnOpen       = 0x00040000
	fileAttributeRecallOnDataAccess = 0x00400000
)

const placeholderMask = fileAttributeOffline | fileAttributeRecallOnOpen | fileAttributeRecallOnDataAccess

// IsPlaceholder は、中身がクラウド上にしか無いファイルかどうかを返す。
//
// 判定に使うのはディレクトリ列挙で既に得ている属性だけで、ファイルは開かない。
// そのため、この判定自体でダウンロードが始まることはない。
func IsPlaceholder(info fs.FileInfo) bool {
	if info == nil {
		return false
	}
	data, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return false
	}
	return data.FileAttributes&placeholderMask != 0
}
