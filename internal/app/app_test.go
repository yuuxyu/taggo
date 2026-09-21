package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

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

	a, err := New()
	if err != nil {
		t.Fatalf("アプリの初期化に失敗: %v", err)
	}
	t.Cleanup(func() { a.Shutdown(context.Background()) })

	if err := a.OpenFolder(root); err != nil {
		t.Fatalf("フォルダの読み込みに失敗: %v", err)
	}
	waitForScan(t, a, len(files))
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

func TestAssetHandlerServesEntriesOnly(t *testing.T) {
	a, root := newTestApp(t, map[string]string{
		"a.md": "---\ntags: [x]\n---\n\n# A\n",
	})
	h := NewAssetHandler(a)

	// 登録済みエントリは配信される。
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, PathFile+"?path="+filepath.Join(root, "a.md"), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("登録済みファイルが配信されない: %d %s", rec.Code, rec.Body.String())
	}

	// フォルダ外のファイルは配信しない。
	outside := filepath.Join(t.TempDir(), "secret.md")
	if err := os.WriteFile(outside, []byte("秘密"), 0o644); err != nil {
		t.Fatalf("ファイル作成に失敗: %v", err)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, PathFile+"?path="+outside, nil))
	if rec.Code == http.StatusOK {
		t.Fatal("開いているフォルダ外のファイルが配信されてしまった")
	}

	// 未登録の拡張子（走査対象外）も配信しない。
	unlisted := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(unlisted, []byte("対象外"), 0o644); err != nil {
		t.Fatalf("ファイル作成に失敗: %v", err)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, PathFile+"?path="+unlisted, nil))
	if rec.Code == http.StatusOK {
		t.Fatal("走査対象外のファイルが配信されてしまった")
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
	// 中身は AVIF、名前は .jpg。taggo がデコードできないので原寸配信へ落ちる。
	avif := "\x00\x00\x00\x20ftypavif\x00\x00\x00\x00avifmif1miaf"
	a, root := newTestApp(t, map[string]string{"実はavif.jpg": avif})

	rec := httptest.NewRecorder()
	NewAssetHandler(a).ServeHTTP(rec,
		httptest.NewRequest(http.MethodGet, PathThumb+"?path="+filepath.Join(root, "実はavif.jpg"), nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("配信に失敗: %d %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "image/avif" {
		t.Fatalf("中身に合った MIME タイプを返していない: %q", got)
	}
}

// TestEntryReportsFormatMismatch は、拡張子と中身の食い違いを
// エントリが伝えることを確かめる。UI はこれを使って理由を表示する。
func TestEntryReportsFormatMismatch(t *testing.T) {
	a, root := newTestApp(t, map[string]string{
		"実はavif.jpg": "\x00\x00\x00\x20ftypavif\x00\x00\x00\x00avifmif1miaf",
	})

	entry, err := a.Entry(filepath.Join(root, "実はavif.jpg"))
	if err != nil {
		t.Fatalf("エントリを取得できない: %v", err)
	}
	if entry.Format != ".avif" {
		t.Fatalf("実体の形式が伝わっていない: %q", entry.Format)
	}
	if entry.Writable {
		t.Fatal("扱えない形式は書き込み不可であるべき")
	}
	if entry.Err == "" {
		t.Fatal("理由が伝わっていない")
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
