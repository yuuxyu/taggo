package meta

import (
	"io"
	"os"
	"path/filepath"
)

// writable は taggo がファイルをその場で書き換えてよいかを判定する。
// オーナーの書き込み権限があり、かつ親ディレクトリも書き込み可能なときにのみ true を返す。
// アトミックな置き換えは同じディレクトリに一時ファイルを作るため、両方が必要になる。
func writable(path string, info os.FileInfo) bool {
	if info.Mode()&0o200 == 0 {
		return false
	}
	return dirWritable(path) == nil
}

// replaceFile は write が生成した内容で path をアトミックに置き換える。
// 一時ファイルは対象と同じディレクトリに作るので rename がファイルシステムを跨がず、
// 元のパーミッションも引き継ぐ。
func replaceFile(path string, write func(w io.Writer) error) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".taggo-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(tmpName) // rename が成功していれば何もしない
	}()

	if err := write(tmp); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, info.Mode().Perm()); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// replaceFileBytes は、出力全体を既にメモリ上に持っている呼び出し側向けの replaceFile。
func replaceFileBytes(path string, data []byte) error {
	return replaceFile(path, func(w io.Writer) error {
		_, err := w.Write(data)
		return err
	})
}
