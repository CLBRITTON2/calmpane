package claudecode

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSubagents(t *testing.T) {
	t.Run("backgroundSubagentRunsUntilItsTaskNotificationWithItsNewestToolCall",
		backgroundSubagentRunsUntilItsTaskNotificationWithItsNewestToolCall)
	t.Run("runningSubagentKeepsRunningAfterItsBackgroundLaunchResult", runningSubagentKeepsRunningAfterItsBackgroundLaunchResult)
	t.Run("runningSubagentEndsOnItsForegroundResult", runningSubagentEndsOnItsForegroundResult)
	t.Run("runningSubagentEndsOnItsInterruptedForegroundResult", runningSubagentEndsOnItsInterruptedForegroundResult)
	t.Run("runningSubagentKeepsRunningAfterAnotherToolsResult", runningSubagentKeepsRunningAfterAnotherToolsResult)
	t.Run("runningSubagentEndsOnItsQueuedTaskNotification", runningSubagentEndsOnItsQueuedTaskNotification)
	t.Run("runningSubagentEndsOnItsTaskNotificationAsUserRecord", runningSubagentEndsOnItsTaskNotificationAsUserRecord)
	t.Run("runningSubagentEndsOnItsTaskNotificationTakenMidTurn", runningSubagentEndsOnItsTaskNotificationTakenMidTurn)
	t.Run("runningSubagentKeepsRunningAfterTaskNotificationOfAnotherToolUse",
		runningSubagentKeepsRunningAfterTaskNotificationOfAnotherToolUse)
	t.Run("activitiesFollowLinesWrittenSinceLastReadAndDropEndedSubagent",
		activitiesFollowLinesWrittenSinceLastReadAndDropEndedSubagent)
	t.Run("activitiesSkipSubagentUntilItsMetaFileIsWritten", activitiesSkipSubagentUntilItsMetaFileIsWritten)
	t.Run("sendMessageResumeRunsUntilItsTaskNotificationButAMessageWithoutResumeEnds",
		sendMessageResumeRunsUntilItsTaskNotificationButAMessageWithoutResumeEnds)
	t.Run("resumedSubagentTimesFromResumeAndShowsOnlyToolCallsSinceIt",
		resumedSubagentTimesFromResumeAndShowsOnlyToolCallsSinceIt)
}

func sendMessageResumeRunsUntilItsTaskNotificationButAMessageWithoutResumeEnds(t *testing.T) {
	sent := []byte(`{"type":"assistant","timestamp":"2026-11-25T11:00:00.000Z","message":{"role":"assistant","content":[` +
		`{"type":"tool_use","id":"toolu_send","name":"SendMessage","input":{"to":"a1","message":"again"}}]}}`)
	resumed := []byte(`{"type":"user","message":{"role":"user","content":[{"tool_use_id":"toolu_send","type":"tool_result",` +
		`"content":[{"type":"text","text":"resuming"}]}]},"toolUseResult":{"success":true,"resumedAgentId":"a1"}}`)
	delivered := []byte(`{"type":"user","message":{"role":"user","content":[{"tool_use_id":"toolu_send","type":"tool_result",` +
		`"content":[{"type":"text","text":"delivered"}]}]},"toolUseResult":{"success":true}}`)
	notified := []byte(`{"type":"queue-operation","operation":"enqueue","content":"<task-notification>\n<task-id>a1</task-id>\n` +
		`<tool-use-id>toolu_send</tool-use-id>\n<status>completed</status>\n</task-notification>"}`)
	resumeTime := time.Date(2026, 11, 25, 11, 0, 0, 0, time.UTC)
	sentSubagent := []RunningSubagent{{ToolUseID: "toolu_send", ResumeTime: resumeTime}}

	running, err := RunningSubagentsAfterLine([]RunningSubagent{}, sent)
	require.NoError(t, err)
	require.Equal(t, sentSubagent, running)

	running, err = RunningSubagentsAfterLine(running, resumed)
	require.NoError(t, err)
	resumedSubagent := []RunningSubagent{{ToolUseID: "toolu_send", AgentID: "a1", ResumeTime: resumeTime}}
	require.Equal(t, resumedSubagent, running)

	running, err = RunningSubagentsAfterLine(running, notified)
	require.NoError(t, err)
	require.Equal(t, []RunningSubagent{}, running)

	running, err = RunningSubagentsAfterLine(running, sent)
	require.NoError(t, err)
	require.Equal(t, sentSubagent, running)

	running, err = RunningSubagentsAfterLine(running, delivered)
	require.NoError(t, err)
	require.Equal(t, []RunningSubagent{}, running)
}

