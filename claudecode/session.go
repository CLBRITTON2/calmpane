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
	"strconv"
	"strings"
	"time"
)

// backgroundJobLine is the output Claude Code's /resume writes into the session of the Claude that opens a background
// session, naming the session's job by its short id.
var backgroundJobLine = regexp.MustCompile(`running in the background \(([0-9a-f]+)\)`)

// ConversationSessionID is the session whose transcript holds the conversation of a Claude whose hooks report
// reportedSessionID. A Claude that /resume pointed at a background session reports its own session, which holds no
// conversation, while the background session's job writes the conversation under the job's session.
func ConversationSessionID(reportedSessionID string) (string, error) {
	transcript, err := openTranscript(reportedSessionID)
	// Claude writes the transcript at the first prompt or /resume, so a Claude without one has opened no background
	// session yet
	if errors.Is(err, fs.ErrNotExist) {
		return reportedSessionID, nil
	}
	if err != nil {
		return "", err
	}
	jobID, err := lastOpenedBackgroundJob(transcript)
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

// lastOpenedBackgroundJob reads only the local command entries: a tool result that quotes /resume output escapes its
// quotes, so its line never holds the unescaped subtype.
func lastOpenedBackgroundJob(transcript io.Reader) (string, error) {
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

// ProcessSessionID is the session the Claude process pid writes, from the sessions file Claude Code keeps for each
// running Claude, which follows /clear and an in-place /resume. Claude has no such file during its trust dialog, nor
// once it starts exiting: the error then wraps fs.ErrNotExist.
func ProcessSessionID(pid int) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find the home directory: %w", err)
	}
	sessionsFilePath := filepath.Join(home, ".claude", "sessions", strconv.Itoa(pid)+".json")
	sessionsFileJSON, err := os.ReadFile(sessionsFilePath)
	if err != nil {
		return "", fmt.Errorf("Cannot read the sessions file of Claude process %d: %w", pid, err)
	}
	var sessionsFile struct {
		PID       int    `json:"pid"`
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(sessionsFileJSON, &sessionsFile); err != nil {
		return "", fmt.Errorf("decode %s: %w", sessionsFilePath, err)
	}
	if sessionsFile.PID != pid {
		return "", fmt.Errorf("%s holds pid %d, not Claude process %d", sessionsFilePath, sessionsFile.PID, pid)
	}
	if sessionsFile.SessionID == "" {
		return "", fmt.Errorf("%s has no sessionId", sessionsFilePath)
	}
	return sessionsFile.SessionID, nil
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

// WaitForTranscript waits for a Claude that just started, which writes the transcript of reportedSessionID at the
// first prompt. Claude runs its status line before that, so each conversation session found while waiting goes to
// conversationSessionIDs at once.
func WaitForTranscript(reportedSessionID string, conversationSessionIDs chan<- string) (*os.File, error) {
	sentSessionID := ""
	for {
		sessionID, err := ConversationSessionID(reportedSessionID)
		if err != nil {
			return nil, err
		}
		if sessionID != sentSessionID {
			conversationSessionIDs <- sessionID
			sentSessionID = sessionID
		}
		transcript, err := openTranscript(sessionID)
		if !errors.Is(err, fs.ErrNotExist) {
			return transcript, err
		}
		time.Sleep(300 * time.Millisecond)
	}
}
