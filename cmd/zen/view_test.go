package main

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
)

func TestLineNumberAtCountsTheLinesBeforeTheOffset(t *testing.T) {
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

func TestFollowTranscriptNamesTheUndecodableLineAndKeepsReading(t *testing.T) {
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
				want := []dialogueEntry{{kind: promptEntry, text: "first"}, {kind: promptEntry, text: "third"}}
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

func TestEnterClearsTheInputAtOnceAndAFailedSendPutsTheWholeTextBack(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		typedSince string
		wantInput  string
		wantShown  string
	}{
		{name: "refused while blocked", err: errAgentBlocked, wantInput: "keep me\nand this line",
			wantShown: "Claude is waiting for you. Press alt+x to answer."},
		{name: "herdr failed", err: errors.New("herdr agent prompt failed: exit status 1"),
			wantInput: "keep me\nand this line", wantShown: "herdr agent prompt failed: exit status 1"},
		{name: "refused after more typing", err: errAgentBlocked, typedSince: "more",
			wantInput: "keep me\nand this line\nmore", wantShown: "Claude is waiting for you. Press alt+x to answer."},
		{name: "sent", err: nil, wantInput: "", wantShown: ""},
		{name: "sent after more typing", err: nil, typedSince: "more", wantInput: "more", wantShown: ""},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			model := zenModel{entries: []dialogueEntry{}, entryRows: [][]string{}, input: NewPromptInput()}
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

func numberedPrompts(first int, count int) []dialogueEntry {
	entries := []dialogueEntry{}
	for number := first; number < first+count; number++ {
		entries = append(entries, dialogueEntry{kind: promptEntry, text: fmt.Sprintf("prompt %d", number)})
	}
	return entries
}

func pressedKey(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

func TestScreenScrolledUpStaysOnItsRowsWhenEntriesArriveAndEndReturnsToTheNewest(t *testing.T) {
	model := zenModel{entries: []dialogueEntry{}, entryRows: [][]string{}, input: NewPromptInput()}
	model = model.withSize(40, 12).withEntries(numberedPrompts(1, 20))
	if !strings.Contains(model.screen(), "prompt 20") {
		t.Fatalf("screen at the bottom lacks the newest prompt:\n%s", model.screen())
	}
	scrolled, _ := model.withKey(pressedKey(tea.KeyPgUp))
	scrolledModel := scrolled.(zenModel)
	before := scrolledModel.screen()
	if strings.Contains(before, "prompt 20") {
		t.Fatalf("screen after PgUp still shows the newest prompt:\n%s", before)
	}
	scrolledModel = scrolledModel.withEntries(numberedPrompts(21, 3))
	if after := scrolledModel.screen(); after != before {
		t.Errorf("screen moved when entries arrived while scrolled up:\nbefore\n%s\nafter\n%s", before, after)
	}
	ended, _ := scrolledModel.withKey(pressedKey(tea.KeyEnd))
	if !strings.Contains(ended.(zenModel).screen(), "prompt 23") {
		t.Errorf("screen after End lacks the newest prompt:\n%s", ended.(zenModel).screen())
	}
}

func TestScreenScrolledPastTheOldestEntryStopsThereAndOnePgDnReturnsToTheNewest(t *testing.T) {
	model := zenModel{entries: []dialogueEntry{}, entryRows: [][]string{}, input: NewPromptInput()}
	model = model.withSize(40, 12).withEntries(numberedPrompts(1, 3))
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

func TestUpScrollsBackThreeRowsAndDownReturns(t *testing.T) {
	model := zenModel{entries: []dialogueEntry{}, entryRows: [][]string{}, input: NewPromptInput()}
	model = model.withSize(40, 12).withEntries(numberedPrompts(1, 20))
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

func TestCtrlYCopiesTheNewestCodeBlockAndShowsCopiedUntilTheNextKey(t *testing.T) {
	model := zenModel{entries: []dialogueEntry{}, entryRows: [][]string{}, input: NewPromptInput()}
	model = model.withSize(60, 30).withEntries([]dialogueEntry{
		{kind: replyEntry, text: "Old:\n\n```\nold block\n```"},
		{kind: replyEntry, text: "New:\n\n```go\nfirst()\n```\n\n```go\nfunc f() {\n\treturn \n}\n```\n\nDone."},
		{kind: promptEntry, text: "thanks\n\n```\nmine\n```"},
	})
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

func TestCtrlYWithNoCodeBlockSaysSoAndCopiesNothing(t *testing.T) {
	model := zenModel{entries: []dialogueEntry{}, entryRows: [][]string{}, input: NewPromptInput()}
	model = model.withSize(60, 12).withEntries([]dialogueEntry{
		{kind: promptEntry, text: "hello"}, {kind: replyEntry, text: "Use `inline code` only."},
	})
	pressed, copyCmd := model.Update(tea.KeyPressMsg{Code: 'y', Mod: tea.ModCtrl})
	if copyCmd != nil {
		t.Errorf("ctrl+y with no code block returned %#v, want no command", copyCmd())
	}
	if !strings.Contains(ansi.Strip(pressed.(zenModel).screen()), "No code block to copy.") {
		t.Errorf("screen lacks the no code block status:\n%s", ansi.Strip(pressed.(zenModel).screen()))
	}
}

func TestANewStatusReplacesTheCopyStatus(t *testing.T) {
	cases := []struct {
		name      string
		statusMsg tea.Msg
		wantShown string
	}{
		{name: "agent status", statusMsg: agentStatusMsg{status: "working"}, wantShown: "Working..."},
		{name: "problem", statusMsg: problemMsg{err: errors.New("herdr pane get failed")},
			wantShown: "herdr pane get failed"},
		{name: "prompt refused", statusMsg: promptSentMsg{err: errAgentBlocked},
			wantShown: "Claude is waiting for you. Press alt+x to answer."},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			model := zenModel{entries: []dialogueEntry{}, entryRows: [][]string{}, input: NewPromptInput()}
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

func TestTheCopyHintMovesToTheNewestCodeBlockWhenAReplyBringsOne(t *testing.T) {
	model := zenModel{entries: []dialogueEntry{}, entryRows: [][]string{}, input: NewPromptInput()}
	model = model.withSize(60, 40).withEntries([]dialogueEntry{{kind: replyEntry, text: "```sh\nfirst\n```"}})
	model = model.withEntries([]dialogueEntry{
		{kind: promptEntry, text: "and then?"}, {kind: replyEntry, text: "```go\nsecond\n```"},
	})
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
	model := zenModel{entries: []dialogueEntry{}, entryRows: [][]string{}, input: NewPromptInput()}
	model = model.withSize(60, 20).withEntries([]dialogueEntry{{kind: replyEntry, text: "Ready."}})
	model.input.SetValue(prompt)
	return model
}

func TestEnterShowsThePromptBandAtOnceAndTheTranscriptPromptTakesItsPlace(t *testing.T) {
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
	withReply := sentModel.withEntries([]dialogueEntry{{kind: replyEntry, text: "Still on the last step."}})
	replyText := ansi.Strip(withReply.screen())
	replyAt := strings.Index(replyText, "Still on the last step.")
	bandAt := strings.Index(replyText, "▌ rename the widget")
	if replyAt < 0 || bandAt < 0 || replyAt > bandAt {
		t.Errorf("screen lacks the reply or the early band, or has the reply under the band:\n%s", replyText)
	}
	recorded := withReply.withEntries([]dialogueEntry{{kind: promptEntry, text: "rename the widget"}})
	recordedText := ansi.Strip(recorded.screen())
	if count := strings.Count(recordedText, "rename the widget"); count != 1 {
		t.Errorf("screen shows the prompt %d times after the transcript records it, want 1:\n%s", count, recordedText)
	}
	if len(recorded.earlyPrompts) != 0 {
		t.Errorf("earlyPrompts = %q, want none once the transcript holds the prompt", recorded.earlyPrompts)
	}
}

func TestASlashCommandGetsNoEarlyBand(t *testing.T) {
	sent, sendPrompt := modelWithTypedPrompt("/compact").withKey(pressedKey(tea.KeyEnter))
	if sendPrompt == nil {
		t.Fatal("Enter returned no command to send the slash command")
	}
	if screenText := ansi.Strip(sent.(zenModel).screen()); strings.Contains(screenText, "▌ /compact") {
		t.Errorf("screen after Enter on a slash command shows a band:\n%s", screenText)
	}
}

func TestAFailedSendRemovesTheEarlyBandAndShowsTheRefusal(t *testing.T) {
	sent, _ := modelWithTypedPrompt("rename the widget").withKey(pressedKey(tea.KeyEnter))
	refused, _ := sent.(zenModel).Update(promptSentMsg{prompt: "rename the widget", err: errAgentBlocked})
	refusedModel := refused.(zenModel)
	screenText := ansi.Strip(refusedModel.screen())
	if strings.Contains(screenText, "▌ rename the widget") {
		t.Errorf("screen after a refused send still shows the early band:\n%s", screenText)
	}
	if !strings.Contains(screenText, "Claude is waiting for you. Press alt+x to answer.") {
		t.Errorf("screen after a refused send lacks the refusal:\n%s", screenText)
	}
}

func TestScreenScrolledUpStaysOnItsRowsWhenAnEarlyBandComesAndGoes(t *testing.T) {
	model := zenModel{entries: []dialogueEntry{}, entryRows: [][]string{}, input: NewPromptInput()}
	model = model.withSize(40, 12).withEntries(numberedPrompts(1, 20))
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
	recorded := sentModel.withEntries(numberedPrompts(21, 1))
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

func TestTheInputWrapsALongLineUnderItsPrompt(t *testing.T) {
	words := strings.Repeat("wrap these words ", 12)
	for _, width := range []int{40, 90} {
		model := zenModel{entries: []dialogueEntry{}, entryRows: [][]string{}, input: NewPromptInput()}
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

func TestTheInputGrowsARowAtATimeUpToEightAndTheScreenKeepsItsHeight(t *testing.T) {
	model := zenModel{entries: []dialogueEntry{}, entryRows: [][]string{}, input: NewPromptInput()}
	model = model.withSize(60, 24).withEntries(numberedPrompts(1, 20))
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

func TestShiftEnterStartsANewLineAndEnterSendsTheTextWithItsNewlines(t *testing.T) {
	model := zenModel{herdr: herdrCLI{binary: filepath.Join(t.TempDir(), "no-herdr.exe")}, entries: []dialogueEntry{},
		entryRows: [][]string{}, input: NewPromptInput()}
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

func TestUpAndDownMoveTheCursorInAnInputOfSeveralRows(t *testing.T) {
	model := zenModel{entries: []dialogueEntry{}, entryRows: [][]string{}, input: NewPromptInput()}
	model = model.withSize(40, 12).withEntries(numberedPrompts(1, 20))
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
	model := zenModel{entries: []dialogueEntry{}, entryRows: [][]string{}, input: NewPromptInput()}
	model = model.withSize(60, 24)
	model.input.SetValue(inputValue)
	return model.withEntries(numberedPrompts(1, 20))
}

func TestTheEntriesFillTheRowsTheInputGivesBack(t *testing.T) {
	model := zenModel{herdr: herdrCLI{binary: filepath.Join(t.TempDir(), "no-herdr.exe")}, entries: []dialogueEntry{},
		entryRows: [][]string{}, input: NewPromptInput()}
	model = model.withSize(60, 24)
	// Entries that arrive under a tall input are drawn only down to its top row. A slash command gets no early band
	// to draw the entries on Enter.
	model.input.SetValue("/x" + strings.Repeat("\nx", inputMaxRows-1))
	model = model.withEntries(numberedPrompts(1, 20))
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
