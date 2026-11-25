package ui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"

	"github.com/CLBRITTON2/calmpane/claudecode"
)

func TestLineNumberAt(t *testing.T) {
	t.Run("lineNumberAtGivesOneAtStart", lineNumberAtGivesOneAtStart)
	t.Run("lineNumberAtGivesTwoAfterFirstLine", lineNumberAtGivesTwoAfterFirstLine)
	t.Run("lineNumberAtGivesThreeAfterSecondLine", lineNumberAtGivesThreeAfterSecondLine)
}

func assertLineNumberAt(t *testing.T, offset int64, want int) {
	t.Helper()
	content := "first\nsecond\nthird\n"
	got, err := LineNumberAt(strings.NewReader(content), offset)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func lineNumberAtGivesOneAtStart(t *testing.T) {
	assertLineNumberAt(t, 0, 1)
}

func lineNumberAtGivesTwoAfterFirstLine(t *testing.T) {
	assertLineNumberAt(t, int64(len("first\n")), 2)
}

func lineNumberAtGivesThreeAfterSecondLine(t *testing.T) {
	assertLineNumberAt(t, int64(len("first\nsecond\n")), 3)
}

func TestFollowTranscript(t *testing.T) {
	t.Run("followTranscriptNamesUndecodableLineAndKeepsReading", followTranscriptNamesUndecodableLineAndKeepsReading)
	t.Run("followTranscriptSendsQueueOperationsInTheirPlaceAmongEntries", followTranscriptSendsQueueOperationsInTheirPlaceAmongEntries)
	t.Run("followTranscriptSendsTurnOfLineWithOnlyToolCall", followTranscriptSendsTurnOfLineWithOnlyToolCall)
	t.Run("followTranscriptSendsRunningSubagentsOnChangeAndALastingProblemOnce",
		followTranscriptSendsRunningSubagentsOnChangeAndALastingProblemOnce)
}

func followTranscriptNamesUndecodableLineAndKeepsReading(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	content := `{"type":"user","message":{"role":"user","content":"first"}}` + "\n" +
		`{"type":"user","message":{"role":"user","content":"cut` + "\n" +
		`{"type":"user","message":{"role":"user","content":"third"}}` + "\n"
	writeErr := os.WriteFile(path, []byte(content), 0o600)
	require.NoError(t, writeErr)
	transcript, err := os.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() {
		closeErr := transcript.Close()
		require.NoError(t, closeErr)
	})
	messages := make(chan tea.Msg, 16)
	go func() {
		sameSession := func() (string, error) { return "session", nil }
		_, err := followTranscript(transcript, "session", sameSession, func(msg tea.Msg) { messages <- msg })
		messages <- fatalMsg{err: err}
	}()
	problemSeen, entriesSeen := false, false
	for !problemSeen || !entriesSeen {
		select {
		case msg := <-messages:
			switch msg := msg.(type) {
			case problemMsg:
				problemSeen = true
				problemText := msg.err.Error()
				namesLineTwo := strings.HasPrefix(problemText, "Cannot read line 2 of ")
				require.True(t, namesLineTwo, problemText)
			case entriesReadMsg:
				entriesSeen = true
				want := []claudecode.DialogueEntry{{Kind: claudecode.PromptEntry, Text: "first"}, {Kind: claudecode.PromptEntry, Text: "third"}}
				require.Equal(t, want, msg.entries)
			case fatalMsg:
				require.FailNow(t, "followTranscript returned", "%v", msg.err)
			}
		case <-time.After(5 * time.Second):
			require.FailNow(t, "no problem and entries after 5 s", "problem seen %t, entries seen %t", problemSeen, entriesSeen)
		}
	}
}

func TestPromptSend(t *testing.T) {
	t.Run("enterClearsInputAndSendRefusedWhileBlockedPutsTextBack", enterClearsInputAndSendRefusedWhileBlockedPutsTextBack)
	t.Run("enterClearsInputAndSendRefusedWhileBlockedNamesConfiguredKey", enterClearsInputAndSendRefusedWhileBlockedNamesConfiguredKey)
	t.Run("enterClearsInputAndHerdrFailurePutsTextBack", enterClearsInputAndHerdrFailurePutsTextBack)
	t.Run("enterClearsInputAndSendRefusedAfterMoreTypingPutsTextBeforeIt", enterClearsInputAndSendRefusedAfterMoreTypingPutsTextBeforeIt)
	t.Run("enterClearsInputAndSentPromptLeavesItEmpty", enterClearsInputAndSentPromptLeavesItEmpty)
	t.Run("enterClearsInputAndSentPromptKeepsMoreTyping", enterClearsInputAndSentPromptKeepsMoreTyping)
	t.Run("enterShowsPromptBandAtOnceAndTranscriptPromptTakesItsPlace", enterShowsPromptBandAtOnceAndTranscriptPromptTakesItsPlace)
	t.Run("enterGivesSlashCommandEarlyBandThatItsCommandEntryReplaces", enterGivesSlashCommandEarlyBandThatItsCommandEntryReplaces)
	t.Run("failedSendRemovesEarlyBandAndShowsRefusal", failedSendRemovesEarlyBandAndShowsRefusal)
	t.Run("shiftEnterStartsNewLineAndEnterSendsTextWithItsNewlines", shiftEnterStartsNewLineAndEnterSendsTextWithItsNewlines)
	t.Run("ctrlEnterSendsTypedPromptThenSendNowKeys", ctrlEnterSendsTypedPromptThenSendNowKeys)
	t.Run("ctrlEnterWithEmptyInputSendsOnlySendNowKeys", ctrlEnterWithEmptyInputSendsOnlySendNowKeys)
	t.Run("ctrlEnterWithRefusedPromptSendsNoKeys", ctrlEnterWithRefusedPromptSendsNoKeys)
}

func assertEnterClearsInputAndSendResultLeavesInputAndStatus(t *testing.T, claudeScreenKey string, sendErr error, typedSince string,
	wantInput string, wantShown string) {
	t.Helper()
	model := newZenModel(nil, testAppearance, claudeScreenKey, "ctrl+e", "")
	model.input.SetValue("keep me\nand this line")
	sent, _ := model.withKey(pressedKey(tea.KeyEnter))
	sentModel := sent.(zenModel)
	inputAfterEnter := sentModel.input.Value()
	require.Empty(t, inputAfterEnter)
	sentModel.input.SetValue(typedSince)
	updated, _ := sentModel.Update(promptSentMsg{prompt: "keep me\nand this line", err: sendErr})
	updatedModel := updated.(zenModel)
	input := updatedModel.input.Value()
	require.Equal(t, wantInput, input)
	statusText := ansi.Strip(updatedModel.status)
	require.Equal(t, wantShown, statusText)
	require.Nil(t, updatedModel.fatal)
}

func enterClearsInputAndSendRefusedWhileBlockedPutsTextBack(t *testing.T) {
	assertEnterClearsInputAndSendResultLeavesInputAndStatus(t, "alt+x", ErrAgentBlocked, "", "keep me\nand this line",
		"Claude is waiting for you. Press alt+x to answer.")
}

func enterClearsInputAndSendRefusedWhileBlockedNamesConfiguredKey(t *testing.T) {
	assertEnterClearsInputAndSendResultLeavesInputAndStatus(t, "alt+z", ErrAgentBlocked, "", "keep me\nand this line",
		"Claude is waiting for you. Press alt+z to answer.")
}

func enterClearsInputAndHerdrFailurePutsTextBack(t *testing.T) {
	assertEnterClearsInputAndSendResultLeavesInputAndStatus(t, "alt+x", errors.New("herdr agent prompt failed: exit status 1"), "",
		"keep me\nand this line", "herdr agent prompt failed: exit status 1")
}

func enterClearsInputAndSendRefusedAfterMoreTypingPutsTextBeforeIt(t *testing.T) {
	assertEnterClearsInputAndSendResultLeavesInputAndStatus(t, "alt+x", ErrAgentBlocked, "more", "keep me\nand this line\nmore",
		"Claude is waiting for you. Press alt+x to answer.")
}

func enterClearsInputAndSentPromptLeavesItEmpty(t *testing.T) {
	assertEnterClearsInputAndSendResultLeavesInputAndStatus(t, "alt+x", nil, "", "", "")
}

func enterClearsInputAndSentPromptKeepsMoreTyping(t *testing.T) {
	assertEnterClearsInputAndSendResultLeavesInputAndStatus(t, "alt+x", nil, "more", "more", "")
}

func numberedPrompts(first int, count int) []claudecode.DialogueEntry {
	entries := []claudecode.DialogueEntry{}
	for number := first; number < first+count; number++ {
		entries = append(entries, claudecode.DialogueEntry{Kind: claudecode.PromptEntry, Text: fmt.Sprintf("prompt %d", number)})
	}
	return entries
}

func pressedKey(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

func TestScrolling(t *testing.T) {
	t.Run("screenScrolledUpStaysOnItsRowsWhenEntriesArriveAndEndReturnsToNewest",
		screenScrolledUpStaysOnItsRowsWhenEntriesArriveAndEndReturnsToNewest)
	t.Run("screenScrolledPastOldestEntryStopsThereAndOnePgDnReturnsToNewest", screenScrolledPastOldestEntryStopsThereAndOnePgDnReturnsToNewest)
	t.Run("upScrollsBackThreeRowsAndDownReturns", upScrollsBackThreeRowsAndDownReturns)
	t.Run("screenScrolledUpStaysOnItsRowsWhenEarlyBandComesAndGoes", screenScrolledUpStaysOnItsRowsWhenEarlyBandComesAndGoes)
	t.Run("screenScrolledUpStaysOnItsRowsWhenQueuedNoteRowComesAndGoes", screenScrolledUpStaysOnItsRowsWhenQueuedNoteRowComesAndGoes)
	t.Run("screenScrolledUpStaysOnItsRowsWhenCancelledPromptIsDropped", screenScrolledUpStaysOnItsRowsWhenCancelledPromptIsDropped)
}

func screenScrolledUpStaysOnItsRowsWhenEntriesArriveAndEndReturnsToNewest(t *testing.T) {
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "")
	model = model.withSize(40, 12).withEntries(numberedPrompts(1, 20), nil)
	atBottom := model.screen()
	require.Contains(t, atBottom, "prompt 20")
	scrolled, _ := model.withKey(pressedKey(tea.KeyPgUp))
	scrolledModel := scrolled.(zenModel)
	before := scrolledModel.screen()
	require.NotContains(t, before, "prompt 20")
	scrolledModel = scrolledModel.withEntries(numberedPrompts(21, 3), nil)
	after := scrolledModel.screen()
	require.Equal(t, before, after)
	ended, _ := scrolledModel.withKey(pressedKey(tea.KeyEnd))
	endedScreen := ended.(zenModel).screen()
	require.Contains(t, endedScreen, "prompt 23")
}

func screenScrolledPastOldestEntryStopsThereAndOnePgDnReturnsToNewest(t *testing.T) {
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "")
	model = model.withSize(40, 12).withEntries(numberedPrompts(1, 3), nil)
	for range 4 {
		scrolled, _ := model.withKey(pressedKey(tea.KeyPgUp))
		model = scrolled.(zenModel)
	}
	pastOldest := model.screen()
	require.Contains(t, pastOldest, "prompt 1")
	paged, _ := model.withKey(pressedKey(tea.KeyPgDown))
	pagedScreen := paged.(zenModel).screen()
	require.Contains(t, pagedScreen, "prompt 3")
}

func upScrollsBackThreeRowsAndDownReturns(t *testing.T) {
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "")
	model = model.withSize(40, 12).withEntries(numberedPrompts(1, 20), nil)
	scrolledUp, _ := model.Update(pressedKey(tea.KeyUp))
	upModel := scrolledUp.(zenModel)
	require.Equal(t, arrowScrollRows, upModel.rowsBelowView)
	upScreen := upModel.screen()
	require.NotContains(t, upScreen, "prompt 20")
	scrolledDown, _ := upModel.Update(pressedKey(tea.KeyDown))
	rowsBelowView := scrolledDown.(zenModel).rowsBelowView
	require.Equal(t, 0, rowsBelowView)
}

func TestCopy(t *testing.T) {
	t.Run("ctrlYOpensCopyModeOnNewestBlockAndEnterCopiesItAndShowsCopiedUntilNextKey",
		ctrlYOpensCopyModeOnNewestBlockAndEnterCopiesItAndShowsCopiedUntilNextKey)
	t.Run("ctrlYSaysSoAndCopiesNothingWithNoCodeBlock", ctrlYSaysSoAndCopiesNothingWithNoCodeBlock)
	t.Run("agentStatusReplacesCopyStatus", agentStatusReplacesCopyStatus)
	t.Run("problemReplacesCopyStatus", problemReplacesCopyStatus)
	t.Run("refusedPromptReplacesCopyStatus", refusedPromptReplacesCopyStatus)
	t.Run("refusedSendNowKeysReplaceCopyStatus", refusedSendNowKeysReplaceCopyStatus)
	t.Run("untakenClaudeInputReplacesCopyStatus", untakenClaudeInputReplacesCopyStatus)
	t.Run("copyHintMovesToNewestCodeBlockWhenReplyBringsOne", copyHintMovesToNewestCodeBlockWhenReplyBringsOne)
	t.Run("upAndDownPickOlderAndNewerBlockAndStopAtOldestAndNewest", upAndDownPickOlderAndNewerBlockAndStopAtOldestAndNewest)
	t.Run("spaceMarksPickedBlockAfterItsLanguageAndSpaceAgainUnmarksIt", spaceMarksPickedBlockAfterItsLanguageAndSpaceAgainUnmarksIt)
	t.Run("enterCopiesMarkedBlocksOldestFirstJoinedByBlankLine", enterCopiesMarkedBlocksOldestFirstJoinedByBlankLine)
	t.Run("escLeavesCopyModeCopyingNothingAndSendsNoInterruptWhileClaudeWorks",
		escLeavesCopyModeCopyingNothingAndSendsNoInterruptWhileClaudeWorks)
	t.Run("typingInCopyModeDoesNotReachInput", typingInCopyModeDoesNotReachInput)
	t.Run("upAndDownScrollViewTheLeastThatShowsWholePickedBlock", upAndDownScrollViewTheLeastThatShowsWholePickedBlock)
	t.Run("pickingBlockTallerThanViewPutsItsTopRuleAtViewTop", pickingBlockTallerThanViewPutsItsTopRuleAtViewTop)
}

var ctrlY = tea.KeyPressMsg{Code: 'y', Mod: tea.ModCtrl}

// modelWithThreeCodeBlocks holds the blocks "one", then "two" and a function in a newer reply, under a prompt.
func modelWithThreeCodeBlocks(height int) zenModel {
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "")
	return model.withSize(60, height).withEntries([]claudecode.DialogueEntry{
		{Kind: claudecode.ReplyEntry, Text: "Old:\n\n```sh\none\n```"},
		{Kind: claudecode.ReplyEntry, Text: "New:\n\n```go\ntwo\n```\n\n```go\nfunc f() {\n\treturn \n}\n```\n\nDone."},
		{Kind: claudecode.PromptEntry, Text: "thanks\n\n```\nmine\n```"},
	}, nil)
}

func pressedKeys(t *testing.T, model zenModel, keys ...tea.KeyPressMsg) zenModel {
	t.Helper()
	for _, key := range keys {
		updated, _ := model.Update(key)
		model = updated.(zenModel)
	}
	return model
}

// pickedTopRules are the screen's top rules drawn with a heavy iris corner, without their colors.
func pickedTopRules(model zenModel) []string {
	rules := []string{}
	for _, row := range strings.Split(model.screen(), "\n") {
		if strings.HasPrefix(row, " "+pickedFrameRuleStyle.Render("┏")) {
			rules = append(rules, strings.TrimSpace(ansi.Strip(row)))
		}
	}
	return rules
}

func ctrlYOpensCopyModeOnNewestBlockAndEnterCopiesItAndShowsCopiedUntilNextKey(t *testing.T) {
	opened, openCmd := modelWithThreeCodeBlocks(30).Update(ctrlY)
	openedModel := opened.(zenModel)
	require.Nil(t, openCmd)
	picked := pickedTopRules(openedModel)
	require.Len(t, picked, 1)
	pickedIsNewestGoBlock := strings.HasPrefix(picked[0], "┏━ go ━")
	require.True(t, pickedIsNewestGoBlock, picked[0])
	require.Contains(t, picked[0], copyKeyHint)
	openedScreen := ansi.Strip(openedModel.screen())
	require.Contains(t, openedScreen, copyModeHint)
	copied, copyCmd := openedModel.Update(pressedKey(tea.KeyEnter))
	copiedModel := copied.(zenModel)
	require.NotNil(t, copyCmd)
	got := copyCmd()
	setClipboard := tea.SetClipboard("func f() {\n\treturn \n}")
	want := setClipboard()
	require.Equal(t, want, got)
	require.Nil(t, copiedModel.copyMode)
	pickedAfterCopy := pickedTopRules(copiedModel)
	require.Empty(t, pickedAfterCopy)
	copiedScreen := ansi.Strip(copiedModel.screen())
	require.Contains(t, copiedScreen, "Copied the code block.")
	typed, _ := copiedModel.Update(pressedKey('a'))
	typedModel := typed.(zenModel)
	typedScreen := ansi.Strip(typedModel.screen())
	require.NotContains(t, typedScreen, "Copied the code block.")
}

