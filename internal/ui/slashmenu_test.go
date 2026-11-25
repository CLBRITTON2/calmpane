package ui

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"

	"github.com/CLBRITTON2/calmpane/claudecode"
)

var slashTestCommands = []claudecode.SlashCommand{
	{Name: "usage", Description: "Show session cost, plan usage, and activity stats"},
	{Name: "model", Description: "Switch model"},
	{Name: "clear", Description: "Start a new session with empty context"},
	{Name: "effort", Description: "Set the model's effort level"},
}

func TestSlashRanking(t *testing.T) {
	t.Run("fuzzyScoreRewardsConsecutiveLettersAtWordStart", fuzzyScoreRewardsConsecutiveLettersAtWordStart)
	t.Run("fuzzyScoreIgnoresCase", fuzzyScoreIgnoresCase)
	t.Run("fuzzyScoreSubtractsGapBeforeLetter", fuzzyScoreSubtractsGapBeforeLetter)
	t.Run("fuzzyScoreRewardsLetterStartingWordAfterHyphen", fuzzyScoreRewardsLetterStartingWordAfterHyphen)
	t.Run("fuzzyScoreMatchesNothingWithLettersOutOfOrder", fuzzyScoreMatchesNothingWithLettersOutOfOrder)
	t.Run("fuzzyScoreMatchesEveryTextWithEmptyQuery", fuzzyScoreMatchesEveryTextWithEmptyQuery)
	t.Run("emptyQueryRanksByNameWithNoHistory", emptyQueryRanksByNameWithNoHistory)
	t.Run("emptyQueryRanksMostUsedFirstThenByName", emptyQueryRanksMostUsedFirstThenByName)
	t.Run("queryKeepsCommandsHoldingItsLettersInOrderWithGaps", queryKeepsCommandsHoldingItsLettersInOrderWithGaps)
	t.Run("queryRanksCommandStartingWithItFirst", queryRanksCommandStartingWithItFirst)
	t.Run("historyCommandsNotOnDiskJoinWithoutDescriptionAndDiskCommandsListOnce",
		historyCommandsNotOnDiskJoinWithoutDescriptionAndDiskCommandsListOnce)
	t.Run("historyBoostLiftsUsedCommandAboveBetterMatch", historyBoostLiftsUsedCommandAboveBetterMatch)
}

func fuzzyScoreRewardsConsecutiveLettersAtWordStart(t *testing.T) {
	score, matched := FuzzyScore("usage", "us")
	// u at the start: 16+8, s next to it in the same run: 16+4+8
	require.Equal(t, 52, score)
	require.True(t, matched)
}

func fuzzyScoreIgnoresCase(t *testing.T) {
	score, matched := FuzzyScore("Usage", "US")
	require.Equal(t, 52, score)
	require.True(t, matched)
}

func fuzzyScoreSubtractsGapBeforeLetter(t *testing.T) {
	score, matched := FuzzyScore("usage", "ue")
	// e after a gap of three letters: 24-3-1-1, then 16
	require.Equal(t, 35, score)
	require.True(t, matched)
}

func fuzzyScoreRewardsLetterStartingWordAfterHyphen(t *testing.T) {
	score, matched := FuzzyScore("add-dir", "d")
	require.Equal(t, 24, score)
	require.True(t, matched)
}

func fuzzyScoreMatchesNothingWithLettersOutOfOrder(t *testing.T) {
	score, matched := FuzzyScore("usage", "su")
	require.Equal(t, 0, score)
	require.False(t, matched)
}

func fuzzyScoreMatchesEveryTextWithEmptyQuery(t *testing.T) {
	score, matched := FuzzyScore("usage", "")
	require.Equal(t, 0, score)
	require.True(t, matched)
}

func matchedNames(matches []slashMatch) []string {
	names := []string{}
	for _, match := range matches {
		names = append(names, match.command.Name)
	}
	return names
}

func emptyQueryRanksByNameWithNoHistory(t *testing.T) {
	matches := RankedSlashCommands(slashTestCommands, []SlashUse{}, "", slashTestNow)
	names := matchedNames(matches)
	require.Equal(t, []string{"clear", "effort", "model", "usage"}, names)
}

func emptyQueryRanksMostUsedFirstThenByName(t *testing.T) {
	uses := []SlashUse{
		{Name: "model", Count: 1, Last: slashTestNow.Add(-48 * time.Hour)},
		{Name: "usage", Count: 3, Last: slashTestNow.Add(-time.Minute)},
	}
	matches := RankedSlashCommands(slashTestCommands, uses, "", slashTestNow)
	names := matchedNames(matches)
	require.Equal(t, []string{"usage", "model", "clear", "effort"}, names)
}

