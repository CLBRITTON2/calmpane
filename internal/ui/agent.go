package ui

import "errors"

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
