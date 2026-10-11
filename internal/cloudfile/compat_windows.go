// Package cloudfile は、クラウド同期サービスが置いたファイルを、ふつうのファイルとして扱えるようにする。
//
// Dropbox や OneDrive は「オンラインのみ」のファイルを、中身を持たない置き換え用のファイル
// （プレースホルダー）としてローカルに置く。taggo はフォルダ直下のノートと、本文が参照する
// 画像を読む時点でダウンロードさせてよい方針なので、プレースホルダーかどうかは見分けない。
// ただし、プレースホルダーがプロセスにどう見えるかは環境で変わるので、起動時に固定しておく。
// このパッケージを読み込む（import _ で足りる）だけで効く。
package cloudfile

import "golang.org/x/sys/windows"

// phcmDisguisePlaceholder は、プレースホルダーを通常ファイルとして見せる設定
// （PHCM_DISGUISE_PLACEHOLDER）。
const phcmDisguisePlaceholder = 1

// init は、このプロセスからのプレースホルダーの見え方を「通常ファイルとして見せる」に固定する。
//
// もう一方の見せる設定（PHCM_EXPOSE_PLACEHOLDERS）では、プレースホルダーに
// REPARSE_POINT 属性が付いて見える。すると os.Stat はシンボリックリンクかどうかを
// 確かめるために CreateFile でハンドルを開くようになり、一覧の列挙や画像の配信の
// 確かめで、読むつもりの無いファイルまで触ってしまう。通常ファイルとして見せる設定なら
// REPARSE_POINT は隠れ、ふつうのファイルと同じように扱える。
//
// どちらになるかはプロセスごとに既定値が異なる（Go の実行ファイルは通常ファイルとして
// 見せる設定だったが、PowerShell では REPARSE_POINT が見えていた）。実行環境によって
// 前提が崩れないよう、起動時に明示しておく。ファイルを扱う前に必ず効くよう、
// パッケージを読み込んだ時点の init で行う。
func init() {
	proc := windows.NewLazySystemDLL("ntdll.dll").NewProc("RtlSetProcessPlaceholderCompatibilityMode")
	if proc.Find() != nil {
		return // クラウドファイルの仕組みが無い古い Windows。プレースホルダーも存在しない
	}
	_, _, _ = proc.Call(phcmDisguisePlaceholder)
}
