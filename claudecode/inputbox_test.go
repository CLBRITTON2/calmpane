package claudecode

import (
	"testing"

	"github.com/CLBRITTON2/calmpane/internal/herdr/herdrtest"
)

func TestClaudeInputBox(t *testing.T) {
	t.Run("inputBoxOnScreenGivesTypedDraftAndNoPlaceholder", inputBoxOnScreenGivesTypedDraftAndNoPlaceholder)
	t.Run("inputBoxShownOnlyWhileScreenShowsInputBox", inputBoxShownOnlyWhileScreenShowsInputBox)
}

func inputBoxOnScreenGivesTypedDraftAndNoPlaceholder(t *testing.T) {
	cases := []struct {
		capture string
		want    InputBox
	}{
		{capture: "claude-input-one-row.ansi", want: InputBox{Draft: "move this draft", Rows: 1}},
		{capture: "claude-input-titled-rule.ansi", want: InputBox{Draft: "move this draft", Rows: 1}},
		{
			capture: "claude-input-wrapped.ansi",
			want: InputBox{Draft: "this unsent draft is long enough to wrap across two rows of the Claude input " +
				"box, so pane read drops the space at the wrap", Rows: 2},
		},
		{
			capture: "claude-input-working.ansi",
			want: InputBox{Draft: "while that ping runs I am drafting the next request so it should move into " +
				"the zen input untouched", Rows: 2},
		},
		{capture: "claude-input-placeholder.ansi", want: InputBox{}},
		{capture: "claude-input-empty.ansi", want: InputBox{}},
		{capture: "claude-input-scrolled.ansi", want: InputBox{}},
	}
	for _, testCase := range cases {
		t.Run(testCase.capture, func(t *testing.T) {
			if got := InputBoxOnScreen(herdrtest.CapturedScreen(t, testCase.capture)); got != testCase.want {
				t.Errorf("InputBoxOnScreen = %+v, want %+v", got, testCase.want)
			}
		})
	}
}

func inputBoxShownOnlyWhileScreenShowsInputBox(t *testing.T) {
	cases := []struct {
		capture string
		want    bool
	}{
		{capture: "claude-input-one-row.ansi", want: true},
		{capture: "claude-input-empty.ansi", want: true},
		{capture: "claude-input-scrolled.ansi", want: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.capture, func(t *testing.T) {
			if got := InputBoxShown(herdrtest.CapturedScreen(t, testCase.capture)); got != testCase.want {
				t.Errorf("InputBoxShown = %t, want %t", got, testCase.want)
			}
		})
	}
}
