// Command calmpane shows the dialogue of a Claude session, in a herdr pane or over the Claude `calmpane claude`
// starts: the user's prompts, Claude's reply text and its questions.
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/CLBRITTON2/calmpane/internal/config"
	"github.com/CLBRITTON2/calmpane/internal/herdr"
	"github.com/CLBRITTON2/calmpane/internal/ptyagent"
	"github.com/CLBRITTON2/calmpane/internal/ui"
	"github.com/CLBRITTON2/calmpane/internal/zenmode"
)

func main() {
	switch {
	case len(os.Args) >= 2 && os.Args[1] == "claude":
		settings, err := loadConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		historyFile, err := slashHistoryFile()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		exitCode, err := ptyagent.RunClaudeUnderZenView(settings, historyFile, os.Args[2:])
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
	api := herdr.API{SocketPath: os.Getenv("HERDR_SOCKET_PATH")}
	var err error
	switch {
	case len(os.Args) == 1:
		settings, configErr := loadConfig()
		claudePaneID := os.Getenv(herdr.ClaudePaneEnv)
		mode, modeErr := zenmode.ModeOfPlugin(cli.PluginID)
		draftFile := mode.DraftFile(claudePaneID)
		draft, draftErr := zenmode.SavedDraft(draftFile)
		saveDraft := func(draft ui.Draft) error { return zenmode.WriteDraft(draftFile, draft) }
		historyFile, historyErr := slashHistoryFile()
		err = ui.RunZenView(zenmode.HerdrAgent{CLI: cli, API: api, ClaudePaneID: claudePaneID}, settings, draft, draftErr,
			saveDraft, historyFile, errors.Join(configErr, modeErr, historyErr))
	case len(os.Args) == 2 && os.Args[1] == "toggle":
		err = withZenMode(cli, func(cli herdr.CLI, mode zenmode.Mode) error {
			return zenmode.ToggleAndNotifyFailure(cli, api, mode)
		})
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

func loadConfig() (config.Config, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return config.Config{}, fmt.Errorf("find the folder of calmpane's config: %w", err)
	}
	return config.Load(filepath.Join(configDir, "calmpane", "config.toml"))
}

// slashHistoryFile keeps how often and how lately zen sent each slash command, in %LOCALAPPDATA%\calmpane.
func slashHistoryFile() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("find the folder of calmpane's slash command history: %w", err)
	}
	return filepath.Join(cacheDir, "calmpane", "slash-history.json"), nil
}

func withZenMode(cli herdr.CLI, command func(herdr.CLI, zenmode.Mode) error) error {
	mode, err := zenmode.ModeOfPlugin(cli.PluginID)
	if err != nil {
		return err
	}
	return command(cli, mode)
}
