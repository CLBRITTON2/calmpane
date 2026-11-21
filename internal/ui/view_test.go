package ui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/CLBRITTON2/herdr-zen/internal/claudecode"
	"github.com/CLBRITTON2/herdr-zen/internal/herdr"
	"github.com/CLBRITTON2/herdr-zen/internal/herdr/herdrtest"
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
		messages <- fatalMsg{err: followTranscript(transcript, func(msg tea.Msg) { messages <- msg })}
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
	t.Run("enterGivesSlashCommandNoEarlyBand", enterGivesSlashCommandNoEarlyBand)
	t.Run("failedSendRemovesEarlyBandAndShowsRefusal", failedSendRemovesEarlyBandAndShowsRefusal)
	t.Run("shiftEnterStartsNewLineAndEnterSendsTextWithItsNewlines", shiftEnterStartsNewLineAndEnterSendsTextWithItsNewlines)
	t.Run("ctrlEnterSendsPromptThenSendNowKeys", ctrlEnterSendsPromptThenSendNowKeys)
}

func enterClearsInputAtOnceAndFailedSendPutsWholeTextBack(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		typedSince string
		wantInput  string
		wantShown  string
	}{
		{name: "refused while blocked", err: herdr.ErrAgentBlocked, wantInput: "keep me\nand this line",
			wantShown: "Claude is waiting for you. Press alt+x to answer."},
		{name: "herdr failed", err: errors.New("herdr agent prompt failed: exit status 1"),
			wantInput: "keep me\nand this line", wantShown: "herdr agent prompt failed: exit status 1"},
		{name: "refused after more typing", err: herdr.ErrAgentBlocked, typedSince: "more",
			wantInput: "keep me\nand this line\nmore", wantShown: "Claude is waiting for you. Press alt+x to answer."},
		{name: "sent", err: nil, wantInput: "", wantShown: ""},
		{name: "sent after more typing", err: nil, typedSince: "more", wantInput: "more", wantShown: ""},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			model := zenModel{entries: []claudecode.DialogueEntry{}, entryRows: [][]string{}, input: NewPromptInput()}
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
	t.Run("screenScrolledUpStaysOnItsRowsWhenQueuedNoteRewrapsBand", screenScrolledUpStaysOnItsRowsWhenQueuedNoteRewrapsBand)
	t.Run("screenScrolledUpStaysOnItsRowsWhenCancelledPromptIsDropped", screenScrolledUpStaysOnItsRowsWhenCancelledPromptIsDropped)
}

func screenScrolledUpStaysOnItsRowsWhenEntriesArriveAndEndReturnsToNewest(t *testing.T) {
	model := zenModel{entries: []claudecode.DialogueEntry{}, entryRows: [][]string{}, input: NewPromptInput()}
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
	model := zenModel{entries: []claudecode.DialogueEntry{}, entryRows: [][]string{}, input: NewPromptInput()}
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
	model := zenModel{entries: []claudecode.DialogueEntry{}, entryRows: [][]string{}, input: NewPromptInput()}
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
	t.Run("ctrlYCopiesNewestCodeBlockAndShowsCopiedUntilNextKey", ctrlYCopiesNewestCodeBlockAndShowsCopiedUntilNextKey)
	t.Run("ctrlYSaysSoAndCopiesNothingWithNoCodeBlock", ctrlYSaysSoAndCopiesNothingWithNoCodeBlock)
	t.Run("newStatusReplacesCopyStatus", newStatusReplacesCopyStatus)
	t.Run("copyHintMovesToNewestCodeBlockWhenReplyBringsOne", copyHintMovesToNewestCodeBlockWhenReplyBringsOne)
}

