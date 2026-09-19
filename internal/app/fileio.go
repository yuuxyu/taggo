package app

import (
	"fmt"
	"io"
	"os"
)

// maxMarkdownPreviewBytes はプレビューで読み込む Markdown の上限。
// これを超えるファイルはエディタで開くべき大きさで、プレビューには向かない。
const maxMarkdownPreviewBytes = 8 << 20

// readFileLimited は上限付きでファイルを読む。
func readFileLimited(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, maxMarkdownPreviewBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxMarkdownPreviewBytes {
		return nil, fmt.Errorf("ファイルが大きすぎてプレビューできません (%d バイト超)", maxMarkdownPreviewBytes)
	}
	return data, nil
}
