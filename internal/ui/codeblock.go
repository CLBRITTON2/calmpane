package ui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2/quick"
	"github.com/charmbracelet/x/ansi"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"

	"github.com/CLBRITTON2/herdr-zen/claudecode"
)

type codeBlock struct {
	// code is the block's text exactly as in the reply, without the fences and the final line break.
	code     string
	language string
	// start and end are the byte offsets of the start of the opening fence line and the end of the closing one.
	start int
	end   int
	// prefix is what precedes the opening fence on its line: a list item's indentation or a quote's "> ".
	prefix string
}

// The extensions glamour's own parser uses, so a block is found where glamour would render one.
var replyParser = goldmark.New(goldmark.WithExtensions(extension.GFM, extension.DefinitionList)).Parser()

var (
	frameRuleStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color(colorHighlightMed))
	frameLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorMuted))
)

const codeIndent = "  "

const copyKeyHint = "ctrl+y copies"

// codeBlockPosition is where a code block is: the index of its entry, and its number among that reply's blocks.
type codeBlockPosition struct {
	entry int
	block int
}

// CodeBlocks finds the fenced code blocks of a reply that hold at least one line. An empty one stays with glamour.
func CodeBlocks(markdown string) ([]codeBlock, error) {
	source := []byte(markdown)
	blocks := []codeBlock{}
	walkErr := ast.Walk(replyParser.Parse(text.NewReader(source)), func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		fenced, isFenced := node.(*ast.FencedCodeBlock)
		if !entering || !isFenced || fenced.Lines().Len() == 0 {
			return ast.WalkContinue, nil
		}
		lines := fenced.Lines()
		var code strings.Builder
		for index := range lines.Len() {
			segment := lines.At(index)
			code.Write(segment.Value(source))
		}
		openingStart := lineStart(markdown, lineStart(markdown, lines.At(0).Start)-1)
		openingLine := markdown[openingStart:lineEnd(markdown, openingStart)]
		fence := openingLine[strings.IndexAny(openingLine, "`~")]
		end := lines.At(lines.Len() - 1).Stop
		if closingLine := markdown[end:lineEnd(markdown, end)]; strings.HasPrefix(strings.TrimLeft(closingLine, " >"),
			strings.Repeat(string(fence), 3)) {
			end = lineEnd(markdown, end)
		}
		blocks = append(blocks, codeBlock{
			code:     strings.TrimSuffix(code.String(), "\n"),
			language: string(fenced.Language(source)),
			start:    openingStart,
			end:      end,
			prefix:   openingLine[:strings.IndexAny(openingLine, "`~")],
		})
		return ast.WalkSkipChildren, nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("find the code blocks of reply %.80q: %w", markdown, walkErr)
	}
	return blocks, nil
}

// NewestCodeBlockPosition is where the newest fenced code block of the entries' replies is, and nil when they hold
// none.
func NewestCodeBlockPosition(entries []claudecode.DialogueEntry) (*codeBlockPosition, error) {
	for index := len(entries) - 1; index >= 0; index-- {
		if entries[index].Kind != claudecode.ReplyEntry {
			continue
		}
		replyBlocks, err := CodeBlocks(entries[index].Text)
		if err != nil {
			return nil, err
		}
		if len(replyBlocks) > 0 {
			return &codeBlockPosition{entry: index, block: len(replyBlocks) - 1}, nil
		}
	}
	return nil, nil
}

func lineStart(markdown string, offset int) int {
	return strings.LastIndex(markdown[:max(offset, 0)], "\n") + 1
}

// lineEnd is the offset just past the line break of the line starting at offset, or the end of markdown.
func lineEnd(markdown string, offset int) int {
	if newline := strings.Index(markdown[offset:], "\n"); newline >= 0 {
		return offset + newline + 1
	}
	return len(markdown)
}

