// Package folderconf は、開いたフォルダの直下に置く taggo の設定ファイル
// （.taggo.json）を読み書きする。
//
// 持つのは、一覧の先頭にピン留めしたノートだけ。ピン留めはそのフォルダの Wiki を
// どう見せるかの情報なので、アプリの設定（%APPDATA%）ではなくノートと同じフォルダに置き、
// ノートと一緒に同期・バックアップされるようにする。
//
// ファイルは利用者が手で直してもよい。taggo が知らない項目があっても、
// 書き込むときに消さずに残す。
package folderconf

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/yuuxyu/taggo/internal/atomicfile"
)

// FileName はフォルダの直下に置く設定ファイルの名前。
const FileName = ".taggo.json"

// keyPinned は設定ファイルの中でピン留めを持つ項目の名前。
const keyPinned = "pinned"

// Config はフォルダの設定。
type Config struct {
	// Pinned はピン留めしたノートのファイル名。ピン留めした順に並ぶ。
	// taggo が扱うのはフォルダ直下のノートだけなので、パスではなくファイル名で持つ。
	Pinned []string `json:"pinned"`
}

// Path は root の設定ファイルのパスを返す。
func Path(root string) string { return filepath.Join(root, FileName) }

// Load は root の設定ファイルを読む。
//
// ファイルが無ければ空の設定を返す。
// JSON として壊れていればエラーを返す（呼び出し側は空の設定として扱えばよい）。
func Load(root string) (Config, error) {
	raw, err := atomicfile.ReadFile(Path(root))
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := json.Unmarshal(raw, &c); err != nil {
		return Config{}, fmt.Errorf("%s を読めませんでした: %w", FileName, err)
	}
	c.Pinned = cleanNames(c.Pinned)
	return c, nil
}

// Save は root の設定ファイルへ c を書き込む。
//
// 既存のファイルにある taggo の知らない項目は残す。既存のファイルが JSON として
// 壊れていれば、利用者の書いた内容を消さないよう書き込まずにエラーを返す。
// 書き込みは同じフォルダの一時ファイルへ書いてから置き換える。
func Save(root string, c Config) error {
	path := Path(root)
	fields := map[string]json.RawMessage{}
	raw, err := atomicfile.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return err
	default:
		if err := json.Unmarshal(raw, &fields); err != nil {
			return fmt.Errorf("%s が壊れているため書き換えません。直すか消してからやり直してください: %w", FileName, err)
		}
		if fields == nil {
			fields = map[string]json.RawMessage{}
		}
	}

	pinned, err := json.Marshal(cleanNames(c.Pinned))
	if err != nil {
		return err
	}
	fields[keyPinned] = pinned

	out, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	if err := atomicfile.Replace(path, out); err != nil {
		return fmt.Errorf("%s へ書き込めませんでした: %w", FileName, err)
	}
	return nil
}

// cleanNames はファイル名の並びから、空のものとフォルダを含むものを除き、
// 大文字小文字を無視して重複を除く。順番は保つ。
func cleanNames(names []string) []string {
	out := make([]string, 0, len(names))
	seen := map[string]struct{}{}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
			continue
		}
		key := strings.ToLower(name)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, name)
	}
	return out
}

// IsConfigFile は、path が root の設定ファイルかを判定する。
// Windows に合わせて、パスの大文字小文字は区別しない。
func IsConfigFile(root, path string) bool {
	return strings.EqualFold(filepath.Clean(path), filepath.Clean(Path(root)))
}
