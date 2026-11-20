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

	"charm.land/bubbles/v2/textinput"
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

func TestPromptSentMsgKeepsTheTypedTextOnAFailureAndClearsItOnSuccess(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		wantInput string
		wantShown string
	}{
		{name: "refused while blocked", err: errAgentBlocked, wantInput: "keep me",
			wantShown: "Claude is waiting for you. Press alt+x to answer."},
		{name: "herdr failed", err: errors.New("herdr agent prompt failed: exit status 1"), wantInput: "keep me",
			wantShown: "herdr agent prompt failed: exit status 1"},
		{name: "sent", err: nil, wantInput: "", wantShown: ""},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			model := zenModel{entries: []dialogueEntry{}, entryRows: [][]string{}, input: textinput.New()}
			model.input.SetValue("keep me")
			updated, _ := model.Update(promptSentMsg{err: testCase.err})
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
	model := zenModel{entries: []dialogueEntry{}, entryRows: [][]string{}, input: textinput.New()}
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
	model := zenModel{entries: []dialogueEntry{}, entryRows: [][]string{}, input: textinput.New()}
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

func TestMouseWheelUpScrollsBackThreeRowsAndWheelDownReturns(t *testing.T) {
	model := zenModel{entries: []dialogueEntry{}, entryRows: [][]string{}, input: textinput.New()}
	model = model.withSize(40, 12).withEntries(numberedPrompts(1, 20))
	wheeledUp, _ := model.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	upModel := wheeledUp.(zenModel)
	if upModel.rowsBelowView != wheelScrollRows {
		t.Errorf("rows below the view after wheel up = %d, want %d", upModel.rowsBelowView, wheelScrollRows)
	}
	if strings.Contains(upModel.screen(), "prompt 20") {
		t.Errorf("screen after wheel up still shows the newest prompt:\n%s", upModel.screen())
	}
	wheeledDown, _ := upModel.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if rowsBelowView := wheeledDown.(zenModel).rowsBelowView; rowsBelowView != 0 {
		t.Errorf("rows below the view after wheel down = %d, want 0", rowsBelowView)
	}
}
