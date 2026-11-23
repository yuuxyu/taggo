package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yuuxyu/taggo/internal/notefile"
)

// bom は、BOM 付きのテスト用の中身を作る。
func bom(s string) string { return string(notefile.BOM) + s }

// readNoteFile はノートのファイルの中身を読む。
func readNoteFile(t *testing.T, root, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// noTempFiles は、書き込みの一時ファイルが残っていないことを確かめる。
func noTempFiles(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".taggo-") {
			t.Errorf("一時ファイルが残っている: %s", e.Name())
		}
	}
}

func versionOf(s string) string { return notefile.Version([]byte(s)) }

// taggo の書き方でノートを作り、作ったノートをすぐに読めること。
func TestCreateNote(t *testing.T) {
	cs, root := newSession(t,
		note{"golang.md", "# golang\n", 1},
		note{"impl.md", "# 実装\n", 1},
	)

	got, _ := call[writeResult](t, cs, "create_note", map[string]any{
		"title": "goroutine の止め方",
		"body":  "# goroutine の止め方\r\n\r\n[[golang]] で [[並行処理]] を書くときのメモ。\r\n\r\n[実装](impl.md) と [予定](todo.md)\r\n",
		"tags":  []string{"golang", "メモ", "#メモ"},
	})
	// 見出しは title から作り、本文に既にあるタグ（golang）と重なったタグ（#メモ）はタグの行に並べない。
	want := "# goroutine の止め方\n\n[[メモ]]\n\n[[golang]] で [[並行処理]] を書くときのメモ。\n\n[実装](impl.md) と [予定](todo.md)\n"
	if content := readNoteFile(t, root, "goroutine の止め方.md"); content != want {
		t.Fatalf("中身が違う:\n%q\nwant\n%q", content, want)
	}
	if got.Name != "goroutine の止め方.md" || !got.Written || got.Version != versionOf(want) {
		t.Fatalf("結果が違う: %+v", got)
	}
	if !slices.Equal(got.TagsAdded, []string{"golang", "メモ", "並行処理"}) ||
		!slices.Equal(got.MissingTagPages, []string{"メモ", "並行処理"}) ||
		!slices.Equal(got.MissingLinks, []string{"todo.md"}) {
		t.Fatalf("タグとリンクの知らせが違う: %+v", got)
	}
	if len(got.Warnings) != 1 || !strings.Contains(got.Warnings[0], "title と同じ") {
		t.Fatalf("見出しを外したことが知らされていない: %v", got.Warnings)
	}
	noTempFiles(t, root)

	// ウォッチャーを待たずに、すぐ読めて、タグでも引ける。
	if read, _ := call[readNoteOutput](t, cs, "read_note", map[string]any{"note": "goroutine の止め方"}); !read.Exists || read.Version != got.Version {
		t.Fatalf("作ったノートをすぐに読めない: %+v", read)
	}
	if list, _ := call[listNotesOutput](t, cs, "list_notes", map[string]any{"tag": "メモ"}); len(list.Notes) != 1 {
		t.Fatalf("作ったノートがタグで引けない: %+v", list)
	}

	// 注意点が無ければ、warnings は null ではなく空の配列で返す。
	plain, _ := call[writeResult](t, cs, "create_note", map[string]any{"title": "注意点なし", "body": "本文", "tags": []string{"golang"}})
	if plain.Warnings == nil || len(plain.Warnings) != 0 {
		t.Fatalf("注意点の無い warnings が空の配列でない: %#v", plain.Warnings)
	}
}

