package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yuuxyu/taggo/internal/model"
	"github.com/yuuxyu/taggo/internal/settings"
	"github.com/yuuxyu/taggo/internal/store"
)

// newTestApp はフォルダを 1 つ読み込み済みのアプリを返す。
// Wails の runtime は使えないので、イベント送信は ctx が nil のまま黙って捨てられる。
func newTestApp(t *testing.T, files map[string]string) (*App, string) {
	t.Helper()

	root := t.TempDir()
	for rel, content := range files {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("ディレクトリ作成に失敗: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("ファイル作成に失敗: %v", err)
		}
	}

	a, err := New(settings.Load(filepath.Join(t.TempDir(), "settings.json")))
	if err != nil {
		t.Fatalf("アプリの初期化に失敗: %v", err)
	}
	t.Cleanup(func() { a.Shutdown(context.Background()) })

	if err := a.OpenFolder(root); err != nil {
		t.Fatalf("フォルダの読み込みに失敗: %v", err)
	}
	notes := 0
	for rel := range files {
		if model.IsMarkdown(rel) {
			notes++
		}
	}
	waitForScan(t, a, notes)
	return a, root
}

// waitForScan は走査が終わって期待件数が載るまで待つ。
func waitForScan(t *testing.T, a *App, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if a.Status().EntryCount >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("走査が完了しない: %d 件しか載っていない", a.Status().EntryCount)
}

func TestOpenFolderAndSearch(t *testing.T) {
	a, _ := newTestApp(t, map[string]string{
		"a.md": "---\ntags: [golang, 設計]\n---\n\n# Go の設計\n",
		"b.md": "---\ntags: [rust]\n---\n\n# Rust\n",
	})

	st := a.Status()
	if st.EntryCount != 2 || st.TagCount != 3 {
		t.Fatalf("読み込み結果が想定外: %+v", st)
	}

	r, err := a.Search(store.SearchOptions{Query: "#golang"})
	if err != nil {
		t.Fatalf("検索に失敗: %v", err)
	}
	if r.Total != 1 {
		t.Fatalf("タグ検索の結果が想定外: %d 件", r.Total)
	}
}

func TestTagsAutocomplete(t *testing.T) {
	a, _ := newTestApp(t, map[string]string{
		"a.md": "---\ntags: [golang, go-routine]\n---\n\n# A\n",
		"b.md": "---\ntags: [golang]\n---\n\n# B\n",
	})

	got := a.Tags("go", 0)
	if len(got) != 2 {
		t.Fatalf("前方一致の候補が想定外: %+v", got)
	}
	if got[0].Tag != "golang" || got[0].Count != 2 {
		t.Fatalf("使用件数の多い順になっていない: %+v", got)
	}
}

func TestSetTagsWritesThroughToFile(t *testing.T) {
	a, root := newTestApp(t, map[string]string{
		"a.md": "---\ntags: [old]\ntitle: 保持されるべき\n---\n\n# A\n",
	})
	path := filepath.Join(root, "a.md")

	res := a.SetTags(path, []string{"新しい", "tag"})
	if !res.OK {
		t.Fatalf("タグ書き込みに失敗: %s", res.Error)
	}
	if len(res.Entry.Tags) != 2 {
		t.Fatalf("書き戻したタグが想定外: %v", res.Entry.Tags)
	}

	// 実ファイルにも反映され、他のフィールドは残っていること。
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ファイル読み込みに失敗: %v", err)
	}
	content := string(raw)
	for _, want := range []string{"新しい", "tag", "保持されるべき", "# A"} {
		if !contains(content, want) {
			t.Fatalf("ファイルに %q が無い:\n%s", want, content)
		}
	}

	// インメモリ DB も同期していること。
	r, _ := a.Search(store.SearchOptions{Query: "#新しい"})
	if r.Total != 1 {
		t.Fatalf("書き込み後の検索に反映されていない: %d 件", r.Total)
	}
}

