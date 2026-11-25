package ui

import (
	"errors"
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
)

type terminalAgentRecorder struct {
	agentRecorder
	screen        string
	inputErr      error
	screenChanged chan struct{}
}

func (r *terminalAgentRecorder) Screen() (string, *tea.Cursor) {
	return r.screen, tea.NewCursor(2, 1)
}

func (r *terminalAgentRecorder) SendKey(key tea.KeyPressMsg) error {
	r.calls = append(r.calls, "key "+key.String())
	return r.inputErr
}

func (r *terminalAgentRecorder) SendPaste(text string) error {
	r.calls = append(r.calls, "paste "+text)
	return r.inputErr
}

func (r *terminalAgentRecorder) Resize(width int, height int) error {
	r.calls = append(r.calls, fmt.Sprintf("resize %dx%d", width, height))
	return r.inputErr
}

func (r *terminalAgentRecorder) ScreenChanged() <-chan struct{} {
	return r.screenChanged
}

func (r *terminalAgentRecorder) Exited() <-chan struct{} {
	return make(chan struct{})
}

func TestAgentScreenSwitchingView(t *testing.T) {
	t.Run("toggleKeyShowsAgentScreenWithItsCursorAndBack", toggleKeyShowsAgentScreenWithItsCursorAndBack)
	t.Run("configuredClaudeScreenKeyShowsAgentScreenAndAltXDoesNot", configuredClaudeScreenKeyShowsAgentScreenAndAltXDoesNot)
	t.Run("keysAndPastesGoToAgentOnlyWhileItsScreenShows", keysAndPastesGoToAgentOnlyWhileItsScreenShows)
	t.Run("windowSizeResizesAgent", windowSizeResizesAgent)
	t.Run("agentKeyFailureShowsOnZenStatusLine", agentKeyFailureShowsOnZenStatusLine)
	t.Run("agentPasteFailureShowsOnZenStatusLine", agentPasteFailureShowsOnZenStatusLine)
	t.Run("agentResizeFailureShowsOnZenStatusLine", agentResizeFailureShowsOnZenStatusLine)
	t.Run("agentScreenChangeWaitsForNextChange", agentScreenChangeWaitsForNextChange)
	t.Run("trustDialogShowsAgentScreenUntilItClosesAndToggleKeyStillWorks",
		trustDialogShowsAgentScreenUntilItClosesAndToggleKeyStillWorks)
	t.Run("agentExitQuits", agentExitQuits)
}

func newAgentScreenSwitchingView(agent *terminalAgentRecorder) tea.Model {
	return agentScreenSwitchingView{zen: newZenModel(agent, testAppearance, "alt+x", "ctrl+e", ""), agent: agent, showsAgentScreen: false}
}

func toggleKeyShowsAgentScreenWithItsCursorAndBack(t *testing.T) {
	agent := &terminalAgentRecorder{screen: "Claude's own screen"}
	view := newAgentScreenSwitchingView(agent)
	zenScreen := view.View().Content
	require.NotContains(t, zenScreen, agent.screen)
	view, _ = view.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModAlt})
	agentScreen := view.View()
	require.Equal(t, agent.screen, agentScreen.Content)
	require.True(t, agentScreen.AltScreen)
	require.NotNil(t, agentScreen.Cursor)
	require.Equal(t, 2, agentScreen.Cursor.Position.X)
	require.Equal(t, 1, agentScreen.Cursor.Position.Y)
	view, _ = view.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModAlt})
	zenScreenAgain := view.View().Content
	require.NotContains(t, zenScreenAgain, agent.screen)
	require.Empty(t, agent.calls)
}

func configuredClaudeScreenKeyShowsAgentScreenAndAltXDoesNot(t *testing.T) {
	agent := &terminalAgentRecorder{screen: "Claude's own screen"}
	var view tea.Model = agentScreenSwitchingView{zen: newZenModel(agent, testAppearance, "alt+z", "ctrl+e", ""), agent: agent,
		showsAgentScreen: false}
	view, _ = view.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModAlt})
	afterAltX := view.(agentScreenSwitchingView)
	require.False(t, afterAltX.showsAgentScreen)
	view, _ = view.Update(tea.KeyPressMsg{Code: 'z', Mod: tea.ModAlt})
	shown := view.View().Content
	require.Equal(t, agent.screen, shown)
}

