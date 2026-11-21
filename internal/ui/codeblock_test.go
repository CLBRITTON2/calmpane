package ui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestCodeBlocks(t *testing.T) {
	t.Run("replyRowsFrameEachCodeBlockAtReplyWidthAndHintOnlyNumberedBlock", replyRowsFrameEachCodeBlockAtReplyWidthAndHintOnlyNumberedBlock)
	t.Run("codeFrameRowsWrapLongLineInsideFrame", codeFrameRowsWrapLongLineInsideFrame)
	t.Run("topRuleDropsHintThenLanguageWhenTooNarrow", topRuleDropsHintThenLanguageWhenTooNarrow)
	t.Run("replyRowsKeepTextAroundCodeBlock", replyRowsKeepTextAroundCodeBlock)
	t.Run("codeBlocksHoldCodeExactlyAsInReply", codeBlocksHoldCodeExactlyAsInReply)
	t.Run("codeFrameRowsDrawCommentInBoldIris", codeFrameRowsDrawCommentInBoldIris)
}

func codeFrameRowsDrawCommentInBoldIris(t *testing.T) {
	block := codeBlock{code: "# 1. Restart zen so it draws the new colors.\ngit status", language: "powershell"}
	rows, err := CodeFrameRows(block, "", 80)
	if err != nil {
		t.Fatalf("CodeFrameRows: %v", err)
	}
	// bold, and iris #c4a7e7 as a 24-bit foreground
	boldIris := "\x1b[1m\x1b[38;2;196;167;231m"
	commentRow, commandRow := rows[1], rows[2]
	if !strings.Contains(commentRow, boldIris) {
		t.Errorf("comment row = %q, want it in bold iris %q", commentRow, boldIris)
	}
	if strings.Contains(commandRow, "38;2;196;167;231") {
		t.Errorf("command row = %q, want it in another color than the comment's iris", commandRow)
	}
}

func replyRowsFrameEachCodeBlockAtReplyWidthAndHintOnlyNumberedBlock(t *testing.T) {
	reply := "Run this:\n\n```powershell\nGet-Process zen |\n\tStop-Process -Force\n```\n\nThen this:\n\n```\nherdr --remote claude-vm\n```\n\nDone."
	for _, width := range []int{40, 100} {
		replyRenderer, err := NewReplyRenderer(width)
		if err != nil {
			t.Fatalf("NewReplyRenderer(%d): %v", width, err)
		}
		rows, err := ReplyRows(reply, 1, width, replyRenderer)
		if err != nil {
			t.Fatalf("ReplyRows at width %d: %v", width, err)
		}
		plain := []string{}
		for _, row := range rows {
			plain = append(plain, ansi.Strip(row))
		}
		topRules, bottomRules := []string{}, []string{}
		inFrame := false
		for index, row := range plain {
			switch {
			case strings.HasPrefix(row, "╭"):
				topRules = append(topRules, row)
				inFrame = true
			case strings.HasPrefix(row, "╰"):
				bottomRules = append(bottomRules, row)
				inFrame = false
			case inFrame:
				if strings.ContainsAny(row, "╭╮╰╯│─") || strings.Contains(rows[index], "\x1b[48") {
					t.Errorf("width %d: code row %q holds a frame character or a background", width, rows[index])
				}
			}
			if strings.HasPrefix(row, "╭") || strings.HasPrefix(row, "╰") {
				if ruleWidth := ansi.StringWidth(row); ruleWidth != width-2 {
					t.Errorf("width %d: rule %q is %d columns wide, want %d", width, row, ruleWidth, width-2)
				}
			}
		}
		if len(topRules) != 2 || len(bottomRules) != 2 {
			t.Fatalf("width %d: rows = %q, want two frames", width, plain)
		}
		if !strings.HasPrefix(topRules[0], "╭─ powershell ─") || strings.Contains(topRules[0], copyKeyHint) {
			t.Errorf("width %d: first top rule = %q, want the language and no hint", width, topRules[0])
		}
		if !strings.HasPrefix(topRules[1], "╭──") || !strings.HasSuffix(topRules[1], "─ "+copyKeyHint+" ─╮") {
			t.Errorf("width %d: newest top rule = %q, want no language and the hint at its right end", width,
				topRules[1])
		}
		joined := strings.Join(plain, "\n")
		for _, codeRow := range []string{"\n  Get-Process zen |\n", "\n      Stop-Process -Force\n",
			"\n  herdr --remote claude-vm\n"} {
			if !strings.Contains(joined, codeRow) {
				t.Errorf("width %d: rows = %q, want the code row %q indented 2", width, plain, codeRow)
			}
		}
		if !strings.Contains(joined, "Run this:") || !strings.Contains(joined, "Done.") ||
			strings.Contains(joined, "ZENCODEBLOCK") {
			t.Errorf("width %d: reply rows = %q, want the prose around the frames and no placeholder", width, joined)
		}
	}
}

