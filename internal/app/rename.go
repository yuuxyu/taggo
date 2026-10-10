package app

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/yuuxyu/taggo/internal/atomicfile"
	"github.com/yuuxyu/taggo/internal/cloudfile"
	"github.com/yuuxyu/taggo/internal/folderconf"
	"github.com/yuuxyu/taggo/internal/meta"
	"github.com/yuuxyu/taggo/internal/model"
	"github.com/yuuxyu/taggo/internal/store"
)

// RenamePlan は、ページの名前を変える前に確かめてもらう、変更の見込み。
type RenamePlan struct {
	// From と To は、変更前と変更後のページ名（拡張子を除いたファイル名）。
	From string `json:"from"`
	To   string `json:"to"`
	// Path は変更後のファイルのパス。
	Path string `json:"path"`
	// Notes は参照を書き換えるノートのファイル名。名前を変えるページ自身も、
	// 見出しなどを書き換えるなら含む。
	Notes []string `json:"notes"`
	// CloudOnly は中身がクラウド上にしか無いため、ダウンロードしてから確かめるノートのファイル名。
	CloudOnly []string `json:"cloudOnly"`
	// Skipped は大きすぎて書き換えないノートのファイル名。
	Skipped []string `json:"skipped"`
}

// RenameResult はページの名前を変えた結果。
type RenameResult struct {
	// Entry は名前を変えたページ。
	Entry *model.Entry `json:"entry"`
	// Rewritten は参照を書き換えたノートの数。名前を変えたページ自身も数える。
	Rewritten int `json:"rewritten"`
	// Failed は書き換えられなかったノートと、その理由。
	Failed []string `json:"failed"`
	// Skipped は大きすぎて書き換えなかったノートのファイル名。
	Skipped []string `json:"skipped"`
	// Warning は、名前は変えられたが知らせておきたいこと（ピン留めを付け替えられなかったなど）。
	Warning string `json:"warning,omitempty"`
}

// EntryRenamed は名前の変更イベントのペイロード。
type EntryRenamed struct {
	// From と To は、変更前と変更後のファイルのパス。
	From  string       `json:"from"`
	To    string       `json:"to"`
	Entry *model.Entry `json:"entry"`
}

// renameTarget は名前を変えるページと、変更後の名前。
type renameTarget struct {
	root    string
	oldPath string
	newPath string
	// from と to は、変更前と変更後のページ名（拡張子を除いたファイル名）。
	from, to string
}

// renameNote は、参照を書き換えるかもしれないノート 1 件。
type renameNote struct {
	path      string
	cloudOnly bool
	tooLarge  bool
}

// PlanRename は、path のページの名前を name に変えたときに書き換わるノートを調べて返す。
// ファイルはまだ何も変えない。中身がクラウド上にしか無いノートは開かずに、件数だけを返す。
func (a *App) PlanRename(path, name string) (*RenamePlan, error) {
	t, err := a.resolveRename(path, name)
	if err != nil {
		return nil, err
	}
	notes, err := renameNotes(t.root)
	if err != nil {
		return nil, err
	}

	plan := &RenamePlan{
		From:      t.from,
		To:        t.to,
		Path:      t.newPath,
		Notes:     []string{},
		CloudOnly: []string{},
		Skipped:   []string{},
	}
	for _, n := range notes {
		name := filepath.Base(n.path)
		switch {
		case n.tooLarge:
			plan.Skipped = append(plan.Skipped, name)
		case n.cloudOnly:
			plan.CloudOnly = append(plan.CloudOnly, name)
		default:
			raw, err := os.ReadFile(n.path)
			if err != nil {
				return nil, fmt.Errorf("「%s」を読めませんでした: %w", name, err)
			}
			if _, changed := a.renameRefs(t, n.path).Rewrite(raw); changed {
				plan.Notes = append(plan.Notes, name)
			}
		}
	}
	return plan, nil
}

