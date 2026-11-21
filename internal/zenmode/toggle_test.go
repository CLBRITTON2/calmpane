package zenmode

import (
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/CLBRITTON2/calmpane/internal/herdr"
	"github.com/CLBRITTON2/calmpane/internal/herdr/herdrtest"
)

func TestMain(m *testing.M) {
	herdrtest.Main(m)
}

// testMode is a zen mode of its own for each test, so tests do not share a mode file, drafts or a lock.
func testMode(t *testing.T) Mode {
	return Mode{stateDir: t.TempDir(), session: "zentest", mutexName: `Local\zen-test.` + t.Name()}
}

func TestZenPaneAction(t *testing.T) {
	t.Run("zenPaneActionOpensViewOnlyForClaudePaneWithoutOneAndShowsExistingOne",
		zenPaneActionOpensViewOnlyForClaudePaneWithoutOneAndShowsExistingOne)
}

func zenPaneActionOpensViewOnlyForClaudePaneWithoutOneAndShowsExistingOne(t *testing.T) {
	cases := []struct {
		name  string
		panes []herdr.Pane
		want  zenPaneAction
	}{
		{
			name:  "Claude pane without a zen pane",
			panes: []herdr.Pane{herdrtest.ClaudePane("w1:p1", true), herdrtest.ClaudePane("w1:p2", false), herdrtest.ZenPane("w1:p3", "w1:p2", false)},
			want:  zenPaneAction{Kind: zenPaneActionOpen, ClaudePaneID: "w1:p1"},
		},
		{
			name:  "Claude pane with a zen pane",
			panes: []herdr.Pane{herdrtest.ClaudePane("w1:p1", true), herdrtest.ZenPane("w1:p3", "w1:p1", false)},
			want:  zenPaneAction{Kind: zenPaneActionShow, ClaudePaneID: "w1:p1", ZenPaneID: "w1:p3"},
		},
		{
			name:  "shell pane",
			panes: []herdr.Pane{{PaneID: "w1:p1", Focused: true}, herdrtest.ClaudePane("w1:p2", false)},
			want:  zenPaneAction{Kind: zenPaneActionNone},
		},
		{
			name:  "zen pane",
			panes: []herdr.Pane{herdrtest.ClaudePane("w1:p1", false), herdrtest.ZenPane("w1:p3", "w1:p1", true)},
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
	cli, calls := herdrtest.Fake(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", true)})
	mode := testMode(t)
	if err := ShowFocusedZenView(cli, mode); err != nil {
		t.Fatal(err)
	}
	if got := calls(); len(got) != 0 {
		t.Errorf("herdr calls = %q, want none", got)
	}
}

func focusHookShowsExistingZenPaneOfFocusedClaudePane(t *testing.T) {
	cli, calls := herdrtest.Fake(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", true), herdrtest.ZenPane("w1:p3", "w1:p1", false)})
	mode := testMode(t)
	if err := os.WriteFile(mode.ModeFile(), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ShowFocusedZenView(cli, mode); err != nil {
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
	cli, calls := herdrtest.Fake(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", true), herdrtest.ClaudePane("w2:p1", false)})
	mode := testMode(t)
	if err := FocusAgentOrItsZenPane(cli, mode, 2); err != nil {
		t.Fatal(err)
	}
	want := []string{"agent list", "agent focus w2:p1"}
	if got := calls(); !reflect.DeepEqual(got, want) {
		t.Errorf("herdr calls = %q, want %q", got, want)
	}
}

func focusAgentShowsZenPaneOfNumberedAgentWithoutFocusingAgent(t *testing.T) {
	agentZenPane := herdrtest.ZenPane("w1:p4", "w1:p3", false)
	agentZenPane.TabID = "w1:t2"
	cli, calls := herdrtest.Fake(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", false), herdrtest.ZenPane("w1:p2", "w1:p1", true),
		herdrtest.ClaudePane("w1:p3", false), agentZenPane})
	mode := testMode(t)
	if err := os.WriteFile(mode.ModeFile(), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := FocusAgentOrItsZenPane(cli, mode, 2); err != nil {
		t.Fatal(err)
	}
	want := []string{"agent list", "pane list", "tab focus w1:t2", "plugin pane focus w1:p4", "pane zoom w1:p4 --on"}
	if got := calls(); !reflect.DeepEqual(got, want) {
		t.Errorf("herdr calls = %q, want %q", got, want)
	}
}

func focusAgentFocusesNumberedAgentWithoutZenPaneForFocusHookToOpenOne(t *testing.T) {
	cli, calls := herdrtest.Fake(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", false), herdrtest.ZenPane("w1:p2", "w1:p1", true),
		herdrtest.ClaudePane("w2:p1", false)})
	mode := testMode(t)
	if err := os.WriteFile(mode.ModeFile(), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := FocusAgentOrItsZenPane(cli, mode, 2); err != nil {
		t.Fatal(err)
	}
	want := []string{"agent list", "pane list", "agent focus w2:p1"}
	if got := calls(); !reflect.DeepEqual(got, want) {
		t.Errorf("herdr calls = %q, want %q", got, want)
	}
}

func focusAgentFailsForNumberPastLastAgent(t *testing.T) {
	cli, calls := herdrtest.Fake(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", true)})
	mode := testMode(t)
	if err := FocusAgentOrItsZenPane(cli, mode, 2); err == nil {
		t.Fatal("FocusAgentOrItsZenPane(2) with one agent returned no error")
	}
	want := []string{"agent list"}
	if got := calls(); !reflect.DeepEqual(got, want) {
		t.Errorf("herdr calls = %q, want %q", got, want)
	}
}

func focusHooksRunTogetherOpenOneZenPaneForClaudePane(t *testing.T) {
	cli, calls := herdrtest.Fake(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", true)})
	mode := testMode(t)
	if err := os.WriteFile(mode.ModeFile(), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	var hooks sync.WaitGroup
	errs := make(chan error, 4)
	for range 4 {
		hooks.Go(func() { errs <- ShowFocusedZenView(cli, mode) })
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
}

func altXTurnsZenModeOnMovesClaudesDraftAfterZenDraftAndOpensAndMarksZenView(t *testing.T) {
	cli, calls := herdrtest.Fake(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", true)})
	mode := testMode(t)
	herdrtest.ShowOnScreen(t, "w1:p1", "claude-input-one-row.ansi", "claude-input-empty.ansi")
	if err := os.WriteFile(mode.DraftFile("w1:p1"), []byte("left by a closed zen pane"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Toggle(cli, mode); err != nil {
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
	cli, calls := herdrtest.Fake(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", true)})
	mode := testMode(t)
	herdrtest.ShowOnScreen(t, "w1:p1", "claude-input-wrapped.ansi", "claude-input-empty.ansi")
	if err := Toggle(cli, mode); err != nil {
		t.Fatal(err)
	}
	if got := calls(); !slices.Contains(got, "pane send-keys w1:p1 ctrl+u ctrl+u ctrl+u ctrl+u") {
		t.Errorf("herdr calls = %q, want ctrl+u twice per row of the box", got)
	}
	assertDraft(t, mode.DraftFile("w1:p1"), "this unsent draft is long enough to wrap across two rows of the Claude "+
		"input box, so pane read drops the space at the wrap")
}

func altXSendsNoKeyAndMovesNothingFromPlaceholderEmptyBoxOrBlockedClaude(t *testing.T) {
	blockedPane := herdrtest.ClaudePane("w1:p1", true)
	blockedPane.AgentStatus = "blocked"
	cases := []struct {
		name    string
		pane    herdr.Pane
		capture string
	}{
		{name: "placeholder", pane: herdrtest.ClaudePane("w1:p1", true), capture: "claude-input-placeholder.ansi"},
		{name: "empty box", pane: herdrtest.ClaudePane("w1:p1", true), capture: "claude-input-empty.ansi"},
		{name: "blocked", pane: blockedPane, capture: "claude-input-one-row.ansi"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			cli, calls := herdrtest.Fake(t, []herdr.Pane{testCase.pane})
			mode := testMode(t)
			herdrtest.ShowOnScreen(t, "w1:p1", testCase.capture, "")
			if err := Toggle(cli, mode); err != nil {
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
	cli, calls := herdrtest.Fake(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", true)})
	mode := testMode(t)
	herdrtest.ShowOnScreen(t, "w1:p1", "claude-input-one-row.ansi", "")
	err := Toggle(cli, mode)
	if err == nil || !strings.Contains(err.Error(), "pane w1:p1 still holds text") {
		t.Errorf("Toggle = %v, want an error naming pane w1:p1", err)
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
	cli, calls := herdrtest.Fake(t, []herdr.Pane{
		herdrtest.ClaudePane("w1:p1", false),
		herdrtest.ZenPane("w1:p2", "w1:p1", true),
		herdrtest.ClaudePane("w1:p3", false),
		herdrtest.ZenPane("w1:p4", "w1:p3", false),
		{PaneID: "w1:p5", Label: herdr.ZenPaneTitle},
	})
	mode := testMode(t)
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
	if err := Toggle(cli, mode); err != nil {
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
	cli, calls := herdrtest.Fake(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", true), herdrtest.ZenPane("w1:p2", "w1:p3", false),
		herdrtest.ClaudePane("w1:p3", false)})
	mode := testMode(t)
	if err := os.WriteFile(mode.ModeFile(), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Toggle(cli, mode); err != nil {
		t.Fatal(err)
	}
	want := []string{"pane list", "pane close w1:p2"}
	if got := calls(); !reflect.DeepEqual(got, want) {
		t.Errorf("herdr calls = %q, want %q", got, want)
	}
}

func TestTurnOn(t *testing.T) {
	t.Run("turnOnWhileOffTurnsZenModeOnAndOpensZenViewOfFocusedClaudePane",
		turnOnWhileOffTurnsZenModeOnAndOpensZenViewOfFocusedClaudePane)
	t.Run("turnOnWhileOnKeepsZenModeOnAndShowsExistingZenPane", turnOnWhileOnKeepsZenModeOnAndShowsExistingZenPane)
	t.Run("turnOnWhileOnKeepsZenModeOnWhenZenPaneFailsToOpen", turnOnWhileOnKeepsZenModeOnWhenZenPaneFailsToOpen)
}

func turnOnWhileOffTurnsZenModeOnAndOpensZenViewOfFocusedClaudePane(t *testing.T) {
	cli, calls := herdrtest.Fake(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", true)})
	mode := testMode(t)
	herdrtest.ShowOnScreen(t, "w1:p1", "claude-input-empty.ansi", "")
	if err := TurnOnAndShowFocusedZenView(cli, mode); err != nil {
		t.Fatal(err)
	}
	if isOn, err := mode.IsOn(); err != nil || !isOn {
		t.Errorf("IsOn = %v, %v, want true", isOn, err)
	}
	want := []string{
		"pane list",
		"pane read w1:p1 --source visible --format ansi",
		"plugin pane open --plugin zen-test --entrypoint zen --placement zoomed --target-pane w1:p1 --env " +
			"ZEN_CLAUDE_PANE=w1:p1 --focus",
		"pane report-metadata w1:z1 --source zen-test --token claude_pane=w1:p1",
	}
	if got := calls(); !reflect.DeepEqual(got, want) {
		t.Errorf("herdr calls = %q, want %q", got, want)
	}
}

func turnOnWhileOnKeepsZenModeOnAndShowsExistingZenPane(t *testing.T) {
	cli, calls := herdrtest.Fake(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", true), herdrtest.ZenPane("w1:p3", "w1:p1", false)})
	mode := testMode(t)
	if err := os.WriteFile(mode.ModeFile(), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := TurnOnAndShowFocusedZenView(cli, mode); err != nil {
		t.Fatal(err)
	}
	if isOn, err := mode.IsOn(); err != nil || !isOn {
		t.Errorf("IsOn = %v, %v, want true", isOn, err)
	}
	want := []string{"pane list", "plugin pane focus w1:p3", "pane zoom w1:p3 --on"}
	if got := calls(); !reflect.DeepEqual(got, want) {
		t.Errorf("herdr calls = %q, want %q", got, want)
	}
}

func turnOnWhileOnKeepsZenModeOnWhenZenPaneFailsToOpen(t *testing.T) {
	cli, _ := herdrtest.Fake(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", true)})
	mode := testMode(t)
	if err := os.WriteFile(mode.ModeFile(), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	herdrtest.ShowOnScreen(t, "w1:p1", "claude-input-one-row.ansi", "")
	err := TurnOnAndShowFocusedZenView(cli, mode)
	if err == nil || !strings.Contains(err.Error(), "pane w1:p1 still holds text") {
		t.Errorf("TurnOnAndShowFocusedZenView = %v, want an error naming pane w1:p1", err)
	}
	if isOn, err := mode.IsOn(); err != nil || !isOn {
		t.Errorf("IsOn = %v, %v, want true", isOn, err)
	}
}
