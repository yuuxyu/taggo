// Package scan は指定フォルダ配下を走査し、Markdown ファイルのメタデータを読み取る。
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

// Options は走査の設定。
type Options struct {
	// Root は走査の起点となるフォルダ。
	Root string
	// Limit は展開するファイル件数の上限。0 以下なら無制限。
	Limit int
	// StartAfter は続きから読み込むときの再開位置で、Root からの相対パス。
	// 走査順でこのパス以前にあるファイルは飛ばす。空なら先頭から読む。
	StartAfter string
	// Workers はメタデータ読み取りの並列数。0 以下なら CPU 数を使う。
	Workers int
	// OnProgress は進捗通知。読み取り済み件数と発見総数を受け取る。nil でもよい。
	OnProgress func(done, found int)
}

// Result は 1 回の走査の結果。
type Result struct {
	Entries []*model.Entry
	// Skipped は、Markdown だがメタデータを読めなかったファイルの数。
	Skipped int
	// CloudOnly は、中身がクラウド上にしか無いため開かなかったファイルの数。
	// エントリ自体は一覧へ出しているので、利用者への注意書きに使う。
	CloudOnly int
	// LimitReached は上限に達して打ち切ったかどうか。
	LimitReached bool
	// Remaining は上限で打ち切ったあとに残っている Markdown ファイルの数。
	// 数えるのはディレクトリの列挙だけで、ファイルは開かない。
	Remaining int
	// Cursor は今回の対象にした最後のファイルの相対パス。
	// 続きを読むときは、これを StartAfter に渡す。
	Cursor string
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
	c, err := collectPaths(ctx, opts)
	if err != nil {
		return Result{}, err
	}
	found := c.found

	cloudOnly := 0
	for _, c := range found {
		if c.cloudOnly {
			cloudOnly++
		}
	}

	cursor := ""
	if len(found) > 0 {
		cursor = relPath(opts.Root, found[len(found)-1].path)
	}

	entries, skipped, err := readAll(ctx, found, opts)
	return Result{
		Entries:      entries,
		Skipped:      skipped,
		CloudOnly:    cloudOnly,
		LimitReached: c.limitReached,
		Remaining:    c.remaining,
		Cursor:       cursor,
	}, err
}

// collected は collectPaths の結果。
type collected struct {
	found        []candidate
	limitReached bool
	// remaining は上限を超えた分の件数。
	remaining int
}

// collectPaths は Markdown ファイルを集める。
// メタデータの読み取りより先に対象を全部確定させることで、
// 進捗の分母（発見総数）を最初から表示できるようにしている。
//
// 上限に達したあとも、残りの件数を数えるために最後まで列挙を続ける。
// この段階ではファイルを一度も開かない。ディレクトリの列挙だけで済むため、
// クラウド同期フォルダを走査してもダウンロードは起こらない。
func collectPaths(ctx context.Context, opts Options) (collected, error) {
	// 走査ルート自体が読めない場合だけは打ち切る。配下の個別エラーは飛ばして続ける。
	info, err := os.Stat(opts.Root)
	if err != nil {
		return collected{}, fmt.Errorf("走査フォルダを開けません: %w", err)
	}
	if !info.IsDir() {
		return collected{}, fmt.Errorf("走査フォルダではありません: %s", opts.Root)
	}

	var after []string
	if opts.StartAfter != "" {
		after = splitPath(opts.StartAfter)
	}

	var c collected
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
			if path == opts.Root {
				return nil
			}
			if isSkippableDir(d.Name()) {
				return filepath.SkipDir
			}
			// 再開位置より前にあるフォルダは、中身も全部読み込み済みなので潜らない。
			if after != nil {
				dir := splitPath(relPath(opts.Root, path))
				if !hasPrefix(after, dir) && compareWalkOrder(dir, after) < 0 {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !model.IsMarkdown(path) {
			return nil
		}
		if after != nil && compareWalkOrder(splitPath(relPath(opts.Root, path)), after) <= 0 {
			return nil
		}
		if c.limitReached || (opts.Limit > 0 && len(c.found) >= opts.Limit) {
			c.limitReached = true
			c.remaining++
			return nil
		}
		// DirEntry の Info は列挙時の情報から作られるので、ここでファイルは開かれない。
		info, err := d.Info()
		if err != nil {
			return nil
		}
		c.found = append(c.found, candidate{
			path:      path,
			info:      info,
			cloudOnly: cloudfile.IsPlaceholder(info),
		})
		return nil
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		return collected{}, err
	}
	return c, ctx.Err()
}

// IsAfter は、Root からの相対パス rel が、走査順で cursor より後ろにあるかを判定する。
// 続きをまだ読み込んでいない範囲のファイルかどうかを見分けるのに使う。
func IsAfter(rel, cursor string) bool {
	return compareWalkOrder(splitPath(rel), splitPath(cursor)) > 0
}

// compareWalkOrder は 2 つの相対パスを filepath.WalkDir の訪問順で比べる。
//
// WalkDir は各フォルダの中身を名前のバイト順に並べ、フォルダに出会うとその場で潜る。
// そのためパス文字列のまま比べると、区切り文字と「.」などの大小でずれる
// （「a」は「a.md」より先に訪れるが、文字列では後ろになる）。要素ごとに比べれば一致する。
func compareWalkOrder(a, b []string) int {
	for i := range min(len(a), len(b)) {
		if c := strings.Compare(a[i], b[i]); c != 0 {
			return c
		}
	}
	return len(a) - len(b)
}

// hasPrefix は、パス要素の並び path が prefix で始まるかを判定する。
func hasPrefix(path, prefix []string) bool {
	if len(prefix) > len(path) {
		return false
	}
	for i := range prefix {
		if path[i] != prefix[i] {
			return false
		}
	}
	return true
}

// splitPath は相対パスを要素に分ける。
func splitPath(rel string) []string {
	return strings.Split(filepath.ToSlash(filepath.Clean(rel)), "/")
}

// relPath は root からの相対パスを返す。求められなければ path をそのまま返す。
func relPath(root, path string) string {
	if rel, err := filepath.Rel(root, path); err == nil {
		return rel
	}
	return path
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
	e.RelPath = relPath(root, c.path)
	return e
}