// 作れない名前、既にあるノート、Front Matter、[[ ]] に書けないタグは、何も作らずに断ること。
func TestCreateNoteRefuses(t *testing.T) {
	root := t.TempDir()
	writeNotes(t, root,
		note{"a.md", "# A\n", 1},
		note{"golang.md", "# Go\n", 1},
		note{"古い.markdown", "# 古い\n", 1},
		note{"z.md", "# 走査の上限で読んでいない\n", 1},
	)
	cs := connect(t, openLibrary(t, root, 3))

	tests := []struct {
		args map[string]any
		want string
	}{
		{map[string]any{"title": "a/b", "body": ""}, "ファイル名にできません"},
		{map[string]any{"title": "CON", "body": ""}, "ファイル名にできません"},
		{map[string]any{"title": "名前.", "body": ""}, "ファイル名にできません"},
		{map[string]any{"title": "  ", "body": ""}, "ファイル名にできません"},
		{map[string]any{"title": "Golang", "body": ""}, "「golang.md」は既にあります"},
		{map[string]any{"title": "古い", "body": ""}, "「古い.markdown」は既にあります"},
		{map[string]any{"title": "z", "body": ""}, "「z.md」は既にあります"},
		{map[string]any{"title": "新しい", "body": "---\ntags: [x]\n---\n本文\n"}, "Front Matter"},
		{map[string]any{"title": "新しい", "body": "", "tags": []string{"a]]b"}}, "タグにできません"},
		{map[string]any{"title": "新しい", "body": "", "tags": []string{" "}}, "空のタグ"},
	}
	for _, tt := range tests {
		if msg := callError(t, cs, "create_note", tt.args); !strings.Contains(msg, tt.want) {
			t.Errorf("create_note(%v) のエラーが違う: %s（want %s）", tt.args, msg, tt.want)
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 4 {
		t.Fatalf("断ったのにファイルが増えた: %v", entries)
	}
}

// dry_run は書き込まずに、出来上がる中身と注意点だけを返すこと。
func TestCreateNoteDryRun(t *testing.T) {
	cs, root := newSession(t)
	writeNotes(t, root, note{"ある.png", "画像の代わり", 0})

	got, _ := call[writeResult](t, cs, "create_note", map[string]any{
		"title":   "下書き",
		"body":    "![ある](ある.png)\n![無い](無い.png)\n![外](https://example.com/a.png)\n",
		"dry_run": true,
	})
	if got.Written || got.Content != "# 下書き\n\n![ある](ある.png)\n![無い](無い.png)\n![外](https://example.com/a.png)\n" {
		t.Fatalf("dry_run の結果が違う: %+v", got)
	}
	if _, err := os.Stat(filepath.Join(root, "下書き.md")); !os.IsNotExist(err) {
		t.Fatalf("dry_run なのにファイルを作った: %v", err)
	}
	joined := strings.Join(got.Warnings, "\n")
	if !strings.Contains(joined, "無い.png") || strings.Contains(joined, "ある.png") || strings.Contains(joined, "example.com") ||
		!strings.Contains(joined, "タグが 1 つも付いていません") {
		t.Fatalf("注意点が違う: %v", got.Warnings)
	}
}

// 見出しの下のタグの行に書き足し、行が無ければ作ること。BOM と改行は保つこと。
func TestAddTags(t *testing.T) {
	tests := []struct {
		name, before, after string
	}{
		{"空行の後に本文", "# T\n\n本文\n", "# T\n\n[[a]]\n\n本文\n"},
		{"すぐ本文", "# T\n本文\n", "# T\n\n[[a]]\n\n本文\n"},
		{"見出しだけ", "# T\n", "# T\n\n[[a]]\n"},
		{"タグの行がある", "# T\n\n[[x]] [[y]]\n\n本文\n", "# T\n\n[[x]] [[y]] [[a]]\n\n本文\n"},
		{"見出しが無い", "本文\n", "[[a]]\n\n本文\n"},
		{"コードブロックの中の見出し", "```\n# コメント\n```\n\n# 見出し\n\n本文\n", "```\n# コメント\n```\n\n# 見出し\n\n[[a]]\n\n本文\n"},
		{"Front Matter", "---\ntitle: x\n---\n本文\n", "---\ntitle: x\n---\n[[a]]\n\n本文\n"},
		{"BOM と CRLF", bom("# T\r\n\r\n本文\r\n"), bom("# T\r\n\r\n[[a]]\r\n\r\n本文\r\n")},
		{"CRLF のタグの行", "# T\r\n\r\n[[x]]\r\n", "# T\r\n\r\n[[x]] [[a]]\r\n"},
	}
	notes := make([]note, len(tests))
	for i, tt := range tests {
		notes[i] = note{name: tt.name + ".md", content: tt.before}
	}
	cs, root := newSession(t, notes...)

	for _, tt := range tests {
		got, _ := call[addTagsOutput](t, cs, "add_tags", map[string]any{"note": tt.name, "tags": []string{"a"}})
		if content := readNoteFile(t, root, tt.name+".md"); content != tt.after {
			t.Errorf("%s: 中身が違う:\n%q\nwant\n%q", tt.name, content, tt.after)
		}
		if !got.Written || !slices.Equal(got.TagsAdded, []string{"a"}) || got.Version != versionOf(tt.after) {
			t.Errorf("%s: 結果が違う: %+v", tt.name, got)
		}
	}
	noTempFiles(t, root)
}

// 既に付いているタグは書き足さず、全部付いていれば書き込まないこと。
func TestAddTagsAlreadyPresent(t *testing.T) {
	before := "# T\n\n[[Golang]] のメモ\n"
	cs, root := newSession(t, note{"t.md", before, 1}, note{"draft.md", "# 下書き\n\n[[未整理]]\n", 1})

	got, _ := call[addTagsOutput](t, cs, "add_tags", map[string]any{"note": "t", "tags": []string{"golang", "新しい"}})
	if want := "# T\n\n[[新しい]]\n\n[[Golang]] のメモ\n"; readNoteFile(t, root, "t.md") != want {
		t.Fatalf("中身が違う: %q", readNoteFile(t, root, "t.md"))
	}
	if !slices.Equal(got.AlreadyPresent, []string{"golang"}) || !slices.Equal(got.TagsAdded, []string{"新しい"}) {
		t.Fatalf("結果が違う: %+v", got)
	}

	again, _ := call[addTagsOutput](t, cs, "add_tags", map[string]any{"note": "t", "tags": []string{"GOLANG", "新しい"}})
	if again.Written || len(again.AlreadyPresent) != 2 || again.Version != got.Version {
		t.Fatalf("全部付いているのに書き込んだ: %+v", again)
	}

	if msg := callError(t, cs, "add_tags", map[string]any{"note": "未整理", "tags": []string{"a"}}); !strings.Contains(msg, "create_note") {
		t.Fatalf("まだ無いページの扱いが違う: %s", msg)
	}
}

// 末尾に、空行を 1 つ挟んで書き足し、改行 1 つで終えること。BOM と改行は保つこと。
func TestAppendToNote(t *testing.T) {
	cs, root := newSession(t,
		note{"lf.md", "# T\n\n本文\n\n\n", 1},
		note{"noeol.md", "# T", 1},
		note{"crlf.md", bom("# T\r\n本文\r\n"), 1},
	)

	call[writeResult](t, cs, "append_to_note", map[string]any{"note": "lf", "text": "追記"})
	if got := readNoteFile(t, root, "lf.md"); got != "# T\n\n本文\n\n追記\n" {
		t.Errorf("LF のノートが違う: %q", got)
	}
	call[writeResult](t, cs, "append_to_note", map[string]any{"note": "noeol", "text": "\n追記\n\n"})
	if got := readNoteFile(t, root, "noeol.md"); got != "# T\n\n追記\n" {
		t.Errorf("末尾に改行の無いノートが違う: %q", got)
	}
	res, _ := call[writeResult](t, cs, "append_to_note", map[string]any{"note": "crlf", "text": "追記 [[タグ]]\n2 行目"})
	if got := readNoteFile(t, root, "crlf.md"); got != bom("# T\r\n本文\r\n\r\n追記 [[タグ]]\r\n2 行目\r\n") {
		t.Errorf("BOM と CRLF のノートが違う: %q", got)
	}
	if !slices.Equal(res.TagsAdded, []string{"タグ"}) || !slices.Equal(res.MissingTagPages, []string{"タグ"}) {
		t.Errorf("付いたタグが知らされていない: %+v", res)
	}
	// 書き足したタグで、すぐに引ける。
	if list, _ := call[listNotesOutput](t, cs, "list_notes", map[string]any{"tag": "タグ"}); len(list.Notes) != 1 || list.Notes[0].Name != "crlf.md" {
		t.Errorf("書き足したタグで引けない: %+v", list)
	}

	callError(t, cs, "append_to_note", map[string]any{"note": "lf", "text": " \n "})
	noTempFiles(t, root)
}

// 版を確かめ、すべての置き換えが当てはまるときだけ書き込むこと。BOM と改行は保つこと。
func TestEditNote(t *testing.T) {
	before := bom("# T\r\n\r\n[[古い]] [[残す]]\r\n\r\n一行目\r\n二行目\r\n同じ\r\n同じ\r\n")
	cs, root := newSession(t, note{"t.md", before, 1})
	version := versionOf(before)
	edit := func(edits ...map[string]any) map[string]any {
		return map[string]any{"note": "t", "base_version": version, "edits": edits}
	}
	unchanged := func() {
		t.Helper()
		if got := readNoteFile(t, root, "t.md"); got != before {
			t.Fatalf("断ったのに書き換わった: %q", got)
		}
	}

	stale := map[string]any{"note": "t", "base_version": versionOf("古い版"), "edits": []map[string]any{{"old_text": "一行目", "new_text": "x"}}}
	if msg := callError(t, cs, "edit_note", stale); !strings.Contains(msg, "読んだあとで書き換えられています") {
		t.Fatalf("版の食い違いが伝わらない: %s", msg)
	}
	if msg := callError(t, cs, "edit_note", edit(map[string]any{"old_text": "無い文", "new_text": "x"})); !strings.Contains(msg, "本文にありません") {
		t.Fatalf("見つからないことが伝わらない: %s", msg)
	}
	if msg := callError(t, cs, "edit_note", edit(map[string]any{"old_text": "同じ", "new_text": "x"})); !strings.Contains(msg, "2 か所") {
		t.Fatalf("2 か所あることが伝わらない: %s", msg)
	}
	// 1 つでも当てはまらなければ、前の置き換えも書き込まない。
	callError(t, cs, "edit_note", edit(
		map[string]any{"old_text": "一行目", "new_text": "1 行目"},
		map[string]any{"old_text": "無い文", "new_text": "x"},
	))
	if msg := callError(t, cs, "edit_note", edit(map[string]any{"old_text": "# T", "new_text": "---\na: b\n---\n# T"})); !strings.Contains(msg, "Front Matter") {
		t.Fatalf("Front Matter を書き足せてしまう: %s", msg)
	}
	unchanged()

	// dry_run は書き込まない。
	dry, _ := call[editOutput](t, cs, "edit_note", map[string]any{
		"note": "t", "base_version": version, "dry_run": true,
		"edits": []map[string]any{{"old_text": "一行目", "new_text": "1 行目"}},
	})
	if dry.Written || !strings.Contains(dry.Content, "1 行目\n二行目") || dry.Version != version {
		t.Fatalf("dry_run の結果が違う: %+v", dry)
	}
	unchanged()

	// old_text は LF で書いてもよい。CRLF のノートの改行にそろえて照らし合わせる。
	got, _ := call[editOutput](t, cs, "edit_note", edit(
		map[string]any{"old_text": "[[古い]] ", "new_text": ""},
		map[string]any{"old_text": "一行目\n二行目", "new_text": "1 行目\n2 行目\n3 行目"},
	))
	after := bom("# T\r\n\r\n[[残す]]\r\n\r\n1 行目\r\n2 行目\r\n3 行目\r\n同じ\r\n同じ\r\n")
	if content := readNoteFile(t, root, "t.md"); content != after {
		t.Fatalf("中身が違う:\n%q\nwant\n%q", content, after)
	}
	wantChanges := []editChange{{Line: 3, Removed: 1, Added: 0}, {Line: 5, Removed: 2, Added: 3}}
	if !got.Written || got.Version != versionOf(after) || !slices.Equal(got.TagsRemoved, []string{"古い"}) ||
		!slices.Equal(got.Changes, wantChanges) {
		t.Fatalf("結果が違う: %+v", got)
	}

	// 返った版で、続けて書き換えられる。中身が変わらなければ書き込まない。
	same, _ := call[editOutput](t, cs, "edit_note", map[string]any{
		"note": "t", "base_version": got.Version,
		"edits": []map[string]any{{"old_text": "3 行目", "new_text": "3 行目"}},
	})
	if same.Written || same.Version != got.Version {
		t.Fatalf("中身が変わらないのに書き込んだ: %+v", same)
	}
	noTempFiles(t, root)
}

// --read-only では書き込むプロンプトを出さず、ほかのプロンプトも書き込まない文にすること。
func TestPromptsByMode(t *testing.T) {
	lib := openLibrary(t, t.TempDir(), 1000)
	ctx := context.Background()
	promptNames := func(cs *mcp.ClientSession) []string {
		t.Helper()
		res, err := cs.ListPrompts(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, p := range res.Prompts {
			names = append(names, p.Name)
		}
		slices.Sort(names)
		return names
	}
	promptText := func(cs *mcp.ClientSession, name string, args map[string]string) string {
		t.Helper()
		res, err := cs.GetPrompt(ctx, &mcp.GetPromptParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("%s を取れない: %v", name, err)
		}
		return res.Messages[0].Content.(*mcp.TextContent).Text
	}

	writable := connectWith(t, lib, true)
	if got := promptNames(writable); !slices.Equal(got, []string{"connect_note", "review_recent", "summarize_tag", "write_missing_pages", "write_note"}) {
		t.Fatalf("書き込めるときのプロンプトが違う: %v", got)
	}
	if text := promptText(writable, "summarize_tag", map[string]string{"tag": "golang"}); !strings.Contains(text, "create_note") {
		t.Fatalf("書き込めるのに、書き込まない文になっている: %s", text)
	}
	if text := promptText(writable, "write_note", map[string]string{"topic": "goroutine", "material": "メモの材料"}); !strings.Contains(text, "「goroutine」") || !strings.Contains(text, "メモの材料") {
		t.Fatalf("話題と材料が入っていない: %s", text)
	}
	if text := promptText(writable, "write_missing_pages", map[string]string{"limit": "3"}); !strings.Contains(text, "3 個まで") {
		t.Fatalf("数が入っていない: %s", text)
	}
	if text := promptText(writable, "connect_note", map[string]string{"note": "golang.md"}); !strings.Contains(text, "「golang.md」") {
		t.Fatalf("ノートが入っていない: %s", text)
	}

	readOnly := connectWith(t, lib, false)
	if got := promptNames(readOnly); !slices.Equal(got, []string{"review_recent", "summarize_tag"}) {
		t.Fatalf("--read-only のプロンプトが違う: %v", got)
	}
	for _, name := range []string{"summarize_tag", "review_recent"} {
		text := promptText(readOnly, name, map[string]string{"tag": "golang"})
		if strings.Contains(text, "create_note") || strings.Contains(text, "edit_note") || strings.Contains(text, "add_tags") {
			t.Fatalf("--read-only なのに書き込むツールを使う文になっている: %s", text)
		}
	}
}
