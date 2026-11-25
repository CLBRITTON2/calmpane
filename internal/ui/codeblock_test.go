package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"

	"github.com/CLBRITTON2/calmpane/internal/config"
)

func TestCodeBlocks(t *testing.T) {
	t.Run("replyRowsFrameEachCodeBlockAtReplyWidthAndHintOnlyNumberedBlockAtForty",
		replyRowsFrameEachCodeBlockAtReplyWidthAndHintOnlyNumberedBlockAtForty)
	t.Run("replyRowsFrameEachCodeBlockAtReplyWidthAndHintOnlyNumberedBlockAtHundred",
		replyRowsFrameEachCodeBlockAtReplyWidthAndHintOnlyNumberedBlockAtHundred)
	t.Run("codeFrameRowsWrapLongLineInsideFrame", codeFrameRowsWrapLongLineInsideFrame)
	t.Run("codeFrameRulesRepeatConfiguredCodeRuleLine", codeFrameRulesRepeatConfiguredCodeRuleLine)
	t.Run("topRuleShowsLanguageAndHintWhenWideEnough", topRuleShowsLanguageAndHintWhenWideEnough)
	t.Run("topRuleDropsHintWhenTooNarrowForIt", topRuleDropsHintWhenTooNarrowForIt)
	t.Run("topRuleDropsLanguageWhenTooNarrowForIt", topRuleDropsLanguageWhenTooNarrowForIt)
	t.Run("topRuleShowsMarkedAfterLanguage", topRuleShowsMarkedAfterLanguage)
	t.Run("topRuleDropsMarkedWhenTooNarrowForIt", topRuleDropsMarkedWhenTooNarrowForIt)
	t.Run("topRuleShowsMarkedAloneWithoutLanguage", topRuleShowsMarkedAloneWithoutLanguage)
	t.Run("replyRowsKeepTextAroundCodeBlockInListItem", replyRowsKeepTextAroundCodeBlockInListItem)
	t.Run("replyRowsKeepTextAroundCodeBlockInQuote", replyRowsKeepTextAroundCodeBlockInQuote)
	t.Run("replyRowsKeepProseSpellingPlaceholderAsText", replyRowsKeepProseSpellingPlaceholderAsText)
	t.Run("replyRowsPutHeadingRightOnTopRuleOfCodeBlockUnderIt", replyRowsPutHeadingRightOnTopRuleOfCodeBlockUnderIt)
	t.Run("codeBlocksKeepTabAndTrailingSpace", codeBlocksKeepTabAndTrailingSpace)
	t.Run("codeBlocksDropListItemIndent", codeBlocksDropListItemIndent)
	t.Run("codeBlocksDropQuoteMarker", codeBlocksDropQuoteMarker)
	t.Run("codeBlocksTakeTildeFenceLeftOpenToReplyEnd", codeBlocksTakeTildeFenceLeftOpenToReplyEnd)
	t.Run("codeBlocksHoldEachBlockInOrder", codeBlocksHoldEachBlockInOrder)
	t.Run("codeBlocksSkipEmptyBlock", codeBlocksSkipEmptyBlock)
	t.Run("codeFrameRowsDrawCommentInBoldIris", codeFrameRowsDrawCommentInBoldIris)
	t.Run("commentStyleOverridesEveryCommentTypeOfThemeAndEmptyKeepsThemes",
		commentStyleOverridesEveryCommentTypeOfThemeAndEmptyKeepsThemes)
	t.Run("codeFrameRowsDrawPickedFrameWithHeavyIrisRules", codeFrameRowsDrawPickedFrameWithHeavyIrisRules)
	t.Run("codeFrameRowsDrawOtherFramesWithLightRulesNotInIris", codeFrameRowsDrawOtherFramesWithLightRulesNotInIris)
}

// iris #c4a7e7 as a 24-bit foreground
const irisForeground = "38;2;196;167;231"

func codeFrameRowsDrawPickedFrameWithHeavyIrisRules(t *testing.T) {
	rows, err := CodeFrameRows(codeBlock{code: "ls", language: "sh"}, codeFrame{picked: true}, 20, testAppearance)
	require.NoError(t, err)
	top, bottom := rows[0], rows[len(rows)-1]
	plainTop := ansi.Strip(top)
	require.Equal(t, "┏━ sh ━━━━━━━━━━━━━┓", plainTop)
	plainBottom := ansi.Strip(bottom)
	require.Equal(t, "┗━━━━━━━━━━━━━━━━━━┛", plainBottom)
	require.Contains(t, top, irisForeground)
	require.Contains(t, bottom, irisForeground)
}

