package ui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/CLBRITTON2/calmpane/claudecode"
)

func TestLineNumberAt(t *testing.T) {
	t.Run("lineNumberAtCountsLinesBeforeOffset", lineNumberAtCountsLinesBeforeOffset)
}

func lineNumberAtCountsLinesBeforeOffset(t *testing.T) {
	content := "first\nsecond\nthird\n"
	cases := []struct {
		offset int64
		want   int
	}{
		{offset: 0, want: 1},
		{offset: int64(len("first\n")), want: 2},
		{offset: int64(len("first\nsecond\n")), want: 3},
	}
	for _, testCase := range cases {
		got, err := LineNumberAt(strings.NewReader(content), testCase.offset)
		if err != nil {
			t.Fatalf("LineNumberAt(%d): %v", testCase.offset, err)
		}
		if got != testCase.want {
			t.Errorf("LineNumberAt(%d) = %d, want %d", testCase.offset, got, testCase.want)
		}
	}
}

func TestFollowTranscript(t *testing.T) {
	t.Run("followTranscriptNamesUndecodableLineAndKeepsReading", followTranscriptNamesUndecodableLineAndKeepsReading)
	t.Run("followTranscriptSendsQueueOperationsInTheirPlaceAmongEntries", followTranscriptSendsQueueOperationsInTheirPlaceAmongEntries)
	t.Run("followTranscriptSendsTurnOfLineWithOnlyToolCall", followTranscriptSendsTurnOfLineWithOnlyToolCall)
}

