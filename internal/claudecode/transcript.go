package claudecode

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

type EntryKind int

const (
	PromptEntry EntryKind = iota
	ReplyEntry
	QuestionEntry
	// CompactSummaryEntry stands for the summary Claude Code writes as a user record when it compacts the session.
	CompactSummaryEntry
)

// A QuestionEntry holds the question in Text and its option labels in Options. A PromptEntry holds in Pastes each
// paste of its Text that Claude Code shows as a label.
type DialogueEntry struct {
	Kind    EntryKind
	Text    string
	Options []string
	Pastes  []string
}

// Claude Code writes these as user records with string content, though the user did not type them.
var notPromptPrefixes = []string{
	"<system-reminder", "<task-notification", "<agent-message", "<local-command-stdout", "<local-command-caveat",
	"<command-name", "<command-message", "<bash-input", "<bash-stdout", "<bash-stderr",
}

// A tag's own line break goes with it, so the pasted text keeps only the blank lines it held.
var pastedContentTag = regexp.MustCompile(`</?pasted_content[^>]*>\n?`)

var pastedContent = regexp.MustCompile(`(?s)<pasted_content[^>]*>(.*?)</pasted_content[^>]*>`)

type transcriptRecord struct {
	Type             string    `json:"type"`
	IsMeta           bool      `json:"isMeta"`
	IsCompactSummary bool      `json:"isCompactSummary"`
	Timestamp        time.Time `json:"timestamp"`
	Message          struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
	// Claude Code writes a prompt sent while it works as an attachment of type queued_command.
	Attachment struct {
		Type        string          `json:"type"`
		Prompt      json.RawMessage `json:"prompt"`
		CommandMode string          `json:"commandMode"`
	} `json:"attachment"`
}

type QueueOperation struct {
	Operation string
	Reason    string
	// Text is the prompt as its band shows it, empty for a dequeue and a dropped_by_hook remove
	Text string
	// EntriesBefore counts the entries of the same read that the transcript holds before this record.
	EntriesBefore int
}

type queueOperationRecord struct {
	Type      string `json:"type"`
	Operation string `json:"operation"`
	Reason    string `json:"reason"`
	Content   string `json:"content"`
}

type contentBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

type toolCallInput struct {
	Description string `json:"description"`
	FilePath    string `json:"file_path"`
	Pattern     string `json:"pattern"`
}

type CurrentTurn struct {
	// PromptTime is zero while the tail holds no prompt.
	PromptTime       time.Time
	ToolCallActivity string
}

type askUserQuestionInput struct {
	Questions []struct {
		Question string `json:"question"`
		Options  []struct {
			Label string `json:"label"`
		} `json:"options"`
	} `json:"questions"`
}

// mayHoldDialogue matches substrings as Claude Code writes them, in compact JSON.
func mayHoldDialogue(line []byte) bool {
	if bytes.Contains(line, []byte(`"type":"user"`)) && bytes.Contains(line, []byte(`"role":"user","content":"`)) {
		return true
	}
	if bytes.Contains(line, []byte(`"type":"queued_command"`)) && bytes.Contains(line, []byte(`"commandMode":"prompt"`)) {
		return true
	}
	return bytes.Contains(line, []byte(`"type":"assistant"`)) &&
		(bytes.Contains(line, []byte(`"type":"text"`)) || bytes.Contains(line, []byte(`"name":"AskUserQuestion"`)))
}

func DialogueEntries(line []byte) ([]DialogueEntry, error) {
	if !mayHoldDialogue(line) {
		return nil, nil
	}
	var record transcriptRecord
	if err := json.Unmarshal(line, &record); err != nil {
		return nil, fmt.Errorf("decode transcript line %.80q: %w", line, err)
	}
	switch record.Type {
	case "user":
		if record.IsCompactSummary {
			return []DialogueEntry{{Kind: CompactSummaryEntry}}, nil
		}
		if record.IsMeta {
			return nil, nil
		}
		return promptEntries(record.Message.Content)
	case "attachment":
		if record.Attachment.Type != "queued_command" || record.Attachment.CommandMode != "prompt" {
			return nil, nil
		}
		return promptEntries(record.Attachment.Prompt)
	case "assistant":
		return assistantEntries(record.Message.Content)
	default:
		return nil, nil
	}
}

// promptEntries holds the prompt in content when it is a string the user typed, and nothing for other content.
func promptEntries(content json.RawMessage) ([]DialogueEntry, error) {
	if !bytes.HasPrefix(content, []byte(`"`)) {
		return nil, nil
	}
	var prompt string
	if err := json.Unmarshal(content, &prompt); err != nil {
		return nil, fmt.Errorf("decode user prompt %.80q: %w", content, err)
	}
	for _, prefix := range notPromptPrefixes {
		if strings.HasPrefix(prompt, prefix) {
			return nil, nil
		}
	}
	var pastes []string
	for _, match := range pastedContent.FindAllStringSubmatch(prompt, -1) {
		if paste := NormalizedPaste(match[1]); IsShownAsLabel(paste) {
			pastes = append(pastes, paste)
		}
	}
	return []DialogueEntry{{Kind: PromptEntry, Text: TypedText(prompt), Pastes: pastes}}, nil
}

