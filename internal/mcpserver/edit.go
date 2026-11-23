package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yuuxyu/taggo/internal/meta"
	"github.com/yuuxyu/taggo/internal/model"
	"github.com/yuuxyu/taggo/internal/notefile"
)

var (
	// headingLineRe は ATX の見出しの行（前後の空白を除いたもの）。
	headingLineRe = regexp.MustCompile(`^#{1,6}[ \t]+\S`)
	// tagLineRe はタグの行。[[タグ]] が空白で区切られて並んでいるだけの行（前後の空白を除いたもの）。
	tagLineRe = regexp.MustCompile(`^(\[\[[^\[\]\n]+\]\][ \t]*)+$`)
)

// ---- add_tags ----

type addTagsInput struct {
	Note   string   `json:"note" jsonschema:"ノートのファイル名（golang.md。拡張子は省いてよい）"`
	Tags   []string `json:"tags" jsonschema:"付けるタグ"`
	DryRun bool     `json:"dry_run,omitempty" jsonschema:"true なら書き込まずに、結果だけを返す"`
}

type addTagsOutput struct {
	writeResult
	AlreadyPresent []string `json:"already_present" jsonschema:"既に付いていたので、書き足さなかったタグ"`
}

// addTags は、既にあるノートにタグを付ける。見出しの下のタグの行に [[タグ]] を書き足し、
// タグの行が無ければ見出しの下に作る。create_note が作るノートと同じ場所にタグがそろう。
func (t *tools) addTags(ctx context.Context, _ *mcp.CallToolRequest, in addTagsInput) (*mcp.CallToolResult, addTagsOutput, error) {
	if err := t.lib.wait(ctx); err != nil {
		return nil, addTagsOutput{}, err
	}
	t.writeMu.Lock()
	defer t.writeMu.Unlock()

	tags, err := checkTags(in.Tags)
	if err != nil {
		return nil, addTagsOutput{}, err
	}
	if len(tags) == 0 {
		return nil, addTagsOutput{}, errors.New("tags に付けるタグを入れてください")
	}
	p, raw, err := t.loadNote(in.Note)
	if err != nil {
		return nil, addTagsOutput{}, err
	}

	// 本文のどこかに既に [[タグ]] があるタグは書き足さない。
	have := tagKeys(meta.Parse(raw).Tags)
	var add []string
	already := []string{}
	for _, tag := range tags {
		if have[model.TagKey(tag)] {
			already = append(already, tag)
		} else {
			add = append(add, tag)
		}
	}

	text := notefile.Decode(raw)
	if len(add) > 0 {
		text.Body = insertTags(text.Body, text.EOL, add)
	}
	res, err := t.finish(p, raw, text.Encode(), in.DryRun)
	if err != nil {
		return nil, addTagsOutput{}, err
	}
	return nil, addTagsOutput{writeResult: res, AlreadyPresent: already}, nil
}

// insertTags は本文 body に、タグ tags の [[タグ]] を書き足す。eol はこのノートの改行。
//
// コードブロックの外で最初の見出しを探し、その下の空行を飛ばした次の行を見る。
//   - タグの行なら、その行の末尾に書き足す。
//   - タグの行でなければ、見出しの下に新しいタグの行を作り、前後に空行を 1 つずつ入れる。
//   - 見出しが無ければ、本文の先頭（Front Matter があればその後ろ）に作る。
//
// 行は \n で分けて、CRLF の \r は行の中身として残したまま扱う。手を入れない行は、書かれたまま残る。
func insertTags(body, eol string, tags []string) string {
	cr := strings.TrimSuffix(eol, "\n")
	line := tagLine(tags)
	lines := strings.Split(body, "\n")
	blank := func(i int) bool { return i < len(lines) && strings.TrimSpace(lines[i]) == "" }

	start := strings.Count(body[:len(body)-len(meta.StripFrontMatter([]byte(body)))], "\n")
	h := firstHeadingLine(lines, start)
	if h < 0 {
		ins := []string{line + cr}
		if !blank(start) {
			ins = append(ins, cr)
		}
		return strings.Join(slices.Insert(lines, start, ins...), "\n")
	}

	next := h + 1
	for blank(next) {
		next++
	}
	if next < len(lines) && tagLineRe.MatchString(strings.TrimSpace(lines[next])) {
		l, hasCR := strings.CutSuffix(lines[next], "\r")
		lines[next] = strings.TrimRight(l, " \t") + " " + line
		if hasCR {
			lines[next] += "\r"
		}
		return strings.Join(lines, "\n")
	}

	ins := []string{cr, line + cr}
	if !blank(h + 1) {
		ins = append(ins, cr)
	}
	return strings.Join(slices.Insert(lines, h+1, ins...), "\n")
}

