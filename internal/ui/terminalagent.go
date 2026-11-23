package ui

import (
	"errors"

	tea "charm.land/bubbletea/v2"

	"github.com/CLBRITTON2/calmpane/internal/config"
)

type agentScreenChangedMsg struct{}

type agentExitedMsg struct{}

// RunZenViewOverTerminalAgent shows the zen view of the agent, or its own screen while the Claude screen key switches
// to it, until the agent exits.
func RunZenViewOverTerminalAgent(agent TerminalAgent, settings config.Config, slashHistoryFile string) error {
	appearance, err := newAppearance(settings)
	if err != nil {
		return err
	}
	program := tea.NewProgram(agentScreenSwitchingView{zen: newZenModel(agent, appearance, settings.Keys.ClaudeScreen, slashHistoryFile),
		agent: agent, showsAgentScreen: false})
	go followAgentSession(agent, settings.StatusLine.Dir, program.Send)
	finalModel, runErr := program.Run()
	if finalModel, isSwitchingView := finalModel.(agentScreenSwitchingView); isSwitchingView {
		return errors.Join(finalModel.zen.fatal, runErr)
	}
	return runErr
}

// agentScreenSwitchingView sends every key and paste to the agent while it shows the agent's screen, the toggle key
// excepted.
type agentScreenSwitchingView struct {
	zen              zenModel
	agent            TerminalAgent
	showsAgentScreen bool
}

func (v agentScreenSwitchingView) Init() tea.Cmd {
	return tea.Batch(v.zen.Init(), waitForAgentScreenChange(v.agent), func() tea.Msg {
		<-v.agent.Exited()
		return agentExitedMsg{}
	})
}

func waitForAgentScreenChange(agent TerminalAgent) tea.Cmd {
	return func() tea.Msg {
		<-agent.ScreenChanged()
		return agentScreenChangedMsg{}
	}
}

func (v agentScreenSwitchingView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case agentScreenChangedMsg:
		return v, waitForAgentScreenChange(v.agent)
	case agentExitedMsg:
		return v, tea.Quit
	case tea.WindowSizeMsg:
		if err := v.agent.Resize(msg.Width, msg.Height); err != nil {
			v.zen.status = v.zen.appearance.errorStyle.Render(err.Error())
		}
	case tea.KeyPressMsg:
		if msg.String() == v.zen.claudeScreenKey {
			v.showsAgentScreen = !v.showsAgentScreen
			return v, nil
		}
		if v.showsAgentScreen {
			return v.withAgentInputFailure(v.agent.SendKey(msg)), nil
		}
	case tea.PasteMsg:
		if v.showsAgentScreen {
			return v.withAgentInputFailure(v.agent.SendPaste(msg.Content)), nil
		}
	}
	updated, cmd := v.zen.Update(msg)
	v.zen = updated.(zenModel)
	return v, cmd
}

// withAgentInputFailure shows a failed key or paste on the zen view's status line, seen once alt+x switches back.
func (v agentScreenSwitchingView) withAgentInputFailure(err error) agentScreenSwitchingView {
	if err != nil {
		v.zen.status = v.zen.appearance.errorStyle.Render(err.Error())
	}
	return v
}

func (v agentScreenSwitchingView) View() tea.View {
	if !v.showsAgentScreen {
		return v.zen.View()
	}
	screen, cursor := v.agent.Screen()
	view := tea.NewView(screen)
	view.Cursor = cursor
	view.AltScreen = true
	return view
}