func queryKeepsCommandsHoldingItsLettersInOrderWithGaps(t *testing.T) {
	matches := RankedSlashCommands(slashTestCommands, []SlashUse{}, "mdl", slashTestNow)
	names := matchedNames(matches)
	require.Equal(t, []string{"model"}, names)
}

func queryRanksCommandStartingWithItFirst(t *testing.T) {
	matches := RankedSlashCommands(slashTestCommands, []SlashUse{}, "e", slashTestNow)
	names := matchedNames(matches)
	// effort starts with e; the others hold it inside a word
	require.Equal(t, []string{"effort", "clear", "model", "usage"}, names)
}

func historyCommandsNotOnDiskJoinWithoutDescriptionAndDiskCommandsListOnce(t *testing.T) {
	uses := []SlashUse{
		{Name: "model", Count: 1, Last: slashTestNow.Add(-48 * time.Hour)},
		{Name: "compact", Count: 1, Last: slashTestNow.Add(-48 * time.Hour)},
	}
	matches := RankedSlashCommands(slashTestCommands, uses, "", slashTestNow)
	want := []claudecode.SlashCommand{
		{Name: "compact", Description: ""},
		{Name: "model", Description: "Switch model"},
		{Name: "clear", Description: "Start a new session with empty context"},
		{Name: "effort", Description: "Set the model's effort level"},
		{Name: "usage", Description: "Show session cost, plan usage, and activity stats"},
	}
	listed := []claudecode.SlashCommand{}
	for _, match := range matches {
		listed = append(listed, match.command)
	}
	require.Equal(t, want, listed)
	rows := slashMenuRows(matches, 0, 60, testAppearance)
	compactRow := strings.TrimRight(ansi.Strip(rows[0]), " ")
	require.Equal(t, "   /compact", compactRow)
}

func historyBoostLiftsUsedCommandAboveBetterMatch(t *testing.T) {
	uses := []SlashUse{{Name: "model", Count: 20, Last: slashTestNow.Add(-time.Minute)}}
	matches := RankedSlashCommands(slashTestCommands, uses, "e", slashTestNow)
	names := matchedNames(matches)
	require.Equal(t, "model", names[0])
}

func TestSlashMenuRows(t *testing.T) {
	t.Run("menuRowsAlignDescriptionsCutAtWidthAndColorSelectedName", menuRowsAlignDescriptionsCutAtWidthAndColorSelectedName)
	t.Run("menuRowsScrollToKeepSelectedShownAmongEight", menuRowsScrollToKeepSelectedShownAmongEight)
}

func menuRowsAlignDescriptionsCutAtWidthAndColorSelectedName(t *testing.T) {
	matches := RankedSlashCommands(slashTestCommands, []SlashUse{}, "", slashTestNow)
	rows := slashMenuRows(matches, 1, 40, testAppearance)
	want := []string{
		"   " + testAppearance.menuNameStyle.Render("/clear") + "   " +
			testAppearance.workingStyle.Render("Start a new session with e…"),
		"   " + testAppearance.menuPickedNameStyle.Render("/effort") + "  " +
			testAppearance.workingStyle.Render("Set the model's effort lev…"),
		"   " + testAppearance.menuNameStyle.Render("/model") + "   " + testAppearance.workingStyle.Render("Switch model"),
		"   " + testAppearance.menuNameStyle.Render("/usage") + "   " +
			testAppearance.workingStyle.Render("Show session cost, plan us…"),
	}
	require.Equal(t, want, rows)
}

func menuRowsScrollToKeepSelectedShownAmongEight(t *testing.T) {
	commands := []claudecode.SlashCommand{}
	for number := range 12 {
		commands = append(commands, claudecode.SlashCommand{Name: fmt.Sprintf("command-%02d", number), Description: ""})
	}
	matches := RankedSlashCommands(commands, []SlashUse{}, "", slashTestNow)
	rows := slashMenuRows(matches, 9, 80, testAppearance)
	require.Len(t, rows, slashMenuMaxRows)
	last := ansi.Strip(rows[len(rows)-1])
	selectedName := matches[9].command.Name
	lastIsSelected := strings.HasPrefix(last, "   /"+selectedName+" ")
	require.True(t, lastIsSelected, last)
}

