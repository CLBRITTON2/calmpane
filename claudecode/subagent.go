package claudecode

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// RunningSubagent is a subagent an Agent tool_use of the session started, or a SendMessage tool_use resumed, that has
// not ended yet: neither a tool_result other than its background launch or its resume nor a task-notification for the
// tool_use is in the transcript.
type RunningSubagent struct {
	ToolUseID string
	// Description is the Agent tool_use's, empty for a resume, whose description is in the subagent's meta file.
	Description string
	// AgentID is the resumed subagent's, empty for an Agent start and until the SendMessage tool_result is written.
	AgentID string
	// ResumeTime is the SendMessage tool_use's timestamp, zero for an Agent start.
	ResumeTime time.Time
}

// SubagentActivity is what a running subagent's own transcript shows.
type SubagentActivity struct {
	Description string
	// StartTime is the timestamp of the subagent transcript's first record, or of the resume.
	StartTime        time.Time
	ToolCallActivity string
}

type subagentRecord struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Message   struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
	// Content is a queue-operation's text.
	Content    json.RawMessage `json:"content"`
	Attachment struct {
		Type   string          `json:"type"`
		Prompt json.RawMessage `json:"prompt"`
	} `json:"attachment"`
	// ToolUseResult is an object for an Agent or SendMessage tool_result, a string for some other tools' results.
	ToolUseResult json.RawMessage `json:"toolUseResult"`
}

type toolResultBlock struct {
	Type      string `json:"type"`
	ToolUseID string `json:"tool_use_id"`
}

// backgroundLaunchStatus is the toolUseResult status of an Agent tool_result that only reports the subagent started in
// the background.
const backgroundLaunchStatus = "async_launched"

var notifiedToolUseID = regexp.MustCompile(`<tool-use-id>([^<]+)</tool-use-id>`)

// RunningSubagentsAfterLine is running after one more line of the session transcript. Claude Code writes a finished or
// stopped background subagent's task-notification as a queue-operation first, then as a user record or a queued_command
// attachment.
func RunningSubagentsAfterLine(running []RunningSubagent, line []byte) ([]RunningSubagent, error) {
	mayStart := bytes.Contains(line, []byte(`"name":"Agent"`)) || bytes.Contains(line, []byte(`"name":"SendMessage"`))
	mayNotify := bytes.Contains(line, []byte(`<tool-use-id>`))
	mayResult := slices.ContainsFunc(running, func(subagent RunningSubagent) bool {
		return bytes.Contains(line, []byte(`"tool_use_id":"`+subagent.ToolUseID+`"`))
	})
	if !mayStart && !mayNotify && !mayResult {
		return running, nil
	}
	var record subagentRecord
	if err := json.Unmarshal(line, &record); err != nil {
		return running, fmt.Errorf("decode transcript line %.80q: %w", line, err)
	}
	switch record.Type {
	case "assistant":
		return withStartedSubagents(running, record.Message.Content, record.Timestamp)
	case "user":
		if bytes.HasPrefix(record.Message.Content, []byte(`"`)) {
			return withoutNotifiedSubagent(running, record.Message.Content)
		}
		return afterSubagentResults(running, record.Message.Content, record.ToolUseResult)
	case "attachment":
		if record.Attachment.Type != "queued_command" {
			return running, nil
		}
		return withoutNotifiedSubagent(running, record.Attachment.Prompt)
	case "queue-operation":
		return withoutNotifiedSubagent(running, record.Content)
	default:
		return running, nil
	}
}

func withStartedSubagents(running []RunningSubagent, content json.RawMessage, timestamp time.Time) ([]RunningSubagent,
	error) {
	var blocks []contentBlock
	if err := json.Unmarshal(content, &blocks); err != nil {
		return running, fmt.Errorf("decode assistant content %.80q: %w", content, err)
	}
	started := slices.Clone(running)
	for _, block := range blocks {
		if block.Type != "tool_use" {
			continue
		}
		if block.Name == "SendMessage" {
			started = append(started, RunningSubagent{ToolUseID: block.ID, ResumeTime: timestamp})
			continue
		}
		if block.Name != "Agent" {
			continue
		}
		var input toolCallInput
		if err := json.Unmarshal(block.Input, &input); err != nil {
			return running, fmt.Errorf("decode Agent input %.80q: %w", block.Input, err)
		}
		started = append(started, RunningSubagent{ToolUseID: block.ID, Description: input.Description})
	}
	return started, nil
}

