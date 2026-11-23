package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yuuxyu/taggo/internal/model"
	"github.com/yuuxyu/taggo/internal/notefile"
	"github.com/yuuxyu/taggo/internal/store"
)

const (
	// overviewTopTags・overviewRecent は、get_overview で返すタグとノートの数。
	overviewTopTags = 30
	overviewRecent  = 10
)

// limits は、一覧を返すツールの limit の既定値と上限。
type limits struct{ def, max int }

var (
	notesLimits     = limits{def: 20, max: 100}
	searchLimits    = limits{def: 20, max: 50}
	backlinksLimits = limits{def: 20, max: 100}
	tagsLimits      = limits{def: 50, max: 200}
)

// or は、limit が省かれていれば既定値を返す。
func (l limits) or(limit int) int { return orDefault(limit, l.def) }

// readChunkRunes は、read_note が 1 回に返す本文の文字数の上限。テストでは小さくする。
var readChunkRunes = 100_000

// tools は、ツールの処理をまとめたもの。
type tools struct {
	lib *library
	// writeMu は、ノートへの書き込みを 1 つずつにする。クライアントはツールを並べて呼ぶことがあり、
	// 読んでから書くまでの間に、別の書き込みが割り込まないようにするため。
	writeMu sync.Mutex
}

// ---- 入力の型と、スキーマの補い ----

// limitOffset は、一覧を返すツールに共通の引数。
type limitOffset struct {
	Limit  int `json:"limit,omitempty" jsonschema:"返す件数"`
	Offset int `json:"offset,omitempty" jsonschema:"先頭から飛ばす件数。前の結果の next_offset を渡すと続きを読める"`
}

// schemaFor は、型 T から入力のスキーマを作り、edit で既定値や選択肢を補う。
func schemaFor[T any](edit func(props map[string]*jsonschema.Schema)) *jsonschema.Schema {
	s, err := jsonschema.For[T](nil)
	if err != nil {
		panic(fmt.Sprintf("入力のスキーマを作れません: %v", err))
	}
	if edit != nil {
		edit(s.Properties)
	}
	return s
}

// 既定値はスキーマに書かず（default）、ハンドラーで補う。SDK（jsonschema-go）は、引数が null の
// 呼び出しに既定値を補おうとして panic し、サーバーごと落ちるため。既定値は説明で伝える。

// setLimit は limit に上限を設定し、既定値を説明に書く。
func setLimit(props map[string]*jsonschema.Schema, l limits) {
	p := props["limit"]
	p.Minimum = ptr(1.0)
	p.Maximum = ptr(float64(l.max))
	p.Description = fmt.Sprintf("返す件数（既定 %d、%d まで）", l.def, l.max)
	props["offset"].Minimum = ptr(0.0)
}

// setEnum は文字列の引数に選択肢を設定する。
func setEnum(p *jsonschema.Schema, values ...string) {
	p.Enum = make([]any, len(values))
	for i, v := range values {
		p.Enum[i] = v
	}
}

// orDefault は、引数が省かれていれば（ゼロ値なら）既定値を返す。
func orDefault[T comparable](v, def T) T {
	var zero T
	if v == zero {
		return def
	}
	return v
}

func ptr[T any](v T) *T { return &v }

// readOnly は、読むだけのツールの注釈。
func readOnly(title string) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{Title: title, ReadOnlyHint: true, OpenWorldHint: ptr(false)}
}

