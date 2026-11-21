package main

import (
	"fmt"
	"strings"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"
)

// Rosé Pine main, https://rosepinetheme.com/palette, as perusal uses it. The base background is the terminal's.
const (
	colorOverlay      = "#26233a"
	colorHighlightMed = "#403d52"
	colorMuted        = "#6e6a86"
	colorSubtle       = "#908caa"
	colorText         = "#e0def4"
	colorLove         = "#eb6f92"
	colorGold         = "#f6c177"
	colorFoam         = "#9ccfd8"
	colorIris         = "#c4a7e7"
)

var (
	bandStyle       = lipgloss.NewStyle().Background(lipgloss.Color(colorOverlay))
	bandMarkerStyle = bandStyle.Foreground(lipgloss.Color(colorIris))
	bandTextStyle   = bandStyle.Foreground(lipgloss.Color(colorText))
	bandNoteStyle   = bandStyle.Foreground(lipgloss.Color(colorMuted))
	questionStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGold)).Bold(true)
	optionStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color(colorSubtle))
	workingStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color(colorMuted))
	waitingStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGold))
	errorStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color(colorLove))
	separatorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color(colorHighlightMed))
)

const bandMarker = "▌ "

const queuedNote = "queued · ctrl+enter to send now"

// replyStyle is glamour's dark style in the colors above, with no background on reply text, no # before headings and
// no blank lines around the document.
var replyStyle = func() ansi.StyleConfig {
	style := styles.DarkStyleConfig
	text, iris, gold, foam := colorText, colorIris, colorGold, colorFoam
	noMargin := uint(0)
	style.Document = ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: &text}, Margin: &noMargin}
	style.Heading.Color = &iris
	style.H1, style.H2, style.H3, style.H4, style.H5, style.H6 = ansi.StyleBlock{}, ansi.StyleBlock{}, ansi.StyleBlock{},
		ansi.StyleBlock{}, ansi.StyleBlock{}, ansi.StyleBlock{}
	// Inline code is gold with no background, so it does not look like a prompt band.
	style.Code = ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: &gold}}
	style.Link = ansi.StylePrimitive{Color: &foam, Underline: style.Link.Underline}
	style.LinkText = ansi.StylePrimitive{Color: &foam, Bold: style.LinkText.Bold}
	style.CodeBlock.Chroma = nil
	style.CodeBlock.Theme = "rose-pine"
	return style
}()

// NewReplyRenderer renders reply markdown one column in from each side of a pane width columns wide.
func NewReplyRenderer(width int) (*glamour.TermRenderer, error) {
	renderer, err := glamour.NewTermRenderer(glamour.WithStyles(replyStyle), glamour.WithWordWrap(max(width-2, 1)),
		glamour.WithChromaFormatter("terminal16m"))
	if err != nil {
		return nil, fmt.Errorf("create the markdown renderer at width %d: %w", width, err)
	}
	return renderer, nil
}

// EntryRows draws an entry as screen rows, the first row blank to separate it from the entry above. In a reply, the
// code block numbered copyHintBlock, -1 for none, carries the copy key's hint.
func EntryRows(entry dialogueEntry, copyHintBlock int, width int, replyRenderer *glamour.TermRenderer) ([]string,
	error) {
	var blockRows []string
	switch entry.kind {
	case promptEntry:
		return PromptEntryRows(entry.text, "", width), nil
	case replyEntry:
		replyRows, err := ReplyRows(entry.text, copyHintBlock, width, replyRenderer)
		if err != nil {
			return nil, err
		}
		blockRows = replyRows
	case questionEntry:
		lines := []string{questionStyle.Width(max(width-2, 1)).Render(entry.text)}
		for number, label := range entry.options {
			lines = append(lines, optionStyle.Width(max(width-2, 1)).Render(fmt.Sprintf("  %d. %s", number+1, label)))
		}
		blockRows = strings.Split(strings.Join(lines, "\n"), "\n")
	}
	rows := []string{""}
	for _, row := range blockRows {
		rows = append(rows, " "+row)
	}
	return rows, nil
}

func PromptEntryRows(prompt string, note string, width int) []string {
	return append([]string{""}, PromptBandRows(prompt, note, width)...)
}

// PromptBandRows draws a prompt on a band the full width, one band row per text row.
func PromptBandRows(prompt string, note string, width int) []string {
	noteWidth := 0
	if note != "" {
		// one column between the text and the note
		noteWidth = lipgloss.Width(note) + 1
	}
	// one column of margin on each side of the marker and the text
	textWidth := max(width-1-lipgloss.Width(bandMarker)-noteWidth, 1)
	wrapped := bandTextStyle.Width(textWidth).PaddingRight(1).Render(strings.ReplaceAll(prompt, "\r\n", "\n"))
	rows := []string{}
	for index, textRow := range strings.Split(wrapped, "\n") {
		noteCell := bandStyle.Render(strings.Repeat(" ", noteWidth))
		if index == 0 && note != "" {
			noteCell = bandNoteStyle.Render(note) + bandStyle.Render(" ")
		}
		rows = append(rows, bandStyle.Render(" ")+bandMarkerStyle.Render(bandMarker)+textRow+noteCell)
	}
	return rows
}
