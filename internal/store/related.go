package store

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/yuuxyu/taggo/internal/model"
)

// groupLimit は 1 つのグループに並べるノートの上限（先頭のタグのページは数えない）。
// ノートの下に並べて眺められる程度に抑え、超えた分は件数だけを返す。
const groupLimit = 24

// RelatedPage は関連ページとして並べるカード 1 枚ぶんの情報。
type RelatedPage struct {
	// Target は本文に書かれている Markdown のリンクの行き先。行き先のノートが
	// 見つからなかったときに、何が書かれていたのかを示すために使う。
	Target string `json:"target,omitempty"`
	// Tag は、このカードがタグのページとしてグループの先頭に置かれたときのタグ。
	Tag string `json:"tag,omitempty"`
	// Path は行き先の実体。まだ無ければ空になる。
	Path      string `json:"path,omitempty"`
	Title     string `json:"title"`
	RelPath   string `json:"relPath,omitempty"`
	Preview   string `json:"preview,omitempty"`
	Thumbnail string `json:"thumbnail,omitempty"`
	// CloudOnly は行き先の中身がクラウド上にしか無いこと。本文を読むと
	// ダウンロードが始まるので、画面側でそれと分かるように出すために使う。
	CloudOnly bool `json:"cloudOnly,omitempty"`
}

// RelatedGroup は関連ページの 1 グループ。
//
// 最初のグループは「開いているノートを指しているノート」（このノートが表すタグを持つノートと、
// Markdown のリンクでこのノートへリンクしているノート）、タグのグループは「そのタグのページと、
// そのタグを持つノート」、Markdown のリンクのグループは「このノートがリンクしているノート」になる。
type RelatedGroup struct {
	// Tag はグループのタグ。Markdown のリンクのグループでは空になる。
	Tag string `json:"tag,omitempty"`
	// Page はグループの先頭に置くタグのページ。ページがまだ無ければ Path が空の
	// 「まだ無いノート」になる。開いているノート自身がそのタグのページのとき、
	// 前のグループで表示済みのとき、ファイル名にできないタグのときは nil。
	Page *RelatedPage `json:"page,omitempty"`
	// Pages はグループに並べるノート。タグのグループでは、開いているノートと
	// 同じタグを多く持つノートほど先に来る。
	Pages []RelatedPage `json:"pages"`
	// More は上限を超えたため並べなかったノートの数。
	More int `json:"more"`
}

// Related は 1 つのノートから見た関連ページ。
type Related struct {
	// Groups は関連ページのグループ。開いているノートを指しているノートのグループ（リンク元）、
	// 付いているタグごとのグループ（タグの順）、Markdown のリンク先のグループの順に並ぶ。
	// 前のグループで表示したノートは、後のグループには出さない。空のグループは含めない。
	Groups []RelatedGroup `json:"groups"`
	// MissingLinks は本文の Markdown のリンクのうち、行き先がまだ無いもの（書かれたまま）。
	// 本文ではこれらのリンクの色を変える。
	MissingLinks []string `json:"missingLinks"`
	// MissingTags はノートのタグのうち、ページがまだ無いもの。
	// 本文ではこれらの [[タグ]] の色を変える。
	MissingTags []string `json:"missingTags"`
}

// EmptyRelated は関連ページが 1 件も無い状態。
// フロントエンドでは配列として扱うので、nil ではなく空スライスで返す。
func EmptyRelated() Related {
	return Related{
		Groups:       []RelatedGroup{},
		MissingLinks: []string{},
		MissingTags:  []string{},
	}
}

// tagGroupSource は、タグのグループを組み立てるために索引から写し取った情報。
type tagGroupSource struct {
	tag     string
	page    string   // タグのページのパス。まだ無ければ空
	named   bool     // ページのファイル名にできるタグか
	members []string // そのタグを持つノートのパス
}