func codeFrameRowsDrawOtherFramesWithLightRulesNotInIris(t *testing.T) {
	rows, err := CodeFrameRows(codeBlock{code: "ls", language: "sh"}, codeFrame{}, 20, testAppearance)
	require.NoError(t, err)
	top, bottom := rows[0], rows[len(rows)-1]
	plainTop := ansi.Strip(top)
	require.Equal(t, "╭─ sh ─────────────╮", plainTop)
	plainBottom := ansi.Strip(bottom)
	require.Equal(t, "╰──────────────────╯", plainBottom)
	require.NotContains(t, top, irisForeground)
	require.NotContains(t, bottom, irisForeground)
}

func codeFrameRowsDrawCommentInBoldIris(t *testing.T) {
	block := codeBlock{code: "# 1. Restart zen so it draws the new colors.\ngit status", language: "powershell"}
	rows, err := CodeFrameRows(block, codeFrame{}, 80, testAppearance)
	require.NoError(t, err)
	// bold, and iris #c4a7e7 as a 24-bit foreground
	boldIris := "\x1b[1m\x1b[38;2;196;167;231m"
	commentRow, commandRow := rows[1], rows[2]
	require.Contains(t, commentRow, boldIris)
	require.NotContains(t, commandRow, irisForeground)
}

func commentStyleOverridesEveryCommentTypeOfThemeAndEmptyKeepsThemes(t *testing.T) {
	block := codeBlock{code: "// keep the count\ncount := 3", language: "go"}
	boldIris := "\x1b[1m\x1b[38;2;196;167;231m"
	draculaComment := "38;2;98;114;164"
	settings := exampleSettings(t)
	settings.Code = config.Code{Theme: "dracula", Comment: "bold #c4a7e7"}
	overrideAppearance := appearanceFrom(t, settings)
	rows, err := CodeFrameRows(block, codeFrame{}, 80, overrideAppearance)
	require.NoError(t, err)
	require.Contains(t, rows[1], boldIris)
	settings.Code.Comment = ""
	themeAppearance := appearanceFrom(t, settings)
	rows, err = CodeFrameRows(block, codeFrame{}, 80, themeAppearance)
	require.NoError(t, err)
	require.Contains(t, rows[1], draculaComment)
	require.NotContains(t, rows[1], "196;167;231")
}

func assertReplyRowsFrameEachCodeBlockAtReplyWidthAndHintOnlyNumberedBlock(t *testing.T, width int) {
	t.Helper()
	reply := "Run this:\n\n```powershell\nGet-Process zen |\n\tStop-Process -Force\n```\n\nThen this:\n\n```\nherdr --remote claude-vm\n```\n\nDone."
	replyRenderer, err := NewReplyRenderer(testAppearance, width)
	require.NoError(t, err)
	rows, err := ReplyRows(reply, map[int]codeFrame{1: {copyHint: true}}, width, replyRenderer, testAppearance)
	require.NoError(t, err)
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
			require.NotContains(t, row, "╭")
			require.NotContains(t, row, "╮")
			require.NotContains(t, row, "╰")
			require.NotContains(t, row, "╯")
			require.NotContains(t, row, "│")
			require.NotContains(t, row, "─")
			require.NotContains(t, rows[index], "\x1b[48")
		}
		isRule := strings.HasPrefix(row, "╭") || strings.HasPrefix(row, "╰")
		if isRule {
			ruleWidth := ansi.StringWidth(row)
			require.Equal(t, width-2, ruleWidth, row)
		}
	}
	require.Len(t, topRules, 2)
	require.Len(t, bottomRules, 2)
	firstShowsLanguage := strings.HasPrefix(topRules[0], "╭─ powershell ─")
	require.True(t, firstShowsLanguage, topRules[0])
	require.NotContains(t, topRules[0], copyKeyHint)
	newestShowsNoLanguage := strings.HasPrefix(topRules[1], "╭──")
	require.True(t, newestShowsNoLanguage, topRules[1])
	newestEndsWithHint := strings.HasSuffix(topRules[1], "─ "+copyKeyHint+" ─╮")
	require.True(t, newestEndsWithHint, topRules[1])
	joined := strings.Join(plain, "\n")
	require.Contains(t, joined, "\n  Get-Process zen |\n")
	require.Contains(t, joined, "\n      Stop-Process -Force\n")
	require.Contains(t, joined, "\n  herdr --remote claude-vm\n")
	require.Contains(t, joined, "Run this:")
	require.Contains(t, joined, "Done.")
	require.NotContains(t, joined, "ZENCODEBLOCK")
}

