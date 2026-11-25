package claudecode

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
)

type EntryKind int

const (
	PromptEntry EntryKind = iota
	ReplyEntry
	QuestionEntry
	// CompactSummaryEntry stands for the summary Claude Code writes as a user record when it compacts the session.
	CompactSummaryEntry
	// CommandEntry holds a slash command the user ran as its name and arguments, "/effort high".
	CommandEntry
	// CommandOutputEntry holds the text a slash command wrote, without its terminal color codes.
	CommandOutputEntry
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
	"<system-reminder", "<task-notification", "<agent-message", "<local-command-caveat", "<bash-input", "<bash-stdout",
	"<bash-stderr",
}

// Claude Code writes a slash command the user ran as these tags, in a user record or a local_command system record:
// <command-name>, <command-message> and <command-args> for the command, <local-command-stdout> for its output.
var (
	commandName   = regexp.MustCompile(`(?s)<command-name>(.*?)</command-name>`)
	commandArgs   = regexp.MustCompile(`(?s)<command-args>(.*?)</command-args>`)
	commandOutput = regexp.MustCompile(`(?s)^<local-command-stdout>(.*)</local-command-stdout>`)
)

// clearCommand opens the transcript of the session it starts, which Claude's own view shows empty.
const clearCommand = "/clear"

// A tag's own line break goes with it, so the pasted text keeps only the blank lines it held.
var pastedContentTag = regexp.MustCompile(`</?pasted_content[^>]*>\n?`)

var pastedContent = regexp.MustCompile(`(?s)<pasted_content[^>]*>(.*?)</pasted_content[^>]*>`)

type transcriptRecord struct {
	Type             string    `json:"type"`
	Subtype          string    `json:"subtype"`
	IsMeta           bool      `json:"isMeta"`
	IsCompactSummary bool      `json:"isCompactSummary"`
	Timestamp        time.Time `json:"timestamp"`
	Message          struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
	// Content is a system record's text.
	Content json.RawMessage `json:"content"`
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
}

type queueOperationRecord struct {
	Type      string `json:"type"`
	Operation string `json:"operation"`
	Reason    string `json:"reason"`
	Content   string `json:"content"`
}

type contentBlock struct {
	Type  string          `json:"type"`
	ID    string          `json:"id"`
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
	if bytes.Contains(line, []byte(`"type":"system"`)) && bytes.Contains(line, []byte(`"subtype":"local_command"`)) {
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
	case "system":
		if record.Subtype != "local_command" {
			return nil, nil
		}
		var content string
		if err := json.Unmarshal(record.Content, &content); err != nil {
			return nil, fmt.Errorf("decode local command content %.80q: %w", record.Content, err)
		}
		return commandEntries(content), nil
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
	if strings.HasPrefix(prompt, "<command-") || strings.HasPrefix(prompt, "<local-command-stdout") {
		return commandEntries(prompt), nil
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
	return []DialogueEntry{{Kind: PromptEntry, Text: typedText(prompt), Pastes: pastes}}, nil
}

// commandEntries holds the command or the output in a record's text, none for an empty output or the /clear that
// opens a session.
func commandEntries(text string) []DialogueEntry {
	if output := commandOutput.FindStringSubmatch(text); output != nil {
		plainOutput := strings.TrimSpace(ansi.Strip(output[1]))
		if plainOutput == "" {
			return nil
		}
		return []DialogueEntry{{Kind: CommandOutputEntry, Text: plainOutput}}
	}
	name := commandName.FindStringSubmatch(text)
	if name == nil || name[1] == clearCommand {
		return nil
	}
	command := name[1]
	if args := commandArgs.FindStringSubmatch(text); args != nil && strings.TrimSpace(args[1]) != "" {
		command += " " + strings.TrimSpace(args[1])
	}
	return []DialogueEntry{{Kind: CommandEntry, Text: command}}
}

func typedText(prompt string) string {
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
	return []QueueOperation{{Operation: record.Operation, Reason: record.Reason, Text: typedText(record.Content)}}, nil
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
	holdsPrompt := slices.ContainsFunc(lineEntries, func(entry DialogueEntry) bool {
		return entry.Kind == PromptEntry || entry.Kind == CommandEntry
	})
	if holdsPrompt {
		var record transcriptRecord
		if err := json.Unmarshal(line, &record); err != nil {
			return turn, fmt.Errorf("decode transcript line %.80q: %w", line, err)
		}
		// A queued_command attachment is a prompt Claude took in mid-turn, so the turn it joins keeps its time.
		if record.Type == "attachment" && !turn.PromptTime.IsZero() {
			return turn, nil
		}
		return CurrentTurn{PromptTime: record.Timestamp}, nil
	}
	toolCall, holdsToolCall, err := newestToolCall(line)
	if err != nil || !holdsToolCall {
		return turn, err
	}
	return CurrentTurn{PromptTime: turn.PromptTime, ToolCallActivity: toolCall.activity}, nil
}

type toolCall struct {
	activity  string
	timestamp time.Time
}

// newestToolCall is the last tool_use of an assistant record, with holdsToolCall false for a line that holds none.
func newestToolCall(line []byte) (newest toolCall, holdsToolCall bool, err error) {
	if !bytes.Contains(line, []byte(`"type":"tool_use"`)) {
		return toolCall{}, false, nil
	}
	var record transcriptRecord
	if err := json.Unmarshal(line, &record); err != nil {
		return toolCall{}, false, fmt.Errorf("decode transcript line %.80q: %w", line, err)
	}
	if record.Type != "assistant" {
		return toolCall{}, false, nil
	}
	var blocks []contentBlock
	if err := json.Unmarshal(record.Message.Content, &blocks); err != nil {
		return toolCall{}, false, fmt.Errorf("decode assistant content %.80q: %w", record.Message.Content, err)
	}
	for index := len(blocks) - 1; index >= 0; index-- {
		if blocks[index].Type != "tool_use" {
			continue
		}
		activity, err := toolCallActivity(blocks[index].Name, blocks[index].Input)
		return toolCall{activity: activity, timestamp: record.Timestamp}, err == nil, err
	}
	return toolCall{}, false, nil
}

func toolCallActivity(toolName string, input json.RawMessage) (string, error) {
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

// SeekToTail moves file to the first whole line of its last 2 MB and returns that offset.
func SeekToTail(file *os.File) (int64, error) {
	info, err := file.Stat()
	if err != nil {
		return 0, err
	}
	start, err := TailStart(file, info.Size(), 2*1024*1024)
	if err != nil {
		return 0, err
	}
	return file.Seek(start, io.SeekStart)
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
