package meta

// dirWritable は、ディレクトリへ書き込めるかを事前に確かめる代わりに常に nil を返す。
//
// Windows では ACL が絡むため、書き込めるかを事前に正しく判定する手軽な方法が無い。
// そのためファイルの読み取り専用属性だけで書き込み可否を決め、
// 実際に書けなかった場合は編集時にエラーとして表面化させる。
func dirWritable(string) error { return nil }
