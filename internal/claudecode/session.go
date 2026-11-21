package claudecode

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// backgroundJobLine is the output Claude Code's /resume writes into the session of the Claude that opens a background
// session, naming the session's job by its short id.
var backgroundJobLine = regexp.MustCompile(`running in the background \(([0-9a-f]+)\)`)

// ConversationSessionID is the session whose transcript holds the conversation shown in a pane that herdr reports as
// reportedSessionID. A Claude that /resume pointed at a background session reports its own session to herdr, which
// holds no conversation, while the background session's job writes the conversation under the job's session.
func ConversationSessionID(reportedSessionID string) (string, error) {
	transcript, err := openTranscript(reportedSessionID)
	if err != nil {
		return "", err
	}
	jobID, err := LastOpenedBackgroundJob(transcript)
	if err := errors.Join(err, transcript.Close()); err != nil {
		return "", fmt.Errorf("Cannot read %s: %w", transcript.Name(), err)
	}
	if jobID == "" {
		return reportedSessionID, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find the home directory: %w", err)
	}
	statePath := filepath.Join(home, ".claude", "jobs", jobID, "state.json")
	stateJSON, err := os.ReadFile(statePath)
	if err != nil {
		// %v, not %w: a missing state.json must show, not match fs.ErrNotExist, which the zen view waits out
		return "", fmt.Errorf("Cannot read %s: %v", statePath, err)
	}
	var state struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(stateJSON, &state); err != nil {
		return "", fmt.Errorf("decode %s: %w", statePath, err)
	}
	if state.SessionID == "" {
		return "", fmt.Errorf("%s has no sessionId", statePath)
	}
	return state.SessionID, nil
}

// LastOpenedBackgroundJob reads only the local command entries: a tool result that quotes /resume output escapes its
// quotes, so its line never holds the unescaped subtype.
func LastOpenedBackgroundJob(transcript io.Reader) (string, error) {
	reader := bufio.NewReader(transcript)
	jobID := ""
	for {
		line, readErr := reader.ReadBytes('\n')
		// Claude may still be writing the last line, which ends at EOF without its newline
		if errors.Is(readErr, io.EOF) {
			return jobID, nil
		}
		if readErr != nil {
			return "", readErr
		}
		if strings.Contains(string(line), `"subtype":"local_command"`) {
			var entry struct {
				Content string `json:"content"`
			}
			if err := json.Unmarshal(line, &entry); err != nil {
				return "", fmt.Errorf("decode local command entry %q: %w", line, err)
			}
			if match := backgroundJobLine.FindStringSubmatch(entry.Content); match != nil {
				jobID = match[1]
			}
		}
	}
}

func openTranscript(sessionID string) (*os.File, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("find the home directory: %w", err)
	}
	pattern := filepath.Join(home, ".claude", "projects", "*", sessionID+".jsonl")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("Cannot read %s: %w", pattern, err)
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("Cannot read %s: %w", pattern, fs.ErrNotExist)
	}
	transcript, err := os.Open(matches[0])
	if err != nil {
		return nil, fmt.Errorf("Cannot read %s: %w", matches[0], err)
	}
	return transcript, nil
}

// WaitForTranscriptAndStatusLinePath waits for a Claude that just started, which writes the transcript of
// reportedSessionID at the first prompt.
func WaitForTranscriptAndStatusLinePath(reportedSessionID string) (*os.File, string, error) {
	for {
		transcript, statusLinePath, err := openClaudeTranscriptAndStatusLinePath(reportedSessionID)
		if !errors.Is(err, fs.ErrNotExist) {
			return transcript, statusLinePath, err
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func openClaudeTranscriptAndStatusLinePath(reportedSessionID string) (*os.File, string, error) {
	sessionID, err := ConversationSessionID(reportedSessionID)
	if err != nil {
		return nil, "", err
	}
	transcript, err := openTranscript(sessionID)
	if err != nil {
		return nil, "", err
	}
	localAppData, err := os.UserCacheDir()
	if err != nil {
		return nil, "", errors.Join(fmt.Errorf("find the local app data directory: %w", err), transcript.Close())
	}
	// windows-dev's claude/statusline.ps1 writes it
	statusLinePath := filepath.Join(localAppData, "windows-dev", "statusline", sessionID)
	return transcript, statusLinePath, nil
}
