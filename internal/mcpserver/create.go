package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf16"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yuuxyu/taggo/internal/atomicfile"
	"github.com/yuuxyu/taggo/internal/meta"
	"github.com/yuuxyu/taggo/internal/model"
	"github.com/yuuxyu/taggo/internal/notefile"
)

// maxNameUnits は、Windows のファイル名 1 つの長さの上限（UTF-16 で数える）。
const maxNameUnits = 255

// h1Re は見出し 1（# 見出し）。捕まえるのは見出しの文字。
var h1Re = regexp.MustCompile(`^#[ \t]+(.+?)[ \t]*#*[ \t]*$`)

type createInput struct {
	Title  string   `json:"title" jsonschema:"ノートの名前。ファイル名（名前.md）と先頭の見出し（# 名前）になり、そのままタグを表す"`
	Body   string   `json:"body" jsonschema:"見出しの下に書く Markdown。見出し（# 名前）は書かなくてよい"`
	Tags   []string `json:"tags,omitempty" jsonschema:"付けるタグ。見出しの下の行に [[タグ]] として並べる"`
	DryRun bool     `json:"dry_run,omitempty" jsonschema:"true なら書き込まずに、出来上がる中身と注意点だけを返す"`
}

// createNote は、taggo の書き方で新しいノートを 1 つ作る。既にあるノートは書き換えない。
//
// 中身は「# title」、空行、tags を並べたタグの行（tags を渡したときだけ）、空行、body の順にする。
// 画面の「md ファイルを作成する」が作るファイル（# 名前 の後ろに空行）と同じ形で、改行は LF、BOM は付けない。
func (t *tools) createNote(ctx context.Context, _ *mcp.CallToolRequest, in createInput) (*mcp.CallToolResult, writeResult, error) {
	if err := t.lib.wait(ctx); err != nil {
		return nil, writeResult{}, err
	}
	t.writeMu.Lock()
	defer t.writeMu.Unlock()

	s := t.lib.store
	root := s.Root()
	name, ok := model.TagFileName(in.Title)
	if !ok {
		return nil, writeResult{}, fmt.Errorf("「%s」はファイル名にできません。< > : \" / \\ | ? * と制御文字、末尾の「.」、"+
			"CON や NUL などの名前は使えません", in.Title)
	}
	if len(utf16.Encode([]rune(name))) > maxNameUnits {
		return nil, writeResult{}, fmt.Errorf("title が長すぎて、ファイル名にできません（%d 文字ほどまで）", maxNameUnits-3)
	}
	title := strings.TrimSuffix(name, ".md")
	path := filepath.Join(root, name)
	if existing := t.existingPage(path, title); existing != "" {
		return nil, writeResult{}, errExists(existing)
	}

	body := strings.ReplaceAll(strings.TrimPrefix(in.Body, string(notefile.BOM)), "\r\n", "\n")
	if hasFrontMatter(body) {
		return nil, writeResult{}, errors.New("body を Front Matter（--- で囲んだブロック）で始めないでください。" +
			"taggo は Front Matter を使いません。タグは tags か、本文の [[タグ]] で付けます")
	}
	warnings := []string{} // 結果の JSON で null ではなく [] にする
	if rest, dropped := dropSameHeading(body, title); dropped {
		body = rest
		warnings = append(warnings, "body の先頭の見出しは title と同じなので外しました（見出しは title から作ります）")
	}
	tags, err := checkTags(in.Tags)
	if err != nil {
		return nil, writeResult{}, err
	}
	// 本文に既に [[ ]] で書いてあるタグは、タグの行に並べない。
	inBody := tagKeys(meta.Parse([]byte(body)).Tags)
	line := make([]string, 0, len(tags))
	for _, tag := range tags {
		if !inBody[model.TagKey(tag)] {
			line = append(line, tag)
		}
	}

	parts := []string{"# " + title}
	if len(line) > 0 {
		parts = append(parts, tagLine(line))
	}
	if b := strings.Trim(body, "\n"); b != "" {
		parts = append(parts, b)
	}
	data := []byte(strings.Join(parts, "\n\n") + "\n")
	if len(data) > notefile.MaxBytes {
		return nil, writeResult{}, fmt.Errorf("中身が %d バイトを超えるため、作れません", notefile.MaxBytes)
	}

	if in.DryRun {
		out := t.report(path, nil, data, true)
		out.Warnings = append(warnings, out.Warnings...)
		out.Content = preview(data)
		return nil, out, nil
	}
	if err := atomicfile.CreateNew(path, data); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return nil, writeResult{}, errExists(name)
		}
		return nil, writeResult{}, fmt.Errorf("ノートを作れませんでした: %w", err)
	}
	if err := t.reindex(path); err != nil {
		return nil, writeResult{}, err
	}
	out := t.report(path, nil, data, true)
	out.Warnings = append(warnings, out.Warnings...)
	out.Written = true
	out.Version = notefile.Version(data)
	return nil, out, nil
}

// existingPage は、path（タグ title のページ）と同じページが既にあれば、そのファイル名を返す。
// 大文字小文字と .md / .markdown の違いは区別しない。走査の上限で読んでいないファイルも確かめる。
func (t *tools) existingPage(path, title string) string {
	s := t.lib.store
	if found := s.FindNote(path); found != "" {
		return filepath.Base(found)
	}
	if found := s.TagPagePath(title); found != "" {
		return filepath.Base(found)
	}
	for _, ext := range []string{".md", ".markdown"} {
		candidate := filepath.Join(filepath.Dir(path), title+ext)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return filepath.Base(candidate)
		}
	}
	return ""
}

// errExists は、同じ名前のノートが既にあるときのエラー。
func errExists(name string) error {
	return fmt.Errorf("「%s」は既にあります。このサーバーは、既にあるノートを create_note で上書きしません。"+
		"read_note で中身を確かめ、書き足すなら append_to_note か edit_note を使ってください", name)
}

// dropSameHeading は、body の先頭が title と同じ見出し 1 なら、その見出しを外した body を返す。
// 見出しは title から作るので、二重にならないようにする。
func dropSameHeading(body, title string) (string, bool) {
	first, rest, _ := strings.Cut(strings.TrimLeft(body, "\n"), "\n")
	m := h1Re.FindStringSubmatch(strings.TrimSpace(first))
	if m == nil || model.TagKey(m[1]) != model.TagKey(title) {
		return body, false
	}
	return strings.TrimLeft(rest, "\n"), true
}
