package meta

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/yuuxyu/taggo/internal/model"
	"gopkg.in/yaml.v3"
)

func init() { Register(markdownHandler{}) }

// markdownHandler は YAML Front Matter の tags フィールドを読み書きする。
type markdownHandler struct{}

func (markdownHandler) Kind() model.Kind     { return model.KindMarkdown }
func (markdownHandler) Extensions() []string { return []string{".md", ".markdown"} }

const (
	// previewRunes はカードに載せる本文抜粋の最大文字数。
	previewRunes = 240
	// maxMarkdownSz を超えるファイルは本文を読まず、ファイル名だけでインデックスする。
	maxMarkdownSz = 8 << 20
)

var (
	headingRe  = regexp.MustCompile(`(?m)^#{1,6}[ \t]+(.+?)[ \t]*#*[ \t]*$`)
	wikiLinkRe = regexp.MustCompile(`\[\[([^\]\[|]+)(?:\|[^\]\[]*)?\]\]`)
)

func (markdownHandler) Read(path string) (Info, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Info{}, err
	}
	if len(raw) > maxMarkdownSz {
		return Info{}, fmt.Errorf("Markdown ファイルが大きすぎてインデックスできません (%d バイト)", len(raw))
	}

	front, body, err := SplitFrontMatter(raw)
	if err != nil {
		// Front Matter が壊れていても本文は読めるので、
		// パースエラーを添えたうえでエントリ自体は表示する価値がある。
		return Info{}, err
	}

	info := Info{
		Tags:    frontMatterTags(front),
		Title:   firstHeading(body),
		Preview: excerpt(body),
		Links:   wikiLinks(body),
	}
	if info.Title == "" {
		if t, ok := front["title"].(string); ok {
			info.Title = strings.TrimSpace(t)
		}
	}
	return info, nil
}

func (markdownHandler) WriteTags(path string, tags []string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	front, body, err := SplitFrontMatter(raw)
	if err != nil {
		return fmt.Errorf("Front Matter が不正なため書き換えを中止しました: %w", err)
	}

	if front == nil {
		front = map[string]any{}
	}
	// 読み取り時は "keywords" も別名として受け付けるが、書き込み時は "tags" に統一し、
	// 別名のほうは削除して両者が食い違わないようにする。
	delete(front, "keywords")
	if len(tags) == 0 {
		delete(front, "tags")
	} else {
		front["tags"] = tags
	}

	out, err := renderMarkdown(front, body)
	if err != nil {
		return err
	}
	return replaceFileBytes(path, out)
}

// SplitFrontMatter は YAML Front Matter ブロックと Markdown 本文を切り分ける。
// Front Matter を持たない文書に対しては nil のマップと、入力全体を本文として返す。
func SplitFrontMatter(raw []byte) (map[string]any, []byte, error) {
	rest, ok := bytes.CutPrefix(raw, []byte("---\n"))
	if !ok {
		if r, okCRLF := bytes.CutPrefix(raw, []byte("---\r\n")); okCRLF {
			rest = r
		} else {
			return nil, raw, nil
		}
	}

	end := findFrontMatterEnd(rest)
	if !end.valid() {
		return nil, raw, nil // 閉じられていない "---" は単なる本文とみなす
	}

	var front map[string]any
	if err := yaml.Unmarshal(rest[:end.start], &front); err != nil {
		return nil, raw, fmt.Errorf("Front Matter の YAML が不正です: %w", err)
	}
	return front, rest[end.bodyAt:], nil
}

// fmBounds は Front Matter の終端位置を表す。
type fmBounds struct {
	start  int // 終端デリミタ行の開始オフセット
	bodyAt int // その次、本文 1 バイト目のオフセット
}

func findFrontMatterEnd(rest []byte) fmBounds {
	for off := 0; off < len(rest); {
		lineEnd := bytes.IndexByte(rest[off:], '\n')
		var line []byte
		next := len(rest)
		if lineEnd < 0 {
			line = rest[off:]
		} else {
			line = rest[off : off+lineEnd]
			next = off + lineEnd + 1
		}
		if t := strings.TrimRight(string(line), "\r"); t == "---" || t == "..." {
			return fmBounds{start: off, bodyAt: next}
		}
		if lineEnd < 0 {
			break
		}
		off = next
	}
	return fmBounds{start: -1, bodyAt: -1}
}

