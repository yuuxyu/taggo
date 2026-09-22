// Package settings は、アプリの好み（起動時に開くフォルダ、並び順など）を
// %APPDATA%\taggo\settings.json へ読み書きする。
//
// ここに置くのは「消えても初期状態へ戻るだけ」のものに限る。ノートのタグや
// 走査結果のように、ノートから導けるものやノートの中身の写しは決して保存しない。
// 設定を一度も変えていなければ、ファイル自体を作らない。
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/yuuxyu/taggo/internal/store"
)

// CurrentVersion は設定ファイルの形式の版。形式を変えたときに、古いファイルを移し替える目印にする。
const CurrentVersion = 1

// 1 回の走査で展開する件数の上限として選べる範囲。
// 下限は小さすぎて「続きを読み込む」ばかりになるのを、上限は RAM の使いすぎを防ぐ。
const (
	MinScanLimit = 1000
	MaxScanLimit = 100000
)

// StartupMode は、起動時にどのフォルダを開くか。
type StartupMode string

const (
	// StartupNone は何も開かずに起動する。
	StartupNone StartupMode = "none"
	// StartupLast は前回開いたフォルダを開く。
	StartupLast StartupMode = "last"
	// StartupFixed は決めておいたフォルダを開く。
	StartupFixed StartupMode = "fixed"
)

// Theme は画面の配色。
type Theme string

const (
	ThemeSystem Theme = "system"
	ThemeLight  Theme = "light"
	ThemeDark   Theme = "dark"
)

// Settings は設定ファイルの中身。フロントエンドへもそのまま渡す。
type Settings struct {
	Version int `json:"version"`
	// StartupMode は起動時にどのフォルダを開くか。
	StartupMode StartupMode `json:"startupMode"`
	// StartupFolder は StartupFixed のときに開くフォルダ。
	StartupFolder string `json:"startupFolder,omitempty"`
	// LastFolder は StartupLast のときに開く、前回開いたフォルダ。
	// 開いたフォルダの履歴を必要以上に残さないよう、StartupLast のときだけ記録する。
	LastFolder string `json:"lastFolder,omitempty"`
	// Sort は起動時の一覧の並び順。ツールバーで変えた並び順は保存しない。
	Sort store.SortOrder `json:"sort"`
	// ScanLimit は 1 回の走査で展開する件数の上限。
	ScanLimit int `json:"scanLimit"`
	// Theme は画面の配色。
	Theme Theme `json:"theme"`
}

// Default は、設定ファイルが無いときの設定を返す。設定を導入する前の振る舞いと同じにしてある。
func Default() Settings {
	return Settings{
		Version:     CurrentVersion,
		StartupMode: StartupNone,
		Sort:        store.SortModifiedDesc,
		ScanLimit:   store.MaxEntries,
		Theme:       ThemeSystem,
	}
}

// Validate は、画面から保存しようとした値が正しいかを確かめる。
func (s Settings) Validate() error {
	switch s.StartupMode {
	case StartupNone, StartupLast:
	case StartupFixed:
		if s.StartupFolder == "" {
			return errors.New("起動時に開くフォルダを選んでください")
		}
	default:
		return fmt.Errorf("起動時に開くフォルダの指定が正しくありません: %q", s.StartupMode)
	}
	if !validSort(s.Sort) {
		return fmt.Errorf("並び順の指定が正しくありません: %q", s.Sort)
	}
	if s.ScanLimit < MinScanLimit || s.ScanLimit > MaxScanLimit {
		return fmt.Errorf("走査の上限件数は %d〜%d 件の範囲で指定してください", MinScanLimit, MaxScanLimit)
	}
	if !validTheme(s.Theme) {
		return fmt.Errorf("テーマの指定が正しくありません: %q", s.Theme)
	}
	return nil
}

// StartupPath は起動時に開くフォルダを返す。何も開かないなら空文字を返す。
func (s Settings) StartupPath() string {
	switch s.StartupMode {
	case StartupLast:
		return s.LastFolder
	case StartupFixed:
		return s.StartupFolder
	}
	return ""
}

// normalize は、知らない値や範囲外の値を項目ごとに既定値へ直す。
// 手で書き換えたファイルや、新しい版の taggo が書いたファイルでも起動できるようにするため。
func (s Settings) normalize() Settings {
	def := Default()
	s.Version = CurrentVersion
	switch s.StartupMode {
	case StartupNone, StartupLast:
	case StartupFixed:
		if s.StartupFolder == "" {
			s.StartupMode = StartupNone
		}
	default:
		s.StartupMode = def.StartupMode
	}
	if !validSort(s.Sort) {
		s.Sort = def.Sort
	}
	if s.ScanLimit < MinScanLimit || s.ScanLimit > MaxScanLimit {
		s.ScanLimit = def.ScanLimit
	}
	if !validTheme(s.Theme) {
		s.Theme = def.Theme
	}
	return s
}

