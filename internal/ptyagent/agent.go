// Package ptyagent runs Claude Code in a pseudo-console zen owns, so the zen view runs in any terminal.
package ptyagent

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/conpty"
	"github.com/charmbracelet/x/vt"
	"golang.org/x/sys/windows"

	"github.com/CLBRITTON2/calmpane/internal/ui"
)

// Agent is a Claude Code started in a pseudo-console, whose screen a terminal emulator keeps and whose session and
// status the hooks in its settings write to stateFile.
type Agent struct {
	pseudoConsole *conpty.ConPty
	emulator      *vt.SafeEmulator
	stateFile     string
	screenChanged chan struct{}
	exited        chan struct{}
	// exitCode and exitErr, every error of Claude's run up to its exit, are set before exited is closed.
	exitCode uint32
	exitErr  error
	// closing is set before the pseudo-console closes, which fails a read of Claude's screen.
	closing atomic.Bool
	// forwardErr is set once the emulator's replies and keys can no longer reach Claude.
	forwardMutex sync.Mutex
	forwardErr   error
	// lastState is the state file's last whole content, kept while the hook rewrites the file.
	stateMutex sync.Mutex
	lastState  hookState
}

func Start(commandLine []string, stateFile string, width int, height int) (*Agent, error) {
	pseudoConsole, err := conpty.New(width, height, 0)
	if err != nil {
		return nil, err
	}
	_, processHandle, err := pseudoConsole.Spawn(commandLine[0], commandLine, &syscall.ProcAttr{})
	if err != nil {
		return nil, errors.Join(fmt.Errorf("start %s: %w", commandLine[0], err), pseudoConsole.Close())
	}
	agent := &Agent{pseudoConsole: pseudoConsole, emulator: vt.NewSafeEmulator(width, height), stateFile: stateFile,
		screenChanged: make(chan struct{}, 1), exited: make(chan struct{})}
	outputDone := make(chan struct{})
	go agent.copyOutputToEmulator(outputDone)
	go agent.forwardEmulatorInput()
	go agent.waitForExit(windows.Handle(processHandle), outputDone)
	return agent, nil
}

func (a *Agent) copyOutputToEmulator(outputDone chan<- struct{}) {
	defer close(outputDone)
	output := make([]byte, 64*1024)
	oscState := outsideOSC
	for {
		length, err := a.pseudoConsole.Read(output)
		if length > 0 {
			var parsableOutput []byte
			parsableOutput, oscState = withoutNonASCIIInOSC(output[:length], oscState)
			if _, writeErr := a.emulator.Write(parsableOutput); writeErr != nil {
				a.exitErr = writeErr
				return
			}
			select {
			case a.screenChanged <- struct{}{}:
			default:
			}
		}
		if err != nil {
			if !a.closing.Load() {
				a.exitErr = fmt.Errorf("read Claude's screen: %w", err)
			}
			return
		}
	}
}

// forwardEmulatorInput writes the keys SendKey encodes, and the emulator's replies to Claude's terminal queries, to
// Claude.
func (a *Agent) forwardEmulatorInput() {
	_, err := io.Copy(a.pseudoConsole, a.emulator)
	a.forwardMutex.Lock()
	defer a.forwardMutex.Unlock()
	a.forwardErr = errors.Join(errors.New("Claude takes no more keys"), err)
}

func (a *Agent) waitForExit(processHandle windows.Handle, outputDone <-chan struct{}) {
	defer close(a.exited)
	_, waitErr := windows.WaitForSingleObject(processHandle, windows.INFINITE)
	exitCodeErr := windows.GetExitCodeProcess(processHandle, &a.exitCode)
	a.closing.Store(true)
	closeErr := errors.Join(a.pseudoConsole.Close(), windows.CloseHandle(processHandle))
	<-outputDone
	a.exitErr = errors.Join(a.exitErr, waitErr, exitCodeErr, closeErr, a.emulator.Close())
}

// Close ends Claude if it still runs, then waits for it to exit and returns its exit code.
func (a *Agent) Close() (int, error) {
	select {
	case <-a.exited:
	default:
		if err := a.End(); err != nil {
			return 0, err
		}
		<-a.exited
	}
	return int(a.exitCode), a.exitErr
}

// End closes Claude's pseudo-console, which ends Claude, without waiting for it to exit.
func (a *Agent) End() error {
	a.closing.Store(true)
	return a.pseudoConsole.Close()
}

func (a *Agent) write(input string) error {
	if _, err := a.pseudoConsole.Write([]byte(input)); err != nil {
		return fmt.Errorf("write to Claude: %w", err)
	}
	return nil
}

