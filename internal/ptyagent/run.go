package ptyagent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/x/term"

	"github.com/CLBRITTON2/calmpane/internal/config"
	"github.com/CLBRITTON2/calmpane/internal/ui"
)

func RunClaudeUnderZenView(settings config.Config, slashHistoryFile string, claudeArguments []string) (int, error) {
	zenExecutable, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("find zen's executable for Claude's hooks: %w", err)
	}
	width, height, err := term.GetSize(os.Stdout.Fd())
	if err != nil {
		return 0, fmt.Errorf("calmpane claude runs in a terminal: %w", err)
	}
	hookDir, err := os.MkdirTemp("", "zen-claude-")
	if err != nil {
		return 0, err
	}
	stateFile := filepath.Join(hookDir, "state")
	settingsFile := filepath.Join(hookDir, "settings.json")
	if err := WriteHookSettings(settingsFile, zenExecutable, stateFile); err != nil {
		return 0, errors.Join(err, os.RemoveAll(hookDir))
	}
	if err := putZenInKillOnCloseJob(); err != nil {
		return 0, errors.Join(err, os.RemoveAll(hookDir))
	}
	parentExited, err := parentExit()
	if err != nil {
		return 0, errors.Join(err, os.RemoveAll(hookDir))
	}
	commandLine := append([]string{"claude", "--settings", settingsFile}, claudeArguments...)
	agent, err := Start(commandLine, stateFile, width, height)
	if err != nil {
		return 0, errors.Join(err, os.RemoveAll(hookDir))
	}
	endedOnParentExit := make(chan error, 1)
	go func() {
		select {
		case <-agent.Exited():
			endedOnParentExit <- nil
		case waitErr := <-parentExited:
			endedOnParentExit <- errors.Join(waitErr, agent.End())
		}
	}()
	viewErr := ui.RunZenViewOverTerminalAgent(agent, settings, slashHistoryFile)
	exitCode, closeErr := agent.Close()
	return exitCode, errors.Join(viewErr, closeErr, <-endedOnParentExit, os.RemoveAll(hookDir))
}