func ctrlYSaysSoAndCopiesNothingWithNoCodeBlock(t *testing.T) {
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "")
	model = model.withSize(60, 12).withEntries([]claudecode.DialogueEntry{
		{Kind: claudecode.PromptEntry, Text: "hello"}, {Kind: claudecode.ReplyEntry, Text: "Use `inline code` only."},
	}, nil)
	pressed, copyCmd := model.Update(ctrlY)
	require.Nil(t, copyCmd)
	pressedModel := pressed.(zenModel)
	require.Nil(t, pressedModel.copyMode)
	screen := ansi.Strip(pressedModel.screen())
	require.Contains(t, screen, "No code block to copy.")
}

func assertNewStatusReplacesCopyStatus(t *testing.T, statusMsg tea.Msg, wantShown string) {
	t.Helper()
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "")
	model = model.withSize(60, 12)
	copied, _ := model.Update(tea.KeyPressMsg{Code: 'y', Mod: tea.ModCtrl})
	updated, _ := copied.(zenModel).Update(statusMsg)
	updatedModel := updated.(zenModel)
	screen := updatedModel.screen()
	shown := ansi.Strip(screen)
	require.NotContains(t, shown, "No code block to copy.")
	require.Contains(t, shown, wantShown)
}

func agentStatusReplacesCopyStatus(t *testing.T) {
	assertNewStatusReplacesCopyStatus(t, agentStatusMsg{status: AgentWorking}, "Working...")
}

func problemReplacesCopyStatus(t *testing.T) {
	assertNewStatusReplacesCopyStatus(t, problemMsg{err: errors.New("herdr pane get failed")}, "herdr pane get failed")
}

func refusedPromptReplacesCopyStatus(t *testing.T) {
	assertNewStatusReplacesCopyStatus(t, promptSentMsg{err: ErrAgentBlocked}, "Claude is waiting for you. Press alt+x to answer.")
}

func refusedSendNowKeysReplaceCopyStatus(t *testing.T) {
	assertNewStatusReplacesCopyStatus(t, claudeKeysSentMsg{err: errors.New("herdr pane send-keys failed")}, "herdr pane send-keys failed")
}

func untakenClaudeInputReplacesCopyStatus(t *testing.T) {
	assertNewStatusReplacesCopyStatus(t, agentInputTakenMsg{err: errors.New("Claude's input in pane 3 still holds text after ctrl+u")},
		"still holds text after ctrl+u")
}

func copyHintMovesToNewestCodeBlockWhenReplyBringsOne(t *testing.T) {
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "")
	model = model.withSize(60, 40).withEntries([]claudecode.DialogueEntry{{Kind: claudecode.ReplyEntry, Text: "```sh\nfirst\n```"}}, nil)
	model = model.withEntries([]claudecode.DialogueEntry{
		{Kind: claudecode.PromptEntry, Text: "and then?"}, {Kind: claudecode.ReplyEntry, Text: "```go\nsecond\n```"},
	}, nil)
	hintedRules := []string{}
	for _, row := range strings.Split(ansi.Strip(model.screen()), "\n") {
		if strings.Contains(row, copyKeyHint) {
			hintedRules = append(hintedRules, strings.TrimSpace(row))
		}
	}
	require.Len(t, hintedRules, 1)
	hintIsOnGoBlock := strings.HasPrefix(hintedRules[0], "╭─ go ─")
	require.True(t, hintIsOnGoBlock, hintedRules[0])
}

func upAndDownPickOlderAndNewerBlockAndStopAtOldestAndNewest(t *testing.T) {
	model := pressedKeys(t, modelWithThreeCodeBlocks(40), ctrlY)
	model = pressedKeys(t, model, pressedKey(tea.KeyUp))
	require.Equal(t, 1, model.copyMode.picked)
	pickedAfterFirstUp := pickedTopRules(model)
	require.Len(t, pickedAfterFirstUp, 1)
	model = pressedKeys(t, model, pressedKey(tea.KeyUp))
	require.Equal(t, 0, model.copyMode.picked)
	pickedAfterSecondUp := pickedTopRules(model)
	require.Len(t, pickedAfterSecondUp, 1)
	model = pressedKeys(t, model, pressedKey(tea.KeyUp))
	require.Equal(t, 0, model.copyMode.picked)
	pickedAfterThirdUp := pickedTopRules(model)
	require.Len(t, pickedAfterThirdUp, 1)
	model = pressedKeys(t, model, pressedKey(tea.KeyDown))
	require.Equal(t, 1, model.copyMode.picked)
	pickedAfterFirstDown := pickedTopRules(model)
	require.Len(t, pickedAfterFirstDown, 1)
	model = pressedKeys(t, model, pressedKey(tea.KeyDown))
	require.Equal(t, 2, model.copyMode.picked)
	pickedAfterSecondDown := pickedTopRules(model)
	require.Len(t, pickedAfterSecondDown, 1)
	model = pressedKeys(t, model, pressedKey(tea.KeyDown))
	require.Equal(t, 2, model.copyMode.picked)
	pickedAfterThirdDown := pickedTopRules(model)
	require.Len(t, pickedAfterThirdDown, 1)
	oldest := pressedKeys(t, model, pressedKey(tea.KeyUp), pressedKey(tea.KeyUp))
	pickedOldest := pickedTopRules(oldest)
	require.Len(t, pickedOldest, 1)
	pickedIsOldestShBlock := strings.HasPrefix(pickedOldest[0], "┏━ sh ━")
	require.True(t, pickedIsOldestShBlock, pickedOldest[0])
}

func spaceMarksPickedBlockAfterItsLanguageAndSpaceAgainUnmarksIt(t *testing.T) {
	marked := pressedKeys(t, modelWithThreeCodeBlocks(40), ctrlY, pressedKey(tea.KeyUp), pressedKey(tea.KeySpace))
	picked := pickedTopRules(marked)
	require.Len(t, picked, 1)
	marksAfterLanguage := strings.HasPrefix(picked[0], "┏━ go · marked ━")
	require.True(t, marksAfterLanguage, picked[0])
	markedScreen := marked.screen()
	irisMarked := markedLabelStyle.Render(markedLabel)
	require.Contains(t, markedScreen, irisMarked)
	unmarked := pressedKeys(t, marked, pressedKey(tea.KeySpace))
	unmarkedScreen := ansi.Strip(unmarked.screen())
	require.NotContains(t, unmarkedScreen, markedLabel)
}

func enterCopiesMarkedBlocksOldestFirstJoinedByBlankLine(t *testing.T) {
	// marked newest first, and the pick left on a block that is not marked
	model := pressedKeys(t, modelWithThreeCodeBlocks(40), ctrlY, pressedKey(tea.KeySpace), pressedKey(tea.KeyUp),
		pressedKey(tea.KeyUp), pressedKey(tea.KeySpace), pressedKey(tea.KeyDown))
	copied, copyCmd := model.Update(pressedKey(tea.KeyEnter))
	require.NotNil(t, copyCmd)
	got := copyCmd()
	setClipboard := tea.SetClipboard("one\n\nfunc f() {\n\treturn \n}")
	want := setClipboard()
	require.Equal(t, want, got)
	copiedModel := copied.(zenModel)
	shown := ansi.Strip(copiedModel.screen())
	require.Contains(t, shown, "Copied 2 code blocks.")
	require.NotContains(t, shown, markedLabel)
}

func escLeavesCopyModeCopyingNothingAndSendsNoInterruptWhileClaudeWorks(t *testing.T) {
	recorder := &agentRecorder{}
	model := modelWithThreeCodeBlocks(40)
	model.agent, model.agentStatus = recorder, AgentWorking
	model = pressedKeys(t, model, ctrlY, pressedKey(tea.KeySpace))
	closed, escCmd := model.Update(pressedKey(tea.KeyEscape))
	require.Nil(t, escCmd)
	require.Nil(t, recorder.calls)
	closedModel := closed.(zenModel)
	shown := ansi.Strip(closedModel.screen())
	require.Nil(t, closedModel.copyMode)
	require.NotContains(t, shown, markedLabel)
	require.NotContains(t, shown, copyModeHint)
	require.NotContains(t, shown, "Copied")
	picked := pickedTopRules(closedModel)
	require.Empty(t, picked)
}

func typingInCopyModeDoesNotReachInput(t *testing.T) {
	model := modelWithThreeCodeBlocks(40)
	model.input.SetValue("draft")
	typed := pressedKeys(t, model, ctrlY, pressedKey('x'), pressedKey(tea.KeyBackspace))
	value := typed.input.Value()
	require.Equal(t, "draft", value)
	require.NotNil(t, typed.copyMode)
}

func upAndDownScrollViewTheLeastThatShowsWholePickedBlock(t *testing.T) {
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "")
	entries := numberedPrompts(1, 6)
	entries = append(entries, claudecode.DialogueEntry{Kind: claudecode.ReplyEntry, Text: "```sh\nold line 1\nold line 2\n```"})
	entries = append(entries, numberedPrompts(7, 12)...)
	entries = append(entries, claudecode.DialogueEntry{Kind: claudecode.ReplyEntry, Text: "```go\nnew\n```"})
	model = model.withSize(60, 14).withEntries(entries, nil)
	olderPicked := pressedKeys(t, model, ctrlY, pressedKey(tea.KeyUp))
	shownRows := strings.Split(entriesText(t, olderPicked), "\n")
	topRule := strings.TrimSpace(shownRows[0])
	startsOnPickedTopRule := strings.HasPrefix(topRule, "┏━ sh ━")
	require.True(t, startsOnPickedTopRule, topRule)
	require.Contains(t, shownRows[1], "old line 1")
	require.Contains(t, shownRows[2], "old line 2")
	shownBottomRule := strings.TrimSpace(shownRows[3])
	pickedBottomRuleFollows := strings.HasPrefix(shownBottomRule, "┗━")
	require.True(t, pickedBottomRuleFollows, shownBottomRule)
	newerPicked := pressedKeys(t, olderPicked, pressedKey(tea.KeyDown))
	newerText := entriesText(t, newerPicked)
	require.Contains(t, newerText, "┏━ go ━")
	newerRows := strings.Split(newerText, "\n")
	// the entries' last row, above the blank row and the copy mode hint
	lastEntryRow := strings.TrimSpace(newerRows[len(newerRows)-3])
	endsOnNewestBottomRule := strings.HasPrefix(lastEntryRow, "┗━")
	require.True(t, endsOnNewestBottomRule, newerText)
}

func pickingBlockTallerThanViewPutsItsTopRuleAtViewTop(t *testing.T) {
	lines := []string{}
	for number := 1; number <= 30; number++ {
		lines = append(lines, fmt.Sprintf("line %d", number))
	}
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "")
	model = model.withSize(60, 14).withEntries([]claudecode.DialogueEntry{
		{Kind: claudecode.ReplyEntry, Text: "```sh\n" + strings.Join(lines, "\n") + "\n```"},
		{Kind: claudecode.PromptEntry, Text: "and then?"}, {Kind: claudecode.ReplyEntry, Text: "```go\nnew\n```"},
	}, nil)
	picked := pressedKeys(t, model, ctrlY, pressedKey(tea.KeyUp))
	rows := strings.Split(entriesText(t, picked), "\n")
	topRule := strings.TrimSpace(rows[0])
	topRuleOnFirstRow := strings.HasPrefix(topRule, "┏━ sh ━")
	require.True(t, topRuleOnFirstRow, topRule)
	require.Contains(t, rows[1], "line 1")
}

func modelWithTypedPrompt(prompt string) zenModel {
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "")
	model = model.withSize(60, 20).withEntries([]claudecode.DialogueEntry{{Kind: claudecode.ReplyEntry, Text: "Ready."}}, nil)
	model.input.SetValue(prompt)
	return model
}

func enterShowsPromptBandAtOnceAndTranscriptPromptTakesItsPlace(t *testing.T) {
	sent, sendPrompt := modelWithTypedPrompt("rename the widget").withKey(pressedKey(tea.KeyEnter))
	require.NotNil(t, sendPrompt)
	screenText := entriesText(t, sent.(zenModel))
	require.Contains(t, screenText, "> rename the widget")
	delivered, _ := sent.Update(promptSentMsg{prompt: "rename the widget", err: nil})
	sentModel := delivered.(zenModel)
	// a reply written before Claude records the prompt goes above the early band
	withReply := sentModel.withEntries([]claudecode.DialogueEntry{{Kind: claudecode.ReplyEntry, Text: "Still on the last step."}}, nil)
	replyText := entriesText(t, withReply)
	replyAt := strings.Index(replyText, "Still on the last step.")
	bandAt := strings.Index(replyText, "> rename the widget")
	require.GreaterOrEqual(t, replyAt, 0, replyText)
	require.GreaterOrEqual(t, bandAt, 0, replyText)
	require.LessOrEqual(t, replyAt, bandAt, replyText)
	recorded := withReply.withEntries([]claudecode.DialogueEntry{{Kind: claudecode.PromptEntry, Text: "rename the widget"}}, nil)
	recordedText := ansi.Strip(recorded.screen())
	count := strings.Count(recordedText, "rename the widget")
	require.Equal(t, 1, count, recordedText)
	require.Empty(t, recorded.earlyPrompts)
}

func enterGivesSlashCommandEarlyBandThatItsCommandEntryReplaces(t *testing.T) {
	sent, sendPrompt := modelWithTypedPrompt("/effort high").withKey(pressedKey(tea.KeyEnter))
	require.NotNil(t, sendPrompt)
	sentModel := sent.(zenModel)
	screenText := entriesText(t, sentModel)
	require.Contains(t, screenText, "> /effort high")
	recorded := sentModel.withEntries([]claudecode.DialogueEntry{
		{Kind: claudecode.CommandEntry, Text: "/effort high"},
		{Kind: claudecode.CommandOutputEntry, Text: "Set effort level to high"},
	}, nil)
	recordedText := entriesText(t, recorded)
	count := strings.Count(recordedText, "/effort high")
	require.Equal(t, 1, count, recordedText)
	require.Contains(t, recordedText, "Set effort level to high")
	require.Empty(t, recorded.earlyPrompts)
}

func failedSendRemovesEarlyBandAndShowsRefusal(t *testing.T) {
	sent, _ := modelWithTypedPrompt("rename the widget").withKey(pressedKey(tea.KeyEnter))
	refused, _ := sent.(zenModel).Update(promptSentMsg{prompt: "rename the widget", err: ErrAgentBlocked})
	refusedModel := refused.(zenModel)
	screenText := entriesText(t, refusedModel)
	require.NotContains(t, screenText, "> rename the widget")
	require.Contains(t, screenText, "Claude is waiting for you. Press alt+x to answer.")
}

func screenScrolledUpStaysOnItsRowsWhenEarlyBandComesAndGoes(t *testing.T) {
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "")
	model = model.withSize(40, 12).withEntries(numberedPrompts(1, 20), nil)
	scrolled, _ := model.withKey(pressedKey(tea.KeyPgUp))
	scrolledModel := scrolled.(zenModel)
	before := scrolledModel.screen()
	scrolledModel.input.SetValue("prompt 21")
	sent, _ := scrolledModel.withKey(pressedKey(tea.KeyEnter))
	delivered, _ := sent.Update(promptSentMsg{prompt: "prompt 21", err: nil})
	sentModel := delivered.(zenModel)
	withEarlyBand := sentModel.screen()
	require.Equal(t, before, withEarlyBand)
	recorded := sentModel.withEntries(numberedPrompts(21, 1), nil)
	withTranscriptPrompt := recorded.screen()
	require.Equal(t, before, withTranscriptPrompt)
}

// inputRows are the screen rows under the separator.
func inputRows(t *testing.T, model zenModel) []string {
	t.Helper()
	_, rowsUnder := screenRowsAroundSeparator(t, model)
	return rowsUnder
}

// entriesText is the screen above the separator, where a band and the input's own "> " cannot be confused.
func entriesText(t *testing.T, model zenModel) string {
	t.Helper()
	rowsAbove, _ := screenRowsAroundSeparator(t, model)
	return strings.Join(rowsAbove, "\n")
}

func screenRowsAroundSeparator(t *testing.T, model zenModel) ([]string, []string) {
	t.Helper()
	screen := ansi.Strip(model.screen())
	rows := strings.Split(screen, "\n")
	for index, row := range rows {
		if strings.HasPrefix(row, "───") {
			return rows[:index], rows[index+1:]
		}
	}
	require.FailNow(t, "screen has no separator", screen)
	return nil, nil
}