// ReplyRows renders a reply's markdown with glamour and each code block in a frame the reply's width. The block
// numbered copyHintBlock, -1 for none, carries the copy key's hint in its top rule.
func ReplyRows(reply string, copyHintBlock int, width int, replyRenderer *glamour.TermRenderer) ([]string, error) {
	blocks, err := CodeBlocks(reply)
	if err != nil {
		return nil, err
	}
	// the token stands in for a code block while glamour renders the rest, and must not occur in the reply's own text
	token := "ZENCODEBLOCK"
	for strings.Contains(reply, token) {
		token += "X"
	}
	placeholder := regexp.MustCompile(token + `(\d+)`)
	var markdown strings.Builder
	written := 0
	for index, block := range blocks {
		markdown.WriteString(reply[written:block.start])
		// blank lines around the placeholder keep it a paragraph of its own, in the same list item or quote
		blankLine := strings.TrimRight(block.prefix, " ")
		fmt.Fprintf(&markdown, "%s\n%s%s%d\n%s\n", blankLine, block.prefix, token, index, blankLine)
		written = block.end
	}
	markdown.WriteString(reply[written:])
	// glamour passes a tab through, and the screen drops it, so text would lose its indentation.
	rendered, err := replyRenderer.Render(strings.ReplaceAll(markdown.String(), "\t", "    "))
	if err != nil {
		return nil, fmt.Errorf("render reply %.80q: %w", reply, err)
	}
	rows := []string{}
	for _, row := range strings.Split(strings.Trim(rendered, "\n"), "\n") {
		plain := ansi.Strip(row)
		match := placeholder.FindStringSubmatchIndex(plain)
		if match == nil {
			rows = append(rows, row)
			continue
		}
		index, err := strconv.Atoi(plain[match[2]:match[3]])
		if err != nil {
			return nil, fmt.Errorf("parse the code block number in reply row %q: %w", plain, err)
		}
		if index >= len(blocks) {
			return nil, fmt.Errorf("reply row %q names no code block of the %d in the reply", plain, len(blocks))
		}
		copyHint := ""
		if index == copyHintBlock {
			copyHint = copyKeyHint
		}
		frameRows, err := CodeFrameRows(blocks[index], copyHint, max(width-2, 1))
		if err != nil {
			return nil, err
		}
		// glamour joins a list item's paragraphs, so the item's text can share the placeholder's row
		before := ansi.Cut(row, 0, ansi.StringWidth(plain[:match[0]]))
		after := ansi.Cut(row, ansi.StringWidth(plain[:match[1]]), ansi.StringWidth(plain))
		if HoldsReplyText(before) {
			rows = append(rows, before)
		}
		rows = append(rows, frameRows...)
		if HoldsReplyText(after) {
			rows = append(rows, after)
		}
	}
	return rows, nil
}

// HoldsReplyText reports whether part of a rendered row holds more than spaces and a quote's bar, which glamour draws
// beside the placeholder of a block in a quote.
func HoldsReplyText(rowPart string) bool {
	quoteBar := strings.TrimSpace(*replyStyle.BlockQuote.IndentToken)
	return strings.TrimSpace(strings.ReplaceAll(ansi.Strip(rowPart), quoteBar, "")) != ""
}

// CodeFrameRows draws a highlighted code block between a top and a bottom rule width columns wide, with no side
// borders, so a drag over the code selects no frame characters. A code line wider than the frame wraps.
func CodeFrameRows(block codeBlock, copyHint string, width int) ([]string, error) {
	var highlighted strings.Builder
	if err := quick.Highlight(&highlighted, strings.ReplaceAll(block.code, "\t", "    "), block.language,
		"terminal16m", "rose-pine"); err != nil {
		return nil, fmt.Errorf("highlight the %q code block %.80q: %w", block.language, block.code, err)
	}
	rows := []string{TopRule(block.language, copyHint, width)}
	codeWidth := max(width-len(codeIndent), 1)
	for _, line := range strings.Split(strings.TrimRight(highlighted.String(), "\n"), "\n") {
		for _, codeRow := range strings.Split(ansi.Hardwrap(line, codeWidth, true), "\n") {
			rows = append(rows, codeIndent+codeRow)
		}
	}
	bottomRule := frameRuleStyle.Render("╰" + strings.Repeat("─", max(width-2, 0)) + "╯")
	return append(rows, bottomRule), nil
}

// TopRule is a code frame's top rule width columns wide, with the language at its left end and the copy hint at its
// right end. A frame too narrow for both drops the hint, then the language.
func TopRule(language string, copyHint string, width int) string {
	labelWidth := func(label string) int {
		if label == "" {
			return 0
		}
		return ansi.StringWidth(label) + 3
	}
	if width-2-labelWidth(language)-labelWidth(copyHint) < 1 {
		copyHint = ""
	}
	if width-2-labelWidth(language) < 1 {
		language = ""
	}
	rule := frameRuleStyle.Render("╭")
	if language != "" {
		rule += frameRuleStyle.Render("─ ") + frameLabelStyle.Render(language) + frameRuleStyle.Render(" ")
	}
	rule += frameRuleStyle.Render(strings.Repeat("─", max(width-2-labelWidth(language)-labelWidth(copyHint), 0)))
	if copyHint != "" {
		rule += frameRuleStyle.Render(" ") + frameLabelStyle.Render(copyHint) + frameRuleStyle.Render(" ─")
	}
	return rule + frameRuleStyle.Render("╮")
}
