package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/yuuxyu/taggo/internal/atomicfile"
	"github.com/yuuxyu/taggo/internal/imagefile"
	"github.com/yuuxyu/taggo/internal/meta"
	"github.com/yuuxyu/taggo/internal/model"
)

// utf8BOM はファイルの先頭に付くことのある UTF-8 の BOM。
// 編集画面には渡さず、保存するときに元のファイルに付いていれば付け直す。
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// maxImageBytes は、編集画面に貼り付けた画像として保存する大きさの上限。
const maxImageBytes = 32 << 20

// closeTimeout は、ウィンドウを閉じるときに画面の保存を待つ時間の上限。
// 画面が応答しなくても、これを過ぎれば閉じる。
const closeTimeout = 5 * time.Second

// NoteContent は編集画面へ渡すノートの中身。
type NoteContent struct {
	// Content はファイルの中身。Front Matter も含めてそのまま渡す。先頭の BOM だけは除く。
	Content string `json:"content"`
	// Version は読んだときのファイルの中身を表す値。保存するときに渡してもらい、
	// その間にほかのアプリがファイルを書き換えていないかを確かめる。
	Version string `json:"version"`
}

// SaveResult は SaveNote の結果。
type SaveResult struct {
	// Conflict は、読んだあとでほかのアプリがファイルを書き換えた（または消した）ため、
	// 保存しなかったこと。
	Conflict bool `json:"conflict,omitempty"`
	// Version は保存したあとのファイルの中身を表す値。Conflict のときは今のファイルの値
	// （消えていれば空）。
	Version string `json:"version"`
	// Entry は保存したあとのエントリ。Conflict のときは nil。
	Entry *model.Entry `json:"entry,omitempty"`
}

// ReadNote は、編集画面で開くノートの中身を読む。
func (a *App) ReadNote(path string) (*NoteContent, error) {
	e, ok := a.store.Get(path)
	if !ok {
		return nil, fmt.Errorf("エントリが見つかりません: %s", path)
	}
	raw, err := readFileLimited(e.Path)
	if err != nil {
		return nil, err
	}
	return &NoteContent{Content: string(bytes.TrimPrefix(raw, utf8BOM)), Version: contentVersion(raw)}, nil
}

