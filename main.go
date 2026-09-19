// taggo はローカルファイルのドキュメントとアセットを、
// ファイル自身に埋め込んだタグで管理するデスクトップアプリ。
package main

import (
	"context"
	"embed"
	"flag"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/yuuxyu/taggo/internal/app"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// 起動時にフォルダを指定できるようにしておくと、
	// ファイルマネージャーからの「このフォルダで開く」やテスト起動が楽になる。
	folder := flag.String("folder", "", "起動時に読み込むフォルダ")
	flag.Parse()
	if *folder == "" && flag.NArg() > 0 {
		*folder = flag.Arg(0)
	}

	a, err := app.New()
	if err != nil {
		log.Fatalf("アプリの初期化に失敗しました: %v", err)
	}

	err = wails.Run(&options.App{
		Title:     "taggo",
		Width:     1280,
		Height:    860,
		MinWidth:  900,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
			// ローカルファイルのプレビュー配信を、通常のアセット配信に重ねる。
			Handler: app.NewAssetHandler(a),
		},
		BackgroundColour: &options.RGBA{R: 250, G: 250, B: 248, A: 255},
		OnStartup: func(ctx context.Context) {
			a.Startup(ctx)
			if *folder != "" {
				if err := a.OpenFolder(*folder); err != nil {
					log.Printf("起動時に指定されたフォルダを開けませんでした: %v", err)
				}
			}
		},
		OnShutdown: a.Shutdown,
		Bind: []any{
			a,
		},
	})
	if err != nil {
		log.Fatalf("アプリの起動に失敗しました: %v", err)
	}
}
