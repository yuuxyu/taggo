package app

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// OpenInEditor は Markdown ファイルを、拡張子に紐づいたアプリ（既定のテキストエディタ）で開く。
// taggo は本文を編集しないため、本文を直したいときはここから好きなエディタへ渡す。
//
// 開けるのは読み込み済みのエントリだけにする。フロントエンドから任意のパスを
// 渡されても、開いているフォルダ外のファイルや実行ファイルを起動しないため。
// クラウド上にだけあるファイルは、開くとダウンロードが始まるので開かない。
func (a *App) OpenInEditor(path string) error {
	e, ok := a.store.Get(path)
	if !ok {
		return fmt.Errorf("エントリが見つかりません: %s", path)
	}
	if e.CloudOnly {
		return fmt.Errorf("クラウド上にだけあるファイルのため開きません: %s", e.Name)
	}
	if err := a.ensureLocal(e); err != nil {
		return err
	}

	file, err := windows.UTF16PtrFromString(e.Path)
	if err != nil {
		return err
	}
	// 動詞を指定せず、エクスプローラーでダブルクリックしたときと同じ既定の動作で開く。
	if err := windows.ShellExecute(0, nil, file, nil, nil, windows.SW_SHOWNORMAL); err != nil {
		return fmt.Errorf("エディタで開けませんでした: %w", err)
	}
	return nil
}
