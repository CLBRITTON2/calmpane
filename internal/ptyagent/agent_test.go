package ptyagent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/sys/windows"

	"github.com/CLBRITTON2/calmpane/internal/ui"
)

func TestMain(m *testing.M) {
	if inputLog := os.Getenv(fakeClaudeInputLogEnv); inputLog != "" {
		exitWithHelperError(runFakeClaude(inputLog))
	}
	if zenMode := os.Getenv(fakeZenEnv); zenMode != "" {
		exitWithHelperError(runFakeZen(zenMode, os.Getenv(fakeZenChildEnv)))
	}
	if childMode := os.Getenv(fakeZenChildEnv); childMode != "" {
		exitWithHelperError(runFakeZenChild(childMode))
	}
	os.Exit(m.Run())
}

func exitWithHelperError(helperErr error) {
	if helperErr != nil {
		fmt.Fprintln(os.Stderr, helperErr)
		os.Exit(1)
	}
	os.Exit(0)
}

// fakeClaudeInputLogEnv makes the test binary act as Claude Code in a pseudo-console: it draws an input box holding
// fakeClaudeDraft, appends each byte it reads to the file the variable names, empties the box on ctrl+u, and exits
// with fakeClaudeExitCode on ctrl+d.
const fakeClaudeInputLogEnv = "ZEN_FAKE_CLAUDE_INPUT_LOG"

const fakeClaudeDraft = "fix the tests"

const fakeClaudeExitCode = 7

func runFakeClaude(inputLog string) (err error) {
	inputLogFile, err := os.OpenFile(inputLog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, inputLogFile.Close()) }()
	consoleInput, err := windows.GetStdHandle(windows.STD_INPUT_HANDLE)
	if err != nil {
		return err
	}
	// Claude Code reads its keys as VT sequences, with no line editing by the console
	if err := windows.SetConsoleMode(consoleInput, windows.ENABLE_VIRTUAL_TERMINAL_INPUT); err != nil {
		return fmt.Errorf("set raw VT input: %w", err)
	}
	if err := drawFakeInputBox(fakeClaudeDraft); err != nil {
		return err
	}
	input := make([]byte, 1024)
	for {
		length, err := os.Stdin.Read(input)
		if err != nil {
			return err
		}
		if _, err := inputLogFile.Write(input[:length]); err != nil {
			return err
		}
		if strings.Contains(string(input[:length]), "\x04") {
			os.Exit(fakeClaudeExitCode)
		}
		if strings.Contains(string(input[:length]), "\x15") {
			if err := drawFakeInputBox(""); err != nil {
				return err
			}
		}
	}
}

func drawFakeInputBox(draft string) error {
	rule := strings.Repeat("─", 40)
	_, err := fmt.Fprintf(os.Stdout, "\x1b[?2004h\x1b[2J\x1b[H%s\r\n❯ %s\r\n%s\r\n", rule, draft, rule)
	return err
}

func TestAgent(t *testing.T) {
	t.Run("sendPromptWritesBracketedPasteThenEnter", sendPromptWritesBracketedPasteThenEnter)
	t.Run("sendPromptToBlockedClaudeWritesNothingAndReturnsAgentBlocked",
		sendPromptToBlockedClaudeWritesNothingAndReturnsAgentBlocked)
	t.Run("sendNowWritesCtrlXCtrlS", sendNowWritesCtrlXCtrlS)
	t.Run("interruptWritesEsc", interruptWritesEsc)
	t.Run("interruptMakesWorkingClaudeIdleUntilItsNextHook", interruptMakesWorkingClaudeIdleUntilItsNextHook)
	t.Run("sendKeyAndPasteReachClaudeThroughEmulator", sendKeyAndPasteReachClaudeThroughEmulator)
	t.Run("waitForSessionIDAndSendKeyFailOnceClaudeExits", waitForSessionIDAndSendKeyFailOnceClaudeExits)
	t.Run("takeInputWritesCtrlUTwicePerBoxRowAndReturnsDraft", takeInputWritesCtrlUTwicePerBoxRowAndReturnsDraft)
	t.Run("statusIsLastWholeHookStateAndUnknownBeforeFirstHook", statusIsLastWholeHookStateAndUnknownBeforeFirstHook)
	t.Run("resizeSizesClaudesScreen", resizeSizesClaudesScreen)
	t.Run("closeEndsClaudeAndExitedCloses", closeEndsClaudeAndExitedCloses)
	t.Run("closeAfterClaudeExitsReturnsClaudesExitCode", closeAfterClaudeExitsReturnsClaudesExitCode)
}

