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
	path := writeTemp(t, "note.md", `# ここから本文

本文の 1 行目です。[別のノート](./other.md) と [参照先](sub/ref.markdown) を参照。[[golang]] の話。

`+"```go\nfmt.Println(\"コードは抜粋に含めない\")\n```"+`

締めの行。[[開発メモ]]
`)

	got, err := readMarkdown(path)
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
	if strings.Contains(got.Preview, "](") || strings.Contains(got.Preview, "[[") {
		t.Fatalf("リンクの記法が抜粋に残っている: %q", got.Preview)
	}
	if !strings.Contains(got.Preview, "別のノート") || !strings.Contains(got.Preview, "golang の話") {
		t.Fatalf("リンクやタグの表示文字が抜粋から消えている: %q", got.Preview)
	}
}

// 先頭の「---」で囲んだブロック（ほかのツールの Front Matter）は、中身を読まずに本文から外すこと。
// tags: や title: を書いてあっても、タグや見出しにはならない。
func TestMarkdownIgnoresFrontMatter(t *testing.T) {
	path := writeTemp(t, "note.md", "---\ntags: [golang]\ntitle: 使わないタイトル\n---\n\n本文だけ。[[本文のタグ]]\n")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	e, err := Read(path, info)
	if err != nil {
		t.Fatalf("読み取りに失敗: %v", err)
	}
	if !reflect.DeepEqual(e.Tags, []string{"本文のタグ"}) {
		t.Fatalf("Front Matter の tags: がタグになっている: %v", e.Tags)
	}
	if e.Title != "note.md" {
		t.Fatalf("Front Matter の title: が見出しになっている: %q", e.Title)
	}
	if e.Preview != "本文だけ。本文のタグ" {
		t.Fatalf("Front Matter が抜粋に混ざっている: %q", e.Preview)
	}
}

func TestStripFrontMatter(t *testing.T) {
	cases := map[string]struct{ in, want string }{
		"あり":         {"---\na: 1\n---\n本文\n", "本文\n"},
		"CRLF":       {"---\r\na: 1\r\n---\r\n本文\r\n", "本文\r\n"},
		"... で閉じる":   {"---\na: 1\n...\n本文\n", "本文\n"},
		"閉じていない":     {"---\n本文\n", "---\n本文\n"},
		"先頭ではない":     {"本文\n---\na\n---\n", "本文\n---\na\n---\n"},
		"YAML でなくても": {"---\n: [壊れた\n---\n本文\n", "本文\n"},
	}
	for name, c := range cases {
		if got := string(StripFrontMatter([]byte(c.in))); got != c.want {
			t.Errorf("%s: got %q, want %q", name, got, c.want)
		}
	}
}

func TestMarkdownExcerptSkipsTableMarkup(t *testing.T) {
	path := writeTemp(t, "table.md", `# 表のあるノート

| 形式 | 保存先 |
| --- | --- |
| Markdown | 本文 |
`)

	got, err := readMarkdown(path)
	if err != nil {
		t.Fatalf("読み取りに失敗: %v", err)
	}
	if strings.Contains(got.Preview, "---") || strings.Contains(got.Preview, "|") {
		t.Fatalf("表の記号が抜粋に残っている: %q", got.Preview)
	}
	if !strings.Contains(got.Preview, "保存先") {
		t.Fatalf("表のセルの中身が抜粋に入っていない: %q", got.Preview)
	}
}

func TestNoteLinks(t *testing.T) {
	body := []byte(`[通常リンク](./sub/別のノート.md) と
[アンカー付き](other.markdown#見出し) と [空白入り](note%20a.md)。
外部は対象外: [web](https://example.com/page.md)。
画像も対象外: ![図](./img/a.png)、![md風](./x.md)。
Markdown 以外へのリンクも対象外: [メモ帳](./memo.txt)。
[[タグ]] はタグとして読むので、リンクには含めない: [[別のノート]]。
同じ行き先は 1 回だけ: [再掲](./SUB/別のノート.md)。
`)

	// 行き先はパスのまま残す。どのノートを指すかの解決は store 側。
	want := []string{"./sub/別のノート.md", "other.markdown", "note a.md"}
	if got := noteLinks(body); !reflect.DeepEqual(got, want) {
		t.Fatalf("リンクの抽出が一致しない: got %v, want %v", got, want)
	}
}

// 本文の [[タグ]] を、出てくる順に集めること。コードの中は読まない。
func TestBodyTags(t *testing.T) {
	body := []byte(strings.Join([]string{
		"# 見出し",
		"",
		"[[golang]] と [[開発 メモ]] について。[[golang]] はもう一度。",
		"`[[インラインコード]]` と ``a ` [[二重]] b`` は対象外、閉じない ` の後の [[閉じない]] は対象。",
		"[[]] と [[改行",
		"をまたぐ]] と [ [空白入り] ] は対象外。",
		"```",
		"[[コードブロック]]",
		"```",
		"[[最後]]",
	}, "\n"))
	want := []string{"golang", "開発 メモ", "golang", "閉じない", "最後"}
	if got := bodyTags(body); !reflect.DeepEqual(got, want) {
		t.Fatalf("[[タグ]] の抽出が違う: got %v, want %v", got, want)
	}
}

// エントリのタグは、本文の [[タグ]] を正規化して重複を除いたものになること。
func TestReadNormalizesTags(t *testing.T) {
	path := writeTemp(t, "note.md", "# 見出し\n\n[[Golang]] と [[ 並行  処理 ]] と [[golang]] の話。\n")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	e, err := Read(path, info)
	if err != nil {
		t.Fatalf("読み取りに失敗: %v", err)
	}
	if want := []string{"Golang", "並行 処理"}; !reflect.DeepEqual(e.Tags, want) {
		t.Fatalf("タグが違う: got %v, want %v", e.Tags, want)
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