func followTranscriptNamesUndecodableLineAndKeepsReading(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	content := `{"type":"user","message":{"role":"user","content":"first"}}` + "\n" +
		`{"type":"user","message":{"role":"user","content":"cut` + "\n" +
		`{"type":"user","message":{"role":"user","content":"third"}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	transcript, err := os.Open(path)
	if err != nil {
		t.Fatalf("open transcript: %v", err)
	}
	t.Cleanup(func() {
		if err := transcript.Close(); err != nil {
			t.Errorf("close transcript: %v", err)
		}
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
				if !strings.HasPrefix(msg.err.Error(), "Cannot read line 2 of ") {
					t.Errorf("problem = %q, want it to name line 2", msg.err)
				}
			case entriesReadMsg:
				entriesSeen = true
				want := []claudecode.DialogueEntry{{Kind: claudecode.PromptEntry, Text: "first"}, {Kind: claudecode.PromptEntry, Text: "third"}}
				if !reflect.DeepEqual(msg.entries, want) {
					t.Errorf("entries = %+v, want %+v", msg.entries, want)
				}
			case fatalMsg:
				t.Fatalf("followTranscript returned: %v", msg.err)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("after 5 s: problem seen %t, entries seen %t", problemSeen, entriesSeen)
		}
	}
}

func TestPromptSend(t *testing.T) {
	t.Run("enterClearsInputAtOnceAndFailedSendPutsWholeTextBack", enterClearsInputAtOnceAndFailedSendPutsWholeTextBack)
	t.Run("enterShowsPromptBandAtOnceAndTranscriptPromptTakesItsPlace", enterShowsPromptBandAtOnceAndTranscriptPromptTakesItsPlace)
	t.Run("enterGivesSlashCommandEarlyBandThatItsCommandEntryReplaces", enterGivesSlashCommandEarlyBandThatItsCommandEntryReplaces)
	t.Run("failedSendRemovesEarlyBandAndShowsRefusal", failedSendRemovesEarlyBandAndShowsRefusal)
	t.Run("shiftEnterStartsNewLineAndEnterSendsTextWithItsNewlines", shiftEnterStartsNewLineAndEnterSendsTextWithItsNewlines)
	t.Run("ctrlEnterSendsPromptThenSendNowKeys", ctrlEnterSendsPromptThenSendNowKeys)
}

func enterClearsInputAtOnceAndFailedSendPutsWholeTextBack(t *testing.T) {
	cases := []struct {
		name            string
		claudeScreenKey string
		err             error
		typedSince      string
		wantInput       string
		wantShown       string
	}{
		{name: "refused while blocked", claudeScreenKey: "alt+x", err: ErrAgentBlocked,
			wantInput: "keep me\nand this line", wantShown: "Claude is waiting for you. Press alt+x to answer."},
		{name: "refused while blocked names configured key", claudeScreenKey: "alt+z", err: ErrAgentBlocked,
			wantInput: "keep me\nand this line", wantShown: "Claude is waiting for you. Press alt+z to answer."},
		{name: "herdr failed", claudeScreenKey: "alt+x", err: errors.New("herdr agent prompt failed: exit status 1"),
			wantInput: "keep me\nand this line", wantShown: "herdr agent prompt failed: exit status 1"},
		{name: "refused after more typing", claudeScreenKey: "alt+x", err: ErrAgentBlocked, typedSince: "more",
			wantInput: "keep me\nand this line\nmore", wantShown: "Claude is waiting for you. Press alt+x to answer."},
		{name: "sent", claudeScreenKey: "alt+x", err: nil, wantInput: "", wantShown: ""},
		{name: "sent after more typing", claudeScreenKey: "alt+x", err: nil, typedSince: "more", wantInput: "more",
			wantShown: ""},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			model := newZenModel(nil, testAppearance, testCase.claudeScreenKey, "ctrl+e", "")
			model.input.SetValue("keep me\nand this line")
			sent, _ := model.withKey(pressedKey(tea.KeyEnter))
			sentModel := sent.(zenModel)
			if sentModel.input.Value() != "" {
				t.Fatalf("input after Enter = %q, want it cleared before the send returns", sentModel.input.Value())
			}
			sentModel.input.SetValue(testCase.typedSince)
			updated, _ := sentModel.Update(promptSentMsg{prompt: "keep me\nand this line", err: testCase.err})
			updatedModel := updated.(zenModel)
			if updatedModel.input.Value() != testCase.wantInput {
				t.Errorf("input = %q, want %q", updatedModel.input.Value(), testCase.wantInput)
			}
			if statusText := ansi.Strip(updatedModel.status); statusText != testCase.wantShown {
				t.Errorf("status = %q, want %q", statusText, testCase.wantShown)
			}
			if updatedModel.fatal != nil {
				t.Errorf("fatal = %v, want the view kept running", updatedModel.fatal)
			}
		})
	}
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
	if !strings.Contains(model.screen(), "prompt 20") {
		t.Fatalf("screen at the bottom lacks the newest prompt:\n%s", model.screen())
	}
	scrolled, _ := model.withKey(pressedKey(tea.KeyPgUp))
	scrolledModel := scrolled.(zenModel)
	before := scrolledModel.screen()
	if strings.Contains(before, "prompt 20") {
		t.Fatalf("screen after PgUp still shows the newest prompt:\n%s", before)
	}
	scrolledModel = scrolledModel.withEntries(numberedPrompts(21, 3), nil)
	if after := scrolledModel.screen(); after != before {
		t.Errorf("screen moved when entries arrived while scrolled up:\nbefore\n%s\nafter\n%s", before, after)
	}
	ended, _ := scrolledModel.withKey(pressedKey(tea.KeyEnd))
	if !strings.Contains(ended.(zenModel).screen(), "prompt 23") {
		t.Errorf("screen after End lacks the newest prompt:\n%s", ended.(zenModel).screen())
	}
}

func screenScrolledPastOldestEntryStopsThereAndOnePgDnReturnsToNewest(t *testing.T) {
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "")
	model = model.withSize(40, 12).withEntries(numberedPrompts(1, 3), nil)
	for range 4 {
		scrolled, _ := model.withKey(pressedKey(tea.KeyPgUp))
		model = scrolled.(zenModel)
	}
	if !strings.Contains(model.screen(), "prompt 1") {
		t.Fatalf("screen after PgUp past the oldest entry lacks it:\n%s", model.screen())
	}
	paged, _ := model.withKey(pressedKey(tea.KeyPgDown))
	if !strings.Contains(paged.(zenModel).screen(), "prompt 3") {
		t.Errorf("screen after one PgDn lacks the newest prompt:\n%s", paged.(zenModel).screen())
	}
}

func upScrollsBackThreeRowsAndDownReturns(t *testing.T) {
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "")
	model = model.withSize(40, 12).withEntries(numberedPrompts(1, 20), nil)
	scrolledUp, _ := model.Update(pressedKey(tea.KeyUp))
	upModel := scrolledUp.(zenModel)
	if upModel.rowsBelowView != arrowScrollRows {
		t.Errorf("rows below the view after Up = %d, want %d", upModel.rowsBelowView, arrowScrollRows)
	}
	if strings.Contains(upModel.screen(), "prompt 20") {
		t.Errorf("screen after Up still shows the newest prompt:\n%s", upModel.screen())
	}
	scrolledDown, _ := upModel.Update(pressedKey(tea.KeyDown))
	if rowsBelowView := scrolledDown.(zenModel).rowsBelowView; rowsBelowView != 0 {
		t.Errorf("rows below the view after Down = %d, want 0", rowsBelowView)
	}
}

func TestCopy(t *testing.T) {
	t.Run("ctrlYOpensCopyModeOnNewestBlockAndEnterCopiesItAndShowsCopiedUntilNextKey",
		ctrlYOpensCopyModeOnNewestBlockAndEnterCopiesItAndShowsCopiedUntilNextKey)
	t.Run("ctrlYSaysSoAndCopiesNothingWithNoCodeBlock", ctrlYSaysSoAndCopiesNothingWithNoCodeBlock)
	t.Run("newStatusReplacesCopyStatus", newStatusReplacesCopyStatus)
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
	if openCmd != nil {
		t.Errorf("ctrl+y returned %#v, want copy mode opened with nothing copied", openCmd())
	}
	if picked := pickedTopRules(openedModel); len(picked) != 1 || !strings.HasPrefix(picked[0], "┏━ go ━") ||
		!strings.Contains(picked[0], copyKeyHint) {
		t.Errorf("iris top rules = %q, want only the newest go block's, with its hint", picked)
	}
	if !strings.Contains(ansi.Strip(openedModel.screen()), copyModeHint) {
		t.Errorf("screen in copy mode lacks the copy mode hint:\n%s", ansi.Strip(openedModel.screen()))
	}
	copied, copyCmd := openedModel.Update(pressedKey(tea.KeyEnter))
	copiedModel := copied.(zenModel)
	if copyCmd == nil {
		t.Fatalf("Enter in copy mode returned no command, want the clipboard set")
	}
	if got, want := copyCmd(), tea.SetClipboard("func f() {\n\treturn \n}")(); !reflect.DeepEqual(got, want) {
		t.Errorf("Enter sets the clipboard with %#v, want %#v", got, want)
	}
	if copiedModel.copyMode != nil || len(pickedTopRules(copiedModel)) != 0 {
		t.Errorf("copy mode still open after Enter, iris top rules %q", pickedTopRules(copiedModel))
	}
	if !strings.Contains(ansi.Strip(copiedModel.screen()), "Copied the code block.") {
		t.Errorf("screen after Enter lacks the copy status:\n%s", ansi.Strip(copiedModel.screen()))
	}
	typed, _ := copiedModel.Update(pressedKey('a'))
	if strings.Contains(ansi.Strip(typed.(zenModel).screen()), "Copied the code block.") {
		t.Errorf("screen after the next key still shows the copy status:\n%s", ansi.Strip(typed.(zenModel).screen()))
	}
}

func ctrlYSaysSoAndCopiesNothingWithNoCodeBlock(t *testing.T) {
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "")
	model = model.withSize(60, 12).withEntries([]claudecode.DialogueEntry{
		{Kind: claudecode.PromptEntry, Text: "hello"}, {Kind: claudecode.ReplyEntry, Text: "Use `inline code` only."},
	}, nil)
	pressed, copyCmd := model.Update(ctrlY)
	if copyCmd != nil {
		t.Errorf("ctrl+y with no code block returned %#v, want no command", copyCmd())
	}
	if pressed.(zenModel).copyMode != nil {
		t.Errorf("ctrl+y with no code block opened copy mode %+v", pressed.(zenModel).copyMode)
	}
	if !strings.Contains(ansi.Strip(pressed.(zenModel).screen()), "No code block to copy.") {
		t.Errorf("screen lacks the no code block status:\n%s", ansi.Strip(pressed.(zenModel).screen()))
	}
}

func newStatusReplacesCopyStatus(t *testing.T) {
	cases := []struct {
		name      string
		statusMsg tea.Msg
		wantShown string
	}{
		{name: "agent status", statusMsg: agentStatusMsg{status: AgentWorking}, wantShown: "Working..."},
		{name: "problem", statusMsg: problemMsg{err: errors.New("herdr pane get failed")},
			wantShown: "herdr pane get failed"},
		{name: "prompt refused", statusMsg: promptSentMsg{err: ErrAgentBlocked},
			wantShown: "Claude is waiting for you. Press alt+x to answer."},
		{name: "send-now keys refused", statusMsg: claudeKeysSentMsg{err: errors.New("herdr pane send-keys failed")},
			wantShown: "herdr pane send-keys failed"},
		{name: "Claude's input not taken",
			statusMsg: agentInputTakenMsg{err: errors.New("Claude's input in pane 3 still holds text after ctrl+u")},
			wantShown: "still holds text after ctrl+u"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "")
			model = model.withSize(60, 12)
			copied, _ := model.Update(tea.KeyPressMsg{Code: 'y', Mod: tea.ModCtrl})
			updated, _ := copied.(zenModel).Update(testCase.statusMsg)
			shown := ansi.Strip(updated.(zenModel).screen())
			if strings.Contains(shown, "No code block to copy.") || !strings.Contains(shown, testCase.wantShown) {
				t.Errorf("screen = %q, want %q in place of the copy status", shown, testCase.wantShown)
			}
		})
	}
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
	if len(hintedRules) != 1 || !strings.HasPrefix(hintedRules[0], "╭─ go ─") {
		t.Errorf("rules with the hint = %q, want only the go block's", hintedRules)
	}
}

func upAndDownPickOlderAndNewerBlockAndStopAtOldestAndNewest(t *testing.T) {
	model := pressedKeys(t, modelWithThreeCodeBlocks(40), ctrlY)
	steps := []struct {
		key        tea.KeyPressMsg
		wantPicked int
	}{
		{key: pressedKey(tea.KeyUp), wantPicked: 1},
		{key: pressedKey(tea.KeyUp), wantPicked: 0},
		{key: pressedKey(tea.KeyUp), wantPicked: 0},
		{key: pressedKey(tea.KeyDown), wantPicked: 1},
		{key: pressedKey(tea.KeyDown), wantPicked: 2},
		{key: pressedKey(tea.KeyDown), wantPicked: 2},
	}
	for number, step := range steps {
		model = pressedKeys(t, model, step.key)
		if model.copyMode.picked != step.wantPicked {
			t.Fatalf("step %d: picked block %d, want %d", number+1, model.copyMode.picked, step.wantPicked)
		}
		if picked := pickedTopRules(model); len(picked) != 1 {
			t.Errorf("step %d: iris top rules = %q, want exactly one", number+1, picked)
		}
	}
	oldest := pressedKeys(t, model, pressedKey(tea.KeyUp), pressedKey(tea.KeyUp))
	if picked := pickedTopRules(oldest); len(picked) != 1 || !strings.HasPrefix(picked[0], "┏━ sh ━") {
		t.Errorf("iris top rules after two Ups = %q, want the oldest sh block's", picked)
	}
}

func spaceMarksPickedBlockAfterItsLanguageAndSpaceAgainUnmarksIt(t *testing.T) {
	marked := pressedKeys(t, modelWithThreeCodeBlocks(40), ctrlY, pressedKey(tea.KeyUp), pressedKey(tea.KeySpace))
	if picked := pickedTopRules(marked); len(picked) != 1 || !strings.HasPrefix(picked[0], "┏━ go · marked ━") {
		t.Errorf("iris top rules after Space = %q, want the picked go block's with marked after the language", picked)
	}
	if !strings.Contains(marked.screen(), markedLabelStyle.Render(markedLabel)) {
		t.Errorf("screen after Space lacks marked in iris:\n%q", marked.screen())
	}
	unmarked := pressedKeys(t, marked, pressedKey(tea.KeySpace))
	if strings.Contains(ansi.Strip(unmarked.screen()), markedLabel) {
		t.Errorf("screen after a second Space still shows marked:\n%s", ansi.Strip(unmarked.screen()))
	}
}

func enterCopiesMarkedBlocksOldestFirstJoinedByBlankLine(t *testing.T) {
	// marked newest first, and the pick left on a block that is not marked
	model := pressedKeys(t, modelWithThreeCodeBlocks(40), ctrlY, pressedKey(tea.KeySpace), pressedKey(tea.KeyUp),
		pressedKey(tea.KeyUp), pressedKey(tea.KeySpace), pressedKey(tea.KeyDown))
	copied, copyCmd := model.Update(pressedKey(tea.KeyEnter))
	if copyCmd == nil {
		t.Fatalf("Enter in copy mode returned no command, want the clipboard set")
	}
	if got, want := copyCmd(), tea.SetClipboard("one\n\nfunc f() {\n\treturn \n}")(); !reflect.DeepEqual(got, want) {
		t.Errorf("Enter sets the clipboard with %#v, want %#v", got, want)
	}
	shown := ansi.Strip(copied.(zenModel).screen())
	if !strings.Contains(shown, "Copied 2 code blocks.") || strings.Contains(shown, markedLabel) {
		t.Errorf("screen after Enter lacks the copy status or still shows marked:\n%s", shown)
	}
}

func escLeavesCopyModeCopyingNothingAndSendsNoInterruptWhileClaudeWorks(t *testing.T) {
	recorder := &agentRecorder{}
	model := modelWithThreeCodeBlocks(40)
	model.agent, model.agentStatus = recorder, AgentWorking
	model = pressedKeys(t, model, ctrlY, pressedKey(tea.KeySpace))
	closed, escCmd := model.Update(pressedKey(tea.KeyEscape))
	if escCmd != nil {
		t.Errorf("Esc in copy mode returned %#v, want no command", escCmd())
	}
	if recorder.calls != nil {
		t.Errorf("Esc in copy mode sent %q to Claude, want nothing", recorder.calls)
	}
	closedModel := closed.(zenModel)
	shown := ansi.Strip(closedModel.screen())
	if closedModel.copyMode != nil || strings.Contains(shown, markedLabel) || strings.Contains(shown, copyModeHint) ||
		strings.Contains(shown, "Copied") || len(pickedTopRules(closedModel)) != 0 {
		t.Errorf("screen after Esc in copy mode still shows copy mode or a copy:\n%s", shown)
	}
}

func typingInCopyModeDoesNotReachInput(t *testing.T) {
	model := modelWithThreeCodeBlocks(40)
	model.input.SetValue("draft")
	typed := pressedKeys(t, model, ctrlY, pressedKey('x'), pressedKey(tea.KeyBackspace))
	if value := typed.input.Value(); value != "draft" {
		t.Errorf("input after typing in copy mode = %q, want %q", value, "draft")
	}
	if typed.copyMode == nil {
		t.Error("typing closed copy mode")
	}
}

func upAndDownScrollViewTheLeastThatShowsWholePickedBlock(t *testing.T) {
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "")
	entries := numberedPrompts(1, 6)
	entries = append(entries, claudecode.DialogueEntry{Kind: claudecode.ReplyEntry, Text: "```sh\nold line 1\nold line 2\n```"})
	entries = append(entries, numberedPrompts(7, 12)...)
	entries = append(entries, claudecode.DialogueEntry{Kind: claudecode.ReplyEntry, Text: "```go\nnew\n```"})
	model = model.withSize(60, 14).withEntries(entries, nil)
	// the entries' last row, above the blank row and the copy mode hint
	lastEntryRow := func(model zenModel) string {
		rows := strings.Split(entriesText(t, model), "\n")
		return strings.TrimSpace(rows[len(rows)-3])
	}
	olderPicked := pressedKeys(t, model, ctrlY, pressedKey(tea.KeyUp))
	shownRows := strings.Split(entriesText(t, olderPicked), "\n")
	if !strings.HasPrefix(strings.TrimSpace(shownRows[0]), "┏━ sh ━") || !strings.Contains(shownRows[1], "old line 1") ||
		!strings.Contains(shownRows[2], "old line 2") || !strings.HasPrefix(strings.TrimSpace(shownRows[3]), "┗━") {
		t.Errorf("screen after Up does not start on the picked block's top rule:\n%s", strings.Join(shownRows, "\n"))
	}
	newerPicked := pressedKeys(t, olderPicked, pressedKey(tea.KeyDown))
	if !strings.Contains(entriesText(t, newerPicked), "┏━ go ━") || !strings.HasPrefix(lastEntryRow(newerPicked), "┗━") {
		t.Errorf("screen after Down does not end on the newest block's bottom rule:\n%s", entriesText(t, newerPicked))
	}
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
	if !strings.HasPrefix(strings.TrimSpace(rows[0]), "┏━ sh ━") || !strings.Contains(rows[1], "line 1") {
		t.Errorf("screen after Up onto a tall block = %q, want its top rule on the first row", rows)
	}
}

func modelWithTypedPrompt(prompt string) zenModel {
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "")
	model = model.withSize(60, 20).withEntries([]claudecode.DialogueEntry{{Kind: claudecode.ReplyEntry, Text: "Ready."}}, nil)
	model.input.SetValue(prompt)
	return model
}

func enterShowsPromptBandAtOnceAndTranscriptPromptTakesItsPlace(t *testing.T) {
	sent, sendPrompt := modelWithTypedPrompt("rename the widget").withKey(pressedKey(tea.KeyEnter))
	if sendPrompt == nil {
		t.Fatal("Enter returned no command to send the prompt")
	}
	if screenText := entriesText(t, sent.(zenModel)); !strings.Contains(screenText, "> rename the widget") {
		t.Fatalf("screen after Enter lacks the prompt's band:\n%s", screenText)
	}
	delivered, _ := sent.Update(promptSentMsg{prompt: "rename the widget", err: nil})
	sentModel := delivered.(zenModel)
	// a reply written before Claude records the prompt goes above the early band
	withReply := sentModel.withEntries([]claudecode.DialogueEntry{{Kind: claudecode.ReplyEntry, Text: "Still on the last step."}}, nil)
	replyText := entriesText(t, withReply)
	replyAt := strings.Index(replyText, "Still on the last step.")
	bandAt := strings.Index(replyText, "> rename the widget")
	if replyAt < 0 || bandAt < 0 || replyAt > bandAt {
		t.Errorf("screen lacks the reply or the early band, or has the reply under the band:\n%s", replyText)
	}
	recorded := withReply.withEntries([]claudecode.DialogueEntry{{Kind: claudecode.PromptEntry, Text: "rename the widget"}}, nil)
	recordedText := ansi.Strip(recorded.screen())
	if count := strings.Count(recordedText, "rename the widget"); count != 1 {
		t.Errorf("screen shows the prompt %d times after the transcript records it, want 1:\n%s", count, recordedText)
	}
	if len(recorded.earlyPrompts) != 0 {
		t.Errorf("earlyPrompts = %+v, want none once the transcript holds the prompt", recorded.earlyPrompts)
	}
}

func enterGivesSlashCommandEarlyBandThatItsCommandEntryReplaces(t *testing.T) {
	sent, sendPrompt := modelWithTypedPrompt("/effort high").withKey(pressedKey(tea.KeyEnter))
	if sendPrompt == nil {
		t.Fatal("Enter returned no command to send the slash command")
	}
	if screenText := entriesText(t, sent.(zenModel)); !strings.Contains(screenText, "> /effort high") {
		t.Fatalf("screen after Enter on a slash command lacks its band:\n%s", screenText)
	}
	recorded := sent.(zenModel).withEntries([]claudecode.DialogueEntry{
		{Kind: claudecode.CommandEntry, Text: "/effort high"},
		{Kind: claudecode.CommandOutputEntry, Text: "Set effort level to high"},
	}, nil)
	recordedText := entriesText(t, recorded)
	if count := strings.Count(recordedText, "/effort high"); count != 1 {
		t.Errorf("screen shows the command %d times after the transcript records it, want 1:\n%s", count, recordedText)
	}
	if !strings.Contains(recordedText, "Set effort level to high") {
		t.Errorf("screen lacks the command's output:\n%s", recordedText)
	}
	if len(recorded.earlyPrompts) != 0 {
		t.Errorf("earlyPrompts = %+v, want none once the transcript holds the command", recorded.earlyPrompts)
	}
}

func failedSendRemovesEarlyBandAndShowsRefusal(t *testing.T) {
	sent, _ := modelWithTypedPrompt("rename the widget").withKey(pressedKey(tea.KeyEnter))
	refused, _ := sent.(zenModel).Update(promptSentMsg{prompt: "rename the widget", err: ErrAgentBlocked})
	refusedModel := refused.(zenModel)
	screenText := entriesText(t, refusedModel)
	if strings.Contains(screenText, "> rename the widget") {
		t.Errorf("screen after a refused send still shows the early band:\n%s", screenText)
	}
	if !strings.Contains(screenText, "Claude is waiting for you. Press alt+x to answer.") {
		t.Errorf("screen after a refused send lacks the refusal:\n%s", screenText)
	}
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
	if after := sentModel.screen(); after != before {
		t.Errorf("screen moved when the early band came:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	recorded := sentModel.withEntries(numberedPrompts(21, 1), nil)
	if after := recorded.screen(); after != before {
		t.Errorf("screen moved when the transcript prompt replaced the early band:\nbefore:\n%s\nafter:\n%s", before,
			after)
	}
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
	rows := strings.Split(ansi.Strip(model.screen()), "\n")
	for index, row := range rows {
		if strings.HasPrefix(row, "───") {
			return rows[:index], rows[index+1:]
		}
	}
	t.Fatalf("screen has no separator:\n%s", strings.Join(rows, "\n"))
	return nil, nil
}

func TestInput(t *testing.T) {
	t.Run("inputWrapsLongLineUnderItsPrompt", inputWrapsLongLineUnderItsPrompt)
	t.Run("inputGrowsRowAtATimeUpToEightAndScreenKeepsItsHeight", inputGrowsRowAtATimeUpToEightAndScreenKeepsItsHeight)
	t.Run("upAndDownMoveCursorWithinInputAndScrollEntriesFromItsEdgeRows", upAndDownMoveCursorWithinInputAndScrollEntriesFromItsEdgeRows)
	t.Run("entriesFillRowsInputGivesBack", entriesFillRowsInputGivesBack)
}

func inputWrapsLongLineUnderItsPrompt(t *testing.T) {
	words := strings.Repeat("wrap these words ", 12)
	for _, width := range []int{40, 90} {
		model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "")
		model = model.withSize(width, 30)
		model.input.SetValue(words)
		rows := inputRows(t, model)
		if len(rows) < 2 {
			t.Fatalf("width %d: input rows = %q, want the text wrapped onto several rows", width, rows)
		}
		if !strings.HasPrefix(rows[0], " > wrap") {
			t.Errorf("width %d: first input row = %q, want it to start with the prompt", width, rows[0])
		}
		joined := ""
		for index, row := range rows {
			if rowWidth := ansi.StringWidth(row); rowWidth > width {
				t.Errorf("width %d: input row %q is %d columns wide", width, row, rowWidth)
			}
			if index > 0 && !strings.HasPrefix(row, "   ") {
				t.Errorf("width %d: wrapped row %q is not indented under the text", width, row)
			}
			joined += strings.TrimSpace(strings.TrimPrefix(row, " > ")) + " "
		}
		if strings.Join(strings.Fields(joined), " ") != strings.TrimSpace(words) {
			t.Errorf("width %d: input rows hold %q, want %q", width, joined, words)
		}
	}
}

func inputGrowsRowAtATimeUpToEightAndScreenKeepsItsHeight(t *testing.T) {
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "")
	model = model.withSize(60, 24).withEntries(numberedPrompts(1, 20), nil)
	for lineCount := 1; lineCount <= 11; lineCount++ {
		model.input.SetValue(strings.TrimSuffix(strings.Repeat("line\n", lineCount), "\n"))
		model = model.withVisibleRowsDrawn()
		wantRows := min(lineCount, inputMaxRows)
		if rows := inputRows(t, model); len(rows) != wantRows {
			t.Errorf("%d lines: %d input rows, want %d", lineCount, len(rows), wantRows)
		}
		if screenRows := strings.Count(model.screen(), "\n") + 1; screenRows != 24 {
			t.Errorf("%d lines: screen has %d rows, want the pane's 24", lineCount, screenRows)
		}
	}
	if value := model.input.Value(); strings.Count(value, "line") != 11 {
		t.Errorf("input = %q, want all 11 lines kept past the 8 rows shown", value)
	}
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
	if value := model.input.Value(); value != "one\ntwo" {
		t.Fatalf("input = %q, want %q", value, "one\ntwo")
	}
	_, sendPrompt := model.withKey(pressedKey(tea.KeyEnter))
	sent, isPromptSent := sendPrompt().(promptSentMsg)
	if !isPromptSent || sent.prompt != "one\ntwo" {
		t.Errorf("Enter sent %+v, want the prompt %q", sent, "one\ntwo")
	}
}

func upAndDownMoveCursorWithinInputAndScrollEntriesFromItsEdgeRows(t *testing.T) {
	cases := []struct {
		name       string
		inputValue string
		inputRows  int
	}{
		{name: "three lines", inputValue: "first\nsecond\nthird", inputRows: 3},
		{name: "line and wrapped line", inputValue: "first\n" + strings.Repeat("wrapped words ", 4), inputRows: 3},
		{name: "one line", inputValue: "only", inputRows: 1},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "")
			model = model.withSize(40, 16)
			model.input.SetValue(testCase.inputValue)
			model = model.withEntries(numberedPrompts(1, 20), nil)
			if rows := inputRows(t, model); len(rows) != testCase.inputRows {
				t.Fatalf("input rows = %q, want %d", rows, testCase.inputRows)
			}
			for press := 1; press <= testCase.inputRows; press++ {
				pressed, _ := model.withKey(pressedKey(tea.KeyUp))
				model = pressed.(zenModel)
				wantRowsBelowView := 0
				if press == testCase.inputRows {
					wantRowsBelowView = arrowScrollRows
				}
				if model.rowsBelowView != wantRowsBelowView {
					t.Errorf("Up %d: rows below the view = %d, want %d", press, model.rowsBelowView, wantRowsBelowView)
				}
			}
			for press := 1; press <= testCase.inputRows; press++ {
				pressed, _ := model.withKey(pressedKey(tea.KeyDown))
				model = pressed.(zenModel)
				wantRowsBelowView := arrowScrollRows
				if press == testCase.inputRows {
					wantRowsBelowView = 0
				}
				if model.rowsBelowView != wantRowsBelowView {
					t.Errorf("Down %d: rows below the view = %d, want %d", press, model.rowsBelowView, wantRowsBelowView)
				}
			}
			if value := model.input.Value(); value != testCase.inputValue {
				t.Errorf("input = %q, want the draft %q unchanged", value, testCase.inputValue)
			}
		})
	}
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
	if screen, want := model.screen(), modelWithInputThenEntries(model.input.Value()).screen(); screen != want {
		t.Errorf("screen after Backspace shrank the input:\n%s\nwant:\n%s", screen, want)
	}
	prompt := model.input.Value()
	sent, _ := model.withKey(pressedKey(tea.KeyEnter))
	want := modelWithInputThenEntries("").withEarlyPromptAdded(earlyPrompt{text: prompt, pastes: nil, queued: false})
	if screen := sent.(zenModel).screen(); screen != want.screen() {
		t.Errorf("screen after Enter cleared the input:\n%s\nwant:\n%s", screen, want.screen())
	}
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
	t.Run("removeThatDropsPromptRemovesItsBand", removeThatDropsPromptRemovesItsBand)
	t.Run("queueOperationsApplyInTheirPlaceAmongEntries", queueOperationsApplyInTheirPlaceAmongEntries)
}

func enqueueAddsNoteRowUnderBandWithItsTextAndMatchingRemoveDropsIt(t *testing.T) {
	model := modelWithSentPrompts([]string{"first one", "second one"})
	queued := model.withEntries(nil, []placedQueueOperation{{QueueOperation: claudecode.QueueOperation{Operation: "enqueue", Text: "second one"}}})
	if row := rowUnderBand(t, queued, "second one"); row != "   "+queuedNote {
		t.Errorf("row under the band after the enqueue = %q, want the queued note under the prompt text", row)
	}
	if row := rowUnderBand(t, queued, "first one"); strings.Contains(row, "queued") {
		t.Errorf("row under the other prompt's band = %q, want no queued note", row)
	}
	taken := queued.withEntries(nil, []placedQueueOperation{
		{QueueOperation: claudecode.QueueOperation{Operation: "remove", Reason: "absorbed_mid_turn", Text: "second one"}},
	})
	if bandTextRow(t, taken, "second one") == "" {
		t.Errorf("band left with the remove, want it kept:\n%s", ansi.Strip(taken.screen()))
	}
	if row := rowUnderBand(t, taken, "second one"); strings.Contains(row, "queued") {
		t.Errorf("row under the band after the remove = %q, want no queued note", row)
	}
}

func dequeueDropsNoteRowOfOldestMarkedBand(t *testing.T) {
	model := modelWithSentPrompts([]string{"first one", "second one"}).withEntries(nil, []placedQueueOperation{
		{QueueOperation: claudecode.QueueOperation{Operation: "enqueue", Text: "first one"}},
		{QueueOperation: claudecode.QueueOperation{Operation: "enqueue", Text: "second one"}},
		{QueueOperation: claudecode.QueueOperation{Operation: "dequeue"}},
	})
	if row := rowUnderBand(t, model, "first one"); strings.Contains(row, "queued") {
		t.Errorf("row under the oldest band after the dequeue = %q, want no queued note", row)
	}
	if row := rowUnderBand(t, model, "second one"); row != "   "+queuedNote {
		t.Errorf("row under the newer band after the dequeue = %q, want the queued note", row)
	}
}

func removeThatDropsPromptRemovesItsBand(t *testing.T) {
	cases := []claudecode.QueueOperation{
		{Operation: "remove", Reason: "cleared_on_cancel", Text: "second one"},
		{Operation: "remove", Reason: "not_carried_to_background", Text: "second one"},
		{Operation: "remove", Reason: "dropped_by_hook"},
	}
	for _, removal := range cases {
		t.Run(removal.Reason, func(t *testing.T) {
			model := modelWithSentPrompts([]string{"first one", "second one"}).withEntries(nil, []placedQueueOperation{
				{QueueOperation: claudecode.QueueOperation{Operation: "enqueue", Text: "second one"}}, {QueueOperation: removal},
			})
			if row := bandTextRow(t, model, "second one"); row != "" {
				t.Errorf("screen still shows the removed band: %q", row)
			}
			if row := bandTextRow(t, model, "first one"); row == "" {
				t.Errorf("screen lost the band of the prompt Claude did not queue:\n%s", ansi.Strip(model.screen()))
			}
		})
	}
}

func queueOperationsApplyInTheirPlaceAmongEntries(t *testing.T) {
	cases := []struct {
		name          string
		entriesBefore int
		wantQueued    bool
	}{
		{name: "enqueue before the prompt", entriesBefore: 0, wantQueued: false},
		{name: "enqueue after the prompt", entriesBefore: 1, wantQueued: true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			model := modelWithSentPrompts([]string{"again", "again"}).withEntries(
				[]claudecode.DialogueEntry{{Kind: claudecode.PromptEntry, Text: "again"}},
				[]placedQueueOperation{
					{QueueOperation: claudecode.QueueOperation{Operation: "enqueue", Text: "again"}, entriesBefore: testCase.entriesBefore},
				})
			want := []earlyPrompt{{text: "again", queued: testCase.wantQueued}}
			if !reflect.DeepEqual(model.earlyPrompts, want) {
				t.Errorf("earlyPrompts = %+v, want %+v", model.earlyPrompts, want)
			}
		})
	}
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
	if len(queued.earlyPromptRows()) == len(scrolledModel.earlyPromptRows()) {
		t.Fatalf("the enqueue left the early prompts at %d rows, want the note row added", len(queued.earlyPromptRows()))
	}
	if after := queued.screen(); after != before {
		t.Errorf("screen moved when the band was marked:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	taken := queued.withEntries(nil, []placedQueueOperation{{QueueOperation: claudecode.QueueOperation{Operation: "dequeue"}}})
	if after := taken.screen(); after != before {
		t.Errorf("screen moved when the mark was cleared:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func ctrlEnterSendsPromptThenSendNowKeys(t *testing.T) {
	cases := []struct {
		name      string
		typed     string
		promptErr error
		wantCalls []string
		wantMsg   tea.Msg
	}{
		{name: "typed prompt", typed: "ship it",
			wantCalls: []string{"prompt ship it", "send now"}, wantMsg: claudeKeysSentMsg{}},
		{name: "empty input", typed: "", wantCalls: []string{"send now"}, wantMsg: claudeKeysSentMsg{}},
		{name: "refused prompt sends no keys", typed: "ship it", promptErr: ErrAgentBlocked,
			wantCalls: []string{"prompt ship it"}, wantMsg: promptSentMsg{prompt: "ship it", err: ErrAgentBlocked}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := &agentRecorder{promptErr: testCase.promptErr}
			model := modelWithTypedPrompt(testCase.typed)
			model.agent = recorder
			pressed, send := model.withKey(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModCtrl})
			if send == nil {
				t.Fatal("ctrl+enter returned no command")
			}
			if value := pressed.(zenModel).input.Value(); value != "" {
				t.Errorf("input after ctrl+enter = %q, want it cleared", value)
			}
			if msg := send(); !reflect.DeepEqual(msg, testCase.wantMsg) {
				t.Errorf("ctrl+enter's command returned %#v, want %#v", msg, testCase.wantMsg)
			}
			if !reflect.DeepEqual(recorder.calls, testCase.wantCalls) {
				t.Errorf("sent %q, want %q", recorder.calls, testCase.wantCalls)
			}
		})
	}
}

func TestEsc(t *testing.T) {
	t.Run("escSendsEscToClaudeAndKeepsInputOnlyWhileItWorks", escSendsEscToClaudeAndKeepsInputOnlyWhileItWorks)
	t.Run("escWhileClaudeIsNotWorkingEmptiesInputLabelsAndPastes", escWhileClaudeIsNotWorkingEmptiesInputLabelsAndPastes)
	t.Run("escBeforeReplyMovesPromptClaudePutBackIntoZenInput", escBeforeReplyMovesPromptClaudePutBackIntoZenInput)
	t.Run("promptPutBackAheadOfExpandedTextLeavesItPlain", promptPutBackAheadOfExpandedTextLeavesItPlain)
	t.Run("cancelledPromptRestoreKeepsEntriesWhenNewestIsNotThatPrompt", cancelledPromptRestoreKeepsEntriesWhenNewestIsNotThatPrompt)
	t.Run("cancelledPromptRestoreChangesNothingWhenClaudeGaveNoPromptBack", cancelledPromptRestoreChangesNothingWhenClaudeGaveNoPromptBack)
}

func escSendsEscToClaudeAndKeepsInputOnlyWhileItWorks(t *testing.T) {
	cases := []struct {
		name        string
		agentStatus AgentStatus
		wantCalls   []string
		wantInput   string
	}{
		{name: "working", agentStatus: AgentWorking, wantCalls: []string{"interrupt"}, wantInput: "half typed"},
		{name: "idle", agentStatus: AgentIdle, wantCalls: nil, wantInput: ""},
		{name: "blocked", agentStatus: AgentBlocked, wantCalls: nil, wantInput: ""},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := &agentRecorder{}
			model := modelWithTypedPrompt("half typed")
			model.agent, model.agentStatus = recorder, testCase.agentStatus
			pressed, send := model.withKey(tea.KeyPressMsg{Code: tea.KeyEscape})
			if send != nil {
				if msg := send(); !reflect.DeepEqual(msg, claudeKeysSentMsg{}) {
					t.Errorf("esc's command returned %#v, want claudeKeysSentMsg{}", msg)
				}
			}
			if !reflect.DeepEqual(recorder.calls, testCase.wantCalls) {
				t.Errorf("sent %q, want %q", recorder.calls, testCase.wantCalls)
			}
			if value := pressed.(zenModel).input.Value(); value != testCase.wantInput {
				t.Errorf("input after esc = %q, want %q", value, testCase.wantInput)
			}
		})
	}
}

func escWhileClaudeIsNotWorkingEmptiesInputLabelsAndPastes(t *testing.T) {
	model := pressedKeys(t, modelWithTwoPastes(t, "ctrl+e", len("look at ")), ctrlE)
	model.agentStatus = AgentIdle
	cleared := pressedKeys(t, model, pressedKey(tea.KeyEscape))
	assertInputAndPastes(t, cleared, "", nil)
	if cleared.expandedPastes != nil {
		t.Errorf("expanded pastes after esc = %+v, want none", cleared.expandedPastes)
	}
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
	if takeInput == nil {
		t.Fatal("Claude stopping after Esc returned no command to take its input")
	}
	taken := takeInput()
	if !reflect.DeepEqual(taken, agentInputTakenMsg{text: "move this draft"}) {
		t.Fatalf("take input returned %#v, want the prompt Claude put back", taken)
	}
	if want := []string{"interrupt", "take input"}; !reflect.DeepEqual(recorder.calls, want) {
		t.Errorf("sent %q, want %q", recorder.calls, want)
	}
	restored, _ := stopped.(zenModel).Update(taken)
	if value := restored.(zenModel).input.Value(); value != "move this draft\ntyped since" {
		t.Errorf("input = %q, want the cancelled prompt ahead of the text typed since", value)
	}
	if screenText := entriesText(t, restored.(zenModel)); strings.Contains(screenText, "> move this draft") {
		t.Errorf("the cancelled prompt's band still shows:\n%s", screenText)
	}
	if _, again := restored.(zenModel).Update(agentStatusMsg{status: AgentIdle}); again != nil {
		t.Error("a later status change took Claude's input again")
	}
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
	if len(restored.entries) != len(model.entries) || len(restored.entryRows) != len(model.entryRows) {
		t.Errorf("entries = %d, want all %d kept", len(restored.entries), len(model.entries))
	}
	if value := restored.input.Value(); value != "move this draft" {
		t.Errorf("input = %q, want the cancelled prompt", value)
	}
}

func cancelledPromptRestoreChangesNothingWhenClaudeGaveNoPromptBack(t *testing.T) {
	model := modelWithTypedPrompt("typed since")
	model = model.withEntries([]claudecode.DialogueEntry{{Kind: claudecode.PromptEntry, Text: "move this draft"}}, nil)
	restored := model.withCancelledPromptRestored("")
	if len(restored.entries) != len(model.entries) || restored.input.Value() != "typed since" {
		t.Errorf("entries = %d and input = %q, want %d and %q", len(restored.entries), restored.input.Value(),
			len(model.entries), "typed since")
	}
}

func screenScrolledUpStaysOnItsRowsWhenCancelledPromptIsDropped(t *testing.T) {
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "")
	model = model.withSize(40, 12).withEntries(numberedPrompts(1, 20), nil)
	scrolled, _ := model.withKey(pressedKey(tea.KeyPgUp))
	before := scrolled.(zenModel).screen()
	restored := scrolled.(zenModel).withCancelledPromptRestored("prompt 20")
	if len(restored.entries) != 19 {
		t.Fatalf("entries = %d, want the cancelled prompt dropped from 20", len(restored.entries))
	}
	beforeEntryRows := strings.Join(strings.Split(before, "\n")[:6], "\n")
	afterEntryRows := strings.Join(strings.Split(restored.screen(), "\n")[:6], "\n")
	if afterEntryRows != beforeEntryRows {
		t.Errorf("entry rows moved when the cancelled prompt was dropped:\nbefore\n%s\nafter\n%s", beforeEntryRows,
			afterEntryRows)
	}
}

func followTranscriptSendsQueueOperationsInTheirPlaceAmongEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	content := `{"type":"user","message":{"role":"user","content":"first"}}` + "\n" +
		`{"type":"queue-operation","operation":"enqueue","content":"second"}` + "\n" +
		`{"type":"user","message":{"role":"user","content":"third"}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	read := nextEntriesRead(t, followTranscriptMessages(t, path))
	want := []placedQueueOperation{{QueueOperation: claudecode.QueueOperation{Operation: "enqueue", Text: "second"}, entriesBefore: 1}}
	if !reflect.DeepEqual(read.queueOperations, want) {
		t.Errorf("queue operations = %+v, want %+v", read.queueOperations, want)
	}
}

func followTranscriptMessages(t *testing.T, path string) chan tea.Msg {
	transcript, err := os.Open(path)
	if err != nil {
		t.Fatalf("open transcript: %v", err)
	}
	t.Cleanup(func() {
		if err := transcript.Close(); err != nil {
			t.Errorf("close transcript: %v", err)
		}
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
				t.Fatalf("followTranscript reported: %v", msg.err)
			case fatalMsg:
				t.Fatalf("followTranscript returned: %v", msg.err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("no entries read after 5 s")
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
	t.Run("workingRowNamesNewestToolCallOfTurnAndTimeSinceItsPrompt", workingRowNamesNewestToolCallOfTurnAndTimeSinceItsPrompt)
	t.Run("workingRowCutsToolCallAtViewWidthAndKeepsTime", workingRowCutsToolCallAtViewWidthAndKeepsTime)
	t.Run("turnDurationShowsSecondsUnderAMinuteAndMinutesThenSecondsSpacedApartFromOne",
		turnDurationShowsSecondsUnderAMinuteAndMinutesThenSecondsSpacedApartFromOne)
	t.Run("blockedAndIdleRowsShowNoToolCallOrTime", blockedAndIdleRowsShowNoToolCallOrTime)
	t.Run("waitingLineWrapsAtNarrowWidthWithinScreenHeight", waitingLineWrapsAtNarrowWidthWithinScreenHeight)
	t.Run("waitingLineShowsWhileClaudeWorksWithoutInputBox", waitingLineShowsWhileClaudeWorksWithoutInputBox)
	t.Run("workingRowShowsWhileClaudeWorksWithInputBox", workingRowShowsWhileClaudeWorksWithInputBox)
	t.Run("workingRowReturnsWhenInputBoxComesBack", workingRowReturnsWhenInputBoxComesBack)
	t.Run("waitingLineStaysHiddenWhileClaudeIsIdleWithoutInputBox", waitingLineStaysHiddenWhileClaudeIsIdleWithoutInputBox)
	t.Run("pollReportsInputBoxGoneWhileClaudeWorksAndBackOnceItStops", pollReportsInputBoxGoneWhileClaudeWorksAndBackOnceItStops)
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

func workingRowNamesNewestToolCallOfTurnAndTimeSinceItsPrompt(t *testing.T) {
	prompt := promptLine("2026-11-20T10:00:00.000Z", "go")
	now := time.Date(2026, 11, 20, 10, 0, 42, 500_000_000, time.UTC)
	cases := []struct {
		name    string
		lines   []string
		wantRow string
	}{
		{name: "Bash with a description",
			lines: []string{prompt,
				toolCallLine("Bash", `{"command":"go test ./cmd/zen/...","description":"Running go test ./cmd/zen/..."}`)},
			wantRow: "Running go test ./cmd/zen/... · 42s"},
		{name: "Bash without a description", lines: []string{prompt, toolCallLine("Bash", `{"command":"ls"}`)},
			wantRow: "Running a command · 42s"},
		{name: "Edit", lines: []string{prompt, toolCallLine("Edit", `{"file_path":"C:\\dev\\zen\\cmd\\zen\\view.go"}`)},
			wantRow: "Editing view.go · 42s"},
		{name: "Write", lines: []string{prompt, toolCallLine("Write", `{"file_path":"C:/dev/zen/notes.md"}`)},
			wantRow: "Editing notes.md · 42s"},
		{name: "Read", lines: []string{prompt, toolCallLine("Read", `{"file_path":"C:/dev/zen/go.mod"}`)},
			wantRow: "Reading go.mod · 42s"},
		{name: "Grep", lines: []string{prompt, toolCallLine("Grep", `{"pattern":"statusLine","path":"cmd"}`)},
			wantRow: "Searching for statusLine · 42s"},
		{name: "Glob", lines: []string{prompt, toolCallLine("Glob", `{"pattern":"**/*.go"}`)},
			wantRow: "Searching for **/*.go · 42s"},
		{name: "Agent", lines: []string{prompt, toolCallLine("Agent", `{"description":"Find the herdr calls","prompt":"x"}`)},
			wantRow: "Find the herdr calls · 42s"},
		{name: "unknown tool", lines: []string{prompt, toolCallLine("Read", `{"file_path":"a.go"}`),
			toolCallLine("WebSearch", `{"query":"herdr"}`)},
			wantRow: "Working... · 42s"},
		{name: "no tool call yet", lines: []string{prompt}, wantRow: "Working... · 42s"},
		{name: "tool call of an earlier turn ignored",
			lines: []string{promptLine("2026-11-20T09:00:00.000Z", "first"), toolCallLine("Bash", `{"command":"ls"}`),
				prompt},
			wantRow: "Working... · 42s"},
		{name: "no prompt in the tail", lines: []string{toolCallLine("Read", `{"file_path":"a.go"}`)},
			wantRow: "Reading a.go"},
		{name: "prompt taken in mid-turn keeps the turn's time and tool call",
			lines: []string{prompt, toolCallLine("Read", `{"file_path":"a.go"}`),
				`{"type":"attachment","timestamp":"2026-11-20T10:00:30.000Z","attachment":{"type":"queued_command",` +
					`"prompt":"also check the logs","commandMode":"prompt","origin":{"kind":"human"}}}`},
			wantRow: "Reading a.go · 42s"},
		{name: "slash command starts a turn",
			lines: []string{promptLine("2026-11-20T09:00:00.000Z", "first"), toolCallLine("Bash", `{"command":"ls"}`),
				promptLine("2026-11-20T10:00:00.000Z", `<command-name>/effort</command-name>\n<command-args>high</command-args>`)},
			wantRow: "Working... · 42s"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			turn := turnAfterLines(t, claudecode.CurrentTurn{}, testCase.lines)
			if row := ansi.Strip(statusLine(AgentWorking, turn, now, 80, testAppearance, "alt+x")); row != testCase.wantRow {
				t.Errorf("working row = %q, want %q", row, testCase.wantRow)
			}
		})
	}
}

func workingRowCutsToolCallAtViewWidthAndKeepsTime(t *testing.T) {
	promptTime := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	turn := claudecode.CurrentTurn{PromptTime: promptTime, ToolCallActivity: "Running a very long command description"}
	if row := ansi.Strip(statusLine(AgentWorking, turn, promptTime.Add(42*time.Second), 20, testAppearance, "alt+x")); row != "Running a ver · 42s" {
		t.Errorf("working row at width 20 = %q, want the tool call cut to fit the time in the 19 columns after the "+
			"margin", row)
	}
}

func turnDurationShowsSecondsUnderAMinuteAndMinutesThenSecondsSpacedApartFromOne(t *testing.T) {
	cases := []struct {
		elapsed time.Duration
		want    string
	}{
		{elapsed: 0, want: "0s"},
		{elapsed: 42*time.Second + 900*time.Millisecond, want: "42s"},
		{elapsed: 59 * time.Second, want: "59s"},
		{elapsed: time.Minute, want: "1m 0s"},
		{elapsed: time.Minute + 11*time.Second, want: "1m 11s"},
		{elapsed: 3*time.Minute + 5*time.Second, want: "3m 5s"},
	}
	for _, testCase := range cases {
		if got := TurnDuration(testCase.elapsed); got != testCase.want {
			t.Errorf("TurnDuration(%s) = %q, want %q", testCase.elapsed, got, testCase.want)
		}
	}
}

func blockedAndIdleRowsShowNoToolCallOrTime(t *testing.T) {
	turn := claudecode.CurrentTurn{PromptTime: time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC), ToolCallActivity: "Reading go.mod"}
	now := turn.PromptTime.Add(42 * time.Second)
	if row := ansi.Strip(statusLine(AgentBlocked, turn, now, 80, testAppearance, "alt+z")); row != "Waiting for you in Claude. Press alt+z to answer." {
		t.Errorf("blocked row = %q", row)
	}
	if row := statusLine(AgentIdle, turn, now, 80, testAppearance, "alt+x"); row != "" {
		t.Errorf("idle row = %q, want it empty", row)
	}
}

func waitingLineWrapsAtNarrowWidthWithinScreenHeight(t *testing.T) {
	sized := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "").withSize(24, 12)
	blockedModel, _ := sized.Update(agentStatusMsg{status: AgentBlocked})
	pickerModel := sized
	pickerModel.agentPickerShown = true
	want := []string{" Waiting for you in", " Claude. Press alt+x to", " answer."}
	for name, model := range map[string]zenModel{"blocked": blockedModel.(zenModel), "picker": pickerModel} {
		t.Run(name, func(t *testing.T) {
			rows := strings.Split(ansi.Strip(model.screen()), "\n")
			if len(rows) != 12 {
				t.Fatalf("screen has %d rows, want the height of 12", len(rows))
			}
			trimmedRows := []string{}
			for _, row := range rows {
				trimmedRows = append(trimmedRows, strings.TrimRight(row, " "))
			}
			if got := trimmedRows[7:10]; !reflect.DeepEqual(got, want) {
				t.Errorf("rows above the separator = %q, want %q\nscreen:\n%s", got, want, strings.Join(rows, "\n"))
			}
		})
	}
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
	if status := ansi.Strip(boxGone.(zenModel).shownStatus()); status != "Waiting for you in Claude. Press alt+x to answer." {
		t.Errorf("status while Claude works with no input box = %q, want the waiting line", status)
	}
}

func workingRowShowsWhileClaudeWorksWithInputBox(t *testing.T) {
	boxShown, _ := workingModel(t).Update(agentInputBoxMsg{shown: true})
	if status := ansi.Strip(boxShown.(zenModel).shownStatus()); !strings.HasPrefix(status, "Reading go.mod · ") {
		t.Errorf("status while Claude works with its input box = %q, want the working row", status)
	}
}

func workingRowReturnsWhenInputBoxComesBack(t *testing.T) {
	boxGone, _ := workingModel(t).Update(agentInputBoxMsg{shown: false})
	boxBack, _ := boxGone.(zenModel).Update(agentInputBoxMsg{shown: true})
	if status := ansi.Strip(boxBack.(zenModel).shownStatus()); !strings.HasPrefix(status, "Reading go.mod · ") {
		t.Errorf("status after the input box came back = %q, want the working row", status)
	}
}

func waitingLineStaysHiddenWhileClaudeIsIdleWithoutInputBox(t *testing.T) {
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "").withSize(60, 12)
	idle, _ := model.Update(agentStatusMsg{status: AgentIdle})
	boxGone, _ := idle.(zenModel).Update(agentInputBoxMsg{shown: false})
	if status := ansi.Strip(boxGone.(zenModel).shownStatus()); status != "" {
		t.Errorf("status while Claude is idle with no input box = %q, want it empty", status)
	}
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
			if msg != wantMsg {
				t.Fatalf("poll sent %#v, want %#v", msg, wantMsg)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("poll sent nothing in 5s, want %#v", wantMsg)
		}
	}
}

