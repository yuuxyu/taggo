package mcpserver

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yuuxyu/taggo/internal/atomicfile"
	"github.com/yuuxyu/taggo/internal/meta"
	"github.com/yuuxyu/taggo/internal/model"
	"github.com/yuuxyu/taggo/internal/notefile"
	"github.com/yuuxyu/taggo/internal/store"
)

// 書き込むツールに共通すること。
//
//   - 書き換えるのは頼まれたところだけで、整形し直さない。元のファイルの BOM と改行（LF / CRLF）を保つ。
//   - 新しいノートは一時ファイルに書いてから、上書きしない名前の変更で置く（atomicfile.CreateNew）。
//     既にあるノートは、画面の保存と同じく atomicfile.Replace で置き換える。
//   - 置き換える直前に、読んだときから書き換えられていないかをもう一度確かめる。プロセスをまたぐ
//     ロックは持たないので、画面の保存と重なる隙間をできるだけ短くするため。
//   - サーバーの中の書き込みは、tools.writeMu で 1 つずつにする（クライアントはツールを並べて呼ぶことがある）。
//   - 書いたノートは、ウォッチャーを待たずに DB へ載せる。続けて read_note などで使えるようにするため。

// writeResult は、書き込むツールに共通の結果。
type writeResult struct {
	Name            string   `json:"name" jsonschema:"ノートのファイル名"`
	Written         bool     `json:"written" jsonschema:"ファイルに書き込んだか。dry_run のときや、中身が変わらないときは false"`
	Version         string   `json:"version" jsonschema:"この呼び出しのあとのファイルの版。続けて edit_note を呼ぶときは base_version に渡せる"`
	TagsAdded       []string `json:"tags_added" jsonschema:"この書き込みで付いたタグ"`
	TagsRemoved     []string `json:"tags_removed" jsonschema:"この書き込みで外れたタグ"`
	MissingTagPages []string `json:"missing_tag_pages" jsonschema:"付いたタグのうち、ページがまだ無いもの"`
	MissingLinks    []string `json:"missing_links" jsonschema:"足したリンクのうち、行き先のノートがまだ無いもの"`
	Warnings        []string `json:"warnings" jsonschema:"気をつけること"`
	Content         string   `json:"content,omitempty" jsonschema:"dry_run のとき、書き込むはずの中身"`
}

// writing は、書き込むツールの注釈。
func writing(title string, destructive, idempotent bool) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		Title:           title,
		DestructiveHint: ptr(destructive),
		IdempotentHint:  idempotent,
		OpenWorldHint:   ptr(false),
	}
}

// loadNote は、書き換えるノートと、その今の中身を読む。まだ無いページは書き換えられない。
func (t *tools) loadNote(note string) (page, []byte, error) {
	s := t.lib.store
	p, err := resolve(s, note)
	if err != nil {
		return page{}, nil, err
	}
	if p.entry == nil {
		if len(s.Backlinks(p.asEntry())) == 0 {
			return page{}, nil, errNotFound(note)
		}
		return page{}, nil, fmt.Errorf("「%s」はまだ無いページです。新しく作るなら create_note を使ってください", p.name())
	}
	raw, err := notefile.ReadLimited(p.path)
	if err != nil {
		return page{}, nil, fmt.Errorf("「%s」を読めませんでした: %w", p.name(), err)
	}
	return p, raw, nil
}

// finish は、書き換えたあとの中身 data を（dry_run でなければ）書き込み、結果を作る。
// original は読んだときの中身。中身が変わらなければ書き込まない（同期サービスに無駄な更新を送らないため）。
func (t *tools) finish(p page, original, data []byte, dryRun bool) (writeResult, error) {
	if len(data) > notefile.MaxBytes {
		return writeResult{}, fmt.Errorf("書き換えると %d バイトを超えるため、書き込めません", notefile.MaxBytes)
	}
	if bytes.Equal(original, data) {
		out := t.report(p.path, original, data, false)
		out.Version = notefile.Version(original)
		return out, nil
	}
	if dryRun {
		out := t.report(p.path, original, data, false)
		out.Version = notefile.Version(original)
		out.Content = preview(data)
		return out, nil
	}
	if err := t.replaceNote(p.path, original, data); err != nil {
		return writeResult{}, err
	}
	out := t.report(p.path, original, data, false)
	out.Written = true
	out.Version = notefile.Version(data)
	return out, nil
}

// replaceNote は、path が読んだときの中身 original から変わっていないことを確かめてから data で置き換え、
// DB へ載せ直す。
func (t *tools) replaceNote(path string, original, data []byte) error {
	name := filepath.Base(path)
	current, err := notefile.ReadLimited(path)
	if err != nil {
		return fmt.Errorf("書き込む前に「%s」を読めませんでした: %w", name, err)
	}
	if !bytes.Equal(current, original) {
		return errChanged(name)
	}
	if err := atomicfile.Replace(path, data); err != nil {
		if errors.Is(err, atomicfile.ErrInUse) {
			return fmt.Errorf("「%s」は、%v。少し待ってから、もう一度呼んでください", name, atomicfile.ErrInUse)
		}
		return fmt.Errorf("「%s」を書き込めませんでした: %w", name, err)
	}
	return t.reindex(path)
}

