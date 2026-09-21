// Package linkcard は、Markdown 本文に貼られた URL から、リンクカードに出す
// タイトル・説明・画像を集める。
//
// 一般のページは HTML の OGP（og:title など）を読む。
// YouTube と X は、それぞれが公開している oEmbed を使う。X のページは
// ブラウザ以外に OGP を返さず、YouTube は地域によって同意画面へ飛ばされるため、
// HTML を読むより oEmbed のほうが確実に情報を取れる。
//
// 取得先は外部の公開サーバーに限る。本文に書かれた URL を手掛かりに、
// 手元のネットワーク内の機器へリクエストを送らせないため。
package linkcard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
	"golang.org/x/sync/singleflight"
)

// カードの種類。フロントエンドはこれを見てカードの見た目を変える。
const (
	KindPage    = "page"
	KindYouTube = "youtube"
	KindX       = "x"
)

// Preview はリンクカードに出す内容。
type Preview struct {
	// URL は本文に書かれていた URL。リダイレクト後の URL ではない。
	URL string `json:"url"`
	// Kind は KindPage / KindYouTube / KindX のいずれか。
	Kind string `json:"kind"`
	// Title はページの題名。X では投稿者の表示名。
	Title string `json:"title"`
	// Description はページの説明。X では投稿の本文。
	Description string `json:"description,omitempty"`
	// Image はサムネイル画像の絶対 URL。
	Image string `json:"image,omitempty"`
	// SiteName はサイト名。
	SiteName string `json:"siteName,omitempty"`
	// Author は動画のチャンネル名や、投稿者の @ から始まる ID。
	Author string `json:"author,omitempty"`
}

const (
	// maxBody は読み込む HTML の上限。OGP はページの先頭近くにあるので、先頭だけ読めば足りる。
	maxBody = 1 << 20
	// maxDescription は説明文を切り詰める文字数。カードでは数行しか出さない。
	maxDescription = 300
	// maxCache はキャッシュに持つ URL の数の上限。
	maxCache = 500
	// okTTL と errTTL はキャッシュの有効期間。失敗は一時的なこともあるので短めにする。
	okTTL  = time.Hour
	errTTL = 5 * time.Minute

	userAgent = "Mozilla/5.0 (compatible; taggo link preview)"
)

const (
	youtubeOEmbed = "https://www.youtube.com/oembed"
	xOEmbed       = "https://publish.twitter.com/oembed"
)

// ErrBlockedAddress は、取得先が外部の公開アドレスでないときに返る。
var ErrBlockedAddress = errors.New("ローカルネットワークのアドレスには接続しません")

// Client は URL ごとのプレビューを取得し、しばらくのあいだ覚えておく。
type Client struct {
	http          *http.Client
	youtubeOEmbed string
	xOEmbed       string
	now           func() time.Time

	group singleflight.Group
	mu    sync.Mutex
	cache map[string]cached
}

type cached struct {
	preview *Preview
	err     error
	expires time.Time
}

// New は外部の公開アドレスにだけ接続するクライアントを返す。
func New() *Client {
	dialer := &net.Dialer{
		Timeout: 5 * time.Second,
		// 名前解決したあとの接続先アドレスで判定する。ホスト名だけ見ても、
		// 公開ドメインがローカルのアドレスを指していれば素通りしてしまう。
		Control: func(_, address string, _ syscall.RawConn) error {
			ap, err := netip.ParseAddrPort(address)
			if err != nil {
				return err
			}
			if !isPublic(ap.Addr()) {
				return ErrBlockedAddress
			}
			return nil
		},
	}
	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		MaxIdleConns:          10,
		IdleConnTimeout:       30 * time.Second,
	}
	return newClient(&http.Client{
		Transport:     transport,
		Timeout:       15 * time.Second,
		CheckRedirect: checkRedirect,
	}, youtubeOEmbed, xOEmbed)
}

func newClient(hc *http.Client, youtube, x string) *Client {
	return &Client{
		http:          hc,
		youtubeOEmbed: youtube,
		xOEmbed:       x,
		now:           time.Now,
		cache:         make(map[string]cached),
	}
}

// checkRedirect はリダイレクトを 5 回までに抑え、http(s) 以外への転送を断る。
func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return errors.New("リダイレクトが多すぎます")
	}
	if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
		return fmt.Errorf("http(s) 以外へのリダイレクトは追いません: %s", req.URL.Scheme)
	}
	return nil
}

// isPublic は、そのアドレスがインターネット上の公開アドレスかどうかを返す。
func isPublic(addr netip.Addr) bool {
	addr = addr.Unmap()
	return addr.IsValid() &&
		!addr.IsLoopback() &&
		!addr.IsPrivate() &&
		!addr.IsLinkLocalUnicast() &&
		!addr.IsLinkLocalMulticast() &&
		!addr.IsInterfaceLocalMulticast() &&
		!addr.IsMulticast() &&
		!addr.IsUnspecified() &&
		// 100.64.0.0/10（キャリアグレード NAT）も手元のネットワークとみなす。
		!netip.MustParsePrefix("100.64.0.0/10").Contains(addr)
}