// SaveNote は、編集画面で書き換えたノートの中身 content を path へ保存する。
//
// baseVersion は ReadNote（か前回の SaveNote）で受け取った Version。今のファイルの中身が
// それと違えば、ほかのアプリでの変更を消さないよう書き込まずに Conflict を返す。
// baseVersion が空なら確かめずに上書きする（利用者が上書きを選んだとき）。
//
// 書き込みは同じフォルダの一時ファイルに書いてから置き換える。元のファイルの先頭に
// BOM があれば付け直す。保存したエントリはウォッチャーを待たずに一覧へ反映する。
func (a *App) SaveNote(path, content, baseVersion string) (*SaveResult, error) {
	if err := a.checkPagePath(path); err != nil {
		return nil, err
	}
	// ほかで消されたノートを、保存で生き返らせない。
	if _, ok := a.store.Get(path); !ok {
		return nil, fmt.Errorf("エントリが見つかりません: %s", path)
	}
	if len(content) > maxNoteBytes {
		return nil, fmt.Errorf("ノートが大きすぎて保存できません (%d バイト超)", maxNoteBytes)
	}

	a.writeMu.Lock()
	defer a.writeMu.Unlock()

	data := []byte(content)
	current, err := readFileLimited(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if baseVersion != "" {
			return &SaveResult{Conflict: true}, nil
		}
	case err != nil:
		return nil, fmt.Errorf("保存する前にノートを読めませんでした: %w", err)
	default:
		if baseVersion != "" && contentVersion(current) != baseVersion {
			return &SaveResult{Conflict: true, Version: contentVersion(current)}, nil
		}
		if bytes.HasPrefix(current, utf8BOM) {
			data = append(append([]byte{}, utf8BOM...), data...)
		}
		// 中身が変わらなければ書き込まない。同期サービスに無駄な更新を送らないため。
		if bytes.Equal(current, data) {
			e, _ := a.store.Get(path)
			return &SaveResult{Version: contentVersion(current), Entry: e}, nil
		}
	}

	if err := atomicfile.Replace(path, data); err != nil {
		return nil, fmt.Errorf("保存できませんでした: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("保存したノートを読めませんでした: %w", err)
	}
	entry, err := meta.Read(path, info)
	if err != nil {
		return nil, err
	}
	entry.RelPath = filepath.Base(path)
	if err := a.store.Put(entry); err != nil {
		return nil, err
	}
	a.emit(EventEntryChanged, map[string]any{"path": path, "entry": entry})
	return &SaveResult{Version: contentVersion(data), Entry: entry}, nil
}

// SaveImage は、編集画面に貼り付けた（ドロップした）画像を、開いているフォルダの直下に保存する。
// 戻り値は、notePath のノートの本文に書く画像のパス。
//
// data は画像のバイト列を Base64 で表したもの。表示できる形式の画像でなければ保存しない。
// 名前は保存した日時（20261011-153000.png）にし、同じ名前のファイルがあれば -2, -3 … を付ける。
// 拡張子は中身の形式から決める。
func (a *App) SaveImage(notePath, data string) (string, error) {
	if err := a.checkPagePath(notePath); err != nil {
		return "", err
	}
	tooLarge := fmt.Errorf("画像が大きすぎて保存できません (%d MB 超)", maxImageBytes>>20)
	// 大きすぎるものは、デコードしてメモリを使う前に断る。
	if base64.StdEncoding.DecodedLen(len(data)) > maxImageBytes+3 {
		return "", tooLarge
	}
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return "", errors.New("画像のデータを読み取れませんでした")
	}
	if len(raw) > maxImageBytes {
		return "", tooLarge
	}
	contentType, ok := imagefile.Sniff(raw)
	if !ok {
		return "", errors.New("表示できる形式（JPEG・PNG・GIF・WebP・AVIF・BMP・SVG）の画像ではありません")
	}
	return saveNewFile(a.store.Root(), time.Now().Format("20060102-150405"), imagefile.Ext(contentType), raw)
}

// saveNewFile は dir に base + ext の名前でファイルを作って data を書き込み、付けた名前を返す。
// 同じ名前のファイルがあれば上書きせず、base-2, base-3 … と名前を変える。
func saveNewFile(dir, base, ext string, data []byte) (string, error) {
	for i := 1; i <= 1000; i++ {
		name := base + ext
		if i > 1 {
			name = fmt.Sprintf("%s-%d%s", base, i, ext)
		}
		path := filepath.Join(dir, name)
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("画像を保存できませんでした: %w", err)
		}
		_, err = f.Write(data)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			os.Remove(path)
			return "", fmt.Errorf("画像を保存できませんでした: %w", err)
		}
		return name, nil
	}
	return "", errors.New("画像の名前を決められませんでした")
}

// TagNames は、本文に [[ と入力したときの補完に出すタグの候補を返す。
// 使われているタグと、ページのあるタグを、使われている件数の多い順に並べる。
func (a *App) TagNames() []string {
	return a.store.TagNames()
}

// contentVersion は、ファイルの中身を表す値を返す。保存の前に、読んだときから
// 書き換えられていないかを確かめるのに使う。
func contentVersion(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// OnBeforeClose は、ウィンドウを閉じる前に、画面へ書きかけのノートの保存を頼むフックを返す。
//
// 最初の 1 回は閉じるのを止めて EventBeforeClose を送る。画面は保存を終えてから終了を
// 呼び直し、2 回目はそのまま閉じる。画面が応答しなくても closeTimeout が過ぎれば閉じる。
//
// App のメソッドにしないのは、Wails が公開メソッドをフロントエンドへ公開してしまうため。
func OnBeforeClose(a *App) func(ctx context.Context) bool {
	return func(ctx context.Context) bool {
		a.mu.Lock()
		quitting := a.quitting
		a.quitting = true
		a.mu.Unlock()
		if quitting || a.ctx == nil {
			return false
		}
		a.emit(EventBeforeClose, nil)
		time.AfterFunc(closeTimeout, func() { runtime.Quit(ctx) })
		return true
	}
}