func validSort(s store.SortOrder) bool {
	switch s {
	case store.SortModifiedDesc, store.SortNameAsc, store.SortRelevance:
		return true
	}
	return false
}

func validTheme(t Theme) bool {
	switch t {
	case ThemeSystem, ThemeLight, ThemeDark:
		return true
	}
	return false
}

// DefaultPath は設定ファイルの置き場所（%APPDATA%\taggo\settings.json）を返す。
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("設定の保存先を決められませんでした: %w", err)
	}
	return filepath.Join(dir, "taggo", "settings.json"), nil
}

// Store は設定ファイルとその中身を束ねたもの。すべてのメソッドは並行呼び出しに耐える。
type Store struct {
	path string
	// warning は読み込み時に既定値へ戻したときの理由。
	warning string

	mu  sync.Mutex
	cur Settings
}

// Load は path の設定ファイルを読み込む。
//
// ファイルが無ければ既定値で始め、ファイルは作らない。読めない・壊れているときも
// 既定値で起動できるようにし、その理由を LoadWarning で返す。壊れたファイルは、
// 次の保存で上書きされないよう settings.json.bak へ退避しておく。
// path が空なら保存先が無い扱いになり、Save はエラーを返す。
func Load(path string) *Store {
	st := &Store{path: path, cur: Default()}
	if path == "" {
		return st
	}

	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return st
	}
	if err != nil {
		st.warning = fmt.Sprintf("設定ファイルを読めなかったため、既定の設定で起動しました: %v", err)
		return st
	}

	// 書かれていない項目は既定値のままにする。
	loaded := Default()
	if err := json.Unmarshal(raw, &loaded); err != nil {
		backup := path + ".bak"
		if rerr := os.Rename(path, backup); rerr != nil {
			st.warning = fmt.Sprintf("設定ファイルが壊れていたため、既定の設定で起動しました: %v", err)
			return st
		}
		st.warning = fmt.Sprintf("設定ファイルが壊れていたため、既定の設定で起動しました。"+
			"元のファイルは %s に退避しました: %v", backup, err)
		return st
	}
	st.cur = loaded.normalize()
	return st
}

// LoadWarning は、読み込み時に既定値へ戻したときの理由を返す。問題が無ければ空文字。
func (st *Store) LoadWarning() string {
	return st.warning
}

// Path は設定ファイルの置き場所を返す。
func (st *Store) Path() string {
	return st.path
}

// Get は今の設定を返す。
func (st *Store) Get() Settings {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.cur
}

// Save は設定を確かめてからファイルへ書き込み、書き込めたものを今の設定にする。
func (st *Store) Save(s Settings) error {
	if err := s.Validate(); err != nil {
		return err
	}
	s.Version = CurrentVersion

	st.mu.Lock()
	defer st.mu.Unlock()
	if err := st.write(s); err != nil {
		return err
	}
	st.cur = s
	return nil
}

// RememberFolder は、開いたフォルダを「前回開いたフォルダ」として覚える。
// StartupLast 以外のときは何も記録しない。
func (st *Store) RememberFolder(dir string) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.cur.StartupMode != StartupLast || st.cur.LastFolder == dir {
		return nil
	}
	next := st.cur
	next.LastFolder = dir
	if err := st.write(next); err != nil {
		return err
	}
	st.cur = next
	return nil
}

// write は設定をファイルへ書き込む。呼び出し元が st.mu を握っていること。
//
// 同じフォルダの一時ファイルへ書いてから置き換えるので、書き込みの途中で落ちても
// 前の設定ファイルが壊れることはない。
func (st *Store) write(s Settings) error {
	if st.path == "" {
		return errors.New("設定の保存先が分からないため保存できません")
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("設定を書き出せませんでした: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(st.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("設定の保存先フォルダを作れませんでした: %w", err)
	}
	tmp, err := os.CreateTemp(dir, "settings-*.tmp")
	if err != nil {
		return fmt.Errorf("設定を保存できませんでした: %w", err)
	}
	defer os.Remove(tmp.Name()) // 置き換えに成功していれば、もう無いので何もしない

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("設定を保存できませんでした: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("設定を保存できませんでした: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("設定を保存できませんでした: %w", err)
	}
	if err := os.Rename(tmp.Name(), st.path); err != nil {
		return fmt.Errorf("設定を保存できませんでした: %w", err)
	}
	return nil
}