func replyRowsFrameEachCodeBlockAtReplyWidthAndHintOnlyNumberedBlockAtForty(t *testing.T) {
	assertReplyRowsFrameEachCodeBlockAtReplyWidthAndHintOnlyNumberedBlock(t, 40)
}

func replyRowsFrameEachCodeBlockAtReplyWidthAndHintOnlyNumberedBlockAtHundred(t *testing.T) {
	assertReplyRowsFrameEachCodeBlockAtReplyWidthAndHintOnlyNumberedBlock(t, 100)
}

func codeFrameRowsWrapLongLineInsideFrame(t *testing.T) {
	block := codeBlock{code: "Get-Process -Name zen | Stop-Process -Force -PassThru -Verbose", language: "powershell"}
	rows, err := CodeFrameRows(block, codeFrame{}, 30, testAppearance)
	require.NoError(t, err)
	rowCount := len(rows)
	require.GreaterOrEqual(t, rowCount, 4)
	codeText := ""
	for _, row := range rows[1 : len(rows)-1] {
		rowWidth := ansi.StringWidth(row)
		require.LessOrEqual(t, rowWidth, 30, row)
		codeText += ansi.Strip(row)
	}
	wrappedCharacters := strings.ReplaceAll(codeText, " ", "")
	codeCharacters := strings.ReplaceAll(block.code, " ", "")
	require.Equal(t, codeCharacters, wrappedCharacters)
}

func codeFrameRulesRepeatConfiguredCodeRuleLine(t *testing.T) {
	settings := exampleSettings(t)
	settings.Lines = config.Lines{Separator: "━", CodeRule: "═"}
	drawn := appearanceFrom(t, settings)
	rows, err := CodeFrameRows(codeBlock{code: "ls", language: "sh"}, codeFrame{}, 20, drawn)
	require.NoError(t, err)
	topRule := ansi.Strip(rows[0])
	drawnBottomRule := ansi.Strip(rows[len(rows)-1])
	for _, rule := range []string{topRule, drawnBottomRule} {
		require.Contains(t, rule, "═══")
		require.NotContains(t, rule, "━")
		require.NotContains(t, rule, "─")
	}
}

func topRuleShowsLanguageAndHintWhenWideEnough(t *testing.T) {
	rule := TopRule("go", codeFrame{copyHint: true}, 34, testAppearance)
	plain := ansi.Strip(rule)
	require.Equal(t, "╭─ go ─────────── ctrl+y copies ─╮", plain)
}

func topRuleDropsHintWhenTooNarrowForIt(t *testing.T) {
	rule := TopRule("go", codeFrame{copyHint: true}, 20, testAppearance)
	plain := ansi.Strip(rule)
	require.Equal(t, "╭─ go ─────────────╮", plain)
}

func topRuleDropsLanguageWhenTooNarrowForIt(t *testing.T) {
	rule := TopRule("go", codeFrame{copyHint: true}, 6, testAppearance)
	plain := ansi.Strip(rule)
	require.Equal(t, "╭────╮", plain)
}

func topRuleShowsMarkedAfterLanguage(t *testing.T) {
	rule := TopRule("go", codeFrame{marked: true}, 17, testAppearance)
	plain := ansi.Strip(rule)
	require.Equal(t, "╭─ go · marked ─╮", plain)
}

func topRuleDropsMarkedWhenTooNarrowForIt(t *testing.T) {
	rule := TopRule("go", codeFrame{marked: true}, 16, testAppearance)
	plain := ansi.Strip(rule)
	require.Equal(t, "╭──────────────╮", plain)
}

func topRuleShowsMarkedAloneWithoutLanguage(t *testing.T) {
	rule := TopRule("", codeFrame{marked: true}, 20, testAppearance)
	plain := ansi.Strip(rule)
	require.Equal(t, "╭─ marked ─────────╮", plain)
}

