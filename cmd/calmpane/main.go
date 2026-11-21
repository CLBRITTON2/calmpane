// Command calmpane shows the dialogue of a Claude session, in a herdr pane or over the Claude `calmpane claude`
// starts: the user's prompts, Claude's reply text and its questions.
package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/CLBRITTON2/calmpane/internal/herdr"
	"github.com/CLBRITTON2/calmpane/internal/ptyagent"
	"github.com/CLBRITTON2/calmpane/internal/ui"
	"github.com/CLBRITTON2/calmpane/internal/zenmode"
)

func main() {
	switch {
	case len(os.Args) >= 2 && os.Args[1] == "claude":
		exitCode, err := ptyagent.RunClaudeUnderZenView(os.Args[2:])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		if err != nil && exitCode == 0 {
			os.Exit(1)
		}
		os.Exit(exitCode)
	case len(os.Args) == 3 && os.Args[1] == "hook":
		if err := ptyagent.WriteHookState(os.Args[2], os.Stdin); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	cli := herdr.CLI{Binary: os.Getenv("HERDR_BIN_PATH"), PluginID: os.Getenv("HERDR_PLUGIN_ID")}
	if cli.Binary == "" || cli.PluginID == "" {
		fmt.Fprintln(os.Stderr, "HERDR_BIN_PATH or HERDR_PLUGIN_ID is not set: calmpane runs from a herdr plugin")
		os.Exit(2)
	}
	var err error
	switch {
	case len(os.Args) == 1:
		claudePaneID := os.Getenv(herdr.ClaudePaneEnv)
		mode, modeErr := zenmode.ModeOfPlugin(cli.PluginID)
		draftFile := mode.DraftFile(claudePaneID)
		draft, draftErr := zenmode.SavedDraft(draftFile)
		saveDraft := func(draft string) error { return zenmode.WriteWhole(draftFile, draft) }
		err = ui.RunZenView(zenmode.HerdrAgent{CLI: cli, ClaudePaneID: claudePaneID}, draft, saveDraft,
			errors.Join(modeErr, draftErr))
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
		err = errors.New("usage: calmpane [toggle|on|focus|focus-agent <number>|claude [claude arguments...]|hook <state file>]")
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
