package app

import (
	"context"
	"encoding/json"
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
	// 読み込むのはフォルダ直下のノートだけ。
	notes := 0
	for rel := range files {
		if model.IsMarkdown(rel) && filepath.Dir(rel) == "." {
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
		"a.md": "# Go の設計\n\n[[golang]] と [[設計]]\n",
		"b.md": "# Rust\n\n[[rust]]\n",
	})

	st := a.Status()
	if st.EntryCount != 2 || st.TagCount != 3 {
		t.Fatalf("読み込み結果が想定外: %+v", st)
	}

	r, err := a.Search(store.SearchOptions{Query: "golang"})
	if err != nil {
		t.Fatalf("検索に失敗: %v", err)
	}
	if r.Total != 1 {
		t.Fatalf("タグでの検索結果が想定外: %d 件", r.Total)
	}
}

// 先頭の「---」で囲んだブロック（ほかのツールの Front Matter）は、本文として見せないこと。
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

	// リンク先とリンク元が同じノートなので、最初のリンク元のグループにカードが 1 枚だけ出て、
	// リンク先のグループには重ねて出さない。
	got := a.RelatedPages(filepath.Join(root, "目次.md")).Groups
	if len(got) != 1 || got[0].Tag != "目次" || len(got[0].Pages) != 1 || got[0].Pages[0].Title != "メモ" {
		t.Fatalf("リンクでつながるノートが取れていない: %+v", got)
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
		"a.md":      "# A\n",
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
		"a.md": "# A\n\n[[初期]]\n",
	})
	path := filepath.Join(root, "a.md")

	// taggo を介さない外部からの書き換えが、インメモリ DB へ反映されること。
	if err := os.WriteFile(path, []byte("# A\n\n[[外部編集]]\n"), 0o644); err != nil {
		t.Fatalf("外部編集の書き込みに失敗: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r, _ := a.Search(store.SearchOptions{Query: "外部編集"})
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

// サブフォルダのノートは読み込まないこと。
func TestSubfoldersAreIgnored(t *testing.T) {
	a, root := newTestApp(t, map[string]string{
		"a.md":                        "# A\n",
		filepath.Join("sub", "b.md"):  "# B\n",
		filepath.Join(".git", "c.md"): "# C\n",
	})
	waitForIdle(t, a)
	if st := a.Status(); st.EntryCount != 1 {
		t.Fatalf("サブフォルダのノートまで読み込んでいる: %+v", st)
	}
	if _, err := a.Entry(filepath.Join(root, "sub", "b.md")); err == nil {
		t.Fatal("サブフォルダのノートが一覧に載っている")
	}
}

// 本文の [[タグ]] がタグとして検索・関連ページに出ること。
// Front Matter の tags: はタグにならないこと。
func TestBodyTags(t *testing.T) {
	a, root := newTestApp(t, map[string]string{
		"golang.md": "# Go 言語\n",
		"body.md":   "# 本文で付けた\n\n[[golang]] の話。\n",
		"front.md":  "---\ntags: [golang]\n---\n\n# Front Matter で付けた\n",
	})

	// 検索語がタグの名前と一致するので、そのタグのページが先頭に来る。
	r, err := a.Search(store.SearchOptions{Query: "golang", Sort: store.SortNameAsc})
	if err != nil {
		t.Fatal(err)
	}
	if r.Total != 2 || r.Head != 1 || r.Entries[0].Path != filepath.Join(root, "golang.md") ||
		r.Entries[1].Name != "body.md" {
		t.Fatalf("検索結果が違う: head=%d %v", r.Head, names(r))
	}

	// タグのページから見ると、本文で [[golang]] と書いたノートだけがリンク元になる。
	groups := a.RelatedPages(filepath.Join(root, "golang.md")).Groups
	if len(groups) != 1 || groups[0].Tag != "golang" || len(groups[0].Pages) != 1 ||
		groups[0].Pages[0].Path != filepath.Join(root, "body.md") {
		t.Fatalf("リンク元が違う: %+v", groups)
	}
}

// まだ無いページは、ファイルを作らずに開けること。そのページを指しているノートは関連ページに出ること。
func TestMissingPages(t *testing.T) {
	a, root := newTestApp(t, map[string]string{
		"a.md": "# A\n\n[[まだ無い]] と [リンク](./書きかけ.md)\n",
		"b.md": "# B\n\n[[まだ無い]]\n",
	})
	from := filepath.Join(root, "a.md")

	page, err := a.TagPage(" まだ無い ")
	if err != nil {
		t.Fatalf("タグのページを開けない: %v", err)
	}
	want := filepath.Join(root, "まだ無い.md")
	if !page.Missing || page.Path != want || page.Title != "まだ無い" {
		t.Fatalf("まだ無いタグのページが違う: %+v", page)
	}
	if _, err := os.Stat(want); err == nil {
		t.Fatal("開いただけでファイルを作った")
	}
	groups := a.RelatedPages(want).Groups
	if len(groups) != 1 || groups[0].Tag != "まだ無い" || len(groups[0].Pages) != 2 {
		t.Fatalf("まだ無いページのリンク元が違う: %+v", groups)
	}

	linked, err := a.LinkedPage(from, "./書きかけ.md")
	if err != nil || !linked.Missing || linked.Path != filepath.Join(root, "書きかけ.md") {
		t.Fatalf("まだ無いリンク先が違う: %+v %v", linked, err)
	}
	if groups := a.RelatedPages(linked.Path).Groups; len(groups) != 1 || groups[0].Pages[0].Path != from {
		t.Fatalf("リンク先のページのリンク元が違う: %+v", groups)
	}

	// 読み込み済みのページは、そのエントリが返る。大文字小文字と拡張子の違いは区別しない。
	if got, err := a.LinkedPage(from, "./B.markdown"); err != nil || got.Missing || got.Name != "b.md" {
		t.Fatalf("既にあるページが返らない: %+v %v", got, err)
	}
	if got, err := a.TagPage("A"); err != nil || got.Missing || got.Name != "a.md" {
		t.Fatalf("既にあるタグのページが返らない: %+v %v", got, err)
	}

	for name, open := range map[string]func() error{
		"フォルダの外":      func() error { _, err := a.LinkedPage(from, "../outside.md"); return err },
		"サブフォルダ":      func() error { _, err := a.LinkedPage(from, "./sub/x.md"); return err },
		"Markdown 以外": func() error { _, err := a.LinkedPage(from, "./script.bat"); return err },
		"未登録のリンク元":    func() error { _, err := a.LinkedPage(filepath.Join(root, "none.md"), "./x.md"); return err },
		"ファイル名にできない":  func() error { _, err := a.TagPage("a/b"); return err },
	} {
		if open() == nil {
			t.Errorf("%s: エラーにならない", name)
		}
	}
	if groups := a.RelatedPages(filepath.Join(root, "sub", "x.md")).Groups; len(groups) != 0 {
		t.Fatalf("扱わない場所のページに関連ページが出ている: %+v", groups)
	}
}

// ピン留めはフォルダ直下の .taggo.json に書き込み、検索語が無いときの一覧の先頭に並ぶこと。
// 設定ファイルにある taggo の知らない項目は残すこと。
func TestSetPinnedWritesFolderConfig(t *testing.T) {
	a, root := newTestApp(t, map[string]string{
		"a.md":        "# A\n",
		"b.md":        "# B\n",
		"c.md":        "# C\n",
		".taggo.json": `{"memo": "残す"}`,
	})
	for _, name := range []string{"c.md", "a.md", "c.md"} {
		if err := a.SetPinned(filepath.Join(root, name), true); err != nil {
			t.Fatalf("ピン留めに失敗: %v", err)
		}
	}

	r, _ := a.Search(store.SearchOptions{Sort: store.SortNameAsc})
	if r.Head != 2 || r.Entries[0].Name != "c.md" || r.Entries[1].Name != "a.md" || r.Entries[2].Name != "b.md" {
		t.Fatalf("ピン留めした順に先頭へ並んでいない: head=%d %v", r.Head, names(r))
	}

	raw, err := os.ReadFile(filepath.Join(root, ".taggo.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		Memo   string   `json:"memo"`
		Pinned []string `json:"pinned"`
	}
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatalf("設定ファイルが JSON でない: %v\n%s", err, raw)
	}
	if saved.Memo != "残す" || len(saved.Pinned) != 2 || saved.Pinned[0] != "c.md" || saved.Pinned[1] != "a.md" {
		t.Fatalf("設定ファイルの中身が違う: %s", raw)
	}

	if err := a.SetPinned(filepath.Join(root, "c.md"), false); err != nil {
		t.Fatalf("ピン留めの解除に失敗: %v", err)
	}
	r, _ = a.Search(store.SearchOptions{Sort: store.SortNameAsc})
	if r.Head != 1 || r.Entries[0].Name != "a.md" {
		t.Fatalf("ピン留めを外せていない: head=%d %v", r.Head, names(r))
	}
}

// 設定ファイルが外で書き換えられたら、ピン留めを読み直すこと。
// 壊れた設定ファイルは書き換えないこと。
func TestPinsFollowConfigFile(t *testing.T) {
	a, root := newTestApp(t, map[string]string{"a.md": "# A\n", "b.md": "# B\n"})
	config := filepath.Join(root, ".taggo.json")

	if err := os.WriteFile(config, []byte(`{"pinned": ["B.md"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		r, _ := a.Search(store.SearchOptions{Sort: store.SortNameAsc})
		if r.Head == 1 && r.Entries[0].Name == "b.md" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("外で書き換えたピン留めが反映されない: head=%d %v", r.Head, names(r))
		}
		time.Sleep(20 * time.Millisecond)
	}

	broken := []byte(`{"pinned": [`)
	if err := os.WriteFile(config, broken, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := a.SetPinned(filepath.Join(root, "a.md"), true); err == nil {
		t.Fatal("壊れた設定ファイルへの書き込みが通ってしまった")
	}
	if got, _ := os.ReadFile(config); string(got) != string(broken) {
		t.Fatalf("壊れた設定ファイルを書き換えた: %s", got)
	}
}

func names(r store.Result) []string {
	out := make([]string, len(r.Entries))
	for i, e := range r.Entries {
		out[i] = e.Name
	}
	return out
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
		if err := os.WriteFile(path, []byte("# "+rel+"\n\n[[共通]]\n"), 0o644); err != nil {
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
	r, _ := a.Search(store.SearchOptions{Query: "共通"})
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
