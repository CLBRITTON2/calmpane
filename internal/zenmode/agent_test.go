package zenmode

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/CLBRITTON2/calmpane/internal/herdr"
	"github.com/CLBRITTON2/calmpane/internal/herdr/herdrtest"
	"github.com/CLBRITTON2/calmpane/internal/ui"
)

func TestHerdrAgent(t *testing.T) {
	t.Run("statusMapsHerdrAgentStatusAndUnknownForAnyOther", statusMapsHerdrAgentStatusAndUnknownForAnyOther)
	t.Run("sendPromptToBlockedClaudeReturnsAgentBlocked", sendPromptToBlockedClaudeReturnsAgentBlocked)
	t.Run("sendPromptSendNowAndInterruptReachClaudePane", sendPromptSendNowAndInterruptReachClaudePane)
	t.Run("sendPromptSendsPromptLongerThanWindowsCommandLineWhole", sendPromptSendsPromptLongerThanWindowsCommandLineWhole)
	t.Run("sendPromptRefusedByHerdrFailsWithItsErrorAndWithoutPrompt", sendPromptRefusedByHerdrFailsWithItsErrorAndWithoutPrompt)
	t.Run("sendPromptWithoutSocketPathFailsNamingHerdrSocketPath", sendPromptWithoutSocketPathFailsNamingHerdrSocketPath)
	t.Run("waitForSessionIDWaitsUntilHerdrReportsClaudeSession", waitForSessionIDWaitsUntilHerdrReportsClaudeSession)
	t.Run("waitForSessionIDFailsForPaneWithoutClaudeSession", waitForSessionIDFailsForPaneWithoutClaudeSession)
	t.Run("waitForSessionIDFailsWithoutClaudePaneID", waitForSessionIDFailsWithoutClaudePaneID)
	t.Run("inputBoxShownOnlyWhileClaudePaneShowsInputBox", inputBoxShownOnlyWhileClaudePaneShowsInputBox)
	t.Run("workingDirectoryIsClaudePaneCwd", workingDirectoryIsClaudePaneCwd)
	t.Run("workingDirectoryFailsForPaneWithoutCwd", workingDirectoryFailsForPaneWithoutCwd)
}

func inputBoxShownOnlyWhileClaudePaneShowsInputBox(t *testing.T) {
	cli, _ := herdrtest.Fake(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", true)})
	for capture, want := range map[string]bool{"claude-input-empty.ansi": true, "claude-input-scrolled.ansi": false} {
		herdrtest.ShowOnScreen(t, "w1:p1", capture, "")
		got, err := HerdrAgent{CLI: cli, ClaudePaneID: "w1:p1"}.InputBoxShown()
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("InputBoxShown with %s on screen = %t, want %t", capture, got, want)
		}
	}
}

