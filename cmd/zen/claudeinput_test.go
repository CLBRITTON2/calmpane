package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClaudeInputBox(t *testing.T) {
	t.Run("inputBoxOnScreenGivesTypedDraftAndNoPlaceholder", inputBoxOnScreenGivesTypedDraftAndNoPlaceholder)
}

func inputBoxOnScreenGivesTypedDraftAndNoPlaceholder(t *testing.T) {
	cases := []struct {
		capture string
		want    claudeInputBox
	}{
		{capture: "claude-input-one-row.ansi", want: claudeInputBox{Draft: "move this draft", Rows: 1}},
		{capture: "claude-input-titled-rule.ansi", want: claudeInputBox{Draft: "move this draft", Rows: 1}},
		{
			capture: "claude-input-wrapped.ansi",
			want: claudeInputBox{Draft: "this unsent draft is long enough to wrap across two rows of the Claude input " +
				"box, so pane read drops the space at the wrap", Rows: 2},
		},
		{
			capture: "claude-input-working.ansi",
			want: claudeInputBox{Draft: "while that ping runs I am drafting the next request so it should move into " +
				"the zen input untouched", Rows: 2},
		},
		{capture: "claude-input-placeholder.ansi", want: claudeInputBox{}},
		{capture: "claude-input-empty.ansi", want: claudeInputBox{}},
		{capture: "claude-input-scrolled.ansi", want: claudeInputBox{}},
	}
	for _, testCase := range cases {
		t.Run(testCase.capture, func(t *testing.T) {
			screen, err := os.ReadFile(filepath.Join("testdata", testCase.capture))
			if err != nil {
				t.Fatal(err)
			}
			if got := ClaudeInputBoxOnScreen(string(screen)); got != testCase.want {
				t.Errorf("ClaudeInputBoxOnScreen = %+v, want %+v", got, testCase.want)
			}
		})
	}
}
