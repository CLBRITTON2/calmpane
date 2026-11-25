package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"

	"github.com/CLBRITTON2/calmpane/internal/config"
)

const roseLocation = "\x1b[38;2;235;188;186mherdr-zen/zen-status-line\x1b[0m"

func modelWithClaudeStatusLine(line string) zenModel {
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "")
	model = model.withSize(40, 12).withEntries(numberedPrompts(1, 20), nil)
	updated, _ := model.Update(claudeStatusLineMsg{line: line})
	return updated.(zenModel)
}

func TestRowsAroundInput(t *testing.T) {
	t.Run("claudeStatusLineDrawnUnderSeparatorWithItsColorsAndCutAtWidth", claudeStatusLineDrawnUnderSeparatorWithItsColorsAndCutAtWidth)
	t.Run("blankRowSeparatesNewestEntryFromWorkingRow", blankRowSeparatesNewestEntryFromWorkingRow)
	t.Run("newestEntrySitsRightAboveEmptyStatusRowWithNoStatus", newestEntrySitsRightAboveEmptyStatusRowWithNoStatus)
	t.Run("inputIsLastRowWithNoClaudeStatusLine", inputIsLastRowWithNoClaudeStatusLine)
	t.Run("separatorRepeatsConfiguredSeparatorLineAcrossWidth", separatorRepeatsConfiguredSeparatorLineAcrossWidth)
}

func separatorRepeatsConfiguredSeparatorLineAcrossWidth(t *testing.T) {
	settings := exampleSettings(t)
	settings.Lines = config.Lines{Separator: "━", CodeRule: "═"}
	model := newZenModel(nil, appearanceFrom(t, settings), "alt+x", "ctrl+e", "").withSize(40, 12)
	screen := ansi.Strip(model.screen())
	separator := strings.Repeat("━", 40)
	require.Contains(t, screen, separator)
	require.NotContains(t, screen, "═")
	require.NotContains(t, screen, "─")
}

func claudeStatusLineDrawnUnderSeparatorWithItsColorsAndCutAtWidth(t *testing.T) {
	line := roseLocation + " \x1b[38;2;156;207;216m" + strings.Repeat("Opus 5.5 ", 8) + "\x1b[0m"
	model := modelWithClaudeStatusLine(line)
	rows := strings.Split(model.screen(), "\n")
	require.Len(t, rows, 12)
	input := ansi.Strip(rows[9])
	startsWithInput := strings.HasPrefix(input, " > ")
	require.True(t, startsWithInput, input)
	separator := ansi.Strip(rows[10])
	wantSeparator := strings.Repeat("─", 40)
	require.Equal(t, wantSeparator, separator)
	statusRow := rows[11]
	keepsLineColorsAfterMargin := strings.HasPrefix(statusRow, " "+roseLocation+" \x1b[38;2;156;207;216mOpus")
	require.True(t, keepsLineColorsAfterMargin, statusRow)
	width := ansi.StringWidth(statusRow)
	require.Equal(t, 40, width)
	resetsColorsAtEnd := strings.HasSuffix(statusRow, ansi.ResetStyle)
	require.True(t, resetsColorsAtEnd, statusRow)
}

func blankRowSeparatesNewestEntryFromWorkingRow(t *testing.T) {
	model := modelWithClaudeStatusLine(roseLocation)
	model.agentStatus = AgentWorking
	rows := strings.Split(model.screen(), "\n")
	newest := ansi.Strip(rows[5])
	require.Contains(t, newest, "prompt 20")
	blank := rows[6]
	require.Empty(t, blank)
	working := ansi.Strip(rows[7])
	require.Equal(t, " Working...", working)
}

func newestEntrySitsRightAboveEmptyStatusRowWithNoStatus(t *testing.T) {
	model := modelWithClaudeStatusLine(roseLocation)
	rows := strings.Split(model.screen(), "\n")
	newest := ansi.Strip(rows[6])
	require.Contains(t, newest, "prompt 20")
	statusRow := rows[7]
	require.Equal(t, " ", statusRow)
}