// firstHeadingLine は、start 行目から見て、コードブロックの外で最初の見出しの行を返す。無ければ -1。
func firstHeadingLine(lines []string, start int) int {
	inFence := false
	for i := start; i < len(lines); i++ {
		l := strings.TrimSpace(lines[i])
		if strings.HasPrefix(l, "```") || strings.HasPrefix(l, "~~~") {
			inFence = !inFence
			continue
		}
		if !inFence && headingLineRe.MatchString(l) {
			return i
		}
	}
	return -1
}

// ---- append_to_note ----

type appendInput struct {
	Note   string `json:"note" jsonschema:"ノートのファイル名（golang.md。拡張子は省いてよい）"`
	Text   string `json:"text" jsonschema:"末尾に書き足す Markdown"`
	DryRun bool   `json:"dry_run,omitempty" jsonschema:"true なら書き込まずに、結果だけを返す"`
}

// appendToNote は、既にあるノートの末尾に、空行を 1 つ挟んで書き足す。元の末尾の改行がいくつあっても、
// 空行 1 つを挟み、改行 1 つで終える。
func (t *tools) appendToNote(ctx context.Context, _ *mcp.CallToolRequest, in appendInput) (*mcp.CallToolResult, writeResult, error) {
	if err := t.lib.wait(ctx); err != nil {
		return nil, writeResult{}, err
	}
	t.writeMu.Lock()
	defer t.writeMu.Unlock()

	add := strings.Trim(in.Text, "\r\n")
	if strings.TrimSpace(add) == "" {
		return nil, writeResult{}, errors.New("text に書き足す文を入れてください")
	}
	p, raw, err := t.loadNote(in.Note)
	if err != nil {
		return nil, writeResult{}, err
	}

	text := notefile.Decode(raw)
	base := strings.TrimRight(text.Body, "\r\n")
	if base == "" {
		text.Body = text.Lines(add) + text.EOL
	} else {
		text.Body = base + text.EOL + text.EOL + text.Lines(add) + text.EOL
	}
	res, err := t.finish(p, raw, text.Encode(), in.DryRun)
	if err != nil {
		return nil, writeResult{}, err
	}
	return nil, res, nil
}

// ---- edit_note ----

// maxEdits は、edit_note の 1 回の呼び出しで当てる置き換えの数の上限。
const maxEdits = 20

type textEdit struct {
	OldText string `json:"old_text" jsonschema:"置き換える文。本文と空白・改行まで同じで、ノートの中に 1 か所だけあるもの"`
	NewText string `json:"new_text" jsonschema:"置き換えたあとの文。空にすると old_text を消す"`
}

type editInput struct {
	Note        string     `json:"note" jsonschema:"ノートのファイル名（golang.md。拡張子は省いてよい）"`
	BaseVersion string     `json:"base_version" jsonschema:"read_note が返した version"`
	Edits       []textEdit `json:"edits" jsonschema:"置き換え。前から順に当てる"`
	DryRun      bool       `json:"dry_run,omitempty" jsonschema:"true なら書き込まずに、結果だけを返す"`
}