func TestInput(t *testing.T) {
	t.Run("inputWrapsLongLineUnderItsPromptAtForty", inputWrapsLongLineUnderItsPromptAtForty)
	t.Run("inputWrapsLongLineUnderItsPromptAtNinety", inputWrapsLongLineUnderItsPromptAtNinety)
	t.Run("inputGrowsRowAtATimeUpToEightAndScreenKeepsItsHeight", inputGrowsRowAtATimeUpToEightAndScreenKeepsItsHeight)
	t.Run("upAndDownMoveCursorWithinThreeLineInputAndScrollEntriesFromItsEdgeRows",
		upAndDownMoveCursorWithinThreeLineInputAndScrollEntriesFromItsEdgeRows)
	t.Run("upAndDownMoveCursorWithinLineAndWrappedLineAndScrollEntriesFromItsEdgeRows",
		upAndDownMoveCursorWithinLineAndWrappedLineAndScrollEntriesFromItsEdgeRows)
	t.Run("upAndDownScrollEntriesFromOneLineInput", upAndDownScrollEntriesFromOneLineInput)
	t.Run("entriesFillRowsInputGivesBack", entriesFillRowsInputGivesBack)
}

func assertInputWrapsLongLineUnderItsPrompt(t *testing.T, width int) {
	t.Helper()
	words := strings.Repeat("wrap these words ", 12)
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "")
	model = model.withSize(width, 30)
	model.input.SetValue(words)
	rows := inputRows(t, model)
	rowCount := len(rows)
	require.GreaterOrEqual(t, rowCount, 2, rows)
	startsWithPrompt := strings.HasPrefix(rows[0], " > wrap")
	require.True(t, startsWithPrompt, rows[0])
	joined := ""
	for index, row := range rows {
		rowWidth := ansi.StringWidth(row)
		require.LessOrEqual(t, rowWidth, width, row)
		if index > 0 {
			indentedUnderText := strings.HasPrefix(row, "   ")
			require.True(t, indentedUnderText, row)
		}
		joined += strings.TrimSpace(strings.TrimPrefix(row, " > ")) + " "
	}
	held := strings.Join(strings.Fields(joined), " ")
	wantHeld := strings.TrimSpace(words)
	require.Equal(t, wantHeld, held)
}

func inputWrapsLongLineUnderItsPromptAtForty(t *testing.T) {
	assertInputWrapsLongLineUnderItsPrompt(t, 40)
}

func inputWrapsLongLineUnderItsPromptAtNinety(t *testing.T) {
	assertInputWrapsLongLineUnderItsPrompt(t, 90)
}

func modelWithInputLines(model zenModel, lineCount int) zenModel {
	lines := strings.Repeat("line\n", lineCount)
	model.input.SetValue(strings.TrimSuffix(lines, "\n"))
	return model.withVisibleRowsDrawn()
}

func assertInputRowsAndScreenHeight(t *testing.T, model zenModel, wantRows int) {
	t.Helper()
	rows := inputRows(t, model)
	require.Len(t, rows, wantRows)
	screen := model.screen()
	newlines := strings.Count(screen, "\n")
	screenRows := newlines + 1
	require.Equal(t, 24, screenRows)
}

func inputGrowsRowAtATimeUpToEightAndScreenKeepsItsHeight(t *testing.T) {
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "")
	model = model.withSize(60, 24).withEntries(numberedPrompts(1, 20), nil)
	model = modelWithInputLines(model, 1)
	assertInputRowsAndScreenHeight(t, model, 1)
	model = modelWithInputLines(model, 2)
	assertInputRowsAndScreenHeight(t, model, 2)
	model = modelWithInputLines(model, 3)
	assertInputRowsAndScreenHeight(t, model, 3)
	model = modelWithInputLines(model, 4)
	assertInputRowsAndScreenHeight(t, model, 4)
	model = modelWithInputLines(model, 5)
	assertInputRowsAndScreenHeight(t, model, 5)
	model = modelWithInputLines(model, 6)
	assertInputRowsAndScreenHeight(t, model, 6)
	model = modelWithInputLines(model, 7)
	assertInputRowsAndScreenHeight(t, model, 7)
	model = modelWithInputLines(model, 8)
	assertInputRowsAndScreenHeight(t, model, 8)
	model = modelWithInputLines(model, 9)
	assertInputRowsAndScreenHeight(t, model, 8)
	model = modelWithInputLines(model, 10)
	assertInputRowsAndScreenHeight(t, model, 8)
	model = modelWithInputLines(model, 11)
	assertInputRowsAndScreenHeight(t, model, 8)
	value := model.input.Value()
	lineCount := strings.Count(value, "line")
	require.Equal(t, 11, lineCount)
}

func shiftEnterStartsNewLineAndEnterSendsTextWithItsNewlines(t *testing.T) {
	model := newZenModel(&agentRecorder{}, testAppearance, "alt+x", "ctrl+e", "")
	model = model.withSize(60, 24)
	shiftEnter := tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift}
	for _, keyPress := range []tea.KeyPressMsg{{Code: 'o', Text: "o"}, {Code: 'n', Text: "n"}, {Code: 'e', Text: "e"},
		shiftEnter, {Code: 't', Text: "t"}, {Code: 'w', Text: "w"}, {Code: 'o', Text: "o"}} {
		updated, _ := model.withKey(keyPress)
		model = updated.(zenModel)
	}
	value := model.input.Value()
	require.Equal(t, "one\ntwo", value)
	_, sendPrompt := model.withKey(pressedKey(tea.KeyEnter))
	msg := sendPrompt()
	require.IsType(t, promptSentMsg{}, msg)
	sent := msg.(promptSentMsg)
	require.Equal(t, "one\ntwo", sent.prompt)
}

func assertUpAndDownMoveCursorWithinInputAndScrollEntriesFromItsEdgeRows(t *testing.T, inputValue string, wantInputRows int) {
	t.Helper()
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "")
	model = model.withSize(40, 16)
	model.input.SetValue(inputValue)
	model = model.withEntries(numberedPrompts(1, 20), nil)
	rows := inputRows(t, model)
	require.Len(t, rows, wantInputRows)
	for press := 1; press <= wantInputRows; press++ {
		pressed, _ := model.withKey(pressedKey(tea.KeyUp))
		model = pressed.(zenModel)
		wantRowsBelowView := 0
		if press == wantInputRows {
			wantRowsBelowView = arrowScrollRows
		}
		require.Equal(t, wantRowsBelowView, model.rowsBelowView, "Up %d", press)
	}
	for press := 1; press <= wantInputRows; press++ {
		pressed, _ := model.withKey(pressedKey(tea.KeyDown))
		model = pressed.(zenModel)
		wantRowsBelowView := arrowScrollRows
		if press == wantInputRows {
			wantRowsBelowView = 0
		}
		require.Equal(t, wantRowsBelowView, model.rowsBelowView, "Down %d", press)
	}
	value := model.input.Value()
	require.Equal(t, inputValue, value)
}

func upAndDownMoveCursorWithinThreeLineInputAndScrollEntriesFromItsEdgeRows(t *testing.T) {
	assertUpAndDownMoveCursorWithinInputAndScrollEntriesFromItsEdgeRows(t, "first\nsecond\nthird", 3)
}

func upAndDownMoveCursorWithinLineAndWrappedLineAndScrollEntriesFromItsEdgeRows(t *testing.T) {
	assertUpAndDownMoveCursorWithinInputAndScrollEntriesFromItsEdgeRows(t, "first\n"+strings.Repeat("wrapped words ", 4), 3)
}

func upAndDownScrollEntriesFromOneLineInput(t *testing.T) {
	assertUpAndDownMoveCursorWithinInputAndScrollEntriesFromItsEdgeRows(t, "only", 1)
}

// modelWithInputThenEntries sets the input before the entries arrive, so they are drawn under its final height.
func modelWithInputThenEntries(inputValue string) zenModel {
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "")
	model = model.withSize(60, 24)
	model.input.SetValue(inputValue)
	return model.withEntries(numberedPrompts(1, 20), nil)
}

func entriesFillRowsInputGivesBack(t *testing.T) {
	model := newZenModel(&agentRecorder{}, testAppearance, "alt+x", "ctrl+e", "")
	model = model.withSize(60, 24)
	// Entries that arrive under a tall input are drawn only down to its top row.
	model.input.SetValue("x" + strings.Repeat("\nx", inputMaxRows-1))
	model = model.withEntries(numberedPrompts(1, 20), nil)
	for range 8 {
		erased, _ := model.withKey(pressedKey(tea.KeyBackspace))
		model = erased.(zenModel)
	}
	screen := model.screen()
	shrunk := modelWithInputThenEntries(model.input.Value())
	wantShrunk := shrunk.screen()
	require.Equal(t, wantShrunk, screen)
	prompt := model.input.Value()
	sent, _ := model.withKey(pressedKey(tea.KeyEnter))
	want := modelWithInputThenEntries("").withEarlyPromptAdded(earlyPrompt{text: prompt, pastes: nil, queued: false})
	wantSent := want.screen()
	sentScreen := sent.(zenModel).screen()
	require.Equal(t, wantSent, sentScreen)
}

// agentRecorder records each call as "prompt <text>", "send now", "interrupt" or "take input".
type agentRecorder struct {
	calls      []string
	promptErr  error
	typedInput string
	// pickerShown stands for a picker or dialog in place of Claude's input box.
	pickerShown bool
	inputBoxErr error
}

func (r *agentRecorder) SendPrompt(prompt string) error {
	r.calls = append(r.calls, "prompt "+prompt)
	return r.promptErr
}

func (r *agentRecorder) SendNow() error {
	r.calls = append(r.calls, "send now")
	return nil
}

func (r *agentRecorder) Interrupt() error {
	r.calls = append(r.calls, "interrupt")
	return nil
}

func (r *agentRecorder) TakeInput() (string, error) {
	r.calls = append(r.calls, "take input")
	return r.typedInput, nil
}

func (r *agentRecorder) Status() (AgentStatus, error) {
	return AgentUnknown, nil
}

func (r *agentRecorder) WaitForSessionID() (string, error) {
	return "", nil
}

func (r *agentRecorder) InputBoxShown() (bool, error) {
	return !r.pickerShown, r.inputBoxErr
}

func (r *agentRecorder) WorkingDirectory() (string, error) {
	return "", nil
}

func modelWithSentPrompts(prompts []string) zenModel {
	model := modelWithTypedPrompt("")
	for _, prompt := range prompts {
		model.input.SetValue(prompt)
		sent, _ := model.withKey(pressedKey(tea.KeyEnter))
		model = sent.(zenModel)
	}
	return model
}

func bandTextRow(t *testing.T, model zenModel, text string) string {
	t.Helper()
	for _, row := range strings.Split(entriesText(t, model), "\n") {
		if strings.Contains(row, "> "+text) {
			return row
		}
	}
	return ""
}

func rowUnderBand(t *testing.T, model zenModel, text string) string {
	t.Helper()
	rows := strings.Split(entriesText(t, model), "\n")
	for index, row := range rows[:len(rows)-1] {
		if strings.Contains(row, "> "+text) {
			return strings.TrimRight(rows[index+1], " ")
		}
	}
	return ""
}

func TestQueuedPrompts(t *testing.T) {
	t.Run("enqueueAddsNoteRowUnderBandWithItsTextAndMatchingRemoveDropsIt", enqueueAddsNoteRowUnderBandWithItsTextAndMatchingRemoveDropsIt)
	t.Run("dequeueDropsNoteRowOfOldestMarkedBand", dequeueDropsNoteRowOfOldestMarkedBand)
	t.Run("removeClearedOnCancelRemovesItsBand", removeClearedOnCancelRemovesItsBand)
	t.Run("removeNotCarriedToBackgroundRemovesItsBand", removeNotCarriedToBackgroundRemovesItsBand)
	t.Run("removeDroppedByHookRemovesQueuedBand", removeDroppedByHookRemovesQueuedBand)
	t.Run("enqueueBeforePromptEntryLeavesEarlyPromptUnqueued", enqueueBeforePromptEntryLeavesEarlyPromptUnqueued)
	t.Run("enqueueAfterPromptEntryMarksEarlyPromptQueued", enqueueAfterPromptEntryMarksEarlyPromptQueued)
}

func enqueueAddsNoteRowUnderBandWithItsTextAndMatchingRemoveDropsIt(t *testing.T) {
	model := modelWithSentPrompts([]string{"first one", "second one"})
	queued := model.withEntries(nil, []placedQueueOperation{{QueueOperation: claudecode.QueueOperation{Operation: "enqueue", Text: "second one"}}})
	rowUnderQueued := rowUnderBand(t, queued, "second one")
	require.Equal(t, "   "+queuedNote, rowUnderQueued)
	rowUnderOther := rowUnderBand(t, queued, "first one")
	require.NotContains(t, rowUnderOther, "queued")
	taken := queued.withEntries(nil, []placedQueueOperation{
		{QueueOperation: claudecode.QueueOperation{Operation: "remove", Reason: "absorbed_mid_turn", Text: "second one"}},
	})
	takenBand := bandTextRow(t, taken, "second one")
	require.NotEmpty(t, takenBand)
	rowUnderTaken := rowUnderBand(t, taken, "second one")
	require.NotContains(t, rowUnderTaken, "queued")
}

func dequeueDropsNoteRowOfOldestMarkedBand(t *testing.T) {
	model := modelWithSentPrompts([]string{"first one", "second one"}).withEntries(nil, []placedQueueOperation{
		{QueueOperation: claudecode.QueueOperation{Operation: "enqueue", Text: "first one"}},
		{QueueOperation: claudecode.QueueOperation{Operation: "enqueue", Text: "second one"}},
		{QueueOperation: claudecode.QueueOperation{Operation: "dequeue"}},
	})
	rowUnderOldest := rowUnderBand(t, model, "first one")
	require.NotContains(t, rowUnderOldest, "queued")
	rowUnderNewer := rowUnderBand(t, model, "second one")
	require.Equal(t, "   "+queuedNote, rowUnderNewer)
}

func assertRemoveDropsQueuedBand(t *testing.T, removal claudecode.QueueOperation) {
	t.Helper()
	model := modelWithSentPrompts([]string{"first one", "second one"}).withEntries(nil, []placedQueueOperation{
		{QueueOperation: claudecode.QueueOperation{Operation: "enqueue", Text: "second one"}}, {QueueOperation: removal},
	})
	removedBand := bandTextRow(t, model, "second one")
	require.Empty(t, removedBand)
	keptBand := bandTextRow(t, model, "first one")
	require.NotEmpty(t, keptBand)
}

func removeClearedOnCancelRemovesItsBand(t *testing.T) {
	assertRemoveDropsQueuedBand(t, claudecode.QueueOperation{Operation: "remove", Reason: "cleared_on_cancel", Text: "second one"})
}

func removeNotCarriedToBackgroundRemovesItsBand(t *testing.T) {
	assertRemoveDropsQueuedBand(t, claudecode.QueueOperation{Operation: "remove", Reason: "not_carried_to_background", Text: "second one"})
}

func removeDroppedByHookRemovesQueuedBand(t *testing.T) {
	assertRemoveDropsQueuedBand(t, claudecode.QueueOperation{Operation: "remove", Reason: "dropped_by_hook"})
}

func earlyPromptsAfterEnqueueAmongEntries(entriesBefore int) []earlyPrompt {
	model := modelWithSentPrompts([]string{"again", "again"}).withEntries(
		[]claudecode.DialogueEntry{{Kind: claudecode.PromptEntry, Text: "again"}},
		[]placedQueueOperation{
			{QueueOperation: claudecode.QueueOperation{Operation: "enqueue", Text: "again"}, entriesBefore: entriesBefore},
		})
	return model.earlyPrompts
}

func enqueueBeforePromptEntryLeavesEarlyPromptUnqueued(t *testing.T) {
	earlyPrompts := earlyPromptsAfterEnqueueAmongEntries(0)
	require.Equal(t, []earlyPrompt{{text: "again", queued: false}}, earlyPrompts)
}

func enqueueAfterPromptEntryMarksEarlyPromptQueued(t *testing.T) {
	earlyPrompts := earlyPromptsAfterEnqueueAmongEntries(1)
	require.Equal(t, []earlyPrompt{{text: "again", queued: true}}, earlyPrompts)
}

func screenScrolledUpStaysOnItsRowsWhenQueuedNoteRowComesAndGoes(t *testing.T) {
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "")
	model = model.withSize(40, 12).withEntries(numberedPrompts(1, 20), nil)
	prompt := "prompt 21"
	model.input.SetValue(prompt)
	sent, _ := model.withKey(pressedKey(tea.KeyEnter))
	scrolled, _ := sent.(zenModel).withKey(pressedKey(tea.KeyPgUp))
	scrolledModel := scrolled.(zenModel)
	before := scrolledModel.screen()
	queued := scrolledModel.withEntries(nil, []placedQueueOperation{{QueueOperation: claudecode.QueueOperation{Operation: "enqueue", Text: prompt}}})
	queuedRows := queued.earlyPromptRows()
	unqueuedRows := scrolledModel.earlyPromptRows()
	unqueuedRowCount := len(unqueuedRows)
	queuedRowCount := len(queuedRows)
	require.NotEqual(t, unqueuedRowCount, queuedRowCount)
	afterMark := queued.screen()
	require.Equal(t, before, afterMark)
	taken := queued.withEntries(nil, []placedQueueOperation{{QueueOperation: claudecode.QueueOperation{Operation: "dequeue"}}})
	afterClear := taken.screen()
	require.Equal(t, before, afterClear)
}