// errChanged は、読んだあとでノートが書き換えられていたときのエラー。
func errChanged(name string) error {
	return fmt.Errorf("「%s」は、読んだあとで書き換えられています（利用者が taggo の画面で書いている途中かもしれません）。"+
		"read_note で読み直してから、もう一度呼んでください", name)
}

// reindex は、書いたノートを読み直して DB へ載せる。
func (t *tools) reindex(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("書いたノートを読めませんでした: %w", err)
	}
	entry, err := meta.Read(path, info)
	if err != nil {
		return err
	}
	entry.RelPath = filepath.Base(path)
	return t.lib.store.Put(entry)
}

var (
	// imageRe は本文の画像（![説明](パス)）。捕まえるのはパス。
	imageRe = regexp.MustCompile(`!\[[^\]\[]*\]\(\s*<?([^)<>\s]+)`)
	// schemeRe は http: などのスキーム。driveRe は Windows のドライブ文字（スキームではない）。
	schemeRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`)
	driveRe  = regexp.MustCompile(`^[a-zA-Z]:[\\/]`)
)

// report は、書く前の中身 before と書いたあとの中身 after を比べて、付いたタグ・外れたタグ・
// ページの無いタグ・行き先の無いリンク・気をつけることをまとめる。新しいノートなら before は nil。
// 書いたあとに呼ぶと、書いたノート自身もページとして数える。
func (t *tools) report(path string, before, after []byte, isNew bool) writeResult {
	s := t.lib.store
	root := s.Root()
	out := writeResult{
		Name:            filepath.Base(path),
		TagsAdded:       []string{},
		TagsRemoved:     []string{},
		MissingTagPages: []string{},
		MissingLinks:    []string{},
		Warnings:        []string{},
	}
	b, a := meta.Parse(before), meta.Parse(after)

	beforeTags, afterTags := model.NormalizeTags(b.Tags), model.NormalizeTags(a.Tags)
	had, has := tagKeys(beforeTags), tagKeys(afterTags)
	own := model.TagKey(model.PageTag(path))
	for _, tag := range afterTags {
		key := model.TagKey(tag)
		if had[key] {
			continue
		}
		out.TagsAdded = append(out.TagsAdded, tag)
		if _, named := model.TagFileName(tag); named && key != own && s.TagPagePath(tag) == "" {
			out.MissingTagPages = append(out.MissingTagPages, tag)
		}
	}
	for _, tag := range beforeTags {
		if !has[model.TagKey(tag)] {
			out.TagsRemoved = append(out.TagsRemoved, tag)
		}
	}

	oldLinks := map[string]bool{}
	for _, link := range b.Links {
		oldLinks[strings.ToLower(link)] = true
	}
	for _, link := range a.Links {
		if oldLinks[strings.ToLower(link)] {
			continue
		}
		target := s.LinkPath(link, path)
		if notefile.CheckPagePath(root, target) != nil {
			out.Warnings = append(out.Warnings, fmt.Sprintf("リンク「%s」は、サブフォルダかフォルダの外を指しています。"+
				"taggo が扱うのはフォルダ直下のノートだけなので、関連ページには出ません", link))
			continue
		}
		if !store.SameNote(target, path) && s.FindNote(target) == "" {
			out.MissingLinks = append(out.MissingLinks, link)
		}
	}

	oldImages := map[string]bool{}
	for _, m := range imageRe.FindAllSubmatch(before, -1) {
		oldImages[string(m[1])] = true
	}
	for _, m := range imageRe.FindAllSubmatch(after, -1) {
		src := string(m[1])
		if oldImages[src] || (schemeRe.MatchString(src) && !driveRe.MatchString(src)) {
			continue
		}
		if !localFileExists(root, path, src) {
			out.Warnings = append(out.Warnings, fmt.Sprintf("画像「%s」が見つかりません。MCP からは画像を置けないので、"+
				"画像は taggo の画面で貼り付けてもらってください", src))
		}
	}

	if isNew && len(afterTags) == 0 {
		out.Warnings = append(out.Warnings, "タグが 1 つも付いていません。検索とリンク元からしかたどれないので、"+
			"tags か本文の [[タグ]] で付けると見つけやすくなります")
	}
	return out
}

// localFileExists は、ノート notePath に書かれたローカルの参照 src のファイルがあるかを確かめる。
// 相対パスはノートの位置、"/" 始まりは開いているフォルダが基準（画面と同じ）。
func localFileExists(root, notePath, src string) bool {
	if unescaped, err := url.PathUnescape(src); err == nil {
		src = unescaped
	}
	var path string
	switch {
	case driveRe.MatchString(src):
		path = src
	case strings.HasPrefix(src, "/") || strings.HasPrefix(src, `\`):
		path = filepath.Join(root, filepath.FromSlash(src))
	default:
		path = filepath.Join(filepath.Dir(notePath), filepath.FromSlash(src))
	}
	_, err := os.Stat(path)
	return err == nil
}

// checkTags は、付けるタグを確かめ、正規化して重複を除いたものを返す。
func checkTags(tags []string) ([]string, error) {
	seen := map[string]bool{}
	out := make([]string, 0, len(tags))
	for _, raw := range tags {
		if strings.ContainsAny(raw, "[]\r\n") {
			return nil, fmt.Errorf("「%s」は [[ ]] の中に書けない文字（[ ] や改行）を含むため、タグにできません", raw)
		}
		tag := model.NormalizeTag(raw)
		if tag == "" {
			return nil, errors.New("tags に空のタグがあります")
		}
		if key := model.TagKey(tag); !seen[key] {
			seen[key] = true
			out = append(out, tag)
		}
	}
	return out, nil
}

// tagKeys は、タグを突き合わせるためのキーの集合を返す。
func tagKeys(tags []string) map[string]bool {
	keys := make(map[string]bool, len(tags))
	for _, tag := range tags {
		keys[model.TagKey(tag)] = true
	}
	return keys
}

// tagLine は、タグを [[タグ]] にして空白で並べた 1 行を返す。
func tagLine(tags []string) string {
	parts := make([]string, len(tags))
	for i, tag := range tags {
		parts[i] = "[[" + tag + "]]"
	}
	return strings.Join(parts, " ")
}

// preview は、dry_run で返す中身。BOM を除き、改行を LF にそろえ、長ければ切り詰める。
func preview(data []byte) string {
	body := strings.ReplaceAll(notefile.Decode(data).Body, "\r\n", "\n")
	if chunk, rest := cutRunes(body, 0, readChunkRunes); rest > 0 {
		return chunk + "\n…（以下略）"
	}
	return body
}

// hasFrontMatter は、本文が Front Matter（先頭の --- で囲んだブロック）で始まるかを判定する。
func hasFrontMatter(body string) bool {
	return len(meta.StripFrontMatter([]byte(body))) != len(body)
}

// registerWrite は、書き込むツールをサーバーに登録する。--read-only のときは呼ばない。
func (t *tools) registerWrite(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:  "create_note",
		Title: "ノートを作る",
		Description: "taggo の書き方で新しいノートを 1 つ作る。既にあるノートは書き換えない（同じ名前があればエラー）。\n" +
			"- title がファイル名（title.md）と先頭の見出し（# title）になる。ファイル名はそのままタグを表すので、タグとして使える名前にする。\n" +
			"- ノートにタグを付けるには、本文に [[タグ]] と書く。tags に渡したタグは、見出しの下の行に [[タグ]] として並べる。\n" +
			"- Front Matter（--- で囲んだ YAML）は書かない。tags: と書いてもタグにはならない。\n" +
			"- タグを付けずにほかのノートを指すには、Markdown のリンク [表示名](ファイル名.md) を使う。ファイル名の空白は %20 で書く。\n" +
			"- 作る前に search_notes と list_tags で、同じ話題のノートが無いか、タグの表記を確かめる。\n" +
			"- dry_run を true にすると、書き込まずに、出来上がる中身と注意点を返す。",
		Annotations: writing("ノートを作る", false, false),
	}, t.createNote)

	mcp.AddTool(s, &mcp.Tool{
		Name:  "add_tags",
		Title: "タグを付ける",
		Description: "既にあるノートにタグを付ける。見出しの下のタグの行（[[タグ]] だけが並んだ行）に [[タグ]] を書き足す。" +
			"タグの行が無ければ、見出しの下に作る。既に付いているタグは書き足さない。ノートを読まずに使える。" +
			"タグを外すときは edit_note を使う。",
		Annotations: writing("タグを付ける", false, true),
	}, t.addTags)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "append_to_note",
		Title:       "ノートに追記する",
		Description: "既にあるノートの末尾に Markdown を書き足す。空行を 1 つ挟んで書き足す。ノートを読まずに使える。",
		Annotations: writing("ノートに追記する", false, false),
	}, t.appendToNote)

	mcp.AddTool(s, &mcp.Tool{
		Name:  "edit_note",
		Title: "ノートを書き換える",
		Description: "既にあるノートの一部を書き換える。edits の old_text を new_text に置き換える（new_text を空にすれば消す）。\n" +
			"- old_text は、本文と空白・改行まで同じで、ノートの中に 1 か所だけある文にする。\n" +
			"- base_version には、read_note が返した version を渡す。読んだあとでノートが書き換えられていれば、" +
			"何も変えずにエラーを返すので、read_note で読み直す。\n" +
			"- edits は、すべて当てはまるときだけまとめて書き込む。\n" +
			"- 頼まれていないところは書き換えない。整形し直さない。\n" +
			"- ファイル名（＝タグ）は変えられない。名前を変えたいときは、利用者に taggo の画面で変えてもらう。",
		InputSchema: schemaFor[editInput](func(p map[string]*jsonschema.Schema) {
			p["edits"].MinItems = ptr(1)
			p["edits"].MaxItems = ptr(maxEdits)
		}),
		Annotations: writing("ノートを書き換える", true, false),
	}, t.editNote)
}
