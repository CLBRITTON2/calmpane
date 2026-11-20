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
	questionStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGold)).Bold(true)
	optionStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color(colorSubtle))
	workingStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color(colorMuted))
	waitingStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGold))
	errorStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color(colorLove))
	separatorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color(colorHighlightMed))
)

const bandMarker = "▌ "

// replyStyle is glamour's dark style in the colors above, with no background on reply text, no # before headings and
// no blank lines around the document.
var replyStyle = func() ansi.StyleConfig {
	style := styles.DarkStyleConfig
	text, iris, gold, overlay, foam := colorText, colorIris, colorGold, colorOverlay, colorFoam
	noMargin := uint(0)
	style.Document = ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: &text}, Margin: &noMargin}
	style.Heading.Color = &iris
	style.H1, style.H2, style.H3, style.H4, style.H5, style.H6 = ansi.StyleBlock{}, ansi.StyleBlock{}, ansi.StyleBlock{},
		ansi.StyleBlock{}, ansi.StyleBlock{}, ansi.StyleBlock{}
	style.Code.Color = &gold
	style.Code.BackgroundColor = &overlay
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

// EntryRows draws an entry as screen rows, the first row blank to separate it from the entry above.
func EntryRows(entry dialogueEntry, width int, replyRenderer *glamour.TermRenderer) ([]string, error) {
	var block string
	switch entry.kind {
	case promptEntry:
		return append([]string{""}, PromptBandRows(entry.text, width)...), nil
	case replyEntry:
		// glamour passes a tab through, and the screen drops it, so Go code would lose its indentation.
		reply, err := replyRenderer.Render(strings.ReplaceAll(entry.text, "\t", "    "))
		if err != nil {
			return nil, fmt.Errorf("render reply %.80q: %w", entry.text, err)
		}
		block = strings.Trim(reply, "\n")
	case questionEntry:
		lines := []string{questionStyle.Width(max(width-2, 1)).Render(entry.text)}
		for number, label := range entry.options {
			lines = append(lines, optionStyle.Width(max(width-2, 1)).Render(fmt.Sprintf("  %d. %s", number+1, label)))
		}
		block = strings.Join(lines, "\n")
	}
	rows := []string{""}
	for _, row := range strings.Split(block, "\n") {
		rows = append(rows, " "+row)
	}
	return rows, nil
}

// PromptBandRows draws a prompt on a band the full width, with a blank band row above and below the text.
func PromptBandRows(prompt string, width int) []string {
	blankRow := bandStyle.Render(strings.Repeat(" ", width))
	// one column of margin on each side of the marker and the text
	textWidth := max(width-1-lipgloss.Width(bandMarker), 1)
	wrapped := bandTextStyle.Width(textWidth).PaddingRight(1).Render(strings.ReplaceAll(prompt, "\r\n", "\n"))
	rows := []string{blankRow}
	for _, textRow := range strings.Split(wrapped, "\n") {
		rows = append(rows, bandStyle.Render(" ")+bandMarkerStyle.Render(bandMarker)+textRow)
	}
	return append(rows, blankRow)
}