// afterSubagentResults drops the subagents whose tool_result is in content, unless toolUseResult reports a background
// launch, or a resume, which names the resumed subagent.
func afterSubagentResults(running []RunningSubagent, content json.RawMessage, toolUseResult json.RawMessage) (
	[]RunningSubagent, error) {
	var subagentResult struct {
		Status         string `json:"status"`
		ResumedAgentID string `json:"resumedAgentId"`
	}
	if bytes.HasPrefix(toolUseResult, []byte(`{`)) {
		if err := json.Unmarshal(toolUseResult, &subagentResult); err != nil {
			return running, fmt.Errorf("decode toolUseResult %.80q: %w", toolUseResult, err)
		}
		if subagentResult.Status == backgroundLaunchStatus {
			return running, nil
		}
	}
	var blocks []toolResultBlock
	if err := json.Unmarshal(content, &blocks); err != nil {
		return running, fmt.Errorf("decode user content %.80q: %w", content, err)
	}
	holdsResult := func(subagent RunningSubagent) bool {
		return slices.ContainsFunc(blocks, func(block toolResultBlock) bool {
			return block.Type == "tool_result" && block.ToolUseID == subagent.ToolUseID
		})
	}
	if subagentResult.ResumedAgentID == "" {
		return slices.DeleteFunc(slices.Clone(running), holdsResult), nil
	}
	resumed := slices.Clone(running)
	for index, subagent := range resumed {
		if holdsResult(subagent) {
			resumed[index].AgentID = subagentResult.ResumedAgentID
		}
	}
	return resumed, nil
}

// withoutNotifiedSubagent drops the subagent whose tool_use a task-notification in text names.
func withoutNotifiedSubagent(running []RunningSubagent, text json.RawMessage) ([]RunningSubagent, error) {
	if !bytes.HasPrefix(text, []byte(`"`)) {
		return running, nil
	}
	var notification string
	if err := json.Unmarshal(text, &notification); err != nil {
		return running, fmt.Errorf("decode record text %.80q: %w", text, err)
	}
	if !strings.HasPrefix(notification, "<task-notification") {
		return running, nil
	}
	match := notifiedToolUseID.FindStringSubmatch(notification)
	if match == nil {
		return running, nil
	}
	return slices.DeleteFunc(slices.Clone(running), func(subagent RunningSubagent) bool {
		return subagent.ToolUseID == match[1]
	}), nil
}

// subagentTranscript follows one subagent's transcript from where it was last read.
type subagentTranscript struct {
	file        *os.File
	reader      *bufio.Reader
	partialLine []byte
	activity    SubagentActivity
}

// SubagentFollower follows the transcripts of a session's running subagents, each from its first record and then
// from its tail.
type SubagentFollower struct {
	subagentsDir string
	transcripts  map[string]*subagentTranscript
}

// NewSubagentFollower follows the subagents of the session whose transcript is at sessionTranscriptPath. Claude Code
// keeps them beside it, in <session id>\subagents.
func NewSubagentFollower(sessionTranscriptPath string) *SubagentFollower {
	sessionDir := strings.TrimSuffix(sessionTranscriptPath, filepath.Ext(sessionTranscriptPath))
	return &SubagentFollower{subagentsDir: filepath.Join(sessionDir, "subagents"),
		transcripts: map[string]*subagentTranscript{}}
}

// Activities reads what the transcripts of running gained since the last call and closes those of subagents no longer
// running. It holds each running subagent whose transcript has its first record, latest start first.
func (f *SubagentFollower) Activities(running []RunningSubagent) ([]SubagentActivity, error) {
	var errs []error
	for toolUseID, transcript := range f.transcripts {
		if !slices.ContainsFunc(running, func(subagent RunningSubagent) bool { return subagent.ToolUseID == toolUseID }) {
			errs = append(errs, transcript.file.Close())
			delete(f.transcripts, toolUseID)
		}
	}
	activities := []SubagentActivity{}
	for _, subagent := range running {
		transcript, opened := f.transcripts[subagent.ToolUseID]
		if !opened {
			var err error
			transcript, err = f.openSubagentTranscript(subagent)
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil {
				errs = append(errs, err)
				continue
			}
			f.transcripts[subagent.ToolUseID] = transcript
		}
		if err := transcript.readNewLines(); err != nil {
			errs = append(errs, fmt.Errorf("Cannot read %s: %w", transcript.file.Name(), err))
		}
		activities = append(activities, transcript.activity)
	}
	slices.SortStableFunc(activities, func(later SubagentActivity, earlier SubagentActivity) int {
		return earlier.StartTime.Compare(later.StartTime)
	})
	return activities, errors.Join(errs...)
}

func (f *SubagentFollower) Close() error {
	var errs []error
	for toolUseID, transcript := range f.transcripts {
		errs = append(errs, transcript.file.Close())
		delete(f.transcripts, toolUseID)
	}
	return errors.Join(errs...)
}

// openSubagentTranscript fails with fs.ErrNotExist until the subagent's transcript can be found and, for an Agent
// start, holds its first whole record.
func (f *SubagentFollower) openSubagentTranscript(subagent RunningSubagent) (*subagentTranscript, error) {
	if subagent.ResumeTime.IsZero() {
		return f.openStartedSubagentTranscript(subagent)
	}
	return f.openResumedSubagentTranscript(subagent)
}