func assertCtrlEnterEmptiesInputAndSends(t *testing.T, typed string, promptErr error, wantCalls []string, wantMsg tea.Msg) {
	t.Helper()
	recorder := &agentRecorder{promptErr: promptErr}
	model := modelWithTypedPrompt(typed)
	model.agent = recorder
	pressed, send := model.withKey(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModCtrl})
	require.NotNil(t, send)
	value := pressed.(zenModel).input.Value()
	require.Empty(t, value)
	msg := send()
	require.Equal(t, wantMsg, msg)
	require.Equal(t, wantCalls, recorder.calls)
}

func ctrlEnterSendsTypedPromptThenSendNowKeys(t *testing.T) {
	assertCtrlEnterEmptiesInputAndSends(t, "ship it", nil, []string{"prompt ship it", "send now"}, claudeKeysSentMsg{})
}

func ctrlEnterWithEmptyInputSendsOnlySendNowKeys(t *testing.T) {
	assertCtrlEnterEmptiesInputAndSends(t, "", nil, []string{"send now"}, claudeKeysSentMsg{})
}

func ctrlEnterWithRefusedPromptSendsNoKeys(t *testing.T) {
	assertCtrlEnterEmptiesInputAndSends(t, "ship it", ErrAgentBlocked, []string{"prompt ship it"},
		promptSentMsg{prompt: "ship it", err: ErrAgentBlocked})
}

func TestEsc(t *testing.T) {
	t.Run("escWhileClaudeWorksSendsEscAndKeepsInput", escWhileClaudeWorksSendsEscAndKeepsInput)
	t.Run("escWhileClaudeIsIdleSendsNothingAndEmptiesInput", escWhileClaudeIsIdleSendsNothingAndEmptiesInput)
	t.Run("escWhileClaudeIsBlockedSendsNothingAndEmptiesInput", escWhileClaudeIsBlockedSendsNothingAndEmptiesInput)
	t.Run("escWhileClaudeIsNotWorkingEmptiesInputLabelsAndPastes", escWhileClaudeIsNotWorkingEmptiesInputLabelsAndPastes)
	t.Run("escBeforeReplyMovesPromptClaudePutBackIntoZenInput", escBeforeReplyMovesPromptClaudePutBackIntoZenInput)
	t.Run("promptPutBackAheadOfExpandedTextLeavesItPlain", promptPutBackAheadOfExpandedTextLeavesItPlain)
	t.Run("cancelledPromptRestoreKeepsEntriesWhenNewestIsNotThatPrompt", cancelledPromptRestoreKeepsEntriesWhenNewestIsNotThatPrompt)
	t.Run("cancelledPromptRestoreChangesNothingWhenClaudeGaveNoPromptBack", cancelledPromptRestoreChangesNothingWhenClaudeGaveNoPromptBack)
}

func modelAfterEsc(t *testing.T, agentStatus AgentStatus) (zenModel, *agentRecorder) {
	t.Helper()
	recorder := &agentRecorder{}
	model := modelWithTypedPrompt("half typed")
	model.agent, model.agentStatus = recorder, agentStatus
	pressed, send := model.withKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if send != nil {
		msg := send()
		require.Equal(t, claudeKeysSentMsg{}, msg)
	}
	return pressed.(zenModel), recorder
}

func escWhileClaudeWorksSendsEscAndKeepsInput(t *testing.T) {
	pressed, recorder := modelAfterEsc(t, AgentWorking)
	require.Equal(t, []string{"interrupt"}, recorder.calls)
	value := pressed.input.Value()
	require.Equal(t, "half typed", value)
}

func assertEscSendsNothingAndEmptiesInput(t *testing.T, agentStatus AgentStatus) {
	t.Helper()
	pressed, recorder := modelAfterEsc(t, agentStatus)
	require.Nil(t, recorder.calls)
	value := pressed.input.Value()
	require.Empty(t, value)
}

func escWhileClaudeIsIdleSendsNothingAndEmptiesInput(t *testing.T) {
	assertEscSendsNothingAndEmptiesInput(t, AgentIdle)
}

func escWhileClaudeIsBlockedSendsNothingAndEmptiesInput(t *testing.T) {
	assertEscSendsNothingAndEmptiesInput(t, AgentBlocked)
}

func escWhileClaudeIsNotWorkingEmptiesInputLabelsAndPastes(t *testing.T) {
	model := pressedKeys(t, modelWithTwoPastes(t, "ctrl+e", len("look at ")), ctrlE)
	model.agentStatus = AgentIdle
	cleared := pressedKeys(t, model, pressedKey(tea.KeyEscape))
	assertInputAndPastes(t, cleared, "", nil)
	require.Nil(t, cleared.expandedPastes)
	pasted, _ := cleared.Update(tea.PasteMsg{Content: fourLinePaste})
	assertInputAndPastes(t, pasted.(zenModel), "[Pasted text #1 +3 lines]", []string{firstPaste})
}

func escBeforeReplyMovesPromptClaudePutBackIntoZenInput(t *testing.T) {
	recorder := &agentRecorder{typedInput: "move this draft"}
	model := modelWithTypedPrompt("typed since")
	model = model.withEntries([]claudecode.DialogueEntry{{Kind: claudecode.PromptEntry, Text: "move this draft"}}, nil)
	model.agent, model.agentStatus = recorder, AgentWorking
	pressed, sendEsc := model.withKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	sendEsc()
	stopped, takeInput := pressed.(zenModel).Update(agentStatusMsg{status: AgentDone})
	require.NotNil(t, takeInput)
	taken := takeInput()
	require.Equal(t, agentInputTakenMsg{text: "move this draft"}, taken)
	require.Equal(t, []string{"interrupt", "take input"}, recorder.calls)
	restored, _ := stopped.(zenModel).Update(taken)
	restoredModel := restored.(zenModel)
	value := restoredModel.input.Value()
	require.Equal(t, "move this draft\ntyped since", value)
	screenText := entriesText(t, restoredModel)
	require.NotContains(t, screenText, "> move this draft")
	_, again := restoredModel.Update(agentStatusMsg{status: AgentIdle})
	require.Nil(t, again)
}

func promptPutBackAheadOfExpandedTextLeavesItPlain(t *testing.T) {
	expanded := pressedKeys(t, modelWithTwoPastes(t, "ctrl+e", len("look at ")), ctrlE)
	restored := expanded.withPromptPutBack("move this draft", nil)
	restored.input.MoveToBegin()
	restored.input.CursorDown()
	restored.input.SetCursorColumn(len("look at line"))
	pressed := pressedKeys(t, restored, ctrlE)
	assertInputAndPastes(t, pressed, "move this draft\n"+firstExpandedPrompt, []string{"", secondPaste})
}

func cancelledPromptRestoreKeepsEntriesWhenNewestIsNotThatPrompt(t *testing.T) {
	model := modelWithTypedPrompt("")
	model = model.withEntries([]claudecode.DialogueEntry{{Kind: claudecode.PromptEntry, Text: "move this draft"},
		{Kind: claudecode.ReplyEntry, Text: "Moved."}}, nil)
	restored := model.withCancelledPromptRestored("move this draft")
	entryCount := len(model.entries)
	require.Len(t, restored.entries, entryCount)
	entryRowCount := len(model.entryRows)
	require.Len(t, restored.entryRows, entryRowCount)
	value := restored.input.Value()
	require.Equal(t, "move this draft", value)
}

func cancelledPromptRestoreChangesNothingWhenClaudeGaveNoPromptBack(t *testing.T) {
	model := modelWithTypedPrompt("typed since")
	model = model.withEntries([]claudecode.DialogueEntry{{Kind: claudecode.PromptEntry, Text: "move this draft"}}, nil)
	restored := model.withCancelledPromptRestored("")
	entryCount := len(model.entries)
	require.Len(t, restored.entries, entryCount)
	value := restored.input.Value()
	require.Equal(t, "typed since", value)
}

func screenScrolledUpStaysOnItsRowsWhenCancelledPromptIsDropped(t *testing.T) {
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "")
	model = model.withSize(40, 12).withEntries(numberedPrompts(1, 20), nil)
	scrolled, _ := model.withKey(pressedKey(tea.KeyPgUp))
	scrolledModel := scrolled.(zenModel)
	before := scrolledModel.screen()
	restored := scrolledModel.withCancelledPromptRestored("prompt 20")
	require.Len(t, restored.entries, 19)
	beforeEntryRows := strings.Join(strings.Split(before, "\n")[:6], "\n")
	afterEntryRows := strings.Join(strings.Split(restored.screen(), "\n")[:6], "\n")
	require.Equal(t, beforeEntryRows, afterEntryRows)
}

func followTranscriptSendsQueueOperationsInTheirPlaceAmongEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	content := `{"type":"user","message":{"role":"user","content":"first"}}` + "\n" +
		`{"type":"queue-operation","operation":"enqueue","content":"second"}` + "\n" +
		`{"type":"user","message":{"role":"user","content":"third"}}` + "\n"
	writeErr := os.WriteFile(path, []byte(content), 0o600)
	require.NoError(t, writeErr)
	read := nextEntriesRead(t, followTranscriptMessages(t, path))
	want := []placedQueueOperation{{QueueOperation: claudecode.QueueOperation{Operation: "enqueue", Text: "second"}, entriesBefore: 1}}
	require.Equal(t, want, read.queueOperations)
}

func followTranscriptSendsRunningSubagentsOnChangeAndALastingProblemOnce(t *testing.T) {
	sessionDir := t.TempDir()
	subagentsDir := filepath.Join(sessionDir, "session", "subagents")
	mkdirErr := os.MkdirAll(subagentsDir, 0o700)
	require.NoError(t, mkdirErr)
	subagentFiles := map[string]string{
		"agent-a1.meta.json": `{"description":"Run the tests","toolUseId":"toolu_tests"}`,
		"agent-a1.jsonl": `{"type":"user","timestamp":"2026-11-20T10:00:00.000Z","message":{"role":"user","content":"x"}}` + "\n" +
			toolCallLine("Bash", `{"command":"go test ./...","description":"Running go test"}`) + "\n",
		"agent-a2.meta.json": `{"toolUseId":`,
	}
	for name, content := range subagentFiles {
		subagentErr := os.WriteFile(filepath.Join(subagentsDir, name), []byte(content), 0o600)
		require.NoError(t, subagentErr)
	}
	path := filepath.Join(sessionDir, "session.jsonl")
	content := agentToolCallLine("toolu_tests", "Run the tests") + "\n" + agentToolCallLine("toolu_broken", "Broken meta") + "\n"
	writeErr := os.WriteFile(path, []byte(content), 0o600)
	require.NoError(t, writeErr)
	messages := followTranscriptMessages(t, path)
	want := []claudecode.SubagentActivity{{Description: "Run the tests",
		StartTime: time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC), ToolCallActivity: "Running go test"}}
	read := nextSubagentsRead(t, messages)
	require.Equal(t, want, read.subagents)
	// the broken meta file fails each 200 ms poll for the whole second
	problems := 0
	for deadline := time.After(time.Second); problems >= 0; {
		select {
		case msg := <-messages:
			switch msg := msg.(type) {
			case problemMsg:
				problems++
			case subagentsReadMsg:
				require.FailNow(t, "subagents sent again unchanged", "%+v", msg.subagents)
			case fatalMsg:
				require.FailNow(t, "followTranscript returned", "%v", msg.err)
			}
		case <-deadline:
			require.Equal(t, 1, problems)
			problems = -1
		}
	}
	notification := `{"type":"queue-operation","operation":"enqueue","content":"<task-notification>\n` +
		`<tool-use-id>toolu_tests</tool-use-id>\n<status>completed</status>\n</task-notification>"}` + "\n"
	transcript, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	require.NoError(t, err)
	_, appendErr := transcript.WriteString(notification)
	require.NoError(t, appendErr)
	closeErr := transcript.Close()
	require.NoError(t, closeErr)
	afterNotification := nextSubagentsRead(t, messages)
	require.Empty(t, afterNotification.subagents)
}

func agentToolCallLine(toolUseID string, description string) string {
	return `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"` + toolUseID +
		`","name":"Agent","input":{"description":"` + description + `","prompt":"x"}}]}}`
}

func nextSubagentsRead(t *testing.T, messages chan tea.Msg) subagentsReadMsg {
	for {
		select {
		case msg := <-messages:
			switch msg := msg.(type) {
			case subagentsReadMsg:
				return msg
			case fatalMsg:
				require.FailNow(t, "followTranscript returned", "%v", msg.err)
			}
		case <-time.After(5 * time.Second):
			require.FailNow(t, "no subagents read after 5 s")
		}
	}
}

func followTranscriptMessages(t *testing.T, path string) chan tea.Msg {
	transcript, err := os.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() {
		closeErr := transcript.Close()
		require.NoError(t, closeErr)
	})
	messages := make(chan tea.Msg, 16)
	go func() {
		sameSession := func() (string, error) { return "session", nil }
		_, err := followTranscript(transcript, "session", sameSession, func(msg tea.Msg) { messages <- msg })
		messages <- fatalMsg{err: err}
	}()
	return messages
}

func nextEntriesRead(t *testing.T, messages chan tea.Msg) entriesReadMsg {
	for {
		select {
		case msg := <-messages:
			switch msg := msg.(type) {
			case entriesReadMsg:
				return msg
			case problemMsg:
				require.FailNow(t, "followTranscript reported", "%v", msg.err)
			case fatalMsg:
				require.FailNow(t, "followTranscript returned", "%v", msg.err)
			}
		case <-time.After(5 * time.Second):
			require.FailNow(t, "no entries read after 5 s")
		}
	}
}

func promptLine(timestamp string, prompt string) string {
	return `{"type":"user","timestamp":"` + timestamp + `","message":{"role":"user","content":"` + prompt + `"}}`
}

func toolCallLine(toolName string, input string) string {
	return `{"type":"assistant","timestamp":"2026-11-20T10:00:01.000Z","message":{"role":"assistant","content":[` +
		`{"type":"text","text":"On it."},{"type":"tool_use","id":"t1","name":"` + toolName + `","input":` + input + `}]}}`
}

