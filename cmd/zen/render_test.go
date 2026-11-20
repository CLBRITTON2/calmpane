package main

import (
	"reflect"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestPromptBandRowsFillTheWidthWhenThePromptWraps(t *testing.T) {
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

func TestAOneLinePromptIsOneBandRowWithABlankRowBeforeTheReply(t *testing.T) {
	replyRenderer, err := NewReplyRenderer(40)
	if err != nil {
		t.Fatalf("NewReplyRenderer: %v", err)
	}
	promptRows, err := EntryRows(dialogueEntry{kind: promptEntry, text: "ok"}, -1, 40, replyRenderer)
	if err != nil {
		t.Fatalf("EntryRows of the prompt: %v", err)
	}
	replyRows, err := EntryRows(dialogueEntry{kind: replyEntry, text: "Done."}, -1, 40, replyRenderer)
	if err != nil {
		t.Fatalf("EntryRows of the reply: %v", err)
	}
	got := plainRows(append(promptRows, replyRows...))
	want := []string{"", " ▌ ok", "", " Done."}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rows = %q, want %q", got, want)
	}
}

func TestAWrappedPromptIsOneBandRowPerTextRow(t *testing.T) {
	got := plainRows(PromptBandRows("rename the widget and then rebuild the whole view\nsecond\nthird", "", 30))
	want := []string{" ▌ rename the widget and then", " ▌ rebuild the whole view", " ▌ second", " ▌ third"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("band rows = %q, want %q", got, want)
	}
}

func TestEntryRowsDropsHeadingAndCodeMarkersFromReply(t *testing.T) {
	replyRenderer, err := NewReplyRenderer(60)
	if err != nil {
		t.Fatalf("NewReplyRenderer: %v", err)
	}
	rows, err := EntryRows(dialogueEntry{kind: replyEntry, text: "## Plan\n\nRun `go test` now."}, -1, 60,
		replyRenderer)
	if err != nil {
		t.Fatalf("EntryRows: %v", err)
	}
	plain := ansi.Strip(strings.Join(rows, "\n"))
	if strings.ContainsAny(plain, "#`") || !strings.Contains(plain, "Plan") || !strings.Contains(plain, "go test") {
		t.Errorf("reply rows = %q, want Plan and go test without # or backticks", plain)
	}
}

func TestInlineCodeInAReplyHasNoBackground(t *testing.T) {
	replyRenderer, err := NewReplyRenderer(60)
	if err != nil {
		t.Fatalf("NewReplyRenderer: %v", err)
	}
	rows, err := EntryRows(dialogueEntry{kind: replyEntry, text: "Run `go test` now."}, -1, 60, replyRenderer)
	if err != nil {
		t.Fatalf("EntryRows: %v", err)
	}
	if joined := strings.Join(rows, "\n"); strings.Contains(joined, "48;") {
		t.Errorf("reply rows = %q, want no background color", joined)
	}
}

func TestEntryRowsNumbersQuestionOptionsFromOne(t *testing.T) {
	entry := dialogueEntry{kind: questionEntry, text: "Which color?", options: []string{"Red (Recommended)", "Blue"}}
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

func TestEntryRowsIndentsTabIndentedCodeWithSpaces(t *testing.T) {
	replyRenderer, err := NewReplyRenderer(60)
	if err != nil {
		t.Fatalf("NewReplyRenderer: %v", err)
	}
	rows, err := EntryRows(dialogueEntry{kind: replyEntry, text: "Run:\n\n    func f() {\n    \treturn\n    }"}, -1, 60,
		replyRenderer)
	if err != nil {
		t.Fatalf("EntryRows: %v", err)
	}
	plain := ansi.Strip(strings.Join(rows, "\n"))
	if strings.Contains(plain, "\t") || !strings.Contains(plain, "    return") {
		t.Errorf("reply rows = %q, want return indented by spaces", plain)
	}
}
