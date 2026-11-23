package app

import "github.com/yuuxyu/taggo/internal/notefile"

// maxNoteBytes は編集画面で扱う Markdown の上限。
// これを超えるファイルは読み込まず、保存もしない。名前の変更でも書き換えない。
const maxNoteBytes = notefile.MaxBytes

// readFileLimited は上限付きでファイルを読む。読んでいる間も、保存（atomicfile.Replace）の
// 置き換えを邪魔しないように開く。
func readFileLimited(path string) ([]byte, error) { return notefile.ReadLimited(path) }

// contentVersion は、ファイルの中身を表す値を返す。保存の前に、読んだときから
// 書き換えられていないかを確かめるのに使う。
func contentVersion(raw []byte) string { return notefile.Version(raw) }