func TestStatusRow(t *testing.T) {
	t.Run("workingRowNamesBashDescriptionAndTimeSinceItsPrompt", workingRowNamesBashDescriptionAndTimeSinceItsPrompt)
	t.Run("workingRowNamesBashWithoutDescriptionAsCommand", workingRowNamesBashWithoutDescriptionAsCommand)
	t.Run("workingRowNamesFileOfEdit", workingRowNamesFileOfEdit)
	t.Run("workingRowNamesFileOfWrite", workingRowNamesFileOfWrite)
	t.Run("workingRowNamesFileOfRead", workingRowNamesFileOfRead)
	t.Run("workingRowNamesPatternOfGrep", workingRowNamesPatternOfGrep)
	t.Run("workingRowNamesPatternOfGlob", workingRowNamesPatternOfGlob)
	t.Run("workingRowNamesDescriptionOfAgent", workingRowNamesDescriptionOfAgent)
	t.Run("workingRowShowsWorkingForUnknownNewestTool", workingRowShowsWorkingForUnknownNewestTool)
	t.Run("workingRowShowsWorkingBeforeFirstToolCall", workingRowShowsWorkingBeforeFirstToolCall)
	t.Run("workingRowIgnoresToolCallOfEarlierTurn", workingRowIgnoresToolCallOfEarlierTurn)
	t.Run("workingRowShowsNoTimeWithNoPromptInTail", workingRowShowsNoTimeWithNoPromptInTail)
	t.Run("workingRowKeepsTurnTimeAndToolCallWhenPromptTakenMidTurn", workingRowKeepsTurnTimeAndToolCallWhenPromptTakenMidTurn)
	t.Run("workingRowStartsTurnAtSlashCommand", workingRowStartsTurnAtSlashCommand)
	t.Run("workingRowCutsToolCallAtViewWidthAndKeepsTime", workingRowCutsToolCallAtViewWidthAndKeepsTime)
	t.Run("turnDurationShowsZeroSeconds", turnDurationShowsZeroSeconds)
	t.Run("turnDurationDropsFractionOfSecond", turnDurationDropsFractionOfSecond)
	t.Run("turnDurationShowsSecondsUnderAMinute", turnDurationShowsSecondsUnderAMinute)
	t.Run("turnDurationShowsMinuteAsMinutesThenZeroSeconds", turnDurationShowsMinuteAsMinutesThenZeroSeconds)
	t.Run("turnDurationShowsMinutesThenSecondsSpacedApart", turnDurationShowsMinutesThenSecondsSpacedApart)
	t.Run("turnDurationShowsSingleDigitSecondsAfterMinutes", turnDurationShowsSingleDigitSecondsAfterMinutes)
	t.Run("blockedAndIdleRowsShowNoToolCallOrTime", blockedAndIdleRowsShowNoToolCallOrTime)
	t.Run("subagentRowNamesRunningSubagentItsToolCallAndTimeSinceItStarted",
		subagentRowNamesRunningSubagentItsToolCallAndTimeSinceItStarted)
	t.Run("subagentRowNamesRunningSubagentAndTimeWithNoNamedToolCall", subagentRowNamesRunningSubagentAndTimeWithNoNamedToolCall)
	t.Run("idleRowStaysEmptyWithRunningSubagent", idleRowStaysEmptyWithRunningSubagent)
	t.Run("subagentRowNamesNewestOfThreeRunningSubagentsAndCountsOthers", subagentRowNamesNewestOfThreeRunningSubagentsAndCountsOthers)
	t.Run("subagentRowCutsDescriptionAndToolCallAndKeepsCountAndTime", subagentRowCutsDescriptionAndToolCallAndKeepsCountAndTime)
	t.Run("workingRowShowsTurnToolCallOnceNoSubagentRuns", workingRowShowsTurnToolCallOnceNoSubagentRuns)
	t.Run("subagentRowGoesWhenSessionChanges", subagentRowGoesWhenSessionChanges)
	t.Run("waitingLineWrapsAtNarrowWidthWithinScreenHeightWhenBlocked", waitingLineWrapsAtNarrowWidthWithinScreenHeightWhenBlocked)
	t.Run("waitingLineWrapsAtNarrowWidthWithinScreenHeightWhenPickerShows",
		waitingLineWrapsAtNarrowWidthWithinScreenHeightWhenPickerShows)
	t.Run("waitingLineShowsWhileClaudeWorksWithoutInputBox", waitingLineShowsWhileClaudeWorksWithoutInputBox)
	t.Run("workingRowShowsWhileClaudeWorksWithInputBox", workingRowShowsWhileClaudeWorksWithInputBox)
	t.Run("workingRowReturnsWhenInputBoxComesBack", workingRowReturnsWhenInputBoxComesBack)
	t.Run("waitingLineShowsWhileClaudeIsIdleWithoutInputBox", waitingLineShowsWhileClaudeIsIdleWithoutInputBox)
	t.Run("waitingLineShowsForDialogBeforeSessionHasTranscript", waitingLineShowsForDialogBeforeSessionHasTranscript)
	t.Run("pollReportsInputBoxGoneWhileClaudeIsIdleAndShownOnceClaudeIsBlocked",
		pollReportsInputBoxGoneWhileClaudeIsIdleAndShownOnceClaudeIsBlocked)
	t.Run("pollReportsInputBoxReadFailureWithoutReportingBoxGone", pollReportsInputBoxReadFailureWithoutReportingBoxGone)
	t.Run("clockTicksWhileWorkingAndWorkingRowClearsWhenTurnEnds", clockTicksWhileWorkingAndWorkingRowClearsWhenTurnEnds)
	t.Run("doneRowShowsTurnTimeAfterClaudeStopsWorkingUntilItWorksAgain", doneRowShowsTurnTimeAfterClaudeStopsWorkingUntilItWorksAgain)
	t.Run("doneRowStaysHiddenWhenEscInterruptedTurnOrClaudeWasNeverSeenWorking", doneRowStaysHiddenWhenEscInterruptedTurnOrClaudeWasNeverSeenWorking)
	t.Run("doneRowStaysHiddenAfterWorkThatRecordedNoNewTurn", doneRowStaysHiddenAfterWorkThatRecordedNoNewTurn)
	t.Run("doneRowShowsNewPromptTimeAfterWorkThatRecordedNoTurn", doneRowShowsNewPromptTimeAfterWorkThatRecordedNoTurn)
	t.Run("doneRowShowsTimeOfSlashCommandThatWritesRecord", doneRowShowsTimeOfSlashCommandThatWritesRecord)
	t.Run("doneRowStaysHiddenAfterSessionChangeWhenNewSessionRecordedNoTurn",
		doneRowStaysHiddenAfterSessionChangeWhenNewSessionRecordedNoTurn)
}

var workingRowPrompt = promptLine("2026-11-20T10:00:00.000Z", "go")

func assertWorkingRowAfterLines(t *testing.T, lines []string, wantRow string) {
	t.Helper()
	now := time.Date(2026, 11, 20, 10, 0, 42, 500_000_000, time.UTC)
	turn := turnAfterLines(t, claudecode.CurrentTurn{}, lines)
	drawnRow := statusLine(AgentWorking, turn, nil, now, 80, testAppearance, "alt+x")
	row := ansi.Strip(drawnRow)
	require.Equal(t, wantRow, row)
}

func workingRowNamesBashDescriptionAndTimeSinceItsPrompt(t *testing.T) {
	assertWorkingRowAfterLines(t, []string{workingRowPrompt,
		toolCallLine("Bash", `{"command":"go test ./cmd/zen/...","description":"Running go test ./cmd/zen/..."}`)},
		"Running go test ./cmd/zen/... · 42s")
}

func workingRowNamesBashWithoutDescriptionAsCommand(t *testing.T) {
	assertWorkingRowAfterLines(t, []string{workingRowPrompt, toolCallLine("Bash", `{"command":"ls"}`)}, "Running a command · 42s")
}

func workingRowNamesFileOfEdit(t *testing.T) {
	assertWorkingRowAfterLines(t, []string{workingRowPrompt, toolCallLine("Edit", `{"file_path":"C:\\dev\\zen\\cmd\\zen\\view.go"}`)},
		"Editing view.go · 42s")
}

func workingRowNamesFileOfWrite(t *testing.T) {
	assertWorkingRowAfterLines(t, []string{workingRowPrompt, toolCallLine("Write", `{"file_path":"C:/dev/zen/notes.md"}`)},
		"Editing notes.md · 42s")
}

func workingRowNamesFileOfRead(t *testing.T) {
	assertWorkingRowAfterLines(t, []string{workingRowPrompt, toolCallLine("Read", `{"file_path":"C:/dev/zen/go.mod"}`)},
		"Reading go.mod · 42s")
}

func workingRowNamesPatternOfGrep(t *testing.T) {
	assertWorkingRowAfterLines(t, []string{workingRowPrompt, toolCallLine("Grep", `{"pattern":"statusLine","path":"cmd"}`)},
		"Searching for statusLine · 42s")
}

func workingRowNamesPatternOfGlob(t *testing.T) {
	assertWorkingRowAfterLines(t, []string{workingRowPrompt, toolCallLine("Glob", `{"pattern":"**/*.go"}`)},
		"Searching for **/*.go · 42s")
}

func workingRowNamesDescriptionOfAgent(t *testing.T) {
	assertWorkingRowAfterLines(t, []string{workingRowPrompt,
		toolCallLine("Agent", `{"description":"Find the herdr calls","prompt":"x"}`)}, "Find the herdr calls · 42s")
}

func workingRowShowsWorkingForUnknownNewestTool(t *testing.T) {
	assertWorkingRowAfterLines(t, []string{workingRowPrompt, toolCallLine("Read", `{"file_path":"a.go"}`),
		toolCallLine("WebSearch", `{"query":"herdr"}`)}, "Working... · 42s")
}

func workingRowShowsWorkingBeforeFirstToolCall(t *testing.T) {
	assertWorkingRowAfterLines(t, []string{workingRowPrompt}, "Working... · 42s")
}

func workingRowIgnoresToolCallOfEarlierTurn(t *testing.T) {
	assertWorkingRowAfterLines(t, []string{promptLine("2026-11-20T09:00:00.000Z", "first"), toolCallLine("Bash", `{"command":"ls"}`),
		workingRowPrompt}, "Working... · 42s")
}

func workingRowShowsNoTimeWithNoPromptInTail(t *testing.T) {
	assertWorkingRowAfterLines(t, []string{toolCallLine("Read", `{"file_path":"a.go"}`)}, "Reading a.go")
}

func workingRowKeepsTurnTimeAndToolCallWhenPromptTakenMidTurn(t *testing.T) {
	assertWorkingRowAfterLines(t, []string{workingRowPrompt, toolCallLine("Read", `{"file_path":"a.go"}`),
		`{"type":"attachment","timestamp":"2026-11-20T10:00:30.000Z","attachment":{"type":"queued_command",` +
			`"prompt":"also check the logs","commandMode":"prompt","origin":{"kind":"human"}}}`},
		"Reading a.go · 42s")
}

func workingRowStartsTurnAtSlashCommand(t *testing.T) {
	assertWorkingRowAfterLines(t, []string{promptLine("2026-11-20T09:00:00.000Z", "first"), toolCallLine("Bash", `{"command":"ls"}`),
		promptLine("2026-11-20T10:00:00.000Z", `<command-name>/effort</command-name>\n<command-args>high</command-args>`)},
		"Working... · 42s")
}

func workingRowCutsToolCallAtViewWidthAndKeepsTime(t *testing.T) {
	promptTime := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	turn := claudecode.CurrentTurn{PromptTime: promptTime, ToolCallActivity: "Running a very long command description"}
	drawnRow := statusLine(AgentWorking, turn, nil, promptTime.Add(42*time.Second), 20, testAppearance, "alt+x")
	row := ansi.Strip(drawnRow)
	// the tool call cut to fit the time in the 19 columns after the margin
	require.Equal(t, "Running a ver · 42s", row)
}

func assertTurnDuration(t *testing.T, elapsed time.Duration, want string) {
	t.Helper()
	got := TurnDuration(elapsed)
	require.Equal(t, want, got)
}

func turnDurationShowsZeroSeconds(t *testing.T) {
	assertTurnDuration(t, 0, "0s")
}

func turnDurationDropsFractionOfSecond(t *testing.T) {
	assertTurnDuration(t, 42*time.Second+900*time.Millisecond, "42s")
}

func turnDurationShowsSecondsUnderAMinute(t *testing.T) {
	assertTurnDuration(t, 59*time.Second, "59s")
}

func turnDurationShowsMinuteAsMinutesThenZeroSeconds(t *testing.T) {
	assertTurnDuration(t, time.Minute, "1m 0s")
}

func turnDurationShowsMinutesThenSecondsSpacedApart(t *testing.T) {
	assertTurnDuration(t, time.Minute+11*time.Second, "1m 11s")
}

func turnDurationShowsSingleDigitSecondsAfterMinutes(t *testing.T) {
	assertTurnDuration(t, 3*time.Minute+5*time.Second, "3m 5s")
}

func blockedAndIdleRowsShowNoToolCallOrTime(t *testing.T) {
	turn := claudecode.CurrentTurn{PromptTime: time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC), ToolCallActivity: "Reading go.mod"}
	now := turn.PromptTime.Add(42 * time.Second)
	drawnBlockedRow := statusLine(AgentBlocked, turn, nil, now, 80, testAppearance, "alt+z")
	blockedRow := ansi.Strip(drawnBlockedRow)
	require.Equal(t, "Waiting for you in Claude. Press alt+z to answer.", blockedRow)
	idleRow := statusLine(AgentIdle, turn, nil, now, 80, testAppearance, "alt+x")
	require.Empty(t, idleRow)
}

var subagentStartTime = time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)

func assertSubagentRow(t *testing.T, subagent claudecode.SubagentActivity, wantRow string) {
	t.Helper()
	turn := claudecode.CurrentTurn{PromptTime: subagentStartTime.Add(-time.Minute), ToolCallActivity: "Running go test"}
	now := subagentStartTime.Add(42 * time.Second)
	subagents := []claudecode.SubagentActivity{subagent}
	drawnRow := statusLine(AgentWorking, turn, subagents, now, 80, testAppearance, "alt+x")
	row := ansi.Strip(drawnRow)
	require.Equal(t, wantRow, row)
}

func subagentRowNamesRunningSubagentItsToolCallAndTimeSinceItStarted(t *testing.T) {
	subagent := claudecode.SubagentActivity{Description: "Find the herdr calls", StartTime: subagentStartTime,
		ToolCallActivity: "Searching for herdr"}
	assertSubagentRow(t, subagent, "Find the herdr calls: Searching for herdr · 42s")
}

func subagentRowNamesRunningSubagentAndTimeWithNoNamedToolCall(t *testing.T) {
	subagent := claudecode.SubagentActivity{Description: "Find the herdr calls", StartTime: subagentStartTime}
	assertSubagentRow(t, subagent, "Find the herdr calls · 42s")
}

func idleRowStaysEmptyWithRunningSubagent(t *testing.T) {
	turn := claudecode.CurrentTurn{PromptTime: subagentStartTime.Add(-time.Minute), ToolCallActivity: "Running go test"}
	now := subagentStartTime.Add(42 * time.Second)
	subagents := []claudecode.SubagentActivity{{Description: "Find the herdr calls", StartTime: subagentStartTime,
		ToolCallActivity: "Searching for herdr"}}
	row := statusLine(AgentIdle, turn, subagents, now, 80, testAppearance, "alt+x")
	require.Empty(t, row)
}

func subagentRowNamesNewestOfThreeRunningSubagentsAndCountsOthers(t *testing.T) {
	newestStart := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	subagents := []claudecode.SubagentActivity{
		{Description: "Review the diff", StartTime: newestStart, ToolCallActivity: "Reading view.go"},
		{Description: "Find the herdr calls", StartTime: newestStart.Add(-time.Minute), ToolCallActivity: "Searching for herdr"},
		{Description: "Run the tests", StartTime: newestStart.Add(-2 * time.Minute), ToolCallActivity: "Running go test"},
	}
	drawnRow := statusLine(AgentWorking, claudecode.CurrentTurn{}, subagents, newestStart.Add(65*time.Second), 80,
		testAppearance, "alt+x")
	row := ansi.Strip(drawnRow)
	require.Equal(t, "Review the diff: Reading view.go +2 more · 1m 5s", row)
}

func subagentRowCutsDescriptionAndToolCallAndKeepsCountAndTime(t *testing.T) {
	startTime := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	subagents := []claudecode.SubagentActivity{
		{Description: "Review the diff", StartTime: startTime, ToolCallActivity: "Reading view.go"},
		{Description: "Run the tests", StartTime: startTime.Add(-time.Minute)},
	}
	drawnRow := statusLine(AgentWorking, claudecode.CurrentTurn{}, subagents, startTime.Add(42*time.Second), 30,
		testAppearance, "alt+x")
	row := ansi.Strip(drawnRow)
	// the description and tool call cut to fit the count and time in the 29 columns after the margin
	require.Equal(t, "Review the diff +1 more · 42s", row)
}

func workingRowShowsTurnToolCallOnceNoSubagentRuns(t *testing.T) {
	startTime := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "").withSize(80, 12)
	model.agentStatus = AgentWorking
	model.now = startTime.Add(42 * time.Second)
	model.turn = claudecode.CurrentTurn{PromptTime: startTime, ToolCallActivity: "Running go test"}
	running := []claudecode.SubagentActivity{{Description: "Review the diff", StartTime: startTime, ToolCallActivity: "Reading view.go"}}
	withSubagent, _ := model.Update(subagentsReadMsg{subagents: running})
	withSubagentModel := withSubagent.(zenModel)
	subagentRow := ansi.Strip(withSubagentModel.shownStatus())
	require.Equal(t, "Review the diff: Reading view.go · 42s", subagentRow)
	withoutSubagent, _ := withSubagent.Update(subagentsReadMsg{subagents: []claudecode.SubagentActivity{}})
	withoutSubagentModel := withoutSubagent.(zenModel)
	turnRow := ansi.Strip(withoutSubagentModel.shownStatus())
	require.Equal(t, "Running go test · 42s", turnRow)
}

func subagentRowGoesWhenSessionChanges(t *testing.T) {
	startTime := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "").withSize(80, 12)
	model.agentStatus = AgentWorking
	model.now = startTime.Add(42 * time.Second)
	running := []claudecode.SubagentActivity{{Description: "Review the diff", StartTime: startTime, ToolCallActivity: "Reading view.go"}}
	withSubagent, _ := model.Update(subagentsReadMsg{subagents: running})
	changed, _ := withSubagent.Update(sessionChangedMsg{})
	changedModel := changed.(zenModel)
	row := ansi.Strip(changedModel.shownStatus())
	require.Equal(t, "Working...", row)
}

func assertWaitingLineWrapsAtNarrowWidthWithinScreenHeight(t *testing.T, model zenModel) {
	t.Helper()
	screen := ansi.Strip(model.screen())
	rows := strings.Split(screen, "\n")
	require.Len(t, rows, 12)
	trimmedRows := []string{}
	for _, row := range rows {
		trimmedRows = append(trimmedRows, strings.TrimRight(row, " "))
	}
	want := []string{" Waiting for you in", " Claude. Press alt+x to", " answer."}
	aboveSeparator := trimmedRows[7:10]
	require.Equal(t, want, aboveSeparator, screen)
}

func waitingLineWrapsAtNarrowWidthWithinScreenHeightWhenBlocked(t *testing.T) {
	sized := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "").withSize(24, 12)
	blocked, _ := sized.Update(agentStatusMsg{status: AgentBlocked})
	assertWaitingLineWrapsAtNarrowWidthWithinScreenHeight(t, blocked.(zenModel))
}

func waitingLineWrapsAtNarrowWidthWithinScreenHeightWhenPickerShows(t *testing.T) {
	picker := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "").withSize(24, 12)
	picker.agentPickerShown = true
	assertWaitingLineWrapsAtNarrowWidthWithinScreenHeight(t, picker)
}

func workingModel(t *testing.T) zenModel {
	t.Helper()
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "").withSize(60, 12)
	promptTime := time.Now().Add(-42 * time.Second)
	read, _ := model.Update(entriesReadMsg{turn: claudecode.CurrentTurn{PromptTime: promptTime, ToolCallActivity: "Reading go.mod"}})
	working, _ := read.(zenModel).Update(agentStatusMsg{status: AgentWorking})
	return working.(zenModel)
}