func TestBulkAddAndRemoveTags(t *testing.T) {
	a, root := newTestApp(t, map[string]string{
		"a.md": "---\ntags: [共通]\n---\n\n# A\n",
		"b.md": "---\ntags: [共通]\n---\n\n# B\n",
	})
	paths := []string{filepath.Join(root, "a.md"), filepath.Join(root, "b.md")}

	for _, res := range a.AddTags(paths, []string{"一括"}) {
		if !res.OK {
			t.Fatalf("一括追加に失敗: %s", res.Error)
		}
	}
	r, _ := a.Search(store.SearchOptions{Query: "#一括"})
	if r.Total != 2 {
		t.Fatalf("一括追加が反映されていない: %d 件", r.Total)
	}

	for _, res := range a.RemoveTags(paths, []string{"共通"}) {
		if !res.OK {
			t.Fatalf("一括削除に失敗: %s", res.Error)
		}
	}
	r, _ = a.Search(store.SearchOptions{Query: "#共通"})
	if r.Total != 0 {
		t.Fatalf("一括削除が反映されていない: %d 件", r.Total)
	}
	// 追加したタグのほうは残っていること。
	r, _ = a.Search(store.SearchOptions{Query: "#一括"})
	if r.Total != 2 {
		t.Fatalf("一括削除で無関係なタグまで消えた: %d 件", r.Total)
	}
}

func TestSetTagsRejectsReadOnlyFile(t *testing.T) {
	a, root := newTestApp(t, map[string]string{
		"ro.md": "---\ntags: [x]\n---\n\n# 読み取り専用\n",
	})
	path := filepath.Join(root, "ro.md")

	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatalf("パーミッション変更に失敗: %v", err)
	}
	// 走査時点の Writable を更新するため、読み込み直す。
	if err := a.OpenFolder(root); err != nil {
		t.Fatalf("再読み込みに失敗: %v", err)
	}
	waitForScan(t, a, 1)

	res := a.SetTags(path, []string{"編集"})
	if res.OK {
		t.Fatal("読み取り専用ファイルへの書き込みが通ってしまった")
	}
	if res.Error == "" {
		t.Fatal("エラー理由が入っていない")
	}

	// 失敗時にメモリ上だけ更新されていないこと。
	r, _ := a.Search(store.SearchOptions{Query: "#編集"})
	if r.Total != 0 {
		t.Fatalf("書き込み失敗なのにメモリ上へ反映された: %d 件", r.Total)
	}
}

func TestMarkdownSourceStripsFrontMatter(t *testing.T) {
	a, root := newTestApp(t, map[string]string{
		"a.md": "---\ntags: [x]\n---\n\n# 本文の見出し\n\n本文。\n",
	})

	body, err := a.MarkdownSource(filepath.Join(root, "a.md"))
	if err != nil {
		t.Fatalf("本文取得に失敗: %v", err)
	}
	if contains(body, "tags:") {
		t.Fatalf("Front Matter が本文に残っている:\n%s", body)
	}
	if !contains(body, "# 本文の見出し") {
		t.Fatalf("本文が欠けている:\n%s", body)
	}
}

func TestRelatedPages(t *testing.T) {
	a, root := newTestApp(t, map[string]string{
		"目次.md":   "# 目次\n\n[メモ](./memo.md) からどうぞ。\n",
		"memo.md": "# メモ\n\n[目次](目次.md) を参照。\n",
	})

	got := a.RelatedPages(filepath.Join(root, "目次.md"))
	if len(got.Incoming) != 1 || got.Incoming[0].Title != "メモ" {
		t.Fatalf("バックリンクが取れていない: %+v", got.Incoming)
	}
	// 通常の Markdown リンクも関連ページとして扱う。
	if len(got.Outgoing) != 1 || got.Outgoing[0].Title != "メモ" {
		t.Fatalf("リンク先が取れていない: %+v", got.Outgoing)
	}
}

// pngHead は PNG の先頭 8 バイト。配信の可否は中身で決まるので、これだけで画像とみなされる。
var pngHead = string([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A})

// avifHead は AVIF の先頭。ftyp ボックスのブランドが avif になっている。
var avifHead = string([]byte{0, 0, 0, 0x20}) + "ftypavif" + string([]byte{0, 0, 0, 0}) + "avifmif1miaf"

// getImage は画像配信のエンドポイントへパスを渡し、応答を返す。
func getImage(a *App, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	NewAssetHandler(a).ServeHTTP(rec,
		httptest.NewRequest(http.MethodGet, PathImage+"?path="+url.QueryEscape(path), nil))
	return rec
}

