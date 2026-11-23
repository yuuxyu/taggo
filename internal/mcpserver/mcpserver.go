// Package mcpserver は、taggo のノートを MCP（Model Context Protocol）のサーバーとして、
// Claude Desktop などのクライアントへ渡す。
//
// taggo.exe を --mcp 付きで起動すると、画面を出さずにこのサーバーとして動く。クライアントは
// taggo.exe を子プロセスとして起動し、標準入出力でやり取りする。画面とは別のプロセスで動き、
// 画面と同じ仕組み（走査・インメモリ DB・ウォッチャー）で、自分の索引を持つ。ファイルが正なので、
// 画面と MCP サーバーの索引が食い違うことはなく、互いの変更はウォッチャーで拾う。
//
// 標準出力はプロトコル専用なので、ログは標準エラーへ書く。
package mcpserver

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"runtime/debug"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	// プレースホルダーの見え方を、ファイルを扱う前に固定する（画面と同じ）。
	_ "github.com/yuuxyu/taggo/internal/cloudfile"
	"github.com/yuuxyu/taggo/internal/settings"
)

// Options は MCP サーバーの起動の設定。
type Options struct {
	// Folder は読むフォルダ。空なら taggo の設定の「起動時に開くフォルダ」を読む。
	Folder string
	// ReadOnly なら、ノートを書き込むツールとプロンプトを出さない。
	ReadOnly bool
}

// Main は MCP サーバーを標準入出力で動かし、クライアントが接続を閉じるまで戻らない。
// 戻り値はプロセスの終了コード。
func Main(opts Options) int {
	log.SetOutput(os.Stderr)
	log.SetPrefix("taggo --mcp: ")

	lib, err := newLibrary()
	if err != nil {
		log.Print(err)
		return 1
	}
	defer lib.close()

	cfg := loadSettings()
	if folder, err := chooseFolder(opts.Folder, cfg); err != nil {
		// 終了してしまうと、利用者はクライアントのログを見ないと理由が分からない。
		// 起動はしておき、ツールを呼ばれたときに理由と直し方を返す。
		log.Print(err)
		lib.fail(err)
	} else {
		lib.open(folder, cfg.ScanLimit)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := newServer(lib, !opts.ReadOnly).Run(ctx, &mcp.StdioTransport{}); err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("MCP サーバーが止まりました: %v", err)
		return 1
	}
	return 0
}

// loadSettings は taggo の設定を読むだけで、何も書き換えない。
// 壊れた設定ファイルの退避は、画面の側に任せる。
func loadSettings() settings.Settings {
	path, err := settings.DefaultPath()
	if err != nil {
		log.Print(err)
		return settings.Default()
	}
	return settings.Peek(path)
}

// 読むフォルダが決まらないときの理由。ツールの結果として、そのまま利用者に伝わる。
var (
	errNoFolder = errors.New("読むフォルダが決まっていません。taggo の設定で「起動時に開くフォルダ」を" +
		"「前回開いたフォルダ」か「指定したフォルダ」にしてから、Claude Desktop を起動し直してください" +
		"（Claude Desktop の設定の args に \"--folder\" とフォルダのパスを足しても指定できます）")
	errNoLastFolder = errors.New("前回開いたフォルダがまだありません。taggo の画面でフォルダを一度開いてから、" +
		"Claude Desktop を起動し直してください")
)

// chooseFolder は読むフォルダを決める。--folder の指定があればそれを、無ければ
// taggo の設定の「起動時に開くフォルダ」を使う。
func chooseFolder(flagFolder string, cfg settings.Settings) (string, error) {
	if flagFolder != "" {
		return flagFolder, nil
	}
	if folder := cfg.StartupPath(); folder != "" {
		return folder, nil
	}
	if cfg.StartupMode == settings.StartupLast {
		return "", errNoLastFolder
	}
	return "", errNoFolder
}

// version は、クライアントへ名乗る taggo の版。go build が埋め込んだ版を使い、
// 無ければ（開発中のビルドでは）"dev" にする。
func version() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return "dev"
}