func pollReportsInputBoxGoneWhileClaudeWorksAndBackOnceItStops(t *testing.T) {
	agent := &statusSwitchingAgent{agentRecorder: &agentRecorder{pickerShown: true}}
	agent.status.Store(int32(AgentWorking))
	sent := make(chan tea.Msg, 4)
	go pollAgentStatus(agent, func(msg tea.Msg) { sent <- msg })
	assertPollSentInOrder(t, sent, []tea.Msg{agentStatusMsg{status: AgentWorking}, agentInputBoxMsg{shown: false}})
	agent.status.Store(int32(AgentIdle))
	assertPollSentInOrder(t, sent, []tea.Msg{agentStatusMsg{status: AgentIdle}, agentInputBoxMsg{shown: true}})
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
	if firstTick == nil {
		t.Fatal("working status returned no clock tick")
	}
	tick, isTick := firstTick().(clockTickMsg)
	if !isTick {
		t.Fatalf("working status's command returned %#v, want a clock tick", firstTick())
	}
	if _, secondStart := started.(zenModel).Update(agentStatusMsg{status: AgentWorking}); secondStart != nil {
		t.Errorf("a second working status started another clock")
	}
	ticked, nextTick := started.(zenModel).Update(tick)
	if nextTick == nil {
		t.Errorf("a tick while working scheduled no next tick")
	}
	if screen := ansi.Strip(ticked.(zenModel).screen()); !strings.Contains(screen, " Reading go.mod · 42s") {
		t.Errorf("screen after a tick lacks the working row:\n%s", screen)
	}
	later, _ := ticked.(zenModel).Update(clockTickMsg{now: tick.now.Add(3 * time.Second)})
	if screen := ansi.Strip(later.(zenModel).screen()); !strings.Contains(screen, " Reading go.mod · 45s") {
		t.Errorf("screen three seconds later lacks the grown time:\n%s", screen)
	}
	idle, _ := later.(zenModel).Update(agentStatusMsg{status: AgentIdle})
	if screen := ansi.Strip(idle.(zenModel).screen()); strings.Contains(screen, "Reading go.mod") {
		t.Errorf("screen after the turn ended still shows the working row:\n%s", screen)
	}
	stopped, afterIdle := idle.(zenModel).Update(clockTickMsg{now: tick.now.Add(4 * time.Second)})
	if afterIdle != nil || stopped.(zenModel).clockTicking {
		t.Errorf("a tick after the turn ended scheduled another")
	}
}

