// Package scan は指定フォルダ配下を走査し、対応ファイルのメタデータを読み取る。
package scan

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/yuuxyu/taggo/internal/cloudfile"
	"github.com/yuuxyu/taggo/internal/meta"
	"github.com/yuuxyu/taggo/internal/model"
)

// ErrLimitReached は、展開件数の上限に達して走査を打ち切ったことを表す。
// 走査結果そのものは有効なので、呼び出し側は警告として扱えばよい。
var ErrLimitReached = errors.New("走査対象が上限件数に達したため、以降のファイルを読み込みませんでした")

// Options は走査の設定。
type Options struct {
	// Root は走査の起点となるフォルダ。
	Root string
	// Limit は展開するファイル件数の上限。0 以下なら無制限。
	Limit int
	// Workers はメタデータ読み取りの並列数。0 以下なら CPU 数を使う。
	Workers int
	// OnProgress は進捗通知。読み取り済み件数と発見総数を受け取る。nil でもよい。
	OnProgress func(done, found int)
}

// Result は 1 回の走査の結果。
type Result struct {
	Entries []*model.Entry
	// Skipped は、対応拡張子だがメタデータを読めなかったファイルの数。
	Skipped int
	// CloudOnly は、中身がクラウド上にしか無いため開かなかったファイルの数。
	// エントリ自体は一覧へ出しているので、利用者への注意書きに使う。
	CloudOnly int
	// LimitReached は上限に達して打ち切ったかどうか。
	LimitReached bool
}

// candidate は走査で見つけた 1 ファイル。
//
// ディレクトリ列挙で得た FileInfo をそのまま持ち回るのは、あとで stat を
// やり直さないため。クラウド上にしか無いファイルは、stat のためにハンドルを
// 開くだけでも呼び戻しが走る方式があるので、触る回数を最小にする。
type candidate struct {
	path      string
	info      fs.FileInfo
	cloudOnly bool
}

// Scan は Root 配下を走査してエントリを組み立てる。
// ctx がキャンセルされた場合は、そこまでに読めた分と ctx.Err() を返す。
func Scan(ctx context.Context, opts Options) (Result, error) {
	found, limitReached, err := collectPaths(ctx, opts)
	if err != nil {
		return Result{}, err
	}

	cloudOnly := 0
	for _, c := range found {
		if c.cloudOnly {
			cloudOnly++
		}
	}

	entries, skipped, err := readAll(ctx, found, opts)
	return Result{
		Entries:      entries,
		Skipped:      skipped,
		CloudOnly:    cloudOnly,
		LimitReached: limitReached,
	}, err
}

// collectPaths は対応拡張子のファイルを集める。
// メタデータの読み取りより先に対象を全部確定させることで、
// 進捗の分母（発見総数）を最初から表示できるようにしている。
//
// この段階ではファイルを一度も開かない。ディレクトリの列挙だけで済むため、
// クラウド同期フォルダを走査してもダウンロードは起こらない。
func collectPaths(ctx context.Context, opts Options) ([]candidate, bool, error) {
	// 走査ルート自体が読めない場合だけは打ち切る。配下の個別エラーは飛ばして続ける。
	info, err := os.Stat(opts.Root)
	if err != nil {
		return nil, false, fmt.Errorf("走査フォルダを開けません: %w", err)
	}
	if !info.IsDir() {
		return nil, false, fmt.Errorf("走査フォルダではありません: %s", opts.Root)
	}

	supported := map[string]struct{}{}
	for _, ext := range meta.Extensions() {
		supported[ext] = struct{}{}
	}

	var (
		found        []candidate
		limitReached bool
	)
	err = filepath.WalkDir(opts.Root, func(path string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			// 読めないディレクトリやファイルは飛ばして走査を続ける。
			// 1 か所の権限エラーで全体が失敗するほうが困る。
			return nil
		}
		if d.IsDir() {
			if isSkippableDir(d.Name()) && path != opts.Root {
				return filepath.SkipDir
			}
			return nil
		}
		if _, ok := supported[model.Ext(path)]; !ok {
			return nil
		}
		if opts.Limit > 0 && len(found) >= opts.Limit {
			limitReached = true
			return filepath.SkipAll
		}
		// DirEntry の Info は列挙時の情報から作られるので、ここでファイルは開かれない。
		info, err := d.Info()
		if err != nil {
			return nil
		}
		found = append(found, candidate{
			path:      path,
			info:      info,
			cloudOnly: cloudfile.IsPlaceholder(info),
		})
		return nil
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		return nil, false, err
	}
	return found, limitReached, ctx.Err()
}

// isSkippableDir は、走査対象から外すディレクトリ名かを判定する。
// バージョン管理や依存関係のディレクトリはタグ管理の対象にならないうえ、
// ファイル数が多く走査時間を押し上げるため除外する。
func isSkippableDir(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	switch name {
	case "node_modules", "vendor", "target", "dist", "build":
		return true
	}
	return false
}

// readAll は各ファイルのメタデータを並列に読み取る。
// 結果の順序は found の順序と一致させ、走査のたびに並びが変わらないようにする。
func readAll(ctx context.Context, found []candidate, opts Options) ([]*model.Entry, int, error) {
	workers := opts.Workers
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	workers = min(workers, max(len(found), 1))

	results := make([]*model.Entry, len(found))
	var (
		wg      sync.WaitGroup
		next    = make(chan int)
		mu      sync.Mutex
		skipped int
		done    int
	)

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				e := readOne(found[i], opts.Root)
				results[i] = e

				mu.Lock()
				if e == nil {
					skipped++
				}
				done++
				current := done
				mu.Unlock()

				if opts.OnProgress != nil {
					opts.OnProgress(current, len(found))
				}
			}
		}()
	}

	var walkErr error
	for i := range found {
		if ctx.Err() != nil {
			walkErr = ctx.Err()
			break
		}
		next <- i
	}
	close(next)
	wg.Wait()

	entries := make([]*model.Entry, 0, len(found))
	for _, e := range results {
		if e != nil {
			entries = append(entries, e)
		}
	}
	return entries, skipped, walkErr
}

// readOne は 1 ファイルを読み取る。読めない場合は nil を返し、呼び出し側で数える。
//
// 中身がクラウド上にしか無いファイルは開かない。開いた時点でダウンロードが
// 始まり、フォルダを開いただけで同期フォルダ全体を引き落としてしまうため、
// 一覧に出すのに要る情報（名前・サイズ・更新日時）だけでエントリを作る。
func readOne(c candidate, root string) *model.Entry {
	var e *model.Entry
	if c.cloudOnly {
		e = model.NewCloudOnly(c.path, c.info.Name(), c.info.Size(), c.info.ModTime())
	} else {
		var err error
		e, err = meta.Read(c.path, c.info)
		if err != nil {
			return nil
		}
	}
	if rel, err := filepath.Rel(root, c.path); err == nil {
		e.RelPath = rel
	} else {
		e.RelPath = c.path
	}
	return e
}
