package claudecode

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConversationSession(t *testing.T) {
	t.Run("waitForTranscriptSendsSessionBeforeFirstPromptWritesTranscript", waitForTranscriptSendsSessionBeforeFirstPromptWritesTranscript)
	t.Run("conversationSessionIDIsSessionOfLastBackgroundJobResumeOpened", conversationSessionIDIsSessionOfLastBackgroundJobResumeOpened)
	t.Run("waitForTranscriptOpensTranscriptAndSendsSessionOfBackgroundJobResumeOpened",
		waitForTranscriptOpensTranscriptAndSendsSessionOfBackgroundJobResumeOpened)
	t.Run("conversationSessionIDFailsNamingMissingJobStateFile", conversationSessionIDFailsNamingMissingJobStateFile)
	t.Run("conversationSessionIDFailsWhenJobStateHasNoSessionID", conversationSessionIDFailsWhenJobStateHasNoSessionID)
	t.Run("lastOpenedBackgroundJobSkipsLastLineClaudeIsStillWriting", lastOpenedBackgroundJobSkipsLastLineClaudeIsStillWriting)
	t.Run("lastOpenedBackgroundJobFailsNamingUndecodableLocalCommandEntry", lastOpenedBackgroundJobFailsNamingUndecodableLocalCommandEntry)
	t.Run("conversationSessionIDIsReportedSessionWhenResumeOpenedNoBackgroundJob",
		conversationSessionIDIsReportedSessionWhenResumeOpenedNoBackgroundJob)
	t.Run("processSessionIDFailsWhenSessionsFileHasNoSessionID", processSessionIDFailsWhenSessionsFileHasNoSessionID)
}

func processSessionIDFailsWhenSessionsFileHasNoSessionID(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	sessionsDir := filepath.Join(home, ".claude", "sessions")
	err := os.MkdirAll(sessionsDir, 0o755)
	require.NoError(t, err)
	sessionsFilePath := filepath.Join(sessionsDir, "4376.json")
	err = os.WriteFile(sessionsFilePath, []byte(`{"pid":4376,"status":"idle"}`), 0o644)
	require.NoError(t, err)

	_, err = ProcessSessionID(4376)
	require.EqualError(t, err, sessionsFilePath+" has no sessionId")
}

func waitForTranscriptSendsSessionBeforeFirstPromptWritesTranscript(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	const sessionID = "24e27105-227a-4078-90e7-50d3dea316bf"
	opened := make(chan *os.File, 1)
	conversationSessionIDs := make(chan string, 1)
	go func() {
		transcript, err := WaitForTranscript(sessionID, conversationSessionIDs)
		assert.NoError(t, err)
		opened <- transcript
	}()
	time.Sleep(time.Second)
	require.Empty(t, opened, "opened before Claude wrote the transcript")
	require.Len(t, conversationSessionIDs, 1, "no conversation session a second into the wait")
	conversationSessionID := <-conversationSessionIDs
	require.Equal(t, sessionID, conversationSessionID)

	projectDir := filepath.Join(home, ".claude", "projects", "C--work")
	err := os.MkdirAll(projectDir, 0o755)
	require.NoError(t, err)
	transcriptPath := filepath.Join(projectDir, sessionID+".jsonl")
	err = os.WriteFile(transcriptPath, []byte{}, 0o644)
	require.NoError(t, err)

	var transcript *os.File
	select {
	case transcript = <-opened:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "still waiting 5 s after Claude wrote the transcript")
	}
	require.NotNil(t, transcript)
	openedPath := transcript.Name()
	require.Equal(t, transcriptPath, openedPath)
	err = transcript.Close()
	require.NoError(t, err)
}

func writeTestTranscript(t *testing.T, home string, sessionID string, content string) {
	t.Helper()
	projectDir := filepath.Join(home, ".claude", "projects", "C--work")
	err := os.MkdirAll(projectDir, 0o755)
	require.NoError(t, err)
	transcriptPath := filepath.Join(projectDir, sessionID+".jsonl")
	err = os.WriteFile(transcriptPath, []byte(content), 0o644)
	require.NoError(t, err)
}

func writeTestJobState(t *testing.T, home string, jobID string, stateJSON string) {
	t.Helper()
	jobDir := filepath.Join(home, ".claude", "jobs", jobID)
	err := os.MkdirAll(jobDir, 0o755)
	require.NoError(t, err)
	statePath := filepath.Join(jobDir, "state.json")
	err = os.WriteFile(statePath, []byte(stateJSON), 0o644)
	require.NoError(t, err)
}

