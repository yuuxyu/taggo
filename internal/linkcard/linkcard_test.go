package linkcard

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"golang.org/x/text/encoding/japanese"
)

// newTestClient は、ローカルのテストサーバーへ接続できるクライアントを返す。
// oEmbed の問い合わせ先もテストサーバーへ向ける。
func newTestClient(srv *httptest.Server) *Client {
	return newClient(srv.Client(), srv.URL+"/youtube-oembed", srv.URL+"/x-oembed")
}

func TestFetchPageReadsOGP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html><html><head>
<title>タイトル要素</title>
<meta property="og:title" content="OGP の&amp;タイトル">
<meta property="og:description" content="説明
  です">
<meta property="og:image" content="/img/card.png">
<meta property="og:site_name" content="例のサイト">
</head><body><meta property="og:title" content="後から出た値は使わない"><svg><title>図の題名</title></svg></body></html>`))
	}))
	defer srv.Close()

	p, err := newTestClient(srv).Fetch(context.Background(), srv.URL+"/article")
	if err != nil {
		t.Fatal(err)
	}
	want := Preview{
		URL:         srv.URL + "/article",
		Kind:        KindPage,
		Title:       "OGP の&タイトル",
		Description: "説明 です",
		Image:       srv.URL + "/img/card.png",
		SiteName:    "例のサイト",
	}
	if *p != want {
		t.Errorf("got %+v\nwant %+v", *p, want)
	}
}

func TestFetchPageReadsMetaInBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><script>var x = 1;</script></head><body>
<title>本文の中のタイトル</title><meta property="og:image" content="https://example.com/a.jpg"></body></html>`))
	}))
	defer srv.Close()

	p, err := newTestClient(srv).Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if p.Title != "本文の中のタイトル" || p.Image != "https://example.com/a.jpg" {
		t.Errorf("got %+v", *p)
	}
}

func TestFetchPageFallsBackToTitleElement(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><title> OGP の無い
ページ </title><meta name="description" content="普通の説明"></head></html>`))
	}))
	defer srv.Close()

	p, err := newTestClient(srv).Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if p.Title != "OGP の無い ページ" || p.Description != "普通の説明" {
		t.Errorf("got %+v", *p)
	}
	if p.SiteName != "127.0.0.1" {
		t.Errorf("サイト名が無いときはホスト名になるはず: %q", p.SiteName)
	}
}

func TestFetchPageDecodesShiftJIS(t *testing.T) {
	body, err := japanese.ShiftJIS.NewEncoder().String(
		`<html><head><meta charset="Shift_JIS"><title>日本語のページ</title></head></html>`)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	p, err := newTestClient(srv).Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if p.Title != "日本語のページ" {
		t.Errorf("got %q", p.Title)
	}
}

func TestFetchPageRejectsNonHTML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF-1.7"))
	}))
	defer srv.Close()

	if _, err := newTestClient(srv).Fetch(context.Background(), srv.URL+"/a.pdf"); err == nil {
		t.Error("HTML でなければエラーになるはず")
	}
}

func TestFetchYouTubeUsesOEmbed(t *testing.T) {
	var gotURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/youtube-oembed" {
			t.Errorf("oEmbed 以外へ問い合わせた: %s", r.URL.Path)
		}
		gotURL = r.URL.Query().Get("url")
		_, _ = w.Write([]byte(`{"title":"動画の題名","author_name":"チャンネル","thumbnail_url":"https://i.ytimg.com/vi/abc/hqdefault.jpg"}`))
	}))
	defer srv.Close()

	const video = "https://youtu.be/abc"
	p, err := newTestClient(srv).Fetch(context.Background(), video)
	if err != nil {
		t.Fatal(err)
	}
	if gotURL != video {
		t.Errorf("oEmbed に渡した URL: %q", gotURL)
	}
	want := Preview{
		URL:      video,
		Kind:     KindYouTube,
		Title:    "動画の題名",
		Image:    "https://i.ytimg.com/vi/abc/hqdefault.jpg",
		SiteName: "YouTube",
		Author:   "チャンネル",
	}
	if *p != want {
		t.Errorf("got %+v\nwant %+v", *p, want)
	}
}

func TestFetchXUsesOEmbed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/x-oembed" {
			t.Errorf("oEmbed 以外へ問い合わせた: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{
			"author_name": "投稿者",
			"author_url": "https://twitter.com/someone",
			"html": "<blockquote class=\"twitter-tweet\"><p lang=\"ja\" dir=\"ltr\">1 行目<br>2 行目 &amp; <a href=\"https://t.co/x\">example.com</a></p>&mdash; 投稿者 (@someone) <a href=\"https://twitter.com/someone/status/1\">2026年9月21日</a></blockquote>"
		}`))
	}))
	defer srv.Close()

	p, err := newTestClient(srv).Fetch(context.Background(), "https://x.com/someone/status/1")
	if err != nil {
		t.Fatal(err)
	}
	want := Preview{
		URL:         "https://x.com/someone/status/1",
		Kind:        KindX,
		Title:       "投稿者",
		Description: "1 行目\n2 行目 & example.com",
		SiteName:    "X",
		Author:      "@someone",
	}
	if *p != want {
		t.Errorf("got %+v\nwant %+v", *p, want)
	}
}

