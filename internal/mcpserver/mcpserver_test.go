package mcpserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yuuxyu/taggo/internal/settings"
)

// note はテスト用のノート 1 つ。daysAgo で更新日時を決める。
type note struct {
	name    string
	content string
	daysAgo int
}

// writeNotes はフォルダにノートを置く。
func writeNotes(t *testing.T, root string, notes ...note) {
	t.Helper()
	for _, n := range notes {
		path := filepath.Join(root, n.name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(n.content), 0o644); err != nil {
			t.Fatal(err)
		}
		mod := time.Now().AddDate(0, 0, -n.daysAgo)
		if err := os.Chtimes(path, mod, mod); err != nil {
			t.Fatal(err)
		}
	}
}

// openLibrary はフォルダを読み込み終えた library を返す。
func openLibrary(t *testing.T, root string, limit int) *library {
	t.Helper()
	lib, err := newLibrary()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(lib.close)
	lib.open(root, limit)
	if err := lib.wait(context.Background()); err != nil {
		t.Fatalf("フォルダを読み込めない: %v", err)
	}
	return lib
}

// connect は library を読むサーバー（書き込むツールも出す）にクライアントをつなぐ。
func connect(t *testing.T, lib *library) *mcp.ClientSession {
	t.Helper()
	return connectWith(t, lib, true)
}

// connectWith は、writable が false なら --read-only と同じサーバーにつなぐ。
func connectWith(t *testing.T, lib *library, writable bool) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := newServer(lib, writable).Connect(ctx, serverTransport, nil); err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil)
	cs, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

// newSession はノートを置いたフォルダを読み込み、クライアントをつなぐ。
func newSession(t *testing.T, notes ...note) (*mcp.ClientSession, string) {
	t.Helper()
	root := t.TempDir()
	writeNotes(t, root, notes...)
	return connect(t, openLibrary(t, root, 1000)), root
}

// call はツールを呼び、構造化した結果を Out に読む。ツールがエラーを返したらテストを落とす。
func call[Out any](t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) (Out, *mcp.CallToolResult) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s を呼べない: %v", name, err)
	}
	if res.IsError {
		t.Fatalf("%s がエラーを返した: %s", name, resultText(res))
	}
	var out Out
	data, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("%s の結果を読めない: %v\n%s", name, err, data)
	}
	return out, res
}

// callError はツールを呼び、エラーの文を返す。エラーにならなければテストを落とす。
func callError(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) string {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s を呼べない: %v", name, err)
	}
	if !res.IsError {
		t.Fatalf("%s がエラーにならなかった: %s", name, resultText(res))
	}
	return resultText(res)
}

