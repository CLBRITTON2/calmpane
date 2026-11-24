package ui

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
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
	t.Run("agentInputFailureShowsOnZenStatusLine", agentInputFailureShowsOnZenStatusLine)
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
	if strings.Contains(view.View().Content, agent.screen) {
		t.Fatal("zen view shows the agent's screen before alt+x")
	}
	view, _ = view.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModAlt})
	agentScreen := view.View()
	if agentScreen.Content != agent.screen || !agentScreen.AltScreen {
		t.Errorf("view after alt+x = %q, alt screen %v, want only the agent's screen on the alt screen",
			agentScreen.Content, agentScreen.AltScreen)
	}
	if agentScreen.Cursor == nil || agentScreen.Cursor.Position.X != 2 || agentScreen.Cursor.Position.Y != 1 {
		t.Errorf("cursor after alt+x = %+v, want the agent's at 2,1", agentScreen.Cursor)
	}
	view, _ = view.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModAlt})
	if strings.Contains(view.View().Content, agent.screen) {
		t.Error("zen view shows the agent's screen after a second alt+x")
	}
	if len(agent.calls) != 0 {
		t.Errorf("agent calls = %q, want alt+x sent to no agent", agent.calls)
	}
}

func configuredClaudeScreenKeyShowsAgentScreenAndAltXDoesNot(t *testing.T) {
	agent := &terminalAgentRecorder{screen: "Claude's own screen"}
	var view tea.Model = agentScreenSwitchingView{zen: newZenModel(agent, testAppearance, "alt+z", "ctrl+e", ""), agent: agent,
		showsAgentScreen: false}
	view, _ = view.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModAlt})
	if view.(agentScreenSwitchingView).showsAgentScreen {
		t.Error("alt+x shows the agent's screen with alt+z as the Claude screen key")
	}
	view, _ = view.Update(tea.KeyPressMsg{Code: 'z', Mod: tea.ModAlt})
	if view.View().Content != agent.screen {
		t.Errorf("view after alt+z = %q, want the agent's screen", view.View().Content)
	}
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
	if want := []string{"key esc", "paste pasted"}; !reflect.DeepEqual(agent.calls, want) {
		t.Errorf("agent calls = %q, want %q", agent.calls, want)
	}
	if typed := view.(agentScreenSwitchingView).zen.input.Value(); typed != "ab" {
		t.Errorf("zen input = %q, want %q", typed, "ab")
	}
}

func windowSizeResizesAgent(t *testing.T) {
	agent := &terminalAgentRecorder{screen: ""}
	view := newAgentScreenSwitchingView(agent)
	view.Update(tea.WindowSizeMsg{Width: 3, Height: 2})
	if want := []string{"resize 3x2"}; !reflect.DeepEqual(agent.calls, want) {
		t.Errorf("agent calls = %q, want %q", agent.calls, want)
	}
}

func agentInputFailureShowsOnZenStatusLine(t *testing.T) {
	for _, msg := range []tea.Msg{tea.KeyPressMsg{Code: 'a', Text: "a"}, tea.PasteMsg{Content: "pasted"},
		tea.WindowSizeMsg{Width: 80, Height: 24}} {
		agent := &terminalAgentRecorder{screen: "", inputErr: nil}
		view, _ := newAgentScreenSwitchingView(agent).Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		agent.inputErr = errors.New("Claude takes no more keys")
		view, _ = view.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModAlt})
		view, _ = view.Update(msg)
		view, _ = view.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModAlt})
		if !strings.Contains(view.View().Content, "Claude takes no more keys") {
			t.Errorf("zen view after a failed %T does not show the failure", msg)
		}
	}
}

func agentScreenChangeWaitsForNextChange(t *testing.T) {
	agent := &terminalAgentRecorder{screen: "", screenChanged: make(chan struct{}, 1)}
	_, cmd := newAgentScreenSwitchingView(agent).Update(agentScreenChangedMsg{})
	if cmd == nil {
		t.Fatal("Update returned no command after the agent's screen changed")
	}
	agent.screenChanged <- struct{}{}
	if _, changed := cmd().(agentScreenChangedMsg); !changed {
		t.Error("Update after the agent's screen changed does not wait for its next change")
	}
}

func trustDialogShowsAgentScreenUntilItClosesAndToggleKeyStillWorks(t *testing.T) {
	trustDialog := " Accessing workspace:\n\n ❯ No, exit\n   Yes, I trust this folder"
	agent := &terminalAgentRecorder{screen: trustDialog}
	view, _ := newAgentScreenSwitchingView(agent).Update(agentScreenChangedMsg{})
	if view.View().Content != trustDialog {
		t.Fatalf("view with the trust dialog on the agent's screen = %q, want the agent's screen", view.View().Content)
	}
	view, _ = view.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModAlt})
	view, _ = view.Update(agentScreenChangedMsg{})
	if strings.Contains(view.View().Content, "Accessing workspace:") {
		t.Error("view after alt+x and a change with the trust dialog still open shows the agent's screen")
	}
	view, _ = view.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModAlt})
	agent.screen = "Claude's own screen"
	view, _ = view.Update(agentScreenChangedMsg{})
	if strings.Contains(view.View().Content, agent.screen) {
		t.Error("view after the trust dialog closed shows the agent's screen")
	}
}

func agentExitQuits(t *testing.T) {
	_, cmd := newAgentScreenSwitchingView(&terminalAgentRecorder{screen: ""}).Update(agentExitedMsg{})
	if cmd == nil {
		t.Fatal("Update returned no command after the agent exited")
	}
	if _, quits := cmd().(tea.QuitMsg); !quits {
		t.Error("Update after the agent exited does not quit")
	}
}