func codeFrameRowsWrapLongLineInsideFrame(t *testing.T) {
	block := codeBlock{code: "Get-Process -Name zen | Stop-Process -Force -PassThru -Verbose", language: "powershell"}
	rows, err := CodeFrameRows(block, "", 30)
	if err != nil {
		t.Fatalf("CodeFrameRows: %v", err)
	}
	if len(rows) < 4 {
		t.Fatalf("rows = %q, want the code wrapped onto at least two rows between the rules", rows)
	}
	codeText := ""
	for _, row := range rows[1 : len(rows)-1] {
		if rowWidth := ansi.StringWidth(row); rowWidth > 30 {
			t.Errorf("code row %q is %d columns wide, more than the frame's 30", ansi.Strip(row), rowWidth)
		}
		codeText += ansi.Strip(row)
	}
	if strings.ReplaceAll(codeText, " ", "") != strings.ReplaceAll(block.code, " ", "") {
		t.Errorf("wrapped rows hold %q, want the characters of %q", codeText, block.code)
	}
}

func topRuleDropsHintThenLanguageWhenTooNarrow(t *testing.T) {
	cases := []struct {
		width int
		want  string
	}{
		{width: 34, want: "╭─ go ─────────── ctrl+y copies ─╮"},
		{width: 20, want: "╭─ go ─────────────╮"},
		{width: 6, want: "╭────╮"},
	}
	for _, testCase := range cases {
		if got := ansi.Strip(TopRule("go", copyKeyHint, testCase.width)); got != testCase.want {
			t.Errorf("TopRule at width %d = %q, want %q", testCase.width, got, testCase.want)
		}
	}
}

func replyRowsKeepTextAroundCodeBlock(t *testing.T) {
	// the rules of a frame in a reply 60 columns wide, which draws 58
	shTopRule := "╭─ sh " + strings.Repeat("─", 51) + "╮"
	plainTopRule := "╭" + strings.Repeat("─", 56) + "╮"
	bottomRule := "╰" + strings.Repeat("─", 56) + "╯"
	cases := []struct {
		name  string
		reply string
		want  []string
	}{
		{name: "in a list item", reply: "1. First step:\n   ```sh\n   echo one\n   ```\n   then this.\n2. Second step",
			want: []string{"", "1. First step:", shTopRule, "echo one", bottomRule,
				"then this.", "2. Second step"}},
		{name: "in a quote", reply: "> Run:\n> ```sh\n> echo one\n> ```\n> done",
			want: []string{"", "│ Run:", "│", shTopRule, "echo one", bottomRule, "│",
				"│ done"}},
		{name: "prose that spells the placeholder", reply: "The row ZENCODEBLOCK0 stays text.\n\n```\necho one\n```",
			want: []string{"The row ZENCODEBLOCK0 stays text.", "", plainTopRule, "echo one", bottomRule}},
		{name: "under a heading", reply: "Done.\n\n## Do This Now\n\n```\n# 1. Reopen zen.\n```",
			want: []string{"Done.", "", "Do This Now", plainTopRule, "# 1. Reopen zen.", bottomRule}},
	}
	replyRenderer, err := NewReplyRenderer(60)
	if err != nil {
		t.Fatalf("NewReplyRenderer: %v", err)
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			rows, err := ReplyRows(testCase.reply, -1, 60, replyRenderer)
			if err != nil {
				t.Fatalf("ReplyRows: %v", err)
			}
			plain := []string{}
			for _, row := range rows {
				plain = append(plain, strings.TrimSpace(ansi.Strip(row)))
			}
			if !reflect.DeepEqual(plain, testCase.want) {
				t.Errorf("rows = %q, want %q", plain, testCase.want)
			}
		})
	}
}

func codeBlocksHoldCodeExactlyAsInReply(t *testing.T) {
	cases := []struct {
		name  string
		reply string
		want  []string
	}{
		{name: "a tab and a trailing space", reply: "Run:\n\n```go\nfunc f() {\n\treturn \n}\n```\n",
			want: []string{"func f() {\n\treturn \n}"}},
		{name: "in a list item", reply: "1. Step:\n   ```sh\n   echo one\n     indented\n   ```\n2. Next",
			want: []string{"echo one\n  indented"}},
		{name: "in a quote", reply: "> ```\n> inside\n> ```", want: []string{"inside"}},
		{name: "tilde fence left open", reply: "~~~\nfirst\n\nlast", want: []string{"first\n\nlast"}},
		{name: "two blocks", reply: "```\none\n```\n\n```\ntwo\n```", want: []string{"one", "two"}},
		{name: "empty block", reply: "```\n```", want: []string{}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			blocks, err := CodeBlocks(testCase.reply)
			if err != nil {
				t.Fatalf("CodeBlocks: %v", err)
			}
			codes := []string{}
			for _, block := range blocks {
				codes = append(codes, block.code)
			}
			if !reflect.DeepEqual(codes, testCase.want) {
				t.Errorf("codes = %q, want %q", codes, testCase.want)
			}
		})
	}
}