func doneRowShowsTurnTimeAfterClaudeStopsWorkingUntilItWorksAgain(t *testing.T) {
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "")
	model = model.withSize(60, 12)
	promptTime := time.Now().Add(-time.Hour)
	read, _ := model.Update(entriesReadMsg{turn: claudecode.CurrentTurn{PromptTime: promptTime}})
	working, _ := read.(zenModel).Update(agentStatusMsg{status: AgentWorking})
	stopped, endTurn := working.(zenModel).Update(agentStatusMsg{status: AgentDone})
	if endTurn == nil {
		t.Fatal("Claude stopping work returned no command to end the turn")
	}
	if _, isTurnEnd := endTurn().(turnEndedMsg); !isTurnEnd {
		t.Fatalf("Claude stopping work's command returned %#v, want a turn end", endTurn())
	}
	restarted, _ := stopped.(zenModel).Update(agentStatusMsg{status: AgentWorking})
	late, _ := restarted.(zenModel).Update(turnEndedMsg{now: promptTime.Add(time.Minute)})
	if screen := ansi.Strip(late.(zenModel).screen()); strings.Contains(screen, "Done") {
		t.Errorf("a turn end arriving after Claude worked again showed the done row:\n%s", screen)
	}
	ended, _ := stopped.(zenModel).Update(turnEndedMsg{now: promptTime.Add(time.Minute + 11*time.Second)})
	if screen := ansi.Strip(ended.(zenModel).screen()); !strings.Contains(screen, " Done · 1m 11s") {
		t.Errorf("screen after the turn ended lacks the done row:\n%s", screen)
	}
	again, _ := ended.(zenModel).Update(agentStatusMsg{status: AgentWorking})
	if screen := ansi.Strip(again.(zenModel).screen()); strings.Contains(screen, "Done") {
		t.Errorf("screen with Claude working again still shows the done row:\n%s", screen)
	}
}