// Related は、そのノートの関連ページをグループに分けて返す。
//
// タグは Front Matter の tags: と本文の [[タグ]] を区別しない。
// Markdown のリンクは書かれたパスなので、行き先はリンク元のノートの位置を基準に 1 つへ定まる。
func (s *Store) Related(entry *model.Entry) Related {
	related := EmptyRelated()
	if entry == nil {
		return related
	}

	// 索引から要るものを写し取ってから、ロックを放してエントリを読む。
	s.mu.RLock()
	ownKey := tagPageKey(entry)
	// このノートを指しているノート。このノートが表すタグを持つものと、Markdown のリンクで
	// このノートへリンクしているものは、どちらもリンク元として同じグループに入れる。
	own := tagGroupSource{
		tag:     s.tagSpelling[ownKey],
		members: sortedPaths(unionPaths(s.tagged[ownKey], s.backlinks[noteKey(entry.Path)])),
	}
	if own.tag == "" {
		own.tag = model.PageTag(entry.Path)
	}
	sources := make([]tagGroupSource, 0, len(entry.Tags))
	for _, tag := range entry.Tags {
		_, named := model.TagFileName(tag)
		sources = append(sources, tagGroupSource{
			tag:     tag,
			page:    pickOne(s.tagPages[model.TagKey(tag)]),
			named:   named,
			members: sortedPaths(s.tagged[strings.ToLower(tag)]),
		})
	}
	destinations := make([]string, len(entry.Links))
	for i, link := range entry.Links {
		destinations[i] = pickOne(s.notePaths[linkTarget(link, entry.Path, s.root)])
	}
	s.mu.RUnlock()

	shown := map[string]struct{}{entry.Path: {}}
	mine := map[string]struct{}{}
	for _, tag := range entry.Tags {
		mine[strings.ToLower(tag)] = struct{}{}
	}

	// このノートを指しているノート（リンク元）を最初のグループにする。見出しはこのノートが
	// 表すタグで、先頭に置くタグのページは開いているノート自身なので、先頭のカードは置かない。
	if pages, more := s.rankByTags(own.members, mine, shown); len(pages) > 0 {
		related.Groups = append(related.Groups, RelatedGroup{Tag: own.tag, Pages: pages, More: more})
	}

	for _, src := range sources {
		group := RelatedGroup{Tag: src.tag}
		switch _, done := shown[src.page]; {
		case src.page == "" && src.named:
			group.Page = &RelatedPage{Tag: src.tag, Title: src.tag}
			related.MissingTags = append(related.MissingTags, src.tag)
		case src.page != "" && !done:
			if e, ok := s.Get(src.page); ok {
				page := relatedPage(e)
				page.Tag = src.tag
				group.Page = &page
				shown[src.page] = struct{}{}
			}
		}
		group.Pages, group.More = s.rankByTags(src.members, mine, shown)
		if group.Page != nil || len(group.Pages) > 0 {
			related.Groups = append(related.Groups, group)
		}
	}

	if group := s.linkGroup(entry, destinations, shown, &related); len(group.Pages) > 0 {
		related.Groups = append(related.Groups, group)
	}
	return related
}

// rankByTags は paths のノートを、tags と重なるタグの多い順（同じなら更新日時の新しい順）に
// 並べ、shown に無いものを上限まで返す。返したノートは shown に加える。
// 上限を超えて返さなかった数も返す。
func (s *Store) rankByTags(paths []string, tags, shown map[string]struct{}) ([]RelatedPage, int) {
	type candidate struct {
		entry  *model.Entry
		shared int
	}
	found := make([]candidate, 0, len(paths))
	for _, path := range paths {
		if _, done := shown[path]; done {
			continue
		}
		e, ok := s.Get(path)
		if !ok {
			continue
		}
		shared := 0
		for _, tag := range e.Tags {
			if _, ok := tags[strings.ToLower(tag)]; ok {
				shared++
			}
		}
		found = append(found, candidate{entry: e, shared: shared})
	}
	sort.SliceStable(found, func(i, j int) bool {
		a, b := found[i], found[j]
		if a.shared != b.shared {
			return a.shared > b.shared
		}
		if !a.entry.ModTime.Equal(b.entry.ModTime) {
			return a.entry.ModTime.After(b.entry.ModTime)
		}
		return a.entry.Path < b.entry.Path
	})

	more := max(len(found)-groupLimit, 0)
	found = found[:len(found)-more]
	pages := make([]RelatedPage, len(found))
	for i, c := range found {
		pages[i] = relatedPage(c.entry)
		shown[c.entry.Path] = struct{}{}
	}
	return pages, more
}