func conversationSessionIDIsSessionOfLastBackgroundJobResumeOpened(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	const viewerSessionID = "4d7d6a4d-cb1b-4769-8c91-14109817ff8e"
	const jobSessionID = "26f6d8f1-0a40-4e74-a315-4e535d101fd3"
	writeTestTranscript(t, home, viewerSessionID,
		`{"type":"system","subtype":"local_command","content":"<local-command-stdout>Opening \"first\", running in the background (11112222)</local-command-stdout>"}`+"\n"+
			`{"type":"system","subtype":"local_command","content":"<local-command-stdout>Opening \"second\", running in the background (26f6d8f1)</local-command-stdout>"}`+"\n"+
			`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"{\"subtype\":\"local_command\",\"content\":\"running in the background (33334444)\"}"}]}}`+"\n"+
			`{"type":"continued-in","continuedInSessionId":"87cbbea0-e025-4895-9cf0-cdca634d2499"}`+"\n")
	writeTestJobState(t, home, "26f6d8f1", `{"sessionId":"`+jobSessionID+`"}`)

	conversationSessionID, err := ConversationSessionID(viewerSessionID)
	require.NoError(t, err)
	require.Equal(t, jobSessionID, conversationSessionID)
}

const resumeOpenedJobLine = `{"type":"system","subtype":"local_command","content":"<local-command-stdout>Opening \"second\", running in the background (26f6d8f1)</local-command-stdout>"}` + "\n"

func waitForTranscriptOpensTranscriptAndSendsSessionOfBackgroundJobResumeOpened(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	const viewerSessionID = "4d7d6a4d-cb1b-4769-8c91-14109817ff8e"
	const jobSessionID = "26f6d8f1-0a40-4e74-a315-4e535d101fd3"
	writeTestTranscript(t, home, viewerSessionID, resumeOpenedJobLine)
	writeTestJobState(t, home, "26f6d8f1", `{"sessionId":"`+jobSessionID+`"}`)
	writeTestTranscript(t, home, jobSessionID, "")

	conversationSessionIDs := make(chan string, 1)
	transcript, err := WaitForTranscript(viewerSessionID, conversationSessionIDs)
	require.NoError(t, err)
	t.Cleanup(func() {
		closeErr := transcript.Close()
		require.NoError(t, closeErr)
	})
	openedPath := transcript.Name()
	wantTranscriptPath := filepath.Join(home, ".claude", "projects", "C--work", jobSessionID+".jsonl")
	require.Equal(t, wantTranscriptPath, openedPath)
	conversationSessionID := <-conversationSessionIDs
	require.Equal(t, jobSessionID, conversationSessionID)
}

func conversationSessionIDFailsNamingMissingJobStateFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	const viewerSessionID = "4d7d6a4d-cb1b-4769-8c91-14109817ff8e"
	writeTestTranscript(t, home, viewerSessionID, resumeOpenedJobLine)

	_, err := ConversationSessionID(viewerSessionID)
	jobStatePath := filepath.Join("jobs", "26f6d8f1", "state.json")
	require.ErrorContains(t, err, jobStatePath)
}

func conversationSessionIDFailsWhenJobStateHasNoSessionID(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	const viewerSessionID = "4d7d6a4d-cb1b-4769-8c91-14109817ff8e"
	writeTestTranscript(t, home, viewerSessionID, resumeOpenedJobLine)
	writeTestJobState(t, home, "26f6d8f1", `{}`)

	_, err := ConversationSessionID(viewerSessionID)
	require.ErrorContains(t, err, "has no sessionId")
}

func lastOpenedBackgroundJobSkipsLastLineClaudeIsStillWriting(t *testing.T) {
	transcript := resumeOpenedJobLine + `{"type":"system","subtype":"local_command","content":"<local-command-stdout>Opening`
	transcriptReader := strings.NewReader(transcript)

	jobID, err := lastOpenedBackgroundJob(transcriptReader)
	require.NoError(t, err)
	require.Equal(t, "26f6d8f1", jobID)
}

func lastOpenedBackgroundJobFailsNamingUndecodableLocalCommandEntry(t *testing.T) {
	transcript := `{"type":"system","subtype":"local_command","content":"cut` + "\n"
	transcriptReader := strings.NewReader(transcript)

	_, err := lastOpenedBackgroundJob(transcriptReader)
	require.ErrorContains(t, err, "decode local command entry")
}

func conversationSessionIDIsReportedSessionWhenResumeOpenedNoBackgroundJob(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	const sessionID = "6da29299-87e9-4418-8fa0-53925bdf322e"
	writeTestTranscript(t, home, sessionID, `{"type":"user","message":{"role":"user","content":"first"}}`+"\n")

	conversationSessionID, err := ConversationSessionID(sessionID)
	require.NoError(t, err)
	require.Equal(t, sessionID, conversationSessionID)
}
