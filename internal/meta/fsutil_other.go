//go:build !unix

package meta

// dirWritable に相当する移植性のある仕組みは Unix 以外に存在しない。
// そのためファイルモードの判定だけで書き込み可否を決め、
// 実際に書けなかった場合は編集時にエラーとして表面化させる。
func dirWritable(string) error { return nil }
