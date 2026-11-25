package zenmode

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/CLBRITTON2/calmpane/internal/herdr"
	"github.com/CLBRITTON2/calmpane/internal/herdr/herdrtest"
	"github.com/CLBRITTON2/calmpane/internal/ui"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	herdrtest.Main(m)
}

// testMode is a zen mode of its own for each test, so tests do not share a mode file, drafts or a lock.
func testMode(t *testing.T) Mode {
	return Mode{stateDir: t.TempDir(), session: "zentest", mutexName: `Local\zen-test.` + t.Name()}
}

func herdrCallsStartingWith(herdrCalls []string, prefix string) []string {
	var matching []string
	for _, call := range herdrCalls {
		if strings.HasPrefix(call, prefix) {
			matching = append(matching, call)
		}
	}
	return matching
}

func assertNoDraftFile(t *testing.T, mode Mode, claudePaneID string) {
	draftFile := mode.DraftFile(claudePaneID)
	_, statErr := os.Stat(draftFile)
	require.ErrorIs(t, statErr, fs.ErrNotExist)
}

func TestZenPaneAction(t *testing.T) {
	t.Run("zenPaneActionOpensViewForFocusedClaudePaneWithoutZenPane", zenPaneActionOpensViewForFocusedClaudePaneWithoutZenPane)
	t.Run("zenPaneActionShowsZenPaneOfFocusedClaudePane", zenPaneActionShowsZenPaneOfFocusedClaudePane)
	t.Run("zenPaneActionIsNoneForFocusedShellPane", zenPaneActionIsNoneForFocusedShellPane)
	t.Run("zenPaneActionIsNoneForFocusedZenPane", zenPaneActionIsNoneForFocusedZenPane)
}

func zenPaneActionOpensViewForFocusedClaudePaneWithoutZenPane(t *testing.T) {
	panes := []herdr.Pane{herdrtest.ClaudePane("w1:p1", true), herdrtest.ClaudePane("w1:p2", false), herdrtest.ZenPane("w1:p3", "w1:p2", false)}
	action, err := ZenPaneActionForFocusedPane(panes)
	require.NoError(t, err)
	want := zenPaneAction{Kind: zenPaneActionOpen, ClaudePaneID: "w1:p1"}
	require.Equal(t, want, action)
}

func zenPaneActionShowsZenPaneOfFocusedClaudePane(t *testing.T) {
	panes := []herdr.Pane{herdrtest.ClaudePane("w1:p1", true), herdrtest.ZenPane("w1:p3", "w1:p1", false)}
	action, err := ZenPaneActionForFocusedPane(panes)
	require.NoError(t, err)
	want := zenPaneAction{Kind: zenPaneActionShow, ClaudePaneID: "w1:p1", ZenPaneID: "w1:p3"}
	require.Equal(t, want, action)
}

func zenPaneActionIsNoneForFocusedShellPane(t *testing.T) {
	panes := []herdr.Pane{{PaneID: "w1:p1", Focused: true}, herdrtest.ClaudePane("w1:p2", false)}
	action, err := ZenPaneActionForFocusedPane(panes)
	require.NoError(t, err)
	want := zenPaneAction{Kind: zenPaneActionNone}
	require.Equal(t, want, action)
}

func zenPaneActionIsNoneForFocusedZenPane(t *testing.T) {
	panes := []herdr.Pane{herdrtest.ClaudePane("w1:p1", false), herdrtest.ZenPane("w1:p3", "w1:p1", true)}
	action, err := ZenPaneActionForFocusedPane(panes)
	require.NoError(t, err)
	want := zenPaneAction{Kind: zenPaneActionNone}
	require.Equal(t, want, action)
}

func TestFocusHook(t *testing.T) {
	t.Run("focusHookDoesNothingWhileZenModeIsOff", focusHookDoesNothingWhileZenModeIsOff)
	t.Run("focusHookShowsExistingZenPaneOfFocusedClaudePane", focusHookShowsExistingZenPaneOfFocusedClaudePane)
	t.Run("focusHooksRunTogetherOpenOneZenPaneForClaudePane", focusHooksRunTogetherOpenOneZenPaneForClaudePane)
}

