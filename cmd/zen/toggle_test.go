package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// fakeHerdrDirEnv makes the test binary act as herdr's CLI on the panes in panes.json under the directory it names,
// logging each command line to calls.log there. A pane's screen is the file FakeScreenFile names, and send-keys
// replaces it with the file named with .after-keys added when there is one.
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
	case "pane read":
		screen, err := os.ReadFile(FakeScreenFile(fakeDir, arguments[2]))
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		_, err = os.Stdout.Write(screen)
		return err
	case "pane send-keys":
		screenFile := FakeScreenFile(fakeDir, arguments[2])
		if err := os.Rename(screenFile+".after-keys", screenFile); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	case "pane get":
		for _, pane := range panes {
			if pane.PaneID == arguments[2] {
				reply, err := json.Marshal(map[string]map[string]herdrPane{"result": {"pane": pane}})
				if err != nil {
					return err
				}
				_, err = os.Stdout.Write(reply)
				return err
			}
		}
		return fmt.Errorf("pane %s not found", arguments[2])
	case "pane list":
		reply, err := json.Marshal(map[string]map[string][]herdrPane{"result": {"panes": panes}})
		if err != nil {
			return err
		}
		_, err = os.Stdout.Write(reply)
		return err
	case "agent list":
		agents := []herdrPane{}
		for _, pane := range panes {
			if pane.Agent != "" {
				agents = append(agents, pane)
			}
		}
		reply, err := json.Marshal(map[string]map[string][]herdrPane{"result": {"agents": agents}})
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

func FakeScreenFile(fakeDir string, paneID string) string {
	return filepath.Join(fakeDir, "screen."+strings.ReplaceAll(paneID, ":", "-"))
}

