// Package store は走査結果をインメモリの BuntDB に展開し、検索を提供する。
//
// 要件どおりディスクへの永続化は行わない。BuntDB は ":memory:" で開き、
// アプリ終了時にメモリごと破棄される。ファイル側のメタデータが常に正なので、
// 失われて困る情報はここには無い。
package store

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/tidwall/buntdb"
	"github.com/yuuxyu/taggo/internal/model"
)

// MaxEntries は 1 回の走査で展開するファイル件数の上限。
// RAM の過剰消費を避けるための安全弁で、要件が定める 2 万件に合わせている。
const MaxEntries = 20000

// キー空間の接頭辞。BuntDB は単一のキー空間なので、用途ごとに接頭辞で分ける。
const (
	entryPrefix = "entry:"
	idxModTime  = "entry_modtime"
	// idxRelPath はフォルダ階層込みの相対パスで並べるためのインデックス。
	// タイトル（見出し）ではなく実際の
	// ファイルパスで並べることで、同じフォルダの中身が自然にまとまる。
	idxRelPath = "entry_relpath"
)

// SortOrder は一覧の並び順。要件どおり手動並べ替えは提供せず、この 3 つに限る。
type SortOrder string

const (
	SortModifiedDesc SortOrder = "modified_desc"
	SortNameAsc      SortOrder = "name_asc"
	SortRelevance    SortOrder = "relevance"
)

// Store は BuntDB とタグ索引を束ねたもの。すべてのメソッドは並行呼び出しに耐える。
type Store struct {
	db *buntdb.DB

	// mu は tagCounts・リンクとタグの索引・ピン留めを守る。BuntDB 自体は内部で同期しているが、
	// これらの派生データとの整合を取るために別途必要になる。
	mu sync.RWMutex
	// tagCounts はタグごとの使用件数。オートコンプリートの候補順に使う。
	tagCounts map[string]int
	// tagSpelling は正規化キー（小文字）から、実際に使われている表記への対応。
	tagSpelling map[string]string
	// tagged はタグの逆引き。小文字のタグから、そのタグが付いているノートのパスを引く。
	// Front Matter の tags: と本文の [[タグ]] を区別せずに載せる。
	tagged map[string]map[string]struct{}
	// backlinks はノート間リンクの逆引き。キーはリンクの行き先を解決したパス。
	backlinks map[string]map[string]struct{}
	// notePaths はリンクの順引き。Markdown の拡張子を除いた小文字のパスから、
	// その実体のパスを引く。
	notePaths map[string]map[string]struct{}
	// tagPages はタグのページの索引。ファイル名が表すタグ（小文字）から、そのノートの
	// パスを引く。a.md と a.markdown のように同じタグを表すファイルが並びうるので集合で持つ。
	tagPages map[string]map[string]struct{}
	// pins はピン留めしたノートのパス。ピン留めした順に並ぶ。
	pins []string
	// root は現在展開しているフォルダ。
	root string
}

