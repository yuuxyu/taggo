package meta

import (
	"strings"
	"testing"
)

// linksToPage は、テストで名前を変えるページ page.md を指すリンクかを判定する。
// 本物では呼び出し側がリンク元の位置から解決するが、ここでは書かれたパスで決めうちにする。
func linksToPage(link string) bool {
	switch strings.ToLower(link) {
	case "page.md", "./page.md", "/page.md", "../wiki/page.md", "page.markdown", "./my page.md":
		return true
	}
	return false
}

func TestRenameRefsRewrite(t *testing.T) {
	cases := []struct {
		name string
		old  string
		new  string
		self bool
		in   string
		want string
	}{
		{
			name: "タグ",
			in:   "[[page]] と [[ Page ]] と [[pages]] と [[other]]\n",
			want: "[[newpage]] と [[newpage]] と [[pages]] と [[other]]\n",
		},
		{
			name: "リンクの行き先と表示文字",
			in:   "[page](./page.md) [詳しく](page.md#節) [page.md](/page.md) [Page](../wiki/page.md)\n",
			want: "[newpage](./newpage.md) [詳しく](newpage.md#節) [newpage.md](/newpage.md) [newpage](../wiki/newpage.md)\n",
		},
		{
			name: "拡張子とタイトルはそのまま",
			in:   "[page](page.markdown \"題\") [ page ](<./page.md>)\n",
			want: "[newpage](newpage.markdown \"題\") [ newpage ](<./newpage.md>)\n",
		},
		{
			name: "ほかのページへのリンクと画像は触らない",
			in:   "[page](./other.md) ![page](./page.md) [page](https://example.com/page.md)\n",
			want: "[page](./other.md) ![page](./page.md) [page](https://example.com/page.md)\n",
		},
		{
			name: "コードの中は触らない",
			in:   "`[[page]]` [[page]]\n```\n[[page]] [page](./page.md)\n```\n~~~\n[[page]]\n~~~\n",
			want: "`[[page]]` [[newpage]]\n```\n[[page]] [page](./page.md)\n```\n~~~\n[[page]]\n~~~\n",
		},
		{
			name: "Front Matter の中は触らない",
			in:   "---\ntags: [[page]]\n---\n[[page]]\n",
			want: "---\ntags: [[page]]\n---\n[[newpage]]\n",
		},
		{
			name: "空白を含む名前は % で書く",
			new:  "new page",
			in:   "[page](./page.md) [page](./other%20page.md) [[page]]\n",
			want: "[new page](./new%20page.md) [page](./other%20page.md) [[new page]]\n",
		},
		{
			name: "% で書かれた名前は新しい名前も % で書く",
			old:  "my page",
			new:  "日本語",
			in:   "[my page](./my%20page.md)\n",
			want: "[日本語](./%E6%97%A5%E6%9C%AC%E8%AA%9E.md)\n",
		},
		{
			name: "自身の見出し",
			self: true,
			in:   "# page\r\n\r\n## page\r\n[[page]]\r\n",
			want: "# newpage\r\n\r\n## page\r\n[[newpage]]\r\n",
		},
		{
			name: "自身でも見出しが名前と違えば触らない",
			self: true,
			in:   "# ページの説明\n",
			want: "# ページの説明\n",
		},
		{
			name: "ほかのノートの見出しは触らない",
			in:   "# page\n",
			want: "# page\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := RenameRefs{OldName: "page", NewName: "newpage", LinksTo: linksToPage, Self: c.self}
			if c.old != "" {
				r.OldName = c.old
			}
			if c.new != "" {
				r.NewName = c.new
			}
			got, changed := r.Rewrite([]byte(c.in))
			if string(got) != c.want {
				t.Fatalf("書き換え結果が違う\n got: %q\nwant: %q", got, c.want)
			}
			if changed != (c.in != c.want) {
				t.Fatalf("changed = %v", changed)
			}
		})
	}
}
