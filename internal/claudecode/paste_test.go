package claudecode

import (
	"strings"
	"testing"
)

func TestPaste(t *testing.T) {
	t.Run("pasteOverEightHundredCharsOrTwoLineBreaksIsShownAsLabel", pasteOverEightHundredCharsOrTwoLineBreaksIsShownAsLabel)
	t.Run("pasteLabelCountsLineBreaksAsLines", pasteLabelCountsLineBreaksAsLines)
	t.Run("collapsedTextExpandsBackToSameText", collapsedTextExpandsBackToSameText)
}

func pasteOverEightHundredCharsOrTwoLineBreaksIsShownAsLabel(t *testing.T) {
	cases := []struct {
		paste string
		want  bool
	}{
		{paste: strings.Repeat("é", 800), want: false},
		{paste: strings.Repeat("é", 801), want: true},
		{paste: "one\ntwo\nthree", want: false},
		{paste: "one\ntwo\nthree\nfour", want: true},
	}
	for _, testCase := range cases {
		if got := IsShownAsLabel(testCase.paste); got != testCase.want {
			t.Errorf("IsShownAsLabel(%.20q, %d runes) = %v, want %v", testCase.paste,
				len([]rune(testCase.paste)), got, testCase.want)
		}
	}
}

func pasteLabelCountsLineBreaksAsLines(t *testing.T) {
	if got := PasteLabel(2, "a\nb\nc\nd"); got != "[Pasted text #2 +3 lines]" {
		t.Errorf("label of four lines = %q, want %q", got, "[Pasted text #2 +3 lines]")
	}
	if got := PasteLabel(1, strings.Repeat("x", 900)); got != "[Pasted text #1]" {
		t.Errorf("label of one long line = %q, want %q", got, "[Pasted text #1]")
	}
}

func collapsedTextExpandsBackToSameText(t *testing.T) {
	pastes := []string{"a\nb\nc\nd", "w\nx\ny\nz"}
	text := "look at a\nb\nc\nd and then w\nx\ny\nz please"
	collapsed := TextWithPastesCollapsed(text, pastes)
	if want := "look at [Pasted text #1 +3 lines] and then [Pasted text #2 +3 lines] please"; collapsed != want {
		t.Errorf("collapsed = %q, want %q", collapsed, want)
	}
	if expanded := TextWithPastesExpanded(collapsed, pastes); expanded != text {
		t.Errorf("expanded = %q, want %q", expanded, text)
	}
}
