package app

import (
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/yuuxyu/taggo/internal/meta"
	"github.com/yuuxyu/taggo/internal/model"
	"github.com/yuuxyu/taggo/internal/thumb"
)

// ローカルファイルをプレビューへ渡すための内部エンドポイント。
// WebView からローカルファイルへ直接アクセスすることはできないため、
// Wails のアセットサーバーにハンドラーを差し込んで配信する。
const (
	// PathFile は原寸のファイルを返す。画像の拡大表示や音声の再生に使う。
	PathFile = "/taggo/file"
	// PathThumb は縮小したサムネイル（JPEG）を返す。カードのグリッドに使う。
	PathThumb = "/taggo/thumb"
)

// NewAssetHandler は PathFile / PathThumb を処理する http.Handler を返す。
//
// 配信対象は「現在開いているフォルダ配下で、かつインメモリ DB に登録済み」の
// ファイルだけに限る。登録済みであることを条件にすることで、
// パスを細工して無関係なファイルを読み出される余地を無くしている。
//
// App のメソッドではなく関数にしているのは、Wails が App の公開メソッドを
// すべてフロントエンドへ公開してしまい、JavaScript から呼べない型を
// 戻り値に持つメソッドがバインディングに混ざるのを避けるため。
func NewAssetHandler(a *App) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(PathFile, a.serveFile)
	mux.HandleFunc(PathThumb, a.serveThumb)
	return mux
}

func (a *App) serveFile(w http.ResponseWriter, r *http.Request) {
	entry, err := a.resolveEntry(r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	f, err := os.Open(entry.Path)
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

	w.Header().Set("Content-Type", contentTypeFor(entry))
	// ServeContent が Range リクエストを処理してくれるので、
	// 音声のシーク操作がそのまま効く。
	http.ServeContent(w, r, entry.Name, info.ModTime(), f)
}

func (a *App) serveThumb(w http.ResponseWriter, r *http.Request) {
	entry, err := a.resolveEntry(r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if entry.Kind != model.KindImage {
		http.Error(w, "画像ファイルではありません", http.StatusBadRequest)
		return
	}

	width := thumb.DefaultWidth
	if v := r.URL.Query().Get("w"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			width = n
		}
	}

	data, err := a.thumbs.Get(entry.Path, width)
	if err != nil {
		// SVG のようにラスタ化できない形式は、原寸をそのまま返して描画側に任せる。
		a.serveFile(w, r)
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "no-cache")
	if _, err := w.Write(data); err != nil {
		return
	}
}

// resolveEntry はクエリのパスを検証し、登録済みエントリを返す。
func (a *App) resolveEntry(raw string) (*model.Entry, error) {
	if raw == "" {
		return nil, fmt.Errorf("path クエリが指定されていません")
	}

	abs, err := filepath.Abs(raw)
	if err != nil {
		return nil, fmt.Errorf("パスを解決できませんでした")
	}

	root := a.store.Root()
	if root == "" {
		return nil, fmt.Errorf("フォルダが開かれていません")
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("開いているフォルダの外のファイルは配信しません")
	}

	entry, ok := a.store.Get(abs)
	if !ok {
		return nil, fmt.Errorf("エントリが見つかりません")
	}
	// 中身がクラウド上にしか無いファイルは、配信しようとした時点で
	// ダウンロードが始まる。画面に出たカードのサムネイル要求だけで
	// 通信が走らないよう、取り込むまでは配信しない。
	if entry.CloudOnly {
		return nil, fmt.Errorf("クラウド上にだけあるファイルのため配信しません")
	}
	return entry, nil
}

// contentTypeFor はエントリに対応する MIME タイプを返す。
//
// 拡張子ではなく、走査時に中身から判定した形式を基準にする。
// 中身が AVIF なのに名前が .jpg というファイルへ image/jpeg を返すと、
// ブラウザはその型を信じて描画を拒否し、表示できない画像になってしまう。
func contentTypeFor(entry *model.Entry) string {
	format := entry.Format
	if format == "" {
		format = entry.Ext
	}
	if t := meta.ContentType(format); t != "" {
		return t
	}
	if t := mime.TypeByExtension(format); t != "" {
		return t
	}
	return "application/octet-stream"
}
