package zenmode

import (
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/CLBRITTON2/calmpane/internal/herdr"
	"github.com/CLBRITTON2/calmpane/internal/herdr/herdrtest"
	"github.com/CLBRITTON2/calmpane/internal/ui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHerdrAgent(t *testing.T) {
	t.Run("statusIsWorkingWhileHerdrReportsWorking", statusIsWorkingWhileHerdrReportsWorking)
	t.Run("statusIsIdleWhileHerdrReportsIdle", statusIsIdleWhileHerdrReportsIdle)
	t.Run("statusIsDoneWhileHerdrReportsDone", statusIsDoneWhileHerdrReportsDone)
	t.Run("statusIsBlockedWhileHerdrReportsBlocked", statusIsBlockedWhileHerdrReportsBlocked)
	t.Run("statusIsUnknownWhileHerdrReportsUnknown", statusIsUnknownWhileHerdrReportsUnknown)
	t.Run("sendPromptToBlockedClaudeReturnsAgentBlocked", sendPromptToBlockedClaudeReturnsAgentBlocked)
	t.Run("sendPromptSendNowAndInterruptReachClaudePane", sendPromptSendNowAndInterruptReachClaudePane)
	t.Run("sendPromptSendsPromptLongerThanWindowsCommandLineWhole", sendPromptSendsPromptLongerThanWindowsCommandLineWhole)
	t.Run("sendPromptRefusedByHerdrFailsWithItsErrorAndWithoutPrompt", sendPromptRefusedByHerdrFailsWithItsErrorAndWithoutPrompt)
	t.Run("sendPromptWithoutSocketPathFailsNamingHerdrSocketPath", sendPromptWithoutSocketPathFailsNamingHerdrSocketPath)
	t.Run("waitForSessionIDIsSessionInSessionsFileOfPaneClaudeProcess", waitForSessionIDIsSessionInSessionsFileOfPaneClaudeProcess)
	t.Run("waitForSessionIDWaitsUntilClaudeProcessWritesSessionsFile", waitForSessionIDWaitsUntilClaudeProcessWritesSessionsFile)
	t.Run("waitForSessionIDWaitsPastMissingLimitWhileTrustDialogShows", waitForSessionIDWaitsPastMissingLimitWhileTrustDialogShows)
	t.Run("waitForSessionIDFailsNamingPidAndPathOnceSessionsFileMissingForLimit",
		waitForSessionIDFailsNamingPidAndPathOnceSessionsFileMissingForLimit)
	t.Run("waitForSessionIDFailsWithAgentGoneOnceHerdrDropsClaudeWhileSessionsFileMissing",
		waitForSessionIDFailsWithAgentGoneOnceHerdrDropsClaudeWhileSessionsFileMissing)
	t.Run("waitForSessionIDFailsWithAgentGoneOnceHerdrDropsClaudeAndPaneRunsNoClaudeProcess",
		waitForSessionIDFailsWithAgentGoneOnceHerdrDropsClaudeAndPaneRunsNoClaudeProcess)
	t.Run("waitForSessionIDFailsNamingPaneWhileHerdrReportsClaudeWithoutClaudeProcess",
		waitForSessionIDFailsNamingPaneWhileHerdrReportsClaudeWithoutClaudeProcess)
	t.Run("waitForSessionIDFailsNamingPathWhenSessionsFileHoldsAnotherPid", waitForSessionIDFailsNamingPathWhenSessionsFileHoldsAnotherPid)
	t.Run("statusIsGoneOncePaneRunsNoAgent", statusIsGoneOncePaneRunsNoAgent)
	t.Run("waitForSessionIDFailsWithoutClaudePaneID", waitForSessionIDFailsWithoutClaudePaneID)
	t.Run("inputBoxShownWhileClaudePaneShowsEmptyInputBox", inputBoxShownWhileClaudePaneShowsEmptyInputBox)
	t.Run("inputBoxNotShownWhileClaudePaneShowsBoxScrolledPastItsTop", inputBoxNotShownWhileClaudePaneShowsBoxScrolledPastItsTop)
	t.Run("workingDirectoryIsClaudePaneCwd", workingDirectoryIsClaudePaneCwd)
	t.Run("workingDirectoryFailsForPaneWithoutCwd", workingDirectoryFailsForPaneWithoutCwd)
}

