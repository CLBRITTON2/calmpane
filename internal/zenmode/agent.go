package zenmode

import (
	"errors"
	"fmt"
	"io/fs"
	"time"

	"github.com/CLBRITTON2/calmpane/claudecode"
	"github.com/CLBRITTON2/calmpane/internal/herdr"
	"github.com/CLBRITTON2/calmpane/internal/ui"
)

// HerdrAgent is the Claude in herdr pane ClaudePaneID.
type HerdrAgent struct {
	CLI          herdr.CLI
	API          herdr.API
	ClaudePaneID string
}

// SendPrompt returns once herdr has written the prompt and its Enter, so keys sent after it reach Claude after them.
func (a HerdrAgent) SendPrompt(prompt string) error {
	err := a.API.PromptAgent(a.ClaudePaneID, prompt)
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

func (a HerdrAgent) InputBoxShown() (bool, error) {
	screen, err := a.CLI.StyledScreen(a.ClaudePaneID)
	if err != nil {
		return false, err
	}
	return claudecode.InputBoxShown(screen), nil
}

// WorkingDirectory is the Claude pane's, which herdr reads from the process running in it.
func (a HerdrAgent) WorkingDirectory() (string, error) {
	claudePane, err := a.CLI.Pane(a.ClaudePaneID)
	if err != nil {
		return "", err
	}
	if claudePane.Cwd == "" {
		return "", fmt.Errorf("herdr reports no working directory for pane %s", a.ClaudePaneID)
	}
	return claudePane.Cwd, nil
}

// herdr keeps reporting the agent across /clear and /resume, so only an exit reads as AgentGone.
func (a HerdrAgent) Status() (ui.AgentStatus, error) {
	claudePane, err := a.CLI.Pane(a.ClaudePaneID)
	if err != nil {
		return ui.AgentUnknown, err
	}
	if claudePane.Agent != "claude" {
		return ui.AgentGone, nil
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

// sessionsFileMissingLimit is how long a Claude past its trust dialog may have no sessions file. /exit deletes the file
// about 0.5 s before herdr drops the agent.
const sessionsFileMissingLimit = 5 * time.Second

// WaitForSessionID reads the session from the sessions file of the pane's own claude.exe, not from herdr's
// agent_session, which a child Claude's hook report can move. It waits while Claude shows its trust dialog, which comes
// before the file, and fails with ui.ErrAgentGone once herdr reports no agent in the pane.
func (a HerdrAgent) WaitForSessionID() (string, error) {
	if a.ClaudePaneID == "" {
		return "", fmt.Errorf("%s is not set: the toggle action opens the zen pane", herdr.ClaudePaneEnv)
	}
	sessionsFileMissingSince := time.Time{}
	for {
		status, err := a.Status()
		if err != nil {
			return "", err
		}
		if status == ui.AgentGone {
			return "", ui.ErrAgentGone
		}
		processes, err := a.CLI.ForegroundProcesses(a.ClaudePaneID)
		if err != nil {
			return "", err
		}
		claudeProcessIDs := []int{}
		for _, process := range processes {
			if process.Name == "claude.exe" {
				claudeProcessIDs = append(claudeProcessIDs, process.PID)
			}
		}
		if len(claudeProcessIDs) != 1 {
			return "", fmt.Errorf("herdr reports Claude in pane %s, whose foreground has %d claude.exe processes %v, not one",
				a.ClaudePaneID, len(claudeProcessIDs), claudeProcessIDs)
		}
		sessionID, sessionErr := claudecode.ProcessSessionID(claudeProcessIDs[0])
		if sessionErr == nil {
			return sessionID, nil
		}
		if !errors.Is(sessionErr, fs.ErrNotExist) {
			return "", fmt.Errorf("Claude in pane %s: %w", a.ClaudePaneID, sessionErr)
		}
		screen, err := a.CLI.StyledScreen(a.ClaudePaneID)
		if err != nil {
			return "", err
		}
		if claudecode.TrustDialogShown(screen) {
			sessionsFileMissingSince = time.Time{}
		} else if sessionsFileMissingSince.IsZero() {
			sessionsFileMissingSince = time.Now()
		} else if time.Since(sessionsFileMissingSince) >= sessionsFileMissingLimit {
			return "", fmt.Errorf("Claude in pane %s has had no sessions file for %s: %w", a.ClaudePaneID,
				sessionsFileMissingLimit, sessionErr)
		}
		time.Sleep(300 * time.Millisecond)
	}
}
