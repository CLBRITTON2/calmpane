package zenmode

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"golang.org/x/sys/windows"

	"github.com/CLBRITTON2/calmpane/internal/herdr"
)

const mutexTimeoutMs = 10000

// Mode is the zen mode of the herdr session session: on while ModeFile exists, so the mode outlives a herdr server
// restart.
type Mode struct {
	stateDir  string
	session   string
	mutexName string
}

type zenPaneActionKind int

const (
	zenPaneActionNone zenPaneActionKind = iota
	zenPaneActionOpen
	zenPaneActionShow
)

type zenPaneAction struct {
	Kind         zenPaneActionKind
	ClaudePaneID string
	ZenPaneID    string
}

func (m Mode) ModeFile() string {
	return filepath.Join(m.stateDir, "zen-mode-on."+m.session)
}

func (m Mode) IsOn() (bool, error) {
	_, err := os.Stat(m.ModeFile())
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("read zen mode file %s: %w", m.ModeFile(), err)
}

// Toggle is the alt+x action.
func Toggle(cli herdr.CLI, mode Mode) error {
	return mode.WhileLocked(func() error {
		isOn, err := mode.IsOn()
		if err != nil {
			return err
		}
		panes, err := cli.Panes()
		if err != nil {
			return err
		}
		if isOn {
			if err := os.Remove(mode.ModeFile()); err != nil {
				return fmt.Errorf("turn zen mode off: %w", err)
			}
			return CloseZenPanesMoveDraftsAndFocusAgent(cli, mode, panes)
		}
		return TurnOnAndOpenOrShowZenPaneOfFocusedPane(cli, mode, panes)
	})
}

// TurnOnAndShowFocusedZenView is the action Win+A invokes before it attaches to herdr, so the client comes up in zen
// mode whichever mode the session was left in.
func TurnOnAndShowFocusedZenView(cli herdr.CLI, mode Mode) error {
	return mode.WhileLocked(func() error {
		isOn, err := mode.IsOn()
		if err != nil {
			return err
		}
		panes, err := cli.Panes()
		if err != nil {
			return err
		}
		if isOn {
			return OpenOrShowZenPaneOfFocusedPane(cli, mode, panes)
		}
		return TurnOnAndOpenOrShowZenPaneOfFocusedPane(cli, mode, panes)
	})
}

// TurnOnAndOpenOrShowZenPaneOfFocusedPane turns zen mode back off when the zen pane fails to open.
func TurnOnAndOpenOrShowZenPaneOfFocusedPane(cli herdr.CLI, mode Mode, panes []herdr.Pane) error {
	if err := os.WriteFile(mode.ModeFile(), []byte{}, 0o644); err != nil {
		return fmt.Errorf("turn zen mode on: %w", err)
	}
	if err := OpenOrShowZenPaneOfFocusedPane(cli, mode, panes); err != nil {
		return errors.Join(err, os.Remove(mode.ModeFile()))
	}
	return nil
}

// ShowFocusedZenView is the pane.focused and pane.agent_detected hook. herdr starts hooks without waiting for earlier
// runs, so it reads the focus that is current while it holds the mutex instead of the pane in its event.
func ShowFocusedZenView(cli herdr.CLI, mode Mode) error {
	return mode.WhileLocked(func() error {
		isOn, err := mode.IsOn()
		if err != nil || !isOn {
			return err
		}
		panes, err := cli.Panes()
		if err != nil {
			return err
		}
		return OpenOrShowZenPaneOfFocusedPane(cli, mode, panes)
	})
}

// FocusAgentOrItsZenPane is the alt+1..9 action, focusing the agentNumber-th agent of herdr's agent list. While zen
// mode is on it focuses the agent's zen pane straight away: focusing the agent first would show its pane until the
// focus hook switches to the zen pane.
func FocusAgentOrItsZenPane(cli herdr.CLI, mode Mode, agentNumber int) error {
	return mode.WhileLocked(func() error {
		agents, err := cli.Agents()
		if err != nil {
			return err
		}
		if agentNumber < 1 || agentNumber > len(agents) {
			return fmt.Errorf("agent %d does not exist: herdr agent list has %d agents", agentNumber, len(agents))
		}
		agentPaneID := agents[agentNumber-1].PaneID
		isOn, err := mode.IsOn()
		if err != nil {
			return err
		}
		if !isOn {
			return cli.FocusAgent(agentPaneID)
		}
		panes, err := cli.Panes()
		if err != nil {
			return err
		}
		for _, pane := range panes {
			if pane.Tokens[herdr.ClaudePaneToken] == agentPaneID {
				if err := cli.FocusTab(pane.TabID); err != nil {
					return err
				}
				return cli.FocusAndZoomZenPane(pane.PaneID)
			}
		}
		return cli.FocusAgent(agentPaneID)
	})
}

