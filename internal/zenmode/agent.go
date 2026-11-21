package zenmode

import (
	"errors"
	"fmt"
	"time"

	"github.com/CLBRITTON2/calmpane/internal/herdr"
	"github.com/CLBRITTON2/calmpane/internal/ui"
)

// HerdrAgent is the Claude in herdr pane ClaudePaneID.
type HerdrAgent struct {
	CLI          herdr.CLI
	ClaudePaneID string
}

// SendPrompt returns once herdr has written the prompt and its Enter, so keys sent after it reach Claude after them.
func (a HerdrAgent) SendPrompt(prompt string) error {
	err := a.CLI.PromptAgent(a.ClaudePaneID, prompt)
	if errors.Is(err, herdr.ErrAgentBlocked) {
		return ui.ErrAgentBlocked
	}
	return err
}

// SendNow sends Claude Code's default chat:sendNow keys. Not Ctrl+Enter, which a terminal without extended keys sends
// as Enter.
func (a HerdrAgent) SendNow() error {
	return a.CLI.SendKeys(a.ClaudePaneID, []string{"ctrl+x", "ctrl+s"})
}

func (a HerdrAgent) Interrupt() error {
	return a.CLI.SendKeys(a.ClaudePaneID, []string{"esc"})
}

func (a HerdrAgent) TakeInput() (string, error) {
	return TakeAgentInput(a.CLI, a.ClaudePaneID)
}

func (a HerdrAgent) Status() (ui.AgentStatus, error) {
	claudePane, err := a.CLI.Pane(a.ClaudePaneID)
	if err != nil {
		return ui.AgentUnknown, err
	}
	switch claudePane.AgentStatus {
	case "working":
		return ui.AgentWorking, nil
	case "idle":
		return ui.AgentIdle, nil
	case "done":
		return ui.AgentDone, nil
	case "blocked":
		return ui.AgentBlocked, nil
	}
	return ui.AgentUnknown, nil
}

// WaitForSessionID waits for a Claude that just started, whose session herdr reports once it is past its startup
// prompts.
func (a HerdrAgent) WaitForSessionID() (string, error) {
	if a.ClaudePaneID == "" {
		return "", fmt.Errorf("%s is not set: the toggle action opens the zen pane", herdr.ClaudePaneEnv)
	}
	for {
		claudePane, err := a.CLI.Pane(a.ClaudePaneID)
		if err != nil {
			return "", err
		}
		if claudePane.AgentSession == nil && claudePane.Agent == "claude" {
			time.Sleep(300 * time.Millisecond)
			continue
		}
		if claudePane.AgentSession == nil || claudePane.AgentSession.Agent != "claude" {
			return "", errors.New("No Claude session in this pane.")
		}
		return claudePane.AgentSession.Value, nil
	}
}