// the rules of a frame in a reply 60 columns wide, which draws 58
var (
	sixtyColumnShTopRule    = "╭─ sh " + strings.Repeat("─", 51) + "╮"
	sixtyColumnPlainTopRule = "╭" + strings.Repeat("─", 56) + "╮"
	sixtyColumnBottomRule   = "╰" + strings.Repeat("─", 56) + "╯"
)

func trimmedReplyRows(t *testing.T, reply string) []string {
	t.Helper()
	replyRenderer, err := NewReplyRenderer(testAppearance, 60)
	require.NoError(t, err)
	rows, err := ReplyRows(reply, nil, 60, replyRenderer, testAppearance)
	require.NoError(t, err)
	trimmed := []string{}
	for _, row := range rows {
		trimmed = append(trimmed, strings.TrimSpace(ansi.Strip(row)))
	}
	return trimmed
}

func replyRowsKeepTextAroundCodeBlockInListItem(t *testing.T) {
	rows := trimmedReplyRows(t, "1. First step:\n   ```sh\n   echo one\n   ```\n   then this.\n2. Second step")
	want := []string{"", "1. First step:", sixtyColumnShTopRule, "echo one", sixtyColumnBottomRule, "then this.", "2. Second step"}
	require.Equal(t, want, rows)
}

func replyRowsKeepTextAroundCodeBlockInQuote(t *testing.T) {
	rows := trimmedReplyRows(t, "> Run:\n> ```sh\n> echo one\n> ```\n> done")
	want := []string{"", "│ Run:", "│", sixtyColumnShTopRule, "echo one", sixtyColumnBottomRule, "│", "│ done"}
	require.Equal(t, want, rows)
}

func replyRowsKeepProseSpellingPlaceholderAsText(t *testing.T) {
	rows := trimmedReplyRows(t, "The row ZENCODEBLOCK0 stays text.\n\n```\necho one\n```")
	want := []string{"The row ZENCODEBLOCK0 stays text.", "", sixtyColumnPlainTopRule, "echo one", sixtyColumnBottomRule}
	require.Equal(t, want, rows)
}

func replyRowsPutHeadingRightOnTopRuleOfCodeBlockUnderIt(t *testing.T) {
	rows := trimmedReplyRows(t, "Done.\n\n## Do This Now\n\n```\n# 1. Reopen zen.\n```")
	want := []string{"Done.", "", "Do This Now", sixtyColumnPlainTopRule, "# 1. Reopen zen.", sixtyColumnBottomRule}
	require.Equal(t, want, rows)
}

func blockCodes(t *testing.T, reply string) []string {
	t.Helper()
	blocks, err := CodeBlocks(reply)
	require.NoError(t, err)
	codes := []string{}
	for _, block := range blocks {
		codes = append(codes, block.code)
	}
	return codes
}

func codeBlocksKeepTabAndTrailingSpace(t *testing.T) {
	codes := blockCodes(t, "Run:\n\n```go\nfunc f() {\n\treturn \n}\n```\n")
	require.Equal(t, []string{"func f() {\n\treturn \n}"}, codes)
}

func codeBlocksDropListItemIndent(t *testing.T) {
	codes := blockCodes(t, "1. Step:\n   ```sh\n   echo one\n     indented\n   ```\n2. Next")
	require.Equal(t, []string{"echo one\n  indented"}, codes)
}

func codeBlocksDropQuoteMarker(t *testing.T) {
	codes := blockCodes(t, "> ```\n> inside\n> ```")
	require.Equal(t, []string{"inside"}, codes)
}

func codeBlocksTakeTildeFenceLeftOpenToReplyEnd(t *testing.T) {
	codes := blockCodes(t, "~~~\nfirst\n\nlast")
	require.Equal(t, []string{"first\n\nlast"}, codes)
}

func codeBlocksHoldEachBlockInOrder(t *testing.T) {
	codes := blockCodes(t, "```\none\n```\n\n```\ntwo\n```")
	require.Equal(t, []string{"one", "two"}, codes)
}

func codeBlocksSkipEmptyBlock(t *testing.T) {
	codes := blockCodes(t, "```\n```")
	require.Equal(t, []string{}, codes)
}
