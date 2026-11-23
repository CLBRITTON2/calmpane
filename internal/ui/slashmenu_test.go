package ui

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/CLBRITTON2/calmpane/claudecode"
)

var slashTestCommands = []claudecode.SlashCommand{
	{Name: "usage", Description: "Show session cost, plan usage, and activity stats"},
	{Name: "model", Description: "Switch model"},
	{Name: "clear", Description: "Start a new session with empty context"},
	{Name: "effort", Description: "Set the model's effort level"},
}

func TestSlashRanking(t *testing.T) {
	t.Run("fuzzyScoreRewardsConsecutiveLettersAtWordStartAndNeedsEveryLetterInOrder",
		fuzzyScoreRewardsConsecutiveLettersAtWordStartAndNeedsEveryLetterInOrder)
	t.Run("emptyQueryRanksMostUsedFirstThenByName", emptyQueryRanksMostUsedFirstThenByName)
	t.Run("queryKeepsCommandsHoldingItsLettersInOrderWithGaps", queryKeepsCommandsHoldingItsLettersInOrderWithGaps)
	t.Run("historyCommandsNotOnDiskJoinWithoutDescriptionAndDiskCommandsListOnce",
		historyCommandsNotOnDiskJoinWithoutDescriptionAndDiskCommandsListOnce)
	t.Run("historyBoostLiftsUsedCommandAboveBetterMatch", historyBoostLiftsUsedCommandAboveBetterMatch)
}

func fuzzyScoreRewardsConsecutiveLettersAtWordStartAndNeedsEveryLetterInOrder(t *testing.T) {
	cases := []struct {
		text        string
		query       string
		wantScore   int
		wantMatched bool
	}{
		// u at the start: 16+8, s next to it in the same run: 16+4+8
		{text: "usage", query: "us", wantScore: 52, wantMatched: true},
		{text: "Usage", query: "US", wantScore: 52, wantMatched: true},
		// e after a gap of three letters: 24-3-1-1, then 16
		{text: "usage", query: "ue", wantScore: 35, wantMatched: true},
		{text: "add-dir", query: "d", wantScore: 24, wantMatched: true},
		{text: "usage", query: "su", wantScore: 0, wantMatched: false},
		{text: "usage", query: "", wantScore: 0, wantMatched: true},
	}
	for _, testCase := range cases {
		score, matched := FuzzyScore(testCase.text, testCase.query)
		if score != testCase.wantScore || matched != testCase.wantMatched {
			t.Errorf("FuzzyScore(%q, %q) = %d, %t, want %d, %t", testCase.text, testCase.query, score, matched,
				testCase.wantScore, testCase.wantMatched)
		}
	}
}

func matchedNames(matches []slashMatch) []string {
	names := []string{}
	for _, match := range matches {
		names = append(names, match.command.Name)
	}
	return names
}

func emptyQueryRanksMostUsedFirstThenByName(t *testing.T) {
	uses := []SlashUse{
		{Name: "model", Count: 1, Last: slashTestNow.Add(-48 * time.Hour)},
		{Name: "usage", Count: 3, Last: slashTestNow.Add(-time.Minute)},
	}
	if names := matchedNames(RankedSlashCommands(slashTestCommands, []SlashUse{}, "", slashTestNow)); !reflect.DeepEqual(names,
		[]string{"clear", "effort", "model", "usage"}) {
		t.Errorf("names with no history = %v, want name order", names)
	}
	names := matchedNames(RankedSlashCommands(slashTestCommands, uses, "", slashTestNow))
	if want := []string{"usage", "model", "clear", "effort"}; !reflect.DeepEqual(names, want) {
		t.Errorf("names = %v, want %v", names, want)
	}
}

func queryKeepsCommandsHoldingItsLettersInOrderWithGaps(t *testing.T) {
	names := matchedNames(RankedSlashCommands(slashTestCommands, []SlashUse{}, "mdl", slashTestNow))
	if want := []string{"model"}; !reflect.DeepEqual(names, want) {
		t.Errorf("names for mdl = %v, want %v", names, want)
	}
	names = matchedNames(RankedSlashCommands(slashTestCommands, []SlashUse{}, "e", slashTestNow))
	// effort starts with e; the others hold it inside a word
	if want := []string{"effort", "clear", "model", "usage"}; !reflect.DeepEqual(names, want) {
		t.Errorf("names for e = %v, want %v", names, want)
	}
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
	if !reflect.DeepEqual(listed, want) {
		t.Errorf("listed = %+v, want %+v", listed, want)
	}
	rows := slashMenuRows(matches, 0, 60, testAppearance)
	if compactRow := strings.TrimRight(ansi.Strip(rows[0]), " "); compactRow != "   /compact" {
		t.Errorf("row of compact = %q, want its name alone", compactRow)
	}
}