func resumedSubagentTimesFromResumeAndShowsOnlyToolCallsSinceIt(t *testing.T) {
	sessionPath := sessionCopy(t)
	follower := NewSubagentFollower(sessionPath)
	t.Cleanup(func() {
		closeErr := follower.Close()
		require.NoError(t, closeErr)
	})
	resumeTime := time.Date(2026, 11, 25, 11, 0, 0, 0, time.UTC)
	sentBeforeResult := []RunningSubagent{{ToolUseID: "toolu_send", ResumeTime: resumeTime}}

	activities, err := follower.Activities(sentBeforeResult)
	require.NoError(t, err)
	require.Empty(t, activities)

	running := []RunningSubagent{{ToolUseID: "toolu_send", AgentID: "a1", ResumeTime: resumeTime}}
	activities, err = follower.Activities(running)
	require.NoError(t, err)
	want := []SubagentActivity{{Description: "Review the diff", StartTime: resumeTime}}
	require.Equal(t, want, activities)

	sessionDir := filepath.Dir(sessionPath)
	transcriptPath := filepath.Join(sessionDir, "subagent-session", "subagents", "agent-a1.jsonl")
	appendToFile(t, transcriptPath, `{"type":"assistant","timestamp":"2026-11-25T11:00:02.000Z","message":{"role":"assistant",`+
		`"content":[{"type":"tool_use","id":"t20","name":"Bash","input":{"command":"go vet ./...","description":"Vetting"}}]}}`+"\n")
	activities, err = follower.Activities(running)
	require.NoError(t, err)
	want = []SubagentActivity{{Description: "Review the diff", StartTime: resumeTime, ToolCallActivity: "Vetting"}}
	require.Equal(t, want, activities)
}

func backgroundSubagentRunsUntilItsTaskNotificationWithItsNewestToolCall(t *testing.T) {
	sessionPath := filepath.Join("testdata", "subagent-session.jsonl")

	running := runningSubagentsOfTranscript(t, sessionPath)
	wantRunning := []RunningSubagent{{ToolUseID: "toolu_tests", Description: "Run the tests"}}
	require.Equal(t, wantRunning, running)

	follower := NewSubagentFollower(sessionPath)
	t.Cleanup(func() {
		closeErr := follower.Close()
		require.NoError(t, closeErr)
	})
	activities, err := follower.Activities(running)
	require.NoError(t, err)
	startTime := time.Date(2026, 11, 25, 10, 0, 6, 100_000_000, time.UTC)
	want := []SubagentActivity{{Description: "Run the tests", StartTime: startTime, ToolCallActivity: "Running go test"}}
	require.Equal(t, want, activities)
}

const killedTaskNotification = `<task-notification>\n<task-id>a1</task-id>\n<tool-use-id>toolu_1</tool-use-id>\n` +
	`<status>killed</status>\n</task-notification>`

func runningSubagentsAfterAgentCallThen(t *testing.T, line string) []RunningSubagent {
	t.Helper()
	agentCall := []byte(`{"type":"assistant","message":{"role":"assistant","content":[` +
		`{"type":"tool_use","id":"toolu_1","name":"Agent","input":{"description":"Review the diff","prompt":"x"}}]}}`)
	running, err := RunningSubagentsAfterLine([]RunningSubagent{}, agentCall)
	require.NoError(t, err)
	require.Len(t, running, 1)
	running, err = RunningSubagentsAfterLine(running, []byte(line))
	require.NoError(t, err)
	return running
}

