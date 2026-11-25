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
	"github.com/stretchr/testify/require"
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
// fakeClaudeDraft, appends each byte it reads to the file the variable names, empties the box on ctrl+u, draws a
// picker in its place on ctrl+p, draws Claude's trust dialog on ctrl+t, and on ctrl+d draws a message with a blank row
// and trailing spaces, then exits with fakeClaudeExitCode.
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
			if _, err := fmt.Fprint(os.Stdout, "\x1b[2J\x1b[HClaude Code didn't start.   \r\n\r\n  Run claude again.\r\n"); err != nil {
				return err
			}
			os.Exit(fakeClaudeExitCode)
		}
		if strings.Contains(string(input[:length]), "\x14") {
			trustDialog := "\x1b[2J\x1b[H Accessing workspace:\r\n\r\n ❯ No, exit\r\n   Yes, I trust this folder\r\n"
			if _, err := fmt.Fprint(os.Stdout, trustDialog); err != nil {
				return err
			}
		}
		if strings.Contains(string(input[:length]), "\x15") {
			if err := drawFakeInputBox(""); err != nil {
				return err
			}
		}
		if strings.Contains(string(input[:length]), "\x10") {
			if _, err := fmt.Fprint(os.Stdout, "\x1b[2J\x1b[HSelect model\r\n❯ 1. Default\r\n"); err != nil {
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
	t.Run("sendPromptWhileTrustDialogShowsWritesNothingAndReturnsAgentBlocked",
		sendPromptWhileTrustDialogShowsWritesNothingAndReturnsAgentBlocked)
	t.Run("waitForSessionIDAndSendKeyFailOnceClaudeExits", waitForSessionIDAndSendKeyFailOnceClaudeExits)
	t.Run("waitForSessionIDErrorHoldsExitCodeAndNonBlankRowsOfLastScreen",
		waitForSessionIDErrorHoldsExitCodeAndNonBlankRowsOfLastScreen)
	t.Run("takeInputWritesCtrlUTwicePerBoxRowAndReturnsDraft", takeInputWritesCtrlUTwicePerBoxRowAndReturnsDraft)
	t.Run("inputBoxShownUntilPickerTakesItsPlace", inputBoxShownUntilPickerTakesItsPlace)
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
	require.NoError(t, err)
	t.Cleanup(func() {
		_, closeErr := agent.Close()
		require.NoError(t, closeErr)
	})
	waitFor(t, "the fake Claude's input box", func() bool {
		screen, _ := agent.Screen()
		return strings.Contains(screen, "❯\u00a0"+fakeClaudeDraft)
	})
	return agent, inputLog, stateFile
}

func waitFor(t *testing.T, condition string, holds func() bool) {
	require.Eventually(t, holds, 10*time.Second, 20*time.Millisecond, "timed out waiting for %s", condition)
}

// waitForInput waits until the fake Claude has read want, and fails when it read anything else.
func waitForInput(t *testing.T, inputLog string, want string) {
	deadline := time.Now().Add(10 * time.Second)
	for {
		input, err := os.ReadFile(inputLog)
		require.NoError(t, err)
		read := string(input)
		if read == want {
			return
		}
		readIsStartOfWant := strings.HasPrefix(want, read)
		require.True(t, readIsStartOfWant, "Claude read %q, want %q", read, want)
		timedOut := time.Now().After(deadline)
		require.False(t, timedOut, "Claude read %q, want %q", read, want)
		time.Sleep(20 * time.Millisecond)
	}
}

func sendPromptWritesBracketedPasteThenEnter(t *testing.T) {
	agent, inputLog, _ := startFakeClaude(t)
	err := agent.SendPrompt("ship it\nnow")
	require.NoError(t, err)
	waitForInput(t, inputLog, "\x1b[200~ship it\nnow\x1b[201~\r")
}

func sendPromptToBlockedClaudeWritesNothingAndReturnsAgentBlocked(t *testing.T) {
	agent, inputLog, stateFile := startFakeClaude(t)
	writeErr := os.WriteFile(stateFile, []byte("blocked session-1\n"), 0o600)
	require.NoError(t, writeErr)
	sendErr := agent.SendPrompt("ship it")
	require.ErrorIs(t, sendErr, ui.ErrAgentBlocked)
	interruptErr := agent.Interrupt()
	require.NoError(t, interruptErr)
	waitForInput(t, inputLog, "\x1b")
}

func sendNowWritesCtrlXCtrlS(t *testing.T) {
	agent, inputLog, _ := startFakeClaude(t)
	err := agent.SendNow()
	require.NoError(t, err)
	waitForInput(t, inputLog, "\x18\x13")
}

func interruptWritesEsc(t *testing.T) {
	agent, inputLog, _ := startFakeClaude(t)
	err := agent.Interrupt()
	require.NoError(t, err)
	waitForInput(t, inputLog, "\x1b")
}

func interruptMakesWorkingClaudeIdleUntilItsNextHook(t *testing.T) {
	agent, _, stateFile := startFakeClaude(t)
	writeErr := os.WriteFile(stateFile, []byte("working session-1\n"), 0o600)
	require.NoError(t, writeErr)
	assertStatus(t, agent, ui.AgentWorking)
	interruptErr := agent.Interrupt()
	require.NoError(t, interruptErr)
	assertStatus(t, agent, ui.AgentIdle)
	promptHook := strings.NewReader(`{"session_id":"session-1","hook_event_name":"UserPromptSubmit"}`)
	hookErr := WriteHookState(stateFile, promptHook)
	require.NoError(t, hookErr)
	assertStatus(t, agent, ui.AgentWorking)
}

func sendKeyAndPasteReachClaudeThroughEmulator(t *testing.T) {
	agent, inputLog, _ := startFakeClaude(t)
	keyErr := agent.SendKey(tea.KeyPressMsg{Code: 'q', Text: "q"})
	require.NoError(t, keyErr)
	pasteErr := agent.SendPaste("pasted")
	require.NoError(t, pasteErr)
	waitForInput(t, inputLog, "q\x1b[200~pasted\x1b[201~")
}

func waitForSessionIDAndSendKeyFailOnceClaudeExits(t *testing.T) {
	agent, _, _ := startFakeClaude(t)
	writeErr := agent.write("\x04")
	require.NoError(t, writeErr)
	_, waitErr := agent.WaitForSessionID()
	require.Error(t, waitErr)
	waitFor(t, "SendKey to fail", func() bool {
		keyErr := agent.SendKey(tea.KeyPressMsg{Code: 'q', Text: "q"})
		return keyErr != nil
	})
}

func waitForSessionIDErrorHoldsExitCodeAndNonBlankRowsOfLastScreen(t *testing.T) {
	agent, _, _ := startFakeClaude(t)
	writeErr := agent.write("\x04")
	require.NoError(t, writeErr)
	_, waitErr := agent.WaitForSessionID()
	want := "Claude exited before it started a session (exit code 7). Its last screen:\n" +
		"Claude Code didn't start.\n  Run claude again."
	require.EqualError(t, waitErr, want)
}

func sendPromptWhileTrustDialogShowsWritesNothingAndReturnsAgentBlocked(t *testing.T) {
	agent, inputLog, _ := startFakeClaude(t)
	keyErr := agent.SendKey(tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	require.NoError(t, keyErr)
	waitFor(t, "the trust dialog", func() bool {
		screen, _ := agent.Screen()
		return strings.Contains(screen, "Yes, I trust this folder")
	})
	sendErr := agent.SendPrompt("ship it")
	require.ErrorIs(t, sendErr, ui.ErrAgentBlocked)
	sendNowErr := agent.SendNow()
	require.NoError(t, sendNowErr)
	waitForInput(t, inputLog, "\x14\x18\x13")
}

func takeInputWritesCtrlUTwicePerBoxRowAndReturnsDraft(t *testing.T) {
	agent, inputLog, _ := startFakeClaude(t)
	draft, err := agent.TakeInput()
	require.NoError(t, err)
	require.Equal(t, fakeClaudeDraft, draft)
	waitForInput(t, inputLog, "\x15\x15")
}

func inputBoxShownUntilPickerTakesItsPlace(t *testing.T) {
	agent, _, _ := startFakeClaude(t)
	shown, err := agent.InputBoxShown()
	require.NoError(t, err)
	require.True(t, shown)
	keyErr := agent.SendKey(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	require.NoError(t, keyErr)
	waitFor(t, "the input box to give way to the picker", func() bool {
		shownNow, shownErr := agent.InputBoxShown()
		return shownErr == nil && !shownNow
	})
}

func statusIsLastWholeHookStateAndUnknownBeforeFirstHook(t *testing.T) {
	agent, _, stateFile := startFakeClaude(t)
	assertStatus(t, agent, ui.AgentUnknown)
	wholeWriteErr := os.WriteFile(stateFile, []byte("working session-1\n"), 0o600)
	require.NoError(t, wholeWriteErr)
	assertStatus(t, agent, ui.AgentWorking)
	partialWriteErr := os.WriteFile(stateFile, []byte("done sess"), 0o600)
	require.NoError(t, partialWriteErr)
	assertStatus(t, agent, ui.AgentWorking)
	sessionID, err := agent.WaitForSessionID()
	require.NoError(t, err)
	require.Equal(t, "session-1", sessionID)
}

func assertStatus(t *testing.T, agent *Agent, want ui.AgentStatus) {
	t.Helper()
	got, err := agent.Status()
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func resizeSizesClaudesScreen(t *testing.T) {
	agent, _, _ := startFakeClaude(t)
	resizeErr := agent.Resize(100, 30)
	require.NoError(t, resizeErr)
	emulatorWidth := agent.emulator.Width()
	require.Equal(t, 100, emulatorWidth)
	emulatorHeight := agent.emulator.Height()
	require.Equal(t, 30, emulatorHeight)
	consoleWidth, consoleHeight, sizeErr := agent.pseudoConsole.Size()
	require.NoError(t, sizeErr)
	require.Equal(t, 100, consoleWidth)
	require.Equal(t, 30, consoleHeight)
}

func closeEndsClaudeAndExitedCloses(t *testing.T) {
	agent, _, _ := startFakeClaude(t)
	_, err := agent.Close()
	require.NoError(t, err)
	exitedClosed := false
	select {
	case <-agent.Exited():
		exitedClosed = true
	default:
	}
	require.True(t, exitedClosed)
	assertStatus(t, agent, ui.AgentUnknown)
}

func closeAfterClaudeExitsReturnsClaudesExitCode(t *testing.T) {
	agent, _, _ := startFakeClaude(t)
	writeErr := agent.write("\x04")
	require.NoError(t, writeErr)
	exitedWithinTenSeconds := false
	select {
	case <-agent.Exited():
		exitedWithinTenSeconds = true
	case <-time.After(10 * time.Second):
	}
	require.True(t, exitedWithinTenSeconds)
	exitCode, err := agent.Close()
	require.NoError(t, err)
	require.Equal(t, fakeClaudeExitCode, exitCode)
}