// register は、読むツールをサーバーに登録する。
func (t *tools) register(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:  "get_overview",
		Title: "Wiki の概要",
		Description: "開いているフォルダのノートの全体像を返す。ノートとタグの数、ピン留めしたノート、" +
			"よく使われているタグ、最近更新したノート。会話の始めに Wiki の様子をつかむときに使う。",
		Annotations: readOnly("Wiki の概要"),
	}, t.overview)

	mcp.AddTool(s, &mcp.Tool{
		Name:  "list_notes",
		Title: "ノートの一覧",
		Description: "ノートを一覧する。tag を渡すと、本文に [[tag]] と書かれたノート（そのタグを付けたノート）だけを返し、" +
			"そのタグのページ（tag.md）は tag_page で別に返す。タグの大文字小文字と空白の連なりは区別しない。" +
			"tag を渡さなければ、すべてのノートを並べる。",
		InputSchema: schemaFor[listNotesInput](func(p map[string]*jsonschema.Schema) {
			setLimit(p, notesLimits)
			setEnum(p["sort"], "modified_desc", "name_asc")
		}),
		Annotations: readOnly("ノートの一覧"),
	}, t.listNotes)

	mcp.AddTool(s, &mcp.Tool{
		Name:  "search_notes",
		Title: "ノートを全文検索",
		Description: "ノートを全文検索する。空白で区切った語をすべて含むノートを返す（大文字小文字は区別しない）。" +
			"語はタイトル・ファイル名・タグ・本文全体から探す。結果には、語が見つかった行を行番号付きで 3 行まで付ける。" +
			"検索語と同じ名前のタグのページは先頭に出す。",
		InputSchema: schemaFor[searchInput](func(p map[string]*jsonschema.Schema) {
			setLimit(p, searchLimits)
		}),
		Annotations: readOnly("ノートを全文検索"),
	}, t.searchNotes)

	mcp.AddTool(s, &mcp.Tool{
		Name:  "read_note",
		Title: "ノートを読む",
		Description: "ノートの本文（Markdown をそのまま）と、付いているタグ・リンク先・リンク元の数を返す。" +
			"version はファイルの中身を表す値（SHA-256）。" +
			"まだ無いページ（タグとして使われているがファイルの無いページ）を指したときは、exists が false になり、" +
			"そのページを指しているノートの数を返す。",
		InputSchema: schemaFor[readNoteInput](func(p map[string]*jsonschema.Schema) {
			p["offset"].Minimum = ptr(0.0)
		}),
		Annotations: readOnly("ノートを読む"),
	}, t.readNote)

	mcp.AddTool(s, &mcp.Tool{
		Name:  "get_backlinks",
		Title: "リンク元を調べる",
		Description: "そのノートを指しているノート（リンク元）を返す。リンク元は 2 種類ある。" +
			"そのノートが表すタグ（golang.md なら [[golang]]）を本文に書いたノート（via に tag）と、" +
			"Markdown のリンクでそのノートへリンクしているノート（via に link）。まだ無いページについても調べられる。",
		InputSchema: schemaFor[backlinksInput](func(p map[string]*jsonschema.Schema) {
			setLimit(p, backlinksLimits)
		}),
		Annotations: readOnly("リンク元を調べる"),
	}, t.backlinks)

	mcp.AddTool(s, &mcp.Tool{
		Name:  "list_tags",
		Title: "タグの一覧",
		Description: "本文で使われているタグを、使われている件数と、タグのページの有無と一緒に返す。" +
			"ノートを書く前に、既にあるタグの表記を確かめるのに使う。" +
			"missing_page_only で、使われているのにページがまだ無いタグだけに絞れる。",
		InputSchema: schemaFor[listTagsInput](func(p map[string]*jsonschema.Schema) {
			setLimit(p, tagsLimits)
			setEnum(p["sort"], "count_desc", "name_asc")
		}),
		Annotations: readOnly("タグの一覧"),
	}, t.listTags)
}

// ---- get_overview ----

type overviewOutput struct {
	Folder    string        `json:"folder" jsonschema:"フォルダの名前"`
	NoteCount int           `json:"note_count"`
	TagCount  int           `json:"tag_count"`
	NotLoaded int           `json:"not_loaded" jsonschema:"読み込みの上限で読んでいないノートの数"`
	Pinned    []NoteSummary `json:"pinned"`
	TopTags   []tagInfo     `json:"top_tags" jsonschema:"よく使われているタグ（30 件まで）"`
	Recent    []NoteSummary `json:"recent" jsonschema:"最近更新したノート（10 件まで）"`
}

// tagInfo は、タグ 1 つの使われ方。
type tagInfo struct {
	Tag   string  `json:"tag"`
	Count int     `json:"count" jsonschema:"このタグを付けたノートの数"`
	Page  *string `json:"page" jsonschema:"タグのページのファイル名。まだ無ければ null"`
}