func inputBoxShownWhileClaudePaneShowsEmptyInputBox(t *testing.T) {
	cli, _ := herdrtest.Fake(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", true)})
	herdrtest.ShowOnScreen(t, "w1:p1", "claude-input-empty.ansi", "")
	shown, err := HerdrAgent{CLI: cli, ClaudePaneID: "w1:p1"}.InputBoxShown()
	require.NoError(t, err)
	require.True(t, shown)
}

func inputBoxNotShownWhileClaudePaneShowsBoxScrolledPastItsTop(t *testing.T) {
	cli, _ := herdrtest.Fake(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", true)})
	herdrtest.ShowOnScreen(t, "w1:p1", "claude-input-scrolled.ansi", "")
	shown, err := HerdrAgent{CLI: cli, ClaudePaneID: "w1:p1"}.InputBoxShown()
	require.NoError(t, err)
	require.False(t, shown)
}

func workingDirectoryIsClaudePaneCwd(t *testing.T) {
	claudePane := herdrtest.ClaudePane("w1:p1", true)
	claudePane.Cwd = `C:\work\project\`
	cli, _ := herdrtest.Fake(t, []herdr.Pane{claudePane})
	workingDirectory, err := HerdrAgent{CLI: cli, ClaudePaneID: "w1:p1"}.WorkingDirectory()
	require.NoError(t, err)
	require.Equal(t, claudePane.Cwd, workingDirectory)
}

func workingDirectoryFailsForPaneWithoutCwd(t *testing.T) {
	cli, _ := herdrtest.Fake(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", true)})
	_, err := HerdrAgent{CLI: cli, ClaudePaneID: "w1:p1"}.WorkingDirectory()
	require.EqualError(t, err, "herdr reports no working directory for pane w1:p1")
}

func assertHerdrStatusMapsToAgentStatus(t *testing.T, herdrStatus string, wantStatus ui.AgentStatus) {
	claudePane := herdrtest.ClaudePane("w1:p1", true)
	claudePane.AgentStatus = herdrStatus
	cli, _ := herdrtest.Fake(t, []herdr.Pane{claudePane})
	agentStatus, err := HerdrAgent{CLI: cli, ClaudePaneID: "w1:p1"}.Status()
	require.NoError(t, err)
	require.Equal(t, wantStatus, agentStatus)
}

func statusIsWorkingWhileHerdrReportsWorking(t *testing.T) {
	assertHerdrStatusMapsToAgentStatus(t, "working", ui.AgentWorking)
}

func statusIsIdleWhileHerdrReportsIdle(t *testing.T) {
	assertHerdrStatusMapsToAgentStatus(t, "idle", ui.AgentIdle)
}

func statusIsDoneWhileHerdrReportsDone(t *testing.T) {
	assertHerdrStatusMapsToAgentStatus(t, "done", ui.AgentDone)
}

func statusIsBlockedWhileHerdrReportsBlocked(t *testing.T) {
	assertHerdrStatusMapsToAgentStatus(t, "blocked", ui.AgentBlocked)
}

func statusIsUnknownWhileHerdrReportsUnknown(t *testing.T) {
	assertHerdrStatusMapsToAgentStatus(t, "unknown", ui.AgentUnknown)
}

func sendPromptToBlockedClaudeReturnsAgentBlocked(t *testing.T) {
	claudePane := herdrtest.ClaudePane("w1:p1", true)
	claudePane.AgentStatus = "blocked"
	api, _ := herdrtest.FakeAPI(t, []herdr.Pane{claudePane})
	err := HerdrAgent{API: api, ClaudePaneID: "w1:p1"}.SendPrompt("go")
	require.ErrorIs(t, err, ui.ErrAgentBlocked)
}

func sendPromptSendNowAndInterruptReachClaudePane(t *testing.T) {
	panes := []herdr.Pane{herdrtest.ClaudePane("w1:p1", true)}
	cli, calls := herdrtest.Fake(t, panes)
	api, requests := herdrtest.FakeAPI(t, panes)
	agent := HerdrAgent{CLI: cli, API: api, ClaudePaneID: "w1:p1"}
	promptErr := agent.SendPrompt("ship it")
	require.NoError(t, promptErr)
	sendNowErr := agent.SendNow()
	require.NoError(t, sendNowErr)
	interruptErr := agent.Interrupt()
	require.NoError(t, interruptErr)
	apiRequests := requests()
	wantRequests := []string{"agent.prompt w1:p1 ship it"}
	require.Equal(t, wantRequests, apiRequests)
	herdrCalls := calls()
	wantCalls := []string{"pane send-keys w1:p1 ctrl+x ctrl+s", "pane send-keys w1:p1 esc"}
	require.Equal(t, wantCalls, herdrCalls)
}

func sendPromptRefusedByHerdrFailsWithItsErrorAndWithoutPrompt(t *testing.T) {
	api, _ := herdrtest.FakeAPI(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", true)})
	err := HerdrAgent{API: api, ClaudePaneID: "w1:p9"}.SendPrompt("secret prompt text")
	require.EqualError(t, err, "herdr agent.prompt failed: pane_not_found: pane w1:p9 not found")
}

func sendPromptWithoutSocketPathFailsNamingHerdrSocketPath(t *testing.T) {
	err := HerdrAgent{API: herdr.API{}, ClaudePaneID: "w1:p1"}.SendPrompt("go")
	require.EqualError(t, err, "herdr agent.prompt failed: HERDR_SOCKET_PATH is not set")
}

func sendPromptSendsPromptLongerThanWindowsCommandLineWhole(t *testing.T) {
	api, requests := herdrtest.FakeAPI(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", true)})
	prompt := strings.Repeat("a line of the pasted log\n", 1600)
	err := HerdrAgent{API: api, ClaudePaneID: "w1:p1"}.SendPrompt(prompt)
	require.NoError(t, err)
	apiRequests := requests()
	wantRequests := []string{"agent.prompt w1:p1 " + prompt}
	require.Equal(t, wantRequests, apiRequests)
}

const claudeSessionID = "24e27105-227a-4078-90e7-50d3dea316bf"

const claudeProcessID = 4376

func writeTestSessionsFile(t *testing.T, home string, sessionsFileJSON string) {
	t.Helper()
	sessionsDir := filepath.Join(home, ".claude", "sessions")
	err := os.MkdirAll(sessionsDir, 0o755)
	require.NoError(t, err)
	sessionsFilePath := filepath.Join(sessionsDir, strconv.Itoa(claudeProcessID)+".json")
	err = os.WriteFile(sessionsFilePath, []byte(sessionsFileJSON), 0o644)
	require.NoError(t, err)
}

func agentInPaneRunningClaudeProcess(t *testing.T, claudePane herdr.Pane) (HerdrAgent, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	cli, _ := herdrtest.Fake(t, []herdr.Pane{claudePane})
	herdrtest.RunInForeground(t, claudePane.PaneID, []herdr.ForegroundProcess{
		{Name: "pwsh.exe", PID: 3672}, {Name: "claude.exe", PID: claudeProcessID}})
	return HerdrAgent{CLI: cli, ClaudePaneID: claudePane.PaneID}, home
}

func waitForSessionIDIsSessionInSessionsFileOfPaneClaudeProcess(t *testing.T) {
	agent, home := agentInPaneRunningClaudeProcess(t, herdrtest.ClaudePane("w1:p1", true))
	writeTestSessionsFile(t, home, `{"pid":4376,"sessionId":"`+claudeSessionID+`","status":"idle"}`)
	sessionID, err := agent.WaitForSessionID()
	require.NoError(t, err)
	require.Equal(t, claudeSessionID, sessionID)
}

func waitForSessionIDWaitsUntilClaudeProcessWritesSessionsFile(t *testing.T) {
	agent, home := agentInPaneRunningClaudeProcess(t, herdrtest.ClaudePane("w1:p1", true))
	reported := make(chan string, 1)
	go func() {
		sessionID, err := agent.WaitForSessionID()
		assert.NoError(t, err)
		reported <- sessionID
	}()
	time.Sleep(time.Second)
	require.Empty(t, reported)
	writeTestSessionsFile(t, home, `{"pid":4376,"sessionId":"`+claudeSessionID+`"}`)
	select {
	case reportedSessionID := <-reported:
		require.Equal(t, claudeSessionID, reportedSessionID)
	case <-time.After(5 * time.Second):
		require.FailNow(t, "still waiting 5 s after Claude wrote its sessions file")
	}
}

func waitForSessionIDWaitsPastMissingLimitWhileTrustDialogShows(t *testing.T) {
	agent, home := agentInPaneRunningClaudeProcess(t, herdrtest.ClaudePane("w1:p1", true))
	herdrtest.ShowOnScreen(t, "w1:p1", "claude-trust-dialog.ansi", "")
	reported := make(chan string, 1)
	go func() {
		sessionID, err := agent.WaitForSessionID()
		assert.NoError(t, err)
		reported <- sessionID
	}()
	time.Sleep(sessionsFileMissingLimit + time.Second)
	require.Empty(t, reported)
	writeTestSessionsFile(t, home, `{"pid":4376,"sessionId":"`+claudeSessionID+`"}`)
	select {
	case reportedSessionID := <-reported:
		require.Equal(t, claudeSessionID, reportedSessionID)
	case <-time.After(5 * time.Second):
		require.FailNow(t, "still waiting 5 s after Claude wrote its sessions file")
	}
}

func waitForSessionIDFailsNamingPidAndPathOnceSessionsFileMissingForLimit(t *testing.T) {
	agent, home := agentInPaneRunningClaudeProcess(t, herdrtest.ClaudePane("w1:p1", true))
	started := time.Now()
	_, err := agent.WaitForSessionID()
	waited := time.Since(started)
	require.GreaterOrEqual(t, waited, sessionsFileMissingLimit)
	require.ErrorIs(t, err, fs.ErrNotExist)
	require.NotErrorIs(t, err, ui.ErrAgentGone)
	sessionsFilePath := filepath.Join(home, ".claude", "sessions", "4376.json")
	require.ErrorContains(t, err, sessionsFilePath)
	require.ErrorContains(t, err, "Claude in pane w1:p1 has had no sessions file for 5s")
	require.ErrorContains(t, err, "Claude process 4376")
}

func waitForSessionIDFailsWithAgentGoneOnceHerdrDropsClaudeWhileSessionsFileMissing(t *testing.T) {
	agent, _ := agentInPaneRunningClaudeProcess(t, herdrtest.ClaudePane("w1:p1", true))
	started := time.Now()
	failed := make(chan error, 1)
	go func() {
		_, err := agent.WaitForSessionID()
		failed <- err
	}()
	time.Sleep(time.Second)
	require.Empty(t, failed)
	agentlessPanesJSON := `[{"pane_id":"w1:p1","focused":true,"agent_status":"idle"}]`
	panesFile := filepath.Join(os.Getenv(herdrtest.FakeDirEnv), "panes.json")
	err := os.WriteFile(panesFile, []byte(agentlessPanesJSON), 0o644)
	require.NoError(t, err)
	select {
	case err = <-failed:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "still waiting 5 s after herdr dropped Claude")
	}
	waited := time.Since(started)
	require.ErrorIs(t, err, ui.ErrAgentGone)
	require.Less(t, waited, sessionsFileMissingLimit)
}

func waitForSessionIDFailsWithAgentGoneOnceHerdrDropsClaudeAndPaneRunsNoClaudeProcess(t *testing.T) {
	cli, _ := herdrtest.Fake(t, []herdr.Pane{{PaneID: "w1:p1", Focused: true, AgentStatus: "idle"}})
	herdrtest.RunInForeground(t, "w1:p1", []herdr.ForegroundProcess{{Name: "pwsh.exe", PID: 3672}})
	_, err := HerdrAgent{CLI: cli, ClaudePaneID: "w1:p1"}.WaitForSessionID()
	require.ErrorIs(t, err, ui.ErrAgentGone)
}

func waitForSessionIDFailsNamingPaneWhileHerdrReportsClaudeWithoutClaudeProcess(t *testing.T) {
	cli, _ := herdrtest.Fake(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", true)})
	herdrtest.RunInForeground(t, "w1:p1", []herdr.ForegroundProcess{{Name: "pwsh.exe", PID: 3672}})
	_, err := HerdrAgent{CLI: cli, ClaudePaneID: "w1:p1"}.WaitForSessionID()
	require.NotErrorIs(t, err, ui.ErrAgentGone)
	require.EqualError(t, err,
		"herdr reports Claude in pane w1:p1, whose foreground has 0 claude.exe processes [], not one")
}

func waitForSessionIDFailsNamingPathWhenSessionsFileHoldsAnotherPid(t *testing.T) {
	agent, home := agentInPaneRunningClaudeProcess(t, herdrtest.ClaudePane("w1:p1", true))
	writeTestSessionsFile(t, home, `{"pid":8200,"sessionId":"`+claudeSessionID+`"}`)
	_, err := agent.WaitForSessionID()
	sessionsFilePath := filepath.Join(home, ".claude", "sessions", "4376.json")
	wantText := "Claude in pane w1:p1: " + sessionsFilePath + " holds pid 8200, not Claude process 4376"
	require.EqualError(t, err, wantText)
}

func statusIsGoneOncePaneRunsNoAgent(t *testing.T) {
	cli, _ := herdrtest.Fake(t, []herdr.Pane{{PaneID: "w1:p1", Focused: true, AgentStatus: "idle"}})
	agentStatus, err := HerdrAgent{CLI: cli, ClaudePaneID: "w1:p1"}.Status()
	require.NoError(t, err)
	require.Equal(t, ui.AgentGone, agentStatus)
}

func waitForSessionIDFailsWithoutClaudePaneID(t *testing.T) {
	cli, calls := herdrtest.Fake(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", true)})
	_, err := HerdrAgent{CLI: cli, ClaudePaneID: ""}.WaitForSessionID()
	wantText := herdr.ClaudePaneEnv + " is not set"
	require.ErrorContains(t, err, wantText)
	herdrCalls := calls()
	require.Empty(t, herdrCalls)
}
