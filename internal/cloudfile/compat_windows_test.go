package cloudfile

import (
	"testing"

	"golang.org/x/sys/windows"
)

// パッケージを読み込んだ時点で、プレースホルダーを通常ファイルとして見せる設定に
// なっていること。これが崩れると os.Stat がプレースホルダーを開きに行く。
func TestPlaceholderCompatibilityMode(t *testing.T) {
	proc := windows.NewLazySystemDLL("ntdll.dll").NewProc("RtlQueryProcessPlaceholderCompatibilityMode")
	if proc.Find() != nil {
		t.Skip("クラウドファイルの仕組みが無い Windows のため確認を飛ばす")
	}
	mode, _, _ := proc.Call()
	if int8(mode) != phcmDisguisePlaceholder {
		t.Fatalf("プレースホルダーの見え方が違う: got %d, want %d", int8(mode), phcmDisguisePlaceholder)
	}
}
