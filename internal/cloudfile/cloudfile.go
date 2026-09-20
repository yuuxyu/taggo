// Package cloudfile は、クラウド同期サービスが置いたファイルの扱いを判定する。
//
// Dropbox・OneDrive・iCloud などは「オンラインのみ」のファイルを、中身を持たない
// 置き換え用のファイル（プレースホルダー）としてローカルに置く。これを普通に開くと
// その場でクラウドからのダウンロードが始まるため、フォルダを走査するだけで
// 配下の全ファイルを引き落としてしまう。
//
// 幸い、プレースホルダーかどうかはファイル属性で分かり、属性の問い合わせでは
// ダウンロードは起こらない。taggo は中身を読む前に必ずここで判定し、
// 該当するファイルは開かずに一覧へ出す。
package cloudfile

import (
	"path/filepath"
	"strings"
)

// 名前でそれと分かるクラウド同期フォルダ。値は利用者へ見せる表示名。
//
// 属性で判定できない方式（Google ドライブのストリーミングのように、仮想ドライブ上で
// 通常のファイルに見えるもの）があるため、フォルダ名からの当たりも併用する。
// 取り違えても確認が一度出るだけなので、広めに拾わず分かりやすいものだけを挙げる。
var syncFolders = []struct {
	prefix string
	name   string
}{
	{"dropbox", "Dropbox"},
	{"onedrive", "OneDrive"}, // 「OneDrive - 会社名」のような名前も拾う
	{"google drive", "Google ドライブ"},
	{"googledrive", "Google ドライブ"},
	{"my drive", "Google ドライブ"},
	{"icloud drive", "iCloud Drive"},
	{"nextcloud", "Nextcloud"},
	{"pcloud", "pCloud"},
}

// LooksLikeSyncFolder は、そのパスがクラウド同期フォルダの中にあるように見えるとき、
// サービスの表示名を返す。分からない場合は空文字を返す。
//
// あくまで名前からの推測であり、これ自体は何も防がない。属性で判定できない方式に
// 備えて、走査を始める前に利用者へ確認するかどうかの判断に使う。
func LooksLikeSyncFolder(path string) string {
	for part := range strings.SplitSeq(filepath.Clean(path), string(filepath.Separator)) {
		lower := strings.ToLower(part)
		for _, f := range syncFolders {
			if strings.HasPrefix(lower, f.prefix) {
				return f.name
			}
		}
	}
	return ""
}
