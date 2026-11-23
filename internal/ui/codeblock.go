package ui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/charmbracelet/x/ansi"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"

	"github.com/CLBRITTON2/calmpane/claudecode"
)

type codeBlock struct {
	// code is the block's text exactly as in the reply, without the fences and the final line break.
	code     string
	language string
	// start and end are the byte offsets of the start of the opening fence line and the end of the closing one.
	start int
	end   int
	// prefix is what precedes the opening fence on its line: a list item's indentation or a quote's "> ".
	prefix       string
	underHeading bool
}

// The extensions glamour's own parser uses, so a block is found where glamour would render one.
var replyParser = goldmark.New(goldmark.WithExtensions(extension.GFM, extension.DefinitionList)).Parser()

var (
	pickedFrameRuleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorIris))
	markedLabelStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color(colorIris))
)

const codeIndent = "  "

const copyKeyHint = "ctrl+y copies"

const markedLabel = "marked"

// codeFrame is how a code block's frame is drawn: copy mode's picked frame has heavy iris rules, and a marked one
// carries markedLabel after its language.
type codeFrame struct {
	copyHint bool
	picked   bool
	marked   bool
}

// frameRules are the characters and color of a frame's top and bottom rules.
type frameRules struct {
	style       lipgloss.Style
	line        string
	topLeft     string
	topRight    string
	bottomLeft  string
	bottomRight string
}

var pickedFrameRules = frameRules{
	style: pickedFrameRuleStyle, line: "━", topLeft: "┏", topRight: "┓", bottomLeft: "┗", bottomRight: "┛",
}

func (frame codeFrame) rules(appearance appearance) frameRules {
	if frame.picked {
		return pickedFrameRules
	}
	return appearance.lightFrameRules
}

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
		_, underHeading := fenced.PreviousSibling().(*ast.Heading)
		blocks = append(blocks, codeBlock{
			code:         strings.TrimSuffix(code.String(), "\n"),
			language:     string(fenced.Language(source)),
			start:        openingStart,
			end:          end,
			prefix:       openingLine[:strings.IndexAny(openingLine, "`~")],
			underHeading: underHeading,
		})
		return ast.WalkSkipChildren, nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("find the code blocks of reply %.80q: %w", markdown, walkErr)
	}
	return blocks, nil
}

// CodeBlockPositions is where each fenced code block of the entries' replies is, oldest first.
func CodeBlockPositions(entries []claudecode.DialogueEntry) ([]codeBlockPosition, error) {
	positions := []codeBlockPosition{}
	for index, entry := range entries {
		if entry.Kind != claudecode.ReplyEntry {
			continue
		}
		replyBlocks, err := CodeBlocks(entry.Text)
		if err != nil {
			return nil, err
		}
		for block := range replyBlocks {
			positions = append(positions, codeBlockPosition{entry: index, block: block})
		}
	}
	return positions, nil
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

// ReplyRows renders a reply's markdown with glamour and each code block in a frame the reply's width, drawn as frames
// holds it under the block's number, or plain when frames lacks it.
func ReplyRows(reply string, frames map[int]codeFrame, width int, replyRenderer *glamour.TermRenderer,
	appearance appearance) ([]string, error) {
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
		frameRows, err := CodeFrameRows(blocks[index], frames[index], max(width-2, 1), appearance)
		if err != nil {
			return nil, err
		}
		// glamour joins a list item's paragraphs, so the item's text can share the placeholder's row
		before := ansi.Cut(row, 0, ansi.StringWidth(plain[:match[0]]))
		after := ansi.Cut(row, ansi.StringWidth(plain[:match[1]]), ansi.StringWidth(plain))
		if HoldsReplyText(before, appearance) {
			rows = append(rows, before)
		}
		// glamour leaves a blank row under a heading, which would set the heading apart from the block it labels
		if blocks[index].underHeading && len(rows) > 0 && !HoldsReplyText(rows[len(rows)-1], appearance) {
			rows = rows[:len(rows)-1]
		}
		rows = append(rows, frameRows...)
		if HoldsReplyText(after, appearance) {
			rows = append(rows, after)
		}
	}
	return rows, nil
}