func inputIsLastRowWithNoClaudeStatusLine(t *testing.T) {
	neverShown := modelWithClaudeStatusLine("")
	neverShownScreen := neverShown.screen()
	plainScreen := ansi.Strip(neverShownScreen)
	rows := strings.Split(plainScreen, "\n")
	require.Len(t, rows, 12)
	last := rows[len(rows)-1]
	lastIsInput := strings.HasPrefix(last, " > ")
	require.True(t, lastIsInput, last)
	shown := modelWithClaudeStatusLine(roseLocation)
	gone, _ := shown.Update(claudeStatusLineMsg{line: ""})
	goneScreen := gone.(zenModel).screen()
	require.Equal(t, neverShownScreen, goneScreen)
}

func nextClaudeStatusLine(t *testing.T, messages <-chan tea.Msg) string {
	t.Helper()
	select {
	case msg := <-messages:
		require.IsType(t, claudeStatusLineMsg{}, msg)
		statusLine := msg.(claudeStatusLineMsg)
		return statusLine.line
	case <-time.After(5 * time.Second):
		require.FailNow(t, "no status line message after 5 s")
		return ""
	}
}

func writeStatusLineFile(t *testing.T, path string, content string) {
	t.Helper()
	err := os.WriteFile(path, []byte(content), 0o600)
	require.NoError(t, err)
}

func TestFollowClaudeStatusLine(t *testing.T) {
	t.Run("followClaudeStatusLineSendsEachWholeLineAndNoneWhenFileGoes", followClaudeStatusLineSendsEachWholeLineAndNoneWhenFileGoes)
	t.Run("followClaudeStatusLineReportsUnreadableFileAsProblemNamingIt", followClaudeStatusLineReportsUnreadableFileAsProblemNamingIt)
	t.Run("followClaudeStatusLineFollowsNewestSessionSent", followClaudeStatusLineFollowsNewestSessionSent)
}

func followClaudeStatusLineFollowsNewestSessionSent(t *testing.T) {
	directory := t.TempDir()
	writeStatusLineFile(t, filepath.Join(directory, "viewer-session-id"), "viewer\n")
	writeStatusLineFile(t, filepath.Join(directory, "job-session-id"), "job\n")
	messages := make(chan tea.Msg, 16)
	// buffered, so a follower that stopped taking sessions fails the test instead of hanging it
	sessionIDs := make(chan string, 1)
	go followClaudeStatusLine(directory, sessionIDs, 10*time.Millisecond, func(msg tea.Msg) { messages <- msg })
	sessionIDs <- "viewer-session-id"
	viewerLine := nextClaudeStatusLine(t, messages)
	require.Equal(t, "viewer", viewerLine)
	sessionIDs <- "job-session-id"
	jobLine := nextClaudeStatusLine(t, messages)
	require.Equal(t, "job", jobLine)
}

func followClaudeStatusLineSendsEachWholeLineAndNoneWhenFileGoes(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "session-id")
	messages := make(chan tea.Msg, 16)
	sessionIDs := make(chan string)
	go followClaudeStatusLine(directory, sessionIDs, 10*time.Millisecond, func(msg tea.Msg) { messages <- msg })
	sessionIDs <- "session-id"
	writeStatusLineFile(t, path, roseLocation+"\n")
	firstLine := nextClaudeStatusLine(t, messages)
	require.Equal(t, roseLocation, firstLine)
	// a write caught midway has no newline yet and is never sent
	writeStatusLineFile(t, path, "half")
	time.Sleep(100 * time.Millisecond)
	writeStatusLineFile(t, path, "changed\n")
	changedLine := nextClaudeStatusLine(t, messages)
	require.Equal(t, "changed", changedLine)
	removeErr := os.Remove(path)
	require.NoError(t, removeErr)
	lineAfterFileWent := nextClaudeStatusLine(t, messages)
	require.Empty(t, lineAfterFileWent)
}

func followClaudeStatusLineReportsUnreadableFileAsProblemNamingIt(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "session-id")
	mkdirErr := os.Mkdir(path, 0o755)
	require.NoError(t, mkdirErr)
	messages := make(chan tea.Msg, 16)
	sessionIDs := make(chan string)
	go followClaudeStatusLine(directory, sessionIDs, 10*time.Millisecond, func(msg tea.Msg) { messages <- msg })
	sessionIDs <- "session-id"
	var msg tea.Msg
	select {
	case msg = <-messages:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "no message after 5 s")
	}
	require.IsType(t, problemMsg{}, msg)
	problem := msg.(problemMsg)
	problemText := problem.err.Error()
	wantPrefix := "Cannot read " + path + ": "
	namesFile := strings.HasPrefix(problemText, wantPrefix)
	require.True(t, namesFile, problemText)
}
