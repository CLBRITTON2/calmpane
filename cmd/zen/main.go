// Command zen shows the dialogue of the Claude session in a herdr pane: the user's prompts, Claude's reply text and
// its questions.
package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/CLBRITTON2/herdr-zen/internal/herdr"
	"github.com/CLBRITTON2/herdr-zen/internal/ui"
	"github.com/CLBRITTON2/herdr-zen/internal/zenmode"
)

func main() {
	cli := herdr.CLI{Binary: os.Getenv("HERDR_BIN_PATH"), PluginID: os.Getenv("HERDR_PLUGIN_ID")}
	if cli.Binary == "" || cli.PluginID == "" {
		fmt.Fprintln(os.Stderr, "HERDR_BIN_PATH or HERDR_PLUGIN_ID is not set: zen runs from a herdr plugin")
		os.Exit(2)
	}
	var err error
	switch {
	case len(os.Args) == 1:
		err = ui.RunZenView(cli)
	case len(os.Args) == 2 && os.Args[1] == "toggle":
		err = withZenMode(cli, zenmode.Toggle)
	case len(os.Args) == 2 && os.Args[1] == "on":
		err = withZenMode(cli, zenmode.TurnOnAndShowFocusedZenView)
	case len(os.Args) == 2 && os.Args[1] == "focus":
		err = withZenMode(cli, zenmode.ShowFocusedZenView)
	case len(os.Args) == 3 && os.Args[1] == "focus-agent":
		agentNumber, parseErr := strconv.Atoi(os.Args[2])
		if parseErr != nil {
			err = fmt.Errorf("focus-agent takes an agent number: %w", parseErr)
			break
		}
		err = withZenMode(cli, func(cli herdr.CLI, mode zenmode.Mode) error {
			return zenmode.FocusAgentOrItsZenPane(cli, mode, agentNumber)
		})
	default:
		err = errors.New("usage: zen [toggle|on|focus|focus-agent <number>]")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func withZenMode(cli herdr.CLI, command func(herdr.CLI, zenmode.Mode) error) error {
	mode, err := zenmode.ModeOfPlugin(cli.PluginID)
	if err != nil {
		return err
	}
	return command(cli, mode)
}
