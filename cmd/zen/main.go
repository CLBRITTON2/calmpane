// Command zen shows the dialogue of the Claude session in a herdr pane: the user's prompts, Claude's reply text and
// its questions.
package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
)

const (
	// toggleKey is the key windows-dev's herdr/config.toml binds to the windows-dev.zen.toggle action.
	toggleKey     = "alt+x"
	claudePaneEnv = "ZEN_CLAUDE_PANE"
)

func main() {
	herdr := herdrCLI{binary: os.Getenv("HERDR_BIN_PATH"), pluginID: os.Getenv("HERDR_PLUGIN_ID")}
	if herdr.binary == "" || herdr.pluginID == "" {
		fmt.Fprintln(os.Stderr, "HERDR_BIN_PATH or HERDR_PLUGIN_ID is not set: zen runs from a herdr plugin")
		os.Exit(2)
	}
	var err error
	switch {
	case len(os.Args) == 1:
		err = RunZenView(herdr)
	case len(os.Args) == 2 && os.Args[1] == "toggle":
		err = withZenMode(herdr, ToggleZenMode)
	case len(os.Args) == 2 && os.Args[1] == "focus":
		err = withZenMode(herdr, ShowFocusedZenView)
	case len(os.Args) == 3 && os.Args[1] == "focus-agent":
		agentNumber, parseErr := strconv.Atoi(os.Args[2])
		if parseErr != nil {
			err = fmt.Errorf("focus-agent takes an agent number: %w", parseErr)
			break
		}
		err = withZenMode(herdr, func(herdr herdrCLI, mode zenMode) error {
			return FocusAgentOrItsZenPane(herdr, mode, agentNumber)
		})
	default:
		err = errors.New("usage: zen [toggle|focus|focus-agent <number>]")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func withZenMode(herdr herdrCLI, command func(herdrCLI, zenMode) error) error {
	mode, err := ZenModeOfPlugin(herdr.pluginID)
	if err != nil {
		return err
	}
	return command(herdr, mode)
}

func ZenModeOfPlugin(pluginID string) (zenMode, error) {
	stateDir := os.Getenv("HERDR_PLUGIN_STATE_DIR")
	if stateDir == "" {
		return zenMode{}, errors.New("HERDR_PLUGIN_STATE_DIR is not set: zen runs from a herdr plugin")
	}
	session := os.Getenv("HERDR_SESSION")
	if session == "" {
		// herdr gives the session started without --session no HERDR_SESSION
		session = "default"
	}
	return zenMode{stateDir: stateDir, session: session, mutexName: `Local\` + pluginID + "." + session}, nil
}
