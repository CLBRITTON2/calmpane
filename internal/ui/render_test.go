package ui

import (
	"reflect"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

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
	if err != nil {
		t.Fatalf("Example: %v", err)
	}
	return settings
}

func appearanceFrom(t *testing.T, settings config.Config) appearance {
	drawn, err := newAppearance(settings)
	if err != nil {
		t.Fatalf("newAppearance: %v", err)
	}
	return drawn
}

func TestEntryRows(t *testing.T) {
	t.Run("promptBandRowsFillWidthWhenPromptWraps", promptBandRowsFillWidthWhenPromptWraps)
	t.Run("oneLinePromptIsOneBandRowWithBlankRowBeforeReply", oneLinePromptIsOneBandRowWithBlankRowBeforeReply)
	t.Run("wrappedPromptHasMarkerOnFirstBandRowOnly", wrappedPromptHasMarkerOnFirstBandRowOnly)
	t.Run("queuedPromptIsUnqueuedBandThenMutedNoteRowWithoutOverlay", queuedPromptIsUnqueuedBandThenMutedNoteRowWithoutOverlay)
	t.Run("replyRowsDropHeadingAndCodeMarkers", replyRowsDropHeadingAndCodeMarkers)
	t.Run("replyRowsDrawInlineCodeWithoutBackground", replyRowsDrawInlineCodeWithoutBackground)
	t.Run("questionRowsNumberOptionsFromOne", questionRowsNumberOptionsFromOne)
	t.Run("replyRowsIndentTabIndentedCodeWithSpaces", replyRowsIndentTabIndentedCodeWithSpaces)
	t.Run("promptRowsShowEachPasteAsItsLabel", promptRowsShowEachPasteAsItsLabel)
	t.Run("compactSummaryIsOneRow", compactSummaryIsOneRow)
}

func promptRowsShowEachPasteAsItsLabel(t *testing.T) {
	entry := claudecode.DialogueEntry{Kind: claudecode.PromptEntry, Text: "l1\nl2\nl3\nl4\n\nwhy did it fail",
		Pastes: []string{"l1\nl2\nl3\nl4"}}
	rows, err := EntryRows(entry, nil, 60, nil, testAppearance)
	if err != nil {
		t.Fatalf("EntryRows: %v", err)
	}
	want := []string{"", " > [Pasted text #1 +3 lines]", "", "   why did it fail"}
	if got := plainRows(rows); !reflect.DeepEqual(got, want) {
		t.Errorf("prompt rows = %q, want %q", got, want)
	}
}

func compactSummaryIsOneRow(t *testing.T) {
	rows, err := EntryRows(claudecode.DialogueEntry{Kind: claudecode.CompactSummaryEntry}, nil, 60, nil, testAppearance)
	if err != nil {
		t.Fatalf("EntryRows: %v", err)
	}
	want := []string{"", " Conversation compacted"}
	if got := plainRows(rows); !reflect.DeepEqual(got, want) {
		t.Errorf("compact summary rows = %q, want %q", got, want)
	}
}

