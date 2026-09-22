package app

import (
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// fileAttributeOffline は、クラウド同期が「中身はここに無い」と示すために使う属性。
const fileAttributeOffline = 0x00001000

func setAttributes(t *testing.T, path string, attrs uint32) {
	t.Helper()
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.SetFileAttributes(p, attrs); err != nil {
		t.Skipf("属性を付けられない環境のため確認を飛ばす: %v", err)
	}
}

// 走査のあとで同期サービスがファイルを「オンラインのみ」へ戻した場合を再現する。
// 変わるのは属性だけなのでウォッチャーは気付かず、DB 上はローカルのままになる。
// その状態でも、プレビューとタグ書き込みのどちらもファイルを開かないこと。
func TestDehydratedAfterScanIsNotOpened(t *testing.T) {
	const body = "---\ntags: [golang]\n---\n\n# 本文\n"

	cases := []struct {
		name string
		open func(a *App, path string) bool // 開けてしまったら true
	}{
		{"Markdown プレビュー", func(a *App, path string) bool {
			_, err := a.MarkdownSource(path)
			return err == nil
		}},
		{"タグ書き込み", func(a *App, path string) bool {
			return a.SetTags(path, []string{"rust"}).OK
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, root := newTestApp(t, map[string]string{"note.md": body})
			path := filepath.Join(root, "note.md")
			setAttributes(t, path, fileAttributeOffline)

			if c.open(a, path) {
				t.Fatal("クラウド上にだけあるファイルを開いた")
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != body {
				t.Fatalf("ファイルの中身が変わった: %q", got)
			}

			// DB と件数はクラウド上のみの状態へ直っていること。
			if e, ok := a.store.Get(path); !ok || !e.CloudOnly {
				t.Fatalf("DB がクラウド上のみの状態へ直っていない: %+v", e)
			}
			if n := a.Status().CloudOnly; n != 1 {
				t.Fatalf("クラウド上のみの件数が違う: got %d, want 1", n)
			}

			// 利用者が取り込めば、ローカルのファイルとして読み直され、件数も戻る。
			setAttributes(t, path, syscall.FILE_ATTRIBUTE_NORMAL)
			fetched, err := a.FetchCloudEntry(path)
			if err != nil {
				t.Fatalf("取り込みに失敗: %v", err)
			}
			if fetched.CloudOnly || len(fetched.Tags) != 1 {
				t.Fatalf("取り込んだエントリが想定外: %+v", fetched)
			}
			if n := a.Status().CloudOnly; n != 0 {
				t.Fatalf("取り込んだあとの件数が違う: got %d, want 0", n)
			}
		})
	}
}

// 本文に埋め込まれた画像の中身がクラウド上にしか無いときは、ノートを開いただけで
// ダウンロードが始まらないよう、配信を断ること。取り込まれてローカルに戻れば配信する。
func TestCloudOnlyImageIsNotServed(t *testing.T) {
	a, root := newTestApp(t, map[string]string{
		"note.md": "# 本文\n\n![図](./a.png)\n",
		"a.png":   pngHead,
	})
	path := filepath.Join(root, "a.png")
	setAttributes(t, path, fileAttributeOffline)

	if rec := getImage(a, path); rec.Code == http.StatusOK {
		t.Fatal("クラウド上にだけある画像を配信した")
	}

	setAttributes(t, path, syscall.FILE_ATTRIBUTE_NORMAL)
	if rec := getImage(a, path); rec.Code != http.StatusOK {
		t.Fatalf("ローカルに戻った画像が配信されない: %d %s", rec.Code, rec.Body.String())
	}
}
