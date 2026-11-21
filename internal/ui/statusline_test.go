package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/CLBRITTON2/herdr-zen/internal/claudecode"
)

const roseLocation = "\x1b[38;2;235;188;186mherdr-zen/zen-status-line\x1b[0m"

func modelWithClaudeStatusLine(line string) zenModel {
	model := zenModel{entries: []claudecode.DialogueEntry{}, entryRows: [][]string{}, input: NewPromptInput()}
	model = model.withSize(40, 12).withEntries(numberedPrompts(1, 20), nil)
	updated, _ := model.Update(claudeStatusLineMsg{line: line})
	return updated.(zenModel)
}

func TestRowsAroundInput(t *testing.T) {
	t.Run("claudeStatusLineDrawnUnderSeparatorWithItsColorsAndCutAtWidth", claudeStatusLineDrawnUnderSeparatorWithItsColorsAndCutAtWidth)
	t.Run("blankRowSeparatesNewestEntryFromWorkingRow", blankRowSeparatesNewestEntryFromWorkingRow)
	t.Run("newestEntrySitsRightAboveEmptyStatusRowWithNoStatus", newestEntrySitsRightAboveEmptyStatusRowWithNoStatus)
	t.Run("inputIsLastRowWithNoClaudeStatusLine", inputIsLastRowWithNoClaudeStatusLine)
}

func claudeStatusLineDrawnUnderSeparatorWithItsColorsAndCutAtWidth(t *testing.T) {
	line := roseLocation + " \x1b[38;2;156;207;216m" + strings.Repeat("Opus 5.5 ", 8) + "\x1b[0m"
	model := modelWithClaudeStatusLine(line)
	rows := strings.Split(model.screen(), "\n")
	if len(rows) != 12 {
		t.Fatalf("screen has %d rows, want the pane's 12:\n%s", len(rows), model.screen())
	}
	if input := ansi.Strip(rows[9]); !strings.HasPrefix(input, " > ") {
		t.Errorf("row 10 = %q, want the input", input)
	}
	if separator := ansi.Strip(rows[10]); separator != strings.Repeat("─", 40) {
		t.Errorf("row 11 = %q, want a separator the full width", separator)
	}
	statusRow := rows[11]
	if !strings.HasPrefix(statusRow, " "+roseLocation+" \x1b[38;2;156;207;216mOpus") {
		t.Errorf("status row = %q, want the line's own colors after a one-column margin", statusRow)
	}
	if width := ansi.StringWidth(statusRow); width != 40 {
		t.Errorf("status row is %d columns wide, want it cut at the pane's 40", width)
	}
	if !strings.HasSuffix(statusRow, ansi.ResetStyle) {
		t.Errorf("status row = %q, want its colors reset at its end", statusRow)
	}
}

func blankRowSeparatesNewestEntryFromWorkingRow(t *testing.T) {
	model := modelWithClaudeStatusLine(roseLocation)
	model.agentStatus = AgentWorking
	rows := strings.Split(model.screen(), "\n")
	if newest := ansi.Strip(rows[5]); !strings.Contains(newest, "prompt 20") {
		t.Errorf("row 6 = %q, want the newest prompt", newest)
	}
	if blank := rows[6]; blank != "" {
		t.Errorf("row 7 = %q, want a blank row", blank)
	}
	if working := ansi.Strip(rows[7]); working != " Working..." {
		t.Errorf("row 8 = %q, want the working row", working)
	}
}

func newestEntrySitsRightAboveEmptyStatusRowWithNoStatus(t *testing.T) {
	model := modelWithClaudeStatusLine(roseLocation)
	rows := strings.Split(model.screen(), "\n")
	if newest := ansi.Strip(rows[6]); !strings.Contains(newest, "prompt 20") {
		t.Errorf("row 7 = %q, want the newest prompt", newest)
	}
	if statusRow := rows[7]; statusRow != " " {
		t.Errorf("row 8 = %q, want the empty status row", statusRow)
	}
}

