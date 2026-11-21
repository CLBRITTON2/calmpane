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

	"github.com/CLBRITTON2/herdr-zen/internal/herdr"
	"github.com/CLBRITTON2/herdr-zen/internal/herdr/herdrtest"
	"github.com/CLBRITTON2/herdr-zen/internal/ui"
)

func TestHerdrAgent(t *testing.T) {
	t.Run("statusMapsHerdrAgentStatusAndUnknownForAnyOther", statusMapsHerdrAgentStatusAndUnknownForAnyOther)
	t.Run("sendPromptToBlockedClaudeReturnsAgentBlocked", sendPromptToBlockedClaudeReturnsAgentBlocked)
	t.Run("sendPromptSendNowAndInterruptReachClaudePane", sendPromptSendNowAndInterruptReachClaudePane)
	t.Run("waitForSessionIDWaitsUntilHerdrReportsClaudeSession", waitForSessionIDWaitsUntilHerdrReportsClaudeSession)
	t.Run("waitForSessionIDFailsForPaneWithoutClaudeSession", waitForSessionIDFailsForPaneWithoutClaudeSession)
	t.Run("waitForSessionIDFailsWithoutClaudePaneID", waitForSessionIDFailsWithoutClaudePaneID)
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
	cli, _ := herdrtest.Fake(t, []herdr.Pane{claudePane})
	if err := (HerdrAgent{CLI: cli, ClaudePaneID: "w1:p1"}).SendPrompt("go"); !errors.Is(err, ui.ErrAgentBlocked) {
		t.Errorf("SendPrompt error = %v, want ui.ErrAgentBlocked", err)
	}
}

func sendPromptSendNowAndInterruptReachClaudePane(t *testing.T) {
	cli, calls := herdrtest.Fake(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", true)})
	agent := HerdrAgent{CLI: cli, ClaudePaneID: "w1:p1"}
	if err := agent.SendPrompt("ship it"); err != nil {
		t.Fatal(err)
	}
	if err := agent.SendNow(); err != nil {
		t.Fatal(err)
	}
	if err := agent.Interrupt(); err != nil {
		t.Fatal(err)
	}
	want := []string{"agent prompt w1:p1 ship it", "pane send-keys w1:p1 ctrl+x ctrl+s", "pane send-keys w1:p1 esc"}
	if got := calls(); !reflect.DeepEqual(got, want) {
		t.Errorf("herdr calls = %q, want %q", got, want)
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
