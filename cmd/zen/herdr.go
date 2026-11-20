package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// claudePaneToken marks a zen pane with the Claude pane it shows.
const claudePaneToken = "claude_pane"

// zenPaneTitle is the title of the zen pane in windows-dev's herdr/zen/herdr-plugin.toml, which herdr shows as its
// label.
const zenPaneTitle = "Zen"

type herdrPane struct {
	PaneID       string            `json:"pane_id"`
	Agent        string            `json:"agent"`
	Label        string            `json:"label"`
	Focused      bool              `json:"focused"`
	AgentStatus  string            `json:"agent_status"`
	Tokens       map[string]string `json:"tokens"`
	AgentSession *struct {
		Agent string `json:"agent"`
		Value string `json:"value"`
	} `json:"agent_session"`
}

type herdrError struct {
	Error struct {
		Code string `json:"code"`
	} `json:"error"`
}

var errAgentBlocked = errors.New("agent_blocked")

type herdrCLI struct {
	binary   string
	pluginID string
}

// subcommand is split on spaces, so an argument with a space goes in arguments. A failure names only the subcommand,
// as the zen view's status line shows it: the arguments of agent prompt hold the whole prompt.
func (h herdrCLI) run(subcommand string, arguments ...string) ([]byte, error) {
	var stdout, stderr bytes.Buffer
	command := exec.Command(h.binary, append(strings.Fields(subcommand), arguments...)...)
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		var reply herdrError
		if json.Unmarshal(stderr.Bytes(), &reply) == nil && reply.Error.Code == errAgentBlocked.Error() {
			return nil, errAgentBlocked
		}
		return nil, fmt.Errorf("herdr %s failed: %w: %s", subcommand, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

func (h herdrCLI) Pane(paneID string) (herdrPane, error) {
	output, err := h.run("pane get", paneID)
	if err != nil {
		return herdrPane{}, err
	}
	var reply struct {
		Result struct {
			Pane herdrPane `json:"pane"`
		} `json:"result"`
	}
	if err := json.Unmarshal(output, &reply); err != nil {
		return herdrPane{}, fmt.Errorf("decode herdr pane get %s reply %q: %w", paneID, output, err)
	}
	return reply.Result.Pane, nil
}

func (h herdrCLI) Panes() ([]herdrPane, error) {
	output, err := h.run("pane list")
	if err != nil {
		return nil, err
	}
	var reply struct {
		Result struct {
			Panes []herdrPane `json:"panes"`
		} `json:"result"`
	}
	if err := json.Unmarshal(output, &reply); err != nil {
		return nil, fmt.Errorf("decode herdr pane list reply %q: %w", output, err)
	}
	return reply.Result.Panes, nil
}

func (h herdrCLI) PromptAgent(paneID string, prompt string) error {
	_, err := h.run("agent prompt", paneID, prompt)
	return err
}

// OpenAndMarkZenPane marks the zen pane before it returns, so a focus hook waiting on the zen mode's mutex finds the
// mark and opens no second zen pane for claudePaneID.
func (h herdrCLI) OpenAndMarkZenPane(claudePaneID string) error {
	output, err := h.run("plugin pane open", "--plugin", h.pluginID, "--entrypoint", "zen",
		"--placement", "zoomed", "--target-pane", claudePaneID, "--env", claudePaneEnv+"="+claudePaneID, "--focus")
	if err != nil {
		return err
	}
	var reply struct {
		Result struct {
			PluginPane struct {
				Pane herdrPane `json:"pane"`
			} `json:"plugin_pane"`
		} `json:"result"`
	}
	if err := json.Unmarshal(output, &reply); err != nil {
		return fmt.Errorf("decode herdr plugin pane open reply %q: %w", output, err)
	}
	_, err = h.run("pane report-metadata", reply.Result.PluginPane.Pane.PaneID, "--source", h.pluginID,
		"--token", claudePaneToken+"="+claudePaneID)
	return err
}

func (h herdrCLI) FocusAndZoomZenPane(zenPaneID string) error {
	if _, err := h.run("plugin pane focus", zenPaneID); err != nil {
		return err
	}
	_, err := h.run("pane zoom", zenPaneID, "--on")
	return err
}

func (h herdrCLI) ClosePane(paneID string) error {
	_, err := h.run("pane close", paneID)
	return err
}

func (h herdrCLI) FocusAgent(paneID string) error {
	_, err := h.run("agent focus", paneID)
	return err
}

func (h herdrCLI) SendText(paneID string, text string) error {
	_, err := h.run("pane send-text", paneID, text)
	return err
}