func historyBoostLiftsUsedCommandAboveBetterMatch(t *testing.T) {
	uses := []SlashUse{{Name: "model", Count: 20, Last: slashTestNow.Add(-time.Minute)}}
	names := matchedNames(RankedSlashCommands(slashTestCommands, uses, "e", slashTestNow))
	if names[0] != "model" {
		t.Errorf("names for e = %v, want the much used model first", names)
	}
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
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("rows =\n%s\nwant\n%s", strings.Join(rows, "\n"), strings.Join(want, "\n"))
	}
}

func menuRowsScrollToKeepSelectedShownAmongEight(t *testing.T) {
	commands := []claudecode.SlashCommand{}
	for number := range 12 {
		commands = append(commands, claudecode.SlashCommand{Name: fmt.Sprintf("command-%02d", number), Description: ""})
	}
	matches := RankedSlashCommands(commands, []SlashUse{}, "", slashTestNow)
	rows := slashMenuRows(matches, 9, 80, testAppearance)
	if len(rows) != slashMenuMaxRows {
		t.Fatalf("menu shows %d rows, want %d", len(rows), slashMenuMaxRows)
	}
	if last := ansi.Strip(rows[len(rows)-1]); !strings.HasPrefix(last, "   /"+matches[9].command.Name+" ") {
		t.Errorf("last row = %q, want the selected /%s", last, matches[9].command.Name)
	}
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
	if err := os.WriteFile(historyFile, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := SavedSlashUses(historyFile)
	if err == nil {
		t.Fatal("SavedSlashUses of a corrupt file gave no error")
	}
	model := newZenModel(nil, testAppearance, "alt+x", historyFile).withSize(200, 20)
	loaded, _ := model.Update(slashCommandsLoadedMsg{commands: slashTestCommands, uses: nil, err: err})
	if shownError := ansi.Strip(loaded.(zenModel).shownStatus()); shownError != err.Error() {
		t.Errorf("status = %q, want %q", shownError, err.Error())
	}
	if commands := loaded.(zenModel).slashCommands; !reflect.DeepEqual(commands, slashTestCommands) {
		t.Errorf("commands = %+v, want the ones on disk %+v", commands, slashTestCommands)
	}
}

func typedKey(letter rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: letter, Text: string(letter)}
}

func modelWithSlashCommands(t *testing.T, agent Agent) zenModel {
	t.Helper()
	model := newZenModel(agent, testAppearance, "alt+x", filepath.Join(t.TempDir(), "slash-history.json"))
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
	if cmd == nil {
		t.Fatalf("%s returned no command", keys[len(keys)-1])
	}
	updated, _ = updated.Update(cmd())
	return updated.(zenModel)
}

func slashInEmptyInputOpensMenuInPlaceOfStatusLine(t *testing.T) {
	model := pressedKeys(t, modelWithSlashCommands(t, nil), typedKey('/'))
	if model.slashMenu == nil {
		t.Fatal("/ in the empty input opened no menu")
	}
	rows := strings.Split(ansi.Strip(model.screen()), "\n")
	if len(rows) != 20 {
		t.Errorf("screen has %d rows, want 20", len(rows))
	}
	menu := []string{}
	for _, row := range rows[len(rows)-4:] {
		menu = append(menu, strings.Fields(row)[0])
	}
	if want := []string{"/clear", "/effort", "/model", "/usage"}; !reflect.DeepEqual(menu, want) {
		t.Errorf("rows under the input start %v, want %v:\n%s", menu, want, strings.Join(rows, "\n"))
	}
	if screen := strings.Join(rows, "\n"); strings.Contains(screen, "Opus · ctx 12%") {
		t.Errorf("screen with the menu open still shows Claude's status line:\n%s", screen)
	}
}

func slashAfterTextOpensNoMenu(t *testing.T) {
	if model := pressedKeys(t, modelWithSlashCommands(t, nil), typedKey('a'), typedKey('/')); model.slashMenu != nil {
		t.Error("/ after text opened the menu")
	}
}

