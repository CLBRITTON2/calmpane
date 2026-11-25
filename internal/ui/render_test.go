package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"

	"github.com/CLBRITTON2/calmpane/claudecode"
	"github.com/CLBRITTON2/calmpane/internal/config"
)

var testAppearance = func() appearance {
	settings, err := config.Example()
	if err != nil {
		panic(err)
	}
	drawn, err := newAppearance(settings)
	if err != nil {
		panic(err)
	}
	return drawn
}()

func exampleSettings(t *testing.T) config.Config {
	settings, err := config.Example()
	require.NoError(t, err)
	return settings
}

func appearanceFrom(t *testing.T, settings config.Config) appearance {
	drawn, err := newAppearance(settings)
	require.NoError(t, err)
	return drawn
}

func TestEntryRows(t *testing.T) {
	t.Run("promptBandRowsFillWidthWhenPromptWrapsAtForty", promptBandRowsFillWidthWhenPromptWrapsAtForty)
	t.Run("promptBandRowsFillWidthWhenPromptWrapsAtHundred", promptBandRowsFillWidthWhenPromptWrapsAtHundred)
	t.Run("oneLinePromptIsOneBandRowWithBlankRowBeforeReply", oneLinePromptIsOneBandRowWithBlankRowBeforeReply)
	t.Run("wrappedPromptHasMarkerOnFirstBandRowOnly", wrappedPromptHasMarkerOnFirstBandRowOnly)
	t.Run("queuedPromptIsUnqueuedBandThenMutedNoteRowWithoutOverlay", queuedPromptIsUnqueuedBandThenMutedNoteRowWithoutOverlay)
	t.Run("replyRowsDropHeadingAndCodeMarkers", replyRowsDropHeadingAndCodeMarkers)
	t.Run("replyRowsDrawInlineCodeWithoutBackground", replyRowsDrawInlineCodeWithoutBackground)
	t.Run("replyRowsDrawStrongTextBoldInStrongColor", replyRowsDrawStrongTextBoldInStrongColor)
	t.Run("questionRowsNumberOptionsFromOne", questionRowsNumberOptionsFromOne)
	t.Run("replyRowsIndentTabIndentedCodeWithSpaces", replyRowsIndentTabIndentedCodeWithSpaces)
	t.Run("promptRowsShowEachPasteAsItsLabel", promptRowsShowEachPasteAsItsLabel)
	t.Run("compactSummaryIsOneRow", compactSummaryIsOneRow)
}

func promptRowsShowEachPasteAsItsLabel(t *testing.T) {
	entry := claudecode.DialogueEntry{Kind: claudecode.PromptEntry, Text: "l1\nl2\nl3\nl4\n\nwhy did it fail",
		Pastes: []string{"l1\nl2\nl3\nl4"}}
	rows, err := EntryRows(entry, nil, 60, nil, testAppearance)
	require.NoError(t, err)
	plain := plainRows(rows)
	want := []string{"", " > [Pasted text #1 +3 lines]", "", "   why did it fail"}
	require.Equal(t, want, plain)
}

func compactSummaryIsOneRow(t *testing.T) {
	entry := claudecode.DialogueEntry{Kind: claudecode.CompactSummaryEntry}
	rows, err := EntryRows(entry, nil, 60, nil, testAppearance)
	require.NoError(t, err)
	plain := plainRows(rows)
	want := []string{"", " Conversation compacted"}
	require.Equal(t, want, plain)
}

func assertPromptBandRowsFillWidthWhenPromptWraps(t *testing.T, width int) {
	t.Helper()
	prompt := "make the card, name it with zen, and check that every row of the band reaches the right edge\n" +
		"second line\twith a tab"
	rows := PromptBandRows(prompt, width, testAppearance)
	rowCount := len(rows)
	require.GreaterOrEqual(t, rowCount, 2)
	for _, row := range rows {
		rowWidth := lipgloss.Width(row)
		require.Equal(t, width, rowWidth, row)
	}
}

func promptBandRowsFillWidthWhenPromptWrapsAtForty(t *testing.T) {
	assertPromptBandRowsFillWidthWhenPromptWraps(t, 40)
}

func promptBandRowsFillWidthWhenPromptWrapsAtHundred(t *testing.T) {
	assertPromptBandRowsFillWidthWhenPromptWraps(t, 100)
}

func plainRows(rows []string) []string {
	plain := []string{}
	for _, row := range rows {
		plain = append(plain, strings.TrimRight(ansi.Strip(row), " "))
	}
	return plain
}

func oneLinePromptIsOneBandRowWithBlankRowBeforeReply(t *testing.T) {
	replyRenderer, err := NewReplyRenderer(testAppearance, 40)
	require.NoError(t, err)
	prompt := claudecode.DialogueEntry{Kind: claudecode.PromptEntry, Text: "ok"}
	promptRows, err := EntryRows(prompt, nil, 40, replyRenderer, testAppearance)
	require.NoError(t, err)
	reply := claudecode.DialogueEntry{Kind: claudecode.ReplyEntry, Text: "Done."}
	replyRows, err := EntryRows(reply, nil, 40, replyRenderer, testAppearance)
	require.NoError(t, err)
	rows := append(promptRows, replyRows...)
	plain := plainRows(rows)
	want := []string{"", " > ok", "", " Done."}
	require.Equal(t, want, plain)
}

