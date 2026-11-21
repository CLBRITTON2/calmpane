package claudecode

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConversationSession(t *testing.T) {
	t.Run("zenViewWaitsForFirstPromptBeforeOpeningTranscript", zenViewWaitsForFirstPromptBeforeOpeningTranscript)
	t.Run("conversationSessionIDIsSessionOfLastBackgroundJobResumeOpened", conversationSessionIDIsSessionOfLastBackgroundJobResumeOpened)
	t.Run("zenViewFollowsTranscriptAndStatusLineOfBackgroundSessionResumeOpened",
		zenViewFollowsTranscriptAndStatusLineOfBackgroundSessionResumeOpened)
	t.Run("conversationSessionIDFailsWithoutWaitingWhenJobHasNoStateFile", conversationSessionIDFailsWithoutWaitingWhenJobHasNoStateFile)
	t.Run("conversationSessionIDFailsWhenJobStateHasNoSessionID", conversationSessionIDFailsWhenJobStateHasNoSessionID)
	t.Run("lastOpenedBackgroundJobSkipsLastLineClaudeIsStillWriting", lastOpenedBackgroundJobSkipsLastLineClaudeIsStillWriting)
	t.Run("lastOpenedBackgroundJobFailsNamingUndecodableLocalCommandEntry", lastOpenedBackgroundJobFailsNamingUndecodableLocalCommandEntry)
	t.Run("conversationSessionIDIsReportedSessionWhenResumeOpenedNoBackgroundJob",
		conversationSessionIDIsReportedSessionWhenResumeOpenedNoBackgroundJob)
}

func zenViewWaitsForFirstPromptBeforeOpeningTranscript(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	const sessionID = "24e27105-227a-4078-90e7-50d3dea316bf"
	opened := make(chan *os.File, 1)
	go func() {
		transcript, _, err := WaitForTranscriptAndStatusLinePath(sessionID)
		if err != nil {
			t.Error(err)
		}
		opened <- transcript
	}()
	time.Sleep(time.Second)
	select {
	case <-opened:
		t.Fatal("opened before Claude wrote the transcript")
	default:
	}
	projectDir := filepath.Join(home, ".claude", "projects", "C--work")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	transcriptPath := filepath.Join(projectDir, sessionID+".jsonl")
	if err := os.WriteFile(transcriptPath, []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case transcript := <-opened:
		if transcript == nil || transcript.Name() != transcriptPath {
			t.Fatalf("opened %v, want %s", transcript, transcriptPath)
		}
		if err := transcript.Close(); err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("still waiting 5 s after Claude wrote the transcript")
	}
}

func writeTestTranscript(t *testing.T, home string, sessionID string, content string) {
	t.Helper()
	projectDir := filepath.Join(home, ".claude", "projects", "C--work")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, sessionID+".jsonl"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeTestJobState(t *testing.T, home string, jobID string, stateJSON string) {
	t.Helper()
	jobDir := filepath.Join(home, ".claude", "jobs", jobID)
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jobDir, "state.json"), []byte(stateJSON), 0o644); err != nil {
		t.Fatal(err)
	}
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
	got, err := ConversationSessionID(viewerSessionID)
	if err != nil {
		t.Fatal(err)
	}
	if got != jobSessionID {
		t.Errorf("ConversationSessionID = %s, want %s", got, jobSessionID)
	}
}

const resumeOpenedJobLine = `{"type":"system","subtype":"local_command","content":"<local-command-stdout>Opening \"second\", running in the background (26f6d8f1)</local-command-stdout>"}` + "\n"

func zenViewFollowsTranscriptAndStatusLineOfBackgroundSessionResumeOpened(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
	const viewerSessionID = "4d7d6a4d-cb1b-4769-8c91-14109817ff8e"
	const jobSessionID = "26f6d8f1-0a40-4e74-a315-4e535d101fd3"
	writeTestTranscript(t, home, viewerSessionID, resumeOpenedJobLine)
	writeTestJobState(t, home, "26f6d8f1", `{"sessionId":"`+jobSessionID+`"}`)
	writeTestTranscript(t, home, jobSessionID, "")
	transcript, statusLinePath, err := WaitForTranscriptAndStatusLinePath(viewerSessionID)
	if err != nil {
		t.Fatal(err)
	}
	defer transcript.Close()
	wantTranscriptPath := filepath.Join(home, ".claude", "projects", "C--work", jobSessionID+".jsonl")
	if transcript.Name() != wantTranscriptPath {
		t.Errorf("opened %s, want %s", transcript.Name(), wantTranscriptPath)
	}
	if filepath.Base(statusLinePath) != jobSessionID {
		t.Errorf("status line path = %s, want one ending in %s", statusLinePath, jobSessionID)
	}
}

func conversationSessionIDFailsWithoutWaitingWhenJobHasNoStateFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	const viewerSessionID = "4d7d6a4d-cb1b-4769-8c91-14109817ff8e"
	writeTestTranscript(t, home, viewerSessionID, resumeOpenedJobLine)
	_, err := ConversationSessionID(viewerSessionID)
	if err == nil || !strings.Contains(err.Error(), filepath.Join("jobs", "26f6d8f1", "state.json")) {
		t.Fatalf("ConversationSessionID error = %v, want one naming the job's state.json", err)
	}
	if errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ConversationSessionID error %v matches fs.ErrNotExist, which the zen view waits out", err)
	}
}

func conversationSessionIDFailsWhenJobStateHasNoSessionID(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	const viewerSessionID = "4d7d6a4d-cb1b-4769-8c91-14109817ff8e"
	writeTestTranscript(t, home, viewerSessionID, resumeOpenedJobLine)
	writeTestJobState(t, home, "26f6d8f1", `{}`)
	_, err := ConversationSessionID(viewerSessionID)
	if err == nil || !strings.Contains(err.Error(), "has no sessionId") {
		t.Fatalf("ConversationSessionID error = %v, want one saying state.json has no sessionId", err)
	}
}

func lastOpenedBackgroundJobSkipsLastLineClaudeIsStillWriting(t *testing.T) {
	transcript := resumeOpenedJobLine + `{"type":"system","subtype":"local_command","content":"<local-command-stdout>Opening`
	got, err := LastOpenedBackgroundJob(strings.NewReader(transcript))
	if err != nil {
		t.Fatal(err)
	}
	if got != "26f6d8f1" {
		t.Errorf("LastOpenedBackgroundJob = %q, want 26f6d8f1", got)
	}
}

func lastOpenedBackgroundJobFailsNamingUndecodableLocalCommandEntry(t *testing.T) {
	transcript := `{"type":"system","subtype":"local_command","content":"cut` + "\n"
	_, err := LastOpenedBackgroundJob(strings.NewReader(transcript))
	if err == nil || !strings.Contains(err.Error(), "decode local command entry") {
		t.Fatalf("LastOpenedBackgroundJob error = %v, want one naming the undecodable entry", err)
	}
}

func conversationSessionIDIsReportedSessionWhenResumeOpenedNoBackgroundJob(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	const sessionID = "6da29299-87e9-4418-8fa0-53925bdf322e"
	writeTestTranscript(t, home, sessionID, `{"type":"user","message":{"role":"user","content":"first"}}`+"\n")
	got, err := ConversationSessionID(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if got != sessionID {
		t.Errorf("ConversationSessionID = %s, want %s", got, sessionID)
	}
}