// ShowOnFakeScreen puts the testdata capture on paneID's screen, and afterKeysCapture there once keys are sent. An
// empty afterKeysCapture leaves the screen as it is.
func ShowOnFakeScreen(t *testing.T, paneID string, capture string, afterKeysCapture string) {
	screenFile := FakeScreenFile(os.Getenv(fakeHerdrDirEnv), paneID)
	for file, capture := range map[string]string{screenFile: capture, screenFile + ".after-keys": afterKeysCapture} {
		if capture == "" {
			continue
		}
		screen, err := os.ReadFile(filepath.Join("testdata", capture))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, screen, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func claudePane(paneID string, focused bool) herdrPane {
	return herdrPane{PaneID: paneID, Agent: "claude", Focused: focused}
}

func zenPane(paneID string, claudePaneID string, focused bool) herdrPane {
	return herdrPane{PaneID: paneID, Label: zenPaneTitle, Focused: focused,
		Tokens: map[string]string{claudePaneToken: claudePaneID}}
}

func TestZenPaneAction(t *testing.T) {
	t.Run("zenPaneActionOpensViewOnlyForClaudePaneWithoutOneAndShowsExistingOne",
		zenPaneActionOpensViewOnlyForClaudePaneWithoutOneAndShowsExistingOne)
}

func zenPaneActionOpensViewOnlyForClaudePaneWithoutOneAndShowsExistingOne(t *testing.T) {
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

func TestFocusHook(t *testing.T) {
	t.Run("focusHookDoesNothingWhileZenModeIsOff", focusHookDoesNothingWhileZenModeIsOff)
	t.Run("focusHookShowsExistingZenPaneOfFocusedClaudePane", focusHookShowsExistingZenPaneOfFocusedClaudePane)
	t.Run("focusHooksRunTogetherOpenOneZenPaneForClaudePane", focusHooksRunTogetherOpenOneZenPaneForClaudePane)
}

func focusHookDoesNothingWhileZenModeIsOff(t *testing.T) {
	herdr, mode, calls := fakeHerdr(t, []herdrPane{claudePane("w1:p1", true)})
	if err := ShowFocusedZenView(herdr, mode); err != nil {
		t.Fatal(err)
	}
	if got := calls(); len(got) != 0 {
		t.Errorf("herdr calls = %q, want none", got)
	}
}

func focusHookShowsExistingZenPaneOfFocusedClaudePane(t *testing.T) {
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

func TestFocusAgent(t *testing.T) {
	t.Run("focusAgentFocusesNumberedAgentWhileZenModeIsOff", focusAgentFocusesNumberedAgentWhileZenModeIsOff)
	t.Run("focusAgentShowsZenPaneOfNumberedAgentWithoutFocusingAgent", focusAgentShowsZenPaneOfNumberedAgentWithoutFocusingAgent)
	t.Run("focusAgentFocusesNumberedAgentWithoutZenPaneForFocusHookToOpenOne", focusAgentFocusesNumberedAgentWithoutZenPaneForFocusHookToOpenOne)
	t.Run("focusAgentFailsForNumberPastLastAgent", focusAgentFailsForNumberPastLastAgent)
}

func focusAgentFocusesNumberedAgentWhileZenModeIsOff(t *testing.T) {
	herdr, mode, calls := fakeHerdr(t, []herdrPane{claudePane("w1:p1", true), claudePane("w2:p1", false)})
	if err := FocusAgentOrItsZenPane(herdr, mode, 2); err != nil {
		t.Fatal(err)
	}
	want := []string{"agent list", "agent focus w2:p1"}
	if got := calls(); !reflect.DeepEqual(got, want) {
		t.Errorf("herdr calls = %q, want %q", got, want)
	}
}

func focusAgentShowsZenPaneOfNumberedAgentWithoutFocusingAgent(t *testing.T) {
	agentZenPane := zenPane("w1:p4", "w1:p3", false)
	agentZenPane.TabID = "w1:t2"
	herdr, mode, calls := fakeHerdr(t, []herdrPane{claudePane("w1:p1", false), zenPane("w1:p2", "w1:p1", true),
		claudePane("w1:p3", false), agentZenPane})
	if err := os.WriteFile(mode.ModeFile(), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := FocusAgentOrItsZenPane(herdr, mode, 2); err != nil {
		t.Fatal(err)
	}
	want := []string{"agent list", "pane list", "tab focus w1:t2", "plugin pane focus w1:p4", "pane zoom w1:p4 --on"}
	if got := calls(); !reflect.DeepEqual(got, want) {
		t.Errorf("herdr calls = %q, want %q", got, want)
	}
}

func focusAgentFocusesNumberedAgentWithoutZenPaneForFocusHookToOpenOne(t *testing.T) {
	herdr, mode, calls := fakeHerdr(t, []herdrPane{claudePane("w1:p1", false), zenPane("w1:p2", "w1:p1", true),
		claudePane("w2:p1", false)})
	if err := os.WriteFile(mode.ModeFile(), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := FocusAgentOrItsZenPane(herdr, mode, 2); err != nil {
		t.Fatal(err)
	}
	want := []string{"agent list", "pane list", "agent focus w2:p1"}
	if got := calls(); !reflect.DeepEqual(got, want) {
		t.Errorf("herdr calls = %q, want %q", got, want)
	}
}

func focusAgentFailsForNumberPastLastAgent(t *testing.T) {
	herdr, mode, calls := fakeHerdr(t, []herdrPane{claudePane("w1:p1", true)})
	if err := FocusAgentOrItsZenPane(herdr, mode, 2); err == nil {
		t.Fatal("FocusAgentOrItsZenPane(2) with one agent returned no error")
	}
	want := []string{"agent list"}
	if got := calls(); !reflect.DeepEqual(got, want) {
		t.Errorf("herdr calls = %q, want %q", got, want)
	}
}

func focusHooksRunTogetherOpenOneZenPaneForClaudePane(t *testing.T) {
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

func TestToggle(t *testing.T) {
	t.Run("altXTurnsZenModeOnMovesClaudesDraftAfterZenDraftAndOpensAndMarksZenView",
		altXTurnsZenModeOnMovesClaudesDraftAfterZenDraftAndOpensAndMarksZenView)
	t.Run("altXMovesWrappedDraftOutOfClaudesInputAsOneLine", altXMovesWrappedDraftOutOfClaudesInputAsOneLine)
	t.Run("altXSendsNoKeyAndMovesNothingFromPlaceholderEmptyBoxOrBlockedClaude", altXSendsNoKeyAndMovesNothingFromPlaceholderEmptyBoxOrBlockedClaude)
	t.Run("altXFailsNamingPaneAndMovesNothingWhenClaudesInputStillHoldsTextAfterClearKeys",
		altXFailsNamingPaneAndMovesNothingWhenClaudesInputStillHoldsTextAfterClearKeys)
	t.Run("altXTurnsZenModeOffClosesEveryZenPaneMovesItsDraftAndFocusesClaudePane",
		altXTurnsZenModeOffClosesEveryZenPaneMovesItsDraftAndFocusesClaudePane)
	t.Run("altXTurningZenModeOffLeavesFocusOnPaneThatIsNotZenPane", altXTurningZenModeOffLeavesFocusOnPaneThatIsNotZenPane)
	t.Run("zenViewSavesUnsentTextAsDraftOfItsClaudePane", zenViewSavesUnsentTextAsDraftOfItsClaudePane)
}

func altXTurnsZenModeOnMovesClaudesDraftAfterZenDraftAndOpensAndMarksZenView(t *testing.T) {
	herdr, mode, calls := fakeHerdr(t, []herdrPane{claudePane("w1:p1", true)})
	ShowOnFakeScreen(t, "w1:p1", "claude-input-one-row.ansi", "claude-input-empty.ansi")
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
		"pane read w1:p1 --source visible --format ansi",
		"pane send-keys w1:p1 ctrl+u ctrl+u",
		"pane read w1:p1 --source visible --format ansi",
		"plugin pane open --plugin zen-test --entrypoint zen --placement zoomed --target-pane w1:p1 --env " +
			"ZEN_CLAUDE_PANE=w1:p1 --focus",
		"pane report-metadata w1:z1 --source zen-test --token claude_pane=w1:p1",
	}
	if got := calls(); !reflect.DeepEqual(got, want) {
		t.Errorf("herdr calls = %q, want %q", got, want)
	}
	assertDraft(t, mode.DraftFile("w1:p1"), "left by a closed zen pane\nmove this draft")
}

func assertDraft(t *testing.T, draftFile string, want string) {
	draft, err := os.ReadFile(draftFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(draft) != want {
		t.Errorf("draft = %q, want %q", draft, want)
	}
}

func altXMovesWrappedDraftOutOfClaudesInputAsOneLine(t *testing.T) {
	herdr, mode, calls := fakeHerdr(t, []herdrPane{claudePane("w1:p1", true)})
	ShowOnFakeScreen(t, "w1:p1", "claude-input-wrapped.ansi", "claude-input-empty.ansi")
	if err := ToggleZenMode(herdr, mode); err != nil {
		t.Fatal(err)
	}
	if got := calls(); !slices.Contains(got, "pane send-keys w1:p1 ctrl+u ctrl+u ctrl+u ctrl+u") {
		t.Errorf("herdr calls = %q, want ctrl+u twice per row of the box", got)
	}
	assertDraft(t, mode.DraftFile("w1:p1"), "this unsent draft is long enough to wrap across two rows of the Claude "+
		"input box, so pane read drops the space at the wrap")
}

func altXSendsNoKeyAndMovesNothingFromPlaceholderEmptyBoxOrBlockedClaude(t *testing.T) {
	blockedPane := claudePane("w1:p1", true)
	blockedPane.AgentStatus = "blocked"
	cases := []struct {
		name    string
		pane    herdrPane
		capture string
	}{
		{name: "placeholder", pane: claudePane("w1:p1", true), capture: "claude-input-placeholder.ansi"},
		{name: "empty box", pane: claudePane("w1:p1", true), capture: "claude-input-empty.ansi"},
		{name: "blocked", pane: blockedPane, capture: "claude-input-one-row.ansi"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			herdr, mode, calls := fakeHerdr(t, []herdrPane{testCase.pane})
			ShowOnFakeScreen(t, "w1:p1", testCase.capture, "")
			if err := ToggleZenMode(herdr, mode); err != nil {
				t.Fatal(err)
			}
			for _, call := range calls() {
				if strings.HasPrefix(call, "pane send-keys") {
					t.Errorf("herdr calls = %q, want no send-keys", calls())
				}
			}
			if _, err := os.Stat(mode.DraftFile("w1:p1")); !os.IsNotExist(err) {
				t.Errorf("draft file: %v, want none", err)
			}
		})
	}
}

func altXFailsNamingPaneAndMovesNothingWhenClaudesInputStillHoldsTextAfterClearKeys(t *testing.T) {
	herdr, mode, calls := fakeHerdr(t, []herdrPane{claudePane("w1:p1", true)})
	ShowOnFakeScreen(t, "w1:p1", "claude-input-one-row.ansi", "")
	err := ToggleZenMode(herdr, mode)
	if err == nil || !strings.Contains(err.Error(), "pane w1:p1 still holds text") {
		t.Errorf("ToggleZenMode = %v, want an error naming pane w1:p1", err)
	}
	if isOn, err := mode.IsOn(); err != nil || isOn {
		t.Errorf("IsOn = %v, %v, want false", isOn, err)
	}
	if _, err := os.Stat(mode.DraftFile("w1:p1")); !os.IsNotExist(err) {
		t.Errorf("draft file: %v, want none", err)
	}
	for _, call := range calls() {
		if strings.HasPrefix(call, "plugin pane open") {
			t.Errorf("herdr calls = %q, want no zen pane opened", calls())
		}
	}
}

func altXTurnsZenModeOffClosesEveryZenPaneMovesItsDraftAndFocusesClaudePane(t *testing.T) {
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

func altXTurningZenModeOffLeavesFocusOnPaneThatIsNotZenPane(t *testing.T) {
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

func zenViewSavesUnsentTextAsDraftOfItsClaudePane(t *testing.T) {
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