func doneRowStaysHiddenWhenEscInterruptedTurnOrClaudeWasNeverSeenWorking(t *testing.T) {
	recorder := &agentRecorder{}
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "")
	model = model.withSize(60, 12)
	model.agent, model.agentStatus = recorder, AgentWorking
	pressed, _ := model.withKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if _, takeInput := pressed.(zenModel).Update(agentStatusMsg{status: AgentDone}); takeInput == nil {
		t.Fatal("Claude stopping after Esc returned no command")
	} else if _, isTurnEnd := takeInput().(turnEndedMsg); isTurnEnd {
		t.Error("Claude stopping after Esc ended the turn with a done row")
	}
	model.agentStatus = AgentIdle
	if _, cmd := model.Update(agentStatusMsg{status: AgentDone}); cmd != nil {
		t.Errorf("a status change Claude never worked before returned %#v, want none", cmd())
	}
}

var turnStart = time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)

func transcriptTimestamp(sinceTurnStart time.Duration) string {
	return turnStart.Add(sinceTurnStart).Format("2006-01-02T15:04:05.000Z")
}

func turnAfterLines(t *testing.T, turn claudecode.CurrentTurn, lines []string) claudecode.CurrentTurn {
	t.Helper()
	for _, line := range lines {
		lineEntries, err := claudecode.DialogueEntries([]byte(line))
		if err != nil {
			t.Fatalf("DialogueEntries(%s): %v", line, err)
		}
		turn, err = claudecode.TurnAfterLine(turn, []byte(line), lineEntries)
		if err != nil {
			t.Fatalf("TurnAfterLine(%s): %v", line, err)
		}
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
	if screen := ansi.Strip(prompted.screen()); !strings.Contains(screen, " Done · 1m 11s") {
		t.Fatalf("screen after the prompt's turn lacks its done row:\n%s", screen)
	}
	dialog := readLinesThenEndTurn(t, prompted, nil, turnStart.Add(27*time.Minute))
	if screen := ansi.Strip(dialog.screen()); strings.Contains(screen, "Done") {
		t.Errorf("work that recorded no new turn showed a done row:\n%s", screen)
	}
}

func doneRowShowsNewPromptTimeAfterWorkThatRecordedNoTurn(t *testing.T) {
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "").withSize(60, 12)
	prompted := readLinesThenEndTurn(t, model, []string{promptLine(transcriptTimestamp(0), "go")}, turnStart.Add(time.Minute))
	dialog := readLinesThenEndTurn(t, prompted, nil, turnStart.Add(27*time.Minute))
	reprompted := readLinesThenEndTurn(t, dialog, []string{promptLine(transcriptTimestamp(30*time.Minute), "again")},
		turnStart.Add(30*time.Minute+5*time.Second))
	if screen := ansi.Strip(reprompted.screen()); !strings.Contains(screen, " Done · 5s") {
		t.Errorf("screen after a new prompt's turn lacks the done row with its time:\n%s", screen)
	}
}