func (t *tools) overview(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, overviewOutput, error) {
	if err := t.lib.wait(ctx); err != nil {
		return nil, overviewOutput{}, err
	}
	s := t.lib.store

	pinnedResult, err := s.Search(store.SearchOptions{})
	if err != nil {
		return nil, overviewOutput{}, err
	}
	pinned := make([]NoteSummary, 0, pinnedResult.Head)
	for _, e := range pinnedResult.Entries[:pinnedResult.Head] {
		pinned = append(pinned, summarize(s, e))
	}

	stats := s.TagStats()
	top := make([]tagInfo, 0, overviewTopTags)
	for _, st := range stats {
		if st.Count == 0 || len(top) == overviewTopTags {
			break
		}
		top = append(top, tagInfo{Tag: st.Tag, Count: st.Count, Page: pageName(st.Page)})
	}

	entries := s.Entries(store.SortModifiedDesc)
	recent := make([]NoteSummary, 0, overviewRecent)
	for _, e := range entries[:min(len(entries), overviewRecent)] {
		recent = append(recent, summarize(s, e))
	}

	return nil, overviewOutput{
		Folder:    filepath.Base(s.Root()),
		NoteCount: len(entries),
		TagCount:  s.TagCount(),
		NotLoaded: t.lib.notLoaded(),
		Pinned:    pinned,
		TopTags:   top,
		Recent:    recent,
	}, nil
}

// ---- list_notes ----

type listNotesInput struct {
	Tag           string `json:"tag,omitempty" jsonschema:"このタグを付けたノートに絞る。先頭の # は無視する"`
	ModifiedAfter string `json:"modified_after,omitempty" jsonschema:"この日時より後に更新したノートに絞る（2026-11-01 か RFC 3339）"`
	Sort          string `json:"sort,omitempty" jsonschema:"並び順。modified_desc は更新日時の新しい順、name_asc はファイル名の順"`
	limitOffset
}

type listNotesOutput struct {
	Tag        string        `json:"tag,omitempty"`
	TagPage    *NoteSummary  `json:"tag_page,omitempty" jsonschema:"tag を渡したとき、そのタグのページ。まだ無ければ省く"`
	Notes      []NoteSummary `json:"notes"`
	Total      int           `json:"total"`
	NextOffset *int          `json:"next_offset,omitempty" jsonschema:"続きがあるときに、次の offset に渡す値"`
}

func (t *tools) listNotes(ctx context.Context, _ *mcp.CallToolRequest, in listNotesInput) (*mcp.CallToolResult, listNotesOutput, error) {
	if err := t.lib.wait(ctx); err != nil {
		return nil, listNotesOutput{}, err
	}
	s := t.lib.store

	var after time.Time
	if in.ModifiedAfter != "" {
		var err error
		if after, err = parseTime(in.ModifiedAfter); err != nil {
			return nil, listNotesOutput{}, err
		}
	}

	order := store.SortModifiedDesc
	if in.Sort == "name_asc" {
		order = store.SortNameAsc
	}
	var out listNotesOutput
	var entries []*model.Entry
	if in.Tag != "" {
		tag := model.NormalizeTag(in.Tag)
		if tag == "" {
			return nil, listNotesOutput{}, errors.New("tag にタグの名前を入れてください")
		}
		out.Tag = tag
		pagePath := s.TagPagePath(tag)
		if e, ok := s.Get(pagePath); ok {
			summary := summarize(s, e)
			out.TagPage = &summary
		}
		entries = sortEntries(s.TaggedWith(tag), order)
		entries = slices.DeleteFunc(entries, func(e *model.Entry) bool { return e.Path == pagePath })
	} else {
		entries = s.Entries(order)
	}
	if !after.IsZero() {
		entries = slices.DeleteFunc(entries, func(e *model.Entry) bool { return !e.ModTime.After(after) })
	}

	page, next := window(entries, in.Offset, notesLimits.or(in.Limit))
	out.Notes = make([]NoteSummary, len(page))
	for i, e := range page {
		out.Notes[i] = summarize(s, e)
	}
	out.Total = len(entries)
	out.NextOffset = next
	return nil, out, nil
}

