// Package atomicfile は、ファイルを書きかけの状態で残さないように置き換える。
//
// 同じフォルダの一時ファイルへ書き切ってから、元のファイルへ名前を変えて重ねる。
// 途中で失敗しても、元のファイルは書き込む前のまま残る。
//
// Windows の通常の名前の変更（MoveFileEx）は、置き換え先をほかが開いている間は
// 「アクセスが拒否されました」で失敗する。taggo 自身のウォッチャーが保存直後のノートを
// 読み直すのと、次の保存が重なるだけでも起きる。そこで置き換えには、開かれていても重ねられる
// POSIX 方式の名前の変更（Windows 10 以降の NTFS）を使い、置き換えるファイルを読むときは
// このパッケージの Open / ReadFile で「読んでいる間も名前の変更や置き換えを許す」ように開く。
// 同期サービス（Dropbox など）やウイルス対策ソフトが許さずに開いている場合は防げないので、
// そのときは少し待ってやり直す。
package atomicfile

import (
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// tempMarker は一時ファイルの名前に入れる印。フォルダの監視は、この印の付いた
// ファイルを taggo 自身の書き込みとして無視する（IsTemp）。
const tempMarker = ".taggo-"

// renameRetryFor は、ほかのプロセスが使用中のときに、名前の変更をやり直し続ける時間の上限。
// 同期サービスやウイルス対策ソフトが読むのは一瞬なので、これだけ待てば通る。
const renameRetryFor = 2 * time.Second

// ErrInUse は、ほかのアプリが使用中のため、待っても置き換えられなかったことを表す。
var ErrInUse = errors.New("ほかのアプリ（同期サービスやウイルス対策ソフトなど）が使用中のため置き換えられませんでした")

// Replace は data で path を置き換える。一時ファイルの名前は ".<元の名前>.taggo-XXXX" にする。
func Replace(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+strings.TrimPrefix(filepath.Base(path), ".")+tempMarker+"*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(tmpName) // 置き換えに成功していれば何もしない
	}()

	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return retryInUse(func() error { return replaceFile(tmpName, path) })
}

// Rename は from を to へ名前を変える。ほかのプロセスが使用中で失敗したときは、少し待ってやり直す。
// 置き換え先が既にあるときの扱いは os.Rename と同じ。
func Rename(from, to string) error {
	return retryInUse(func() error { return os.Rename(from, to) })
}

// retryInUse は rename を呼び、ほかのプロセスが使用中で失敗したときは、
// 間隔を少しずつ延ばしながら renameRetryFor のあいだやり直す。
func retryInUse(rename func() error) error {
	deadline := time.Now().Add(renameRetryFor)
	wait := 10 * time.Millisecond
	for {
		err := rename()
		if err == nil || !inUse(err) {
			return err
		}
		if time.Now().Add(wait).After(deadline) {
			return errors.Join(ErrInUse, err)
		}
		time.Sleep(wait)
		wait = min(wait*2, 200*time.Millisecond)
	}
}

// FILE_RENAME_INFO の Flags に入れる値。
const (
	fileRenameReplaceIfExists = 0x1
	fileRenamePosixSemantics  = 0x2
)

// replaceFile は from を to へ名前を変え、to が既にあれば置き換える。
//
// まず POSIX 方式で置き換える。この方式は、to をほかが開いていても（名前の変更を許して
// 開いていれば）重ねられる。置き換えられた古いファイルは、開いている側が閉じるまで中身が残る。
// POSIX 方式で置き換えられなかったとき（FAT などのファイルシステム・古い Windows・同期サービスの
// フィルタが断ったときなど）は、その場で従来の os.Rename でも試す。どちらも使用中で失敗したら、
// 呼び出し側（retryInUse）が少し待ってやり直す。
func replaceFile(from, to string) error {
	if err := posixReplace(from, to); err == nil {
		return nil
	}
	return os.Rename(from, to)
}

// posixReplace は SetFileInformationByHandle（FileRenameInfoEx）で、POSIX 方式の名前の変更をする。
func posixReplace(from, to string) error {
	src, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	dst, err := windows.UTF16FromString(to)
	if err != nil {
		return err
	}
	h, err := windows.CreateFile(src, windows.DELETE|windows.SYNCHRONIZE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: err}
	}
	defer windows.CloseHandle(h)

	// FILE_RENAME_INFO（64 ビット）: Flags(4) 詰め物(4) RootDirectory(8) FileNameLength(4) FileName(…)
	nameLen := (len(dst) - 1) * 2             // 末尾の 0 を除いたバイト数
	buf := make([]uint64, (20+nameLen+2+7)/8) // 構造体の境界をそろえるため 8 バイト単位で確保する
	b := unsafe.Slice((*byte)(unsafe.Pointer(&buf[0])), len(buf)*8)
	binary.LittleEndian.PutUint32(b[0:], fileRenameReplaceIfExists|fileRenamePosixSemantics)
	binary.LittleEndian.PutUint32(b[16:], uint32(nameLen))
	for i, c := range dst {
		binary.LittleEndian.PutUint16(b[20+i*2:], c)
	}
	if err := windows.SetFileInformationByHandle(h, windows.FileRenameInfoEx, &b[0], uint32(len(b))); err != nil {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: err}
	}
	return nil
}

// inUse は、ほかのプロセスがファイルを開いているために失敗したかを判定する。
// 名前の変更を許さずに開かれていると、Windows は「アクセスが拒否されました」を返す。
func inUse(err error) bool {
	return errors.Is(err, windows.ERROR_ACCESS_DENIED) ||
		errors.Is(err, windows.ERROR_SHARING_VIOLATION) ||
		errors.Is(err, windows.ERROR_LOCK_VIOLATION)
}

// Open は、読んでいる間もほかから名前の変更や置き換え（Replace）ができるようにファイルを開く。
// os.Open は名前の変更を許さずに開くので、読んでいる最中の Replace や名前の変更が失敗してしまう。
func Open(path string) (*os.File, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(h), path), nil
}

// ReadFile は Open で開いてファイル全体を読む。
func ReadFile(path string) ([]byte, error) {
	f, err := Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

// IsTemp は、ファイル名 name が Replace の書き込みに使う一時ファイルかを判定する。
func IsTemp(name string) bool {
	return strings.HasPrefix(name, ".") && strings.Contains(name, tempMarker)
}