func waitingLineShowsWhileClaudeWorksWithoutInputBox(t *testing.T) {
	boxGone, _ := workingModel(t).Update(agentInputBoxMsg{shown: false})
	boxGoneModel := boxGone.(zenModel)
	status := ansi.Strip(boxGoneModel.shownStatus())
	require.Equal(t, "Waiting for you in Claude. Press alt+x to answer.", status)
}

func workingRowShowsWhileClaudeWorksWithInputBox(t *testing.T) {
	boxShown, _ := workingModel(t).Update(agentInputBoxMsg{shown: true})
	boxShownModel := boxShown.(zenModel)
	status := ansi.Strip(boxShownModel.shownStatus())
	showsWorkingRow := strings.HasPrefix(status, "Reading go.mod · ")
	require.True(t, showsWorkingRow, status)
}

func workingRowReturnsWhenInputBoxComesBack(t *testing.T) {
	boxGone, _ := workingModel(t).Update(agentInputBoxMsg{shown: false})
	boxBack, _ := boxGone.(zenModel).Update(agentInputBoxMsg{shown: true})
	boxBackModel := boxBack.(zenModel)
	status := ansi.Strip(boxBackModel.shownStatus())
	showsWorkingRow := strings.HasPrefix(status, "Reading go.mod · ")
	require.True(t, showsWorkingRow, status)
}

func waitingLineShowsWhileClaudeIsIdleWithoutInputBox(t *testing.T) {
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "").withSize(60, 12)
	idle, _ := model.Update(agentStatusMsg{status: AgentIdle})
	boxGone, _ := idle.(zenModel).Update(agentInputBoxMsg{shown: false})
	boxGoneModel := boxGone.(zenModel)
	status := ansi.Strip(boxGoneModel.shownStatus())
	require.Equal(t, "Waiting for you in Claude. Press alt+x to answer.", status)
}

// transcriptlessSessionAgent reports a session that has no transcript yet, as Claude does before its first prompt.
type transcriptlessSessionAgent struct {
	*statusSwitchingAgent
}

func (a transcriptlessSessionAgent) WaitForSessionID() (string, error) {
	return "no-prompt-yet", nil
}

func waitingLineShowsForDialogBeforeSessionHasTranscript(t *testing.T) {
	t.Setenv("USERPROFILE", t.TempDir())
	agent := transcriptlessSessionAgent{&statusSwitchingAgent{agentRecorder: &agentRecorder{pickerShown: true}}}
	agent.status.Store(int32(AgentIdle))
	messages := make(chan tea.Msg, 64)
	go followAgentSession(agent, "", func(msg tea.Msg) { messages <- msg })
	model := newZenModel(agent, testAppearance, "alt+x", "ctrl+e", "").withSize(60, 12)
	modelAfterMessagesUntil(t, model, messages, func(model zenModel, _ tea.Msg) bool {
		return ansi.Strip(model.shownStatus()) == "Waiting for you in Claude. Press alt+x to answer."
	})
}

// statusSwitchingAgent reports the status a test stores, read by the poll's goroutine.
type statusSwitchingAgent struct {
	*agentRecorder
	status atomic.Int32
}

func (a *statusSwitchingAgent) Status() (AgentStatus, error) {
	return AgentStatus(a.status.Load()), nil
}

func assertPollSentInOrder(t *testing.T, sent chan tea.Msg, want []tea.Msg) {
	t.Helper()
	for _, wantMsg := range want {
		select {
		case msg := <-sent:
			require.Equal(t, wantMsg, msg)
		case <-time.After(5 * time.Second):
			require.FailNow(t, "poll sent nothing in 5s", "want %#v", wantMsg)
		}
	}
}

func pollReportsInputBoxGoneWhileClaudeIsIdleAndShownOnceClaudeIsBlocked(t *testing.T) {
	agent := &statusSwitchingAgent{agentRecorder: &agentRecorder{pickerShown: true}}
	agent.status.Store(int32(AgentIdle))
	sent := make(chan tea.Msg, 4)
	go pollAgentStatus(agent, func(msg tea.Msg) { sent <- msg })
	assertPollSentInOrder(t, sent, []tea.Msg{agentStatusMsg{status: AgentIdle}, agentInputBoxMsg{shown: false}})
	agent.status.Store(int32(AgentBlocked))
	assertPollSentInOrder(t, sent, []tea.Msg{agentStatusMsg{status: AgentBlocked}, agentInputBoxMsg{shown: true}})
}

func pollReportsInputBoxReadFailureWithoutReportingBoxGone(t *testing.T) {
	readErr := errors.New("pane read failed")
	sent := make(chan tea.Msg, 4)
	agent := &statusSwitchingAgent{agentRecorder: &agentRecorder{pickerShown: true, inputBoxErr: readErr}}
	agent.status.Store(int32(AgentWorking))
	go pollAgentStatus(agent, func(msg tea.Msg) { sent <- msg })
	assertPollSentInOrder(t, sent,
		[]tea.Msg{agentStatusMsg{status: AgentWorking}, problemMsg{err: readErr}, problemMsg{err: readErr}})
}

func clockTicksWhileWorkingAndWorkingRowClearsWhenTurnEnds(t *testing.T) {
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "")
	model = model.withSize(60, 12)
	promptTime := time.Now().Add(-42 * time.Second)
	read, _ := model.Update(entriesReadMsg{turn: claudecode.CurrentTurn{PromptTime: promptTime, ToolCallActivity: "Reading go.mod"}})
	started, firstTick := read.(zenModel).Update(agentStatusMsg{status: AgentWorking})
	require.NotNil(t, firstTick)
	msg := firstTick()
	require.IsType(t, clockTickMsg{}, msg)
	tick := msg.(clockTickMsg)
	startedModel := started.(zenModel)
	_, secondStart := startedModel.Update(agentStatusMsg{status: AgentWorking})
	require.Nil(t, secondStart)
	ticked, nextTick := startedModel.Update(tick)
	require.NotNil(t, nextTick)
	tickedModel := ticked.(zenModel)
	tickedScreen := ansi.Strip(tickedModel.screen())
	require.Contains(t, tickedScreen, " Reading go.mod · 42s")
	later, _ := tickedModel.Update(clockTickMsg{now: tick.now.Add(3 * time.Second)})
	laterModel := later.(zenModel)
	laterScreen := ansi.Strip(laterModel.screen())
	require.Contains(t, laterScreen, " Reading go.mod · 45s")
	idle, _ := laterModel.Update(agentStatusMsg{status: AgentIdle})
	idleModel := idle.(zenModel)
	idleScreen := ansi.Strip(idleModel.screen())
	require.NotContains(t, idleScreen, "Reading go.mod")
	stopped, afterIdle := idleModel.Update(clockTickMsg{now: tick.now.Add(4 * time.Second)})
	require.Nil(t, afterIdle)
	stoppedModel := stopped.(zenModel)
	require.False(t, stoppedModel.clockTicking)
}

func doneRowShowsTurnTimeAfterClaudeStopsWorkingUntilItWorksAgain(t *testing.T) {
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "")
	model = model.withSize(60, 12)
	promptTime := time.Now().Add(-time.Hour)
	read, _ := model.Update(entriesReadMsg{turn: claudecode.CurrentTurn{PromptTime: promptTime}})
	working, _ := read.(zenModel).Update(agentStatusMsg{status: AgentWorking})
	stopped, endTurn := working.(zenModel).Update(agentStatusMsg{status: AgentDone})
	require.NotNil(t, endTurn)
	msg := endTurn()
	require.IsType(t, turnEndedMsg{}, msg)
	stoppedModel := stopped.(zenModel)
	restarted, _ := stoppedModel.Update(agentStatusMsg{status: AgentWorking})
	late, _ := restarted.(zenModel).Update(turnEndedMsg{now: promptTime.Add(time.Minute)})
	lateModel := late.(zenModel)
	lateScreen := ansi.Strip(lateModel.screen())
	require.NotContains(t, lateScreen, "Done")
	ended, _ := stoppedModel.Update(turnEndedMsg{now: promptTime.Add(time.Minute + 11*time.Second)})
	endedModel := ended.(zenModel)
	endedScreen := ansi.Strip(endedModel.screen())
	require.Contains(t, endedScreen, " Done · 1m 11s")
	again, _ := endedModel.Update(agentStatusMsg{status: AgentWorking})
	againModel := again.(zenModel)
	againScreen := ansi.Strip(againModel.screen())
	require.NotContains(t, againScreen, "Done")
}

func doneRowStaysHiddenWhenEscInterruptedTurnOrClaudeWasNeverSeenWorking(t *testing.T) {
	recorder := &agentRecorder{}
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "")
	model = model.withSize(60, 12)
	model.agent, model.agentStatus = recorder, AgentWorking
	pressed, _ := model.withKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	_, takeInput := pressed.(zenModel).Update(agentStatusMsg{status: AgentDone})
	require.NotNil(t, takeInput)
	msg := takeInput()
	require.IsNotType(t, turnEndedMsg{}, msg)
	model.agentStatus = AgentIdle
	_, cmd := model.Update(agentStatusMsg{status: AgentDone})
	require.Nil(t, cmd)
}

var turnStart = time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)

func transcriptTimestamp(sinceTurnStart time.Duration) string {
	return turnStart.Add(sinceTurnStart).Format("2006-01-02T15:04:05.000Z")
}

func turnAfterLines(t *testing.T, turn claudecode.CurrentTurn, lines []string) claudecode.CurrentTurn {
	t.Helper()
	for _, line := range lines {
		lineEntries, err := claudecode.DialogueEntries([]byte(line))
		require.NoError(t, err, line)
		turn, err = claudecode.TurnAfterLine(turn, []byte(line), lineEntries)
		require.NoError(t, err, line)
	}
	return turn
}

func readLinesThenEndTurn(t *testing.T, model zenModel, lines []string, end time.Time) zenModel {
	t.Helper()
	read, _ := model.Update(entriesReadMsg{turn: turnAfterLines(t, model.turn, lines)})
	working, _ := read.(zenModel).Update(agentStatusMsg{status: AgentWorking})
	idle, _ := working.(zenModel).Update(agentStatusMsg{status: AgentIdle})
	ended, _ := idle.(zenModel).Update(turnEndedMsg{now: end})
	return ended.(zenModel)
}

func doneRowStaysHiddenAfterWorkThatRecordedNoNewTurn(t *testing.T) {
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "").withSize(60, 12)
	prompted := readLinesThenEndTurn(t, model, []string{promptLine(transcriptTimestamp(0), "go")},
		turnStart.Add(time.Minute+11*time.Second))
	promptedScreen := ansi.Strip(prompted.screen())
	require.Contains(t, promptedScreen, " Done · 1m 11s")
	dialog := readLinesThenEndTurn(t, prompted, nil, turnStart.Add(27*time.Minute))
	dialogScreen := ansi.Strip(dialog.screen())
	require.NotContains(t, dialogScreen, "Done")
}

func doneRowShowsNewPromptTimeAfterWorkThatRecordedNoTurn(t *testing.T) {
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "").withSize(60, 12)
	prompted := readLinesThenEndTurn(t, model, []string{promptLine(transcriptTimestamp(0), "go")}, turnStart.Add(time.Minute))
	dialog := readLinesThenEndTurn(t, prompted, nil, turnStart.Add(27*time.Minute))
	reprompted := readLinesThenEndTurn(t, dialog, []string{promptLine(transcriptTimestamp(30*time.Minute), "again")},
		turnStart.Add(30*time.Minute+5*time.Second))
	screen := ansi.Strip(reprompted.screen())
	require.Contains(t, screen, " Done · 5s")
}

func doneRowShowsTimeOfSlashCommandThatWritesRecord(t *testing.T) {
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "").withSize(60, 12)
	prompted := readLinesThenEndTurn(t, model, []string{promptLine(transcriptTimestamp(0), "go")}, turnStart.Add(time.Minute))
	effortCommand := promptLine(transcriptTimestamp(10*time.Minute),
		`<command-name>/effort</command-name>\n<command-args>high</command-args>`)
	commanded := readLinesThenEndTurn(t, prompted, []string{effortCommand}, turnStart.Add(10*time.Minute+3*time.Second))
	screen := ansi.Strip(commanded.screen())
	require.Contains(t, screen, " Done · 3s")
}

func doneRowStaysHiddenAfterSessionChangeWhenNewSessionRecordedNoTurn(t *testing.T) {
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "").withSize(60, 12)
	prompted := readLinesThenEndTurn(t, model, []string{promptLine(transcriptTimestamp(0), "go")}, turnStart.Add(time.Minute))
	changed, _ := prompted.Update(sessionChangedMsg{})
	cleared := readLinesThenEndTurn(t, changed.(zenModel), nil, turnStart.Add(2*time.Minute))
	screen := ansi.Strip(cleared.screen())
	require.NotContains(t, screen, "Done")
}

func followTranscriptSendsTurnOfLineWithOnlyToolCall(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	err := os.WriteFile(path, []byte(promptLine("2026-11-20T10:00:00.000Z", "go")+"\n"), 0o600)
	require.NoError(t, err)
	messages := followTranscriptMessages(t, path)
	nextEntriesRead(t, messages)
	transcript, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	require.NoError(t, err)
	_, writeErr := transcript.WriteString(`{"type":"assistant","timestamp":"2026-11-20T10:00:01.000Z","message":{` +
		`"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"C:/dev/go.mod"}}]}}` + "\n")
	require.NoError(t, writeErr)
	closeErr := transcript.Close()
	require.NoError(t, closeErr)
	read := nextEntriesRead(t, messages)
	want := claudecode.CurrentTurn{PromptTime: time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC), ToolCallActivity: "Reading go.mod"}
	require.Empty(t, read.entries)
	promptTimeMatches := read.turn.PromptTime.Equal(want.PromptTime)
	require.True(t, promptTimeMatches, read)
	require.Equal(t, want.ToolCallActivity, read.turn.ToolCallActivity)
}

func TestPaste(t *testing.T) {
	t.Run("longPasteShowsAsLabelAndEnterSendsWholeText", longPasteShowsAsLabelAndEnterSendsWholeText)
	t.Run("ctrlEnterSendsWholeTextAndRefusalCarriesPastes", ctrlEnterSendsWholeTextAndRefusalCarriesPastes)
	t.Run("shortPasteShowsAsItsText", shortPasteShowsAsItsText)
	t.Run("failedSendPutsPastesBackAsLabelsNumberedInOrder", failedSendPutsPastesBackAsLabelsNumberedInOrder)
	t.Run("transcriptPromptReplacingBandShowsBandsPastesAsLabels", transcriptPromptReplacingBandShowsBandsPastesAsLabels)
	t.Run("zenViewSavesDraftAsLabelsAndPastes", zenViewSavesDraftAsLabelsAndPastes)
	t.Run("restoredDraftShowsLabelsAndEnterSendsPastesExpanded", restoredDraftShowsLabelsAndEnterSendsPastesExpanded)
	t.Run("screenScrolledUpStaysOnItsRowsWhenPastedPromptReplacesBand", screenScrolledUpStaysOnItsRowsWhenPastedPromptReplacesBand)
	t.Run("expandKeyOnLabelTurnsOnlyThatLabelIntoItsTextWithCursorAfterIt",
		expandKeyOnLabelTurnsOnlyThatLabelIntoItsTextWithCursorAfterIt)
	t.Run("expandKeyDirectlyAfterLabelTurnsItIntoItsText", expandKeyDirectlyAfterLabelTurnsItIntoItsText)
	t.Run("expandKeyOnTypedTextAtInputStartChangesNothing", expandKeyOnTypedTextAtInputStartChangesNothing)
	t.Run("expandKeyOnTypedTextBeforeLabelChangesNothing", expandKeyOnTypedTextBeforeLabelChangesNothing)
	t.Run("expandKeyOnTypedTextBetweenLabelsChangesNothing", expandKeyOnTypedTextBetweenLabelsChangesNothing)
	t.Run("promptSentAfterExpandingShowsTextAndOtherLabelAndSendsBothPastes",
		promptSentAfterExpandingShowsTextAndOtherLabelAndSendsBothPastes)
	t.Run("draftAfterExpandingHoldsTextAndOtherLabel", draftAfterExpandingHoldsTextAndOtherLabel)
	t.Run("rebindExpandKeyLeavesCtrlEToTheInput", rebindExpandKeyLeavesCtrlEToTheInput)
	t.Run("expandKeyDirectlyAfterExpandedTextTurnsItBackIntoItsLabelWithCursorAfterIt",
		expandKeyDirectlyAfterExpandedTextTurnsItBackIntoItsLabelWithCursorAfterIt)
	t.Run("expandKeyAtStartOfExpandedTextTurnsItBackIntoItsLabelWithCursorAfterIt",
		expandKeyAtStartOfExpandedTextTurnsItBackIntoItsLabelWithCursorAfterIt)
	t.Run("expandKeyOnInnerLineOfExpandedTextTurnsItBackIntoItsLabelWithCursorAfterIt",
		expandKeyOnInnerLineOfExpandedTextTurnsItBackIntoItsLabelWithCursorAfterIt)
	t.Run("collapsingOneOfTwoExpandedPastesLeavesOtherExpandedAndCollapsible",
		collapsingOneOfTwoExpandedPastesLeavesOtherExpandedAndCollapsible)
	t.Run("expandKeyChangesNothingOnExpandedTextTypedInside", expandKeyChangesNothingOnExpandedTextTypedInside)
	t.Run("expandKeyChangesNothingOnExpandedTextWithLastCharacterDeleted",
		expandKeyChangesNothingOnExpandedTextWithLastCharacterDeleted)
	t.Run("expandKeyChangesNothingOnExpandedTextTypedInsideAndDeletedAgain",
		expandKeyChangesNothingOnExpandedTextTypedInsideAndDeletedAgain)
	t.Run("expandedTextStaysCollapsibleAfterEditsOutsideItAndCursorMoves",
		expandedTextStaysCollapsibleAfterEditsOutsideItAndCursorMoves)
	t.Run("pastesBeforeExpandedTextLeaveItCollapsibleAndPasteInsideMakesItPlain",
		pastesBeforeExpandedTextLeaveItCollapsibleAndPasteInsideMakesItPlain)
	t.Run("expandKeyPressedRepeatedlyAlternatesSameTextAndLabel", expandKeyPressedRepeatedlyAlternatesSameTextAndLabel)
	t.Run("promptSentAfterCollapsingSendsWholePaste", promptSentAfterCollapsingSendsWholePaste)
	t.Run("sendingExpandedTextLeavesNothingToCollapse", sendingExpandedTextLeavesNothingToCollapse)
}