// linkGroup は、このノートが Markdown のリンクでリンクしているノートのグループを組み立てる。
// 本文でリンクしている順に行き先を並べる。リンク元は最初のグループに入れるので、ここには入れない。
// 行き先がまだ無いリンクは「まだ無いノート」として残し、related.MissingLinks にも入れる。
func (s *Store) linkGroup(entry *model.Entry, destinations []string, shown map[string]struct{}, related *Related) RelatedGroup {
	group := RelatedGroup{Pages: []RelatedPage{}}
	add := func(page RelatedPage) {
		if len(group.Pages) >= groupLimit {
			group.More++
			return
		}
		group.Pages = append(group.Pages, page)
	}

	missing := map[string]struct{}{}
	for i, link := range entry.Links {
		path := destinations[i]
		if path == "" {
			key := strings.ToLower(link)
			if _, dup := missing[key]; dup {
				continue
			}
			missing[key] = struct{}{}
			related.MissingLinks = append(related.MissingLinks, link)
			add(RelatedPage{Target: link, Title: linkLabel(link)})
			continue
		}
		if _, done := shown[path]; done {
			continue
		}
		if e, ok := s.Get(path); ok {
			page := relatedPage(e)
			page.Target = link
			add(page)
			shown[path] = struct{}{}
		}
	}
	return group
}

// linkTarget はリンク 1 件の行き先を索引のキーへ直す。
//
// リンク元のあるフォルダを基準に絶対パスへ直す。"/" 始まりは開いている
// フォルダが基準。キーでは Markdown の拡張子を落として .md と .markdown の
// 違いを吸収し、小文字へ揃える（Windows はパスの大文字小文字を区別しない）。
func linkTarget(link, fromPath, root string) string {
	return noteKey(linkPath(link, fromPath, root))
}

// linkPath はリンク 1 件の行き先を、ファイルシステム上のパスへ直す。
// リンク元のあるフォルダが基準で、"/" 始まりは開いているフォルダが基準。
func linkPath(link, fromPath, root string) string {
	base := filepath.Dir(fromPath)
	if strings.HasPrefix(link, "/") || strings.HasPrefix(link, `\`) {
		base = root
	}
	return filepath.Join(base, filepath.FromSlash(link))
}

// LinkPath は fromPath のノートに書かれたリンクの行き先を、ファイルシステム上の
// パスとして返す。索引のキーと違って、大文字小文字と拡張子は書かれたまま残す。
// まだ無いノートを作るときに、どこへ作るのかを決めるために使う。
func (s *Store) LinkPath(link, fromPath string) string {
	return linkPath(link, fromPath, s.Root())
}

// noteKey は Markdown のパスを索引のキーへ直す。
func noteKey(path string) string {
	return strings.ToLower(trimMarkdownExt(filepath.Clean(path)))
}

func trimMarkdownExt(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".md" || ext == ".markdown" {
		return path[:len(path)-len(ext)]
	}
	return path
}

// linkLabel はリンクの表示名。拡張子を除いたファイル名を使う。
func linkLabel(link string) string {
	name := link
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	return trimMarkdownExt(name)
}

// unionPaths は 2 つのパスの集合を合わせた新しい集合を返す。
func unionPaths(a, b map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{}, len(a)+len(b))
	for path := range a {
		out[path] = struct{}{}
	}
	for path := range b {
		out[path] = struct{}{}
	}
	return out
}

// sortedPaths は集合をパス順のスライスへ直す。
// 呼び出し元が s.mu を握っていること。
func sortedPaths(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for path := range set {
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

// pickOne は候補から 1 つ選ぶ。同じ場所に a.md と a.markdown が並んでいる
// ような場合に、どちらを指すのかを決めうちにするための順序。
// 呼び出し元が s.mu を握っていること。
func pickOne(candidates map[string]struct{}) string {
	best := ""
	for path := range candidates {
		if best == "" || path < best {
			best = path
		}
	}
	return best
}

func relatedPage(e *model.Entry) RelatedPage {
	return RelatedPage{
		Path:      e.Path,
		Title:     e.Title,
		RelPath:   e.RelPath,
		Preview:   e.Preview,
		Thumbnail: e.Thumbnail,
		CloudOnly: e.CloudOnly,
	}
}