func keysAndPastesGoToAgentOnlyWhileItsScreenShows(t *testing.T) {
	agent := &terminalAgentRecorder{screen: "Claude's own screen"}
	view := newAgentScreenSwitchingView(agent)
	for _, msg := range []tea.Msg{
		tea.KeyPressMsg{Code: 'a', Text: "a"},
		tea.KeyPressMsg{Code: 'x', Mod: tea.ModAlt},
		tea.KeyPressMsg{Code: tea.KeyEscape},
		tea.PasteMsg{Content: "pasted"},
		tea.KeyPressMsg{Code: 'x', Mod: tea.ModAlt},
		tea.KeyPressMsg{Code: 'b', Text: "b"},
	} {
		view, _ = view.Update(msg)
	}
	require.Equal(t, []string{"key esc", "paste pasted"}, agent.calls)
	typed := view.(agentScreenSwitchingView).zen.input.Value()
	require.Equal(t, "ab", typed)
}

func windowSizeResizesAgent(t *testing.T) {
	agent := &terminalAgentRecorder{screen: ""}
	view := newAgentScreenSwitchingView(agent)
	view.Update(tea.WindowSizeMsg{Width: 3, Height: 2})
	require.Equal(t, []string{"resize 3x2"}, agent.calls)
}

func assertAgentInputFailureShowsOnZenStatusLine(t *testing.T, failingInput tea.Msg) {
	t.Helper()
	agent := &terminalAgentRecorder{screen: "", inputErr: nil}
	view, _ := newAgentScreenSwitchingView(agent).Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	agent.inputErr = errors.New("Claude takes no more keys")
	view, _ = view.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModAlt})
	view, _ = view.Update(failingInput)
	view, _ = view.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModAlt})
	shown := view.View().Content
	require.Contains(t, shown, "Claude takes no more keys")
}

func agentKeyFailureShowsOnZenStatusLine(t *testing.T) {
	assertAgentInputFailureShowsOnZenStatusLine(t, tea.KeyPressMsg{Code: 'a', Text: "a"})
}

func agentPasteFailureShowsOnZenStatusLine(t *testing.T) {
	assertAgentInputFailureShowsOnZenStatusLine(t, tea.PasteMsg{Content: "pasted"})
}

func agentResizeFailureShowsOnZenStatusLine(t *testing.T) {
	assertAgentInputFailureShowsOnZenStatusLine(t, tea.WindowSizeMsg{Width: 80, Height: 24})
}

func agentScreenChangeWaitsForNextChange(t *testing.T) {
	agent := &terminalAgentRecorder{screen: "", screenChanged: make(chan struct{}, 1)}
	_, cmd := newAgentScreenSwitchingView(agent).Update(agentScreenChangedMsg{})
	require.NotNil(t, cmd)
	agent.screenChanged <- struct{}{}
	msg := cmd()
	require.IsType(t, agentScreenChangedMsg{}, msg)
}

func trustDialogShowsAgentScreenUntilItClosesAndToggleKeyStillWorks(t *testing.T) {
	trustDialog := " Accessing workspace:\n\n ❯ No, exit\n   Yes, I trust this folder"
	agent := &terminalAgentRecorder{screen: trustDialog}
	view, _ := newAgentScreenSwitchingView(agent).Update(agentScreenChangedMsg{})
	withDialog := view.View().Content
	require.Equal(t, trustDialog, withDialog)
	view, _ = view.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModAlt})
	view, _ = view.Update(agentScreenChangedMsg{})
	toggledAway := view.View().Content
	require.NotContains(t, toggledAway, "Accessing workspace:")
	view, _ = view.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModAlt})
	agent.screen = "Claude's own screen"
	view, _ = view.Update(agentScreenChangedMsg{})
	afterDialogClosed := view.View().Content
	require.NotContains(t, afterDialogClosed, agent.screen)
}

func agentExitQuits(t *testing.T) {
	_, cmd := newAgentScreenSwitchingView(&terminalAgentRecorder{screen: ""}).Update(agentExitedMsg{})
	require.NotNil(t, cmd)
	msg := cmd()
	require.IsType(t, tea.QuitMsg{}, msg)
}
