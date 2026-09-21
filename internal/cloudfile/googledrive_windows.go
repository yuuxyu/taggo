package cloudfile

import (
	"strings"

	"golang.org/x/sys/windows"
)

// IsGoogleDriveStreaming は、path が Google ドライブ for desktop のストリーミング用
// 仮想ドライブ（既定では G:）の中にあるかを返す。
//
// この方式は Windows のクラウドファイルの仕組みを使わないため、クラウド上にしか無い
// ファイルも通常のファイルに見え、属性で見分けられない。走査すると中身の読み取りで
// ダウンロードが始まってしまうので、taggo はこの仮想ドライブを対象外とする。
//
// 仮想ドライブは、ファイルシステム名が FAT32 でボリュームラベルが「Google Drive」の
// ボリュームとして見える。これはボリュームの情報を問い合わせるだけで、ファイルは開かない。
// 実物の Google ドライブでは動作を確かめていない（公開情報に基づく判定）。
// ミラーリング方式のフォルダは通常の NTFS 上にあり全ファイルがローカルにあるので、
// ここでは該当せず、ふつうに読み込める。
func IsGoogleDriveStreaming(path string) bool {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return false
	}
	// フォルダへマウントされている場合もあるので、ドライブ文字ではなく
	// path を含むボリュームのルートを求めてから問い合わせる。
	root := make([]uint16, windows.MAX_PATH+1)
	if err := windows.GetVolumePathName(p, &root[0], uint32(len(root))); err != nil {
		return false
	}
	label := make([]uint16, windows.MAX_PATH+1)
	fsName := make([]uint16, windows.MAX_PATH+1)
	if err := windows.GetVolumeInformation(&root[0], &label[0], uint32(len(label)),
		nil, nil, nil, &fsName[0], uint32(len(fsName))); err != nil {
		return false
	}
	return isGoogleDriveVolume(windows.UTF16ToString(fsName), windows.UTF16ToString(label))
}

// isGoogleDriveVolume は、ボリュームの情報が Google ドライブの仮想ドライブのものかを判定する。
//
// FAT32 だけでは USB メモリなども拾ってしまうため、ラベルと組み合わせる。
// 表示言語によってラベルが変わる場合に備えて、日本語の表記も受け付ける。
func isGoogleDriveVolume(fsName, label string) bool {
	if !strings.EqualFold(fsName, "FAT32") {
		return false
	}
	l := strings.ToLower(strings.TrimSpace(label))
	return strings.HasPrefix(l, "google drive") || strings.HasPrefix(l, "google ドライブ")
}
