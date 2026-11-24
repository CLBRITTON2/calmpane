package ui

import (
	"fmt"
	"strings"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/ansi"
	glamourstyles "charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	chromastyles "github.com/alecthomas/chroma/v2/styles"

	"github.com/CLBRITTON2/calmpane/claudecode"
	"github.com/CLBRITTON2/calmpane/internal/config"
)

// colorIris draws copy mode's picked frame and marked label and the slash menu's selected name, which the config does
// not set.
const colorIris = "#c4a7e7"

// appearance is how the view draws, from the config's colors, lines and code sections.
type appearance struct {
	bandStyle       lipgloss.Style
	bandMarkerStyle lipgloss.Style
	bandTextStyle   lipgloss.Style
	promptNoteStyle lipgloss.Style
	questionStyle   lipgloss.Style
	optionStyle     lipgloss.Style
	workingStyle    lipgloss.Style
	waitingStyle    lipgloss.Style
	doneStyle       lipgloss.Style
	errorStyle      lipgloss.Style
	separatorStyle  lipgloss.Style
	separatorLine   string
	frameLabelStyle lipgloss.Style
	lightFrameRules frameRules
	// menuNameStyle draws a command's name in the slash command menu, and menuPickedNameStyle the selected one's.
	menuNameStyle       lipgloss.Style
	menuPickedNameStyle lipgloss.Style
	replyStyle          ansi.StyleConfig
	codeStyle           *chroma.Style
}

func newAppearance(settings config.Config) (appearance, error) {
	colors := settings.Colors
	foreground := func(color string) lipgloss.Style { return lipgloss.NewStyle().Foreground(lipgloss.Color(color)) }
	bandStyle := lipgloss.NewStyle().Background(lipgloss.Color(colors.PromptBand))
	codeStyleBuilder := chromastyles.Get(settings.Code.Theme).Builder()
	if settings.Code.Comment != "" {
		// a theme such as dracula styles CommentSingle on its own, which would win over an entry on Comment alone
		for tokenType := range chroma.StandardTypes {
			if tokenType.InCategory(chroma.Comment) {
				codeStyleBuilder.Add(tokenType, settings.Code.Comment)
			}
		}
	}
	codeStyle, err := codeStyleBuilder.Build()
	if err != nil {
		return appearance{}, fmt.Errorf("build the code style from %q with comments %q: %w", settings.Code.Theme,
			settings.Code.Comment, err)
	}
	// glamour's dark style with no background on reply text, no # before headings and no blank lines around the reply
	replyStyle := glamourstyles.DarkStyleConfig
	noMargin := uint(0)
	replyStyle.Document = ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: &colors.ReplyText}, Margin: &noMargin}
	replyStyle.Heading.Color = &colors.Heading
	replyStyle.H1, replyStyle.H2, replyStyle.H3, replyStyle.H4, replyStyle.H5, replyStyle.H6 = ansi.StyleBlock{},
		ansi.StyleBlock{}, ansi.StyleBlock{}, ansi.StyleBlock{}, ansi.StyleBlock{}, ansi.StyleBlock{}
	// Inline code has no background, so it does not look like a prompt band.
	replyStyle.Code = ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: &colors.InlineCode}}
	replyStyle.Link = ansi.StylePrimitive{Color: &colors.Link, Underline: replyStyle.Link.Underline}
	replyStyle.LinkText = ansi.StylePrimitive{Color: &colors.Link, Bold: replyStyle.LinkText.Bold}
	replyStyle.Strong = ansi.StylePrimitive{Color: &colors.Strong, Bold: replyStyle.Strong.Bold}
	replyStyle.CodeBlock.Chroma = nil
	replyStyle.CodeBlock.Theme = settings.Code.Theme
	return appearance{
		bandStyle:           bandStyle,
		bandMarkerStyle:     bandStyle.Foreground(lipgloss.Color(colors.PromptMarker)),
		bandTextStyle:       bandStyle.Foreground(lipgloss.Color(colors.PromptText)),
		promptNoteStyle:     foreground(colors.PromptNote),
		questionStyle:       foreground(colors.Question).Bold(true),
		optionStyle:         foreground(colors.Option),
		workingStyle:        foreground(colors.Working),
		waitingStyle:        foreground(colors.Waiting),
		doneStyle:           foreground(colors.Done),
		errorStyle:          foreground(colors.Error),
		separatorStyle:      foreground(colors.Separator),
		separatorLine:       settings.Lines.Separator,
		frameLabelStyle:     foreground(colors.CodeLabel),
		menuNameStyle:       foreground(colors.ReplyText),
		menuPickedNameStyle: foreground(colorIris),
		lightFrameRules: frameRules{
			style: foreground(colors.CodeRule), line: settings.Lines.CodeRule,
			topLeft: "╭", topRight: "╮", bottomLeft: "╰", bottomRight: "╯",
		},
		replyStyle: replyStyle,
		codeStyle:  codeStyle,
	}, nil
}

