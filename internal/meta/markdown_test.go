package meta

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("テストファイルの作成に失敗: %v", err)
	}
	return path
}

func TestMarkdownRead(t *testing.T) {
	h := markdownHandler{}
	path := writeTemp(t, "note.md", `---
tags:
  - golang
  - 開発メモ
created: 2026-09-18
---

# ここから本文

本文の 1 行目です。[別のノート](./other.md) と [参照先](sub/ref.markdown) を参照。

`+"```go\nfmt.Println(\"コードは抜粋に含めない\")\n```"+`

締めの行。
`)

	got, err := h.Read(path)
	if err != nil {
		t.Fatalf("Markdown の読み取りに失敗: %v", err)
	}
	if want := []string{"golang", "開発メモ"}; !reflect.DeepEqual(got.Tags, want) {
		t.Fatalf("タグが一致しない: got %v, want %v", got.Tags, want)
	}
	if got.Title != "ここから本文" {
		t.Fatalf("タイトルが見出しから取れていない: %q", got.Title)
	}
	if want := []string{"./other.md", "sub/ref.markdown"}; !reflect.DeepEqual(got.Links, want) {
		t.Fatalf("リンクが一致しない: got %v, want %v", got.Links, want)
	}
	if strings.Contains(got.Preview, "コードは抜粋に含めない") {
		t.Fatalf("コードブロックが抜粋に混ざっている: %q", got.Preview)
	}
	if !strings.Contains(got.Preview, "本文の 1 行目です") {
		t.Fatalf("本文が抜粋に入っていない: %q", got.Preview)
	}
	if strings.Contains(got.Preview, "](") {
		t.Fatalf("リンクの URL が抜粋に残っている: %q", got.Preview)
	}
	if !strings.Contains(got.Preview, "別のノート") {
		t.Fatalf("リンクの表示文字が抜粋から消えている: %q", got.Preview)
	}
}

func TestMarkdownExcerptSkipsTableMarkup(t *testing.T) {
	h := markdownHandler{}
	path := writeTemp(t, "table.md", `# 表のあるノート

| 形式 | 保存先 |
| --- | --- |
| Markdown | Front Matter |
`)

	got, err := h.Read(path)
	if err != nil {
		t.Fatalf("読み取りに失敗: %v", err)
	}
	if strings.Contains(got.Preview, "---") || strings.Contains(got.Preview, "|") {
		t.Fatalf("表の記号が抜粋に残っている: %q", got.Preview)
	}
	if !strings.Contains(got.Preview, "Front Matter") {
		t.Fatalf("表のセルの中身が抜粋に入っていない: %q", got.Preview)
	}
}

func TestMarkdownWriteTagsPreservesOtherFields(t *testing.T) {
	path := writeTemp(t, "note.md", `---
title: 元のタイトル
created: 2026-09-18
tags: [old]
---

# 見出し

本文。
`)

	// パッケージ関数側を通して、正規化（重複除去・ソート）まで含めて確認する。
	h := markdownHandler{}
	if err := WriteTags(path, []string{"新タグ", "another", " 新タグ "}); err != nil {
		t.Fatalf("タグ書き込みに失敗: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("書き込み後の読み込みに失敗: %v", err)
	}
	front, body, err := SplitFrontMatter(raw)
	if err != nil {
		t.Fatalf("書き込み後の Front Matter が壊れている: %v", err)
	}
	if front["created"] == nil || front["title"] != "元のタイトル" {
		t.Fatalf("タグ以外のフィールドが失われた: %+v", front)
	}
	if !strings.Contains(string(body), "# 見出し") {
		t.Fatalf("本文が失われた: %q", string(body))
	}

	got, err := h.Read(path)
	if err != nil {
		t.Fatalf("書き込み後の読み取りに失敗: %v", err)
	}
	if want := []string{"another", "新タグ"}; !reflect.DeepEqual(got.Tags, want) {
		t.Fatalf("書き戻したタグが一致しない: got %v, want %v", got.Tags, want)
	}
}

func TestMarkdownWriteTagsCreatesFrontMatter(t *testing.T) {
	path := writeTemp(t, "plain.md", "# Front Matter の無い文書\n\n本文だけ。\n")

	h := markdownHandler{}
	if err := h.WriteTags(path, []string{"追加"}); err != nil {
		t.Fatalf("タグ書き込みに失敗: %v", err)
	}
	got, err := h.Read(path)
	if err != nil {
		t.Fatalf("読み取りに失敗: %v", err)
	}
	if want := []string{"追加"}; !reflect.DeepEqual(got.Tags, want) {
		t.Fatalf("タグが一致しない: got %v, want %v", got.Tags, want)
	}
	if got.Title != "Front Matter の無い文書" {
		t.Fatalf("本文の見出しが失われた: %q", got.Title)
	}
}

func TestMarkdownCommaSeparatedTags(t *testing.T) {
	h := markdownHandler{}
	path := writeTemp(t, "csv.md", "---\ntags: golang, 設計, テスト\n---\n\n本文。\n")

	got, err := h.Read(path)
	if err != nil {
		t.Fatalf("読み取りに失敗: %v", err)
	}
	if len(got.Tags) != 3 {
		t.Fatalf("カンマ区切りタグが分割されていない: %v", got.Tags)
	}
}

func TestNoteLinks(t *testing.T) {
	body := []byte(`[通常リンク](./sub/別のノート.md) と
[アンカー付き](other.markdown#見出し) と [空白入り](note%20a.md)。
外部は対象外: [web](https://example.com/page.md)。
画像も対象外: ![図](./img/a.png)、![md風](./x.md)。
Markdown 以外へのリンクも対象外: [メモ帳](./memo.txt)。
WikiLink 記法は使わない: [[別のノート]]。
同じ行き先は 1 回だけ: [再掲](./SUB/別のノート.md)。
`)

	// 行き先はパスのまま残す。どのノートを指すかの解決は store 側。
	want := []string{"./sub/別のノート.md", "other.markdown", "note a.md"}
	if got := noteLinks(body); !reflect.DeepEqual(got, want) {
		t.Fatalf("リンクの抽出が一致しない: got %v, want %v", got, want)
	}
}

// Front Matter の tag: で、そのノートが説明しているタグを宣言できること。
// 付いているタグ（tags:）とは別に読み、1 つに決められない書き方は無視する。
func TestMarkdownTagPage(t *testing.T) {
	h := markdownHandler{}
	cases := []struct {
		name  string
		front string
		want  string
	}{
		{"文字列", "tag: golang\ntags: [プログラミング]\n", "golang"},
		{"数値", "tag: 2026\n", "2026"},
		{"リストは対象外", "tag: [golang, rust]\n", ""},
		{"宣言なし", "tags: [golang]\n", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := writeTemp(t, "page.md", "---\n"+c.front+"---\n\n# 見出し\n")
			got, err := h.Read(path)
			if err != nil {
				t.Fatalf("読み取りに失敗: %v", err)
			}
			if got.TagPage != c.want {
				t.Fatalf("タグページのタグが違う: got %q, want %q", got.TagPage, c.want)
			}
		})
	}

	// tag: はタグとしては数えない。
	path := writeTemp(t, "page.md", "---\ntag: golang\ntags: [プログラミング]\n---\n")
	got, _ := h.Read(path)
	if want := []string{"プログラミング"}; !reflect.DeepEqual(got.Tags, want) {
		t.Fatalf("tag: がタグに混ざっている: got %v, want %v", got.Tags, want)
	}
}

