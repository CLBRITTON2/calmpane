package herdr

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// ClaudePaneEnv carries the Claude pane's id to the zen view a plugin pane opens.
const ClaudePaneEnv = "ZEN_CLAUDE_PANE"

// ClaudePaneToken marks a zen pane with the Claude pane it shows.
const ClaudePaneToken = "claude_pane"

// ZenPaneTitle is the title of the zen pane in windows-dev's herdr/zen/herdr-plugin.toml, which herdr shows as its
// label.
const ZenPaneTitle = "Zen"

type Pane struct {
	PaneID       string            `json:"pane_id"`
	TabID        string            `json:"tab_id"`
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

type errorReply struct {
	Error struct {
		Code string `json:"code"`
	} `json:"error"`
}

var ErrAgentBlocked = errors.New("agent_blocked")

type CLI struct {
	Binary   string
	PluginID string
}

// subcommand is split on spaces, so an argument with a space goes in arguments. A failure names only the subcommand,
// as the zen view's status line shows it: the arguments of agent prompt hold the whole prompt.
func (h CLI) run(subcommand string, arguments ...string) ([]byte, error) {
	var stdout, stderr bytes.Buffer
	command := exec.Command(h.Binary, append(strings.Fields(subcommand), arguments...)...)
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		var reply errorReply
		if json.Unmarshal(stderr.Bytes(), &reply) == nil && reply.Error.Code == ErrAgentBlocked.Error() {
			return nil, ErrAgentBlocked
		}
		return nil, fmt.Errorf("herdr %s failed: %w: %s", subcommand, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

func (h CLI) Pane(paneID string) (Pane, error) {
	output, err := h.run("pane get", paneID)
	if err != nil {
		return Pane{}, err
	}
	var reply struct {
		Result struct {
			Pane Pane `json:"pane"`
		} `json:"result"`
	}
	if err := json.Unmarshal(output, &reply); err != nil {
		return Pane{}, fmt.Errorf("decode herdr pane get %s reply %q: %w", paneID, output, err)
	}
	return reply.Result.Pane, nil
}

func (h CLI) Panes() ([]Pane, error) {
	output, err := h.run("pane list")
	if err != nil {
		return nil, err
	}
	var reply struct {
		Result struct {
			Panes []Pane `json:"panes"`
		} `json:"result"`
	}
	if err := json.Unmarshal(output, &reply); err != nil {
		return nil, fmt.Errorf("decode herdr pane list reply %q: %w", output, err)
	}
	return reply.Result.Panes, nil
}

// Agents is in herdr's agent list order, the order its focus_agent keys number the agents in.
func (h CLI) Agents() ([]Pane, error) {
	output, err := h.run("agent list")
	if err != nil {
		return nil, err
	}
	var reply struct {
		Result struct {
			Agents []Pane `json:"agents"`
		} `json:"result"`
	}
	if err := json.Unmarshal(output, &reply); err != nil {
		return nil, fmt.Errorf("decode herdr agent list reply %q: %w", output, err)
	}
	return reply.Result.Agents, nil
}

func (h CLI) PromptAgent(paneID string, prompt string) error {
	_, err := h.run("agent prompt", paneID, prompt)
	return err
}

func (h CLI) SendKeys(paneID string, keys []string) error {
	_, err := h.run("pane send-keys", append([]string{paneID}, keys...)...)
	return err
}

// OpenAndMarkZenPane marks the zen pane before it returns, so a focus hook waiting on the zen mode's mutex finds the
// mark and opens no second zen pane for claudePaneID.
func (h CLI) OpenAndMarkZenPane(claudePaneID string) error {
	output, err := h.run("plugin pane open", "--plugin", h.PluginID, "--entrypoint", "zen",
		"--placement", "zoomed", "--target-pane", claudePaneID, "--env", ClaudePaneEnv+"="+claudePaneID, "--focus")
	if err != nil {
		return err
	}
	var reply struct {
		Result struct {
			PluginPane struct {
				Pane Pane `json:"pane"`
			} `json:"plugin_pane"`
		} `json:"result"`
	}
	if err := json.Unmarshal(output, &reply); err != nil {
		return fmt.Errorf("decode herdr plugin pane open reply %q: %w", output, err)
	}
	_, err = h.run("pane report-metadata", reply.Result.PluginPane.Pane.PaneID, "--source", h.PluginID,
		"--token", ClaudePaneToken+"="+claudePaneID)
	return err
}

func (h CLI) FocusAndZoomZenPane(zenPaneID string) error {
	if _, err := h.run("plugin pane focus", zenPaneID); err != nil {
		return err
	}
	_, err := h.run("pane zoom", zenPaneID, "--on")
	return err
}

// FocusTab is how an API call moves the attached client to another tab: after plugin pane focus alone, herdr 0.9.5's
// client switches back to the tab it last showed on the next key press.
func (h CLI) FocusTab(tabID string) error {
	_, err := h.run("tab focus", tabID)
	return err
}

func (h CLI) ClosePane(paneID string) error {
	_, err := h.run("pane close", paneID)
	return err
}

func (h CLI) FocusAgent(paneID string) error {
	_, err := h.run("agent focus", paneID)
	return err
}

// StyledScreen keeps the SGR sequences, which tell a dimmed placeholder apart from typed text.
func (h CLI) StyledScreen(paneID string) (string, error) {
	output, err := h.run("pane read", paneID, "--source", "visible", "--format", "ansi")
	return string(output), err
}

func (h CLI) SendText(paneID string, text string) error {
	_, err := h.run("pane send-text", paneID, text)
	return err
}