func OpenOrShowZenPaneOfFocusedPane(cli herdr.CLI, mode Mode, panes []herdr.Pane) error {
	action, err := ZenPaneActionForFocusedPane(panes)
	if err != nil {
		return err
	}
	switch action.Kind {
	case zenPaneActionOpen:
		claudePane, err := FocusedPane(panes)
		if err != nil {
			return err
		}
		if err := MoveAgentInputIntoDraft(cli, claudePane, mode.DraftFile(action.ClaudePaneID)); err != nil {
			return err
		}
		return cli.OpenAndMarkZenPane(action.ClaudePaneID)
	case zenPaneActionShow:
		return cli.FocusAndZoomZenPane(action.ZenPaneID)
	}
	return nil
}

func ZenPaneActionForFocusedPane(panes []herdr.Pane) (zenPaneAction, error) {
	focusedPane, err := FocusedPane(panes)
	if err != nil {
		return zenPaneAction{}, err
	}
	if focusedPane.Agent != "claude" {
		return zenPaneAction{Kind: zenPaneActionNone}, nil
	}
	for _, pane := range panes {
		if pane.Tokens[herdr.ClaudePaneToken] == focusedPane.PaneID {
			return zenPaneAction{Kind: zenPaneActionShow, ClaudePaneID: focusedPane.PaneID, ZenPaneID: pane.PaneID}, nil
		}
	}
	return zenPaneAction{Kind: zenPaneActionOpen, ClaudePaneID: focusedPane.PaneID}, nil
}

// CloseZenPanesMoveDraftsAndFocusAgent also closes the zen panes a herdr server restart brought back as plain shells,
// which keep the zen pane's title but lost calmpane.exe and the claude_pane token.
func CloseZenPanesMoveDraftsAndFocusAgent(cli herdr.CLI, mode Mode, panes []herdr.Pane) error {
	focusedPane, err := FocusedPane(panes)
	if err != nil {
		return err
	}
	for _, pane := range panes {
		claudePaneID, isZenPane := pane.Tokens[herdr.ClaudePaneToken]
		if !isZenPane && pane.Label != herdr.ZenPaneTitle {
			continue
		}
		if err := cli.ClosePane(pane.PaneID); err != nil {
			return err
		}
		if isZenPane {
			if err := MoveDraftIntoAgent(cli, mode.DraftFile(claudePaneID), claudePaneID); err != nil {
				return err
			}
		}
	}
	if claudePaneID, isZenPane := focusedPane.Tokens[herdr.ClaudePaneToken]; isZenPane {
		return cli.FocusAgent(claudePaneID)
	}
	return nil
}

func FocusedPane(panes []herdr.Pane) (herdr.Pane, error) {
	focusedPanes := []herdr.Pane{}
	for _, pane := range panes {
		if pane.Focused {
			focusedPanes = append(focusedPanes, pane)
		}
	}
	if len(focusedPanes) != 1 {
		return herdr.Pane{}, fmt.Errorf("herdr pane list has %d focused panes, want 1: %+v", len(focusedPanes), panes)
	}
	return focusedPanes[0], nil
}

// WhileLocked runs work holding a named mutex. A Windows mutex belongs to a thread, so the goroutine stays on one.
func (m Mode) WhileLocked(work func() error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	name, err := windows.UTF16PtrFromString(m.mutexName)
	if err != nil {
		return fmt.Errorf("mutex name %q: %w", m.mutexName, err)
	}
	mutex, err := windows.CreateMutex(nil, false, name)
	if mutex == 0 {
		return fmt.Errorf("create mutex %s: %w", m.mutexName, err)
	}
	waitResult, err := windows.WaitForSingleObject(mutex, mutexTimeoutMs)
	if err != nil {
		return errors.Join(fmt.Errorf("wait for mutex %s: %w", m.mutexName, err), windows.CloseHandle(mutex))
	}
	if waitResult != windows.WAIT_OBJECT_0 && waitResult != windows.WAIT_ABANDONED {
		return errors.Join(fmt.Errorf("another calmpane.exe held mutex %s for over %d ms", m.mutexName, mutexTimeoutMs),
			windows.CloseHandle(mutex))
	}
	workErr := work()
	return errors.Join(workErr, windows.ReleaseMutex(mutex), windows.CloseHandle(mutex))
}

func ModeOfPlugin(pluginID string) (Mode, error) {
	stateDir := os.Getenv("HERDR_PLUGIN_STATE_DIR")
	if stateDir == "" {
		return Mode{}, errors.New("HERDR_PLUGIN_STATE_DIR is not set: calmpane runs from a herdr plugin")
	}
	session := os.Getenv("HERDR_SESSION")
	if session == "" {
		// herdr gives the session started without --session no HERDR_SESSION
		session = "default"
	}
	return Mode{stateDir: stateDir, session: session, mutexName: `Local\` + pluginID + "." + session}, nil
}