func focusHookDoesNothingWhileZenModeIsOff(t *testing.T) {
	cli, calls := herdrtest.Fake(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", true)})
	mode := testMode(t)
	err := ShowFocusedZenView(cli, mode)
	require.NoError(t, err)
	herdrCalls := calls()
	require.Empty(t, herdrCalls)
}

func focusHookShowsExistingZenPaneOfFocusedClaudePane(t *testing.T) {
	cli, calls := herdrtest.Fake(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", true), herdrtest.ZenPane("w1:p3", "w1:p1", false)})
	mode := testMode(t)
	modeFile := mode.ModeFile()
	modeErr := os.WriteFile(modeFile, []byte{}, 0o644)
	require.NoError(t, modeErr)
	showErr := ShowFocusedZenView(cli, mode)
	require.NoError(t, showErr)
	herdrCalls := calls()
	wantCalls := []string{"pane list", "plugin pane focus w1:p3", "pane zoom w1:p3 --on"}
	require.Equal(t, wantCalls, herdrCalls)
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
	err := FocusAgentOrItsZenPane(cli, mode, 2)
	require.NoError(t, err)
	herdrCalls := calls()
	wantCalls := []string{"agent list", "agent focus w2:p1"}
	require.Equal(t, wantCalls, herdrCalls)
}

func focusAgentShowsZenPaneOfNumberedAgentWithoutFocusingAgent(t *testing.T) {
	agentZenPane := herdrtest.ZenPane("w1:p4", "w1:p3", false)
	agentZenPane.TabID = "w1:t2"
	cli, calls := herdrtest.Fake(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", false), herdrtest.ZenPane("w1:p2", "w1:p1", true),
		herdrtest.ClaudePane("w1:p3", false), agentZenPane})
	mode := testMode(t)
	modeFile := mode.ModeFile()
	modeErr := os.WriteFile(modeFile, []byte{}, 0o644)
	require.NoError(t, modeErr)
	focusErr := FocusAgentOrItsZenPane(cli, mode, 2)
	require.NoError(t, focusErr)
	herdrCalls := calls()
	wantCalls := []string{"agent list", "pane list", "tab focus w1:t2", "plugin pane focus w1:p4", "pane zoom w1:p4 --on"}
	require.Equal(t, wantCalls, herdrCalls)
}

func focusAgentFocusesNumberedAgentWithoutZenPaneForFocusHookToOpenOne(t *testing.T) {
	cli, calls := herdrtest.Fake(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", false), herdrtest.ZenPane("w1:p2", "w1:p1", true),
		herdrtest.ClaudePane("w2:p1", false)})
	mode := testMode(t)
	modeFile := mode.ModeFile()
	modeErr := os.WriteFile(modeFile, []byte{}, 0o644)
	require.NoError(t, modeErr)
	focusErr := FocusAgentOrItsZenPane(cli, mode, 2)
	require.NoError(t, focusErr)
	herdrCalls := calls()
	wantCalls := []string{"agent list", "pane list", "agent focus w2:p1"}
	require.Equal(t, wantCalls, herdrCalls)
}

