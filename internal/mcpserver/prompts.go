package mcpserver

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// now は今の日時。テストでは差し替える。
var now = time.Now

// registerPrompts は、利用者がクライアントのメニューから選んで使うプロンプトを登録する。
func registerPrompts(s *mcp.Server) {
	s.AddPrompt(&mcp.Prompt{
		Name:        "summarize_tag",
		Title:       "タグをまとめる",
		Description: "そのタグを付けたノートを読み、タグが何を表しているのかと、ノートごとの要点をまとめる",
		Arguments: []*mcp.PromptArgument{
			{Name: "tag", Title: "タグ", Description: "まとめるタグ", Required: true},
		},
	}, summarizeTag)

	s.AddPrompt(&mcp.Prompt{
		Name:        "review_recent",
		Title:       "最近のノートを振り返る",
		Description: "最近更新したノートを読み、話題ごとにまとめて、付け足すとよいタグを挙げる",
		Arguments: []*mcp.PromptArgument{
			{Name: "days", Title: "日数", Description: "何日前からのノートを振り返るか（既定 7）"},
		},
	}, reviewRecent)
}

func summarizeTag(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	tag := strings.TrimSpace(req.Params.Arguments["tag"])
	if tag == "" {
		return nil, fmt.Errorf("tag にまとめるタグを入れてください")
	}
	text := fmt.Sprintf(`タグ「%[1]s」を付けたノートをまとめて。
1. list_notes（tag=%[1]s）でノートを集め、read_note で読む。多ければ、更新日時の新しいものから 20 件まで。
2. このタグが何を表しているのか、ノートごとの要点（[[ノート名]] で示す）、ノートの間の食い違いや、抜けている話題を書く。
3. タグのページ（%[1]s.md）がまだ無ければ、まとめを元にしたタグのページの下書きを見せる。
   タグのページには、そのタグが何を表すのかを書く。タグを持つノートの一覧は taggo の関連ページに出るので、本文に並べ直さない。
   ノートの書き換えは taggo の画面で私がするので、書き足す文はそのまま写せる形で示す。`, tag)
	return userPrompt("タグ「"+tag+"」をまとめる", text), nil
}

func reviewRecent(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	days := 7
	if raw := strings.TrimSpace(req.Params.Arguments["days"]); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 366 {
			return nil, fmt.Errorf("days には 1〜366 の日数を入れてください: %q", raw)
		}
		days = n
	}
	// Claude に日付を数えさせないよう、サーバーで日付にしてから渡す。
	since := now().AddDate(0, 0, -days).Format(time.DateOnly)
	text := fmt.Sprintf(`%[1]s 以降に更新したノートを振り返りたい。
1. list_notes（modified_after=%[1]s）で集めて読む。
2. 話題ごとにまとめ、[[ノート名]] で示す。
3. タグを付け足すとよいノートを挙げる。既にあるタグの表記は list_tags で確かめる。
   ノートの書き換えは taggo の画面で私がするので、書き足す [[タグ]] の文字列をそのまま示す。`, since)
	return userPrompt(fmt.Sprintf("%d 日間のノートを振り返る", days), text), nil
}

// userPrompt は、利用者の発言 1 つだけのプロンプトを作る。
func userPrompt(description, text string) *mcp.GetPromptResult {
	return &mcp.GetPromptResult{
		Description: description,
		Messages: []*mcp.PromptMessage{
			{Role: "user", Content: &mcp.TextContent{Text: text}},
		},
	}
}