// 本文に埋め込まれた画像は、一覧に載っていなくても開いているフォルダ配下なら配信すること。
func TestAssetHandlerServesEmbeddedImages(t *testing.T) {
	a, root := newTestApp(t, map[string]string{
		"a.md":                                 "# A\n\n![図](./img/a.png)\n",
		filepath.Join("img", "a.png"):          pngHead,
		filepath.Join(".attachments", "b.png"): pngHead,
	})

	// 画像は一覧（DB）には載らない。
	if n := a.Status().EntryCount; n != 1 {
		t.Fatalf("Markdown だけが読み込まれるはずが %d 件", n)
	}

	rec := getImage(a, filepath.Join(root, "img", "a.png"))
	if rec.Code != http.StatusOK {
		t.Fatalf("フォルダ内の画像が配信されない: %d %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("MIME タイプが違う: %q", got)
	}

	// 走査では飛ばす隠しフォルダの画像も、本文から参照されうるので配信する。
	if rec := getImage(a, filepath.Join(root, ".attachments", "b.png")); rec.Code != http.StatusOK {
		t.Fatalf("隠しフォルダの画像が配信されない: %d", rec.Code)
	}
}

// 画像でないファイルや、フォルダの外のファイルは配信しないこと。
func TestAssetHandlerRejectsOthers(t *testing.T) {
	a, root := newTestApp(t, map[string]string{
		"a.md":      "---\ntags: [x]\n---\n\n# A\n",
		"notes.txt": "対象外",
		"偽物.png":    "画像ではない",
	})

	for name, path := range map[string]string{
		"Markdown":   filepath.Join(root, "a.md"),
		"テキスト":       filepath.Join(root, "notes.txt"),
		"拡張子だけ画像":    filepath.Join(root, "偽物.png"),
		"存在しない":      filepath.Join(root, "無い.png"),
		"フォルダ":       root,
		"相対パスで外へ出る":  filepath.Join(root, "..", "secret.png"),
		"path クエリが空": "",
	} {
		t.Run(name, func(t *testing.T) {
			if rec := getImage(a, path); rec.Code == http.StatusOK {
				t.Fatalf("配信してはいけないファイルが配信された: %q", path)
			}
		})
	}

	outside := filepath.Join(t.TempDir(), "secret.png")
	if err := os.WriteFile(outside, []byte(pngHead), 0o644); err != nil {
		t.Fatalf("ファイル作成に失敗: %v", err)
	}
	if rec := getImage(a, outside); rec.Code == http.StatusOK {
		t.Fatal("開いているフォルダ外の画像が配信されてしまった")
	}
}

func TestWatcherSyncsExternalEdits(t *testing.T) {
	a, root := newTestApp(t, map[string]string{
		"a.md": "---\ntags: [初期]\n---\n\n# A\n",
	})
	path := filepath.Join(root, "a.md")

	// taggo を介さない外部からの書き換えが、インメモリ DB へ反映されること。
	if err := os.WriteFile(path, []byte("---\ntags: [外部編集]\n---\n\n# A\n"), 0o644); err != nil {
		t.Fatalf("外部編集の書き込みに失敗: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r, _ := a.Search(store.SearchOptions{Query: "#外部編集"})
		if r.Total == 1 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("外部編集がインメモリ DB へ反映されなかった")
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

// TestAssetHandlerContentTypeFollowsContent は、拡張子ではなく中身に合わせた
// MIME タイプを返すことを確かめる。嘘の型を返すとブラウザが描画を拒否し、
// 「画像を表示できません」になってしまう。
func TestAssetHandlerContentTypeFollowsContent(t *testing.T) {
	// 中身は AVIF、名前は .jpg。
	a, root := newTestApp(t, map[string]string{"a.md": "# A", "実はavif.jpg": avifHead})

	rec := getImage(a, filepath.Join(root, "実はavif.jpg"))
	if rec.Code != http.StatusOK {
		t.Fatalf("配信に失敗: %d %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "image/avif" {
		t.Fatalf("中身に合った MIME タイプを返していない: %q", got)
	}
}

// 同じ名前のノートが別のフォルダにあるとき、関連ページが混ざらないこと。
// README.md のようにありふれた名前で起きやすい。
func TestRelatedPagesDoNotMixSameNameNotes(t *testing.T) {
	a, root := newTestApp(t, map[string]string{
		filepath.Join("a", "note.md"):   "# メモ\n\n[README](./README.md) を見てください。\n",
		filepath.Join("a", "README.md"): "# A の説明\n",
		filepath.Join("b", "README.md"): "# B の説明\n",
	})

	here := a.RelatedPages(filepath.Join(root, "a", "README.md"))
	if len(here.Incoming) != 1 || here.Incoming[0].Title != "メモ" {
		t.Fatalf("同じフォルダの README にリンク元が付いていない: %+v", here.Incoming)
	}
	if got := a.RelatedPages(filepath.Join(root, "b", "README.md")); len(got.Incoming) != 0 {
		t.Fatalf("無関係なフォルダの README に関連が出ている: %+v", got.Incoming)
	}

	from := a.RelatedPages(filepath.Join(root, "a", "note.md"))
	if len(from.Outgoing) != 1 || from.Outgoing[0].Path != filepath.Join(root, "a", "README.md") {
		t.Fatalf("リンク先が同じフォルダの README になっていない: %+v", from.Outgoing)
	}
}

// newLimitedApp は上限を小さくしたアプリで、フォルダを 1 つ読み込む。
func newLimitedApp(t *testing.T, maxEntries int, files []string) (*App, string) {
	t.Helper()

	root := t.TempDir()
	for _, rel := range files {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("ディレクトリ作成に失敗: %v", err)
		}
		if err := os.WriteFile(path, []byte("---\ntags: [共通]\n---\n\n# "+rel+"\n"), 0o644); err != nil {
			t.Fatalf("ファイル作成に失敗: %v", err)
		}
	}

	a, err := New(settings.Load(filepath.Join(t.TempDir(), "settings.json")))
	if err != nil {
		t.Fatalf("アプリの初期化に失敗: %v", err)
	}
	a.maxEntries = maxEntries
	t.Cleanup(func() { a.Shutdown(context.Background()) })

	if err := a.OpenFolder(root); err != nil {
		t.Fatalf("フォルダの読み込みに失敗: %v", err)
	}
	waitForIdle(t, a)
	return a, root
}

// waitForIdle は進行中の走査が終わるまで待つ。
func waitForIdle(t *testing.T, a *App) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !a.Status().Scanning {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("走査が完了しない")
}

func TestLoadMoreReadsTheRest(t *testing.T) {
	a, _ := newLimitedApp(t, 2, []string{"a.md", "b.md", "c.md", "d.md", "e.md"})

	st := a.Status()
	if st.EntryCount != 2 || st.Remaining != 3 {
		t.Fatalf("初回の読み込みが想定外: %+v", st)
	}

	// 上限と同じ件数だけ続きを読む。
	if err := a.LoadMore(false); err != nil {
		t.Fatalf("続きの読み込みに失敗: %v", err)
	}
	waitForIdle(t, a)
	if st := a.Status(); st.EntryCount != 4 || st.Remaining != 1 {
		t.Fatalf("続きの読み込み結果が想定外: %+v", st)
	}

	// 残りをすべて読む。読んだ分はすべて検索できる。
	if err := a.LoadMore(true); err != nil {
		t.Fatalf("残りの読み込みに失敗: %v", err)
	}
	waitForIdle(t, a)
	if st := a.Status(); st.EntryCount != 5 || st.Remaining != 0 {
		t.Fatalf("残りの読み込み結果が想定外: %+v", st)
	}
	r, _ := a.Search(store.SearchOptions{Query: "#共通"})
	if r.Total != 5 {
		t.Fatalf("読み込んだ分が検索に出ない: %d 件", r.Total)
	}

	if err := a.LoadMore(false); err == nil {
		t.Fatal("読み切ったあとの続きの読み込みはエラーにすべき")
	}
}

func TestWatcherIgnoresFilesNotLoadedYet(t *testing.T) {
	a, root := newLimitedApp(t, 1, []string{"a.md", "b.md"})

	// まだ読み込んでいない範囲に作られたファイルは、一覧に紛れ込ませない。
	if !a.notLoadedYet(filepath.Join(root, "c.md")) {
		t.Fatal("再開位置より後ろのファイルを読み込み済みとみなしている")
	}
	// 読み込んだ範囲のファイルは、そのまま反映する。
	if a.notLoadedYet(filepath.Join(root, "a.md")) || a.notLoadedYet(filepath.Join(root, "0.md")) {
		t.Fatal("読み込み済みの範囲のファイルを除外している")
	}
}