func TypedText(prompt string) string {
	return strings.TrimSpace(pastedContentTag.ReplaceAllString(prompt, ""))
}

func QueueOperations(line []byte) ([]QueueOperation, error) {
	if !bytes.Contains(line, []byte(`"type":"queue-operation"`)) {
		return nil, nil
	}
	var record queueOperationRecord
	if err := json.Unmarshal(line, &record); err != nil {
		return nil, fmt.Errorf("decode transcript line %.80q: %w", line, err)
	}
	if record.Type != "queue-operation" {
		return nil, nil
	}
	return []QueueOperation{{Operation: record.Operation, Reason: record.Reason, Text: TypedText(record.Content)}}, nil
}

func assistantEntries(content json.RawMessage) ([]DialogueEntry, error) {
	var blocks []contentBlock
	if err := json.Unmarshal(content, &blocks); err != nil {
		return nil, fmt.Errorf("decode assistant content %.80q: %w", content, err)
	}
	var entries []DialogueEntry
	for _, block := range blocks {
		if block.Type == "text" {
			entries = append(entries, DialogueEntry{Kind: ReplyEntry, Text: block.Text})
		}
		if block.Type == "tool_use" && block.Name == "AskUserQuestion" {
			var input askUserQuestionInput
			if err := json.Unmarshal(block.Input, &input); err != nil {
				return nil, fmt.Errorf("decode AskUserQuestion input %.80q: %w", block.Input, err)
			}
			for _, question := range input.Questions {
				labels := []string{}
				for _, option := range question.Options {
					labels = append(labels, option.Label)
				}
				entries = append(entries, DialogueEntry{Kind: QuestionEntry, Text: question.Question, Options: labels})
			}
		}
	}
	return entries, nil
}

func TurnAfterLine(turn CurrentTurn, line []byte, lineEntries []DialogueEntry) (CurrentTurn, error) {
	holdsPrompt := slices.ContainsFunc(lineEntries, func(entry DialogueEntry) bool { return entry.Kind == PromptEntry })
	if !holdsPrompt && !bytes.Contains(line, []byte(`"type":"tool_use"`)) {
		return turn, nil
	}
	var record transcriptRecord
	if err := json.Unmarshal(line, &record); err != nil {
		return turn, fmt.Errorf("decode transcript line %.80q: %w", line, err)
	}
	// A queued_command attachment is a prompt Claude took in mid-turn, so the turn it joins keeps its time.
	if holdsPrompt && record.Type == "attachment" && !turn.PromptTime.IsZero() {
		return turn, nil
	}
	if holdsPrompt {
		return CurrentTurn{PromptTime: record.Timestamp}, nil
	}
	if record.Type != "assistant" {
		return turn, nil
	}
	var blocks []contentBlock
	if err := json.Unmarshal(record.Message.Content, &blocks); err != nil {
		return turn, fmt.Errorf("decode assistant content %.80q: %w", record.Message.Content, err)
	}
	for index := len(blocks) - 1; index >= 0; index-- {
		if blocks[index].Type != "tool_use" {
			continue
		}
		toolCallActivity, err := ToolCallActivity(blocks[index].Name, blocks[index].Input)
		if err != nil {
			return turn, err
		}
		return CurrentTurn{PromptTime: turn.PromptTime, ToolCallActivity: toolCallActivity}, nil
	}
	return turn, nil
}

func ToolCallActivity(toolName string, input json.RawMessage) (string, error) {
	var fields toolCallInput
	if err := json.Unmarshal(input, &fields); err != nil {
		return "", fmt.Errorf("decode %s input %.80q: %w", toolName, input, err)
	}
	switch toolName {
	case "Bash":
		if fields.Description == "" {
			return "Running a command", nil
		}
		return fields.Description, nil
	case "Edit", "Write":
		return "Editing " + filepath.Base(fields.FilePath), nil
	case "Read":
		return "Reading " + filepath.Base(fields.FilePath), nil
	case "Grep", "Glob":
		return "Searching for " + fields.Pattern, nil
	case "Agent":
		return fields.Description, nil
	default:
		return "", nil
	}
}

func TailStart(file io.ReaderAt, fileSize int64, tailBytes int64) (int64, error) {
	if fileSize <= tailBytes {
		return 0, nil
	}
	// The byte before the cut decides whether the line at the cut is whole.
	offset := fileSize - tailBytes - 1
	chunk := make([]byte, 64*1024)
	for offset < fileSize {
		readCount, err := file.ReadAt(chunk, offset)
		if newline := bytes.IndexByte(chunk[:readCount], '\n'); newline >= 0 {
			return offset + int64(newline) + 1, nil
		}
		offset += int64(readCount)
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("read at offset %d: %w", offset, err)
		}
	}
	return fileSize, nil
}
