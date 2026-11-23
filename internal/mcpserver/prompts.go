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
// writable が false（--read-only）なら、書き込むプロンプトは登録せず、ほかのプロンプトも
// 書き込まない文にする。
func registerPrompts(s *mcp.Server, writable bool) {
	if writable {
		s.AddPrompt(&mcp.Prompt{
			Name:        "write_note",
			Title:       "ノートを書く",
			Description: "既にあるノートとタグを確かめてから、taggo の書き方でノートを書く（新しく作るか、既にあるノートに書き足す）",
			Arguments: []*mcp.PromptArgument{
				{Name: "topic", Title: "話題", Description: "何についてのノートか", Required: true},
				{Name: "material", Title: "材料", Description: "元にするメモや文章（任意）"},
			},
		}, writeNote)
	}

	s.AddPrompt(&mcp.Prompt{
		Name:        "summarize_tag",
		Title:       "タグをまとめる",
		Description: "そのタグを付けたノートを読み、タグが何を表しているのかと、ノートごとの要点をまとめる",
		Arguments: []*mcp.PromptArgument{
			{Name: "tag", Title: "タグ", Description: "まとめるタグ", Required: true},
		},
	}, summarizeTag(writable))

	if writable {
		s.AddPrompt(&mcp.Prompt{
			Name:        "write_missing_pages",
			Title:       "まだ無いページを書く",
			Description: "使われているのにページがまだ無いタグを選び、そのタグが何を表すのかを書いたページを作る",
			Arguments: []*mcp.PromptArgument{
				{Name: "limit", Title: "数", Description: "いくつのタグについて書くか（既定 5）"},
			},
		}, writeMissingPages)

		s.AddPrompt(&mcp.Prompt{
			Name:        "connect_note",
			Title:       "ノートをつなげる",
			Description: "ノートに付けるとよいタグと、つなげるとよいノートを探し、選んだものを書き込む",
			Arguments: []*mcp.PromptArgument{
				{Name: "note", Title: "ノート", Description: "つなげるノートのファイル名", Required: true},
			},
		}, connectNote)
	}

	s.AddPrompt(&mcp.Prompt{
		Name:        "review_recent",
		Title:       "最近のノートを振り返る",
		Description: "最近更新したノートを読み、話題ごとにまとめて、付け足すとよいタグを挙げる",
		Arguments: []*mcp.PromptArgument{
			{Name: "days", Title: "日数", Description: "何日前からのノートを振り返るか（既定 7）"},
		},
	}, reviewRecent(writable))
}

func writeNote(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	topic := strings.TrimSpace(req.Params.Arguments["topic"])
	if topic == "" {
		return nil, fmt.Errorf("topic に、何についてのノートかを入れてください")
	}
	text := fmt.Sprintf(`taggo に「%s」についてのノートを書きたい。次の順で進めて。
1. search_notes と list_tags で、同じ話題のノートと、使えそうなタグの表記を確かめる。
   同じ話題のノートが既にあれば、新しく作るか、そのノートに書き足すかを聞く。
2. 下書きを見せる。新しく作るなら、タイトル（＝ファイル名＝タグ）、付けるタグ、本文。
   関係するノートは [[ノート名]] か Markdown のリンクで指す。
3. 私が了承したら、create_note か append_to_note で書く。warnings と、ページの無いタグがあれば伝える。`, topic)
	if material := strings.TrimSpace(req.Params.Arguments["material"]); material != "" {
		text += "\n\n元にする材料：\n" + material
	}
	return userPrompt("「"+topic+"」のノートを書く", text), nil
}

