package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// fakeHerdrDirEnv makes the test binary act as herdr's CLI on the panes in panes.json under the directory it names,
// logging each command line to calls.log there.
const fakeHerdrDirEnv = "ZEN_FAKE_HERDR_DIR"

func TestMain(m *testing.M) {
	if fakeDir := os.Getenv(fakeHerdrDirEnv); fakeDir != "" {
		if err := RunFakeHerdr(fakeDir, os.Args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func RunFakeHerdr(fakeDir string, arguments []string) error {
	callLog, err := os.OpenFile(filepath.Join(fakeDir, "calls.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(callLog, strings.Join(arguments, " ")); err != nil {
		return errors.Join(err, callLog.Close())
	}
	if err := callLog.Close(); err != nil {
		return err
	}
	panesFile := filepath.Join(fakeDir, "panes.json")
	panesJSON, err := os.ReadFile(panesFile)
	if err != nil {
		return err
	}
	var panes []herdrPane
	if err := json.Unmarshal(panesJSON, &panes); err != nil {
		return err
	}
	command := strings.Join(arguments[:2], " ")
	switch command {
	case "pane list":
		reply, err := json.Marshal(map[string]map[string][]herdrPane{"result": {"panes": panes}})
		if err != nil {
			return err
		}
		_, err = os.Stdout.Write(reply)
		return err
	case "plugin pane":
		if arguments[2] != "open" {
			return nil
		}
		zenPane := herdrPane{PaneID: fmt.Sprintf("w1:z%d", len(panes)), Label: zenPaneTitle, Focused: true}
		for index := range panes {
			panes[index].Focused = false
		}
		panes = append(panes, zenPane)
		if _, err := fmt.Printf(`{"result":{"plugin_pane":{"pane":{"pane_id":%q}}}}`, zenPane.PaneID); err != nil {
			return err
		}
	case "pane report-metadata":
		token := strings.SplitN(arguments[len(arguments)-1], "=", 2)
		for index := range panes {
			if panes[index].PaneID == arguments[2] {
				panes[index].Tokens = map[string]string{token[0]: token[1]}
			}
		}
	default:
		return nil
	}
	panesJSON, err = json.Marshal(panes)
	if err != nil {
		return err
	}
	return os.WriteFile(panesFile, panesJSON, 0o644)
}

func fakeHerdr(t *testing.T, panes []herdrPane) (herdrCLI, zenMode, func() []string) {
	fakeDir := t.TempDir()
	panesJSON, err := json.Marshal(panes)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fakeDir, "panes.json"), panesJSON, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(fakeHerdrDirEnv, fakeDir)
	testBinary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	calls := func() []string {
		callLog, err := os.ReadFile(filepath.Join(fakeDir, "calls.log"))
		if os.IsNotExist(err) {
			return []string{}
		}
		if err != nil {
			t.Fatal(err)
		}
		return strings.Split(strings.TrimSpace(string(callLog)), "\n")
	}
	mode := zenMode{stateDir: t.TempDir(), session: "zentest", mutexName: `Local\zen-test.` + t.Name()}
	return herdrCLI{binary: testBinary, pluginID: "zen-test"}, mode, calls
}

func claudePane(paneID string, focused bool) herdrPane {
	return herdrPane{PaneID: paneID, Agent: "claude", Focused: focused}
}

func zenPane(paneID string, claudePaneID string, focused bool) herdrPane {
	return herdrPane{PaneID: paneID, Label: zenPaneTitle, Focused: focused,
		Tokens: map[string]string{claudePaneToken: claudePaneID}}
}

func TestZenPaneActionOpensAZenViewOnlyForAClaudePaneWithoutOneAndShowsAnExistingOne(t *testing.T) {
	cases := []struct {
		name  string
		panes []herdrPane
		want  zenPaneAction
	}{
		{
			name:  "Claude pane without a zen pane",
			panes: []herdrPane{claudePane("w1:p1", true), claudePane("w1:p2", false), zenPane("w1:p3", "w1:p2", false)},
			want:  zenPaneAction{Kind: zenPaneActionOpen, ClaudePaneID: "w1:p1"},
		},
		{
			name:  "Claude pane with a zen pane",
			panes: []herdrPane{claudePane("w1:p1", true), zenPane("w1:p3", "w1:p1", false)},
			want:  zenPaneAction{Kind: zenPaneActionShow, ClaudePaneID: "w1:p1", ZenPaneID: "w1:p3"},
		},
		{
			name:  "shell pane",
			panes: []herdrPane{{PaneID: "w1:p1", Focused: true}, claudePane("w1:p2", false)},
			want:  zenPaneAction{Kind: zenPaneActionNone},
		},
		{
			name:  "zen pane",
			panes: []herdrPane{claudePane("w1:p1", false), zenPane("w1:p3", "w1:p1", true)},
			want:  zenPaneAction{Kind: zenPaneActionNone},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := ZenPaneActionForFocusedPane(testCase.panes)
			if err != nil {
				t.Fatal(err)
			}
			if got != testCase.want {
				t.Errorf("ZenPaneActionForFocusedPane = %+v, want %+v", got, testCase.want)
			}
		})
	}
}

func TestTheFocusHookDoesNothingWhileZenModeIsOff(t *testing.T) {
	herdr, mode, calls := fakeHerdr(t, []herdrPane{claudePane("w1:p1", true)})
	if err := ShowFocusedZenView(herdr, mode); err != nil {
		t.Fatal(err)
	}
	if got := calls(); len(got) != 0 {
		t.Errorf("herdr calls = %q, want none", got)
	}
}

func TestTheFocusHookShowsTheExistingZenPaneOfTheFocusedClaudePane(t *testing.T) {
	herdr, mode, calls := fakeHerdr(t, []herdrPane{claudePane("w1:p1", true), zenPane("w1:p3", "w1:p1", false)})
	if err := os.WriteFile(mode.ModeFile(), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ShowFocusedZenView(herdr, mode); err != nil {
		t.Fatal(err)
	}
	want := []string{"pane list", "plugin pane focus w1:p3", "pane zoom w1:p3 --on"}
	if got := calls(); !reflect.DeepEqual(got, want) {
		t.Errorf("herdr calls = %q, want %q", got, want)
	}
}

func TestFocusHooksRunTogetherOpenOneZenPaneForTheClaudePane(t *testing.T) {
	herdr, mode, calls := fakeHerdr(t, []herdrPane{claudePane("w1:p1", true)})
	if err := os.WriteFile(mode.ModeFile(), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	var hooks sync.WaitGroup
	errs := make(chan error, 4)
	for range 4 {
		hooks.Go(func() { errs <- ShowFocusedZenView(herdr, mode) })
	}
	hooks.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	opens := 0
	for _, call := range calls() {
		if strings.HasPrefix(call, "plugin pane open") {
			opens++
		}
	}
	if opens != 1 {
		t.Errorf("zen panes opened = %d, want 1: %q", opens, calls())
	}
}

func TestAltXTurnsZenModeOnAndOpensAndMarksTheFocusedClaudePanesZenViewWithoutAStaleDraft(t *testing.T) {
	herdr, mode, calls := fakeHerdr(t, []herdrPane{claudePane("w1:p1", true)})
	if err := os.WriteFile(mode.DraftFile("w1:p1"), []byte("left by a closed zen pane"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ToggleZenMode(herdr, mode); err != nil {
		t.Fatal(err)
	}
	if isOn, err := mode.IsOn(); err != nil || !isOn {
		t.Errorf("IsOn = %v, %v, want true", isOn, err)
	}
	want := []string{
		"pane list",
		"plugin pane open --plugin zen-test --entrypoint zen --placement zoomed --target-pane w1:p1 --env " +
			"ZEN_CLAUDE_PANE=w1:p1 --focus",
		"pane report-metadata w1:z1 --source zen-test --token claude_pane=w1:p1",
	}
	if got := calls(); !reflect.DeepEqual(got, want) {
		t.Errorf("herdr calls = %q, want %q", got, want)
	}
	if _, err := os.Stat(mode.DraftFile("w1:p1")); !os.IsNotExist(err) {
		t.Errorf("stale draft file: %v, want it removed", err)
	}
}

func TestAltXTurnsZenModeOffClosesEveryZenPaneMovesItsDraftAndFocusesTheClaudePane(t *testing.T) {
	herdr, mode, calls := fakeHerdr(t, []herdrPane{
		claudePane("w1:p1", false),
		zenPane("w1:p2", "w1:p1", true),
		claudePane("w1:p3", false),
		zenPane("w1:p4", "w1:p3", false),
		{PaneID: "w1:p5", Label: zenPaneTitle},
	})
	if err := os.WriteFile(mode.ModeFile(), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	draft := "first line\nsecond line"
	if err := os.WriteFile(mode.DraftFile("w1:p1"), []byte(draft), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mode.DraftFile("w1:p3"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ToggleZenMode(herdr, mode); err != nil {
		t.Fatal(err)
	}
	if isOn, err := mode.IsOn(); err != nil || isOn {
		t.Errorf("IsOn = %v, %v, want false", isOn, err)
	}
	want := []string{
		"pane list",
		"pane close w1:p2",
		"pane send-text w1:p1 first line",
		"second line",
		"pane close w1:p4",
		"pane close w1:p5",
		"agent focus w1:p1",
	}
	if got := calls(); !reflect.DeepEqual(got, want) {
		t.Errorf("herdr calls = %q, want %q", got, want)
	}
	for _, claudePaneID := range []string{"w1:p1", "w1:p3"} {
		if _, err := os.Stat(mode.DraftFile(claudePaneID)); !os.IsNotExist(err) {
			t.Errorf("draft file of %s after the move: %v, want it removed", claudePaneID, err)
		}
	}
}

func TestAltXTurningZenModeOffLeavesTheFocusOnAPaneThatIsNotAZenPane(t *testing.T) {
	herdr, mode, calls := fakeHerdr(t, []herdrPane{claudePane("w1:p1", true), zenPane("w1:p2", "w1:p3", false),
		claudePane("w1:p3", false)})
	if err := os.WriteFile(mode.ModeFile(), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ToggleZenMode(herdr, mode); err != nil {
		t.Fatal(err)
	}
	want := []string{"pane list", "pane close w1:p2"}
	if got := calls(); !reflect.DeepEqual(got, want) {
		t.Errorf("herdr calls = %q, want %q", got, want)
	}
}

func TestTheZenViewSavesItsUnsentTextAsTheDraftOfItsClaudePane(t *testing.T) {
	draftFile := filepath.Join(t.TempDir(), "draft")
	var view tea.Model = draftSavingView{zen: zenModel{entries: []dialogueEntry{}, entryRows: [][]string{},
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
