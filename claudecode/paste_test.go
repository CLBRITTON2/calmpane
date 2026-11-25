package claudecode

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPaste(t *testing.T) {
	t.Run("pasteOfEightHundredCharsIsNotShownAsLabel", pasteOfEightHundredCharsIsNotShownAsLabel)
	t.Run("pasteOverEightHundredCharsIsShownAsLabel", pasteOverEightHundredCharsIsShownAsLabel)
	t.Run("pasteOfTwoLineBreaksIsNotShownAsLabel", pasteOfTwoLineBreaksIsNotShownAsLabel)
	t.Run("pasteOverTwoLineBreaksIsShownAsLabel", pasteOverTwoLineBreaksIsShownAsLabel)
	t.Run("pasteLabelCountsLineBreaksAsLines", pasteLabelCountsLineBreaksAsLines)
	t.Run("pasteLabelOfOneLongLineHasNoLineCount", pasteLabelOfOneLongLineHasNoLineCount)
	t.Run("collapsedTextExpandsBackToSameText", collapsedTextExpandsBackToSameText)
	t.Run("textPartsFollowLabelsInTextOrderAndLeaveOutPasteWithoutLabel",
		textPartsFollowLabelsInTextOrderAndLeaveOutPasteWithoutLabel)
	t.Run("textHoldsPasteLabelWithinTypedText", textHoldsPasteLabelWithinTypedText)
	t.Run("textHoldsPasteLabelWithLineCount", textHoldsPasteLabelWithLineCount)
	t.Run("textHoldsNoPasteLabelForLabelStartAlone", textHoldsNoPasteLabelForLabelStartAlone)
	t.Run("textHoldsNoPasteLabelForLabelsWithoutNumber", textHoldsNoPasteLabelForLabelsWithoutNumber)
	t.Run("emptyPasteKeepsItsNumberAndHasNoLabel", emptyPasteKeepsItsNumberAndHasNoLabel)
}

func textHoldsPasteLabelWithinTypedText(t *testing.T) {
	longLine := strings.Repeat("x", 900)
	label := PasteLabel(1, longLine)
	text := "see " + label + " first"
	holds := HoldsPasteLabel(text)
	require.True(t, holds)
}

func textHoldsPasteLabelWithLineCount(t *testing.T) {
	text := PasteLabel(12, "a\nb\nc\nd")
	holds := HoldsPasteLabel(text)
	require.True(t, holds)
}

func textHoldsNoPasteLabelForLabelStartAlone(t *testing.T) {
	holds := HoldsPasteLabel("[Pasted text")
	require.False(t, holds)
}

func textHoldsNoPasteLabelForLabelsWithoutNumber(t *testing.T) {
	holds := HoldsPasteLabel("[Pasted text #] and [Pasted text #2 lines]")
	require.False(t, holds)
}

func pasteOfEightHundredCharsIsNotShownAsLabel(t *testing.T) {
	paste := strings.Repeat("é", 800)
	shownAsLabel := IsShownAsLabel(paste)
	require.False(t, shownAsLabel)
}

func pasteOverEightHundredCharsIsShownAsLabel(t *testing.T) {
	paste := strings.Repeat("é", 801)
	shownAsLabel := IsShownAsLabel(paste)
	require.True(t, shownAsLabel)
}

func pasteOfTwoLineBreaksIsNotShownAsLabel(t *testing.T) {
	shownAsLabel := IsShownAsLabel("one\ntwo\nthree")
	require.False(t, shownAsLabel)
}

func pasteOverTwoLineBreaksIsShownAsLabel(t *testing.T) {
	shownAsLabel := IsShownAsLabel("one\ntwo\nthree\nfour")
	require.True(t, shownAsLabel)
}

func pasteLabelCountsLineBreaksAsLines(t *testing.T) {
	label := PasteLabel(2, "a\nb\nc\nd")
	require.Equal(t, "[Pasted text #2 +3 lines]", label)
}

func pasteLabelOfOneLongLineHasNoLineCount(t *testing.T) {
	longLine := strings.Repeat("x", 900)
	label := PasteLabel(1, longLine)
	require.Equal(t, "[Pasted text #1]", label)
}

func collapsedTextExpandsBackToSameText(t *testing.T) {
	pastes := []string{"a\nb\nc\nd", "w\nx\ny\nz"}
	text := "look at a\nb\nc\nd and then w\nx\ny\nz please"

	collapsed := TextWithPastesCollapsed(text, pastes)
	require.Equal(t, "look at [Pasted text #1 +3 lines] and then [Pasted text #2 +3 lines] please", collapsed)

	expanded := TextWithPastesExpanded(collapsed, pastes)
	require.Equal(t, text, expanded)
}

func textPartsFollowLabelsInTextOrderAndLeaveOutPasteWithoutLabel(t *testing.T) {
	pastes := []string{"a\nb\nc\nd", "w\nx\ny\nz", "deleted\nlabel\nof\nthis"}
	text := "[Pasted text #2 +3 lines] before [Pasted text #1 +3 lines]"

	parts := TextParts(text, pastes)
	want := []TextPart{{Text: "w\nx\ny\nz", IsPaste: true, Number: 2}, {Text: " before "},
		{Text: "a\nb\nc\nd", IsPaste: true, Number: 1}}
	require.Equal(t, want, parts)
}

func emptyPasteKeepsItsNumberAndHasNoLabel(t *testing.T) {
	pastes := []string{"", "w\nx\ny\nz"}
	text := "typed [Pasted text #1] and w\nx\ny\nz"

	collapsed := TextWithPastesCollapsed(text, pastes)
	require.Equal(t, "typed [Pasted text #1] and [Pasted text #2 +3 lines]", collapsed)

	parts := TextParts(collapsed, pastes)
	want := []TextPart{{Text: "typed [Pasted text #1] and "}, {Text: "w\nx\ny\nz", IsPaste: true, Number: 2}}
	require.Equal(t, want, parts)
}