func TestSlashMenu(t *testing.T) {
	t.Run("slashInEmptyInputOpensMenuInPlaceOfStatusLine", slashInEmptyInputOpensMenuInPlaceOfStatusLine)
	t.Run("slashAfterTextOpensNoMenu", slashAfterTextOpensNoMenu)
	t.Run("upAndDownMoveSelectionAndStopAtFirstAndLast", upAndDownMoveSelectionAndStopAtFirstAndLast)
	t.Run("tabPutsSelectedNameAndSpaceInInputAndClosesMenu", tabPutsSelectedNameAndSpaceInInputAndClosesMenu)
	t.Run("enterSendsSelectedCommand", enterSendsSelectedCommand)
	t.Run("escClosesMenuAndKeepsInput", escClosesMenuAndKeepsInput)
	t.Run("spaceClosesMenuAndEnterSendsAsTyped", spaceClosesMenuAndEnterSendsAsTyped)
	t.Run("deletingSlashClosesMenu", deletingSlashClosesMenu)
	t.Run("slashOpensNoMenuWithNoCommandsOnDiskAndEmptyHistory", slashOpensNoMenuWithNoCommandsOnDiskAndEmptyHistory)
	t.Run("loadedDiskCommandsAndHistoryFillMenu", loadedDiskCommandsAndHistoryFillMenu)
	t.Run("corruptHistoryShowsErrorOnStatusLineAndKeepsDiskCommands", corruptHistoryShowsErrorOnStatusLineAndKeepsDiskCommands)
}

func corruptHistoryShowsErrorOnStatusLineAndKeepsDiskCommands(t *testing.T) {
	historyFile := filepath.Join(t.TempDir(), "slash-history.json")
	writeErr := os.WriteFile(historyFile, []byte("{not json"), 0o644)
	require.NoError(t, writeErr)
	_, err := SavedSlashUses(historyFile)
	require.Error(t, err)
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", historyFile).withSize(200, 20)
	loaded, _ := model.Update(slashCommandsLoadedMsg{commands: slashTestCommands, uses: nil, err: err})
	loadedModel := loaded.(zenModel)
	shownError := ansi.Strip(loadedModel.shownStatus())
	wantError := err.Error()
	require.Equal(t, wantError, shownError)
	require.Equal(t, slashTestCommands, loadedModel.slashCommands)
}

func typedKey(letter rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: letter, Text: string(letter)}
}

func modelWithSlashCommands(t *testing.T, agent Agent) zenModel {
	t.Helper()
	model := newZenModel(agent, testAppearance, "alt+x", "ctrl+e", filepath.Join(t.TempDir(), "slash-history.json"))
	model = model.withSize(60, 20).withEntries([]claudecode.DialogueEntry{{Kind: claudecode.ReplyEntry, Text: "Ready."}}, nil)
	model.slashCommands = slashTestCommands
	model.claudeStatusLine = "Opus · ctx 12%"
	return model.withVisibleRowsDrawn()
}

// keysSent applies keys to model and runs the command the last one gives, as bubbletea would.
func keysSent(t *testing.T, model zenModel, keys ...tea.KeyPressMsg) zenModel {
	t.Helper()
	typed := pressedKeys(t, model, keys[:len(keys)-1]...)
	updated, cmd := typed.Update(keys[len(keys)-1])
	require.NotNil(t, cmd)
	updated, _ = updated.Update(cmd())
	return updated.(zenModel)
}

func slashInEmptyInputOpensMenuInPlaceOfStatusLine(t *testing.T) {
	model := pressedKeys(t, modelWithSlashCommands(t, nil), typedKey('/'))
	require.NotNil(t, model.slashMenu)
	screen := ansi.Strip(model.screen())
	rows := strings.Split(screen, "\n")
	require.Len(t, rows, 20)
	menu := []string{}
	for _, row := range rows[len(rows)-4:] {
		menu = append(menu, strings.Fields(row)[0])
	}
	require.Equal(t, []string{"/clear", "/effort", "/model", "/usage"}, menu)
	require.NotContains(t, screen, "Opus · ctx 12%")
}

func slashAfterTextOpensNoMenu(t *testing.T) {
	model := pressedKeys(t, modelWithSlashCommands(t, nil), typedKey('a'), typedKey('/'))
	require.Nil(t, model.slashMenu)
}