const firstExpandedPrompt = "look at " + firstPaste + " and [Pasted text #2 +3 lines]"

func expandedFirstPaste(t *testing.T) zenModel {
	t.Helper()
	return pressedKeys(t, modelWithTwoPastes(t, "ctrl+e", len("look at ")), ctrlE)
}

func assertExpandKeyTurnsExpandedTextBackIntoItsLabelWithCursorAfterIt(t *testing.T, expanded zenModel) {
	t.Helper()
	collapsed := pressedKeys(t, expanded, ctrlE)
	assertInputAndPastes(t, collapsed, twoLabelsPrompt, []string{firstPaste, secondPaste})
	afterLabel := len("look at [Pasted text #1 +3 lines]")
	line := collapsed.input.Line()
	column := collapsed.input.Column()
	require.Equal(t, 0, line)
	require.Equal(t, afterLabel, column)
}

func expandKeyDirectlyAfterExpandedTextTurnsItBackIntoItsLabelWithCursorAfterIt(t *testing.T) {
	expanded := expandedFirstPaste(t)
	assertExpandKeyTurnsExpandedTextBackIntoItsLabelWithCursorAfterIt(t, expanded)
}

func expandKeyAtStartOfExpandedTextTurnsItBackIntoItsLabelWithCursorAfterIt(t *testing.T) {
	expanded := expandedFirstPaste(t)
	expanded.input.MoveToBegin()
	expanded.input.SetCursorColumn(len("look at "))
	assertExpandKeyTurnsExpandedTextBackIntoItsLabelWithCursorAfterIt(t, expanded)
}

func expandKeyOnInnerLineOfExpandedTextTurnsItBackIntoItsLabelWithCursorAfterIt(t *testing.T) {
	expanded := expandedFirstPaste(t)
	expanded.input.CursorUp()
	assertExpandKeyTurnsExpandedTextBackIntoItsLabelWithCursorAfterIt(t, expanded)
}

func collapsingOneOfTwoExpandedPastesLeavesOtherExpandedAndCollapsible(t *testing.T) {
	model := pressedKeys(t, modelWithTwoPastes(t, "ctrl+e", len(twoLabelsPrompt)), ctrlE)
	model.input.MoveToBegin()
	model.input.SetCursorColumn(len("look at "))
	bothExpanded := pressedKeys(t, model, ctrlE)
	assertInputAndPastes(t, bothExpanded, "look at "+firstPaste+" and "+secondPaste, []string{"", ""})
	secondAtEnd := bothExpanded
	secondAtEnd.input.MoveToEnd()
	secondCollapsed := pressedKeys(t, secondAtEnd, ctrlE)
	assertInputAndPastes(t, secondCollapsed, firstExpandedPrompt, []string{"", secondPaste})
	firstCollapsed := pressedKeys(t, bothExpanded, ctrlE)
	assertInputAndPastes(t, firstCollapsed, "look at [Pasted text #1 +3 lines] and "+secondPaste, []string{firstPaste, ""})
	firstCollapsed.input.MoveToEnd()
	bothCollapsed := pressedKeys(t, firstCollapsed, ctrlE)
	assertInputAndPastes(t, bothCollapsed, twoLabelsPrompt, []string{firstPaste, secondPaste})
}

func assertExpandKeyChangesNothingAfterEdits(t *testing.T, keys []tea.KeyPressMsg, wantEdited string) {
	t.Helper()
	expanded := expandedFirstPaste(t)
	edited := pressedKeys(t, expanded, keys...)
	assertInputAndPastes(t, edited, wantEdited, []string{"", secondPaste})
	pressed := pressedKeys(t, edited, ctrlE)
	assertInputAndPastes(t, pressed, wantEdited, []string{"", secondPaste})
}

func expandKeyChangesNothingOnExpandedTextTypedInside(t *testing.T) {
	assertExpandKeyChangesNothingAfterEdits(t, []tea.KeyPressMsg{pressedKey(tea.KeyLeft), typedKey('X')},
		"look at line one\nline two\nline three\nline fouXr and [Pasted text #2 +3 lines]")
}

func expandKeyChangesNothingOnExpandedTextWithLastCharacterDeleted(t *testing.T) {
	assertExpandKeyChangesNothingAfterEdits(t, []tea.KeyPressMsg{pressedKey(tea.KeyBackspace)},
		"look at line one\nline two\nline three\nline fou and [Pasted text #2 +3 lines]")
}

func expandKeyChangesNothingOnExpandedTextTypedInsideAndDeletedAgain(t *testing.T) {
	assertExpandKeyChangesNothingAfterEdits(t, []tea.KeyPressMsg{pressedKey(tea.KeyLeft), typedKey('X'),
		pressedKey(tea.KeyBackspace)}, firstExpandedPrompt)
}

func expandedTextStaysCollapsibleAfterEditsOutsideItAndCursorMoves(t *testing.T) {
	expanded := expandedFirstPaste(t)
	expanded.input.MoveToBegin()
	expanded.input.SetCursorColumn(len("look"))
	edited := pressedKeys(t, expanded, typedKey('s'), pressedKey(tea.KeyRight), pressedKey(tea.KeyLeft))
	edited.input.MoveToEnd()
	edited = pressedKeys(t, edited, typedKey('!'))
	edited.input.MoveToBegin()
	edited.input.SetCursorColumn(len("looks at line"))
	collapsed := pressedKeys(t, edited, ctrlE)
	assertInputAndPastes(t, collapsed, "looks at [Pasted text #1 +3 lines] and [Pasted text #2 +3 lines]!",
		[]string{firstPaste, secondPaste})
}

func pastesBeforeExpandedTextLeaveItCollapsibleAndPasteInsideMakesItPlain(t *testing.T) {
	before := expandedFirstPaste(t)
	before.input.MoveToBegin()
	shortPasted, _ := before.Update(tea.PasteMsg{Content: "hi,"})
	labelPasted, _ := shortPasted.(zenModel).Update(tea.PasteMsg{Content: fourLinePaste})
	pastedBefore := labelPasted.(zenModel)
	pastedBefore.input.MoveToBegin()
	pastedBefore.input.SetCursorColumn(len("hi,[Pasted text #3 +3 lines]look at l"))
	collapsed := pressedKeys(t, pastedBefore, ctrlE)
	assertInputAndPastes(t, collapsed, "hi,[Pasted text #3 +3 lines]"+twoLabelsPrompt, []string{firstPaste, secondPaste, firstPaste})
	inside := expandedFirstPaste(t)
	inside.input.CursorUp()
	insidePasted, _ := inside.Update(tea.PasteMsg{Content: "hi,"})
	pressed := pressedKeys(t, insidePasted.(zenModel), ctrlE)
	assertInputAndPastes(t, pressed, "look at line one\nline two\nline threhi,e\nline four and [Pasted text #2 +3 lines]",
		[]string{"", secondPaste})
}

func expandKeyPressedRepeatedlyAlternatesSameTextAndLabel(t *testing.T) {
	model := modelWithTwoPastes(t, "ctrl+e", len("look at "))
	firstPress := pressedKeys(t, model, ctrlE)
	assertInputAndPastes(t, firstPress, firstExpandedPrompt, []string{"", secondPaste})
	secondPress := pressedKeys(t, firstPress, ctrlE)
	assertInputAndPastes(t, secondPress, twoLabelsPrompt, []string{firstPaste, secondPaste})
	thirdPress := pressedKeys(t, secondPress, ctrlE)
	assertInputAndPastes(t, thirdPress, firstExpandedPrompt, []string{"", secondPaste})
	fourthPress := pressedKeys(t, thirdPress, ctrlE)
	assertInputAndPastes(t, fourthPress, twoLabelsPrompt, []string{firstPaste, secondPaste})
}

func promptSentAfterCollapsingSendsWholePaste(t *testing.T) {
	recorder := &agentRecorder{}
	model := pressedKeys(t, modelWithTwoPastes(t, "ctrl+e", len("look at ")), ctrlE, ctrlE)
	model.agent = recorder
	_, send := model.withKey(pressedKey(tea.KeyEnter))
	send()
	require.Equal(t, []string{"prompt look at " + firstPaste + " and " + secondPaste}, recorder.calls)
}

func sendingExpandedTextLeavesNothingToCollapse(t *testing.T) {
	model := expandedFirstPaste(t)
	model.agent = &agentRecorder{}
	sent, send := model.withKey(pressedKey(tea.KeyEnter))
	send()
	sentModel := sent.(zenModel)
	require.Nil(t, sentModel.expandedPastes)
}

const (
	firstPaste      = "line one\nline two\nline three\nline four"
	secondPaste     = "w\nx\ny\nz"
	twoLabelsPrompt = "look at [Pasted text #1 +3 lines] and [Pasted text #2 +3 lines]"
)

var ctrlE = tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl}

// modelWithTwoPastes holds twoLabelsPrompt with its cursor at column, and expands a label on expandPasteKey.
func modelWithTwoPastes(t *testing.T, expandPasteKey string, column int) zenModel {
	t.Helper()
	model := newZenModel(nil, testAppearance, "alt+x", expandPasteKey, "").withSize(60, 20)
	pasted, _ := model.Update(tea.PasteMsg{Content: fourLinePaste})
	model = pasted.(zenModel)
	model.input.InsertString(" and ")
	pasted, _ = model.Update(tea.PasteMsg{Content: secondPaste})
	model = pasted.(zenModel)
	model.input.MoveToBegin()
	model.input.InsertString("look at ")
	value := model.input.Value()
	require.Equal(t, twoLabelsPrompt, value)
	model.input.MoveToBegin()
	model.input.SetCursorColumn(column)
	return model
}

func assertInputAndPastes(t *testing.T, model zenModel, wantValue string, wantPastes []string) {
	t.Helper()
	value := model.input.Value()
	require.Equal(t, wantValue, value)
	require.Equal(t, wantPastes, model.pastes)
}

func expandKeyOnLabelTurnsOnlyThatLabelIntoItsTextWithCursorAfterIt(t *testing.T) {
	expanded := pressedKeys(t, modelWithTwoPastes(t, "ctrl+e", len("look at [Pas")), ctrlE)
	assertInputAndPastes(t, expanded, "look at "+firstPaste+" and [Pasted text #2 +3 lines]", []string{"", secondPaste})
	afterPaste := len("line four")
	line := expanded.input.Line()
	column := expanded.input.Column()
	require.Equal(t, 3, line)
	require.Equal(t, afterPaste, column)
}

func expandKeyDirectlyAfterLabelTurnsItIntoItsText(t *testing.T) {
	afterFirst := pressedKeys(t, modelWithTwoPastes(t, "ctrl+e", len("look at [Pasted text #1 +3 lines]")), ctrlE)
	assertInputAndPastes(t, afterFirst, "look at "+firstPaste+" and [Pasted text #2 +3 lines]", []string{"", secondPaste})
	afterSecond := pressedKeys(t, modelWithTwoPastes(t, "ctrl+e", len(twoLabelsPrompt)), ctrlE)
	assertInputAndPastes(t, afterSecond, "look at [Pasted text #1 +3 lines] and "+secondPaste, []string{firstPaste, ""})
}

func assertExpandKeyOnTypedTextChangesNothing(t *testing.T, column int) {
	t.Helper()
	model := modelWithTwoPastes(t, "ctrl+e", column)
	pressed := pressedKeys(t, model, ctrlE)
	assertInputAndPastes(t, pressed, twoLabelsPrompt, []string{firstPaste, secondPaste})
	cursorColumn := pressed.input.Column()
	require.Equal(t, column, cursorColumn)
}

func expandKeyOnTypedTextAtInputStartChangesNothing(t *testing.T) {
	assertExpandKeyOnTypedTextChangesNothing(t, 0)
}

func expandKeyOnTypedTextBeforeLabelChangesNothing(t *testing.T) {
	assertExpandKeyOnTypedTextChangesNothing(t, len("look a"))
}

func expandKeyOnTypedTextBetweenLabelsChangesNothing(t *testing.T) {
	assertExpandKeyOnTypedTextChangesNothing(t, len("look at [Pasted text #1 +3 lines] a"))
}

func promptSentAfterExpandingShowsTextAndOtherLabelAndSendsBothPastes(t *testing.T) {
	recorder := &agentRecorder{}
	model := expandedFirstPaste(t)
	model.agent = recorder
	sent, send := model.withKey(pressedKey(tea.KeyEnter))
	send()
	text := "look at " + firstPaste + " and " + secondPaste
	require.Equal(t, []string{"prompt " + text}, recorder.calls)
	sentModel := sent.(zenModel)
	bandText := entriesText(t, sentModel)
	require.Contains(t, bandText, "> look at line one")
	require.Contains(t, bandText, "line four and [Pasted text #2 +3 lines]")
	recorded := sentModel.withEntries([]claudecode.DialogueEntry{
		{Kind: claudecode.PromptEntry, Text: text, Pastes: []string{text}}}, nil)
	entryText := entriesText(t, recorded)
	require.Contains(t, entryText, "> look at line one")
	require.Contains(t, entryText, "line four and [Pasted text #2 +3 lines]")
}

func draftAfterExpandingHoldsTextAndOtherLabel(t *testing.T) {
	saved := Draft{}
	saveDraft := func(draft Draft) error {
		saved = draft
		return nil
	}
	var view tea.Model = draftSavingView{zen: modelWithTwoPastes(t, "ctrl+e", len("look at ")), saveDraft: saveDraft,
		savedDraft: Draft{}}
	view, _ = view.Update(ctrlE)
	want := Draft{Text: "look at " + firstPaste + " and [Pasted text #2 +3 lines]", Pastes: []string{"", secondPaste}}
	require.Equal(t, want, saved)
}

func rebindExpandKeyLeavesCtrlEToTheInput(t *testing.T) {
	model := modelWithTwoPastes(t, "ctrl+o", len("look at "))
	ctrlEPressed := pressedKeys(t, model, ctrlE)
	assertInputAndPastes(t, ctrlEPressed, twoLabelsPrompt, []string{firstPaste, secondPaste})
	expanded := pressedKeys(t, model, tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	assertInputAndPastes(t, expanded, "look at "+firstPaste+" and [Pasted text #2 +3 lines]", []string{"", secondPaste})
}

func screenScrolledUpStaysOnItsRowsWhenPastedPromptReplacesBand(t *testing.T) {
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "")
	model = model.withSize(40, 12).withEntries(numberedPrompts(1, 20), nil)
	scrolled, _ := model.withKey(pressedKey(tea.KeyPgUp))
	pasted, _ := scrolled.Update(tea.PasteMsg{Content: fourLinePaste})
	sent, _ := pasted.(zenModel).withKey(pressedKey(tea.KeyEnter))
	sentModel := sent.(zenModel)
	before := sentModel.screen()
	recorded := sentModel.withEntries([]claudecode.DialogueEntry{{Kind: claudecode.PromptEntry,
		Text: "line one\nline two\nline three\nline four"}}, nil)
	after := recorded.screen()
	require.Equal(t, before, after)
	newestRows := plainRows(recorded.entryRows[len(recorded.entryRows)-1])
	require.Equal(t, []string{"", " > [Pasted text #1 +3 lines]"}, newestRows)
}

const fourLinePaste = "line one\r\nline two\r\nline three\r\nline four\r\n"

func modelWithPaste(typed string, content string) zenModel {
	model := modelWithTypedPrompt(typed)
	model.input.CursorEnd()
	pasted, _ := model.Update(tea.PasteMsg{Content: content})
	return pasted.(zenModel)
}

