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
	// LimitReached は上限に達して打ち切ったかどうか。
	LimitReached bool
}

// Scan は Root 配下を走査してエントリを組み立てる。
// ctx がキャンセルされた場合は、そこまでに読めた分と ctx.Err() を返す。
func Scan(ctx context.Context, opts Options) (Result, error) {
	paths, limitReached, err := collectPaths(ctx, opts)
	if err != nil {
		return Result{}, err
	}

	entries, skipped, err := readAll(ctx, paths, opts)
	return Result{
		Entries:      entries,
		Skipped:      skipped,
		LimitReached: limitReached,
	}, err
}

// collectPaths は対応拡張子のファイルパスを集める。
// メタデータの読み取りより先にパスを全部確定させることで、
// 進捗の分母（発見総数）を最初から表示できるようにしている。
func collectPaths(ctx context.Context, opts Options) ([]string, bool, error) {
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
		paths        []string
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
		if opts.Limit > 0 && len(paths) >= opts.Limit {
			limitReached = true
			return filepath.SkipAll
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		return nil, false, err
	}
	return paths, limitReached, ctx.Err()
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
// 結果の順序は paths の順序と一致させ、走査のたびに並びが変わらないようにする。
func readAll(ctx context.Context, paths []string, opts Options) ([]*model.Entry, int, error) {
	workers := opts.Workers
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	workers = min(workers, max(len(paths), 1))

	results := make([]*model.Entry, len(paths))
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
				e := readOne(paths[i], opts.Root)
				results[i] = e

				mu.Lock()
				if e == nil {
					skipped++
				}
				done++
				current := done
				mu.Unlock()

				if opts.OnProgress != nil {
					opts.OnProgress(current, len(paths))
				}
			}
		}()
	}

	var walkErr error
	for i := range paths {
		if ctx.Err() != nil {
			walkErr = ctx.Err()
			break
		}
		next <- i
	}
	close(next)
	wg.Wait()

	entries := make([]*model.Entry, 0, len(paths))
	for _, e := range results {
		if e != nil {
			entries = append(entries, e)
		}
	}
	return entries, skipped, walkErr
}

// readOne は 1 ファイルを読み取る。読めない場合は nil を返し、呼び出し側で数える。
func readOne(path, root string) *model.Entry {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	e, err := meta.Read(path, info)
	if err != nil {
		return nil
	}
	if rel, err := filepath.Rel(root, path); err == nil {
		e.RelPath = rel
	} else {
		e.RelPath = path
	}
	return e
}
