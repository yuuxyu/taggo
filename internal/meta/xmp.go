package meta

import (
	"html"
	"regexp"
	"strings"
)

// XMP パケットの dc:subject バッグを書き換えるためのユーティリティ。
//
// 既存パケットがある場合は dc:subject の要素だけを差し替え、それ以外のプロパティ
// （著作権・撮影条件など他ツールが書いた情報）はそのまま残す。
// 完全な XMP ライブラリを持ち込むより、この差し替えのほうが情報の取りこぼしが少ない。

var (
	dcSubjectRe    = regexp.MustCompile(`(?s)<dc:subject\b.*?</dc:subject>|<dc:subject\b[^>]*/>`)
	rdfDescOpenRe  = regexp.MustCompile(`<rdf:Description\b[^>]*>`)
	rdfDescEmptyRe = regexp.MustCompile(`<rdf:Description\b([^>]*)/>`)
)

// buildXMPPacket は tags を dc:subject として反映した XMP パケットを返す。
// existing が空、または rdf:Description を含まない場合は最小構成のパケットを新規生成する。
func buildXMPPacket(existing []byte, tags []string) []byte {
	subject := renderDCSubject(tags)

	body := string(existing)
	if strings.TrimSpace(body) == "" {
		return []byte(newXMPPacket(subject))
	}

	// 既存の dc:subject はいったんすべて取り除く。
	body = dcSubjectRe.ReplaceAllString(body, "")

	// 自己終端の <rdf:Description .../> には子要素を入れられないので開始・終了タグ形式へ開く。
	body = rdfDescEmptyRe.ReplaceAllString(body, "<rdf:Description$1></rdf:Description>")

	if subject == "" {
		return []byte(body) // タグが無いなら dc:subject を消しただけで完了
	}

	loc := rdfDescOpenRe.FindStringIndex(body)
	if loc == nil {
		// rdf:Description が見当たらないパケットは構造を推測できないため作り直す。
		return []byte(newXMPPacket(subject))
	}
	return []byte(body[:loc[1]] + subject + body[loc[1]:])
}

// renderDCSubject は dc:subject 要素を組み立てる。tags が空なら空文字を返す。
func renderDCSubject(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("<dc:subject><rdf:Bag>")
	for _, t := range tags {
		b.WriteString("<rdf:li>")
		b.WriteString(html.EscapeString(t))
		b.WriteString("</rdf:li>")
	}
	b.WriteString("</rdf:Bag></dc:subject>")
	return b.String()
}

// newXMPPacket は dc:subject だけを含む最小構成の XMP パケットを生成する。
func newXMPPacket(subject string) string {
	// パケット先頭の BOM は XMP 仕様が要求するもの。ソース上は明示的にエスケープして書く。
	return "<?xpacket begin=\"\ufeff\" id=\"W5M0MpCehiHzreSzNTczkc9d\"?>" +
		`<x:xmpmeta xmlns:x="adobe:ns:meta/" x:xmptk="taggo">` +
		`<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">` +
		`<rdf:Description rdf:about="" xmlns:dc="http://purl.org/dc/elements/1.1/">` +
		subject +
		`</rdf:Description></rdf:RDF></x:xmpmeta>` +
		`<?xpacket end="w"?>`
}