func summarizeTag(writable bool) mcp.PromptHandler {
	return func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		tag := strings.TrimSpace(req.Params.Arguments["tag"])
		if tag == "" {
			return nil, fmt.Errorf("tag にまとめるタグを入れてください")
		}
		text := fmt.Sprintf(`タグ「%[1]s」を付けたノートをまとめて。
1. list_notes（tag=%[1]s）でノートを集め、read_note で読む。多ければ、更新日時の新しいものから 20 件まで。
2. このタグが何を表しているのか、ノートごとの要点（[[ノート名]] で示す）、ノートの間の食い違いや、抜けている話題を書く。
`, tag)
		if writable {
			text += fmt.Sprintf(`3. タグのページ（%[1]s.md）がまだ無ければ作るか、あれば書き直すかを聞く。
   タグのページには、そのタグが何を表すのかを書く。タグを持つノートの一覧は taggo の関連ページに出るので、本文に並べ直さない。
   作るときは create_note（title は %[1]s）で、書き直すときは read_note で読んでから edit_note で変えるところだけを置き換える。`, tag)
		} else {
			text += fmt.Sprintf(`3. タグのページ（%[1]s.md）がまだ無ければ、まとめを元にしたタグのページの下書きを見せる。
   タグのページには、そのタグが何を表すのかを書く。タグを持つノートの一覧は taggo の関連ページに出るので、本文に並べ直さない。
   ノートの書き換えは taggo の画面で私がするので、書き足す文はそのまま写せる形で示す。`, tag)
		}
		return userPrompt("タグ「"+tag+"」をまとめる", text), nil
	}
}

func writeMissingPages(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	limit, err := intArgument(req, "limit", 5, 1, 50)
	if err != nil {
		return nil, err
	}
	text := fmt.Sprintf(`使われているのにページがまだ無いタグを、件数の多い順に %d 個まで選んで、タグのページを書きたい。
1. list_tags（missing_page_only=true）で候補を出す。
2. 1 つずつ get_backlinks で、そのタグを使っているノートを見て、そのタグが何を表しているのかを読み取り、短いページの下書きを作る。
   タグを持つノートの一覧は taggo の関連ページに出るので、本文に並べ直さない。
3. 下書きをまとめて見せ、私が選んだものだけを create_note で作る（title はタグの表記のまま）。`, limit)
	return userPrompt(fmt.Sprintf("まだ無いページを %d 個まで書く", limit), text), nil
}

func connectNote(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	note := strings.TrimSpace(req.Params.Arguments["note"])
	if note == "" {
		return nil, fmt.Errorf("note につなげるノートのファイル名を入れてください")
	}
	text := fmt.Sprintf(`ノート「%s」を、ほかのノートとつなげたい。
1. read_note で読み、主な話題の語で search_notes を呼び、list_tags でタグの表記を確かめる。
2. 次を一覧にして見せる。
   - このノートに付けるとよいタグ（既にあるタグの表記を使う）
   - このノートからリンクするとよいノート
   - このノートを指すとよいノート（そのノートに、このノートのタグを付ける）
3. 私が選んだものだけを、add_tags か edit_note で書く。edit_note の前には read_note で読む。`, note)
	return userPrompt("「"+note+"」をつなげる", text), nil
}

func reviewRecent(writable bool) mcp.PromptHandler {
	return func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		days, err := intArgument(req, "days", 7, 1, 366)
		if err != nil {
			return nil, err
		}
		// Claude に日付を数えさせないよう、サーバーで日付にしてから渡す。
		since := now().AddDate(0, 0, -days).Format(time.DateOnly)
		text := fmt.Sprintf(`%[1]s 以降に更新したノートを振り返りたい。
1. list_notes（modified_after=%[1]s）で集めて読む。
2. 話題ごとにまとめ、[[ノート名]] で示す。
`, since)
		if writable {
			text += "3. タグを付け足すとよいノートを挙げ（既にあるタグの表記は list_tags で確かめる）、私が選んだものに add_tags で付ける。"
		} else {
			text += `3. タグを付け足すとよいノートを挙げる。既にあるタグの表記は list_tags で確かめる。
   ノートの書き換えは taggo の画面で私がするので、書き足す [[タグ]] の文字列をそのまま示す。`
		}
		return userPrompt(fmt.Sprintf("%d 日間のノートを振り返る", days), text), nil
	}
}

// intArgument は、プロンプトの引数 name を整数として読む。省かれていれば def を返す。
func intArgument(req *mcp.GetPromptRequest, name string, def, minValue, maxValue int) (int, error) {
	raw := strings.TrimSpace(req.Params.Arguments[name])
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < minValue || n > maxValue {
		return 0, fmt.Errorf("%s には %d〜%d の数を入れてください: %q", name, minValue, maxValue, raw)
	}
	return n, nil
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