// startFakeClaude starts the fake Claude and waits until its input box shows.
func startFakeClaude(t *testing.T) (agent *Agent, inputLog string, stateFile string) {
	testDir := t.TempDir()
	inputLog = filepath.Join(testDir, "input.log")
	stateFile = filepath.Join(testDir, "state")
	t.Setenv(fakeClaudeInputLogEnv, inputLog)
	agent, err := Start([]string{os.Args[0]}, stateFile, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := agent.Close(); err != nil {
			t.Error(err)
		}
	})
	waitFor(t, "the fake Claude's input box", func() bool {
		screen, _ := agent.Screen()
		return strings.Contains(screen, "❯\u00a0"+fakeClaudeDraft)
	})
	return agent, inputLog, stateFile
}

func waitFor(t *testing.T, condition string, holds func() bool) {
	deadline := time.Now().Add(10 * time.Second)
	for !holds() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", condition)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// waitForInput waits until the fake Claude has read want, and fails when it read anything else.
func waitForInput(t *testing.T, inputLog string, want string) {
	deadline := time.Now().Add(10 * time.Second)
	for {
		input, err := os.ReadFile(inputLog)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		if string(input) == want {
			return
		}
		if !strings.HasPrefix(want, string(input)) || time.Now().After(deadline) {
			t.Fatalf("Claude read %q, want %q", input, want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func sendPromptWritesBracketedPasteThenEnter(t *testing.T) {
	agent, inputLog, _ := startFakeClaude(t)
	if err := agent.SendPrompt("ship it\nnow"); err != nil {
		t.Fatal(err)
	}
	waitForInput(t, inputLog, "\x1b[200~ship it\nnow\x1b[201~\r")
}

func sendPromptToBlockedClaudeWritesNothingAndReturnsAgentBlocked(t *testing.T) {
	agent, inputLog, stateFile := startFakeClaude(t)
	if err := os.WriteFile(stateFile, []byte("blocked session-1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := agent.SendPrompt("ship it"); !errors.Is(err, ui.ErrAgentBlocked) {
		t.Errorf("SendPrompt error = %v, want ui.ErrAgentBlocked", err)
	}
	if err := agent.Interrupt(); err != nil {
		t.Fatal(err)
	}
	waitForInput(t, inputLog, "\x1b")
}

func sendNowWritesCtrlXCtrlS(t *testing.T) {
	agent, inputLog, _ := startFakeClaude(t)
	if err := agent.SendNow(); err != nil {
		t.Fatal(err)
	}
	waitForInput(t, inputLog, "\x18\x13")
}

func interruptWritesEsc(t *testing.T) {
	agent, inputLog, _ := startFakeClaude(t)
	if err := agent.Interrupt(); err != nil {
		t.Fatal(err)
	}
	waitForInput(t, inputLog, "\x1b")
}

func interruptMakesWorkingClaudeIdleUntilItsNextHook(t *testing.T) {
	agent, _, stateFile := startFakeClaude(t)
	if err := os.WriteFile(stateFile, []byte("working session-1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	assertStatus(t, agent, ui.AgentWorking)
	if err := agent.Interrupt(); err != nil {
		t.Fatal(err)
	}
	assertStatus(t, agent, ui.AgentIdle)
	promptHook := `{"session_id":"session-1","hook_event_name":"UserPromptSubmit"}`
	if err := WriteHookState(stateFile, strings.NewReader(promptHook)); err != nil {
		t.Fatal(err)
	}
	assertStatus(t, agent, ui.AgentWorking)
}

func sendKeyAndPasteReachClaudeThroughEmulator(t *testing.T) {
	agent, inputLog, _ := startFakeClaude(t)
	if err := agent.SendKey(tea.KeyPressMsg{Code: 'q', Text: "q"}); err != nil {
		t.Fatal(err)
	}
	if err := agent.SendPaste("pasted"); err != nil {
		t.Fatal(err)
	}
	waitForInput(t, inputLog, "q\x1b[200~pasted\x1b[201~")
}

func waitForSessionIDAndSendKeyFailOnceClaudeExits(t *testing.T) {
	agent, _, _ := startFakeClaude(t)
	if err := agent.write("\x04"); err != nil {
		t.Fatal(err)
	}
	if _, err := agent.WaitForSessionID(); err == nil {
		t.Error("WaitForSessionID returned no error after Claude exited with no session")
	}
	waitFor(t, "SendKey to fail", func() bool {
		return agent.SendKey(tea.KeyPressMsg{Code: 'q', Text: "q"}) != nil
	})
}

func takeInputWritesCtrlUTwicePerBoxRowAndReturnsDraft(t *testing.T) {
	agent, inputLog, _ := startFakeClaude(t)
	draft, err := agent.TakeInput()
	if err != nil {
		t.Fatal(err)
	}
	if draft != fakeClaudeDraft {
		t.Errorf("TakeInput = %q, want %q", draft, fakeClaudeDraft)
	}
	waitForInput(t, inputLog, "\x15\x15")
}

func statusIsLastWholeHookStateAndUnknownBeforeFirstHook(t *testing.T) {
	agent, _, stateFile := startFakeClaude(t)
	assertStatus(t, agent, ui.AgentUnknown)
	if err := os.WriteFile(stateFile, []byte("working session-1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	assertStatus(t, agent, ui.AgentWorking)
	if err := os.WriteFile(stateFile, []byte("done sess"), 0o600); err != nil {
		t.Fatal(err)
	}
	assertStatus(t, agent, ui.AgentWorking)
	sessionID, err := agent.WaitForSessionID()
	if err != nil {
		t.Fatal(err)
	}
	if sessionID != "session-1" {
		t.Errorf("WaitForSessionID = %q, want session-1", sessionID)
	}
}

func assertStatus(t *testing.T, agent *Agent, want ui.AgentStatus) {
	t.Helper()
	got, err := agent.Status()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("Status = %d, want %d", got, want)
	}
}

func resizeSizesClaudesScreen(t *testing.T) {
	agent, _, _ := startFakeClaude(t)
	if err := agent.Resize(100, 30); err != nil {
		t.Fatal(err)
	}
	if width, height := agent.emulator.Width(), agent.emulator.Height(); width != 100 || height != 30 {
		t.Errorf("emulator size = %dx%d, want 100x30", width, height)
	}
	width, height, err := agent.pseudoConsole.Size()
	if err != nil {
		t.Fatal(err)
	}
	if width != 100 || height != 30 {
		t.Errorf("pseudo-console size = %dx%d, want 100x30", width, height)
	}
}

func closeEndsClaudeAndExitedCloses(t *testing.T) {
	agent, _, _ := startFakeClaude(t)
	if _, err := agent.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-agent.Exited():
	default:
		t.Error("Exited is open after Close returned")
	}
	assertStatus(t, agent, ui.AgentUnknown)
}

func closeAfterClaudeExitsReturnsClaudesExitCode(t *testing.T) {
	agent, _, _ := startFakeClaude(t)
	if err := agent.write("\x04"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-agent.Exited():
	case <-time.After(10 * time.Second):
		t.Fatal("Exited is open 10s after Claude was told to exit")
	}
	exitCode, err := agent.Close()
	if err != nil {
		t.Fatal(err)
	}
	if exitCode != fakeClaudeExitCode {
		t.Errorf("Close exit code = %d, want %d", exitCode, fakeClaudeExitCode)
	}
}
