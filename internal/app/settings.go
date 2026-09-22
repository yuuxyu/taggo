package app

import (
	"fmt"
	"os"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/yuuxyu/taggo/internal/settings"
)

// OpenStartupFolder は起動時のフォルダを開く。Wails の起動フックから呼ぶ。
//
// コマンドライン引数でフォルダが指定されていればそれを優先し、無ければ設定のフォルダを開く。
// 開けなかった理由は、画面の準備ができてから TakeStartupWarnings で伝える。
func (a *App) OpenStartupFolder(arg string) {
	if w := a.settings.LoadWarning(); w != "" {
		a.addStartupWarning(w)
	}
	if arg != "" {
		if err := a.OpenFolder(arg); err != nil {
			a.addStartupWarning(fmt.Sprintf("起動時に指定されたフォルダを開けませんでした: %v", err))
		}
		return
	}

	dir := a.settings.Get().StartupPath()
	if dir == "" {
		return
	}
	// 外付けドライブを外していたり、フォルダを移したりしていても、起動自体はできるようにする。
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		a.addStartupWarning(fmt.Sprintf("起動時に開くフォルダが見つからなかったため、開きませんでした: %s", dir))
		return
	}
	if err := a.OpenFolder(dir); err != nil {
		a.addStartupWarning(fmt.Sprintf("起動時に開くフォルダを開けませんでした: %v", err))
	}
}

// addStartupWarning は起動時の警告を、画面が取りに来るまで取っておく。
func (a *App) addStartupWarning(message string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.startupWarnings = append(a.startupWarnings, message)
}

// TakeStartupWarnings は起動時の警告を返し、以後は返さない。
// 起動フックの時点では画面がイベントを受け取れないため、画面から取りに来てもらう。
func (a *App) TakeStartupWarnings() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	warnings := a.startupWarnings
	a.startupWarnings = nil
	return warnings
}

// GetSettings は今の設定を返す。
func (a *App) GetSettings() settings.Settings {
	return a.settings.Get()
}

// SettingsPath は設定ファイルの置き場所を返す。設定画面に出して、何をどこに持っているかを見せる。
func (a *App) SettingsPath() string {
	return a.settings.Path()
}

// SaveSettings は設定を保存し、今のアプリへ反映したうえで、保存した設定を返す。
//
// 走査の上限件数は、次にフォルダを開いたときと続きを読み込むときから効く。
// 並び順は起動時の既定値なので、今の一覧へ反映するかは画面側が決める。
func (a *App) SaveSettings(next settings.Settings) (settings.Settings, error) {
	cur := a.settings.Get()

	// 前回開いたフォルダは画面から書き換えさせず、ここで決める。
	next.LastFolder = ""
	if next.StartupMode == settings.StartupLast {
		next.LastFolder = cur.LastFolder
		if cur.StartupMode != settings.StartupLast {
			// 切り替えた時点で開いているフォルダを「前回」にしておく。
			// そうしないと、次にフォルダを開くまで起動時に何も開かない。
			next.LastFolder = a.store.Root()
		}
	}

	if err := a.settings.Save(next); err != nil {
		return cur, err
	}
	saved := a.settings.Get()

	a.mu.Lock()
	a.maxEntries = saved.ScanLimit
	a.mu.Unlock()
	a.applyWindowTheme(saved.Theme)
	return saved, nil
}

// applyWindowTheme はタイトルバーなど、ウィンドウの枠の配色を設定に合わせる。
// 画面の中身の配色はフロントエンドが切り替える。
func (a *App) applyWindowTheme(theme settings.Theme) {
	if a.ctx == nil {
		return
	}
	switch theme {
	case settings.ThemeLight:
		runtime.WindowSetLightTheme(a.ctx)
	case settings.ThemeDark:
		runtime.WindowSetDarkTheme(a.ctx)
	default:
		runtime.WindowSetSystemDefaultTheme(a.ctx)
	}
}