func (a *Agent) SendPrompt(prompt string) error {
	status, err := a.Status()
	if err != nil {
		return err
	}
	if status == ui.AgentBlocked {
		return ui.ErrAgentBlocked
	}
	return a.write(ansi.BracketedPasteStart + prompt + ansi.BracketedPasteEnd + "\r")
}

// SendNow writes Claude Code's default chat:sendNow keys, ctrl+x ctrl+s.
func (a *Agent) SendNow() error {
	return a.write("\x18\x13")
}

// Interrupt writes Esc and puts Claude in idle until its next hook, since Claude Code runs no hook when interrupted.
func (a *Agent) Interrupt() error {
	if err := a.write("\x1b"); err != nil {
		return err
	}
	state, err := a.state()
	if err != nil {
		return err
	}
	if state.sessionID == "" {
		return nil
	}
	a.stateMutex.Lock()
	defer a.stateMutex.Unlock()
	a.lastState = hookState{status: ui.AgentIdle, sessionID: state.sessionID}
	if err := writeHookStateFile(a.stateFile, a.lastState); err != nil {
		return fmt.Errorf("write Claude's idle state after Esc: %w", err)
	}
	return nil
}

func (a *Agent) TakeInput() (string, error) {
	// The screen keeps only its dim attribute, the one claudecode.InputBoxOnScreen reads.
	styledScreen := func() (string, error) {
		rows := make([]string, a.emulator.Height())
		for rowIndex := range rows {
			row := make(uv.Line, a.emulator.Width())
			for column := range row {
				cell := a.emulator.CellAt(column, rowIndex)
				if cell == nil {
					continue
				}
				row[column] = *cell
				row[column].Style = uv.Style{Attrs: cell.Style.Attrs & uv.AttrFaint}
			}
			rows[rowIndex] = row.Render()
		}
		return strings.Join(rows, "\n"), nil
	}
	sendCtrlU := func(count int) error { return a.write(strings.Repeat("\x15", count)) }
	return ui.TakeClaudeInput("Claude's input", styledScreen, sendCtrlU)
}

// Status is AgentUnknown before Claude's first hook and once it has exited.
func (a *Agent) Status() (ui.AgentStatus, error) {
	select {
	case <-a.exited:
		return ui.AgentUnknown, nil
	default:
	}
	state, err := a.state()
	return state.status, err
}

// state is the last state the hooks wrote whole, empty before the first hook.
func (a *Agent) state() (hookState, error) {
	a.stateMutex.Lock()
	defer a.stateMutex.Unlock()
	content, err := os.ReadFile(a.stateFile)
	if errors.Is(err, fs.ErrNotExist) {
		return a.lastState, nil
	}
	if err != nil {
		return hookState{}, fmt.Errorf("read Claude's hook state: %w", err)
	}
	if state, whole := parseHookState(string(content)); whole {
		a.lastState = state
	}
	return a.lastState, nil
}

// WaitForSessionID waits for Claude's SessionStart hook, which a --resume or --continue runs with the resumed session.
func (a *Agent) WaitForSessionID() (string, error) {
	for {
		state, err := a.state()
		if err != nil {
			return "", err
		}
		if state.sessionID != "" {
			return state.sessionID, nil
		}
		select {
		case <-a.exited:
			return "", errors.New("Claude exited before it started a session")
		case <-time.After(300 * time.Millisecond):
		}
	}
}

func (a *Agent) Screen() (string, *tea.Cursor) {
	screen := a.emulator.Render()
	if a.emulator.CursorHidden() {
		return screen, nil
	}
	position := a.emulator.CursorPosition()
	return screen, tea.NewCursor(position.X, position.Y)
}

func (a *Agent) SendKey(key tea.KeyPressMsg) error {
	if err := a.inputErr(); err != nil {
		return err
	}
	a.emulator.SendKey(uv.KeyPressEvent(key))
	return nil
}

func (a *Agent) SendPaste(text string) error {
	if err := a.inputErr(); err != nil {
		return err
	}
	a.emulator.Paste(text)
	return nil
}

func (a *Agent) inputErr() error {
	a.forwardMutex.Lock()
	defer a.forwardMutex.Unlock()
	return a.forwardErr
}

func (a *Agent) Resize(width int, height int) error {
	a.emulator.Resize(width, height)
	return a.pseudoConsole.Resize(width, height)
}

func (a *Agent) ScreenChanged() <-chan struct{} {
	return a.screenChanged
}

func (a *Agent) Exited() <-chan struct{} {
	return a.exited
}

var _ ui.TerminalAgent = (*Agent)(nil)
