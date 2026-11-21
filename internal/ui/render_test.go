package ui

import (
	"reflect"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/CLBRITTON2/herdr-zen/internal/claudecode"
)

func TestEntryRows(t *testing.T) {
	t.Run("promptBandRowsFillWidthWhenPromptWraps", promptBandRowsFillWidthWhenPromptWraps)
	t.Run("oneLinePromptIsOneBandRowWithBlankRowBeforeReply", oneLinePromptIsOneBandRowWithBlankRowBeforeReply)
	t.Run("wrappedPromptIsOneBandRowPerTextRow", wrappedPromptIsOneBandRowPerTextRow)
	t.Run("replyRowsDropHeadingAndCodeMarkers", replyRowsDropHeadingAndCodeMarkers)
	t.Run("replyRowsDrawInlineCodeWithoutBackground", replyRowsDrawInlineCodeWithoutBackground)
	t.Run("questionRowsNumberOptionsFromOne", questionRowsNumberOptionsFromOne)
	t.Run("replyRowsIndentTabIndentedCodeWithSpaces", replyRowsIndentTabIndentedCodeWithSpaces)
}

func promptBandRowsFillWidthWhenPromptWraps(t *testing.T) {
	prompt := "make the card, name it with zen, and check that every row of the band reaches the right edge\n" +
		"second line\twith a tab"
	for _, note := range []string{"", queuedNote} {
		for _, width := range []int{40, 100} {
			rows := PromptBandRows(prompt, note, width)
			if len(rows) < 2 {
				t.Fatalf("width %d: %d rows, want a row for each line of the prompt", width, len(rows))
			}
			for index, row := range rows {
				if rowWidth := lipgloss.Width(row); rowWidth != width {
					t.Errorf("note %q, width %d: row %d is %d columns wide: %q", note, width, index, rowWidth, row)
				}
			}
			if firstTextRow := ansi.Strip(rows[0]); note != "" && !strings.HasSuffix(firstTextRow, " "+note+" ") {
				t.Errorf("note %q, width %d: first text row %q does not end in the note", note, width, firstTextRow)
			}
		}
	}
}

func plainRows(rows []string) []string {
	plain := []string{}
	for _, row := range rows {
		plain = append(plain, strings.TrimRight(ansi.Strip(row), " "))
	}
	return plain
}

func oneLinePromptIsOneBandRowWithBlankRowBeforeReply(t *testing.T) {
	replyRenderer, err := NewReplyRenderer(40)
	if err != nil {
		t.Fatalf("NewReplyRenderer: %v", err)
	}
	promptRows, err := EntryRows(claudecode.DialogueEntry{Kind: claudecode.PromptEntry, Text: "ok"}, -1, 40, replyRenderer)
	if err != nil {
		t.Fatalf("EntryRows of the prompt: %v", err)
	}
	replyRows, err := EntryRows(claudecode.DialogueEntry{Kind: claudecode.ReplyEntry, Text: "Done."}, -1, 40, replyRenderer)
	if err != nil {
		t.Fatalf("EntryRows of the reply: %v", err)
	}
	got := plainRows(append(promptRows, replyRows...))
	want := []string{"", " ▌ ok", "", " Done."}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rows = %q, want %q", got, want)
	}
}

func wrappedPromptIsOneBandRowPerTextRow(t *testing.T) {
	got := plainRows(PromptBandRows("rename the widget and then rebuild the whole view\nsecond\nthird", "", 30))
	want := []string{" ▌ rename the widget and then", " ▌ rebuild the whole view", " ▌ second", " ▌ third"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("band rows = %q, want %q", got, want)
	}
}

func replyRowsDropHeadingAndCodeMarkers(t *testing.T) {
	replyRenderer, err := NewReplyRenderer(60)
	if err != nil {
		t.Fatalf("NewReplyRenderer: %v", err)
	}
	rows, err := EntryRows(claudecode.DialogueEntry{Kind: claudecode.ReplyEntry, Text: "## Plan\n\nRun `go test` now."}, -1, 60,
		replyRenderer)
	if err != nil {
		t.Fatalf("EntryRows: %v", err)
	}
	plain := ansi.Strip(strings.Join(rows, "\n"))
	if strings.ContainsAny(plain, "#`") || !strings.Contains(plain, "Plan") || !strings.Contains(plain, "go test") {
		t.Errorf("reply rows = %q, want Plan and go test without # or backticks", plain)
	}
}

func replyRowsDrawInlineCodeWithoutBackground(t *testing.T) {
	replyRenderer, err := NewReplyRenderer(60)
	if err != nil {
		t.Fatalf("NewReplyRenderer: %v", err)
	}
	rows, err := EntryRows(claudecode.DialogueEntry{Kind: claudecode.ReplyEntry, Text: "Run `go test` now."}, -1, 60, replyRenderer)
	if err != nil {
		t.Fatalf("EntryRows: %v", err)
	}
	if joined := strings.Join(rows, "\n"); strings.Contains(joined, "48;") {
		t.Errorf("reply rows = %q, want no background color", joined)
	}
}

func questionRowsNumberOptionsFromOne(t *testing.T) {
	entry := claudecode.DialogueEntry{Kind: claudecode.QuestionEntry, Text: "Which color?", Options: []string{"Red (Recommended)", "Blue"}}
	rows, err := EntryRows(entry, -1, 60, nil)
	if err != nil {
		t.Fatalf("EntryRows: %v", err)
	}
	plain := plainRows(rows)
	want := []string{"", " Which color?", "   1. Red (Recommended)", "   2. Blue"}
	if !reflect.DeepEqual(plain, want) {
		t.Errorf("question rows = %q, want %q", plain, want)
	}
}

func replyRowsIndentTabIndentedCodeWithSpaces(t *testing.T) {
	replyRenderer, err := NewReplyRenderer(60)
	if err != nil {
		t.Fatalf("NewReplyRenderer: %v", err)
	}
	rows, err := EntryRows(claudecode.DialogueEntry{Kind: claudecode.ReplyEntry, Text: "Run:\n\n    func f() {\n    \treturn\n    }"}, -1, 60,
		replyRenderer)
	if err != nil {
		t.Fatalf("EntryRows: %v", err)
	}
	plain := ansi.Strip(strings.Join(rows, "\n"))
	if strings.Contains(plain, "\t") || !strings.Contains(plain, "    return") {
		t.Errorf("reply rows = %q, want return indented by spaces", plain)
	}
}