func ctrlYCopiesNewestCodeBlockAndShowsCopiedUntilNextKey(t *testing.T) {
	model := zenModel{entries: []claudecode.DialogueEntry{}, entryRows: [][]string{}, input: NewPromptInput()}
	model = model.withSize(60, 30).withEntries([]claudecode.DialogueEntry{
		{Kind: claudecode.ReplyEntry, Text: "Old:\n\n```\nold block\n```"},
		{Kind: claudecode.ReplyEntry, Text: "New:\n\n```go\nfirst()\n```\n\n```go\nfunc f() {\n\treturn \n}\n```\n\nDone."},
		{Kind: claudecode.PromptEntry, Text: "thanks\n\n```\nmine\n```"},
	}, nil)
	copied, copyCmd := model.Update(tea.KeyPressMsg{Code: 'y', Mod: tea.ModCtrl})
	copiedModel := copied.(zenModel)
	if copyCmd == nil {
		t.Fatalf("ctrl+y returned no command, want the clipboard set")
	}
	if got, want := copyCmd(), tea.SetClipboard("func f() {\n\treturn \n}")(); !reflect.DeepEqual(got, want) {
		t.Errorf("ctrl+y sets the clipboard with %#v, want %#v", got, want)
	}
	if !strings.Contains(ansi.Strip(copiedModel.screen()), "Copied the last code block.") {
		t.Errorf("screen after ctrl+y lacks the copy status:\n%s", ansi.Strip(copiedModel.screen()))
	}
	typed, _ := copiedModel.Update(pressedKey('a'))
	if strings.Contains(ansi.Strip(typed.(zenModel).screen()), "Copied the last code block.") {
		t.Errorf("screen after the next key still shows the copy status:\n%s", ansi.Strip(typed.(zenModel).screen()))
	}
}

