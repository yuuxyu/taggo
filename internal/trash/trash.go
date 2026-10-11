// Package trash は、ファイルを Windows のごみ箱へ移す。
//
// ノートを消しても取り返しがつくよう、完全には消さずにごみ箱へ移す。ごみ箱の無いドライブ
// （ネットワークドライブなど）では Windows が完全に消すかどうかを確かめるダイアログを出し、
// 利用者がやめれば何もしない（ErrCancelled）。
//
// 移すのは SHFileOperationW で行う。golang.org/x/sys/windows には無いので shell32.dll から直接呼ぶ。
package trash

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ErrCancelled は、ごみ箱の無いドライブで、完全に消す確認を利用者がやめたことを表す。
var ErrCancelled = errors.New("削除をやめました")

// retryFor は、ほかのプロセスが使用中のときに、やり直し続ける時間の上限。
// 同期サービスやウイルス対策ソフトが読むのは一瞬なので、これだけ待てば通る。
const retryFor = 2 * time.Second

// SHFILEOPSTRUCTW の wFunc と fFlags に入れる値。
const (
	foDelete = 0x0003
	// fofSilent は進み具合のダイアログを出さない。
	fofSilent = 0x0004
	// fofNoConfirmation は「ごみ箱へ移しますか？」を出さない（taggo の画面で確かめ済み）。
	fofNoConfirmation = 0x0010
	// fofAllowUndo はごみ箱へ移す。
	fofAllowUndo = 0x0040
	// fofNoErrorUI は失敗したときのダイアログを出さない（戻り値で知らせる）。
	fofNoErrorUI = 0x0400
	// fofWantNukeWarning は、ごみ箱へ移せずに完全に消すことになるときだけ、確認のダイアログを出す。
	// fofNoConfirmation より優先される。
	fofWantNukeWarning = 0x4000
)

// SHFileOperationW が返す、Win32 のエラーコードと重ならない古い値のうち、使用中を表すもの。
const deAccessDeniedSrc = 0x78

// shFileOpStruct は SHFILEOPSTRUCTW。taggo は 64 ビット版だけを作るので、64 ビットの並びに合わせる
// （32 ビット版の shellapi.h は 1 バイト境界に詰めるので並びが違う）。
type shFileOpStruct struct {
	hwnd                 uintptr
	wFunc                uint32
	from                 *uint16
	to                   *uint16
	flags                uint16
	anyOperationsAborted int32
	nameMappings         uintptr
	progressTitle        *uint16
}

var procSHFileOperationW = windows.NewLazySystemDLL("shell32.dll").NewProc("SHFileOperationW")

// Move は path のファイルをごみ箱へ移す。ほかのプロセスが使用中で失敗したときは、
// 間隔を少しずつ延ばしながら retryFor のあいだやり直す。
func Move(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(retryFor)
	wait := 10 * time.Millisecond
	for {
		err := move(abs)
		if err == nil || !inUse(err) {
			return err
		}
		if time.Now().Add(wait).After(deadline) {
			return fmt.Errorf("ほかのアプリ（同期サービスやウイルス対策ソフトなど）が使用中のため、ごみ箱へ移せませんでした: %w", err)
		}
		time.Sleep(wait)
		wait = min(wait*2, 200*time.Millisecond)
	}
}

// move は SHFileOperationW を 1 回呼んで、path をごみ箱へ移す。
func move(path string) error {
	// pFrom は、末尾に 0 を 2 つ並べたパスの並び。
	from, err := windows.UTF16FromString(path)
	if err != nil {
		return err
	}
	from = append(from, 0)

	// シェルの処理は COM を使うので、このスレッドで初期化しておく。
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	switch err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); {
	case err == nil, errors.Is(err, windows.Errno(1)): // S_OK か S_FALSE（初期化済み）
		defer windows.CoUninitialize()
	case errors.Is(err, windows.Errno(windows.RPC_E_CHANGED_MODE)):
		// 別の方式で初期化済み。そのまま使える。
	default:
		return fmt.Errorf("ごみ箱へ移す準備ができませんでした: %w", err)
	}

	op := shFileOpStruct{
		wFunc: foDelete,
		from:  &from[0],
		flags: fofAllowUndo | fofNoConfirmation | fofWantNukeWarning | fofSilent | fofNoErrorUI,
	}
	r, _, _ := procSHFileOperationW.Call(uintptr(unsafe.Pointer(&op)))
	switch {
	case op.anyOperationsAborted != 0, windows.Errno(r) == windows.ERROR_CANCELLED:
		return ErrCancelled
	case r == deAccessDeniedSrc:
		return windows.ERROR_ACCESS_DENIED
	case r != 0:
		return windows.Errno(r)
	}
	return nil
}

// inUse は、ほかのプロセスがファイルを開いているために失敗したかを判定する。
func inUse(err error) bool {
	return errors.Is(err, windows.ERROR_ACCESS_DENIED) ||
		errors.Is(err, windows.ERROR_SHARING_VIOLATION) ||
		errors.Is(err, windows.ERROR_LOCK_VIOLATION)
}