// editChange は、書き換えた場所 1 つ。
type editChange struct {
	Line    int `json:"line" jsonschema:"書き換えた場所の、書き換えたあとの行番号（1 から）"`
	Removed int `json:"removed" jsonschema:"消した行の数"`
	Added   int `json:"added" jsonschema:"足した行の数"`
}

type editOutput struct {
	writeResult
	Changes []editChange `json:"changes"`
}

// editNote は、既にあるノートの一部を置き換える。
//
// 本文と old_text は、このノートの改行にそろえてから、そのまま比べる。空白を読み替えるような、
// あいまいな照らし合わせはしない（どこが書き換わるのかを予想できるようにするため）。
// edits は前から順に、1 つ前までを当てた本文へ当て、1 つでも当てはまらなければ何も書き込まない。
//
// 版（base_version）を受け取るのは、読まずに書き換えないため。画面で同じノートを書いている途中なら、
// Claude に読み直させるためでもある。
func (t *tools) editNote(ctx context.Context, _ *mcp.CallToolRequest, in editInput) (*mcp.CallToolResult, editOutput, error) {
	if err := t.lib.wait(ctx); err != nil {
		return nil, editOutput{}, err
	}
	t.writeMu.Lock()
	defer t.writeMu.Unlock()

	if in.BaseVersion == "" {
		return nil, editOutput{}, errors.New("base_version に、read_note が返した version を渡してください")
	}
	if len(in.Edits) == 0 || len(in.Edits) > maxEdits {
		return nil, editOutput{}, fmt.Errorf("edits には、置き換えを 1〜%d 個入れてください", maxEdits)
	}
	p, raw, err := t.loadNote(in.Note)
	if err != nil {
		return nil, editOutput{}, err
	}
	if notefile.Version(raw) != in.BaseVersion {
		return nil, editOutput{}, errChanged(p.name())
	}

	text := notefile.Decode(raw)
	body := text.Body
	changes := make([]editChange, 0, len(in.Edits))
	for i, e := range in.Edits {
		if e.OldText == "" {
			return nil, editOutput{}, fmt.Errorf("edits[%d] の old_text が空です。書き足すなら append_to_note を使ってください", i)
		}
		old, repl := text.Lines(e.OldText), text.Lines(e.NewText)
		switch n := strings.Count(body, old); {
		case n == 0:
			return nil, editOutput{}, fmt.Errorf("edits[%d] の old_text が本文にありません。空白や改行も本文と同じにしてください"+
				"（read_note で読み直すと確実です）。何も書き換えていません", i)
		case n > 1:
			return nil, editOutput{}, fmt.Errorf("edits[%d] の old_text が %d か所にあります。前後の文も含めて、1 か所に絞ってください。"+
				"何も書き換えていません", i, n)
		}
		at := strings.Index(body, old)
		body = body[:at] + repl + body[at+len(old):]
		changes = append(changes, editChange{
			Line:    strings.Count(body[:at], "\n") + 1,
			Removed: countLines(old),
			Added:   countLines(repl),
		})
	}
	// taggo は Front Matter を使わないので、新しく書き足す書き換えは断る。
	if !hasFrontMatter(text.Body) && hasFrontMatter(body) {
		return nil, editOutput{}, errors.New("ノートの先頭に Front Matter（--- で囲んだブロック）を書き足す書き換えはできません。" +
			"taggo は Front Matter を使いません。タグは本文の [[タグ]] で付けます")
	}

	text.Body = body
	res, err := t.finish(p, raw, text.Encode(), in.DryRun)
	if err != nil {
		return nil, editOutput{}, err
	}
	return nil, editOutput{writeResult: res, Changes: changes}, nil
}

// countLines は、文 s が何行にわたるかを返す。空なら 0。
func countLines(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(strings.TrimRight(s, "\r\n"), "\n") + 1
}
