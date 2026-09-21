package store

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/tidwall/buntdb"
	"github.com/yuuxyu/taggo/internal/model"
)

// sameTagLimit は「同じタグのノート」として出す件数の上限。
// 右の列に収まり、眺めて選べる程度の数に抑える。
const sameTagLimit = 5

// RelatedPage は関連ページとして並べるカード 1 枚ぶんの情報。
type RelatedPage struct {
	// Target は本文に書かれているリンクの行き先。行き先のノートが見つからなかった
	// ときに、何が書かれていたのかを示すために使う。
	Target string `json:"target,omitempty"`
	// Path は行き先の実体。見つからなければ空になる。
	Path    string   `json:"path,omitempty"`
	Title   string   `json:"title"`
	RelPath string   `json:"relPath,omitempty"`
	Preview string   `json:"preview,omitempty"`
	Tags    []string `json:"tags,omitempty"`
	// Kind は行き先の種類。タグページには画像や音声も並ぶので、見せ方を切り替えるのに使う。
	Kind model.Kind `json:"kind,omitempty"`
	// CloudOnly は行き先の中身がクラウド上にしか無いこと。サムネイルを
	// 取りに行くとダウンロードが始まるので、画面側で控えるために使う。
	CloudOnly bool `json:"cloudOnly,omitempty"`
	// TagPage は行き先がタグページなら、そのタグ。
	TagPage string `json:"tagPage,omitempty"`
}

// Related は 1 つのノートから見た関連ページ。
type Related struct {
	// Outgoing はそのノートがリンクしているページ。本文に出てくる順に並ぶ。
	Outgoing []RelatedPage `json:"outgoing"`
	// Incoming はそのノートへリンクしているページ。タイトル順に並ぶ。
	Incoming []RelatedPage `json:"incoming"`
	// SameTag はタグが重なっているノート。リンクで既に出ているものは除き、
	// 重なるタグの多い順、同じなら更新日時の新しい順に、上限件数まで並ぶ。
	SameTag []RelatedPage `json:"sameTag"`
	// Tagged は、そのノートがタグページのときに、説明しているタグが付いたファイル。
	// 種類を問わず更新日時の新しい順に、上限件数まで並ぶ。
	Tagged []RelatedPage `json:"tagged"`
	// TaggedTotal は Tagged の上限を超えた分も含めた件数。
	TaggedTotal int `json:"taggedTotal"`
	// Duplicates は、同じタグをタグページとして宣言しているほかのノート。
	// 1 つのタグにタグページは 1 つのはずなので、あれば画面で警告する。
	Duplicates []RelatedPage `json:"duplicates"`
}

// EmptyRelated は関連ページが 1 件も無い状態。
// フロントエンドでは配列として扱うので、nil ではなく空スライスで返す。
func EmptyRelated() Related {
	return Related{
		Outgoing:   []RelatedPage{},
		Incoming:   []RelatedPage{},
		SameTag:    []RelatedPage{},
		Tagged:     []RelatedPage{},
		Duplicates: []RelatedPage{},
	}
}

// Related は、そのノートが参照しているページ、そのノートを参照している
// ページ、タグが重なるノートをまとめて返す。
//
// リンクは Markdown の記法で書かれたパスなので、行き先はリンク元のノートの
// 位置を基準に 1 つへ定まる。名前が同じというだけで、無関係なフォルダの
// ノートを関連に出すことはない。
func (s *Store) Related(entry *model.Entry) Related {
	related := EmptyRelated()
	if entry == nil {
		return related
	}

	s.mu.RLock()
	// リンク 1 件ごとの行き先。本文に出てくる順を保つため、まとめずに持つ。
	destinations := make([]string, len(entry.Links))
	for i, link := range entry.Links {
		destinations[i] = pickOne(s.notePaths[linkTarget(link, entry.Path, s.root)])
	}
	sources := pathsExcept(s.backlinks[noteKey(entry.Path)], entry.Path)
	s.mu.RUnlock()

	seen := map[string]struct{}{}
	for i, link := range entry.Links {
		path := destinations[i]
		if path == entry.Path {
			continue // 自分自身への参照は関連に出さない
		}
		if path == "" {
			// まだ存在しないノートへのリンク。書きかけのメモでは珍しくないので、
			// 行き先が無いことが分かる形で残す。
			related.Outgoing = append(related.Outgoing, RelatedPage{Target: link, Title: linkLabel(link)})
			continue
		}
		if _, dup := seen[path]; dup {
			continue
		}
		seen[path] = struct{}{}
		if e, ok := s.Get(path); ok {
			related.Outgoing = append(related.Outgoing, relatedPage(link, e))
		}
	}

	for _, path := range sources {
		seen[path] = struct{}{}
		if e, ok := s.Get(path); ok {
			related.Incoming = append(related.Incoming, relatedPage("", e))
		}
	}
	sort.Slice(related.Incoming, func(i, j int) bool {
		if related.Incoming[i].Title != related.Incoming[j].Title {
			return related.Incoming[i].Title < related.Incoming[j].Title
		}
		return related.Incoming[i].Path < related.Incoming[j].Path
	})

	seen[entry.Path] = struct{}{}
	s.fillTagPage(entry, &related, seen)
	related.SameTag = s.sameTagNotes(entry.Tags, seen)
	return related
}