const bandMarker = "> "

const queuedNote = "queued · ctrl+enter to send now"

// compactSummaryRow is what Claude Code shows in place of the summary it writes when it compacts the session.
const compactSummaryRow = "Conversation compacted"

// NewReplyRenderer renders reply markdown one column in from each side of a pane width columns wide.
func NewReplyRenderer(appearance appearance, width int) (*glamour.TermRenderer, error) {
	renderer, err := glamour.NewTermRenderer(glamour.WithStyles(appearance.replyStyle),
		glamour.WithWordWrap(max(width-2, 1)), glamour.WithChromaFormatter("terminal16m"))
	if err != nil {
		return nil, fmt.Errorf("create the markdown renderer at width %d: %w", width, err)
	}
	return renderer, nil
}

// EntryRows draws an entry as screen rows, the first row blank to separate it from the entry above. A reply's code
// frames are drawn as frames holds them, by block number.
func EntryRows(entry claudecode.DialogueEntry, frames map[int]codeFrame, width int, replyRenderer *glamour.TermRenderer,
	appearance appearance) ([]string, error) {
	var blockRows []string
	switch entry.Kind {
	case claudecode.PromptEntry:
		return PromptEntryRows(claudecode.TextWithPastesCollapsed(entry.Text, entry.Pastes), "", width, appearance), nil
	case claudecode.CommandEntry:
		return PromptEntryRows(entry.Text, "", width, appearance), nil
	case claudecode.CommandOutputEntry:
		return CommandOutputRows(entry.Text, width, appearance), nil
	case claudecode.ReplyEntry:
		replyRows, err := ReplyRows(entry.Text, frames, width, replyRenderer, appearance)
		if err != nil {
			return nil, err
		}
		blockRows = replyRows
	case claudecode.QuestionEntry:
		lines := []string{appearance.questionStyle.Width(max(width-2, 1)).Render(entry.Text)}
		for number, label := range entry.Options {
			lines = append(lines, appearance.optionStyle.Width(max(width-2, 1)).Render(fmt.Sprintf("  %d. %s", number+1, label)))
		}
		blockRows = strings.Split(strings.Join(lines, "\n"), "\n")
	case claudecode.CompactSummaryEntry:
		blockRows = []string{appearance.workingStyle.Render(compactSummaryRow)}
	}
	rows := []string{""}
	for _, row := range blockRows {
		rows = append(rows, " "+row)
	}
	return rows, nil
}

// PromptEntryRows draws a prompt's band, then note, unless empty, on its own row under the band's text.
func PromptEntryRows(prompt string, note string, width int, appearance appearance) []string {
	rows := append([]string{""}, PromptBandRows(prompt, width, appearance)...)
	if note != "" {
		rows = append(rows, " "+strings.Repeat(" ", lipgloss.Width(bandMarker))+appearance.promptNoteStyle.Render(note))
	}
	return rows
}

// CommandOutputRows draws a slash command's output under its band, from the column of the band's text, with no blank
// row between them.
func CommandOutputRows(output string, width int, appearance appearance) []string {
	indent := " " + strings.Repeat(" ", lipgloss.Width(bandMarker))
	wrapped := appearance.workingStyle.Width(max(width-1-lipgloss.Width(indent), 1)).Render(output)
	rows := []string{}
	for _, row := range strings.Split(wrapped, "\n") {
		rows = append(rows, indent+row)
	}
	return rows
}

// PromptBandRows draws a prompt on a band the full width, one band row per text row, the marker on the first.
func PromptBandRows(prompt string, width int, appearance appearance) []string {
	// one column of margin on each side of the marker and the text
	textWidth := max(width-1-lipgloss.Width(bandMarker), 1)
	wrapped := appearance.bandTextStyle.Width(textWidth).PaddingRight(1).Render(strings.ReplaceAll(prompt, "\r\n", "\n"))
	rows := []string{}
	for index, textRow := range strings.Split(wrapped, "\n") {
		markerCell := appearance.bandStyle.Render(strings.Repeat(" ", lipgloss.Width(bandMarker)))
		if index == 0 {
			markerCell = appearance.bandMarkerStyle.Render(bandMarker)
		}
		rows = append(rows, appearance.bandStyle.Render(" ")+markerCell+textRow)
	}
	return rows
}