// RenamePage は、path のページのファイル名を name に変え、そのページを指している
// [[タグ]] と Markdown のリンク（行き先と、名前と同じ表示文字）を、フォルダ直下の
// すべてのノートで書き換える。ページ自身の最初の見出しが名前と同じなら、それも書き換える。
//
// 中身がクラウド上にしか無いノートも、ここで読むためダウンロードが始まる。
// 利用者が PlanRename の結果を確かめたうえで呼ぶこと。
//
// 先にすべてのノートを読んで書き換え後の中身を作り、読めないノートがあれば何も変えずに断る。
// 名前を変えてから各ノートを書き換えるので、途中で書き込めなかったノートは結果の Failed で知らせる。
func (a *App) RenamePage(path, name string) (*RenameResult, error) {
	a.renameMu.Lock()
	defer a.renameMu.Unlock()

	t, err := a.resolveRename(path, name)
	if err != nil {
		return nil, err
	}
	notes, err := renameNotes(t.root)
	if err != nil {
		return nil, err
	}

	result := &RenameResult{Failed: []string{}, Skipped: []string{}}
	type rewrite struct {
		path string
		data []byte
	}
	var rewrites []rewrite
	var unreadable []string
	for _, n := range notes {
		name := filepath.Base(n.path)
		if n.tooLarge {
			result.Skipped = append(result.Skipped, name)
			continue
		}
		raw, err := os.ReadFile(n.path)
		if err != nil {
			unreadable = append(unreadable, fmt.Sprintf("%s（%v）", name, err))
			continue
		}
		if data, changed := a.renameRefs(t, n.path).Rewrite(raw); changed {
			rewrites = append(rewrites, rewrite{path: n.path, data: data})
		}
	}
	if len(unreadable) > 0 {
		return nil, fmt.Errorf("読めないノートがあったため、名前を変えませんでした: %s", strings.Join(unreadable, "、"))
	}

	if err := os.Rename(t.oldPath, t.newPath); err != nil {
		return nil, fmt.Errorf("名前を変えられませんでした: %w", err)
	}
	for _, w := range rewrites {
		dest := w.path
		if strings.EqualFold(dest, t.oldPath) {
			dest = t.newPath
		}
		if err := atomicfile.Replace(dest, w.data); err != nil {
			result.Failed = append(result.Failed, fmt.Sprintf("%s（%v）", filepath.Base(dest), err))
			continue
		}
		result.Rewritten++
	}

	// 一覧（DB）を新しい状態へ直す。ウォッチャーも同じ変更を拾うが、待たずに反映しておく。
	// クラウド上にしか無かったノートは、読んだことでローカルに来ているので読み直す。
	result.Entry, err = a.moveEntry(t)
	if err != nil {
		return nil, err
	}
	rewritten := make(map[string]bool, len(rewrites))
	for _, w := range rewrites {
		rewritten[w.path] = true
	}
	for _, n := range notes {
		if strings.EqualFold(n.path, t.oldPath) {
			continue
		}
		if n.cloudOnly || rewritten[n.path] {
			a.refreshEntry(n.path)
		}
	}
	result.Warning = a.renamePin(t)
	a.emit(EventEntryRenamed, EntryRenamed{From: t.oldPath, To: t.newPath, Entry: result.Entry})
	return result, nil
}

// resolveRename は、path のページの名前を name に変えてよいかを確かめ、変更後のパスを決める。
func (a *App) resolveRename(path, name string) (renameTarget, error) {
	if err := a.checkPagePath(path); err != nil {
		return renameTarget{}, err
	}
	a.mu.Lock()
	scanning := a.scanCancel != nil
	a.mu.Unlock()
	if scanning {
		return renameTarget{}, errors.New("読み込みの途中です。終わってからもう一度お試しください")
	}
	entry, ok := a.store.Get(path)
	if !ok {
		return renameTarget{}, fmt.Errorf("エントリが見つかりません: %s", path)
	}

	ext := filepath.Ext(entry.Path)
	from := strings.TrimSuffix(filepath.Base(entry.Path), ext)
	to := model.NormalizeTag(name)
	switch {
	case to == "":
		return renameTarget{}, errors.New("新しい名前を入力してください")
	case to == from:
		return renameTarget{}, errors.New("名前が変わっていません")
	case strings.ContainsAny(to, "[]"):
		return renameTarget{}, errors.New("「[」と「]」は [[タグ]] に書けないため、名前に使えません")
	}
	if _, ok := model.TagFileName(to); !ok {
		return renameTarget{}, fmt.Errorf("「%s」はファイル名に使えない文字を含むため、名前にできません", to)
	}

	root := a.store.Root()
	// 同じ名前のページが既にあれば断る。.md と .markdown は同じページとして扱うので両方を見る。
	// 大文字小文字だけを変えるときは、見つかるのは名前を変えるページ自身なので構わない。
	for _, e := range []string{".md", ".markdown"} {
		other := filepath.Join(root, to+e)
		if strings.EqualFold(other, entry.Path) {
			continue
		}
		_, err := os.Lstat(other)
		if err == nil {
			return renameTarget{}, fmt.Errorf("「%s」は既にあります", filepath.Base(other))
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return renameTarget{}, err
		}
	}
	return renameTarget{
		root:    root,
		oldPath: entry.Path,
		newPath: filepath.Join(root, to+ext),
		from:    from,
		to:      to,
	}, nil
}