func upAndDownMoveSelectionAndStopAtFirstAndLast(t *testing.T) {
	model := pressedKeys(t, modelWithSlashCommands(t, nil), typedKey('/'))
	model = pressedKeys(t, model, pressedKey(tea.KeyDown))
	require.Equal(t, 1, model.slashMenu.selected)
	model = pressedKeys(t, model, pressedKey(tea.KeyDown))
	require.Equal(t, 2, model.slashMenu.selected)
	model = pressedKeys(t, model, pressedKey(tea.KeyDown))
	require.Equal(t, 3, model.slashMenu.selected)
	model = pressedKeys(t, model, pressedKey(tea.KeyDown))
	require.Equal(t, 3, model.slashMenu.selected)
	model = pressedKeys(t, model, pressedKey(tea.KeyUp))
	require.Equal(t, 2, model.slashMenu.selected)
	model = pressedKeys(t, model, pressedKey(tea.KeyUp))
	require.Equal(t, 1, model.slashMenu.selected)
	model = pressedKeys(t, model, pressedKey(tea.KeyUp))
	require.Equal(t, 0, model.slashMenu.selected)
	model = pressedKeys(t, model, pressedKey(tea.KeyUp))
	require.Equal(t, 0, model.slashMenu.selected)
	require.Equal(t, 0, model.rowsBelowView)
}

func tabPutsSelectedNameAndSpaceInInputAndClosesMenu(t *testing.T) {
	model := pressedKeys(t, modelWithSlashCommands(t, nil), typedKey('/'), pressedKey(tea.KeyDown), pressedKey(tea.KeyTab))
	require.Nil(t, model.slashMenu)
	input := model.input.Value()
	require.Equal(t, "/effort ", input)
}

func enterSendsSelectedCommand(t *testing.T) {
	agent := &agentRecorder{}
	keysSent(t, modelWithSlashCommands(t, agent), typedKey('/'), typedKey('m'), typedKey('l'), pressedKey(tea.KeyEnter))
	require.Equal(t, []string{"prompt /model"}, agent.calls)
}

func escClosesMenuAndKeepsInput(t *testing.T) {
	model := pressedKeys(t, modelWithSlashCommands(t, nil), typedKey('/'), typedKey('u'), pressedKey(tea.KeyEscape))
	require.Nil(t, model.slashMenu)
	input := model.input.Value()
	require.Equal(t, "/u", input)
	screen := ansi.Strip(model.screen())
	require.Contains(t, screen, "Opus · ctx 12%")
}

func spaceClosesMenuAndEnterSendsAsTyped(t *testing.T) {
	agent := &agentRecorder{}
	model := pressedKeys(t, modelWithSlashCommands(t, agent), typedKey('/'), typedKey('u'), typedKey(' '))
	require.Nil(t, model.slashMenu)
	keysSent(t, model, typedKey('x'), pressedKey(tea.KeyEnter))
	require.Equal(t, []string{"prompt /u x"}, agent.calls)
}

func deletingSlashClosesMenu(t *testing.T) {
	model := pressedKeys(t, modelWithSlashCommands(t, nil), typedKey('/'), pressedKey(tea.KeyBackspace))
	require.Nil(t, model.slashMenu)
}

func slashOpensNoMenuWithNoCommandsOnDiskAndEmptyHistory(t *testing.T) {
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "").withSize(60, 20)
	loaded, _ := model.Update(slashCommandsLoadedMsg{commands: []claudecode.SlashCommand{}, uses: []SlashUse{}, err: nil})
	typed := pressedKeys(t, loaded.(zenModel), typedKey('/'))
	require.Nil(t, typed.slashMenu)
}

func loadedDiskCommandsAndHistoryFillMenu(t *testing.T) {
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "").withSize(60, 20)
	loaded, _ := model.Update(slashCommandsLoadedMsg{commands: []claudecode.SlashCommand{
		{Name: "tidy", Description: "Tidy the imports (user)"},
	}, uses: []SlashUse{{Name: "effort", Count: 1, Last: time.Now()}}, err: nil})
	typed := pressedKeys(t, loaded.(zenModel), typedKey('/'))
	screen := ansi.Strip(typed.screen())
	rows := strings.Split(screen, "\n")
	menu := []string{}
	for _, row := range rows[len(rows)-2:] {
		menu = append(menu, strings.TrimRight(row, " "))
	}
	require.Equal(t, []string{"   /effort", "   /tidy    Tidy the imports (user)"}, menu)
}

func TestSlashCommandSent(t *testing.T) {
	t.Run("sentCommandNotListedCountsUnderTypedNameInHistoryFile", sentCommandNotListedCountsUnderTypedNameInHistoryFile)
	t.Run("sentPathIsNotCountedInHistory", sentPathIsNotCountedInHistory)
	t.Run("pickerAfterCommandShowsWaitingLineUntilInputBoxReturns", pickerAfterCommandShowsWaitingLineUntilInputBoxReturns)
	t.Run("settledCommandDropsItsEarlyBandUnlessQueued", settledCommandDropsItsEarlyBandUnlessQueued)
	t.Run("failedHistoryWriteShowsErrorOnStatusLine", failedHistoryWriteShowsErrorOnStatusLine)
}