// Fetch は URL のプレビューを返す。同じ URL は一定時間キャッシュから返し、
// 同時に来た同じ URL の要求は 1 回の取得にまとめる。
func (c *Client) Fetch(ctx context.Context, raw string) (*Preview, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("http(s) の URL ではありません: %s", raw)
	}
	key := u.String()

	if p, err, ok := c.lookup(key); ok {
		return p, err
	}
	v, err, _ := c.group.Do(key, func() (any, error) {
		p, err := c.fetch(ctx, u)
		c.store(key, p, err)
		return p, err
	})
	if err != nil {
		return nil, err
	}
	return v.(*Preview), nil
}

func (c *Client) lookup(key string) (*Preview, error, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.cache[key]
	if !ok || c.now().After(e.expires) {
		return nil, nil, false
	}
	return e.preview, e.err, true
}

func (c *Client) store(key string, p *Preview, err error) {
	// 呼び出し元の都合で打ち切られた取得は、次に開いたときにやり直せるよう覚えない。
	if errors.Is(err, context.Canceled) {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if len(c.cache) >= maxCache {
		for k, e := range c.cache {
			if now.After(e.expires) {
				delete(c.cache, k)
			}
		}
		// 期限切れを捨てても空かなければ、全部を捨てて作り直す。
		if len(c.cache) >= maxCache {
			c.cache = make(map[string]cached)
		}
	}
	ttl := okTTL
	if err != nil {
		ttl = errTTL
	}
	c.cache[key] = cached{preview: p, err: err, expires: now.Add(ttl)}
}

func (c *Client) fetch(ctx context.Context, u *url.URL) (*Preview, error) {
	switch {
	case isYouTube(u):
		p, err := c.fetchYouTube(ctx, u)
		if err == nil {
			return p, nil
		}
		// チャンネルのページのように oEmbed が扱わない URL は、ページの OGP を読む。
		return c.fetchPage(ctx, u)
	case isXPost(u):
		return c.fetchX(ctx, u)
	default:
		return c.fetchPage(ctx, u)
	}
}

// hostOf は、比較しやすいように小文字にして先頭の www. / m. を落としたホスト名を返す。
func hostOf(u *url.URL) string {
	h := strings.ToLower(u.Hostname())
	for _, prefix := range []string{"www.", "m.", "mobile."} {
		h = strings.TrimPrefix(h, prefix)
	}
	return h
}

func isYouTube(u *url.URL) bool {
	switch hostOf(u) {
	case "youtube.com", "music.youtube.com", "youtu.be":
		return true
	}
	return false
}

// isXPost は、X（Twitter）の個別の投稿を指す URL かどうかを返す。
// oEmbed で本文が取れるのは投稿だけなので、プロフィールなどは対象にしない。
func isXPost(u *url.URL) bool {
	switch hostOf(u) {
	case "x.com", "twitter.com":
	default:
		return false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	return len(parts) >= 3 && parts[1] == "status" && parts[2] != ""
}

// getJSON は endpoint へ query を付けて GET し、JSON を v へ読み込む。
func (c *Client) getJSON(ctx context.Context, endpoint string, query url.Values, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+query.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("oEmbed の取得に失敗しました: %s", resp.Status)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(v)
}

func (c *Client) fetchYouTube(ctx context.Context, u *url.URL) (*Preview, error) {
	var res struct {
		Title        string `json:"title"`
		AuthorName   string `json:"author_name"`
		ThumbnailURL string `json:"thumbnail_url"`
	}
	q := url.Values{"url": {u.String()}, "format": {"json"}}
	if err := c.getJSON(ctx, c.youtubeOEmbed, q, &res); err != nil {
		return nil, err
	}
	if res.Title == "" {
		return nil, errors.New("動画のタイトルが見つかりません")
	}
	return &Preview{
		URL:      u.String(),
		Kind:     KindYouTube,
		Title:    clean(res.Title),
		Image:    absoluteURL(u, res.ThumbnailURL),
		SiteName: "YouTube",
		Author:   clean(res.AuthorName),
	}, nil
}

func (c *Client) fetchX(ctx context.Context, u *url.URL) (*Preview, error) {
	var res struct {
		AuthorName string `json:"author_name"`
		AuthorURL  string `json:"author_url"`
		HTML       string `json:"html"`
	}
	q := url.Values{
		"url":         {u.String()},
		"omit_script": {"true"},
		"dnt":         {"true"},
		"lang":        {"ja"},
	}
	if err := c.getJSON(ctx, c.xOEmbed, q, &res); err != nil {
		return nil, err
	}
	text := tweetText(res.HTML)
	if res.AuthorName == "" && text == "" {
		return nil, errors.New("投稿の内容が見つかりません")
	}

	author := ""
	if au, err := url.Parse(res.AuthorURL); err == nil {
		if name := strings.Trim(au.Path, "/"); name != "" {
			author = "@" + name
		}
	}
	return &Preview{
		URL:         u.String(),
		Kind:        KindX,
		Title:       clean(res.AuthorName),
		Description: truncate(strings.TrimSpace(text), maxDescription),
		SiteName:    "X",
		Author:      author,
	}, nil
}

// tweetText は、oEmbed が返す埋め込み用 HTML から投稿の本文を取り出す。
// 本文は <blockquote> の最初の <p> に入っていて、改行は <br> で表される。
func tweetText(src string) string {
	z := html.NewTokenizer(strings.NewReader(src))
	var b strings.Builder
	inP := false
	for {
		switch z.Next() {
		case html.ErrorToken:
			return b.String()
		case html.StartTagToken, html.SelfClosingTagToken:
			name, _ := z.TagName()
			switch string(name) {
			case "p":
				inP = true
			case "br":
				if inP {
					b.WriteByte('\n')
				}
			}
		case html.EndTagToken:
			if name, _ := z.TagName(); string(name) == "p" && inP {
				return b.String()
			}
		case html.TextToken:
			if inP {
				b.Write(z.Text())
			}
		}
	}
}

func (c *Client) fetchPage(ctx context.Context, u *url.URL) (*Preview, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9,*/*;q=0.1")
	req.Header.Set("Accept-Language", "ja,en;q=0.8")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ページの取得に失敗しました: %s", resp.Status)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "" {
		mt, _, _ := mime.ParseMediaType(contentType)
		if mt != "text/html" && mt != "application/xhtml+xml" {
			return nil, fmt.Errorf("HTML ではありません: %s", mt)
		}
	}
	// Shift_JIS や EUC-JP のページもあるので、宣言された文字コードから UTF-8 へ直す。
	body, err := charset.NewReader(io.LimitReader(resp.Body, maxBody), contentType)
	if err != nil {
		return nil, err
	}
	h := parseHead(body)

	// 相対 URL の画像は、リダイレクトを追ったあとの URL を基準に解決する。
	base := resp.Request.URL
	p := &Preview{
		URL:         u.String(),
		Kind:        KindPage,
		Title:       clean(first(h.meta["og:title"], h.meta["twitter:title"], h.title)),
		Description: truncate(clean(first(h.meta["og:description"], h.meta["twitter:description"], h.meta["description"])), maxDescription),
		Image: absoluteURL(base, first(
			h.meta["og:image"], h.meta["og:image:url"], h.meta["og:image:secure_url"],
			h.meta["twitter:image"], h.meta["twitter:image:src"],
		)),
		SiteName: clean(first(h.meta["og:site_name"], base.Hostname())),
	}
	if p.Title == "" {
		return nil, errors.New("ページのタイトルが見つかりません")
	}
	return p, nil
}

// head は HTML から読み取った内容。meta はキーを小文字にしてあり、最初に現れた値を持つ。
type head struct {
	title string
	meta  map[string]string
}

// parseHead は HTML から <title> と <meta> を集める。
// YouTube のように <body> の中へ <meta> を置くページもあるので、<head> を
// 抜けても読み続ける。同じキーが何度も現れたら、最初の値を使う。
func parseHead(r io.Reader) head {
	h := head{meta: make(map[string]string)}
	z := html.NewTokenizer(r)
	inTitle, sawTitle := false, false
	for {
		switch z.Next() {
		case html.ErrorToken:
			return h
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			switch string(name) {
			case "meta":
				if hasAttr {
					addMeta(z, h.meta)
				}
			case "title":
				// <svg> の中の <title> などを拾わないよう、最初の 1 つだけ使う。
				inTitle = !sawTitle
				sawTitle = true
			}
		case html.EndTagToken:
			if name, _ := z.TagName(); string(name) == "title" {
				inTitle = false
			}
		case html.TextToken:
			if inTitle {
				h.title += string(z.Text())
			}
		}
	}
}

// addMeta は <meta property="og:…" content="…"> や <meta name="…" content="…"> を meta へ足す。
func addMeta(z *html.Tokenizer, meta map[string]string) {
	var key, content string
	hasContent := false
	for {
		k, v, more := z.TagAttr()
		switch string(k) {
		case "property", "name":
			if key == "" {
				key = strings.ToLower(strings.TrimSpace(string(v)))
			}
		case "content":
			content, hasContent = string(v), true
		}
		if !more {
			break
		}
	}
	if key == "" || !hasContent || strings.TrimSpace(content) == "" {
		return
	}
	if _, ok := meta[key]; !ok {
		meta[key] = content
	}
}

// absoluteURL は ref を base 基準の絶対 URL にする。http(s) でなければ空を返す。
func absoluteURL(base *url.URL, ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	u, err := base.Parse(ref)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	return u.String()
}

func first(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// clean は改行や連続する空白を 1 つの空白へまとめる。
func clean(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// truncate は s を n 文字（バイトではなく文字）までに切り詰める。
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n])) + "…"
}
