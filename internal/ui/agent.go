package ui

import (
	"errors"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/CLBRITTON2/herdr-zen/claudecode"
)

// Agent is the coding agent whose dialogue the zen view shows and whose input it sends.
type Agent interface {
	// SendPrompt returns ErrAgentBlocked when the agent refuses the prompt while it waits for an answer.
	SendPrompt(prompt string) error
	// SendNow makes the agent take a prompt it queued while working at once, a prompt SendPrompt just sent included.
	SendNow() error
	Interrupt() error
	// TakeInput empties the agent's own input box and returns the text it held.
	TakeInput() (string, error)
	Status() (AgentStatus, error)
	// WaitForSessionID waits until the agent reports the session whose transcript holds its dialogue.
	WaitForSessionID() (string, error)
}

type AgentStatus int

const (
	AgentUnknown AgentStatus = iota
	AgentWorking
	AgentIdle
	AgentDone
	AgentBlocked
)

var ErrAgentBlocked = errors.New("the agent waits for an answer and takes no prompt")

// TerminalAgent is an Agent running in a terminal zen owns, so zen can show the agent's own screen in place of the
// zen view and type into it.
type TerminalAgent interface {
	Agent
	// Screen is the agent's screen with its SGR sequences, and its cursor, nil while the agent hides it.
	Screen() (string, *tea.Cursor)
	SendKey(key tea.KeyPressMsg) error
	SendPaste(text string) error
	Resize(width int, height int) error
	// ScreenChanged receives after the agent writes to its screen.
	ScreenChanged() <-chan struct{}
	// Exited is closed once the agent has exited.
	Exited() <-chan struct{}
}

// TakeClaudeInput empties Claude Code's input box, read from styledScreen, and returns the text it held. inputName
// names the box in the error of a box that still holds text after the keys.
func TakeClaudeInput(inputName string, styledScreen func() (string, error), sendCtrlU func(count int) error) (string,
	error) {
	screen, err := styledScreen()
	if err != nil {
		return "", err
	}
	box := claudecode.InputBoxOnScreen(screen)
	if box.Draft == "" {
		return "", nil
	}
	// Claude Code 2.1.329: ctrl+u deletes back to the start of the cursor's row or the line break before it, so twice
	// the rows empties the box. ctrl+c would interrupt a working turn, and ctrl+l only redraws.
	if err := sendCtrlU(2 * box.Rows); err != nil {
		return "", err
	}
	// Claude takes the keys after they are written
	deadline := time.Now().Add(2 * time.Second)
	for {
		screen, err := styledScreen()
		if err != nil {
			return "", err
		}
		if claudecode.InputBoxOnScreen(screen).Draft == "" {
			return box.Draft, nil
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("%s still holds text after ctrl+u, so zen took none of it: ctrl+y in Claude pastes "+
				"back what ctrl+u deleted", inputName)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