func upAndDownMoveSelectionAndStopAtFirstAndLast(t *testing.T) {
	model := pressedKeys(t, modelWithSlashCommands(t, nil), typedKey('/'))
	cases := []struct {
		key  tea.KeyPressMsg
		want int
	}{
		{key: pressedKey(tea.KeyDown), want: 1},
		{key: pressedKey(tea.KeyDown), want: 2},
		{key: pressedKey(tea.KeyDown), want: 3},
		{key: pressedKey(tea.KeyDown), want: 3},
		{key: pressedKey(tea.KeyUp), want: 2},
		{key: pressedKey(tea.KeyUp), want: 1},
		{key: pressedKey(tea.KeyUp), want: 0},
		{key: pressedKey(tea.KeyUp), want: 0},
	}
	for index, testCase := range cases {
		model = pressedKeys(t, model, testCase.key)
		if model.slashMenu.selected != testCase.want {
			t.Errorf("after key %d (%s) selected = %d, want %d", index+1, testCase.key, model.slashMenu.selected,
				testCase.want)
		}
	}
	if model.rowsBelowView != 0 {
		t.Errorf("up and down in the menu scrolled the view by %d rows", model.rowsBelowView)
	}
}

func tabPutsSelectedNameAndSpaceInInputAndClosesMenu(t *testing.T) {
	model := pressedKeys(t, modelWithSlashCommands(t, nil), typedKey('/'), pressedKey(tea.KeyDown), pressedKey(tea.KeyTab))
	if model.slashMenu != nil {
		t.Error("tab left the menu open")
	}
	if input := model.input.Value(); input != "/effort " {
		t.Errorf("input after tab = %q, want %q", input, "/effort ")
	}
}

func enterSendsSelectedCommand(t *testing.T) {
	agent := &agentRecorder{}
	keysSent(t, modelWithSlashCommands(t, agent), typedKey('/'), typedKey('m'), typedKey('l'), pressedKey(tea.KeyEnter))
	if want := []string{"prompt /model"}; !reflect.DeepEqual(agent.calls, want) {
		t.Errorf("calls = %v, want %v", agent.calls, want)
	}
}

func escClosesMenuAndKeepsInput(t *testing.T) {
	model := pressedKeys(t, modelWithSlashCommands(t, nil), typedKey('/'), typedKey('u'), pressedKey(tea.KeyEscape))
	if model.slashMenu != nil {
		t.Error("esc left the menu open")
	}
	if input := model.input.Value(); input != "/u" {
		t.Errorf("input after esc = %q, want %q", input, "/u")
	}
	if !strings.Contains(ansi.Strip(model.screen()), "Opus · ctx 12%") {
		t.Errorf("screen after esc lacks Claude's status line:\n%s", ansi.Strip(model.screen()))
	}
}

func spaceClosesMenuAndEnterSendsAsTyped(t *testing.T) {
	agent := &agentRecorder{}
	model := pressedKeys(t, modelWithSlashCommands(t, agent), typedKey('/'), typedKey('u'), typedKey(' '))
	if model.slashMenu != nil {
		t.Fatal("a space left the menu open")
	}
	keysSent(t, model, typedKey('x'), pressedKey(tea.KeyEnter))
	if want := []string{"prompt /u x"}; !reflect.DeepEqual(agent.calls, want) {
		t.Errorf("calls = %v, want %v", agent.calls, want)
	}
}

func deletingSlashClosesMenu(t *testing.T) {
	model := pressedKeys(t, modelWithSlashCommands(t, nil), typedKey('/'), pressedKey(tea.KeyBackspace))
	if model.slashMenu != nil {
		t.Error("deleting the / left the menu open")
	}
}

func slashOpensNoMenuWithNoCommandsOnDiskAndEmptyHistory(t *testing.T) {
	model := newZenModel(nil, testAppearance, "alt+x", "").withSize(60, 20)
	loaded, _ := model.Update(slashCommandsLoadedMsg{commands: []claudecode.SlashCommand{}, uses: []SlashUse{}, err: nil})
	if typed := pressedKeys(t, loaded.(zenModel), typedKey('/')); typed.slashMenu != nil {
		t.Error("/ opened a menu with no command on disk and an empty history")
	}
}

