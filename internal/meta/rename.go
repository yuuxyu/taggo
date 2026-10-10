package meta

import (
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yuuxyu/taggo/internal/model"
)

// renameLinkRe は書き換えのために、Markdown のリンクを部品に分けて捕まえる。
// 何をリンクとみなすかは mdLinkRe と同じで、捕まえるのは次のとおり。
//
//	1 … 画像の印の "!"
//	2 … 表示文字
//	3 … "(" の直後の空白と "<"
//	4 … 行き先
//	5 … ">" とタイトル
var renameLinkRe = regexp.MustCompile(`(!?)\[([^\]\[]*)\]\((\s*<?)([^)<>\s]+)(>?[^)]*)\)`)

// linkNameEscaper は、新しい名前をリンクの行き先へ書くときに、リンクとして
// 読めなくなる文字だけを % で書き表す。日本語などはそのまま書く。
var linkNameEscaper = strings.NewReplacer(
	"%", "%25",
	" ", "%20",
	"\t", "%09",
	"#", "%23",
	"?", "%3F",
	")", "%29",
	"<", "%3C",
	">", "%3E",
)

// RenameRefs は、名前を変えるページを指す参照を、本文から探して新しい名前へ書き換える。
type RenameRefs struct {
	// OldName と NewName は、変更前と変更後のページ名（拡張子を除いたファイル名）。
	OldName, NewName string
	// LinksTo は、Markdown のリンクの行き先（フラグメントを除いてデコードしたパス）が、
	// 名前を変えるページを指しているかを返す。行き先はリンク元の位置で決まるので、呼び出し側が解決する。
	LinksTo func(link string) bool
	// Self は、書き換える文書が名前を変えるページ自身かどうか。
	Self bool
}

// Rewrite は文書 raw の参照を書き換えたものと、書き換えたところがあったかを返す。
//
// 書き換えるのは次のもの。コードブロック・インラインコード・先頭の「---」で囲んだブロックの中は触らない。
//   - `[[変更前の名前]]`。タグの突き合わせと同じく、大文字小文字と空白の連なりは区別しない
//   - 名前を変えるページへの Markdown のリンクの行き先。書き換えるのはファイル名の部分だけで、
//     フォルダの部分・拡張子・「#」以降は書かれたまま残す
//   - そのリンクの表示文字が、変更前の名前か拡張子付きのファイル名と同じなら、その表示文字
//   - Self のときは、最初の見出しが変更前の名前と同じなら、その見出し
func (r RenameRefs) Rewrite(raw []byte) ([]byte, bool) {
	body := StripFrontMatter(raw)
	head := raw[:len(raw)-len(body)]
	oldKey := model.TagKey(r.OldName)

	text := string(body)
	if r.Self {
		text = r.renameHeading(text, oldKey)
	}

	var b strings.Builder
	inFence := false
	for line := range strings.SplitAfterSeq(text, "\n") {
		if isFence(strings.TrimSpace(line)) {
			inFence = !inFence
			b.WriteString(line)
			continue
		}
		if inFence || !strings.Contains(line, "[") {
			b.WriteString(line)
			continue
		}
		last := 0
		for _, span := range inlineCodeSpans(line) {
			b.WriteString(r.rewriteText(line[last:span[0]], oldKey))
			b.WriteString(line[span[0]:span[1]])
			last = span[1]
		}
		b.WriteString(r.rewriteText(line[last:], oldKey))
	}

	out := b.String()
	if out == string(body) {
		return raw, false
	}
	return append(append([]byte{}, head...), out...), true
}

// renameHeading は、最初の見出しが変更前の名前と同じなら、新しい名前にする。
// 見出しの前後の空白や閉じの「#」は書かれたまま残す。
func (r RenameRefs) renameHeading(text, oldKey string) string {
	m := headingRe.FindStringSubmatchIndex(text)
	if m == nil {
		return text
	}
	heading := text[m[2]:m[3]]
	name := strings.TrimSpace(heading)
	if model.TagKey(name) != oldKey {
		return text
	}
	start := m[2] + strings.Index(heading, name)
	return text[:start] + r.NewName + text[start+len(name):]
}

// rewriteText は、コードの外にある文字列の中の参照を書き換える。
func (r RenameRefs) rewriteText(s, oldKey string) string {
	if s == "" {
		return s
	}
	s = renameLinkRe.ReplaceAllStringFunc(s, func(m string) string {
		return r.rewriteLink(m, oldKey)
	})
	return bracketTagRe.ReplaceAllStringFunc(s, func(m string) string {
		if model.TagKey(m[2:len(m)-2]) != oldKey {
			return m
		}
		return "[[" + r.NewName + "]]"
	})
}

// rewriteLink は Markdown のリンク 1 つが名前を変えるページを指していれば、
// その行き先と表示文字を書き換える。
func (r RenameRefs) rewriteLink(m, oldKey string) string {
	g := renameLinkRe.FindStringSubmatch(m)
	image, label, open, href, rest := g[1], g[2], g[3], g[4], g[5]
	if image != "" {
		return m
	}
	link, ok := noteFileLink(href)
	if !ok || r.LinksTo == nil || !r.LinksTo(link) {
		return m
	}
	return "[" + r.renameLabel(label, oldKey) + "](" + open + r.renameHref(href) + rest + ")"
}

// renameHref はリンクの行き先のうち、ファイル名の拡張子を除いた部分だけを新しい名前にする。
// もとのファイル名が % で書き表されていれば、新しい名前も同じように書き表す。
func (r RenameRefs) renameHref(href string) string {
	path, fragment := href, ""
	if i := strings.IndexByte(href, '#'); i >= 0 {
		path, fragment = href[:i], href[i:]
	}
	dir, file := "", path
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		dir, file = path[:i+1], path[i+1:]
	}
	name := linkNameEscaper.Replace(r.NewName)
	if strings.Contains(file, "%") {
		name = url.PathEscape(r.NewName)
	}
	return dir + name + filepath.Ext(file) + fragment
}

// renameLabel は、リンクの表示文字が変更前の名前（拡張子付きのファイル名を含む）なら新しい名前にする。
// 表示文字の前後の空白は書かれたまま残す。
func (r RenameRefs) renameLabel(label, oldKey string) string {
	name := strings.TrimSpace(label)
	if name == "" {
		return label
	}
	replacement := ""
	switch ext := filepath.Ext(name); {
	case model.TagKey(name) == oldKey:
		replacement = r.NewName
	case model.IsMarkdown(name) && model.TagKey(strings.TrimSuffix(name, ext)) == oldKey:
		replacement = r.NewName + ext
	default:
		return label
	}
	return strings.Replace(label, name, replacement, 1)
}
