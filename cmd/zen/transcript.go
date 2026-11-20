package main

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

type entryKind int

const (
	promptEntry entryKind = iota
	replyEntry
	questionEntry
)

// A questionEntry holds the question in text and its option labels in options.
type dialogueEntry struct {
	kind    entryKind
	text    string
	options []string
}

// Claude Code writes these as user records with string content, though the user did not type them.
var notPromptPrefixes = []string{
	"<system-reminder", "<task-notification", "<agent-message", "<local-command-stdout", "<local-command-caveat",
	"<command-name", "<command-message", "<bash-input", "<bash-stdout", "<bash-stderr",
}

// A tag's own line break goes with it, so the pasted text keeps only the blank lines it held.
var pastedContentTag = regexp.MustCompile(`</?pasted_content[^>]*>\n?`)

type transcriptRecord struct {
	Type      string    `json:"type"`
	IsMeta    bool      `json:"isMeta"`
	Timestamp time.Time `json:"timestamp"`
	Message   struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
	// Claude Code writes a prompt sent while it works as an attachment of type queued_command.
	Attachment struct {
		Type        string          `json:"type"`
		Prompt      json.RawMessage `json:"prompt"`
		CommandMode string          `json:"commandMode"`
	} `json:"attachment"`
}

type queueOperation struct {
	operation string
	reason    string
	// text is the prompt as its band shows it, empty for a dequeue and a dropped_by_hook remove
	text string
	// entriesBefore counts the entries of the same read that the transcript holds before this record.
	entriesBefore int
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

type currentTurn struct {
	// promptTime is zero while the tail holds no prompt.
	promptTime       time.Time
	toolCallActivity string
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

func DialogueEntries(line []byte) ([]dialogueEntry, error) {
	if !mayHoldDialogue(line) {
		return nil, nil
	}
	var record transcriptRecord
	if err := json.Unmarshal(line, &record); err != nil {
		return nil, fmt.Errorf("decode transcript line %.80q: %w", line, err)
	}
	switch record.Type {
	case "user":
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
func promptEntries(content json.RawMessage) ([]dialogueEntry, error) {
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
	return []dialogueEntry{{kind: promptEntry, text: TypedText(prompt)}}, nil
}

func TypedText(prompt string) string {
	return strings.TrimSpace(pastedContentTag.ReplaceAllString(prompt, ""))
}

func QueueOperations(line []byte) ([]queueOperation, error) {
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
	return []queueOperation{{operation: record.Operation, reason: record.Reason, text: TypedText(record.Content)}}, nil
}

func assistantEntries(content json.RawMessage) ([]dialogueEntry, error) {
	var blocks []contentBlock
	if err := json.Unmarshal(content, &blocks); err != nil {
		return nil, fmt.Errorf("decode assistant content %.80q: %w", content, err)
	}
	var entries []dialogueEntry
	for _, block := range blocks {
		if block.Type == "text" {
			entries = append(entries, dialogueEntry{kind: replyEntry, text: block.Text})
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
				entries = append(entries, dialogueEntry{kind: questionEntry, text: question.Question, options: labels})
			}
		}
	}
	return entries, nil
}

func TurnAfterLine(turn currentTurn, line []byte, lineEntries []dialogueEntry) (currentTurn, error) {
	holdsPrompt := slices.ContainsFunc(lineEntries, func(entry dialogueEntry) bool { return entry.kind == promptEntry })
	if !holdsPrompt && !bytes.Contains(line, []byte(`"type":"tool_use"`)) {
		return turn, nil
	}
	var record transcriptRecord
	if err := json.Unmarshal(line, &record); err != nil {
		return turn, fmt.Errorf("decode transcript line %.80q: %w", line, err)
	}
	if holdsPrompt {
		return currentTurn{promptTime: record.Timestamp}, nil
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
		return currentTurn{promptTime: turn.promptTime, toolCallActivity: toolCallActivity}, nil
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