func runningSubagentKeepsRunningAfterItsBackgroundLaunchResult(t *testing.T) {
	running := runningSubagentsAfterAgentCallThen(t,
		`{"type":"user","message":{"role":"user","content":[{"tool_use_id":"toolu_1","type":"tool_result",`+
			`"content":[{"type":"text","text":"launched"}]}]},"toolUseResult":{"isAsync":true,"status":"async_launched"}}`)
	require.Len(t, running, 1)
}

func runningSubagentEndsOnItsForegroundResult(t *testing.T) {
	running := runningSubagentsAfterAgentCallThen(t,
		`{"type":"user","message":{"role":"user","content":[{"tool_use_id":"toolu_1","type":"tool_result",`+
			`"content":[{"type":"text","text":"report"}]}]},"toolUseResult":{"status":"completed"}}`)
	require.Empty(t, running)
}

func runningSubagentEndsOnItsInterruptedForegroundResult(t *testing.T) {
	running := runningSubagentsAfterAgentCallThen(t,
		`{"type":"user","message":{"role":"user","content":[{"tool_use_id":"toolu_1","type":"tool_result",`+
			`"content":"interrupted","is_error":true}]},"toolUseResult":"Error: interrupted"}`)
	require.Empty(t, running)
}

func runningSubagentKeepsRunningAfterAnotherToolsResult(t *testing.T) {
	running := runningSubagentsAfterAgentCallThen(t,
		`{"type":"user","message":{"role":"user","content":[{"tool_use_id":"toolu_2","type":"tool_result",`+
			`"content":"ok"}]},"toolUseResult":{"stdout":"ok"}}`)
	require.Len(t, running, 1)
}

func runningSubagentEndsOnItsQueuedTaskNotification(t *testing.T) {
	running := runningSubagentsAfterAgentCallThen(t,
		`{"type":"queue-operation","operation":"enqueue","content":"`+killedTaskNotification+`"}`)
	require.Empty(t, running)
}

func runningSubagentEndsOnItsTaskNotificationAsUserRecord(t *testing.T) {
	running := runningSubagentsAfterAgentCallThen(t,
		`{"type":"user","message":{"role":"user","content":"`+killedTaskNotification+`"}}`)
	require.Empty(t, running)
}

func runningSubagentEndsOnItsTaskNotificationTakenMidTurn(t *testing.T) {
	running := runningSubagentsAfterAgentCallThen(t,
		`{"type":"attachment","attachment":{"type":"queued_command","prompt":"`+killedTaskNotification+`",`+
			`"commandMode":"task-notification"}}`)
	require.Empty(t, running)
}

func runningSubagentKeepsRunningAfterTaskNotificationOfAnotherToolUse(t *testing.T) {
	running := runningSubagentsAfterAgentCallThen(t,
		`{"type":"queue-operation","operation":"enqueue","content":"<task-notification>\n<task-id>b1</task-id>\n`+
			`<tool-use-id>toolu_9</tool-use-id>\n</task-notification>"}`)
	require.Len(t, running, 1)
}