func (b fmBounds) valid() bool { return b.start >= 0 }

func frontMatterTags(front map[string]any) []string {
	if front == nil {
		return nil
	}
	var out []string
	for _, key := range []string{"tags", "keywords"} {
		out = append(out, coerceTagList(front[key])...)
	}
	return out
}

// coerceTagList は、実際の Front Matter で使われるタグの書き方をすべて受け付ける。
// YAML のシーケンス、カンマ区切り文字列、単一スカラーのいずれにも対応する。
func coerceTagList(v any) []string {
	switch t := v.(type) {
	case nil:
		return nil
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			out = append(out, coerceTagList(item)...)
		}
		return out
	case []string:
		return t
	case string:
		if strings.ContainsAny(t, ",") {
			return strings.Split(t, ",")
		}
		return []string{t}
	default:
		return []string{fmt.Sprint(t)}
	}
}

// renderMarkdown は Front Matter と本文から文書を組み直す。
// Front Matter が空の場合は、空ブロックを書かずにブロックごと省く。
func renderMarkdown(front map[string]any, body []byte) ([]byte, error) {
	if len(front) == 0 {
		return body, nil
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(front); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}

	var out bytes.Buffer
	out.WriteString("---\n")
	out.Write(buf.Bytes())
	out.WriteString("---\n")
	// ブロックと本文の間は必ず空行 1 行にそろえる。
	trimmed := bytes.TrimLeft(body, "\r\n")
	if len(trimmed) > 0 {
		out.WriteString("\n")
		out.Write(trimmed)
	}
	return out.Bytes(), nil
}

// isTableRule は GFM の表の区切り行（| --- | :--: |）かどうかを判定する。
func isTableRule(line string) bool {
	if !strings.Contains(line, "-") {
		return false
	}
	for _, r := range line {
		switch r {
		case '|', '-', ':', ' ', '\t':
		default:
			return false
		}
	}
	return true
}

func firstHeading(body []byte) string {
	if m := headingRe.FindSubmatch(body); m != nil {
		return strings.TrimSpace(string(m[1]))
	}
	return ""
}

func wikiLinks(body []byte) []string {
	matches := wikiLinkRe.FindAllSubmatch(body, -1)
	if matches == nil {
		return nil
	}
	seen := make(map[string]struct{}, len(matches))
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		target := strings.TrimSpace(string(m[1]))
		// "#アンカー" が続く場合、指しているのは同じ文書の内部位置なので切り落とす。
		if i := strings.IndexByte(target, '#'); i > 0 {
			target = strings.TrimSpace(target[:i])
		}
		if target == "" {
			continue
		}
		if _, dup := seen[strings.ToLower(target)]; dup {
			continue
		}
		seen[strings.ToLower(target)] = struct{}{}
		out = append(out, target)
	}
	return out
}

// excerpt は Markdown カードに表示するプレーンテキストの抜粋を作る。
// 本文冒頭から、見出し記号・強調記号・コードフェンスを取り除いて組み立てる。
func excerpt(body []byte) string {
	var b strings.Builder
	inFence := false
	for _, line := range strings.Split(string(body), "\n") {
		l := strings.TrimSpace(line)
		if strings.HasPrefix(l, "```") || strings.HasPrefix(l, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence || l == "" {
			continue
		}
		if isTableRule(l) {
			continue // 表の区切り行（| --- | --- |）は読む値が無い
		}
		l = strings.TrimLeft(l, "#>-*+ \t")
		l = strings.ReplaceAll(l, "**", "")
		l = strings.ReplaceAll(l, "__", "")
		// 表のセル区切りは空白に、WikiLink は表示名だけに落とす。
		l = strings.Trim(l, "|")
		l = strings.ReplaceAll(l, "|", " ")
		l = wikiLinkRe.ReplaceAllString(l, "$1")
		l = strings.Join(strings.Fields(l), " ")
		if l == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(l)
		if len([]rune(b.String())) >= previewRunes {
			break
		}
	}

	r := []rune(b.String())
	if len(r) > previewRunes {
		return strings.TrimSpace(string(r[:previewRunes])) + "…"
	}
	return b.String()
}
