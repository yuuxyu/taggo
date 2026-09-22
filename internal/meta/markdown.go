package meta

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// markdownHandler は YAML Front Matter の tags フィールドを読み書きする。
type markdownHandler struct{}

const (
	// previewRunes はカードに載せる本文抜粋の最大文字数。
	previewRunes = 240
	// maxMarkdownSz を超えるファイルは本文を読まず、ファイル名だけでインデックスする。
	maxMarkdownSz = 8 << 20
)

var (
	headingRe = regexp.MustCompile(`(?m)^#{1,6}[ \t]+(.+?)[ \t]*#*[ \t]*$`)
	// mdLinkRe は Markdown のリンク。先頭の "!" が付くものは画像なので、
	// 見分けられるよう一緒に捕まえる。
	mdLinkRe = regexp.MustCompile(`(!?)\[[^\]\[]*\]\(\s*<?([^)<>\s]+)>?[^)]*\)`)
	// mdImageTextRe / mdLinkTextRe は抜粋を作るときに使う。画像は丸ごと落とし、
	// リンクは表示されている文字だけを残す。
	mdImageTextRe = regexp.MustCompile(`!\[[^\]\[]*\]\([^)]*\)`)
	mdLinkTextRe  = regexp.MustCompile(`\[([^\]\[]*)\]\([^)]*\)`)
	// schemeRe は http: や mailto: などのスキーム。driveRe は Windows の
	// ドライブ文字で、スキームに見えるがローカルパスなので除外に使う。
	schemeRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`)
	driveRe  = regexp.MustCompile(`^[a-zA-Z]:[\\/]`)
	// youtubeRe は YouTube 動画の URL。捕まえるのは 11 文字の動画 ID。
	youtubeRe = regexp.MustCompile(`https?://(?:(?:www|m|music)\.)?(?:youtube(?:-nocookie)?\.com/(?:watch\?(?:[^\s)>]*&)?v=|shorts/|embed/|live/)|youtu\.be/)([A-Za-z0-9_-]{11})`)
)

const (
	// maxThumbnailLen を超える画像の参照は、カードのサムネイルに使わない。
	// 巨大な値をエントリに抱え込まないため。
	maxThumbnailLen = 2048
	// youtubeThumbnail は動画 ID から作るサムネイル画像の URL。mqdefault は
	// 16:9 で上下の黒帯が無いので、カードの枠に切り抜いても見栄えが崩れない。
	youtubeThumbnail = "https://i.ytimg.com/vi/%s/mqdefault.jpg"
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
		Tags:      frontMatterTags(front),
		Title:     firstHeading(body),
		Preview:   excerpt(body),
		Thumbnail: thumbnail(body),
		Links:     noteLinks(body),
		TagPage:   tagPageOf(front),
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

// tagPageOf は Front Matter の `tag:` を読み、そのノートが説明しているタグを返す。
//
// 1 つのページが説明するタグは 1 つに限るので、文字列や数値のような単一の値だけを
// 受け付ける。リストのように複数書かれていたら、どれを指すのか決められないので
// タグページとはみなさない。
func tagPageOf(front map[string]any) string {
	switch v := front["tag"].(type) {
	case nil, []any, map[string]any:
		return ""
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
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

// noteLinks は本文からノート間のリンクを集める。
//
// 対象は Markdown のリンクのうち .md / .markdown を指しているものだけで、
// 行き先は `./sub/impl.md` のように書かれたまま返す。どのノートを指すのかは、
// ファイルの位置を知っている store 側で解決する。
// 画像と外部 URL は対象にしない。
func noteLinks(body []byte) []string {
	seen := map[string]struct{}{}
	var out []string

	for _, m := range mdLinkRe.FindAllSubmatch(body, -1) {
		if len(m[1]) > 0 {
			continue // 画像
		}
		href, ok := noteFileLink(string(m[2]))
		if !ok {
			continue
		}
		key := strings.ToLower(href)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, href)
	}
	return out
}

// noteFileLink はリンク先が Markdown ファイルを指していれば、そのパスを
// 書かれたまま返す。スキーム付きの URL は外部を指しているものとして扱わない。
func noteFileLink(href string) (string, bool) {
	if href == "" || (!driveRe.MatchString(href) && schemeRe.MatchString(href)) {
		return "", false
	}

	path := href
	if i := strings.IndexByte(path, '#'); i >= 0 {
		path = path[:i]
	}
	if unescaped, err := url.PathUnescape(path); err == nil {
		path = unescaped
	}

	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".markdown") {
		return path, true
	}
	return "", false
}

// thumbnail は、カードのサムネイルにする画像を本文から探す。
//
// 本文の先頭から見て最初に現れる、Markdown の画像か YouTube 動画の URL を使う。
// 画像は書かれたままのパス（または http(s) の URL）を返し、ノートからの相対パスの
// 解決は、開いているフォルダを知っているフロントエンド側に任せる。
// YouTube は動画 ID からサムネイル画像の URL を組み立てて返す。
// コードブロックの中に書かれたものは使わない。見つからなければ "" を返す。
func thumbnail(body []byte) string {
	inFence := false
	for line := range strings.SplitSeq(string(body), "\n") {
		l := strings.TrimSpace(line)
		if strings.HasPrefix(l, "```") || strings.HasPrefix(l, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence || l == "" {
			continue
		}

		// 1 行に画像と動画の両方があれば、先に書かれているほうを使う。
		img, imgAt := firstImage(l)
		ytAt := -1
		var ytID string
		if m := youtubeRe.FindStringSubmatchIndex(l); m != nil {
			ytAt, ytID = m[0], l[m[2]:m[3]]
		}
		switch {
		case imgAt >= 0 && (ytAt < 0 || imgAt < ytAt):
			return img
		case ytAt >= 0:
			return fmt.Sprintf(youtubeThumbnail, ytID)
		}
	}
	return ""
}

// firstImage は行の中で最初に現れる、サムネイルに使える画像の参照とその位置を返す。
// 使えない画像（data: URI など）は読み飛ばす。見つからなければ位置に -1 を返す。
func firstImage(line string) (string, int) {
	for _, m := range mdLinkRe.FindAllStringSubmatchIndex(line, -1) {
		if m[3] == m[2] {
			continue // "!" の無いふつうのリンク
		}
		if src := line[m[4]:m[5]]; usableImage(src) {
			return src, m[0]
		}
	}
	return "", -1
}

// usableImage は、画像の参照をカードのサムネイルに使ってよいかを判定する。
// ローカルのパスと http(s) の URL だけを受け付ける。
func usableImage(src string) bool {
	if src == "" || len(src) > maxThumbnailLen {
		return false
	}
	if driveRe.MatchString(src) || !schemeRe.MatchString(src) {
		return true
	}
	lower := strings.ToLower(src)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
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
		// 表のセル区切りは空白に、リンクは表示されている文字だけに落とす。
		l = strings.Trim(l, "|")
		l = strings.ReplaceAll(l, "|", " ")
		l = mdImageTextRe.ReplaceAllString(l, "")
		l = mdLinkTextRe.ReplaceAllString(l, "$1")
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
