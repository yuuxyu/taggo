package store

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/yuuxyu/taggo/internal/model"
)

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
}

// Related は 1 つのノートから見た関連ページ。
type Related struct {
	// Outgoing はそのノートがリンクしているページ。本文に出てくる順に並ぶ。
	Outgoing []RelatedPage `json:"outgoing"`
	// Incoming はそのノートへリンクしているページ。タイトル順に並ぶ。
	Incoming []RelatedPage `json:"incoming"`
}

// Related は、そのノートが参照しているページと、そのノートを参照している
// ページをまとめて返す。
//
// リンクは Markdown の記法で書かれたパスなので、行き先はリンク元のノートの
// 位置を基準に 1 つへ定まる。名前が同じというだけで、無関係なフォルダの
// ノートを関連に出すことはない。
func (s *Store) Related(entry *model.Entry) Related {
	related := Related{Outgoing: []RelatedPage{}, Incoming: []RelatedPage{}}
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
	return related
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
		Target:  target,
		Path:    e.Path,
		Title:   e.Title,
		RelPath: e.RelPath,
		Preview: e.Preview,
		Tags:    e.Tags,
	}
}