func workingDirectoryIsClaudePaneCwd(t *testing.T) {
	claudePane := herdrtest.ClaudePane("w1:p1", true)
	claudePane.Cwd = `C:\work\project\`
	cli, _ := herdrtest.Fake(t, []herdr.Pane{claudePane})
	got, err := HerdrAgent{CLI: cli, ClaudePaneID: "w1:p1"}.WorkingDirectory()
	if err != nil {
		t.Fatal(err)
	}
	if got != claudePane.Cwd {
		t.Errorf("WorkingDirectory = %q, want %q", got, claudePane.Cwd)
	}
}

func workingDirectoryFailsForPaneWithoutCwd(t *testing.T) {
	cli, _ := herdrtest.Fake(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", true)})
	_, err := HerdrAgent{CLI: cli, ClaudePaneID: "w1:p1"}.WorkingDirectory()
	want := "herdr reports no working directory for pane w1:p1"
	if err == nil || err.Error() != want {
		t.Errorf("WorkingDirectory error = %v, want %q", err, want)
	}
}

func statusMapsHerdrAgentStatusAndUnknownForAnyOther(t *testing.T) {
	for herdrStatus, want := range map[string]ui.AgentStatus{
		"working": ui.AgentWorking, "idle": ui.AgentIdle, "done": ui.AgentDone, "blocked": ui.AgentBlocked,
		"unknown": ui.AgentUnknown,
	} {
		t.Run(herdrStatus, func(t *testing.T) {
			claudePane := herdrtest.ClaudePane("w1:p1", true)
			claudePane.AgentStatus = herdrStatus
			cli, _ := herdrtest.Fake(t, []herdr.Pane{claudePane})
			got, err := HerdrAgent{CLI: cli, ClaudePaneID: "w1:p1"}.Status()
			if err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Errorf("Status = %d, want %d", got, want)
			}
		})
	}
}

func sendPromptToBlockedClaudeReturnsAgentBlocked(t *testing.T) {
	claudePane := herdrtest.ClaudePane("w1:p1", true)
	claudePane.AgentStatus = "blocked"
	api, _ := herdrtest.FakeAPI(t, []herdr.Pane{claudePane})
	if err := (HerdrAgent{API: api, ClaudePaneID: "w1:p1"}).SendPrompt("go"); !errors.Is(err, ui.ErrAgentBlocked) {
		t.Errorf("SendPrompt error = %v, want ui.ErrAgentBlocked", err)
	}
}

func sendPromptSendNowAndInterruptReachClaudePane(t *testing.T) {
	panes := []herdr.Pane{herdrtest.ClaudePane("w1:p1", true)}
	cli, calls := herdrtest.Fake(t, panes)
	api, requests := herdrtest.FakeAPI(t, panes)
	agent := HerdrAgent{CLI: cli, API: api, ClaudePaneID: "w1:p1"}
	if err := agent.SendPrompt("ship it"); err != nil {
		t.Fatal(err)
	}
	if err := agent.SendNow(); err != nil {
		t.Fatal(err)
	}
	if err := agent.Interrupt(); err != nil {
		t.Fatal(err)
	}
	if got, want := requests(), []string{"agent.prompt w1:p1 ship it"}; !reflect.DeepEqual(got, want) {
		t.Errorf("herdr API requests = %q, want %q", got, want)
	}
	want := []string{"pane send-keys w1:p1 ctrl+x ctrl+s", "pane send-keys w1:p1 esc"}
	if got := calls(); !reflect.DeepEqual(got, want) {
		t.Errorf("herdr calls = %q, want %q", got, want)
	}
}

func sendPromptRefusedByHerdrFailsWithItsErrorAndWithoutPrompt(t *testing.T) {
	api, _ := herdrtest.FakeAPI(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", true)})
	err := HerdrAgent{API: api, ClaudePaneID: "w1:p9"}.SendPrompt("secret prompt text")
	want := "herdr agent.prompt failed: pane_not_found: pane w1:p9 not found"
	if err == nil || err.Error() != want {
		t.Errorf("SendPrompt error = %v, want %q", err, want)
	}
}

func sendPromptWithoutSocketPathFailsNamingHerdrSocketPath(t *testing.T) {
	err := HerdrAgent{API: herdr.API{}, ClaudePaneID: "w1:p1"}.SendPrompt("go")
	want := "herdr agent.prompt failed: HERDR_SOCKET_PATH is not set"
	if err == nil || err.Error() != want {
		t.Errorf("SendPrompt error = %v, want %q", err, want)
	}
}

func sendPromptSendsPromptLongerThanWindowsCommandLineWhole(t *testing.T) {
	api, requests := herdrtest.FakeAPI(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", true)})
	prompt := strings.Repeat("a line of the pasted log\n", 1600)
	if err := (HerdrAgent{API: api, ClaudePaneID: "w1:p1"}).SendPrompt(prompt); err != nil {
		t.Fatal(err)
	}
	if got := requests(); len(got) != 1 || got[0] != "agent.prompt w1:p1 "+prompt {
		t.Errorf("herdr API requests = %.80q, want one agent.prompt of the %d character prompt", got, len(prompt))
	}
}

func waitForSessionIDWaitsUntilHerdrReportsClaudeSession(t *testing.T) {
	cli, _ := herdrtest.Fake(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", true)})
	reported := make(chan string, 1)
	go func() {
		sessionID, err := HerdrAgent{CLI: cli, ClaudePaneID: "w1:p1"}.WaitForSessionID()
		if err != nil {
			t.Error(err)
		}
		reported <- sessionID
	}()
	time.Sleep(time.Second)
	select {
	case sessionID := <-reported:
		t.Fatalf("returned session %q before herdr reported one", sessionID)
	default:
	}
	const sessionID = "24e27105-227a-4078-90e7-50d3dea316bf"
	startedPane := herdrtest.ClaudePane("w1:p1", true)
	startedPane.AgentSession = &struct {
		Agent string `json:"agent"`
		Value string `json:"value"`
	}{Agent: "claude", Value: sessionID}
	panesJSON, err := json.Marshal([]herdr.Pane{startedPane})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(os.Getenv(herdrtest.FakeDirEnv), "panes.json"), panesJSON, 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-reported:
		if got != sessionID {
			t.Errorf("WaitForSessionID = %q, want %q", got, sessionID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("still waiting 5 s after herdr reported the session")
	}
}

func waitForSessionIDFailsForPaneWithoutClaudeSession(t *testing.T) {
	cli, _ := herdrtest.Fake(t, []herdr.Pane{{PaneID: "w1:p1", Focused: true}})
	_, err := HerdrAgent{CLI: cli, ClaudePaneID: "w1:p1"}.WaitForSessionID()
	if err == nil || err.Error() != "No Claude session in this pane." {
		t.Errorf("WaitForSessionID error = %v, want No Claude session in this pane.", err)
	}
}

func waitForSessionIDFailsWithoutClaudePaneID(t *testing.T) {
	cli, calls := herdrtest.Fake(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", true)})
	_, err := HerdrAgent{CLI: cli, ClaudePaneID: ""}.WaitForSessionID()
	if err == nil || !strings.Contains(err.Error(), herdr.ClaudePaneEnv+" is not set") {
		t.Errorf("WaitForSessionID error = %v, want one naming %s", err, herdr.ClaudePaneEnv)
	}
	if got := calls(); len(got) != 0 {
		t.Errorf("herdr calls = %q, want none", got)
	}
}
