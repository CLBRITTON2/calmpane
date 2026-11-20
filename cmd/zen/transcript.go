package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
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
	Type    string `json:"type"`
	IsMeta  bool   `json:"isMeta"`
	Message struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

type contentBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
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
		contentIsString := bytes.HasPrefix(record.Message.Content, []byte(`"`))
		if !contentIsString || record.IsMeta {
			return nil, nil
		}
		var prompt string
		if err := json.Unmarshal(record.Message.Content, &prompt); err != nil {
			return nil, fmt.Errorf("decode user prompt %.80q: %w", record.Message.Content, err)
		}
		for _, prefix := range notPromptPrefixes {
			if strings.HasPrefix(prompt, prefix) {
				return nil, nil
			}
		}
		typedText := strings.TrimSpace(pastedContentTag.ReplaceAllString(prompt, ""))
		return []dialogueEntry{{kind: promptEntry, text: typedText}}, nil
	case "assistant":
		return assistantEntries(record.Message.Content)
	default:
		return nil, nil
	}
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