func wrappedPromptHasMarkerOnFirstBandRowOnly(t *testing.T) {
	rows := PromptBandRows("rename the widget and then rebuild the whole view\nsecond\nthird", 30, testAppearance)
	plain := plainRows(rows)
	want := []string{" > rename the widget and then", "   rebuild the whole view", "   second", "   third"}
	require.Equal(t, want, plain)
}

func queuedPromptIsUnqueuedBandThenMutedNoteRowWithoutOverlay(t *testing.T) {
	prompt := "rename the widget and then rebuild the whole view"
	unqueued := PromptEntryRows(prompt, "", 30, testAppearance)
	unqueuedPlain := plainRows(unqueued)
	wantUnqueued := []string{"", " > rename the widget and then", "   rebuild the whole view"}
	require.Equal(t, wantUnqueued, unqueuedPlain)
	queued := PromptEntryRows(prompt, queuedNote, 30, testAppearance)
	queuedBand := queued[:len(queued)-1]
	require.Equal(t, unqueued, queuedBand)
	noteRow := queued[len(queued)-1]
	notePlain := ansi.Strip(noteRow)
	require.Equal(t, "   "+queuedNote, notePlain)
	mutedForeground := "38;2;110;106;134"
	require.Contains(t, noteRow, mutedForeground)
	overlayBackground := "48;2;38;35;58"
	require.NotContains(t, noteRow, overlayBackground)
}

func replyRowsDropHeadingAndCodeMarkers(t *testing.T) {
	replyRenderer, err := NewReplyRenderer(testAppearance, 60)
	require.NoError(t, err)
	reply := claudecode.DialogueEntry{Kind: claudecode.ReplyEntry, Text: "## Plan\n\nRun `go test` now."}
	rows, err := EntryRows(reply, nil, 60, replyRenderer, testAppearance)
	require.NoError(t, err)
	joined := strings.Join(rows, "\n")
	plain := ansi.Strip(joined)
	require.NotContains(t, plain, "#")
	require.NotContains(t, plain, "`")
	require.Contains(t, plain, "Plan")
	require.Contains(t, plain, "go test")
}

func replyRowsDrawInlineCodeWithoutBackground(t *testing.T) {
	replyRenderer, err := NewReplyRenderer(testAppearance, 60)
	require.NoError(t, err)
	reply := claudecode.DialogueEntry{Kind: claudecode.ReplyEntry, Text: "Run `go test` now."}
	rows, err := EntryRows(reply, nil, 60, replyRenderer, testAppearance)
	require.NoError(t, err)
	joined := strings.Join(rows, "\n")
	require.NotContains(t, joined, "48;")
}

func replyRowsDrawStrongTextBoldInStrongColor(t *testing.T) {
	settings := exampleSettings(t)
	settings.Colors.Strong = "#eb6f92"
	drawn := appearanceFrom(t, settings)
	replyRenderer, err := NewReplyRenderer(drawn, 60)
	require.NoError(t, err)
	reply := claudecode.DialogueEntry{Kind: claudecode.ReplyEntry, Text: "Run it **now** please."}
	rows, err := EntryRows(reply, nil, 60, replyRenderer, drawn)
	require.NoError(t, err)
	// love #eb6f92 as a 24-bit foreground, then bold
	boldLove := "\x1b[38;2;235;111;146;1m"
	joined := strings.Join(rows, "\n")
	require.Contains(t, joined, boldLove+"now")
}

func questionRowsNumberOptionsFromOne(t *testing.T) {
	entry := claudecode.DialogueEntry{Kind: claudecode.QuestionEntry, Text: "Which color?", Options: []string{"Red (Recommended)", "Blue"}}
	rows, err := EntryRows(entry, nil, 60, nil, testAppearance)
	require.NoError(t, err)
	plain := plainRows(rows)
	want := []string{"", " Which color?", "   1. Red (Recommended)", "   2. Blue"}
	require.Equal(t, want, plain)
}

func replyRowsIndentTabIndentedCodeWithSpaces(t *testing.T) {
	replyRenderer, err := NewReplyRenderer(testAppearance, 60)
	require.NoError(t, err)
	reply := claudecode.DialogueEntry{Kind: claudecode.ReplyEntry, Text: "Run:\n\n    func f() {\n    \treturn\n    }"}
	rows, err := EntryRows(reply, nil, 60, replyRenderer, testAppearance)
	require.NoError(t, err)
	joined := strings.Join(rows, "\n")
	plain := ansi.Strip(joined)
	require.NotContains(t, plain, "\t")
	require.Contains(t, plain, "    return")
}