func TestIsXPost(t *testing.T) {
	for raw, want := range map[string]bool{
		"https://x.com/someone/status/123":             true,
		"https://twitter.com/someone/status/123?s=20":  true,
		"https://mobile.twitter.com/someone/status/12": true,
		"https://x.com/someone":                        false,
		"https://x.com/someone/likes":                  false,
		"https://example.com/someone/status/123":       false,
	} {
		u := mustParse(t, raw)
		if got := isXPost(u); got != want {
			t.Errorf("isXPost(%q) = %v, want %v", raw, got, want)
		}
	}
}

func mustParse(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestFetchCachesResults(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<title>一度だけ</title>`))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	for range 3 {
		if _, err := c.Fetch(context.Background(), srv.URL); err != nil {
			t.Fatal(err)
		}
	}
	if n := hits.Load(); n != 1 {
		t.Errorf("取得は 1 回のはず: %d 回", n)
	}
}

func TestFetchRejectsNonHTTP(t *testing.T) {
	c := New()
	for _, raw := range []string{"file:///C:/secret.txt", "javascript:alert(1)", "notes/a.md", ""} {
		if _, err := c.Fetch(context.Background(), raw); err == nil {
			t.Errorf("%q はエラーになるはず", raw)
		}
	}
}

func TestNewBlocksLocalAddresses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("ローカルのサーバーへ接続してはいけない")
	}))
	defer srv.Close()

	_, err := New().Fetch(context.Background(), srv.URL)
	if !errors.Is(err, ErrBlockedAddress) {
		t.Errorf("ErrBlockedAddress になるはず: %v", err)
	}
}

func TestIsPublic(t *testing.T) {
	for addr, want := range map[string]bool{
		"8.8.8.8":         true,
		"2001:4860::8888": true,
		"127.0.0.1":       false,
		"::1":             false,
		"10.0.0.1":        false,
		"192.168.1.1":     false,
		"172.16.0.1":      false,
		"169.254.169.254": false,
		"100.64.0.1":      false,
		"0.0.0.0":         false,
		"::ffff:10.0.0.1": false,
		"fe80::1":         false,
		"fc00::1":         false,
	} {
		if got := isPublic(netip.MustParseAddr(addr)); got != want {
			t.Errorf("isPublic(%s) = %v, want %v", addr, got, want)
		}
	}
}

func TestTruncateCountsRunes(t *testing.T) {
	if got := truncate(strings.Repeat("あ", 5), 3); got != "あああ…" {
		t.Errorf("got %q", got)
	}
	if got := truncate("あい", 3); got != "あい" {
		t.Errorf("got %q", got)
	}
}