func doneRowShowsTimeOfSlashCommandThatWritesRecord(t *testing.T) {
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "").withSize(60, 12)
	prompted := readLinesThenEndTurn(t, model, []string{promptLine(transcriptTimestamp(0), "go")}, turnStart.Add(time.Minute))
	effortCommand := promptLine(transcriptTimestamp(10*time.Minute),
		`<command-name>/effort</command-name>\n<command-args>high</command-args>`)
	commanded := readLinesThenEndTurn(t, prompted, []string{effortCommand}, turnStart.Add(10*time.Minute+3*time.Second))
	if screen := ansi.Strip(commanded.screen()); !strings.Contains(screen, " Done · 3s") {
		t.Errorf("screen after a slash command that wrote a record lacks the done row with its time:\n%s", screen)
	}
}

func doneRowStaysHiddenAfterSessionChangeWhenNewSessionRecordedNoTurn(t *testing.T) {
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "").withSize(60, 12)
	prompted := readLinesThenEndTurn(t, model, []string{promptLine(transcriptTimestamp(0), "go")}, turnStart.Add(time.Minute))
	changed, _ := prompted.Update(sessionChangedMsg{})
	cleared := readLinesThenEndTurn(t, changed.(zenModel), nil, turnStart.Add(2*time.Minute))
	if screen := ansi.Strip(cleared.screen()); strings.Contains(screen, "Done") {
		t.Errorf("work in a new session that recorded no turn showed a done row:\n%s", screen)
	}
}