func longPasteShowsAsLabelAndEnterSendsWholeText(t *testing.T) {
	recorder := &agentRecorder{}
	model := modelWithPaste("look at ", fourLinePaste)
	model.agent = recorder
	value := model.input.Value()
	require.Equal(t, "look at [Pasted text #1 +3 lines]", value)
	sent, send := model.withKey(pressedKey(tea.KeyEnter))
	send()
	require.Equal(t, []string{"prompt look at line one\nline two\nline three\nline four"}, recorder.calls)
	sentModel := sent.(zenModel)
	row := bandTextRow(t, sentModel, "look at [Pasted text #1 +3 lines]")
	screen := ansi.Strip(sentModel.screen())
	require.NotEmpty(t, row, screen)
}

func ctrlEnterSendsWholeTextAndRefusalCarriesPastes(t *testing.T) {
	recorder := &agentRecorder{promptErr: ErrAgentBlocked}
	model := modelWithPaste("look at ", fourLinePaste)
	model.agent = recorder
	_, send := model.withKey(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModCtrl})
	paste := "line one\nline two\nline three\nline four"
	want := promptSentMsg{prompt: "look at " + paste, pastes: []string{paste}, err: ErrAgentBlocked}
	msg := send()
	require.Equal(t, want, msg)
	require.Equal(t, []string{"prompt look at " + paste}, recorder.calls)
}

func shortPasteShowsAsItsText(t *testing.T) {
	model := modelWithPaste("look at ", "line one\nline two")
	value := model.input.Value()
	require.Equal(t, "look at line one\nline two", value)
}

func failedSendPutsPastesBackAsLabelsNumberedInOrder(t *testing.T) {
	model := modelWithPaste("look at ", fourLinePaste)
	sent, _ := model.withKey(pressedKey(tea.KeyEnter))
	sentModel := sent.(zenModel)
	pastedSince, _ := sentModel.Update(tea.PasteMsg{Content: "w\nx\ny\nz"})
	refused, _ := pastedSince.Update(promptSentMsg{prompt: "look at line one\nline two\nline three\nline four",
		pastes: []string{"line one\nline two\nline three\nline four"}, err: ErrAgentBlocked})
	refusedModel := refused.(zenModel)
	value := refusedModel.input.Value()
	require.Equal(t, "look at [Pasted text #1 +3 lines]\n[Pasted text #2 +3 lines]", value)
	inputText := refusedModel.InputText()
	require.Equal(t, "look at line one\nline two\nline three\nline four\nw\nx\ny\nz", inputText)
}

func transcriptPromptReplacingBandShowsBandsPastesAsLabels(t *testing.T) {
	sent, _ := modelWithPaste("look at ", fourLinePaste).withKey(pressedKey(tea.KeyEnter))
	// herdr sends the whole prompt as one paste, so Claude Code wraps all of it in one tag.
	text := "look at line one\nline two\nline three\nline four"
	recorded := sent.(zenModel).withEntries([]claudecode.DialogueEntry{
		{Kind: claudecode.PromptEntry, Text: text, Pastes: []string{text}}}, nil)
	screenText := entriesText(t, recorded)
	require.Contains(t, screenText, "> look at [Pasted text #1 +3 lines]")
	require.NotContains(t, screenText, "line two")
}

func zenViewSavesDraftAsLabelsAndPastes(t *testing.T) {
	saved := []Draft{}
	saveDraft := func(draft Draft) error {
		saved = append(saved, draft)
		return nil
	}
	var view tea.Model = draftSavingView{zen: newZenModel(nil, testAppearance, "alt+x", "ctrl+e", ""), saveDraft: saveDraft,
		savedDraft: Draft{}}
	view, _ = view.Update(tea.PasteMsg{Content: fourLinePaste})
	want := []Draft{{Text: "[Pasted text #1 +3 lines]", Pastes: []string{"line one\nline two\nline three\nline four"}}}
	require.Equal(t, want, saved)
}

func restoredDraftShowsLabelsAndEnterSendsPastesExpanded(t *testing.T) {
	recorder := &agentRecorder{}
	paste := "line one\nline two\nline three\nline four"
	model := newZenModel(recorder, testAppearance, "alt+x", "ctrl+e", "").withSize(60, 20).
		withDraftRestored(Draft{Text: "look at [Pasted text #1 +3 lines]", Pastes: []string{paste}}, nil)
	value := model.input.Value()
	require.Equal(t, "look at [Pasted text #1 +3 lines]", value)
	_, send := model.withKey(pressedKey(tea.KeyEnter))
	send()
	require.Equal(t, []string{"prompt look at " + paste}, recorder.calls)
}

func TestDraftSavingView(t *testing.T) {
	t.Run("zenViewSavesUnsentTextAsDraftOfItsClaudePane", zenViewSavesUnsentTextAsDraftOfItsClaudePane)
	t.Run("zenViewSavesNothingWhenNeitherShownTextNorPastesChanged", zenViewSavesNothingWhenNeitherShownTextNorPastesChanged)
	t.Run("draftReadErrorShowsOnStatusLineUntilNextKeyOrNewerError", draftReadErrorShowsOnStatusLineUntilNextKeyOrNewerError)
}

func zenViewSavesUnsentTextAsDraftOfItsClaudePane(t *testing.T) {
	draft := Draft{}
	saveDraft := func(saved Draft) error {
		draft = saved
		return nil
	}
	var view tea.Model = draftSavingView{zen: newZenModel(nil, testAppearance, "alt+x", "ctrl+e", ""), saveDraft: saveDraft,
		savedDraft: Draft{}}
	for _, keyPress := range []tea.KeyPressMsg{
		{Code: 'h', Text: "h"}, {Code: 'i', Text: "i"}, {Code: tea.KeyEnter, Mod: tea.ModShift}, {Code: 'x', Text: "x"},
	} {
		view, _ = view.Update(keyPress)
	}
	require.Equal(t, "hi\nx", draft.Text)
	require.Empty(t, draft.Pastes)
}

func zenViewSavesNothingWhenNeitherShownTextNorPastesChanged(t *testing.T) {
	saves := 0
	saveDraft := func(draft Draft) error {
		saves++
		return nil
	}
	draft := Draft{Text: "look at [Pasted text #1 +3 lines]", Pastes: []string{"line one\nline two\nline three\nline four"}}
	zen := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "").withSize(60, 20).withDraftRestored(draft, nil)
	var view tea.Model = draftSavingView{zen: zen, saveDraft: saveDraft, savedDraft: draft}
	for _, msg := range []tea.Msg{tea.WindowSizeMsg{Width: 70, Height: 20}, pressedKey(tea.KeyLeft), pressedKey(tea.KeyEnd)} {
		view, _ = view.Update(msg)
	}
	require.Equal(t, 0, saves)
}

func draftReadErrorShowsOnStatusLineUntilNextKeyOrNewerError(t *testing.T) {
	draftErr := errors.New("Cannot read the draft C:/state/draft.default.w1-p1: invalid character 'a'")
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "").withSize(60, 20).withDraftRestored(Draft{}, draftErr)
	shown := ansi.Strip(model.shownStatus())
	wantShown := draftErr.Error()
	require.Equal(t, wantShown, shown)
	value := model.input.Value()
	require.Empty(t, value)
	typed := pressedKeys(t, model, tea.KeyPressMsg{Code: 'h', Text: "h"})
	typedShown := ansi.Strip(typed.shownStatus())
	require.NotContains(t, typedShown, "Cannot read the draft")
	problem := errors.New("herdr pane get failed")
	updated, _ := model.Update(problemMsg{err: problem})
	updatedModel := updated.(zenModel)
	updatedShown := ansi.Strip(updatedModel.shownStatus())
	wantUpdatedShown := problem.Error()
	require.Equal(t, wantUpdatedShown, updatedShown)
}

func TestFatal(t *testing.T) {
	t.Run("fatalErrorWrapsAtViewWidthSoItsEndShows", fatalErrorWrapsAtViewWidthSoItsEndShows)
}

func fatalErrorWrapsAtViewWidthSoItsEndShows(t *testing.T) {
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "").withSize(30, 12)
	model.fatal = errors.New(`C:\Users\someone\AppData\Roaming\calmpane\config.toml: colors.link = "#9ccfd" is not a color`)
	rows := strings.Split(model.View().Content, "\n")
	for _, row := range rows {
		rowWidth := ansi.StringWidth(row)
		require.LessOrEqual(t, rowWidth, 30, row)
	}
	shown := strings.Join(plainRows(rows), " ")
	require.Contains(t, shown, `colors.link = "#9ccfd" is not a color`)
}

func TestSessionChange(t *testing.T) {
	t.Run("sessionChangeDropsEntriesAndFollowsNewTranscriptAndStatusLine",
		sessionChangeDropsEntriesAndFollowsNewTranscriptAndStatusLine)
	t.Run("followTranscriptReturnsReportedSessionOnceItDiffers", followTranscriptReturnsReportedSessionOnceItDiffers)
}

// switchingSessionAgent reports the session its test last set, as Claude's hooks do after /clear.
type switchingSessionAgent struct {
	agentRecorder
	sessionIDs chan string
	sessionID  string
}

func (a *switchingSessionAgent) WaitForSessionID() (string, error) {
	select {
	case sessionID := <-a.sessionIDs:
		a.sessionID = sessionID
	default:
	}
	return a.sessionID, nil
}

func writeTranscriptLine(t *testing.T, home string, sessionID string, line string) {
	t.Helper()
	path := filepath.Join(home, ".claude", "projects", "scratch", sessionID+".jsonl")
	mkdirErr := os.MkdirAll(filepath.Dir(path), 0o755)
	require.NoError(t, mkdirErr)
	writeErr := os.WriteFile(path, []byte(line+"\n"), 0o600)
	require.NoError(t, writeErr)
}

// modelAfterMessagesUntil applies the messages followAgentSession sends to model until done holds for the model a
// message gives and the message.
func modelAfterMessagesUntil(t *testing.T, model zenModel, messages <-chan tea.Msg,
	done func(model zenModel, msg tea.Msg) bool) zenModel {
	t.Helper()
	for {
		select {
		case msg := <-messages:
			require.IsNotType(t, fatalMsg{}, msg, "followAgentSession sent %#v", msg)
			updated, _ := model.Update(msg)
			model = updated.(zenModel)
			if done(model, msg) {
				return model
			}
		case <-time.After(10 * time.Second):
			screen := ansi.Strip(model.screen())
			require.FailNow(t, "the view never got there in 10 s", "the view shows:\n%s", screen)
		}
	}
}

func sessionChangeDropsEntriesAndFollowsNewTranscriptAndStatusLine(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	statusLineDir := t.TempDir()
	writeTranscriptLine(t, home, "first", `{"type":"user","message":{"role":"user","content":"before the clear"}}`)
	for sessionID, line := range map[string]string{"first": "first status\n", "second": "second status\n"} {
		writeErr := os.WriteFile(filepath.Join(statusLineDir, sessionID), []byte(line), 0o600)
		require.NoError(t, writeErr)
	}
	agent := &switchingSessionAgent{sessionIDs: make(chan string, 1), sessionID: "first"}
	messages := make(chan tea.Msg, 64)
	go followAgentSession(agent, statusLineDir, func(msg tea.Msg) { messages <- msg })
	model := newZenModel(agent, testAppearance, "alt+x", "ctrl+e", "").withSize(60, 20)
	showsBoth := func(model zenModel, prompt string, status string) bool {
		screen := ansi.Strip(model.screen())
		return strings.Contains(screen, prompt) && strings.Contains(screen, status)
	}
	model = modelAfterMessagesUntil(t, model, messages, func(model zenModel, _ tea.Msg) bool {
		return showsBoth(model, "before the clear", "first status")
	})
	writeTranscriptLine(t, home, "second", `{"type":"user","message":{"role":"user","content":"after the clear"}}`)
	agent.sessionIDs <- "second"
	model = modelAfterMessagesUntil(t, model, messages, sessionChanged)
	changedScreen := ansi.Strip(model.screen())
	require.NotContains(t, changedScreen, "before the clear")
	model = modelAfterMessagesUntil(t, model, messages, func(model zenModel, _ tea.Msg) bool {
		return showsBoth(model, "after the clear", "second status")
	})
	newSessionScreen := ansi.Strip(model.screen())
	require.NotContains(t, newSessionScreen, "before the clear")
	// a session with no transcript leaves the second one closed, so the temporary home can be removed
	agent.sessionIDs <- "none"
	modelAfterMessagesUntil(t, model, messages, sessionChanged)
}

func sessionChanged(_ zenModel, msg tea.Msg) bool {
	_, changed := msg.(sessionChangedMsg)
	return changed
}

func followTranscriptReturnsReportedSessionOnceItDiffers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	err := os.WriteFile(path, []byte(`{"type":"user","message":{"role":"user","content":"first"}}`+"\n"), 0o600)
	require.NoError(t, err)
	transcript, err := os.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() {
		closeErr := transcript.Close()
		require.NoError(t, closeErr)
	})
	newSession := func() (string, error) { return "after clear", nil }
	next, err := followTranscript(transcript, "before clear", newSession, func(tea.Msg) {})
	require.NoError(t, err)
	require.Equal(t, "after clear", next)
}

func TestAgentGone(t *testing.T) {
	t.Run("goneStatusQuitsView", goneStatusQuitsView)
	t.Run("pollSendsGoneOnceAndStops", pollSendsGoneOnceAndStops)
	t.Run("followAgentSessionSendsGoneWhenAgentExitsBeforeFirstPrompt",
		followAgentSessionSendsGoneWhenAgentExitsBeforeFirstPrompt)
	t.Run("followAgentSessionSendsGoneWhenSessionCheckFindsAgentGone",
		followAgentSessionSendsGoneWhenSessionCheckFindsAgentGone)
	t.Run("followAgentSessionSendsFatalWhenAgentIsGoneAtStart", followAgentSessionSendsFatalWhenAgentIsGoneAtStart)
}

func goneStatusQuitsView(t *testing.T) {
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "").withSize(60, 12)
	_, cmd := model.Update(agentStatusMsg{status: AgentGone})
	require.NotNil(t, cmd)
	msg := cmd()
	require.IsType(t, tea.QuitMsg{}, msg)
}

func pollSendsGoneOnceAndStops(t *testing.T) {
	agent := &statusSwitchingAgent{agentRecorder: &agentRecorder{}}
	agent.status.Store(int32(AgentGone))
	sent := make(chan tea.Msg, 4)
	stopped := make(chan struct{})
	go func() {
		pollAgentStatus(agent, func(msg tea.Msg) { sent <- msg })
		close(stopped)
	}()
	assertPollSentInOrder(t, sent, []tea.Msg{agentStatusMsg{status: AgentGone}})
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "poll still runs 5 s after the agent was gone")
	}
}

func followAgentSessionSendsGoneWhenAgentExitsBeforeFirstPrompt(t *testing.T) {
	t.Setenv("USERPROFILE", t.TempDir())
	agent := transcriptlessSessionAgent{&statusSwitchingAgent{agentRecorder: &agentRecorder{}}}
	agent.status.Store(int32(AgentIdle))
	messages := make(chan tea.Msg, 64)
	go followAgentSession(agent, "", func(msg tea.Msg) { messages <- msg })
	model := newZenModel(agent, testAppearance, "alt+x", "ctrl+e", "").withSize(60, 12)
	model = modelAfterMessagesUntil(t, model, messages, func(model zenModel, _ tea.Msg) bool {
		return model.agentStatus == AgentIdle
	})
	agent.status.Store(int32(AgentGone))
	modelAfterMessagesUntil(t, model, messages, agentGone)
}

// goneAfterSessionAgent reports its session once, then reports itself gone, as herdr does once Claude exits.
type goneAfterSessionAgent struct {
	agentRecorder
	sessionReported atomic.Bool
}

func (a *goneAfterSessionAgent) WaitForSessionID() (string, error) {
	if a.sessionReported.Swap(true) {
		return "", ErrAgentGone
	}
	return "first", nil
}

func followAgentSessionSendsGoneWhenSessionCheckFindsAgentGone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	writeTranscriptLine(t, home, "first", `{"type":"user","message":{"role":"user","content":"before the exit"}}`)
	agent := &goneAfterSessionAgent{}
	messages := make(chan tea.Msg, 64)
	go followAgentSession(agent, "", func(msg tea.Msg) { messages <- msg })
	model := newZenModel(agent, testAppearance, "alt+x", "ctrl+e", "").withSize(60, 12)
	modelAfterMessagesUntil(t, model, messages, agentGone)
}

func followAgentSessionSendsFatalWhenAgentIsGoneAtStart(t *testing.T) {
	agent := &goneAfterSessionAgent{}
	agent.sessionReported.Store(true)
	messages := make(chan tea.Msg, 64)
	go followAgentSession(agent, "", func(msg tea.Msg) { messages <- msg })
	select {
	case msg := <-messages:
		require.IsType(t, fatalMsg{}, msg)
		fatal := msg.(fatalMsg)
		require.ErrorIs(t, fatal.err, ErrAgentGone)
	case <-time.After(5 * time.Second):
		require.FailNow(t, "followAgentSession sent nothing in 5 s")
	}
}

func agentGone(_ zenModel, msg tea.Msg) bool {
	status, isStatus := msg.(agentStatusMsg)
	return isStatus && status.status == AgentGone
}
