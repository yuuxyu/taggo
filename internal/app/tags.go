package app

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/yuuxyu/taggo/internal/meta"
	"github.com/yuuxyu/taggo/internal/model"
)

// TagEditResult は 1 ファイルへのタグ編集の結果。
// 一括編集では成功と失敗が混在しうるため、ファイル単位で結果を返す。
type TagEditResult struct {
	Path string `json:"path"`
	// OK が false のとき Error に理由が入る。
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	// Entry は書き込み成功時の最新状態。失敗時は nil。
	Entry *model.Entry `json:"entry,omitempty"`
}

// SetTags は 1 ファイルのタグを置き換える。
func (a *App) SetTags(path string, tags []string) TagEditResult {
	return a.applyTags(path, func([]string) []string {
		return tags
	})
}

// AddTags は複数ファイルへ同じタグを追加する。既に付いているファイルはそのまま。
func (a *App) AddTags(paths []string, tags []string) []TagEditResult {
	return a.bulk(paths, func(current []string) []string {
		return append(append([]string{}, current...), tags...)
	})
}

// RemoveTags は複数ファイルから指定タグを取り除く。
func (a *App) RemoveTags(paths []string, tags []string) []TagEditResult {
	remove := map[string]struct{}{}
	for _, t := range model.NormalizeTags(tags) {
		remove[strings.ToLower(t)] = struct{}{}
	}

	return a.bulk(paths, func(current []string) []string {
		kept := make([]string, 0, len(current))
		for _, t := range current {
			if _, drop := remove[strings.ToLower(t)]; drop {
				continue
			}
			kept = append(kept, t)
		}
		return kept
	})
}

// ReplaceTags は複数ファイルのタグをまとめて同じ内容に置き換える。
func (a *App) ReplaceTags(paths []string, tags []string) []TagEditResult {
	return a.bulk(paths, func([]string) []string { return tags })
}

// bulk は複数ファイルへ同じ変換を適用する。
// 途中で失敗しても残りの処理は続け、結果をファイル単位で返す。
func (a *App) bulk(paths []string, transform func(current []string) []string) []TagEditResult {
	out := make([]TagEditResult, 0, len(paths))
	for _, p := range paths {
		out = append(out, a.applyTags(p, transform))
	}
	return out
}

// applyTags は 1 ファイルのタグを transform の結果で書き換え、
// 書き込みに成功した場合だけインメモリ DB を更新する。
//
// 要件どおり、読み取り専用ファイルや Markdown 以外のファイルはエラーとして返し、
// メモリ上だけ更新して実ファイルと食い違わせることはしない。
func (a *App) applyTags(path string, transform func(current []string) []string) TagEditResult {
	entry, ok := a.store.Get(path)
	if !ok {
		return failed(path, fmt.Errorf("エントリが見つかりません: %s", path))
	}
	if entry.CloudOnly {
		// 書き込むにはまず中身をダウンロードすることになる。黙って通信を起こさず、
		// 取り込むかどうかは利用者に決めてもらう。
		return failed(path, fmt.Errorf("クラウド上にだけあるファイルです。取り込んでから編集してください: %s", entry.Name))
	}
	if !entry.Writable {
		return failed(path, fmt.Errorf("読み取り専用のファイルです: %s", entry.Name))
	}
	// タグの書き換えは中身の読み直しを伴う。走査のあとで「オンラインのみ」へ
	// 戻っていれば、ここで開くとダウンロードが始まるので確かめ直す。
	if err := a.ensureLocal(entry); err != nil {
		return failed(path, err)
	}

	next := model.NormalizeTags(transform(entry.Tags))
	if err := meta.WriteTags(path, next); err != nil {
		return failed(path, describeWriteError(entry, err))
	}

	// 実ファイルへ書けたので、読み直してインメモリ DB を更新する。
	info, err := os.Stat(path)
	if err != nil {
		return failed(path, fmt.Errorf("書き込み後のファイルを読めませんでした: %w", err))
	}
	updated, err := meta.Read(path, info)
	if err != nil {
		return failed(path, fmt.Errorf("書き込み後のメタデータを読めませんでした: %w", err))
	}
	updated.RelPath = entry.RelPath

	if err := a.store.Put(updated); err != nil {
		return failed(path, err)
	}
	a.emit(EventEntryChanged, map[string]any{"path": path, "entry": updated})

	return TagEditResult{Path: path, OK: true, Entry: updated}
}

// describeWriteError は、ライブラリ由来のエラーを UI に出して意味の通る文言へ整える。
func describeWriteError(entry *model.Entry, err error) error {
	switch {
	case errors.Is(err, meta.ErrUnsupported):
		return fmt.Errorf("対応していないファイル形式です: %s", entry.Name)
	case errors.Is(err, os.ErrPermission):
		return fmt.Errorf("書き込み権限がありません: %s", entry.Name)
	default:
		return fmt.Errorf("%s への書き込みに失敗しました: %w", entry.Name, err)
	}
}

func failed(path string, err error) TagEditResult {
	return TagEditResult{Path: path, OK: false, Error: err.Error()}
}