// openStartedSubagentTranscript finds the subagent's transcript through the meta file Claude Code writes beside it,
// whose toolUseId is the Agent tool_use.
func (f *SubagentFollower) openStartedSubagentTranscript(subagent RunningSubagent) (*subagentTranscript, error) {
	metaPaths, err := filepath.Glob(filepath.Join(f.subagentsDir, "agent-*.meta.json"))
	if err != nil {
		return nil, fmt.Errorf("Cannot read %s: %w", f.subagentsDir, err)
	}
	for _, metaPath := range metaPaths {
		metaJSON, err := os.ReadFile(metaPath)
		if err != nil {
			return nil, fmt.Errorf("Cannot read %s: %w", metaPath, err)
		}
		var meta struct {
			ToolUseID string `json:"toolUseId"`
		}
		if err := json.Unmarshal(metaJSON, &meta); err != nil {
			return nil, fmt.Errorf("decode %s: %w", metaPath, err)
		}
		if meta.ToolUseID != subagent.ToolUseID {
			continue
		}
		path := strings.TrimSuffix(metaPath, ".meta.json") + ".jsonl"
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("Cannot read %s: %w", path, err)
		}
		startTime, err := firstRecordTime(file)
		if err != nil {
			return nil, errors.Join(fmt.Errorf("Cannot read %s: %w", path, err), file.Close())
		}
		return tailedSubagentTranscript(file, SubagentActivity{Description: subagent.Description, StartTime: startTime})
	}
	return nil, fmt.Errorf("no subagent meta file for tool_use %s in %s: %w", subagent.ToolUseID, f.subagentsDir,
		fs.ErrNotExist)
}

// openResumedSubagentTranscript opens the transcript the subagent wrote before its resume, which it goes on writing,
// with the description of its meta file.
func (f *SubagentFollower) openResumedSubagentTranscript(subagent RunningSubagent) (*subagentTranscript, error) {
	if subagent.AgentID == "" {
		return nil, fmt.Errorf("SendMessage tool_use %s has no result yet: %w", subagent.ToolUseID, fs.ErrNotExist)
	}
	metaPath := filepath.Join(f.subagentsDir, "agent-"+subagent.AgentID+".meta.json")
	metaJSON, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, fmt.Errorf("Cannot read %s: %w", metaPath, err)
	}
	var meta struct {
		Description string `json:"description"`
	}
	if err := json.Unmarshal(metaJSON, &meta); err != nil {
		return nil, fmt.Errorf("decode %s: %w", metaPath, err)
	}
	path := filepath.Join(f.subagentsDir, "agent-"+subagent.AgentID+".jsonl")
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("Cannot read %s: %w", path, err)
	}
	return tailedSubagentTranscript(file, SubagentActivity{Description: meta.Description, StartTime: subagent.ResumeTime})
}

func tailedSubagentTranscript(file *os.File, activity SubagentActivity) (*subagentTranscript, error) {
	if _, err := SeekToTail(file); err != nil {
		return nil, errors.Join(fmt.Errorf("Cannot read %s: %w", file.Name(), err), file.Close())
	}
	return &subagentTranscript{file: file, reader: bufio.NewReaderSize(file, 1024*1024), activity: activity}, nil
}

// firstRecordTime fails with fs.ErrNotExist while the file holds no whole line.
func firstRecordTime(file *os.File) (time.Time, error) {
	line, err := bufio.NewReader(file).ReadBytes('\n')
	if errors.Is(err, io.EOF) {
		return time.Time{}, fmt.Errorf("first record not written yet: %w", fs.ErrNotExist)
	}
	if err != nil {
		return time.Time{}, err
	}
	var record struct {
		Timestamp time.Time `json:"timestamp"`
	}
	if err := json.Unmarshal(line, &record); err != nil {
		return time.Time{}, fmt.Errorf("decode transcript line %.80q: %w", line, err)
	}
	return record.Timestamp, nil
}

// readNewLines keeps the newest tool call of the whole lines written since the last read, skipping those older than the
// start, which a resumed subagent's transcript holds, and keeps reading past an undecodable line, which it reports.
func (t *subagentTranscript) readNewLines() error {
	var errs []error
	for {
		line, err := t.reader.ReadBytes('\n')
		if errors.Is(err, io.EOF) {
			t.partialLine = append(t.partialLine, line...)
			return errors.Join(errs...)
		}
		if err != nil {
			return errors.Join(append(errs, err)...)
		}
		line = append(t.partialLine, line...)
		t.partialLine = nil
		toolCall, holdsToolCall, err := newestToolCall(line)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if holdsToolCall && !toolCall.timestamp.Before(t.activity.StartTime) {
			t.activity.ToolCallActivity = toolCall.activity
		}
	}
}