func failedHistoryWriteShowsErrorOnStatusLine(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "calmpane")
	blockerErr := os.WriteFile(blocker, []byte{}, 0o644)
	require.NoError(t, blockerErr)
	model := modelWithSlashCommands(t, &agentRecorder{pickerShown: true})
	model.slashHistoryFile = filepath.Join(blocker, "slash-history.json")
	sent, cmd := model.Update(promptSentMsg{prompt: "/usage", pastes: nil, err: nil})
	var writeErr error
	for _, msg := range msgsOf(cmd) {
		if written, isWritten := msg.(slashUsesWrittenMsg); isWritten {
			writeErr = written.err
			sent, _ = sent.Update(msg)
		}
	}
	require.Error(t, writeErr)
	sentModel := sent.(zenModel)
	shownError := ansi.Strip(sentModel.shownStatus())
	wantError := writeErr.Error()
	require.Equal(t, wantError, shownError)
}

// msgsOf runs cmd and the commands of a batch it gives, as bubbletea would, and gives their messages.
func msgsOf(cmd tea.Cmd) []tea.Msg {
	msg := cmd()
	batch, isBatch := msg.(tea.BatchMsg)
	if !isBatch {
		return []tea.Msg{msg}
	}
	msgs := []tea.Msg{}
	for _, inner := range batch {
		msgs = append(msgs, msgsOf(inner)...)
	}
	return msgs
}

func sentCommandNotListedCountsUnderTypedNameInHistoryFile(t *testing.T) {
	model := modelWithSlashCommands(t, &agentRecorder{pickerShown: true})
	sent, cmd := model.Update(promptSentMsg{prompt: "/compact keep the tests", pastes: nil, err: nil})
	require.NotNil(t, cmd)
	for _, msg := range msgsOf(cmd) {
		if written, isWritten := msg.(slashUsesWrittenMsg); isWritten {
			require.NoError(t, written.err)
		}
	}
	saved, err := SavedSlashUses(model.slashHistoryFile)
	require.NoError(t, err)
	require.Len(t, saved, 1)
	require.Equal(t, "compact", saved[0].Name)
	require.Equal(t, 1, saved[0].Count)
	uses := sent.(zenModel).slashUses
	require.Len(t, uses, 1)
	require.Equal(t, "compact", uses[0].Name)
}

func sentPathIsNotCountedInHistory(t *testing.T) {
	model := modelWithSlashCommands(t, &agentRecorder{pickerShown: true})
	sent, cmd := model.Update(promptSentMsg{prompt: "/tmp/x is empty", pastes: nil, err: nil})
	for _, msg := range msgsOf(cmd) {
		require.IsNotType(t, slashUsesWrittenMsg{}, msg)
	}
	uses := sent.(zenModel).slashUses
	require.Empty(t, uses)
	_, statErr := os.Stat(model.slashHistoryFile)
	require.ErrorIs(t, statErr, fs.ErrNotExist)
}

func pickerAfterCommandShowsWaitingLineUntilInputBoxReturns(t *testing.T) {
	model := modelWithSlashCommands(t, &agentRecorder{pickerShown: true})
	sent, cmd := model.Update(promptSentMsg{prompt: "/model", pastes: nil, err: nil})
	for _, msg := range msgsOf(cmd) {
		if _, isShown := msg.(agentPickerShownMsg); isShown {
			sent, _ = sent.Update(msg)
		}
	}
	sentModel := sent.(zenModel)
	waiting := ansi.Strip(sentModel.shownStatus())
	require.Equal(t, "Waiting for you in Claude. Press alt+x to answer.", waiting)
	closed, _ := sent.Update(agentPickerClosedMsg{prompt: "/model", err: nil})
	closedModel := closed.(zenModel)
	statusAfterClose := ansi.Strip(closedModel.shownStatus())
	require.NotContains(t, statusAfterClose, "Waiting for you")
}

func settledCommandDropsItsEarlyBandUnlessQueued(t *testing.T) {
	model := modelWithSlashCommands(t, nil)
	model.input.SetValue("/model")
	sent, _ := model.withKey(pressedKey(tea.KeyEnter))
	settled, _ := sent.Update(slashCommandSettledMsg{prompt: "/model", err: nil})
	settledEntries := entriesText(t, settled.(zenModel))
	require.NotContains(t, settledEntries, "> /model")
	queued := sent.(zenModel).withEarlyPromptMarkedQueuedAt(0)
	stillQueued, _ := queued.Update(slashCommandSettledMsg{prompt: "/model", err: nil})
	queuedEntries := entriesText(t, stillQueued.(zenModel))
	require.Contains(t, queuedEntries, "> /model")
}