func ctrlYSaysSoAndCopiesNothingWithNoCodeBlock(t *testing.T) {
	model := zenModel{entries: []claudecode.DialogueEntry{}, entryRows: [][]string{}, input: NewPromptInput()}
	model = model.withSize(60, 12).withEntries([]claudecode.DialogueEntry{
		{Kind: claudecode.PromptEntry, Text: "hello"}, {Kind: claudecode.ReplyEntry, Text: "Use `inline code` only."},
	}, nil)
	pressed, copyCmd := model.Update(tea.KeyPressMsg{Code: 'y', Mod: tea.ModCtrl})
	if copyCmd != nil {
		t.Errorf("ctrl+y with no code block returned %#v, want no command", copyCmd())
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
		{name: "agent status", statusMsg: agentStatusMsg{status: "working"}, wantShown: "Working..."},
		{name: "problem", statusMsg: problemMsg{err: errors.New("herdr pane get failed")},
			wantShown: "herdr pane get failed"},
		{name: "prompt refused", statusMsg: promptSentMsg{err: herdr.ErrAgentBlocked},
			wantShown: "Claude is waiting for you. Press alt+x to answer."},
		{name: "send-now keys refused", statusMsg: claudeKeysSentMsg{err: errors.New("herdr pane send-keys failed")},
			wantShown: "herdr pane send-keys failed"},
		{name: "Claude's input not taken",
			statusMsg: agentInputTakenMsg{err: errors.New("Claude's input in pane 3 still holds text after ctrl+u")},
			wantShown: "still holds text after ctrl+u"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			model := zenModel{entries: []claudecode.DialogueEntry{}, entryRows: [][]string{}, input: NewPromptInput()}
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
	model := zenModel{entries: []claudecode.DialogueEntry{}, entryRows: [][]string{}, input: NewPromptInput()}
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

func modelWithTypedPrompt(prompt string) zenModel {
	model := zenModel{entries: []claudecode.DialogueEntry{}, entryRows: [][]string{}, input: NewPromptInput()}
	model = model.withSize(60, 20).withEntries([]claudecode.DialogueEntry{{Kind: claudecode.ReplyEntry, Text: "Ready."}}, nil)
	model.input.SetValue(prompt)
	return model
}

func enterShowsPromptBandAtOnceAndTranscriptPromptTakesItsPlace(t *testing.T) {
	sent, sendPrompt := modelWithTypedPrompt("rename the widget").withKey(pressedKey(tea.KeyEnter))
	if sendPrompt == nil {
		t.Fatal("Enter returned no command to send the prompt")
	}
	if screenText := ansi.Strip(sent.(zenModel).screen()); !strings.Contains(screenText, "▌ rename the widget") {
		t.Fatalf("screen after Enter lacks the prompt's band:\n%s", screenText)
	}
	delivered, _ := sent.Update(promptSentMsg{prompt: "rename the widget", err: nil})
	sentModel := delivered.(zenModel)
	// a reply written before Claude records the prompt goes above the early band
	withReply := sentModel.withEntries([]claudecode.DialogueEntry{{Kind: claudecode.ReplyEntry, Text: "Still on the last step."}}, nil)
	replyText := ansi.Strip(withReply.screen())
	replyAt := strings.Index(replyText, "Still on the last step.")
	bandAt := strings.Index(replyText, "▌ rename the widget")
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

func enterGivesSlashCommandNoEarlyBand(t *testing.T) {
	sent, sendPrompt := modelWithTypedPrompt("/compact").withKey(pressedKey(tea.KeyEnter))
	if sendPrompt == nil {
		t.Fatal("Enter returned no command to send the slash command")
	}
	if screenText := ansi.Strip(sent.(zenModel).screen()); strings.Contains(screenText, "▌ /compact") {
		t.Errorf("screen after Enter on a slash command shows a band:\n%s", screenText)
	}
}

func failedSendRemovesEarlyBandAndShowsRefusal(t *testing.T) {
	sent, _ := modelWithTypedPrompt("rename the widget").withKey(pressedKey(tea.KeyEnter))
	refused, _ := sent.(zenModel).Update(promptSentMsg{prompt: "rename the widget", err: herdr.ErrAgentBlocked})
	refusedModel := refused.(zenModel)
	screenText := ansi.Strip(refusedModel.screen())
	if strings.Contains(screenText, "▌ rename the widget") {
		t.Errorf("screen after a refused send still shows the early band:\n%s", screenText)
	}
	if !strings.Contains(screenText, "Claude is waiting for you. Press alt+x to answer.") {
		t.Errorf("screen after a refused send lacks the refusal:\n%s", screenText)
	}
}

func screenScrolledUpStaysOnItsRowsWhenEarlyBandComesAndGoes(t *testing.T) {
	model := zenModel{entries: []claudecode.DialogueEntry{}, entryRows: [][]string{}, input: NewPromptInput()}
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
	rows := strings.Split(ansi.Strip(model.screen()), "\n")
	for index, row := range rows {
		if strings.HasPrefix(row, "───") {
			return rows[index+1:]
		}
	}
	t.Fatalf("screen has no separator:\n%s", strings.Join(rows, "\n"))
	return nil
}

func TestInput(t *testing.T) {
	t.Run("inputWrapsLongLineUnderItsPrompt", inputWrapsLongLineUnderItsPrompt)
	t.Run("inputGrowsRowAtATimeUpToEightAndScreenKeepsItsHeight", inputGrowsRowAtATimeUpToEightAndScreenKeepsItsHeight)
	t.Run("upAndDownMoveCursorInInputOfSeveralRows", upAndDownMoveCursorInInputOfSeveralRows)
	t.Run("entriesFillRowsInputGivesBack", entriesFillRowsInputGivesBack)
}

func inputWrapsLongLineUnderItsPrompt(t *testing.T) {
	words := strings.Repeat("wrap these words ", 12)
	for _, width := range []int{40, 90} {
		model := zenModel{entries: []claudecode.DialogueEntry{}, entryRows: [][]string{}, input: NewPromptInput()}
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
	model := zenModel{entries: []claudecode.DialogueEntry{}, entryRows: [][]string{}, input: NewPromptInput()}
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
	model := zenModel{claude: &claudeInputRecorder{}, entries: []claudecode.DialogueEntry{}, entryRows: [][]string{},
		input: NewPromptInput()}
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

func upAndDownMoveCursorInInputOfSeveralRows(t *testing.T) {
	model := zenModel{entries: []claudecode.DialogueEntry{}, entryRows: [][]string{}, input: NewPromptInput()}
	model = model.withSize(40, 12).withEntries(numberedPrompts(1, 20), nil)
	model.input.SetValue("first\nsecond")
	movedUp, _ := model.withKey(pressedKey(tea.KeyUp))
	movedUpModel := movedUp.(zenModel)
	if movedUpModel.rowsBelowView != 0 || movedUpModel.input.Line() != 0 {
		t.Errorf("after Up: rows below view %d, input line %d, want 0 and 0", movedUpModel.rowsBelowView,
			movedUpModel.input.Line())
	}
	movedDown, _ := movedUpModel.withKey(pressedKey(tea.KeyDown))
	if line := movedDown.(zenModel).input.Line(); line != 1 {
		t.Errorf("after Down: input line %d, want 1", line)
	}
}

// modelWithInputThenEntries sets the input before the entries arrive, so they are drawn under its final height.
func modelWithInputThenEntries(inputValue string) zenModel {
	model := zenModel{entries: []claudecode.DialogueEntry{}, entryRows: [][]string{}, input: NewPromptInput()}
	model = model.withSize(60, 24)
	model.input.SetValue(inputValue)
	return model.withEntries(numberedPrompts(1, 20), nil)
}

func entriesFillRowsInputGivesBack(t *testing.T) {
	model := zenModel{claude: &claudeInputRecorder{}, entries: []claudecode.DialogueEntry{}, entryRows: [][]string{},
		input: NewPromptInput()}
	model = model.withSize(60, 24)
	// Entries that arrive under a tall input are drawn only down to its top row. A slash command gets no early band
	// to draw the entries on Enter.
	model.input.SetValue("/x" + strings.Repeat("\nx", inputMaxRows-1))
	model = model.withEntries(numberedPrompts(1, 20), nil)
	for range 8 {
		erased, _ := model.withKey(pressedKey(tea.KeyBackspace))
		model = erased.(zenModel)
	}
	if screen, want := model.screen(), modelWithInputThenEntries(model.input.Value()).screen(); screen != want {
		t.Errorf("screen after Backspace shrank the input:\n%s\nwant:\n%s", screen, want)
	}
	sent, _ := model.withKey(pressedKey(tea.KeyEnter))
	if screen, want := sent.(zenModel).screen(), modelWithInputThenEntries("").screen(); screen != want {
		t.Errorf("screen after Enter cleared the input:\n%s\nwant:\n%s", screen, want)
	}
}

// claudeInputRecorder records each send as "prompt <pane> <text>" or "keys <pane> <keys>".
type claudeInputRecorder struct {
	calls     []string
	promptErr error
	// screens are the pane reads StyledScreen returns in turn, the last one again once they run out.
	screens []string
}

func (r *claudeInputRecorder) StyledScreen(paneID string) (string, error) {
	if len(r.screens) == 0 {
		return "", nil
	}
	screen := r.screens[0]
	if len(r.screens) > 1 {
		r.screens = r.screens[1:]
	}
	return screen, nil
}

func (r *claudeInputRecorder) PromptAgent(paneID string, prompt string) error {
	r.calls = append(r.calls, "prompt "+paneID+" "+prompt)
	return r.promptErr
}

func (r *claudeInputRecorder) SendKeys(paneID string, keys []string) error {
	r.calls = append(r.calls, "keys "+paneID+" "+strings.Join(keys, " "))
	return nil
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

func bandTextRow(model zenModel, text string) string {
	for _, row := range strings.Split(ansi.Strip(model.screen()), "\n") {
		if strings.Contains(row, "▌ "+text) {
			return row
		}
	}
	return ""
}

func TestQueuedPrompts(t *testing.T) {
	t.Run("enqueueMarksBandWithItsTextAndMatchingRemoveClearsIt", enqueueMarksBandWithItsTextAndMatchingRemoveClearsIt)
	t.Run("dequeueClearsOldestMarkedBand", dequeueClearsOldestMarkedBand)
	t.Run("removeThatDropsPromptRemovesItsBand", removeThatDropsPromptRemovesItsBand)
	t.Run("queueOperationsApplyInTheirPlaceAmongEntries", queueOperationsApplyInTheirPlaceAmongEntries)
}

func enqueueMarksBandWithItsTextAndMatchingRemoveClearsIt(t *testing.T) {
	model := modelWithSentPrompts([]string{"first one", "second one"})
	queued := model.withEntries(nil, []claudecode.QueueOperation{{Operation: "enqueue", Text: "second one"}})
	if row := bandTextRow(queued, "second one"); !strings.HasSuffix(row, " "+queuedNote+" ") {
		t.Errorf("band row after the enqueue = %q, want it to end in the queued note", row)
	}
	if row := bandTextRow(queued, "first one"); strings.Contains(row, "queued") {
		t.Errorf("band row of the other prompt = %q, want no queued note", row)
	}
	taken := queued.withEntries(nil, []claudecode.QueueOperation{
		{Operation: "remove", Reason: "absorbed_mid_turn", Text: "second one"},
	})
	if row := bandTextRow(taken, "second one"); row == "" || strings.Contains(row, "queued") {
		t.Errorf("band row after the remove = %q, want the band without the queued note", row)
	}
}

func dequeueClearsOldestMarkedBand(t *testing.T) {
	model := modelWithSentPrompts([]string{"first one", "second one"}).withEntries(nil, []claudecode.QueueOperation{
		{Operation: "enqueue", Text: "first one"}, {Operation: "enqueue", Text: "second one"}, {Operation: "dequeue"},
	})
	if row := bandTextRow(model, "first one"); strings.Contains(row, "queued") {
		t.Errorf("oldest band row after the dequeue = %q, want no queued note", row)
	}
	if row := bandTextRow(model, "second one"); !strings.Contains(row, "queued") {
		t.Errorf("newer band row after the dequeue = %q, want the queued note", row)
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
			model := modelWithSentPrompts([]string{"first one", "second one"}).withEntries(nil, []claudecode.QueueOperation{
				{Operation: "enqueue", Text: "second one"}, removal,
			})
			if row := bandTextRow(model, "second one"); row != "" {
				t.Errorf("screen still shows the removed band: %q", row)
			}
			if row := bandTextRow(model, "first one"); row == "" {
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
				[]claudecode.QueueOperation{{Operation: "enqueue", Text: "again", EntriesBefore: testCase.entriesBefore}})
			want := []earlyPrompt{{text: "again", queued: testCase.wantQueued}}
			if !reflect.DeepEqual(model.earlyPrompts, want) {
				t.Errorf("earlyPrompts = %+v, want %+v", model.earlyPrompts, want)
			}
		})
	}
}

func screenScrolledUpStaysOnItsRowsWhenQueuedNoteRewrapsBand(t *testing.T) {
	model := zenModel{entries: []claudecode.DialogueEntry{}, entryRows: [][]string{}, input: NewPromptInput()}
	model = model.withSize(40, 12).withEntries(numberedPrompts(1, 20), nil)
	// fits one band row at width 40, and wraps onto two short of the note
	prompt := "prompt 21 with words that wrap now"
	model.input.SetValue(prompt)
	sent, _ := model.withKey(pressedKey(tea.KeyEnter))
	scrolled, _ := sent.(zenModel).withKey(pressedKey(tea.KeyPgUp))
	scrolledModel := scrolled.(zenModel)
	before := scrolledModel.screen()
	queued := scrolledModel.withEntries(nil, []claudecode.QueueOperation{{Operation: "enqueue", Text: prompt}})
	if len(queued.earlyPromptRows()) == len(scrolledModel.earlyPromptRows()) {
		t.Fatalf("the queued note left the band at %d rows, want it rewrapped", len(queued.earlyPromptRows()))
	}
	if after := queued.screen(); after != before {
		t.Errorf("screen moved when the band was marked:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	taken := queued.withEntries(nil, []claudecode.QueueOperation{{Operation: "dequeue"}})
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
			wantCalls: []string{"prompt 3 ship it", "keys 3 ctrl+x ctrl+s"}, wantMsg: claudeKeysSentMsg{}},
		{name: "empty input", typed: "", wantCalls: []string{"keys 3 ctrl+x ctrl+s"}, wantMsg: claudeKeysSentMsg{}},
		{name: "refused prompt sends no keys", typed: "ship it", promptErr: herdr.ErrAgentBlocked,
			wantCalls: []string{"prompt 3 ship it"}, wantMsg: promptSentMsg{prompt: "ship it", err: herdr.ErrAgentBlocked}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := &claudeInputRecorder{promptErr: testCase.promptErr}
			model := modelWithTypedPrompt(testCase.typed)
			model.claude, model.claudePaneID = recorder, "3"
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
	t.Run("escSendsEscToClaudeOnlyWhileItWorks", escSendsEscToClaudeOnlyWhileItWorks)
	t.Run("escBeforeReplyMovesPromptClaudePutBackIntoZenInput", escBeforeReplyMovesPromptClaudePutBackIntoZenInput)
	t.Run("cancelledPromptRestoreKeepsEntriesWhenNewestIsNotThatPrompt", cancelledPromptRestoreKeepsEntriesWhenNewestIsNotThatPrompt)
	t.Run("cancelledPromptRestoreChangesNothingWhenClaudeGaveNoPromptBack", cancelledPromptRestoreChangesNothingWhenClaudeGaveNoPromptBack)
}

func escSendsEscToClaudeOnlyWhileItWorks(t *testing.T) {
	cases := []struct {
		agentStatus string
		wantCalls   []string
	}{
		{agentStatus: "working", wantCalls: []string{"keys 3 esc"}},
		{agentStatus: "idle", wantCalls: nil},
		{agentStatus: "blocked", wantCalls: nil},
	}
	for _, testCase := range cases {
		t.Run(testCase.agentStatus, func(t *testing.T) {
			recorder := &claudeInputRecorder{}
			model := modelWithTypedPrompt("half typed")
			model.claude, model.claudePaneID, model.agentStatus = recorder, "3", testCase.agentStatus
			pressed, send := model.withKey(tea.KeyPressMsg{Code: tea.KeyEscape})
			if send != nil {
				if msg := send(); !reflect.DeepEqual(msg, claudeKeysSentMsg{}) {
					t.Errorf("esc's command returned %#v, want claudeKeysSentMsg{}", msg)
				}
			}
			if !reflect.DeepEqual(recorder.calls, testCase.wantCalls) {
				t.Errorf("sent %q, want %q", recorder.calls, testCase.wantCalls)
			}
			if value := pressed.(zenModel).input.Value(); value != "half typed" {
				t.Errorf("input after esc = %q, want it kept", value)
			}
		})
	}
}

func escBeforeReplyMovesPromptClaudePutBackIntoZenInput(t *testing.T) {
	recorder := &claudeInputRecorder{screens: []string{
		herdrtest.CapturedScreen(t, "claude-input-one-row.ansi"), herdrtest.CapturedScreen(t, "claude-input-empty.ansi"),
	}}
	model := modelWithTypedPrompt("typed since")
	model = model.withEntries([]claudecode.DialogueEntry{{Kind: claudecode.PromptEntry, Text: "move this draft"}}, nil)
	model.claude, model.claudePaneID, model.agentStatus = recorder, "3", "working"
	pressed, sendEsc := model.withKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	sendEsc()
	stopped, takeInput := pressed.(zenModel).Update(agentStatusMsg{status: "done"})
	if takeInput == nil {
		t.Fatal("Claude stopping after Esc returned no command to take its input")
	}
	taken := takeInput()
	if !reflect.DeepEqual(taken, agentInputTakenMsg{text: "move this draft"}) {
		t.Fatalf("take input returned %#v, want the prompt Claude put back", taken)
	}
	if want := []string{"keys 3 esc", "keys 3 ctrl+u ctrl+u"}; !reflect.DeepEqual(recorder.calls, want) {
		t.Errorf("sent %q, want %q", recorder.calls, want)
	}
	restored, _ := stopped.(zenModel).Update(taken)
	if value := restored.(zenModel).input.Value(); value != "move this draft\ntyped since" {
		t.Errorf("input = %q, want the cancelled prompt ahead of the text typed since", value)
	}
	if screenText := ansi.Strip(restored.(zenModel).screen()); strings.Contains(screenText, "▌ move this draft") {
		t.Errorf("the cancelled prompt's band still shows:\n%s", screenText)
	}
	if _, again := restored.(zenModel).Update(agentStatusMsg{status: "idle"}); again != nil {
		t.Error("a later status change took Claude's input again")
	}
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
	model := zenModel{entries: []claudecode.DialogueEntry{}, entryRows: [][]string{}, input: NewPromptInput()}
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
	want := []claudecode.QueueOperation{{Operation: "enqueue", Text: "second", EntriesBefore: 1}}
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
		messages <- fatalMsg{err: followTranscript(transcript, func(msg tea.Msg) { messages <- msg })}
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
	t.Run("clockTicksWhileWorkingAndWorkingRowClearsWhenTurnEnds", clockTicksWhileWorkingAndWorkingRowClearsWhenTurnEnds)
	t.Run("doneRowShowsTurnTimeAfterClaudeStopsWorkingUntilItWorksAgain", doneRowShowsTurnTimeAfterClaudeStopsWorkingUntilItWorksAgain)
	t.Run("doneRowStaysHiddenWhenEscInterruptedTurnOrClaudeWasNeverSeenWorking", doneRowStaysHiddenWhenEscInterruptedTurnOrClaudeWasNeverSeenWorking)
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
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			turn := claudecode.CurrentTurn{}
			for _, line := range testCase.lines {
				lineEntries, err := claudecode.DialogueEntries([]byte(line))
				if err != nil {
					t.Fatalf("DialogueEntries(%s): %v", line, err)
				}
				turn, err = claudecode.TurnAfterLine(turn, []byte(line), lineEntries)
				if err != nil {
					t.Fatalf("TurnAfterLine(%s): %v", line, err)
				}
			}
			if row := ansi.Strip(statusLine("working", turn, now, 80)); row != testCase.wantRow {
				t.Errorf("working row = %q, want %q", row, testCase.wantRow)
			}
		})
	}
}

func workingRowCutsToolCallAtViewWidthAndKeepsTime(t *testing.T) {
	promptTime := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	turn := claudecode.CurrentTurn{PromptTime: promptTime, ToolCallActivity: "Running a very long command description"}
	if row := ansi.Strip(statusLine("working", turn, promptTime.Add(42*time.Second), 20)); row != "Running a ver · 42s" {
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
	if row := ansi.Strip(statusLine("blocked", turn, now, 80)); row != "Waiting for you in Claude. Press alt+x to answer." {
		t.Errorf("blocked row = %q", row)
	}
	if row := statusLine("idle", turn, now, 80); row != "" {
		t.Errorf("idle row = %q, want it empty", row)
	}
}

func clockTicksWhileWorkingAndWorkingRowClearsWhenTurnEnds(t *testing.T) {
	model := zenModel{entries: []claudecode.DialogueEntry{}, entryRows: [][]string{}, input: NewPromptInput()}
	model = model.withSize(60, 12)
	promptTime := time.Now().Add(-42 * time.Second)
	read, _ := model.Update(entriesReadMsg{turn: claudecode.CurrentTurn{PromptTime: promptTime, ToolCallActivity: "Reading go.mod"}})
	started, firstTick := read.(zenModel).Update(agentStatusMsg{status: "working"})
	if firstTick == nil {
		t.Fatal("working status returned no clock tick")
	}
	tick, isTick := firstTick().(clockTickMsg)
	if !isTick {
		t.Fatalf("working status's command returned %#v, want a clock tick", firstTick())
	}
	if _, secondStart := started.(zenModel).Update(agentStatusMsg{status: "working"}); secondStart != nil {
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
	idle, _ := later.(zenModel).Update(agentStatusMsg{status: "idle"})
	if screen := ansi.Strip(idle.(zenModel).screen()); strings.Contains(screen, "Reading go.mod") {
		t.Errorf("screen after the turn ended still shows the working row:\n%s", screen)
	}
	stopped, afterIdle := idle.(zenModel).Update(clockTickMsg{now: tick.now.Add(4 * time.Second)})
	if afterIdle != nil || stopped.(zenModel).clockTicking {
		t.Errorf("a tick after the turn ended scheduled another")
	}
}

func doneRowShowsTurnTimeAfterClaudeStopsWorkingUntilItWorksAgain(t *testing.T) {
	model := zenModel{entries: []claudecode.DialogueEntry{}, entryRows: [][]string{}, input: NewPromptInput()}
	model = model.withSize(60, 12)
	promptTime := time.Now().Add(-time.Hour)
	read, _ := model.Update(entriesReadMsg{turn: claudecode.CurrentTurn{PromptTime: promptTime}})
	working, _ := read.(zenModel).Update(agentStatusMsg{status: "working"})
	stopped, endTurn := working.(zenModel).Update(agentStatusMsg{status: "done"})
	if endTurn == nil {
		t.Fatal("Claude stopping work returned no command to end the turn")
	}
	if _, isTurnEnd := endTurn().(turnEndedMsg); !isTurnEnd {
		t.Fatalf("Claude stopping work's command returned %#v, want a turn end", endTurn())
	}
	restarted, _ := stopped.(zenModel).Update(agentStatusMsg{status: "working"})
	late, _ := restarted.(zenModel).Update(turnEndedMsg{now: promptTime.Add(time.Minute)})
	if screen := ansi.Strip(late.(zenModel).screen()); strings.Contains(screen, "Done") {
		t.Errorf("a turn end arriving after Claude worked again showed the done row:\n%s", screen)
	}
	ended, _ := stopped.(zenModel).Update(turnEndedMsg{now: promptTime.Add(time.Minute + 11*time.Second)})
	if screen := ansi.Strip(ended.(zenModel).screen()); !strings.Contains(screen, " Done · 1m 11s") {
		t.Errorf("screen after the turn ended lacks the done row:\n%s", screen)
	}
	again, _ := ended.(zenModel).Update(agentStatusMsg{status: "working"})
	if screen := ansi.Strip(again.(zenModel).screen()); strings.Contains(screen, "Done") {
		t.Errorf("screen with Claude working again still shows the done row:\n%s", screen)
	}
}

func doneRowStaysHiddenWhenEscInterruptedTurnOrClaudeWasNeverSeenWorking(t *testing.T) {
	recorder := &claudeInputRecorder{screens: []string{herdrtest.CapturedScreen(t, "claude-input-empty.ansi")}}
	model := zenModel{entries: []claudecode.DialogueEntry{}, entryRows: [][]string{}, input: NewPromptInput()}
	model = model.withSize(60, 12)
	model.claude, model.claudePaneID, model.agentStatus = recorder, "3", "working"
	pressed, _ := model.withKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if _, takeInput := pressed.(zenModel).Update(agentStatusMsg{status: "done"}); takeInput == nil {
		t.Fatal("Claude stopping after Esc returned no command")
	} else if _, isTurnEnd := takeInput().(turnEndedMsg); isTurnEnd {
		t.Error("Claude stopping after Esc ended the turn with a done row")
	}
	model.agentStatus = "idle"
	if _, cmd := model.Update(agentStatusMsg{status: "done"}); cmd != nil {
		t.Errorf("a status change Claude never worked before returned %#v, want none", cmd())
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
	t.Run("zenViewSavesDraftWithPastesExpanded", zenViewSavesDraftWithPastesExpanded)
	t.Run("screenScrolledUpStaysOnItsRowsWhenPastedPromptReplacesBand", screenScrolledUpStaysOnItsRowsWhenPastedPromptReplacesBand)
}

func screenScrolledUpStaysOnItsRowsWhenPastedPromptReplacesBand(t *testing.T) {
	model := zenModel{entries: []claudecode.DialogueEntry{}, entryRows: [][]string{}, input: NewPromptInput()}
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
		[]string{"", " ▌ [Pasted text #1 +3 lines]"}) {
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
	recorder := &claudeInputRecorder{}
	model := modelWithPaste("look at ", fourLinePaste)
	model.claude, model.claudePaneID = recorder, "3"
	if value := model.input.Value(); value != "look at [Pasted text #1 +3 lines]" {
		t.Fatalf("input after the paste = %q, want the paste's label", value)
	}
	sent, send := model.withKey(pressedKey(tea.KeyEnter))
	send()
	if want := []string{"prompt 3 look at line one\nline two\nline three\nline four"}; !reflect.DeepEqual(recorder.calls, want) {
		t.Errorf("sent %q, want %q", recorder.calls, want)
	}
	if row := bandTextRow(sent.(zenModel), "look at [Pasted text #1 +3 lines]"); row == "" {
		t.Errorf("screen after Enter lacks the band with the paste's label:\n%s", ansi.Strip(sent.(zenModel).screen()))
	}
}

func ctrlEnterSendsWholeTextAndRefusalCarriesPastes(t *testing.T) {
	recorder := &claudeInputRecorder{promptErr: herdr.ErrAgentBlocked}
	model := modelWithPaste("look at ", fourLinePaste)
	model.claude, model.claudePaneID = recorder, "3"
	_, send := model.withKey(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModCtrl})
	paste := "line one\nline two\nline three\nline four"
	want := promptSentMsg{prompt: "look at " + paste, pastes: []string{paste}, err: herdr.ErrAgentBlocked}
	if msg := send(); !reflect.DeepEqual(msg, want) {
		t.Errorf("ctrl+enter's command returned %#v, want %#v", msg, want)
	}
	if wantCalls := []string{"prompt 3 look at " + paste}; !reflect.DeepEqual(recorder.calls, wantCalls) {
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
		pastes: []string{"line one\nline two\nline three\nline four"}, err: herdr.ErrAgentBlocked})
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
	screenText := ansi.Strip(recorded.screen())
	if !strings.Contains(screenText, "▌ look at [Pasted text #1 +3 lines]") || strings.Contains(screenText, "line two") {
		t.Errorf("screen after the transcript records the prompt lacks its paste's label or shows the paste:\n%s",
			screenText)
	}
}

func zenViewSavesDraftWithPastesExpanded(t *testing.T) {
	draftFile := filepath.Join(t.TempDir(), "draft")
	var view tea.Model = draftSavingView{zen: zenModel{entries: []claudecode.DialogueEntry{}, entryRows: [][]string{},
		input: NewPromptInput()}, draftFile: draftFile, savedDraft: ""}
	view, _ = view.Update(tea.PasteMsg{Content: fourLinePaste})
	draft, err := os.ReadFile(draftFile)
	if err != nil {
		t.Fatal(err)
	}
	if want := "line one\nline two\nline three\nline four"; string(draft) != want {
		t.Errorf("draft = %q, want %q", draft, want)
	}
}

func TestDraftSavingView(t *testing.T) {
	t.Run("zenViewSavesUnsentTextAsDraftOfItsClaudePane", zenViewSavesUnsentTextAsDraftOfItsClaudePane)
}

func zenViewSavesUnsentTextAsDraftOfItsClaudePane(t *testing.T) {
	draftFile := filepath.Join(t.TempDir(), "draft")
	var view tea.Model = draftSavingView{zen: zenModel{entries: []claudecode.DialogueEntry{}, entryRows: [][]string{},
		input: NewPromptInput()}, draftFile: draftFile, savedDraft: ""}
	for _, keyPress := range []tea.KeyPressMsg{
		{Code: 'h', Text: "h"}, {Code: 'i', Text: "i"}, {Code: tea.KeyEnter, Mod: tea.ModShift}, {Code: 'x', Text: "x"},
	} {
		view, _ = view.Update(keyPress)
	}
	draft, err := os.ReadFile(draftFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(draft) != "hi\nx" {
		t.Errorf("draft = %q, want %q", draft, "hi\nx")
	}
}