func followTranscriptSendsTurnOfLineWithOnlyToolCall(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	if err := os.WriteFile(path, []byte(promptLine("2026-11-20T10:00:00.000Z", "go")+"\n"), 0o600); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	messages := followTranscriptMessages(t, path)
	nextEntriesRead(t, messages)
	transcript, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open transcript to append: %v", err)
	}
	_, writeErr := transcript.WriteString(`{"type":"assistant","timestamp":"2026-11-20T10:00:01.000Z","message":{` +
		`"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"C:/dev/go.mod"}}]}}` + "\n")
	if err := errors.Join(writeErr, transcript.Close()); err != nil {
		t.Fatalf("append tool call: %v", err)
	}
	read := nextEntriesRead(t, messages)
	want := claudecode.CurrentTurn{PromptTime: time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC), ToolCallActivity: "Reading go.mod"}
	if len(read.entries) != 0 || !read.turn.PromptTime.Equal(want.PromptTime) ||
		read.turn.ToolCallActivity != want.ToolCallActivity {
		t.Errorf("message after the tool call = %+v, want no entries and the turn %+v", read, want)
	}
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
	t.Run("expandKeyOnTypedTextChangesNothing", expandKeyOnTypedTextChangesNothing)
	t.Run("promptSentAfterExpandingShowsTextAndOtherLabelAndSendsBothPastes",
		promptSentAfterExpandingShowsTextAndOtherLabelAndSendsBothPastes)
	t.Run("draftAfterExpandingHoldsTextAndOtherLabel", draftAfterExpandingHoldsTextAndOtherLabel)
	t.Run("rebindExpandKeyLeavesCtrlEToTheInput", rebindExpandKeyLeavesCtrlEToTheInput)
	t.Run("expandKeyInOrDirectlyAfterExpandedTextTurnsItBackIntoItsLabelWithCursorAfterIt",
		expandKeyInOrDirectlyAfterExpandedTextTurnsItBackIntoItsLabelWithCursorAfterIt)
	t.Run("collapsingOneOfTwoExpandedPastesLeavesOtherExpandedAndCollapsible",
		collapsingOneOfTwoExpandedPastesLeavesOtherExpandedAndCollapsible)
	t.Run("expandKeyChangesNothingOnExpandedTextEditedInside", expandKeyChangesNothingOnExpandedTextEditedInside)
	t.Run("expandedTextStaysCollapsibleAfterEditsOutsideItAndCursorMoves",
		expandedTextStaysCollapsibleAfterEditsOutsideItAndCursorMoves)
	t.Run("pastesBeforeExpandedTextLeaveItCollapsibleAndPasteInsideMakesItPlain",
		pastesBeforeExpandedTextLeaveItCollapsibleAndPasteInsideMakesItPlain)
	t.Run("expandKeyPressedRepeatedlyAlternatesSameTextAndLabel", expandKeyPressedRepeatedlyAlternatesSameTextAndLabel)
	t.Run("promptSentAfterCollapsingSendsWholePaste", promptSentAfterCollapsingSendsWholePaste)
	t.Run("sendingExpandedTextLeavesNothingToCollapse", sendingExpandedTextLeavesNothingToCollapse)
}

const firstExpandedPrompt = "look at " + firstPaste + " and [Pasted text #2 +3 lines]"

func expandKeyInOrDirectlyAfterExpandedTextTurnsItBackIntoItsLabelWithCursorAfterIt(t *testing.T) {
	cursorPlacements := []struct {
		name  string
		place func(input *textarea.Model)
	}{
		{name: "directly after", place: func(input *textarea.Model) {}},
		{name: "at its start", place: func(input *textarea.Model) {
			input.MoveToBegin()
			input.SetCursorColumn(len("look at "))
		}},
		{name: "on an inner line", place: func(input *textarea.Model) { input.CursorUp() }},
	}
	for _, placement := range cursorPlacements {
		t.Run(placement.name, func(t *testing.T) {
			expanded := pressedKeys(t, modelWithTwoPastes(t, "ctrl+e", len("look at ")), ctrlE)
			placement.place(&expanded.input)
			collapsed := pressedKeys(t, expanded, ctrlE)
			assertInputAndPastes(t, collapsed, twoLabelsPrompt, []string{firstPaste, secondPaste})
			afterLabel := len("look at [Pasted text #1 +3 lines]")
			if line, column := collapsed.input.Line(), collapsed.input.Column(); line != 0 || column != afterLabel {
				t.Errorf("cursor at line %d column %d, want line 0 column %d, after the label", line, column, afterLabel)
			}
		})
	}
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

func expandKeyChangesNothingOnExpandedTextEditedInside(t *testing.T) {
	edits := []struct {
		name       string
		keys       []tea.KeyPressMsg
		wantEdited string
	}{
		{name: "typed inside", keys: []tea.KeyPressMsg{pressedKey(tea.KeyLeft), typedKey('X')},
			wantEdited: "look at line one\nline two\nline three\nline fouXr and [Pasted text #2 +3 lines]"},
		{name: "last character deleted", keys: []tea.KeyPressMsg{pressedKey(tea.KeyBackspace)},
			wantEdited: "look at line one\nline two\nline three\nline fou and [Pasted text #2 +3 lines]"},
		{name: "typed and deleted again", keys: []tea.KeyPressMsg{pressedKey(tea.KeyLeft), typedKey('X'),
			pressedKey(tea.KeyBackspace)}, wantEdited: firstExpandedPrompt},
	}
	for _, edit := range edits {
		t.Run(edit.name, func(t *testing.T) {
			expanded := pressedKeys(t, modelWithTwoPastes(t, "ctrl+e", len("look at ")), ctrlE)
			edited := pressedKeys(t, expanded, edit.keys...)
			assertInputAndPastes(t, edited, edit.wantEdited, []string{"", secondPaste})
			pressed := pressedKeys(t, edited, ctrlE)
			assertInputAndPastes(t, pressed, edit.wantEdited, []string{"", secondPaste})
		})
	}
}

func expandedTextStaysCollapsibleAfterEditsOutsideItAndCursorMoves(t *testing.T) {
	expanded := pressedKeys(t, modelWithTwoPastes(t, "ctrl+e", len("look at ")), ctrlE)
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
	before := pressedKeys(t, modelWithTwoPastes(t, "ctrl+e", len("look at ")), ctrlE)
	before.input.MoveToBegin()
	shortPasted, _ := before.Update(tea.PasteMsg{Content: "hi,"})
	labelPasted, _ := shortPasted.(zenModel).Update(tea.PasteMsg{Content: fourLinePaste})
	pastedBefore := labelPasted.(zenModel)
	pastedBefore.input.MoveToBegin()
	pastedBefore.input.SetCursorColumn(len("hi,[Pasted text #3 +3 lines]look at l"))
	collapsed := pressedKeys(t, pastedBefore, ctrlE)
	assertInputAndPastes(t, collapsed, "hi,[Pasted text #3 +3 lines]"+twoLabelsPrompt, []string{firstPaste, secondPaste, firstPaste})
	inside := pressedKeys(t, modelWithTwoPastes(t, "ctrl+e", len("look at ")), ctrlE)
	inside.input.CursorUp()
	insidePasted, _ := inside.Update(tea.PasteMsg{Content: "hi,"})
	pressed := pressedKeys(t, insidePasted.(zenModel), ctrlE)
	assertInputAndPastes(t, pressed, "look at line one\nline two\nline threhi,e\nline four and [Pasted text #2 +3 lines]",
		[]string{"", secondPaste})
}

func expandKeyPressedRepeatedlyAlternatesSameTextAndLabel(t *testing.T) {
	model := modelWithTwoPastes(t, "ctrl+e", len("look at "))
	for press := 1; press <= 4; press++ {
		model = pressedKeys(t, model, ctrlE)
		if press%2 == 1 {
			assertInputAndPastes(t, model, firstExpandedPrompt, []string{"", secondPaste})
		} else {
			assertInputAndPastes(t, model, twoLabelsPrompt, []string{firstPaste, secondPaste})
		}
	}
}

func promptSentAfterCollapsingSendsWholePaste(t *testing.T) {
	recorder := &agentRecorder{}
	model := pressedKeys(t, modelWithTwoPastes(t, "ctrl+e", len("look at ")), ctrlE, ctrlE)
	model.agent = recorder
	_, send := model.withKey(pressedKey(tea.KeyEnter))
	send()
	if want := []string{"prompt look at " + firstPaste + " and " + secondPaste}; !reflect.DeepEqual(recorder.calls, want) {
		t.Errorf("sent %q, want %q", recorder.calls, want)
	}
}

func sendingExpandedTextLeavesNothingToCollapse(t *testing.T) {
	model := pressedKeys(t, modelWithTwoPastes(t, "ctrl+e", len("look at ")), ctrlE)
	model.agent = &agentRecorder{}
	sent, send := model.withKey(pressedKey(tea.KeyEnter))
	send()
	if expandedPastes := sent.(zenModel).expandedPastes; expandedPastes != nil {
		t.Errorf("expanded pastes after sending = %+v, want none", expandedPastes)
	}
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
	if value := model.input.Value(); value != twoLabelsPrompt {
		t.Fatalf("input after two pastes = %q, want %q", value, twoLabelsPrompt)
	}
	model.input.MoveToBegin()
	model.input.SetCursorColumn(column)
	return model
}

func assertInputAndPastes(t *testing.T, model zenModel, wantValue string, wantPastes []string) {
	t.Helper()
	if value := model.input.Value(); value != wantValue {
		t.Errorf("input = %q, want %q", value, wantValue)
	}
	if !reflect.DeepEqual(model.pastes, wantPastes) {
		t.Errorf("pastes = %q, want %q", model.pastes, wantPastes)
	}
}

func expandKeyOnLabelTurnsOnlyThatLabelIntoItsTextWithCursorAfterIt(t *testing.T) {
	expanded := pressedKeys(t, modelWithTwoPastes(t, "ctrl+e", len("look at [Pas")), ctrlE)
	assertInputAndPastes(t, expanded, "look at "+firstPaste+" and [Pasted text #2 +3 lines]", []string{"", secondPaste})
	if line, column := expanded.input.Line(), expanded.input.Column(); line != 3 || column != len("line four") {
		t.Errorf("cursor at line %d column %d, want line 3 column %d, after the paste's text", line, column,
			len("line four"))
	}
}

func expandKeyDirectlyAfterLabelTurnsItIntoItsText(t *testing.T) {
	afterFirst := pressedKeys(t, modelWithTwoPastes(t, "ctrl+e", len("look at [Pasted text #1 +3 lines]")), ctrlE)
	assertInputAndPastes(t, afterFirst, "look at "+firstPaste+" and [Pasted text #2 +3 lines]", []string{"", secondPaste})
	afterSecond := pressedKeys(t, modelWithTwoPastes(t, "ctrl+e", len(twoLabelsPrompt)), ctrlE)
	assertInputAndPastes(t, afterSecond, "look at [Pasted text #1 +3 lines] and "+secondPaste, []string{firstPaste, ""})
}

func expandKeyOnTypedTextChangesNothing(t *testing.T) {
	for _, column := range []int{0, len("look a"), len("look at [Pasted text #1 +3 lines] a")} {
		model := modelWithTwoPastes(t, "ctrl+e", column)
		pressed := pressedKeys(t, model, ctrlE)
		assertInputAndPastes(t, pressed, twoLabelsPrompt, []string{firstPaste, secondPaste})
		if pressed.input.Column() != column {
			t.Errorf("cursor moved from column %d to %d", column, pressed.input.Column())
		}
	}
}

func promptSentAfterExpandingShowsTextAndOtherLabelAndSendsBothPastes(t *testing.T) {
	recorder := &agentRecorder{}
	model := pressedKeys(t, modelWithTwoPastes(t, "ctrl+e", len("look at ")), ctrlE)
	model.agent = recorder
	sent, send := model.withKey(pressedKey(tea.KeyEnter))
	send()
	text := "look at " + firstPaste + " and " + secondPaste
	if want := []string{"prompt " + text}; !reflect.DeepEqual(recorder.calls, want) {
		t.Errorf("sent %q, want %q", recorder.calls, want)
	}
	bandText := entriesText(t, sent.(zenModel))
	if !strings.Contains(bandText, "> look at line one") || !strings.Contains(bandText, "line four and [Pasted text #2 +3 lines]") {
		t.Errorf("band lacks the expanded text or the second label:\n%s", bandText)
	}
	recorded := sent.(zenModel).withEntries([]claudecode.DialogueEntry{
		{Kind: claudecode.PromptEntry, Text: text, Pastes: []string{text}}}, nil)
	entryText := entriesText(t, recorded)
	if !strings.Contains(entryText, "> look at line one") || !strings.Contains(entryText, "line four and [Pasted text #2 +3 lines]") {
		t.Errorf("transcript prompt lacks the expanded text or the second label:\n%s", entryText)
	}
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
	if !reflect.DeepEqual(saved, want) {
		t.Errorf("saved draft = %+q, want %+q", saved, want)
	}
}

func rebindExpandKeyLeavesCtrlEToTheInput(t *testing.T) {
	model := modelWithTwoPastes(t, "ctrl+o", len("look at "))
	assertInputAndPastes(t, pressedKeys(t, model, ctrlE), twoLabelsPrompt, []string{firstPaste, secondPaste})
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
	if after := recorded.screen(); after != before {
		t.Errorf("screen moved when the transcript prompt replaced the pasted prompt's band:\nbefore:\n%s\nafter:\n%s",
			before, after)
	}
	if newestRows := plainRows(recorded.entryRows[len(recorded.entryRows)-1]); !reflect.DeepEqual(newestRows,
		[]string{"", " > [Pasted text #1 +3 lines]"}) {
		t.Errorf("rows of the recorded prompt = %q, want its paste's label", newestRows)
	}
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
	if value := model.input.Value(); value != "look at [Pasted text #1 +3 lines]" {
		t.Fatalf("input after the paste = %q, want the paste's label", value)
	}
	sent, send := model.withKey(pressedKey(tea.KeyEnter))
	send()
	if want := []string{"prompt look at line one\nline two\nline three\nline four"}; !reflect.DeepEqual(recorder.calls, want) {
		t.Errorf("sent %q, want %q", recorder.calls, want)
	}
	if row := bandTextRow(t, sent.(zenModel), "look at [Pasted text #1 +3 lines]"); row == "" {
		t.Errorf("screen after Enter lacks the band with the paste's label:\n%s", ansi.Strip(sent.(zenModel).screen()))
	}
}

func ctrlEnterSendsWholeTextAndRefusalCarriesPastes(t *testing.T) {
	recorder := &agentRecorder{promptErr: ErrAgentBlocked}
	model := modelWithPaste("look at ", fourLinePaste)
	model.agent = recorder
	_, send := model.withKey(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModCtrl})
	paste := "line one\nline two\nline three\nline four"
	want := promptSentMsg{prompt: "look at " + paste, pastes: []string{paste}, err: ErrAgentBlocked}
	if msg := send(); !reflect.DeepEqual(msg, want) {
		t.Errorf("ctrl+enter's command returned %#v, want %#v", msg, want)
	}
	if wantCalls := []string{"prompt look at " + paste}; !reflect.DeepEqual(recorder.calls, wantCalls) {
		t.Errorf("sent %q, want %q", recorder.calls, wantCalls)
	}
}

