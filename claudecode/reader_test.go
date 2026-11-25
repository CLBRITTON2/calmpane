package claudecode_test

import (
	"bufio"
	"os"
	"path/filepath"
	"testing"

	"github.com/CLBRITTON2/calmpane/claudecode"
	"github.com/stretchr/testify/require"
)

func TestReadSession(t *testing.T) {
	t.Run("sessionTranscriptReadsAsDialogueWithoutToolCallsOrThinking", sessionTranscriptReadsAsDialogueWithoutToolCallsOrThinking)
}

func sessionTranscriptReadsAsDialogueWithoutToolCallsOrThinking(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	const sessionID = "6da29299-87e9-4418-8fa0-53925bdf322e"
	fixturePath := filepath.Join("testdata", "session.jsonl")
	fixture, err := os.ReadFile(fixturePath)
	require.NoError(t, err)
	projectDir := filepath.Join(home, ".claude", "projects", "C--work")
	err = os.MkdirAll(projectDir, 0o755)
	require.NoError(t, err)
	transcriptPath := filepath.Join(projectDir, sessionID+".jsonl")
	err = os.WriteFile(transcriptPath, fixture, 0o644)
	require.NoError(t, err)

	conversationSessionIDs := make(chan string, 1)
	transcript, err := claudecode.WaitForTranscript(sessionID, conversationSessionIDs)
	require.NoError(t, err)
	t.Cleanup(func() {
		closeErr := transcript.Close()
		require.NoError(t, closeErr)
	})
	entries := []claudecode.DialogueEntry{}
	scanner := bufio.NewScanner(transcript)
	for scanner.Scan() {
		line := scanner.Bytes()
		lineEntries, err := claudecode.DialogueEntries(line)
		require.NoError(t, err)
		entries = append(entries, lineEntries...)
	}
	scanErr := scanner.Err()
	require.NoError(t, scanErr)

	want := []claudecode.DialogueEntry{
		{Kind: claudecode.PromptEntry, Text: "rename the widget"},
		{Kind: claudecode.ReplyEntry, Text: "Looking for the widget."},
		{Kind: claudecode.QuestionEntry, Text: "Which name?", Options: []string{"Gadget", "Gizmo"}},
		{Kind: claudecode.ReplyEntry, Text: "Renamed it to Gadget."},
	}
	require.Equal(t, want, entries)
}
