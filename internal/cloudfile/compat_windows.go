package cloudfile

import "golang.org/x/sys/windows"

// phcmDisguisePlaceholder は、プレースホルダーを通常ファイルとして見せる設定
// （PHCM_DISGUISE_PLACEHOLDER）。
const phcmDisguisePlaceholder = 1

// init は、このプロセスからのプレースホルダーの見え方を「通常ファイルとして見せる」に固定する。
//
// もう一方の見せる設定（PHCM_EXPOSE_PLACEHOLDERS）では、プレースホルダーに
// REPARSE_POINT 属性が付いて見える。すると os.Stat はシンボリックリンクかどうかを
// 確かめるために CreateFile でハンドルを開くようになり、属性の問い合わせだけでは
// 済まなくなる。通常ファイルとして見せる設定なら REPARSE_POINT は隠れる一方、
// 判定に使う RECALL_ON_DATA_ACCESS は見えたままであることを Dropbox で確かめてある。
//
// どちらになるかはプロセスごとに既定値が異なる（Go の実行ファイルは通常ファイルとして
// 見せる設定だったが、PowerShell では REPARSE_POINT が見えていた）。実行環境によって
// 前提が崩れないよう、起動時に明示しておく。判定の正しさがこの設定に依存するため、
// パッケージを読み込んだ時点で必ず効くよう init で行う。
func init() {
	proc := windows.NewLazySystemDLL("ntdll.dll").NewProc("RtlSetProcessPlaceholderCompatibilityMode")
	if proc.Find() != nil {
		return // クラウドファイルの仕組みが無い古い Windows。プレースホルダーも存在しない
	}
	_, _, _ = proc.Call(phcmDisguisePlaceholder)
}