func shortPasteShowsAsItsText(t *testing.T) {
	model := modelWithPaste("look at ", "line one\nline two")
	if value := model.input.Value(); value != "look at line one\nline two" {
		t.Errorf("input after the paste = %q, want the paste's text", value)
	}
}

func failedSendPutsPastesBackAsLabelsNumberedInOrder(t *testing.T) {
	model := modelWithPaste("look at ", fourLinePaste)
	sent, _ := model.withKey(pressedKey(tea.KeyEnter))
	sentModel := sent.(zenModel)
	pastedSince, _ := sentModel.Update(tea.PasteMsg{Content: "w\nx\ny\nz"})
	refused, _ := pastedSince.Update(promptSentMsg{prompt: "look at line one\nline two\nline three\nline four",
		pastes: []string{"line one\nline two\nline three\nline four"}, err: ErrAgentBlocked})
	refusedModel := refused.(zenModel)
	if want := "look at [Pasted text #1 +3 lines]\n[Pasted text #2 +3 lines]"; refusedModel.input.Value() != want {
		t.Errorf("input after the refused send = %q, want %q", refusedModel.input.Value(), want)
	}
	if want := "look at line one\nline two\nline three\nline four\nw\nx\ny\nz"; refusedModel.InputText() != want {
		t.Errorf("text Enter would send = %q, want %q", refusedModel.InputText(), want)
	}
}

func transcriptPromptReplacingBandShowsBandsPastesAsLabels(t *testing.T) {
	sent, _ := modelWithPaste("look at ", fourLinePaste).withKey(pressedKey(tea.KeyEnter))
	// herdr sends the whole prompt as one paste, so Claude Code wraps all of it in one tag.
	text := "look at line one\nline two\nline three\nline four"
	recorded := sent.(zenModel).withEntries([]claudecode.DialogueEntry{
		{Kind: claudecode.PromptEntry, Text: text, Pastes: []string{text}}}, nil)
	screenText := entriesText(t, recorded)
	if !strings.Contains(screenText, "> look at [Pasted text #1 +3 lines]") || strings.Contains(screenText, "line two") {
		t.Errorf("screen after the transcript records the prompt lacks its paste's label or shows the paste:\n%s",
			screenText)
	}
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
	if !reflect.DeepEqual(saved, want) {
		t.Errorf("saved drafts = %+q, want %+q", saved, want)
	}
}

func restoredDraftShowsLabelsAndEnterSendsPastesExpanded(t *testing.T) {
	recorder := &agentRecorder{}
	paste := "line one\nline two\nline three\nline four"
	model := newZenModel(recorder, testAppearance, "alt+x", "ctrl+e", "").withSize(60, 20).
		withDraftRestored(Draft{Text: "look at [Pasted text #1 +3 lines]", Pastes: []string{paste}}, nil)
	if value := model.input.Value(); value != "look at [Pasted text #1 +3 lines]" {
		t.Fatalf("input of the restored draft = %q, want the paste's label", value)
	}
	_, send := model.withKey(pressedKey(tea.KeyEnter))
	send()
	if want := []string{"prompt look at " + paste}; !reflect.DeepEqual(recorder.calls, want) {
		t.Errorf("sent %q, want %q", recorder.calls, want)
	}
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
	if draft.Text != "hi\nx" || len(draft.Pastes) != 0 {
		t.Errorf("draft = %+q, want %q and no pastes", draft, "hi\nx")
	}
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
	if saves != 0 {
		t.Errorf("saves after updates that leave the draft as it was = %d, want 0", saves)
	}
}

func draftReadErrorShowsOnStatusLineUntilNextKeyOrNewerError(t *testing.T) {
	draftErr := errors.New("Cannot read the draft C:/state/draft.default.w1-p1: invalid character 'a'")
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "").withSize(60, 20).withDraftRestored(Draft{}, draftErr)
	if shown := ansi.Strip(model.shownStatus()); shown != draftErr.Error() {
		t.Errorf("status line = %q, want %q", shown, draftErr.Error())
	}
	if value := model.input.Value(); value != "" {
		t.Errorf("input = %q, want it empty", value)
	}
	typed := pressedKeys(t, model, tea.KeyPressMsg{Code: 'h', Text: "h"})
	if shown := ansi.Strip(typed.shownStatus()); strings.Contains(shown, "Cannot read the draft") {
		t.Errorf("status line after a key = %q, want the draft error gone", shown)
	}
	problem := errors.New("herdr pane get failed")
	updated, _ := model.Update(problemMsg{err: problem})
	if shown := ansi.Strip(updated.(zenModel).shownStatus()); shown != problem.Error() {
		t.Errorf("status line after a newer error = %q, want %q", shown, problem.Error())
	}
}

func TestFatal(t *testing.T) {
	t.Run("fatalErrorWrapsAtViewWidthSoItsEndShows", fatalErrorWrapsAtViewWidthSoItsEndShows)
}

func fatalErrorWrapsAtViewWidthSoItsEndShows(t *testing.T) {
	model := newZenModel(nil, testAppearance, "alt+x", "ctrl+e", "").withSize(30, 12)
	model.fatal = errors.New(`C:\Users\someone\AppData\Roaming\calmpane\config.toml: colors.link = "#9ccfd" is not a color`)
	rows := strings.Split(model.View().Content, "\n")
	for _, row := range rows {
		if rowWidth := ansi.StringWidth(row); rowWidth > 30 {
			t.Errorf("row %q is %d columns wide, more than the view's 30", row, rowWidth)
		}
	}
	if shown := strings.Join(plainRows(rows), " "); !strings.Contains(shown, `colors.link = "#9ccfd" is not a color`) {
		t.Errorf("fatal rows = %q, want the whole error", rows)
	}
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
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(line+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// modelAfterMessagesUntil applies the messages followAgentSession sends to model until done holds for the model a
// message gives and the message.
func modelAfterMessagesUntil(t *testing.T, model zenModel, messages <-chan tea.Msg,
	done func(model zenModel, msg tea.Msg) bool) zenModel {
	t.Helper()
	for {
		select {
		case msg := <-messages:
			if fatal, isFatal := msg.(fatalMsg); isFatal {
				t.Fatalf("followAgentSession: %v", fatal.err)
			}
			updated, _ := model.Update(msg)
			model = updated.(zenModel)
			if done(model, msg) {
				return model
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("after 10 s the view shows:\n%s", ansi.Strip(model.screen()))
		}
	}
}

func sessionChangeDropsEntriesAndFollowsNewTranscriptAndStatusLine(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	statusLineDir := t.TempDir()
	writeTranscriptLine(t, home, "first", `{"type":"user","message":{"role":"user","content":"before the clear"}}`)
	for sessionID, line := range map[string]string{"first": "first status\n", "second": "second status\n"} {
		if err := os.WriteFile(filepath.Join(statusLineDir, sessionID), []byte(line), 0o600); err != nil {
			t.Fatal(err)
		}
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
	if screen := ansi.Strip(model.screen()); strings.Contains(screen, "before the clear") {
		t.Errorf("view after the session change still shows the old session's prompt:\n%s", screen)
	}
	model = modelAfterMessagesUntil(t, model, messages, func(model zenModel, _ tea.Msg) bool {
		return showsBoth(model, "after the clear", "second status")
	})
	if screen := ansi.Strip(model.screen()); strings.Contains(screen, "before the clear") {
		t.Errorf("view of the new session shows the old session's prompt:\n%s", screen)
	}
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
	if err := os.WriteFile(path, []byte(`{"type":"user","message":{"role":"user","content":"first"}}`+"\n"), 0o600); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	transcript, err := os.Open(path)
	if err != nil {
		t.Fatalf("open transcript: %v", err)
	}
	t.Cleanup(func() {
		if err := transcript.Close(); err != nil {
			t.Errorf("close transcript: %v", err)
		}
	})
	newSession := func() (string, error) { return "after clear", nil }
	next, err := followTranscript(transcript, "before clear", newSession, func(tea.Msg) {})
	if err != nil {
		t.Fatalf("followTranscript: %v", err)
	}
	if next != "after clear" {
		t.Errorf("followTranscript returned session %q, want %q", next, "after clear")
	}
}