// sameTagNotes は tags と 1 つ以上タグが重なるノートを返す。
// exclude に含まれるパス（自分自身やリンクで既に出ているノート）は除く。
//
// タグの逆引き索引は持っていないので、全件をなめる。検索と同じく
// 上限 2 万件の範囲なら、プレビューを開くたびに走らせても十分に速い。
func (s *Store) sameTagNotes(tags []string, exclude map[string]struct{}) []RelatedPage {
	pages := []RelatedPage{}
	if len(tags) == 0 {
		return pages
	}
	want := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		want[strings.ToLower(tag)] = struct{}{}
	}

	type candidate struct {
		entry  *model.Entry
		shared int
	}
	var found []candidate
	// 更新日時の新しい順になめるので、重なる数が同じなら新しいノートが先に来る。
	_ = s.db.View(func(tx *buntdb.Tx) error {
		return tx.Descend(idxModTime, func(_, raw string) bool {
			e := decodeEntry(raw)
			if e == nil || e.Kind != model.KindMarkdown {
				return true
			}
			if _, skip := exclude[e.Path]; skip {
				return true
			}
			shared := 0
			for _, tag := range e.Tags {
				if _, ok := want[strings.ToLower(tag)]; ok {
					shared++
				}
			}
			if shared > 0 {
				found = append(found, candidate{entry: e, shared: shared})
			}
			return true
		})
	})

	sort.SliceStable(found, func(i, j int) bool { return found[i].shared > found[j].shared })
	if len(found) > sameTagLimit {
		found = found[:sameTagLimit]
	}
	for _, c := range found {
		pages = append(pages, relatedPage("", c.entry))
	}
	return pages
}

// noteKeysOf は、そのエントリがリンク先として名指されうるキーを返す。
// リンク先になれるのは Markdown だけなので、それ以外は索引に載せない。
func noteKeysOf(e *model.Entry) []string {
	if e.Kind != model.KindMarkdown {
		return nil
	}
	return []string{noteKey(e.Path)}
}

// linkTarget はリンク 1 件の行き先を索引のキーへ直す。
//
// リンク元のあるフォルダを基準に絶対パスへ直す。"/" 始まりは開いている
// フォルダが基準。キーでは Markdown の拡張子を落として .md と .markdown の
// 違いを吸収し、小文字へ揃える（Windows はパスの大文字小文字を区別しない）。
func linkTarget(link, fromPath, root string) string {
	base := filepath.Dir(fromPath)
	if strings.HasPrefix(link, "/") || strings.HasPrefix(link, `\`) {
		base = root
	}
	return noteKey(filepath.Join(base, filepath.FromSlash(link)))
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

// pathsExcept は集合をパス順のスライスへ直す。自分自身への参照は落とす。
// 呼び出し元が s.mu を握っていること。
func pathsExcept(set map[string]struct{}, self string) []string {
	out := make([]string, 0, len(set))
	for path := range set {
		if path == self {
			continue
		}
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

func relatedPage(target string, e *model.Entry) RelatedPage {
	return RelatedPage{
		Target:    target,
		Path:      e.Path,
		Title:     e.Title,
		RelPath:   e.RelPath,
		Preview:   e.Preview,
		Tags:      e.Tags,
		Kind:      e.Kind,
		CloudOnly: e.CloudOnly,
		TagPage:   e.TagPage,
	}
}