// sortEntries は、エントリを並び順どおりに並べ替える。
func sortEntries(entries []*model.Entry, order store.SortOrder) []*model.Entry {
	slices.SortStableFunc(entries, func(a, b *model.Entry) int {
		if order == store.SortNameAsc {
			return strings.Compare(strings.ToLower(a.RelPath), strings.ToLower(b.RelPath))
		}
		return b.ModTime.Compare(a.ModTime)
	})
	return entries
}

// parseTime は、日付（2026-11-01。その日の 0 時）か RFC 3339 の日時を読む。
func parseTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation(time.DateOnly, s, time.Local); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("modified_after の「%s」を日時として読めません。2026-11-01 か RFC 3339（2026-11-01T09:00:00+09:00）で書いてください", s)
}

// ---- search_notes ----

type searchInput struct {
	Query string `json:"query" jsonschema:"探す語。空白で区切ると、すべてを含むノートを返す"`
	limitOffset
}

type searchOutput struct {
	Query      string      `json:"query"`
	Results    []searchHit `json:"results"`
	Total      int         `json:"total"`
	NextOffset *int        `json:"next_offset,omitempty" jsonschema:"続きがあるときに、次の offset に渡す値"`
}

type searchHit struct {
	Name     string      `json:"name"`
	Title    string      `json:"title"`
	Tags     []string    `json:"tags"`
	Modified string      `json:"modified"`
	Matches  []lineMatch `json:"matches" jsonschema:"語が見つかった行（3 行まで）。タイトルやタグだけで見つかったときは空"`
}

func (t *tools) searchNotes(ctx context.Context, _ *mcp.CallToolRequest, in searchInput) (*mcp.CallToolResult, searchOutput, error) {
	if err := t.lib.wait(ctx); err != nil {
		return nil, searchOutput{}, err
	}
	if strings.TrimSpace(in.Query) == "" {
		return nil, searchOutput{}, errors.New("query に探す語を入れてください。すべてのノートを並べるなら list_notes を使ってください")
	}
	hits, err := fullTextSearch(ctx, t.lib.store, in.Query)
	if err != nil {
		return nil, searchOutput{}, err
	}

	page, next := window(hits, in.Offset, searchLimits.or(in.Limit))
	results := make([]searchHit, len(page))
	for i, h := range page {
		results[i] = searchHit{
			Name:     filepath.Base(h.entry.Path),
			Title:    h.entry.Title,
			Tags:     nonNil(h.entry.Tags),
			Modified: formatTime(h.entry.ModTime),
			Matches:  h.matches,
		}
	}
	return nil, searchOutput{Query: in.Query, Results: results, Total: len(hits), NextOffset: next}, nil
}

// ---- read_note ----

type readNoteInput struct {
	Note   string `json:"note" jsonschema:"ノートのファイル名（golang.md。拡張子は省いてよい）"`
	Offset int    `json:"offset,omitempty" jsonschema:"長いノートの続きを読むときの文字位置。前の結果の next_offset を渡す"`
}

type readNoteOutput struct {
	Name          string    `json:"name"`
	Exists        bool      `json:"exists" jsonschema:"ファイルがあるか。false ならまだ無いページ"`
	Title         string    `json:"title"`
	Modified      string    `json:"modified,omitempty"`
	Version       string    `json:"version,omitempty" jsonschema:"ファイルの中身を表す値（SHA-256）"`
	Tags          []tagRef  `json:"tags"`
	Links         []linkRef `json:"links" jsonschema:"本文の、ほかのノートへの Markdown のリンク"`
	BacklinkCount int       `json:"backlink_count" jsonschema:"このノートを指しているノートの数（get_backlinks で一覧できる）"`
	Truncated     bool      `json:"truncated" jsonschema:"本文が長いため、途中までしか返していない"`
	NextOffset    *int      `json:"next_offset,omitempty" jsonschema:"続きを読むときに、次の offset に渡す値"`
}

type tagRef struct {
	Tag  string  `json:"tag"`
	Page *string `json:"page" jsonschema:"タグのページのファイル名。まだ無ければ null"`
}

