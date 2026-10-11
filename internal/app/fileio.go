package app

import (
	"fmt"
	"io"
	"os"
)

// maxNoteBytes は編集画面で扱う Markdown の上限。
// これを超えるファイルは読み込まず、保存もしない。名前の変更でも書き換えない。
const maxNoteBytes = 8 << 20

// readFileLimited は上限付きでファイルを読む。
func readFileLimited(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, maxNoteBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxNoteBytes {
		return nil, fmt.Errorf("ファイルが大きすぎて開けません (%d バイト超)", maxNoteBytes)
	}
	return data, nil
}
