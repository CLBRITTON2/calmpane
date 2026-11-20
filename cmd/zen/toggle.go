package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"golang.org/x/sys/windows"
)

const mutexTimeoutMs = 10000

// zenMode is the zen mode of the herdr session session: on while ModeFile exists, so the mode outlives a herdr server
// restart.
type zenMode struct {
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

func (m zenMode) ModeFile() string {
	return filepath.Join(m.stateDir, "zen-mode-on."+m.session)
}

func (m zenMode) IsOn() (bool, error) {
	_, err := os.Stat(m.ModeFile())
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("read zen mode file %s: %w", m.ModeFile(), err)
}

// ToggleZenMode is the alt+x action.
func ToggleZenMode(herdr herdrCLI, mode zenMode) error {
	return mode.WhileLocked(func() error {
		isOn, err := mode.IsOn()
		if err != nil {
			return err
		}
		panes, err := herdr.Panes()
		if err != nil {
			return err
		}
		if isOn {
			if err := os.Remove(mode.ModeFile()); err != nil {
				return fmt.Errorf("turn zen mode off: %w", err)
			}
			return CloseZenPanesMoveDraftsAndFocusAgent(herdr, mode, panes)
		}
		if err := os.WriteFile(mode.ModeFile(), []byte{}, 0o644); err != nil {
			return fmt.Errorf("turn zen mode on: %w", err)
		}
		if err := OpenOrShowZenPaneOfFocusedPane(herdr, mode, panes); err != nil {
			return errors.Join(err, os.Remove(mode.ModeFile()))
		}
		return nil
	})
}

// ShowFocusedZenView is the pane.focused and pane.agent_detected hook. herdr starts hooks without waiting for earlier
// runs, so it reads the focus that is current while it holds the mutex instead of the pane in its event.
func ShowFocusedZenView(herdr herdrCLI, mode zenMode) error {
	return mode.WhileLocked(func() error {
		isOn, err := mode.IsOn()
		if err != nil || !isOn {
			return err
		}
		panes, err := herdr.Panes()
		if err != nil {
			return err
		}
		return OpenOrShowZenPaneOfFocusedPane(herdr, mode, panes)
	})
}

func OpenOrShowZenPaneOfFocusedPane(herdr herdrCLI, mode zenMode, panes []herdrPane) error {
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
		if err := MoveAgentInputIntoDraft(herdr, claudePane, mode.DraftFile(action.ClaudePaneID)); err != nil {
			return err
		}
		return herdr.OpenAndMarkZenPane(action.ClaudePaneID)
	case zenPaneActionShow:
		return herdr.FocusAndZoomZenPane(action.ZenPaneID)
	}
	return nil
}

func ZenPaneActionForFocusedPane(panes []herdrPane) (zenPaneAction, error) {
	focusedPane, err := FocusedPane(panes)
	if err != nil {
		return zenPaneAction{}, err
	}
	if focusedPane.Agent != "claude" {
		return zenPaneAction{Kind: zenPaneActionNone}, nil
	}
	for _, pane := range panes {
		if pane.Tokens[claudePaneToken] == focusedPane.PaneID {
			return zenPaneAction{Kind: zenPaneActionShow, ClaudePaneID: focusedPane.PaneID, ZenPaneID: pane.PaneID}, nil
		}
	}
	return zenPaneAction{Kind: zenPaneActionOpen, ClaudePaneID: focusedPane.PaneID}, nil
}

// CloseZenPanesMoveDraftsAndFocusAgent also closes the zen panes a herdr server restart brought back as plain shells,
// which keep the zen pane's title but lost zen.exe and the claude_pane token.
func CloseZenPanesMoveDraftsAndFocusAgent(herdr herdrCLI, mode zenMode, panes []herdrPane) error {
	focusedPane, err := FocusedPane(panes)
	if err != nil {
		return err
	}
	for _, pane := range panes {
		claudePaneID, isZenPane := pane.Tokens[claudePaneToken]
		if !isZenPane && pane.Label != zenPaneTitle {
			continue
		}
		if err := herdr.ClosePane(pane.PaneID); err != nil {
			return err
		}
		if isZenPane {
			if err := MoveDraftIntoAgent(herdr, mode.DraftFile(claudePaneID), claudePaneID); err != nil {
				return err
			}
		}
	}
	if claudePaneID, isZenPane := focusedPane.Tokens[claudePaneToken]; isZenPane {
		return herdr.FocusAgent(claudePaneID)
	}
	return nil
}

func FocusedPane(panes []herdrPane) (herdrPane, error) {
	focusedPanes := []herdrPane{}
	for _, pane := range panes {
		if pane.Focused {
			focusedPanes = append(focusedPanes, pane)
		}
	}
	if len(focusedPanes) != 1 {
		return herdrPane{}, fmt.Errorf("herdr pane list has %d focused panes, want 1: %+v", len(focusedPanes), panes)
	}
	return focusedPanes[0], nil
}

// WhileLocked runs work holding a named mutex. A Windows mutex belongs to a thread, so the goroutine stays on one.
func (m zenMode) WhileLocked(work func() error) error {
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
		return errors.Join(fmt.Errorf("another zen.exe held mutex %s for over %d ms", m.mutexName, mutexTimeoutMs),
			windows.CloseHandle(mutex))
	}
	workErr := work()
	return errors.Join(workErr, windows.ReleaseMutex(mutex), windows.CloseHandle(mutex))
}