type linkRef struct {
	Target string  `json:"target" jsonschema:"本文に書かれたリンク先"`
	Note   *string `json:"note" jsonschema:"リンク先のノートのファイル名。まだ無ければ null"`
}

func (t *tools) readNote(ctx context.Context, _ *mcp.CallToolRequest, in readNoteInput) (*mcp.CallToolResult, readNoteOutput, error) {
	if err := t.lib.wait(ctx); err != nil {
		return nil, readNoteOutput{}, err
	}
	s := t.lib.store
	p, err := resolve(s, in.Note)
	if err != nil {
		return nil, readNoteOutput{}, err
	}
	backlinks := len(s.Backlinks(p.asEntry()))

	if p.entry == nil {
		if backlinks == 0 {
			return nil, readNoteOutput{}, errNotFound(in.Note)
		}
		out := readNoteOutput{
			Name:          p.name(),
			Title:         model.PageTag(p.path),
			Tags:          []tagRef{},
			Links:         []linkRef{},
			BacklinkCount: backlinks,
		}
		return textResult(out, fmt.Sprintf("「%s」はまだ無いページです。%d 件のノートがこのページを指しています（get_backlinks で一覧できます）。",
			out.Name, backlinks)), out, nil
	}

	e := p.entry
	raw, err := notefile.ReadLimited(e.Path)
	if err != nil {
		return nil, readNoteOutput{}, fmt.Errorf("「%s」を読めませんでした: %w", p.name(), err)
	}
	out := readNoteOutput{
		Name:          p.name(),
		Exists:        true,
		Title:         e.Title,
		Modified:      formatTime(e.ModTime),
		Version:       notefile.Version(raw),
		Tags:          make([]tagRef, len(e.Tags)),
		Links:         make([]linkRef, len(e.Links)),
		BacklinkCount: backlinks,
	}
	for i, tag := range e.Tags {
		out.Tags[i] = tagRef{Tag: tag, Page: pageName(s.TagPagePath(tag))}
	}
	for i, link := range e.Links {
		out.Links[i] = linkRef{Target: link, Note: pageName(s.FindNote(s.LinkPath(link, e.Path)))}
	}

	// BOM を除き、改行は LF にそろえて返す。位置（offset）は文字数で数える。
	body := strings.ReplaceAll(notefile.Decode(raw).Body, "\r\n", "\n")
	chunk, rest := cutRunes(body, in.Offset, readChunkRunes)
	if rest > 0 {
		out.Truncated = true
		next := in.Offset + utf8.RuneCountInString(chunk)
		out.NextOffset = &next
	}
	return textResult(out, chunk), out, nil
}

// cutRunes は s の offset 文字目から n 文字を切り出し、残りの文字数も返す。
func cutRunes(s string, offset, n int) (string, int) {
	r := []rune(s)
	offset = min(max(offset, 0), len(r))
	end := min(offset+n, len(r))
	return string(r[offset:end]), len(r) - end
}

// textResult は、構造化した結果（JSON）に、別のテキストを添えた結果を作る。
// 本文を JSON の中に埋めると、エスケープで長く読みにくくなるので、テキストのまま返す。
// 構造化した結果を読まないクライアントのために、JSON もテキストで添える。
func textResult(structured any, text string) *mcp.CallToolResult {
	data, err := json.Marshal(structured)
	if err != nil {
		data = []byte("{}")
	}
	return &mcp.CallToolResult{Content: []mcp.Content{
		&mcp.TextContent{Text: string(data)},
		&mcp.TextContent{Text: text},
	}}
}

// ---- get_backlinks ----

type backlinksInput struct {
	Note string `json:"note" jsonschema:"ノートのファイル名（golang.md。拡張子は省いてよい）"`
	limitOffset
}

type backlinksOutput struct {
	Note       string         `json:"note"`
	Exists     bool           `json:"exists" jsonschema:"ファイルがあるか。false ならまだ無いページ"`
	Tag        string         `json:"tag" jsonschema:"このノートが表すタグ"`
	Backlinks  []backlinkItem `json:"backlinks"`
	Total      int            `json:"total"`
	NextOffset *int           `json:"next_offset,omitempty" jsonschema:"続きがあるときに、次の offset に渡す値"`
}

