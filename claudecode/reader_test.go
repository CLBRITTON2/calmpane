package claudecode_test

import (
	"bufio"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/CLBRITTON2/calmpane/claudecode"
)

func TestReadSession(t *testing.T) {
	t.Run("sessionTranscriptReadsAsDialogueWithoutToolCallsOrThinking", sessionTranscriptReadsAsDialogueWithoutToolCallsOrThinking)
}

func sessionTranscriptReadsAsDialogueWithoutToolCallsOrThinking(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	const sessionID = "6da29299-87e9-4418-8fa0-53925bdf322e"
	fixture, err := os.ReadFile(filepath.Join("testdata", "session.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	projectDir := filepath.Join(home, ".claude", "projects", "C--work")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, sessionID+".jsonl"), fixture, 0o644); err != nil {
		t.Fatal(err)
	}
	conversationSessionIDs := make(chan string, 1)
	transcript, err := claudecode.WaitForTranscript(sessionID, conversationSessionIDs)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := transcript.Close(); err != nil {
			t.Error(err)
		}
	})
	entries := []claudecode.DialogueEntry{}
	scanner := bufio.NewScanner(transcript)
	for scanner.Scan() {
		lineEntries, err := claudecode.DialogueEntries(scanner.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, lineEntries...)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	want := []claudecode.DialogueEntry{
		{Kind: claudecode.PromptEntry, Text: "rename the widget"},
		{Kind: claudecode.ReplyEntry, Text: "Looking for the widget."},
		{Kind: claudecode.QuestionEntry, Text: "Which name?", Options: []string{"Gadget", "Gizmo"}},
		{Kind: claudecode.ReplyEntry, Text: "Renamed it to Gadget."},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Errorf("entries = %+v, want %+v", entries, want)
	}
}