// HoldsReplyText reports whether part of a rendered row holds more than spaces and a quote's bar, which glamour draws
// beside the placeholder of a block in a quote.
func HoldsReplyText(rowPart string, appearance appearance) bool {
	quoteBar := strings.TrimSpace(*appearance.replyStyle.BlockQuote.IndentToken)
	return strings.TrimSpace(strings.ReplaceAll(ansi.Strip(rowPart), quoteBar, "")) != ""
}

// CodeFrameRows draws a highlighted code block between a top and a bottom rule width columns wide, with no side
// borders, so a drag over the code selects no frame characters. A code line wider than the frame wraps.
func CodeFrameRows(block codeBlock, frame codeFrame, width int, appearance appearance) ([]string, error) {
	code := strings.ReplaceAll(block.code, "\t", "    ")
	lexer := lexers.Get(block.language)
	if lexer == nil {
		lexer = lexers.Analyse(code)
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}
	tokens, err := chroma.Coalesce(lexer).Tokenise(nil, code)
	if err != nil {
		return nil, fmt.Errorf("tokenise the %q code block %.80q: %w", block.language, block.code, err)
	}
	var highlighted strings.Builder
	if err := formatters.TTY16m.Format(&highlighted, appearance.codeStyle, tokens); err != nil {
		return nil, fmt.Errorf("highlight the %q code block %.80q: %w", block.language, block.code, err)
	}
	rows := []string{TopRule(block.language, frame, width, appearance)}
	codeWidth := max(width-len(codeIndent), 1)
	for _, line := range strings.Split(strings.TrimRight(highlighted.String(), "\n"), "\n") {
		for _, codeRow := range strings.Split(ansi.Hardwrap(line, codeWidth, true), "\n") {
			rows = append(rows, codeIndent+codeRow)
		}
	}
	rules := frame.rules(appearance)
	bottomRule := rules.style.Render(rules.bottomLeft + strings.Repeat(rules.line, max(width-2, 0)) + rules.bottomRight)
	return append(rows, bottomRule), nil
}

// TopRule is a code frame's top rule width columns wide, with the language and the marked label at its left end and
// the copy hint at its right end. A frame too narrow for both drops the hint, then the left label.
func TopRule(language string, frame codeFrame, width int, appearance appearance) string {
	labelWidth := func(label string) int {
		if label == "" {
			return 0
		}
		return ansi.StringWidth(label) + 3
	}
	label, styledLabel := language, appearance.frameLabelStyle.Render(language)
	if frame.marked && language != "" {
		label += " · " + markedLabel
		styledLabel += appearance.frameLabelStyle.Render(" · ") + markedLabelStyle.Render(markedLabel)
	} else if frame.marked {
		label, styledLabel = markedLabel, markedLabelStyle.Render(markedLabel)
	}
	copyHint := ""
	if frame.copyHint {
		copyHint = copyKeyHint
	}
	if width-2-labelWidth(label)-labelWidth(copyHint) < 1 {
		copyHint = ""
	}
	if width-2-labelWidth(label) < 1 {
		label = ""
	}
	rules := frame.rules(appearance)
	rule := rules.style.Render(rules.topLeft)
	if label != "" {
		rule += rules.style.Render(rules.line+" ") + styledLabel + rules.style.Render(" ")
	}
	rule += rules.style.Render(strings.Repeat(rules.line, max(width-2-labelWidth(label)-labelWidth(copyHint), 0)))
	if copyHint != "" {
		rule += rules.style.Render(" ") + appearance.frameLabelStyle.Render(copyHint) + rules.style.Render(" "+rules.line)
	}
	return rule + rules.style.Render(rules.topRight)
}
