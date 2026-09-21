// Package cloudfile は、クラウド同期サービスが置いたファイルの扱いを判定する。
//
// Dropbox や OneDrive は「オンラインのみ」のファイルを、中身を持たない
// 置き換え用のファイル（プレースホルダー）としてローカルに置く。これを普通に開くと
// その場でクラウドからのダウンロードが始まるため、フォルダを走査するだけで
// 配下の全ファイルを引き落としてしまう。
//
// 幸い、プレースホルダーかどうかはファイル属性で分かり、属性の問い合わせでは
// ダウンロードは起こらない。taggo は中身を読む前に必ずここで判定し、
// 該当するファイルは開かずに一覧へ出す。
//
// なお、ファイルを開くときに「ダウンロードしない」と指定する FILE_FLAG_OPEN_NO_RECALL は
// 安全網にならない。Dropbox で試すと、この指定を付けて読んでもダウンロードが始まった。
// そのため、開く前に属性で確かめる以外に防ぐ手段は無い。
package cloudfile

import (
	"errors"
	"io/fs"
	"os"
)

// ErrCloudOnly は、中身がクラウド上にしか無いため開かなかったことを表す。
var ErrCloudOnly = errors.New("中身がクラウド上にしか無いファイルです")

// StatLocal は path の属性を問い合わせ、中身がローカルにあるかを確かめる。
//
// 中身がクラウド上にしか無ければ ErrCloudOnly を返す。そのときも FileInfo は返すので、
// 呼び出し側は名前やサイズを使ってエントリを作り直せる。
//
// os.Stat はプレースホルダーを通常ファイルとして見せる設定（compat_windows.go）の下では
// 属性を問い合わせるだけでファイルを開かないため、これ自体でダウンロードは起こらない。
func StatLocal(path string) (fs.FileInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if IsPlaceholder(info) {
		return info, ErrCloudOnly
	}
	return info, nil
}
