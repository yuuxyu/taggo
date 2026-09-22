package app

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/yuuxyu/taggo/internal/cloudfile"
	"github.com/yuuxyu/taggo/internal/imagefile"
)

// PathImage は、Markdown の本文に埋め込まれた画像を配信する内部エンドポイント。
// WebView からローカルファイルへ直接アクセスすることはできないため、
// Wails のアセットサーバーにハンドラーを差し込んで配信する。
const PathImage = "/taggo/image"

// NewAssetHandler は PathImage を処理する http.Handler を返す。
//
// 画像は一覧にもタグ管理にも載せないので、インメモリ DB には登録されていない。
// その代わり、配信するのは「現在開いているフォルダ配下にあり、中身がブラウザで
// 表示できる画像」のファイルだけに限る。パスを細工して無関係なファイルを
// 読み出される余地を無くすため、画像でないものは中身を見て断る。
//
// App のメソッドではなく関数にしているのは、Wails が App の公開メソッドを
// すべてフロントエンドへ公開してしまい、JavaScript から呼べない型を
// 戻り値に持つメソッドがバインディングに混ざるのを避けるため。
func NewAssetHandler(a *App) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(PathImage, a.serveImage)
	return mux
}

func (a *App) serveImage(w http.ResponseWriter, r *http.Request) {
	path, err := a.resolveImage(r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	contentType, ok := imagefile.ContentType(path)
	if !ok {
		http.Error(w, "表示できる画像ファイルではありません", http.StatusUnsupportedMediaType)
		return
	}

	f, err := os.Open(path)
	if err != nil {
		http.Error(w, "ファイルを開けませんでした", http.StatusNotFound)
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		http.Error(w, "ファイル情報を取得できませんでした", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", contentType)
	// 中身から判定した型をブラウザに推測し直させない。
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// SVG を直接開かれても、中のスクリプトや外部リソースは動かさない。
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
}

// resolveImage はクエリのパスを検証し、配信してよい画像ファイルの絶対パスを返す。
func (a *App) resolveImage(raw string) (string, error) {
	if raw == "" {
		return "", errors.New("path クエリが指定されていません")
	}

	abs, err := filepath.Abs(raw)
	if err != nil {
		return "", errors.New("パスを解決できませんでした")
	}

	root := a.store.Root()
	if root == "" {
		return "", errors.New("フォルダが開かれていません")
	}
	if !within(root, abs) {
		return "", errors.New("開いているフォルダの外のファイルは配信しません")
	}

	// 中身がクラウド上にしか無いファイルは、開いた時点でダウンロードが始まる。
	// ノートを開いただけで通信が走らないよう、属性だけを見て断る。
	info, err := cloudfile.StatLocal(abs)
	if errors.Is(err, cloudfile.ErrCloudOnly) {
		return "", fmt.Errorf("クラウド上にだけある画像のため配信しません: %s", filepath.Base(abs))
	}
	if err != nil {
		return "", errors.New("ファイルが見つかりません")
	}
	if info.IsDir() {
		return "", errors.New("フォルダは配信しません")
	}

	// フォルダ内に置いたシンボリックリンクから外へ抜けられないよう、実体の場所でも確かめる。
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", errors.New("パスを解決できませんでした")
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil || !within(realRoot, real) {
		return "", errors.New("開いているフォルダの外のファイルは配信しません")
	}
	return abs, nil
}

// within は path が root 配下（root 自身を含む）にあるかを判定する。
func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
