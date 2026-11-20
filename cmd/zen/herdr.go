package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// claudePaneToken marks a zen pane with the Claude pane it shows, so the toggle action knows to close it.
const claudePaneToken = "claude_pane"

const pluginID = "windows-dev.zen"

type herdrPane struct {
	PaneID       string            `json:"pane_id"`
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
	binary string
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

func (h herdrCLI) MarkZenPane(zenPaneID string, claudePaneID string) error {
	_, err := h.run("pane report-metadata", zenPaneID, "--source", pluginID,
		"--token", claudePaneToken+"="+claudePaneID)
	return err
}

func (h herdrCLI) PromptAgent(paneID string, prompt string) error {
	_, err := h.run("agent prompt", paneID, prompt)
	return err
}

func (h herdrCLI) OpenZenPaneZoomed(claudePaneID string) error {
	_, err := h.run("plugin pane open", "--plugin", pluginID, "--entrypoint", "zen",
		"--placement", "zoomed", "--target-pane", claudePaneID, "--env", claudePaneEnv+"="+claudePaneID, "--focus")
	return err
}

func (h herdrCLI) CloseZenPaneAndFocusAgent(zenPaneID string, claudePaneID string) error {
	if _, err := h.run("plugin pane close", zenPaneID); err != nil {
		return err
	}
	_, err := h.run("agent focus", claudePaneID)
	return err
}
