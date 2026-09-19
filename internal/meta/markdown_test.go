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

本文の 1 行目です。[[別のノート]] と [[参照先|別名]] を参照。

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
	if want := []string{"別のノート", "参照先"}; !reflect.DeepEqual(got.Links, want) {
		t.Fatalf("WikiLink が一致しない: got %v, want %v", got.Links, want)
	}
	if strings.Contains(got.Preview, "コードは抜粋に含めない") {
		t.Fatalf("コードブロックが抜粋に混ざっている: %q", got.Preview)
	}
	if !strings.Contains(got.Preview, "本文の 1 行目です") {
		t.Fatalf("本文が抜粋に入っていない: %q", got.Preview)
	}
	if strings.Contains(got.Preview, "[[") {
		t.Fatalf("WikiLink の括弧が抜粋に残っている: %q", got.Preview)
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
