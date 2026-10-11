package app

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// OpenInEditor は Markdown ファイルを、拡張子に紐づいたアプリ（既定のテキストエディタ）で開く。
// 本文は taggo の中で編集できるが、使い慣れたエディタで直したいときのために残してある。
// 外のエディタで保存した変更はウォッチャーが拾い、編集画面へ反映する。
//
// 開けるのは読み込み済みのエントリだけにする。フロントエンドから任意のパスを
// 渡されても、開いているフォルダ外のファイルや実行ファイルを起動しないため。
func (a *App) OpenInEditor(path string) error {
	e, ok := a.store.Get(path)
	if !ok {
		return fmt.Errorf("エントリが見つかりません: %s", path)
	}
	return openWithDefaultApp(e.Path)
}

// openWithDefaultApp はファイルを、拡張子に紐づいたアプリで開く。
func openWithDefaultApp(path string) error {
	file, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	// 動詞を指定せず、エクスプローラーでダブルクリックしたときと同じ既定の動作で開く。
	if err := windows.ShellExecute(0, nil, file, nil, nil, windows.SW_SHOWNORMAL); err != nil {
		return fmt.Errorf("エディタで開けませんでした: %w", err)
	}
	return nil
}