type backlinkItem struct {
	Name     string   `json:"name"`
	Title    string   `json:"title"`
	Tags     []string `json:"tags"`
	Modified string   `json:"modified"`
	Via      []string `json:"via" jsonschema:"どう指しているか。tag はタグ（[[ ]]）で、link は Markdown のリンクで"`
}

func (t *tools) backlinks(ctx context.Context, _ *mcp.CallToolRequest, in backlinksInput) (*mcp.CallToolResult, backlinksOutput, error) {
	if err := t.lib.wait(ctx); err != nil {
		return nil, backlinksOutput{}, err
	}
	s := t.lib.store
	p, err := resolve(s, in.Note)
	if err != nil {
		return nil, backlinksOutput{}, err
	}
	links := s.Backlinks(p.asEntry())
	if p.entry == nil && len(links) == 0 {
		return nil, backlinksOutput{}, errNotFound(in.Note)
	}

	page, next := window(links, in.Offset, backlinksLimits.or(in.Limit))
	items := make([]backlinkItem, len(page))
	for i, b := range page {
		via := make([]string, 0, 2)
		if b.ByTag {
			via = append(via, "tag")
		}
		if b.ByLink {
			via = append(via, "link")
		}
		items[i] = backlinkItem{
			Name:     filepath.Base(b.Entry.Path),
			Title:    b.Entry.Title,
			Tags:     nonNil(b.Entry.Tags),
			Modified: formatTime(b.Entry.ModTime),
			Via:      via,
		}
	}
	return nil, backlinksOutput{
		Note:       p.name(),
		Exists:     p.entry != nil,
		Tag:        model.PageTag(p.path),
		Backlinks:  items,
		Total:      len(links),
		NextOffset: next,
	}, nil
}

// ---- list_tags ----

type listTagsInput struct {
	Query           string `json:"query,omitempty" jsonschema:"タグの名前の一部。含むタグだけに絞る（大文字小文字は区別しない）"`
	MissingPageOnly bool   `json:"missing_page_only,omitempty" jsonschema:"true なら、使われているのにページがまだ無いタグだけにする"`
	Sort            string `json:"sort,omitempty" jsonschema:"並び順。count_desc は使われている件数の多い順、name_asc は名前の順"`
	limitOffset
}

type listTagsOutput struct {
	Tags       []tagInfo `json:"tags"`
	Total      int       `json:"total"`
	NextOffset *int      `json:"next_offset,omitempty" jsonschema:"続きがあるときに、次の offset に渡す値"`
}

func (t *tools) listTags(ctx context.Context, _ *mcp.CallToolRequest, in listTagsInput) (*mcp.CallToolResult, listTagsOutput, error) {
	if err := t.lib.wait(ctx); err != nil {
		return nil, listTagsOutput{}, err
	}
	query := strings.ToLower(strings.TrimSpace(in.Query))
	stats := slices.DeleteFunc(t.lib.store.TagStats(), func(st store.TagStat) bool {
		// どのノートもファイル名のタグのページなので、誰も使っていないタグまで並べると、
		// ノートの一覧とほとんど同じになる。使われているタグだけにする。
		if st.Count == 0 {
			return true
		}
		if query != "" && !strings.Contains(strings.ToLower(st.Tag), query) {
			return true
		}
		if in.MissingPageOnly {
			// ファイル名にできないタグは、ページを作れないので候補にしない。
			_, named := model.TagFileName(st.Tag)
			return st.Page != "" || !named
		}
		return false
	})
	if in.Sort == "name_asc" {
		slices.SortStableFunc(stats, func(a, b store.TagStat) int {
			return strings.Compare(strings.ToLower(a.Tag), strings.ToLower(b.Tag))
		})
	}

	page, next := window(stats, in.Offset, tagsLimits.or(in.Limit))
	tags := make([]tagInfo, len(page))
	for i, st := range page {
		tags[i] = tagInfo{Tag: st.Tag, Count: st.Count, Page: pageName(st.Page)}
	}
	return nil, listTagsOutput{Tags: tags, Total: len(stats), NextOffset: next}, nil
}