// renameRefs は、notePath のノートの中で t のページを指す参照を書き換える設定を返す。
// Markdown のリンクの行き先は、リンク元のノートの位置を基準に解決する。
func (a *App) renameRefs(t renameTarget, notePath string) meta.RenameRefs {
	return meta.RenameRefs{
		OldName: t.from,
		NewName: t.to,
		Self:    strings.EqualFold(notePath, t.oldPath),
		LinksTo: func(link string) bool {
			return store.SameNote(a.store.LinkPath(link, notePath), t.oldPath)
		},
	}
}

// renameNotes は、参照を書き換えるかもしれないノートとして、root 直下の Markdown を集める。
//
// 読み込みの上限で一覧に載っていないノートも、参照を残さないよう対象にする。
// クラウド上にしか無いかどうかは、ディレクトリの列挙で得た属性で見分け、ファイルは開かない。
func renameNotes(root string) ([]renameNote, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("フォルダを読めませんでした: %w", err)
	}
	var notes []renameNote
	for _, d := range entries {
		if d.IsDir() || !model.IsMarkdown(d.Name()) || atomicfile.IsTemp(d.Name()) {
			continue
		}
		// DirEntry の Info は列挙時の情報から作られるので、ここでファイルは開かれない。
		info, err := d.Info()
		if err != nil {
			continue // 列挙のあとで消えた
		}
		notes = append(notes, renameNote{
			path:      filepath.Join(root, d.Name()),
			cloudOnly: cloudfile.IsPlaceholder(info),
			tooLarge:  info.Size() > maxMarkdownPreviewBytes,
		})
	}
	return notes, nil
}

// moveEntry は、名前を変えたページのエントリを DB の中で付け替え、新しいエントリを返す。
func (a *App) moveEntry(t renameTarget) (*model.Entry, error) {
	prev, had := a.store.Get(t.oldPath)
	if err := a.store.Delete(t.oldPath); err != nil {
		return nil, err
	}
	a.adjustCloudOnly(had && prev.CloudOnly, false)

	info, err := os.Stat(t.newPath)
	if err != nil {
		return nil, fmt.Errorf("名前を変えたページを読めませんでした: %w", err)
	}
	entry, err := meta.Read(t.newPath, info)
	if err != nil {
		return nil, err
	}
	entry.RelPath = filepath.Base(t.newPath)
	if err := a.putEntry(entry); err != nil {
		return nil, err
	}
	return entry, nil
}

// refreshEntry は、読み込み済みのノートを読み直して DB と画面へ反映する。
// 読み込みの上限で一覧に載っていないノートは、そのまま載せないでおく。
func (a *App) refreshEntry(path string) {
	if _, ok := a.store.Get(path); !ok {
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	entry, err := meta.Read(path, info)
	if err != nil {
		return
	}
	entry.RelPath = filepath.Base(path)
	if err := a.putEntry(entry); err != nil {
		return
	}
	a.emit(EventEntryChanged, map[string]any{"path": path, "entry": entry})
}

// renamePin は、名前を変えたページをピン留めしていれば、設定ファイルのファイル名を付け替える。
// 付け替えられなかったときは、その理由を返す。
func (a *App) renamePin(t renameTarget) string {
	if !a.store.IsPinned(t.oldPath) {
		return ""
	}
	a.pinMu.Lock()
	defer a.pinMu.Unlock()

	cfg, err := folderconf.Load(t.root)
	if err == nil {
		oldName, newName := filepath.Base(t.oldPath), filepath.Base(t.newPath)
		for i, name := range cfg.Pinned {
			if strings.EqualFold(name, oldName) {
				cfg.Pinned[i] = newName
			}
		}
		err = folderconf.Save(t.root, cfg)
	}
	if err != nil {
		return fmt.Sprintf("ピン留めを付け替えられませんでした: %v", err)
	}
	a.store.SetPins(pinPaths(t.root, cfg.Pinned))
	a.emit(EventPinsChanged, PinsChanged{})
	return ""
}