func activitiesFollowLinesWrittenSinceLastReadAndDropEndedSubagent(t *testing.T) {
	sessionPath := sessionCopy(t)
	follower := NewSubagentFollower(sessionPath)
	t.Cleanup(func() {
		closeErr := follower.Close()
		require.NoError(t, closeErr)
	})
	running := []RunningSubagent{{ToolUseID: "toolu_tests", Description: "Run the tests"},
		{ToolUseID: "toolu_review", Description: "Review the diff"}}
	_, err := follower.Activities(running)
	require.NoError(t, err)

	sessionDir := filepath.Dir(sessionPath)
	transcriptPath := filepath.Join(sessionDir, "subagent-session", "subagents", "agent-a2.jsonl")
	appendToFile(t, transcriptPath, `{"type":"assistant","timestamp":"2026-11-25T10:00:20.000Z","message":{"role":"assistant",`)
	activities, err := follower.Activities(running)
	require.NoError(t, err)
	require.Len(t, activities, 2)
	testsActivity := activities[0]
	require.Equal(t, "Running go test", testsActivity.ToolCallActivity)

	appendToFile(t, transcriptPath, `"content":[{"type":"text","text":"x"}]}}`+"\n"+`{"type":"assistant","message":{"content":[{"type":"tool_use",`+"\n"+
		`{"type":"assistant","timestamp":"2026-11-25T10:00:21.000Z","message":{"role":"assistant","content":[`+
		`{"type":"tool_use","id":"t9","name":"Grep","input":{"pattern":"TODO"}}]}}`+"\n")
	testsOnly := running[:1]
	activities, err = follower.Activities(testsOnly)
	require.Error(t, err)
	errorText := err.Error()
	namesTranscript := strings.HasPrefix(errorText, "Cannot read "+transcriptPath)
	require.True(t, namesTranscript, errorText)
	startTime := time.Date(2026, 11, 25, 10, 0, 6, 100_000_000, time.UTC)
	want := []SubagentActivity{{Description: "Run the tests", StartTime: startTime, ToolCallActivity: "Searching for TODO"}}
	require.Equal(t, want, activities)
	require.Len(t, follower.transcripts, 1)
}

func activitiesSkipSubagentUntilItsMetaFileIsWritten(t *testing.T) {
	sessionPath := sessionCopy(t)
	sessionDir := filepath.Dir(sessionPath)
	metaPath := filepath.Join(sessionDir, "subagent-session", "subagents", "agent-a2.meta.json")
	meta, err := os.ReadFile(metaPath)
	require.NoError(t, err)
	err = os.Remove(metaPath)
	require.NoError(t, err)
	follower := NewSubagentFollower(sessionPath)
	t.Cleanup(func() {
		closeErr := follower.Close()
		require.NoError(t, closeErr)
	})
	running := []RunningSubagent{{ToolUseID: "toolu_tests", Description: "Run the tests"}}

	activities, err := follower.Activities(running)
	require.NoError(t, err)
	require.Empty(t, activities)

	err = os.WriteFile(metaPath, meta, 0o600)
	require.NoError(t, err)
	activities, err = follower.Activities(running)
	require.NoError(t, err)
	require.Len(t, activities, 1)
	testsActivity := activities[0]
	require.Equal(t, "Running go test", testsActivity.ToolCallActivity)
}

func runningSubagentsOfTranscript(t *testing.T, path string) []RunningSubagent {
	t.Helper()
	transcript, err := os.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() {
		closeErr := transcript.Close()
		require.NoError(t, closeErr)
	})
	running := []RunningSubagent{}
	scanner := bufio.NewScanner(transcript)
	for scanner.Scan() {
		line := scanner.Bytes()
		running, err = RunningSubagentsAfterLine(running, line)
		require.NoError(t, err)
	}
	scanErr := scanner.Err()
	require.NoError(t, scanErr)
	return running
}

// sessionCopy copies the testdata subagent session into a temp dir and returns its transcript's path there.
func sessionCopy(t *testing.T) string {
	t.Helper()
	destination := t.TempDir()
	testdata := os.DirFS("testdata")
	err := os.CopyFS(destination, testdata)
	require.NoError(t, err)
	return filepath.Join(destination, "subagent-session.jsonl")
}

func appendToFile(t *testing.T, path string, text string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	require.NoError(t, err)
	_, err = file.WriteString(text)
	require.NoError(t, err)
	err = file.Close()
	require.NoError(t, err)
}