func focusAgentFailsForNumberPastLastAgent(t *testing.T) {
	cli, calls := herdrtest.Fake(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", true)})
	mode := testMode(t)
	err := FocusAgentOrItsZenPane(cli, mode, 2)
	require.Error(t, err)
	herdrCalls := calls()
	wantCalls := []string{"agent list"}
	require.Equal(t, wantCalls, herdrCalls)
}

func focusHooksRunTogetherOpenOneZenPaneForClaudePane(t *testing.T) {
	cli, calls := herdrtest.Fake(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", true)})
	mode := testMode(t)
	modeFile := mode.ModeFile()
	modeErr := os.WriteFile(modeFile, []byte{}, 0o644)
	require.NoError(t, modeErr)
	var hooks sync.WaitGroup
	hookErrs := make(chan error, 4)
	for range 4 {
		hooks.Go(func() { hookErrs <- ShowFocusedZenView(cli, mode) })
	}
	hooks.Wait()
	close(hookErrs)
	for hookErr := range hookErrs {
		require.NoError(t, hookErr)
	}
	herdrCalls := calls()
	openCalls := herdrCallsStartingWith(herdrCalls, "plugin pane open")
	require.Len(t, openCalls, 1)
}

func TestToggle(t *testing.T) {
	t.Run("altXTurnsZenModeOnMovesClaudesDraftAfterZenDraftKeepingItsPastesAndOpensAndMarksZenView",
		altXTurnsZenModeOnMovesClaudesDraftAfterZenDraftKeepingItsPastesAndOpensAndMarksZenView)
	t.Run("altXMovesWrappedDraftOutOfClaudesInputAsOneLine", altXMovesWrappedDraftOutOfClaudesInputAsOneLine)
	t.Run("altXSendsNoKeyAndMovesNothingFromPlaceholder", altXSendsNoKeyAndMovesNothingFromPlaceholder)
	t.Run("altXSendsNoKeyAndMovesNothingFromEmptyBox", altXSendsNoKeyAndMovesNothingFromEmptyBox)
	t.Run("altXSendsNoKeyAndMovesNothingFromBlockedClaude", altXSendsNoKeyAndMovesNothingFromBlockedClaude)
	t.Run("altXFailsNamingPaneAndMovesNothingWhenClaudesInputStillHoldsTextAfterClearKeys",
		altXFailsNamingPaneAndMovesNothingWhenClaudesInputStillHoldsTextAfterClearKeys)
	t.Run("altXRefusesAndSendsNoKeyWhenClaudesInputHoldsPasteLabel", altXRefusesAndSendsNoKeyWhenClaudesInputHoldsPasteLabel)
	t.Run("altXMovesTypedPastedTextWithoutLabelNumber", altXMovesTypedPastedTextWithoutLabelNumber)
	t.Run("altXTurnsZenModeOffClosesEveryZenPaneMovesItsDraftAndFocusesClaudePane",
		altXTurnsZenModeOffClosesEveryZenPaneMovesItsDraftAndFocusesClaudePane)
	t.Run("altXTurningZenModeOffLeavesFocusOnPaneThatIsNotZenPane", altXTurningZenModeOffLeavesFocusOnPaneThatIsNotZenPane)
	t.Run("altXRefusedShowsErrorInSilentNotificationTitledZenMode", altXRefusedShowsErrorInSilentNotificationTitledZenMode)
	t.Run("altXSucceedingShowsNoNotification", altXSucceedingShowsNoNotification)
	t.Run("altXFailingNotificationKeepsToggleErrorAndAddsItsOwn", altXFailingNotificationKeepsToggleErrorAndAddsItsOwn)
}

func altXRefusedShowsErrorInSilentNotificationTitledZenMode(t *testing.T) {
	cli, calls := herdrtest.Fake(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", true)})
	mode := testMode(t)
	herdrtest.ShowOnScreen(t, "w1:p1", "claude-input-pasted-label.ansi", "")
	err := ToggleAndNotifyFailure(cli, herdr.API{}, mode)
	require.ErrorIs(t, err, ui.ErrClaudeInputHoldsPaste)
	herdrCalls := calls()
	wantCalls := []string{
		"pane list",
		"pane read w1:p1 --source visible --format ansi",
		"notification show Zen mode --body " + ui.ErrClaudeInputHoldsPaste.Error() + " --sound none",
	}
	require.Equal(t, wantCalls, herdrCalls)
}

func altXSucceedingShowsNoNotification(t *testing.T) {
	cli, calls := herdrtest.Fake(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", true)})
	mode := testMode(t)
	herdrtest.ShowOnScreen(t, "w1:p1", "claude-input-empty.ansi", "")
	err := ToggleAndNotifyFailure(cli, herdr.API{}, mode)
	require.NoError(t, err)
	herdrCalls := calls()
	notificationCalls := herdrCallsStartingWith(herdrCalls, "notification")
	require.Empty(t, notificationCalls)
}

func altXFailingNotificationKeepsToggleErrorAndAddsItsOwn(t *testing.T) {
	cli := herdr.CLI{Binary: filepath.Join(t.TempDir(), "missing-herdr.exe"), PluginID: "zen-test"}
	mode := testMode(t)
	err := ToggleAndNotifyFailure(cli, herdr.API{}, mode)
	require.ErrorContains(t, err, "herdr pane list failed")
	require.ErrorContains(t, err, "herdr notification show failed")
}

func altXTurnsZenModeOnMovesClaudesDraftAfterZenDraftKeepingItsPastesAndOpensAndMarksZenView(t *testing.T) {
	cli, calls := herdrtest.Fake(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", true)})
	mode := testMode(t)
	herdrtest.ShowOnScreen(t, "w1:p1", "claude-input-one-row.ansi", "claude-input-empty.ansi")
	pastes := []string{"a\nb\nc\nd"}
	draftFile := mode.DraftFile("w1:p1")
	writeErr := WriteDraft(draftFile, ui.Draft{Text: "left by a closed zen pane [Pasted text #1 +3 lines]", Pastes: pastes})
	require.NoError(t, writeErr)
	toggleErr := Toggle(cli, herdr.API{}, mode)
	require.NoError(t, toggleErr)
	isOn, isOnErr := mode.IsOn()
	require.NoError(t, isOnErr)
	require.True(t, isOn)
	herdrCalls := calls()
	wantCalls := []string{
		"pane list",
		"pane read w1:p1 --source visible --format ansi",
		"pane send-keys w1:p1 ctrl+u ctrl+u",
		"pane read w1:p1 --source visible --format ansi",
		"plugin pane open --plugin zen-test --entrypoint zen --placement zoomed --target-pane w1:p1 --env " +
			"ZEN_CLAUDE_PANE=w1:p1 --focus",
		"pane report-metadata w1:z1 --source zen-test --token claude_pane=w1:p1",
	}
	require.Equal(t, wantCalls, herdrCalls)
	assertDraft(t, draftFile, ui.Draft{Text: "left by a closed zen pane [Pasted text #1 +3 lines]\nmove this draft", Pastes: pastes})
}

func assertDraft(t *testing.T, draftFile string, want ui.Draft) {
	draft, err := SavedDraft(draftFile)
	require.NoError(t, err)
	require.Equal(t, want, draft)
}

func altXMovesWrappedDraftOutOfClaudesInputAsOneLine(t *testing.T) {
	cli, calls := herdrtest.Fake(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", true)})
	mode := testMode(t)
	herdrtest.ShowOnScreen(t, "w1:p1", "claude-input-wrapped.ansi", "claude-input-empty.ansi")
	err := Toggle(cli, herdr.API{}, mode)
	require.NoError(t, err)
	herdrCalls := calls()
	require.Contains(t, herdrCalls, "pane send-keys w1:p1 ctrl+u ctrl+u ctrl+u ctrl+u")
	draftFile := mode.DraftFile("w1:p1")
	assertDraft(t, draftFile, ui.Draft{Text: "this unsent draft is long enough to wrap across two rows of " +
		"the Claude input box, so pane read drops the space at the wrap"})
}

func assertToggleSendsNoKeyAndWritesNoDraft(t *testing.T, claudePane herdr.Pane, capture string) {
	cli, calls := herdrtest.Fake(t, []herdr.Pane{claudePane})
	mode := testMode(t)
	herdrtest.ShowOnScreen(t, "w1:p1", capture, "")
	err := Toggle(cli, herdr.API{}, mode)
	require.NoError(t, err)
	herdrCalls := calls()
	sendKeyCalls := herdrCallsStartingWith(herdrCalls, "pane send-keys")
	require.Empty(t, sendKeyCalls)
	assertNoDraftFile(t, mode, "w1:p1")
}

func altXSendsNoKeyAndMovesNothingFromPlaceholder(t *testing.T) {
	claudePane := herdrtest.ClaudePane("w1:p1", true)
	assertToggleSendsNoKeyAndWritesNoDraft(t, claudePane, "claude-input-placeholder.ansi")
}

func altXSendsNoKeyAndMovesNothingFromEmptyBox(t *testing.T) {
	claudePane := herdrtest.ClaudePane("w1:p1", true)
	assertToggleSendsNoKeyAndWritesNoDraft(t, claudePane, "claude-input-empty.ansi")
}

func altXSendsNoKeyAndMovesNothingFromBlockedClaude(t *testing.T) {
	blockedPane := herdrtest.ClaudePane("w1:p1", true)
	blockedPane.AgentStatus = "blocked"
	assertToggleSendsNoKeyAndWritesNoDraft(t, blockedPane, "claude-input-one-row.ansi")
}

func altXFailsNamingPaneAndMovesNothingWhenClaudesInputStillHoldsTextAfterClearKeys(t *testing.T) {
	cli, calls := herdrtest.Fake(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", true)})
	mode := testMode(t)
	herdrtest.ShowOnScreen(t, "w1:p1", "claude-input-one-row.ansi", "")
	toggleErr := Toggle(cli, herdr.API{}, mode)
	require.ErrorContains(t, toggleErr, "pane w1:p1 still holds text")
	isOn, isOnErr := mode.IsOn()
	require.NoError(t, isOnErr)
	require.False(t, isOn)
	assertNoDraftFile(t, mode, "w1:p1")
	herdrCalls := calls()
	openCalls := herdrCallsStartingWith(herdrCalls, "plugin pane open")
	require.Empty(t, openCalls)
}

func altXRefusesAndSendsNoKeyWhenClaudesInputHoldsPasteLabel(t *testing.T) {
	cli, calls := herdrtest.Fake(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", true)})
	mode := testMode(t)
	herdrtest.ShowOnScreen(t, "w1:p1", "claude-input-pasted-label.ansi", "claude-input-empty.ansi")
	toggleErr := Toggle(cli, herdr.API{}, mode)
	require.ErrorIs(t, toggleErr, ui.ErrClaudeInputHoldsPaste)
	isOn, isOnErr := mode.IsOn()
	require.NoError(t, isOnErr)
	require.False(t, isOn)
	assertNoDraftFile(t, mode, "w1:p1")
	herdrCalls := calls()
	wantCalls := []string{"pane list", "pane read w1:p1 --source visible --format ansi"}
	require.Equal(t, wantCalls, herdrCalls)
}

func altXMovesTypedPastedTextWithoutLabelNumber(t *testing.T) {
	cli, _ := herdrtest.Fake(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", true)})
	mode := testMode(t)
	herdrtest.ShowOnScreen(t, "w1:p1", "claude-input-typed-label-start.ansi", "claude-input-empty.ansi")
	err := Toggle(cli, herdr.API{}, mode)
	require.NoError(t, err)
	draftFile := mode.DraftFile("w1:p1")
	assertDraft(t, draftFile, ui.Draft{Text: "[Pasted text"})
}

func altXTurnsZenModeOffClosesEveryZenPaneMovesItsDraftAndFocusesClaudePane(t *testing.T) {
	panes := []herdr.Pane{
		herdrtest.ClaudePane("w1:p1", false),
		herdrtest.ZenPane("w1:p2", "w1:p1", true),
		herdrtest.ClaudePane("w1:p3", false),
		herdrtest.ZenPane("w1:p4", "w1:p3", false),
		{PaneID: "w1:p5", Label: herdr.ZenPaneTitle},
	}
	cli, calls := herdrtest.Fake(t, panes)
	api, requests := herdrtest.FakeAPI(t, panes)
	mode := testMode(t)
	modeFile := mode.ModeFile()
	modeErr := os.WriteFile(modeFile, []byte{}, 0o644)
	require.NoError(t, modeErr)
	claudeDraftFile := mode.DraftFile("w1:p1")
	draftErr := WriteDraft(claudeDraftFile, ui.Draft{Text: "first line\nsecond line"})
	require.NoError(t, draftErr)
	emptyDraftFile := mode.DraftFile("w1:p3")
	emptyDraftErr := os.WriteFile(emptyDraftFile, []byte{}, 0o644)
	require.NoError(t, emptyDraftErr)
	toggleErr := Toggle(cli, api, mode)
	require.NoError(t, toggleErr)
	isOn, isOnErr := mode.IsOn()
	require.NoError(t, isOnErr)
	require.False(t, isOn)
	herdrCalls := calls()
	wantCalls := []string{
		"pane list",
		"pane close w1:p2",
		"pane close w1:p4",
		"pane close w1:p5",
		"agent focus w1:p1",
	}
	require.Equal(t, wantCalls, herdrCalls)
	apiRequests := requests()
	wantRequests := []string{"pane.send_text w1:p1 first line\nsecond line"}
	require.Equal(t, wantRequests, apiRequests)
	assertNoDraftFile(t, mode, "w1:p1")
	assertNoDraftFile(t, mode, "w1:p3")
}

func altXTurningZenModeOffLeavesFocusOnPaneThatIsNotZenPane(t *testing.T) {
	cli, calls := herdrtest.Fake(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", true), herdrtest.ZenPane("w1:p2", "w1:p3", false),
		herdrtest.ClaudePane("w1:p3", false)})
	mode := testMode(t)
	modeFile := mode.ModeFile()
	modeErr := os.WriteFile(modeFile, []byte{}, 0o644)
	require.NoError(t, modeErr)
	toggleErr := Toggle(cli, herdr.API{}, mode)
	require.NoError(t, toggleErr)
	herdrCalls := calls()
	wantCalls := []string{"pane list", "pane close w1:p2"}
	require.Equal(t, wantCalls, herdrCalls)
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
	turnOnErr := TurnOnAndShowFocusedZenView(cli, mode)
	require.NoError(t, turnOnErr)
	isOn, isOnErr := mode.IsOn()
	require.NoError(t, isOnErr)
	require.True(t, isOn)
	herdrCalls := calls()
	wantCalls := []string{
		"pane list",
		"pane read w1:p1 --source visible --format ansi",
		"plugin pane open --plugin zen-test --entrypoint zen --placement zoomed --target-pane w1:p1 --env " +
			"ZEN_CLAUDE_PANE=w1:p1 --focus",
		"pane report-metadata w1:z1 --source zen-test --token claude_pane=w1:p1",
	}
	require.Equal(t, wantCalls, herdrCalls)
}

func turnOnWhileOnKeepsZenModeOnAndShowsExistingZenPane(t *testing.T) {
	cli, calls := herdrtest.Fake(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", true), herdrtest.ZenPane("w1:p3", "w1:p1", false)})
	mode := testMode(t)
	modeFile := mode.ModeFile()
	modeErr := os.WriteFile(modeFile, []byte{}, 0o644)
	require.NoError(t, modeErr)
	turnOnErr := TurnOnAndShowFocusedZenView(cli, mode)
	require.NoError(t, turnOnErr)
	isOn, isOnErr := mode.IsOn()
	require.NoError(t, isOnErr)
	require.True(t, isOn)
	herdrCalls := calls()
	wantCalls := []string{"pane list", "plugin pane focus w1:p3", "pane zoom w1:p3 --on"}
	require.Equal(t, wantCalls, herdrCalls)
}

func turnOnWhileOnKeepsZenModeOnWhenZenPaneFailsToOpen(t *testing.T) {
	cli, _ := herdrtest.Fake(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", true)})
	mode := testMode(t)
	modeFile := mode.ModeFile()
	modeErr := os.WriteFile(modeFile, []byte{}, 0o644)
	require.NoError(t, modeErr)
	herdrtest.ShowOnScreen(t, "w1:p1", "claude-input-one-row.ansi", "")
	turnOnErr := TurnOnAndShowFocusedZenView(cli, mode)
	require.ErrorContains(t, turnOnErr, "pane w1:p1 still holds text")
	isOn, isOnErr := mode.IsOn()
	require.NoError(t, isOnErr)
	require.True(t, isOn)
}