// resultText は結果のテキストをつなげる。
func resultText(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

func summaryNames(notes []NoteSummary) []string {
	out := make([]string, len(notes))
	for i, n := range notes {
		out[i] = n.Name
	}
	return out
}

// annotation はツールの注釈を、比べやすい文字列にする。
func annotation(a *mcp.ToolAnnotations) string {
	if a == nil {
		return "注釈なし"
	}
	if a.ReadOnlyHint {
		return "読むだけ"
	}
	s := "書き込む"
	if a.DestructiveHint != nil && *a.DestructiveHint {
		s += "・書き換える"
	}
	if a.IdempotentHint {
		s += "・何度呼んでも同じ"
	}
	return s
}

// 書き込めるときは 10 のツールが、--read-only のときは読む 6 つだけがそろい、それぞれの注釈を持つこと。
func TestToolsAndAnnotations(t *testing.T) {
	root := t.TempDir()
	lib := openLibrary(t, root, 1000)
	readTools := map[string]string{
		"get_overview": "読むだけ", "list_notes": "読むだけ", "search_notes": "読むだけ",
		"read_note": "読むだけ", "get_backlinks": "読むだけ", "list_tags": "読むだけ",
	}
	writeTools := map[string]string{
		"create_note":    "書き込む",
		"add_tags":       "書き込む・何度呼んでも同じ",
		"append_to_note": "書き込む",
		"edit_note":      "書き込む・書き換える",
	}

	for _, writable := range []bool{true, false} {
		res, err := connectWith(t, lib, writable).ListTools(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]string{}
		for _, tool := range res.Tools {
			got[tool.Name] = annotation(tool.Annotations)
			if tool.Annotations != nil && (tool.Annotations.OpenWorldHint == nil || *tool.Annotations.OpenWorldHint) {
				t.Errorf("%s が外と通信する注釈になっている", tool.Name)
			}
		}
		want := maps.Clone(readTools)
		if writable {
			maps.Copy(want, writeTools)
		}
		if !maps.Equal(got, want) {
			t.Errorf("writable=%v のツールが違う:\n got  %v\n want %v", writable, got, want)
		}
	}
}

// 概要に、数・ピン留め・よく使うタグ・最近のノートが入ること。読み込みの上限も知らせること。
func TestOverview(t *testing.T) {
	root := t.TempDir()
	writeNotes(t, root,
		note{"a.md", "# A\n\n[[golang]] [[設計]]\n", 2},
		note{"b.md", "# B\n\n[[golang]]\n", 1},
		note{"golang.md", "# Go\n", 3},
		note{".taggo.json", `{"pinned":["b.md"]}`, 0},
	)
	cs := connect(t, openLibrary(t, root, 1000))

	got, _ := call[overviewOutput](t, cs, "get_overview", nil)
	if got.Folder != filepath.Base(root) || got.NoteCount != 3 || got.TagCount != 2 || got.NotLoaded != 0 {
		t.Fatalf("数が違う: %+v", got)
	}
	if names := summaryNames(got.Pinned); !slices.Equal(names, []string{"b.md"}) || !got.Pinned[0].Pinned {
		t.Fatalf("ピン留めが違う: %+v", got.Pinned)
	}
	if len(got.TopTags) != 2 || got.TopTags[0].Tag != "golang" || got.TopTags[0].Count != 2 ||
		got.TopTags[0].Page == nil || *got.TopTags[0].Page != "golang.md" || got.TopTags[1].Page != nil {
		t.Fatalf("よく使うタグが違う: %+v", got.TopTags)
	}
	if names := summaryNames(got.Recent); !slices.Equal(names, []string{"b.md", "a.md", "golang.md"}) {
		t.Fatalf("最近のノートが違う: %v", names)
	}

	// 上限で読まなかったノートの数を知らせる。
	limited := connect(t, openLibrary(t, root, 1))
	if got, _ := call[overviewOutput](t, limited, "get_overview", nil); got.NoteCount != 1 || got.NotLoaded != 2 {
		t.Fatalf("読み込みの上限が知らされていない: %+v", got)
	}
}

// タグで絞ると、そのタグを付けたノートだけが並び、タグのページは別に返ること。
func TestListNotesByTag(t *testing.T) {
	cs, _ := newSession(t,
		note{"Go 言語.md", "# Go 言語\n\n[[go 言語]] 自身にも付けている\n", 5},
		note{"old.md", "# 古い\n\n[[Go 言語]]\n", 10},
		note{"new.md", "# 新しい\n\n[[go  言語]]\n", 1},
		note{"other.md", "# 無関係\n\n[[rust]]\n", 0},
	)

	got, _ := call[listNotesOutput](t, cs, "list_notes", map[string]any{"tag": "#GO 言語"})
	if got.TagPage == nil || got.TagPage.Name != "Go 言語.md" {
		t.Fatalf("タグのページが返っていない: %+v", got.TagPage)
	}
	if names := summaryNames(got.Notes); !slices.Equal(names, []string{"new.md", "old.md"}) || got.Total != 2 {
		t.Fatalf("タグを付けたノートが違う: %v (total %d)", names, got.Total)
	}

	after := time.Now().AddDate(0, 0, -3).Format(time.DateOnly)
	got, _ = call[listNotesOutput](t, cs, "list_notes", map[string]any{"tag": "go 言語", "modified_after": after})
	if names := summaryNames(got.Notes); !slices.Equal(names, []string{"new.md"}) {
		t.Fatalf("更新日時で絞れていない: %v", names)
	}

	all, _ := call[listNotesOutput](t, cs, "list_notes", map[string]any{"sort": "name_asc", "limit": 2})
	if names := summaryNames(all.Notes); !slices.Equal(names, []string{"Go 言語.md", "new.md"}) || all.Total != 4 ||
		all.NextOffset == nil || *all.NextOffset != 2 {
		t.Fatalf("すべてのノートの一覧が違う: %v (total %d, next %v)", names, all.Total, all.NextOffset)
	}
	rest, _ := call[listNotesOutput](t, cs, "list_notes", map[string]any{"sort": "name_asc", "limit": 2, "offset": 2})
	if names := summaryNames(rest.Notes); !slices.Equal(names, []string{"old.md", "other.md"}) || rest.NextOffset != nil {
		t.Fatalf("続きが違う: %v (next %v)", names, rest.NextOffset)
	}

	if msg := callError(t, cs, "list_notes", map[string]any{"modified_after": "先週"}); !strings.Contains(msg, "日時として読めません") {
		t.Fatalf("日時の誤りが伝わらない: %s", msg)
	}
}

// 本文全体を探し、見つかった行をファイルの行番号で返すこと。
func TestSearchNotes(t *testing.T) {
	cs, _ := newSession(t,
		note{"long.md", "# 長いノート\n\n" + strings.Repeat("前置き。", 100) + "\n\nここで goroutine の止め方を書く。\n", 3},
		note{"fm.md", "---\ntitle: x\n---\n# Front Matter のあるノート\n\nchannel と goroutine\n", 2},
		note{"code.md", "# コード\n\n```go\nselect {}\n```\n", 1},
		note{"crlf.md", "# CRLF\r\n\r\nWindows の改行\r\n", 1},
		note{"wide.md", "# 長い行\n\n" + strings.Repeat("あ", 100) + "目印" + strings.Repeat("い", 100) + "\n", 1},
		note{"golang.md", "# Go\n", 5},
		note{"a.md", "# golang の使い方\n", 0},
	)

	search := func(query string) searchOutput {
		t.Helper()
		got, _ := call[searchOutput](t, cs, "search_notes", map[string]any{"query": query})
		return got
	}
	hit := func(got searchOutput, name string) searchHit {
		t.Helper()
		for _, h := range got.Results {
			if h.Name == name {
				return h
			}
		}
		t.Fatalf("%s が見つからない: %+v", name, got.Results)
		return searchHit{}
	}

	// 抜粋（240 文字）より後ろの本文も探す。
	if h := hit(search("goroutine"), "long.md"); len(h.Matches) != 1 || h.Matches[0].Line != 5 ||
		!strings.Contains(h.Matches[0].Text, "goroutine の止め方") {
		t.Fatalf("抜粋より後ろの語の行が違う: %+v", h.Matches)
	}
	// 語はすべて含むノートだけ。行番号は Front Matter を含めたファイルの行番号。
	if got := search("goroutine channel"); len(got.Results) != 1 || got.Results[0].Name != "fm.md" || got.Results[0].Matches[0].Line != 6 {
		t.Fatalf("複数の語の検索が違う: %+v", got.Results)
	}
	if h := hit(search("SELECT"), "code.md"); h.Matches[0].Text != "select {}" {
		t.Fatalf("コードブロックの中を探していない: %+v", h.Matches)
	}
	if h := hit(search("改行"), "crlf.md"); h.Matches[0] != (lineMatch{Line: 3, Text: "Windows の改行"}) {
		t.Fatalf("CRLF の行が違う: %+v", h.Matches)
	}
	if h := hit(search("目印"), "wide.md"); !strings.Contains(h.Matches[0].Text, "目印") ||
		!strings.HasPrefix(h.Matches[0].Text, "…") || !strings.HasSuffix(h.Matches[0].Text, "…") {
		t.Fatalf("長い行を語の前後で切り詰めていない: %q", h.Matches[0].Text)
	}
	// 検索語と同じ名前のタグのページを先頭に出す。
	if got := search("golang"); len(got.Results) < 2 || got.Results[0].Name != "golang.md" || got.Results[1].Name != "a.md" {
		t.Fatalf("タグのページが先頭にない: %+v", got.Results)
	}

	if msg := callError(t, cs, "search_notes", map[string]any{"query": "  "}); !strings.Contains(msg, "list_notes") {
		t.Fatalf("空の検索語の扱いが違う: %s", msg)
	}
}

// ノートの本文とタグ・リンク・リンク元を返すこと。ファイル名は表記の違いを区別せずに解決すること。
func TestReadNote(t *testing.T) {
	raw := "# Go\r\n\r\n[[言語]] [[未整理]]\r\n\r\n[実装](impl.md) と [予定](todo.md)\r\n"
	bom := "\ufeffBOM 付きのノート\r\n"
	cs, _ := newSession(t,
		note{"Golang.md", raw, 1},
		note{"impl.md", "# 実装\n\n[[golang]]\n", 1},
		note{"言語.md", "# 言語\n", 1},
		note{"bom.md", bom, 1},
	)

	got, res := call[readNoteOutput](t, cs, "read_note", map[string]any{"note": "golang"})
	sum := sha256.Sum256([]byte(raw))
	if got.Name != "Golang.md" || !got.Exists || got.Title != "Go" || got.Version != hex.EncodeToString(sum[:]) ||
		got.BacklinkCount != 1 || got.Truncated {
		t.Fatalf("ノートの情報が違う: %+v", got)
	}
	pages := map[string]*string{}
	for _, tag := range got.Tags {
		pages[tag.Tag] = tag.Page
	}
	if len(got.Tags) != 2 || pages["言語"] == nil || *pages["言語"] != "言語.md" || pages["未整理"] != nil {
		t.Fatalf("タグが違う: %+v", got.Tags)
	}
	if len(got.Links) != 2 || *got.Links[0].Note != "impl.md" || got.Links[1].Note != nil {
		t.Fatalf("リンクが違う: %+v", got.Links)
	}
	// 本文は改行を LF にそろえたテキストで返す。
	if body := res.Content[1].(*mcp.TextContent).Text; body != "# Go\n\n[[言語]] [[未整理]]\n\n[実装](impl.md) と [予定](todo.md)\n" {
		t.Fatalf("本文が違う: %q", body)
	}
	// BOM は本文に含めないが、版はファイルの中身そのもの（BOM を含む）から作る。
	bomGot, res := call[readNoteOutput](t, cs, "read_note", map[string]any{"note": "bom"})
	bomSum := sha256.Sum256([]byte(bom))
	if body := res.Content[1].(*mcp.TextContent).Text; body != "BOM 付きのノート\n" || bomGot.Version != hex.EncodeToString(bomSum[:]) {
		t.Fatalf("BOM 付きのノートが違う: %q %+v", body, bomGot)
	}

	// まだ無いページは、エラーにせず、指しているノートの数を返す。
	if got, _ := call[readNoteOutput](t, cs, "read_note", map[string]any{"note": "未整理"}); got.Exists || got.BacklinkCount != 1 {
		t.Fatalf("まだ無いページの扱いが違う: %+v", got)
	}
	for _, bad := range []string{"無い", "../secret.md", `sub\a.md`, "a.txt"} {
		callError(t, cs, "read_note", map[string]any{"note": bad})
	}
}

// 長いノートは区切って返し、続きを読めること。
func TestReadNoteInChunks(t *testing.T) {
	saved := readChunkRunes
	readChunkRunes = 5
	t.Cleanup(func() { readChunkRunes = saved })

	cs, _ := newSession(t, note{"long.md", "あいうえおかきくけこさ", 0})
	first, res := call[readNoteOutput](t, cs, "read_note", map[string]any{"note": "long"})
	if !first.Truncated || first.NextOffset == nil || *first.NextOffset != 5 || res.Content[1].(*mcp.TextContent).Text != "あいうえお" {
		t.Fatalf("1 回目が違う: %+v", first)
	}
	last, res := call[readNoteOutput](t, cs, "read_note", map[string]any{"note": "long", "offset": 10})
	if last.Truncated || last.NextOffset != nil || res.Content[1].(*mcp.TextContent).Text != "さ" {
		t.Fatalf("最後が違う: %+v %q", last, res.Content[1].(*mcp.TextContent).Text)
	}
}

// リンク元を、タグで指しているかリンクで指しているかと一緒に返すこと。
func TestBacklinks(t *testing.T) {
	cs, _ := newSession(t,
		note{"golang.md", "# Go\n", 9},
		note{"tagged.md", "# タグで指す\n\n[[golang]]\n", 3},
		note{"linked.md", "# リンクで指す\n\n[Go](golang.md)\n", 2},
		note{"both.md", "# 両方\n\n[[Golang]] と [Go](./golang.md)\n", 1},
		note{"draft.md", "# 下書き\n\n[[未整理]]\n", 0},
	)

	got, _ := call[backlinksOutput](t, cs, "get_backlinks", map[string]any{"note": "golang.md"})
	via := map[string][]string{}
	for _, b := range got.Backlinks {
		via[b.Name] = b.Via
	}
	if got.Total != 3 || got.Tag != "golang" || !got.Exists ||
		!slices.Equal(via["both.md"], []string{"tag", "link"}) ||
		!slices.Equal(via["tagged.md"], []string{"tag"}) ||
		!slices.Equal(via["linked.md"], []string{"link"}) {
		t.Fatalf("リンク元が違う: %+v", got)
	}

	page, _ := call[backlinksOutput](t, cs, "get_backlinks", map[string]any{"note": "golang", "limit": 1})
	if len(page.Backlinks) != 1 || page.Total != 3 || page.NextOffset == nil {
		t.Fatalf("件数で区切れていない: %+v", page)
	}

	missing, _ := call[backlinksOutput](t, cs, "get_backlinks", map[string]any{"note": "未整理"})
	if missing.Exists || missing.Total != 1 || missing.Backlinks[0].Name != "draft.md" {
		t.Fatalf("まだ無いページのリンク元が違う: %+v", missing)
	}
	callError(t, cs, "get_backlinks", map[string]any{"note": "どこにも無い"})
}

// タグを、件数とページの有無と一緒に返し、ページの無いタグだけにも絞れること。
func TestListTags(t *testing.T) {
	cs, _ := newSession(t,
		note{"a.md", "# A\n\n[[golang]] [[設計]] [[a/b]]\n", 0},
		note{"b.md", "# B\n\n[[golang]] [[設計]]\n", 0},
		note{"c.md", "# C\n\n[[golang]]\n", 0},
		note{"golang.md", "# Go\n", 0},
		note{"孤立.md", "# 誰も使っていない\n", 0},
	)

	all, _ := call[listTagsOutput](t, cs, "list_tags", nil)
	if all.Total != 3 || all.Tags[0].Tag != "golang" || all.Tags[0].Count != 3 || *all.Tags[0].Page != "golang.md" {
		t.Fatalf("タグの一覧が違う: %+v", all.Tags)
	}

	// ページの無いタグの候補には、ページを作れないタグ（a/b）と、使われていないタグは入れない。
	missing, _ := call[listTagsOutput](t, cs, "list_tags", map[string]any{"missing_page_only": true})
	if len(missing.Tags) != 1 || missing.Tags[0].Tag != "設計" || missing.Tags[0].Page != nil {
		t.Fatalf("ページの無いタグが違う: %+v", missing.Tags)
	}

	byName, _ := call[listTagsOutput](t, cs, "list_tags", map[string]any{"query": "G", "sort": "name_asc"})
	if len(byName.Tags) != 1 || byName.Tags[0].Tag != "golang" {
		t.Fatalf("名前で絞れていない: %+v", byName.Tags)
	}
}

// プロンプトに、引数と、サーバーで数えた日付が入ること。
func TestPrompts(t *testing.T) {
	saved := now
	now = func() time.Time { return time.Date(2026, 11, 23, 10, 0, 0, 0, time.Local) }
	t.Cleanup(func() { now = saved })

	cs, _ := newSession(t)
	ctx := context.Background()

	res, err := cs.GetPrompt(ctx, &mcp.GetPromptParams{Name: "summarize_tag", Arguments: map[string]string{"tag": "golang"}})
	if err != nil {
		t.Fatal(err)
	}
	if text := res.Messages[0].Content.(*mcp.TextContent).Text; !strings.Contains(text, "list_notes（tag=golang）") || !strings.Contains(text, "golang.md") {
		t.Fatalf("タグが入っていない: %s", text)
	}

	res, err = cs.GetPrompt(ctx, &mcp.GetPromptParams{Name: "review_recent", Arguments: map[string]string{"days": "3"}})
	if err != nil {
		t.Fatal(err)
	}
	if text := res.Messages[0].Content.(*mcp.TextContent).Text; !strings.Contains(text, "modified_after=2026-11-20") {
		t.Fatalf("日付が入っていない: %s", text)
	}

	if _, err := cs.GetPrompt(ctx, &mcp.GetPromptParams{Name: "review_recent", Arguments: map[string]string{"days": "先週"}}); err == nil {
		t.Fatal("読めない日数で、エラーにならなかった")
	}
}

// 読むフォルダは --folder、設定の起動時に開くフォルダの順で決まること。
func TestChooseFolder(t *testing.T) {
	fixed := settings.Default()
	fixed.StartupMode, fixed.StartupFolder = settings.StartupFixed, `C:\notes`
	last := settings.Default()
	last.StartupMode, last.LastFolder = settings.StartupLast, `C:\last`
	neverOpened := settings.Default()
	neverOpened.StartupMode = settings.StartupLast

	tests := []struct {
		flag string
		cfg  settings.Settings
		want string
		err  error
	}{
		{`C:\flag`, fixed, `C:\flag`, nil},
		{"", fixed, `C:\notes`, nil},
		{"", last, `C:\last`, nil},
		{"", neverOpened, "", errNoLastFolder},
		{"", settings.Default(), "", errNoFolder},
	}
	for _, tt := range tests {
		got, err := chooseFolder(tt.flag, tt.cfg)
		if got != tt.want || !errors.Is(err, tt.err) {
			t.Errorf("chooseFolder(%q, %s) = %q, %v; want %q, %v", tt.flag, tt.cfg.StartupMode, got, err, tt.want, tt.err)
		}
	}
}

// 読むフォルダが決まらなくてもサーバーは動き、ツールの結果で理由を伝えること。
func TestFolderNotChosen(t *testing.T) {
	lib, err := newLibrary()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(lib.close)
	lib.fail(errNoFolder)
	cs := connect(t, lib)

	if msg := callError(t, cs, "list_notes", nil); !strings.Contains(msg, "起動時に開くフォルダ") {
		t.Fatalf("理由が伝わらない: %s", msg)
	}

	missing, err := newLibrary()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(missing.close)
	missing.open(filepath.Join(t.TempDir(), "無いフォルダ"), 1000)
	if msg := callError(t, connect(t, missing), "get_overview", nil); !strings.Contains(msg, "見つかりません") {
		t.Fatalf("フォルダが無いことが伝わらない: %s", msg)
	}
}

// 走査が終わる前に呼ばれたら、待ちきれなければ読み込み中であることを伝えること。
func TestWaitWhileLoading(t *testing.T) {
	lib, err := newLibrary()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(lib.close)
	lib.waitTimeout = 10 * time.Millisecond
	if err := lib.wait(context.Background()); err == nil || !strings.Contains(err.Error(), "読み込んでいます") {
		t.Fatalf("読み込み中であることが伝わらない: %v", err)
	}
}

// 画面やほかのアプリでの変更を、ウォッチャーで拾うこと。
func TestPicksUpChanges(t *testing.T) {
	cs, root := newSession(t, note{"a.md", "# A\n", 1})

	names := func() []string {
		got, _ := call[listNotesOutput](t, cs, "list_notes", map[string]any{"sort": "name_asc"})
		return summaryNames(got.Notes)
	}
	eventually := func(want []string) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if slices.Equal(names(), want) {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatalf("変更を拾っていない: %v（want %v）", names(), want)
	}

	writeNotes(t, root, note{"b.md", "# B\n", 0})
	eventually([]string{"a.md", "b.md"})
	if err := os.Remove(filepath.Join(root, "a.md")); err != nil {
		t.Fatal(err)
	}
	eventually([]string{"b.md"})
}