func loadedDiskCommandsAndHistoryFillMenu(t *testing.T) {
	model := newZenModel(nil, testAppearance, "alt+x", "").withSize(60, 20)
	loaded, _ := model.Update(slashCommandsLoadedMsg{commands: []claudecode.SlashCommand{
		{Name: "tidy", Description: "Tidy the imports (user)"},
	}, uses: []SlashUse{{Name: "effort", Count: 1, Last: time.Now()}}, err: nil})
	rows := strings.Split(ansi.Strip(pressedKeys(t, loaded.(zenModel), typedKey('/')).screen()), "\n")
	menu := []string{}
	for _, row := range rows[len(rows)-2:] {
		menu = append(menu, strings.TrimRight(row, " "))
	}
	if want := []string{"   /effort", "   /tidy    Tidy the imports (user)"}; !reflect.DeepEqual(menu, want) {
		t.Errorf("rows under the input = %q, want %q:\n%s", menu, want, strings.Join(rows, "\n"))
	}
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
	if err := os.WriteFile(blocker, []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
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
	if writeErr == nil {
		t.Fatal("writing the history under a file gave no error")
	}
	if shownError := ansi.Strip(sent.(zenModel).shownStatus()); shownError != writeErr.Error() {
		t.Errorf("status = %q, want %q", shownError, writeErr.Error())
	}
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
	if cmd == nil {
		t.Fatal("a sent slash command gave no command to write the history")
	}
	for _, msg := range msgsOf(cmd) {
		if written, isWritten := msg.(slashUsesWrittenMsg); isWritten && written.err != nil {
			t.Fatalf("write the history: %v", written.err)
		}
	}
	saved, err := SavedSlashUses(model.slashHistoryFile)
	if err != nil {
		t.Fatalf("SavedSlashUses: %v", err)
	}
	if len(saved) != 1 || saved[0].Name != "compact" || saved[0].Count != 1 {
		t.Errorf("history = %+v, want compact once", saved)
	}
	if uses := sent.(zenModel).slashUses; len(uses) != 1 || uses[0].Name != "compact" {
		t.Errorf("model's uses = %+v, want compact once", uses)
	}
}

func sentPathIsNotCountedInHistory(t *testing.T) {
	model := modelWithSlashCommands(t, &agentRecorder{pickerShown: true})
	sent, cmd := model.Update(promptSentMsg{prompt: "/tmp/x is empty", pastes: nil, err: nil})
	for _, msg := range msgsOf(cmd) {
		if _, isWritten := msg.(slashUsesWrittenMsg); isWritten {
			t.Error("a sent path wrote the history")
		}
	}
	if uses := sent.(zenModel).slashUses; len(uses) != 0 {
		t.Errorf("model's uses = %+v, want none", uses)
	}
	if _, err := os.Stat(model.slashHistoryFile); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("stat the history file: %v, want it not written", err)
	}
}

func pickerAfterCommandShowsWaitingLineUntilInputBoxReturns(t *testing.T) {
	model := modelWithSlashCommands(t, &agentRecorder{pickerShown: true})
	sent, cmd := model.Update(promptSentMsg{prompt: "/model", pastes: nil, err: nil})
	for _, msg := range msgsOf(cmd) {
		if _, isShown := msg.(agentPickerShownMsg); isShown {
			sent, _ = sent.Update(msg)
		}
	}
	waiting := ansi.Strip(sent.(zenModel).shownStatus())
	if want := "Waiting for you in Claude. Press alt+x to answer."; waiting != want {
		t.Errorf("status with the picker shown = %q, want %q", waiting, want)
	}
	closed, _ := sent.Update(agentPickerClosedMsg{prompt: "/model", err: nil})
	if statusAfterClose := ansi.Strip(closed.(zenModel).shownStatus()); strings.Contains(statusAfterClose, "Waiting for you") {
		t.Errorf("status after the picker closed = %q, want the waiting line gone", statusAfterClose)
	}
}

func settledCommandDropsItsEarlyBandUnlessQueued(t *testing.T) {
	model := modelWithSlashCommands(t, nil)
	model.input.SetValue("/model")
	sent, _ := model.withKey(pressedKey(tea.KeyEnter))
	settled, _ := sent.Update(slashCommandSettledMsg{prompt: "/model", err: nil})
	if screen := entriesText(t, settled.(zenModel)); strings.Contains(screen, "> /model") {
		t.Errorf("screen after the command settled still shows its band:\n%s", screen)
	}
	queued := sent.(zenModel).withEarlyPromptMarkedQueuedAt(0)
	stillQueued, _ := queued.Update(slashCommandSettledMsg{prompt: "/model", err: nil})
	if screen := entriesText(t, stillQueued.(zenModel)); !strings.Contains(screen, "> /model") {
		t.Errorf("screen after a queued command settled lacks its band:\n%s", screen)
	}
}