// Open はインメモリの BuntDB を開き、検索用インデックスを張る。
func Open() (*Store, error) {
	db, err := buntdb.Open(":memory:")
	if err != nil {
		return nil, fmt.Errorf("インメモリ DB の初期化に失敗しました: %w", err)
	}

	s := &Store{
		db:          db,
		tagCounts:   map[string]int{},
		tagSpelling: map[string]string{},
		tagged:      map[string]map[string]struct{}{},
		backlinks:   map[string]map[string]struct{}{},
		notePaths:   map[string]map[string]struct{}{},
		tagPages:    map[string]map[string]struct{}{},
	}
	if err := s.createIndexes(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// createIndexes は更新日時順・名前順のソートに使うカスタムインデックスを作る。
// BuntDB のインデックスは値（JSON）から取り出した項目で並ぶので、
// ソートのたびに全件を読み直す必要がなくなる。
func (s *Store) createIndexes() error {
	if err := s.db.CreateIndex(idxModTime, entryPrefix+"*", buntdb.IndexJSON("modTime")); err != nil {
		return fmt.Errorf("更新日時インデックスの作成に失敗しました: %w", err)
	}
	if err := s.db.CreateIndex(idxRelPath, entryPrefix+"*", buntdb.IndexJSON("relPath")); err != nil {
		return fmt.Errorf("パスインデックスの作成に失敗しました: %w", err)
	}
	return nil
}

// Close は DB を閉じる。
func (s *Store) Close() error { return s.db.Close() }

// Root は現在展開しているフォルダのパスを返す。
func (s *Store) Root() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.root
}

// Reset は全データを捨てて、新しいルートを設定する。フォルダを選び直したときに呼ぶ。
func (s *Store) Reset(root string) error {
	if err := s.db.Update(func(tx *buntdb.Tx) error {
		return tx.DeleteAll()
	}); err != nil {
		return fmt.Errorf("インメモリ DB の初期化に失敗しました: %w", err)
	}
	// buntdb の DeleteAll は中身だけを捨ててインデックス定義は保持するため、
	// ここで張り直す必要はない。

	s.mu.Lock()
	defer s.mu.Unlock()
	s.tagCounts = map[string]int{}
	s.tagSpelling = map[string]string{}
	s.tagged = map[string]map[string]struct{}{}
	s.backlinks = map[string]map[string]struct{}{}
	s.notePaths = map[string]map[string]struct{}{}
	s.tagPages = map[string]map[string]struct{}{}
	s.pins = nil
	s.root = root
	return nil
}

// Put はエントリを登録または更新する。既存エントリがあれば派生データも差し替える。
func (s *Store) Put(e *model.Entry) error {
	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("エントリの JSON 変換に失敗しました: %w", err)
	}

	var previous *model.Entry
	err = s.db.Update(func(tx *buntdb.Tx) error {
		old, replaced, err := tx.Set(entryPrefix+e.Path, string(data), nil)
		if err != nil {
			return err
		}
		if replaced {
			previous = decodeEntry(old)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("エントリの登録に失敗しました: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if previous != nil {
		s.unindexDerived(previous)
	}
	s.indexDerived(e)
	return nil
}

// PutAll は複数エントリを 1 トランザクションで登録する。初回走査のように
// 大量に投入する経路では、1 件ずつ Put するより大幅に速い。
func (s *Store) PutAll(entries []*model.Entry) error {
	encoded := make([]string, len(entries))
	for i, e := range entries {
		data, err := json.Marshal(e)
		if err != nil {
			return fmt.Errorf("エントリの JSON 変換に失敗しました: %w", err)
		}
		encoded[i] = string(data)
	}

	previous := make([]*model.Entry, len(entries))
	err := s.db.Update(func(tx *buntdb.Tx) error {
		for i, e := range entries {
			old, replaced, err := tx.Set(entryPrefix+e.Path, encoded[i], nil)
			if err != nil {
				return err
			}
			if replaced {
				previous[i] = decodeEntry(old)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("エントリの一括登録に失敗しました: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for i, e := range entries {
		if previous[i] != nil {
			s.unindexDerived(previous[i])
		}
		s.indexDerived(e)
	}
	return nil
}

// Delete はエントリを取り除く。存在しないパスを渡してもエラーにはしない。
func (s *Store) Delete(path string) error {
	var removed *model.Entry
	err := s.db.Update(func(tx *buntdb.Tx) error {
		old, err := tx.Delete(entryPrefix + path)
		if err == buntdb.ErrNotFound {
			return nil
		}
		if err != nil {
			return err
		}
		removed = decodeEntry(old)
		return nil
	})
	if err != nil {
		return fmt.Errorf("エントリの削除に失敗しました: %w", err)
	}
	if removed == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.unindexDerived(removed)
	return nil
}

// Get は 1 件のエントリを返す。
func (s *Store) Get(path string) (*model.Entry, bool) {
	var e *model.Entry
	_ = s.db.View(func(tx *buntdb.Tx) error {
		raw, err := tx.Get(entryPrefix + path)
		if err != nil {
			return nil
		}
		e = decodeEntry(raw)
		return nil
	})
	return e, e != nil
}

// Count は登録済みエントリ数を返す。
func (s *Store) Count() int {
	n := 0
	_ = s.db.View(func(tx *buntdb.Tx) error {
		return tx.AscendKeys(entryPrefix+"*", func(_, _ string) bool {
			n++
			return true
		})
	})
	return n
}

// TagCount はタグの種類数を返す。
func (s *Store) TagCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.tagCounts)
}

// decodeEntry は JSON 文字列を Entry へ戻す。壊れていれば nil を返す。
// 保存しているのは自分で書いた JSON なので、実際には起こらない想定の防御。
func decodeEntry(raw string) *model.Entry {
	var e model.Entry
	if err := json.Unmarshal([]byte(raw), &e); err != nil {
		return nil
	}
	return &e
}

// indexDerived はタグ件数とリンク・タグの索引を、エントリの内容に合わせて加算する。
// 呼び出し元が s.mu を握っていること。
func (s *Store) indexDerived(e *model.Entry) {
	for _, tag := range e.Tags {
		key := strings.ToLower(tag)
		s.tagCounts[key]++
		if _, ok := s.tagSpelling[key]; !ok {
			s.tagSpelling[key] = tag
		}
		addPath(s.tagged, key, e.Path)
	}
	for _, link := range e.Links {
		addPath(s.backlinks, linkTarget(link, e.Path, s.root), e.Path)
	}
	addPath(s.notePaths, noteKey(e.Path), e.Path)
	if key := tagPageKey(e); key != "" {
		addPath(s.tagPages, key, e.Path)
	}
}

// unindexDerived は indexDerived の逆操作。
// 呼び出し元が s.mu を握っていること。
func (s *Store) unindexDerived(e *model.Entry) {
	for _, tag := range e.Tags {
		key := strings.ToLower(tag)
		removePath(s.tagged, key, e.Path)
		if s.tagCounts[key] <= 1 {
			delete(s.tagCounts, key)
			delete(s.tagSpelling, key)
			continue
		}
		s.tagCounts[key]--
	}
	for _, link := range e.Links {
		removePath(s.backlinks, linkTarget(link, e.Path, s.root), e.Path)
	}
	removePath(s.notePaths, noteKey(e.Path), e.Path)
	if key := tagPageKey(e); key != "" {
		removePath(s.tagPages, key, e.Path)
	}
}

// addPath は索引 index のキー key の集合へ path を加える。
func addPath(index map[string]map[string]struct{}, key, path string) {
	if index[key] == nil {
		index[key] = map[string]struct{}{}
	}
	index[key][path] = struct{}{}
}

// removePath は索引 index のキー key の集合から path を除く。空になった集合はキーごと消す。
func removePath(index map[string]map[string]struct{}, key, path string) {
	delete(index[key], path)
	if len(index[key]) == 0 {
		delete(index, key)
	}
}