func inputIsLastRowWithNoClaudeStatusLine(t *testing.T) {
	neverShown := modelWithClaudeStatusLine("")
	rows := strings.Split(ansi.Strip(neverShown.screen()), "\n")
	if len(rows) != 12 {
		t.Errorf("screen has %d rows, want the pane's 12", len(rows))
	}
	if last := rows[len(rows)-1]; !strings.HasPrefix(last, " > ") {
		t.Errorf("last row = %q, want the input", last)
	}
	shown := modelWithClaudeStatusLine(roseLocation)
	gone, _ := shown.Update(claudeStatusLineMsg{line: ""})
	if screen, want := gone.(zenModel).screen(), neverShown.screen(); screen != want {
		t.Errorf("screen after the status line went:\n%s\nwant the entries back in its rows:\n%s", screen, want)
	}
}

func nextClaudeStatusLine(t *testing.T, messages <-chan tea.Msg) string {
	t.Helper()
	select {
	case msg := <-messages:
		statusLine, isStatusLine := msg.(claudeStatusLineMsg)
		if !isStatusLine {
			t.Fatalf("got %#v, want a claudeStatusLineMsg", msg)
		}
		return statusLine.line
	case <-time.After(5 * time.Second):
		t.Fatalf("no status line message after 5 s")
		return ""
	}
}

func writeStatusLineFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestFollowClaudeStatusLine(t *testing.T) {
	t.Run("followClaudeStatusLineSendsEachWholeLineAndNoneWhenFileGoes", followClaudeStatusLineSendsEachWholeLineAndNoneWhenFileGoes)
	t.Run("followClaudeStatusLineReportsUnreadableFileAsProblemNamingIt", followClaudeStatusLineReportsUnreadableFileAsProblemNamingIt)
	t.Run("followClaudeStatusLineFollowsNewestPathSent", followClaudeStatusLineFollowsNewestPathSent)
}

func followClaudeStatusLineFollowsNewestPathSent(t *testing.T) {
	directory := t.TempDir()
	viewerPath := filepath.Join(directory, "viewer-session-id")
	jobPath := filepath.Join(directory, "job-session-id")
	writeStatusLineFile(t, viewerPath, "viewer\n")
	writeStatusLineFile(t, jobPath, "job\n")
	messages := make(chan tea.Msg, 16)
	// buffered, so a follower that stopped taking paths fails the test instead of hanging it
	paths := make(chan string, 1)
	go followClaudeStatusLine(paths, 10*time.Millisecond, func(msg tea.Msg) { messages <- msg })
	paths <- viewerPath
	if line := nextClaudeStatusLine(t, messages); line != "viewer" {
		t.Errorf("line = %q, want %q", line, "viewer")
	}
	paths <- jobPath
	if line := nextClaudeStatusLine(t, messages); line != "job" {
		t.Errorf("line after the job's path = %q, want %q", line, "job")
	}
}

func followClaudeStatusLineSendsEachWholeLineAndNoneWhenFileGoes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session-id")
	messages := make(chan tea.Msg, 16)
	paths := make(chan string)
	go followClaudeStatusLine(paths, 10*time.Millisecond, func(msg tea.Msg) { messages <- msg })
	paths <- path
	writeStatusLineFile(t, path, roseLocation+"\n")
	if line := nextClaudeStatusLine(t, messages); line != roseLocation {
		t.Errorf("line = %q, want %q", line, roseLocation)
	}
	// a write caught midway has no newline yet and is never sent
	writeStatusLineFile(t, path, "half")
	time.Sleep(100 * time.Millisecond)
	writeStatusLineFile(t, path, "changed\n")
	if line := nextClaudeStatusLine(t, messages); line != "changed" {
		t.Errorf("line = %q, want %q", line, "changed")
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove %s: %v", path, err)
	}
	if line := nextClaudeStatusLine(t, messages); line != "" {
		t.Errorf("line after the file went = %q, want none", line)
	}
}

func followClaudeStatusLineReportsUnreadableFileAsProblemNamingIt(t *testing.T) {
	path := t.TempDir()
	messages := make(chan tea.Msg, 16)
	paths := make(chan string)
	go followClaudeStatusLine(paths, 10*time.Millisecond, func(msg tea.Msg) { messages <- msg })
	paths <- path
	select {
	case msg := <-messages:
		problem, isProblem := msg.(problemMsg)
		if !isProblem {
			t.Fatalf("got %#v, want a problemMsg", msg)
		}
		if want := "Cannot read " + path + ": "; !strings.HasPrefix(problem.err.Error(), want) {
			t.Errorf("problem = %q, want it to start with %q", problem.err, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("no message after 5 s")
	}
}
