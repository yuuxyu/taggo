package cloudfile

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var procCfGetSyncRootInfoByPath = windows.NewLazySystemDLL("cldapi.dll").NewProc("CfGetSyncRootInfoByPath")

// cfSyncRootInfoProvider は CfGetSyncRootInfoByPath に渡す情報の種類
// （CF_SYNC_ROOT_INFO_PROVIDER）。同期サービスの名前が得られる。
const cfSyncRootInfoProvider = 2

// cfSyncRootProviderInfo は CF_SYNC_ROOT_PROVIDER_INFO に対応する。
type cfSyncRootProviderInfo struct {
	ProviderStatus  uint32
	ProviderName    [256]uint16
	ProviderVersion [256]uint16
}

// SyncRootProvider は、path が Windows に登録されたクラウド同期フォルダ
// （シンクルート）の中にあるとき、同期サービスの名前（「Dropbox」など）を返す。
// 同期フォルダの外なら空文字を返す。
//
// 同期サービスが Windows のクラウドファイル API に登録した情報を問い合わせるだけで、
// ファイルは開かない。フォルダ名を変えていても正しく判定でき、名前が偶然似ているだけの
// フォルダを取り違えることもない。
//
// クラウドファイル API を使わない方式（Google ドライブの仮想ドライブなど）は判定できない。
// そうした方式は動作を確かめられないため、taggo の対象外としている。
func SyncRootProvider(path string) string {
	if procCfGetSyncRootInfoByPath.Find() != nil {
		return "" // クラウドファイル API が無い古い Windows
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return ""
	}
	var info cfSyncRootProviderInfo
	var returned uint32
	hr, _, _ := procCfGetSyncRootInfoByPath.Call(
		uintptr(unsafe.Pointer(p)),
		cfSyncRootInfoProvider,
		uintptr(unsafe.Pointer(&info)),
		unsafe.Sizeof(info),
		uintptr(unsafe.Pointer(&returned)),
	)
	if hr != 0 {
		return "" // 同期フォルダの外（ERROR_CLOUD_FILE_NOT_UNDER_SYNC_ROOT など）
	}
	if name := windows.UTF16ToString(info.ProviderName[:]); name != "" {
		return name
	}
	return "クラウド同期"
}