// タグを書き換えても、tag: の宣言は残ること。
func TestMarkdownWriteTagsKeepsTagPage(t *testing.T) {
	h := markdownHandler{}
	path := writeTemp(t, "page.md", "---\ntag: golang\ntags: [a]\n---\n\n本文\n")
	if err := h.WriteTags(path, []string{"b"}); err != nil {
		t.Fatalf("書き込みに失敗: %v", err)
	}
	got, err := h.Read(path)
	if err != nil {
		t.Fatalf("読み取りに失敗: %v", err)
	}
	if got.TagPage != "golang" {
		t.Fatalf("タグの書き換えで tag: が消えた: %q", got.TagPage)
	}
}

func TestMarkdownThumbnail(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"画像なし", "# 見出し\n\n本文だけ。\n", ""},
		{"ローカル画像", "# 見出し\n\n説明。\n\n![図](./img/figure.png \"図の説明\")\n", "./img/figure.png"},
		{"山括弧で囲んだ画像", "![図](<assets/a.png>)\n", "assets/a.png"},
		{"外部 URL の画像", "![](https://example.com/a.jpg)\n", "https://example.com/a.jpg"},
		{"Windows の絶対パス", "![](C:/notes/a.png)\n", "C:/notes/a.png"},
		{"YouTube の watch URL", "紹介。\n\nhttps://www.youtube.com/watch?v=dQw4w9WgXcQ\n", "https://i.ytimg.com/vi/dQw4w9WgXcQ/mqdefault.jpg"},
		{"YouTube の短縮 URL", "[動画](https://youtu.be/dQw4w9WgXcQ?t=10)\n", "https://i.ytimg.com/vi/dQw4w9WgXcQ/mqdefault.jpg"},
		{"YouTube の他のパラメータ付き", "<https://youtube.com/watch?list=PL1&v=dQw4w9WgXcQ>\n", "https://i.ytimg.com/vi/dQw4w9WgXcQ/mqdefault.jpg"},
		{"YouTube ショート", "https://m.youtube.com/shorts/dQw4w9WgXcQ\n", "https://i.ytimg.com/vi/dQw4w9WgXcQ/mqdefault.jpg"},
		{"先に書かれた動画を使う", "https://youtu.be/dQw4w9WgXcQ\n\n![](a.png)\n", "https://i.ytimg.com/vi/dQw4w9WgXcQ/mqdefault.jpg"},
		{"同じ行なら先に書かれた画像を使う", "![](a.png) https://youtu.be/dQw4w9WgXcQ\n", "a.png"},
		{"ふつうのリンクは使わない", "[資料](doc.png)\n\n![](b.png)\n", "b.png"},
		{"data URI は使わない", "![](data:image/png;base64,AAAA)\n\n![](c.png)\n", "c.png"},
		{"コードブロックの中は使わない", "```md\n![](code.png)\n```\n\n![](real.png)\n", "real.png"},
		{"YouTube 以外の動画サイトは使わない", "https://www.youtube.com/channel/abc\n", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := thumbnail([]byte(c.body)); got != c.want {
				t.Fatalf("サムネイルが一致しない: got %q, want %q", got, c.want)
			}
		})
	}
}
