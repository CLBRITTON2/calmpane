package ptyagent

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/CLBRITTON2/calmpane/internal/ui"
)

// hookState is what the last hook Claude ran wrote to the state file.
type hookState struct {
	status    ui.AgentStatus
	sessionID string
}

// hookInput holds the fields zen reads from the JSON Claude Code writes to a hook's stdin.
type hookInput struct {
	SessionID     string `json:"session_id"`
	HookEventName string `json:"hook_event_name"`
}

// statusOfHookEvent is the status each hook event in the settings Claude starts with puts Claude in.
var statusOfHookEvent = map[string]ui.AgentStatus{
	"SessionStart":     ui.AgentIdle,
	"UserPromptSubmit": ui.AgentWorking,
	"PostToolUse":      ui.AgentWorking,
	"Stop":             ui.AgentDone,
	"Notification":     ui.AgentBlocked,
}

var statusNames = map[ui.AgentStatus]string{
	ui.AgentIdle:    "idle",
	ui.AgentWorking: "working",
	ui.AgentDone:    "done",
	ui.AgentBlocked: "blocked",
}

// WriteHookState writes the session and status of the hook event Claude Code describes on hookStdin to stateFile, as
// one line ending in LF. It writes the file in place, since Windows refuses to rename over a file zen holds open.
func WriteHookState(stateFile string, hookStdin io.Reader) error {
	var input hookInput
	if err := json.NewDecoder(hookStdin).Decode(&input); err != nil {
		return fmt.Errorf("read the hook input Claude Code wrote: %w", err)
	}
	status, hooked := statusOfHookEvent[input.HookEventName]
	if !hooked {
		return fmt.Errorf("calmpane hook does not handle %q", input.HookEventName)
	}
	if input.SessionID == "" {
		return fmt.Errorf("Claude Code ran the %s hook with no session_id", input.HookEventName)
	}
	return writeHookStateFile(stateFile, hookState{status: status, sessionID: input.SessionID})
}

// writeHookStateFile writes state as the one line ending in LF that parseHookState reads.
func writeHookStateFile(stateFile string, state hookState) error {
	return os.WriteFile(stateFile, []byte(statusNames[state.status]+" "+state.sessionID+"\n"), 0o600)
}

// parseHookState reports whole as false while a hook is partway through rewriting the state file.
func parseHookState(stateFileContent string) (state hookState, whole bool) {
	line, endsInLF := strings.CutSuffix(stateFileContent, "\n")
	statusName, sessionID, hasSessionID := strings.Cut(line, " ")
	if !endsInLF || !hasSessionID {
		return hookState{}, false
	}
	for status, savedName := range statusNames {
		if savedName == statusName {
			return hookState{status: status, sessionID: sessionID}, true
		}
	}
	return hookState{}, false
}

// hookCommand is a command Claude Code's hooks run in Git Bash on Windows, so its paths take forward slashes.
type hookCommand struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

type hookMatcher struct {
	Matcher string        `json:"matcher"`
	Hooks   []hookCommand `json:"hooks"`
}

type hookSettings struct {
	Hooks map[string][]hookMatcher `json:"hooks"`
}

// WriteHookSettings writes to settingsFile the Claude Code settings that run zenExecutable's hook subcommand with
// stateFile. Notification runs it only for a permission prompt or a question, not for the reminder Claude shows
// after a minute idle.
func WriteHookSettings(settingsFile string, zenExecutable string, stateFile string) error {
	hook := []hookCommand{{Type: "command",
		Command: fmt.Sprintf("%q hook %q", filepath.ToSlash(zenExecutable), filepath.ToSlash(stateFile))}}
	settings := hookSettings{Hooks: map[string][]hookMatcher{}}
	for hookEvent := range statusOfHookEvent {
		settings.Hooks[hookEvent] = []hookMatcher{{Matcher: "", Hooks: hook}}
	}
	settings.Hooks["Notification"] = []hookMatcher{{Matcher: "permission_prompt|elicitation_dialog", Hooks: hook}}
	content, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(settingsFile, content, 0o600); err != nil {
		return fmt.Errorf("write Claude's hook settings: %w", err)
	}
	return nil
}