func promptBandRowsFillWidthWhenPromptWraps(t *testing.T) {
	prompt := "make the card, name it with zen, and check that every row of the band reaches the right edge\n" +
		"second line\twith a tab"
	for _, width := range []int{40, 100} {
		rows := PromptBandRows(prompt, width, testAppearance)
		if len(rows) < 2 {
			t.Fatalf("width %d: %d rows, want a row for each line of the prompt", width, len(rows))
		}
		for index, row := range rows {
			if rowWidth := lipgloss.Width(row); rowWidth != width {
				t.Errorf("width %d: row %d is %d columns wide: %q", width, index, rowWidth, row)
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
	replyRenderer, err := NewReplyRenderer(testAppearance, 40)
	if err != nil {
		t.Fatalf("NewReplyRenderer: %v", err)
	}
	promptRows, err := EntryRows(claudecode.DialogueEntry{Kind: claudecode.PromptEntry, Text: "ok"},
		nil, 40, replyRenderer, testAppearance)
	if err != nil {
		t.Fatalf("EntryRows of the prompt: %v", err)
	}
	replyRows, err := EntryRows(claudecode.DialogueEntry{Kind: claudecode.ReplyEntry, Text: "Done."},
		nil, 40, replyRenderer, testAppearance)
	if err != nil {
		t.Fatalf("EntryRows of the reply: %v", err)
	}
	got := plainRows(append(promptRows, replyRows...))
	want := []string{"", " > ok", "", " Done."}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rows = %q, want %q", got, want)
	}
}

func wrappedPromptHasMarkerOnFirstBandRowOnly(t *testing.T) {
	got := plainRows(PromptBandRows("rename the widget and then rebuild the whole view\nsecond\nthird", 30, testAppearance))
	want := []string{" > rename the widget and then", "   rebuild the whole view", "   second", "   third"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("band rows = %q, want %q", got, want)
	}
}

func queuedPromptIsUnqueuedBandThenMutedNoteRowWithoutOverlay(t *testing.T) {
	prompt := "rename the widget and then rebuild the whole view"
	unqueued := PromptEntryRows(prompt, "", 30, testAppearance)
	wantUnqueued := []string{"", " > rename the widget and then", "   rebuild the whole view"}
	if got := plainRows(unqueued); !reflect.DeepEqual(got, wantUnqueued) {
		t.Errorf("unqueued rows = %q, want %q with no note row", got, wantUnqueued)
	}
	queued := PromptEntryRows(prompt, queuedNote, 30, testAppearance)
	if !reflect.DeepEqual(queued[:len(queued)-1], unqueued) {
		t.Errorf("queued band rows = %q, want the unqueued rows %q", queued[:len(queued)-1], unqueued)
	}
	noteRow := queued[len(queued)-1]
	mutedForeground := "38;2;110;106;134"
	overlayBackground := "48;2;38;35;58"
	if ansi.Strip(noteRow) != "   "+queuedNote || !strings.Contains(noteRow, mutedForeground) ||
		strings.Contains(noteRow, overlayBackground) {
		t.Errorf("note row = %q, want %q in muted with no overlay", noteRow, "   "+queuedNote)
	}
}

func replyRowsDropHeadingAndCodeMarkers(t *testing.T) {
	replyRenderer, err := NewReplyRenderer(testAppearance, 60)
	if err != nil {
		t.Fatalf("NewReplyRenderer: %v", err)
	}
	rows, err := EntryRows(claudecode.DialogueEntry{Kind: claudecode.ReplyEntry, Text: "## Plan\n\nRun `go test` now."}, nil, 60,
		replyRenderer, testAppearance)
	if err != nil {
		t.Fatalf("EntryRows: %v", err)
	}
	plain := ansi.Strip(strings.Join(rows, "\n"))
	if strings.ContainsAny(plain, "#`") || !strings.Contains(plain, "Plan") || !strings.Contains(plain, "go test") {
		t.Errorf("reply rows = %q, want Plan and go test without # or backticks", plain)
	}
}

func replyRowsDrawInlineCodeWithoutBackground(t *testing.T) {
	replyRenderer, err := NewReplyRenderer(testAppearance, 60)
	if err != nil {
		t.Fatalf("NewReplyRenderer: %v", err)
	}
	rows, err := EntryRows(claudecode.DialogueEntry{Kind: claudecode.ReplyEntry, Text: "Run `go test` now."},
		nil, 60, replyRenderer, testAppearance)
	if err != nil {
		t.Fatalf("EntryRows: %v", err)
	}
	if joined := strings.Join(rows, "\n"); strings.Contains(joined, "48;") {
		t.Errorf("reply rows = %q, want no background color", joined)
	}
}

func questionRowsNumberOptionsFromOne(t *testing.T) {
	entry := claudecode.DialogueEntry{Kind: claudecode.QuestionEntry, Text: "Which color?", Options: []string{"Red (Recommended)", "Blue"}}
	rows, err := EntryRows(entry, nil, 60, nil, testAppearance)
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
	replyRenderer, err := NewReplyRenderer(testAppearance, 60)
	if err != nil {
		t.Fatalf("NewReplyRenderer: %v", err)
	}
	rows, err := EntryRows(claudecode.DialogueEntry{Kind: claudecode.ReplyEntry, Text: "Run:\n\n    func f() {\n    \treturn\n    }"}, nil, 60,
		replyRenderer, testAppearance)
	if err != nil {
		t.Fatalf("EntryRows: %v", err)
	}
	plain := ansi.Strip(strings.Join(rows, "\n"))
	if strings.Contains(plain, "\t") || !strings.Contains(plain, "    return") {
		t.Errorf("reply rows = %q, want return indented by spaces", plain)
	}
}
