package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type transcriptRecord struct {
	Type    string `json:"type"`
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

func DialogueEntries(line []byte) ([]string, error) {
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
		if !contentIsString {
			return nil, nil
		}
		var prompt string
		if err := json.Unmarshal(record.Message.Content, &prompt); err != nil {
			return nil, fmt.Errorf("decode user prompt %.80q: %w", record.Message.Content, err)
		}
		return []string{"> " + prompt}, nil
	case "assistant":
		return assistantEntries(record.Message.Content)
	default:
		return nil, nil
	}
}

func assistantEntries(content json.RawMessage) ([]string, error) {
	var blocks []contentBlock
	if err := json.Unmarshal(content, &blocks); err != nil {
		return nil, fmt.Errorf("decode assistant content %.80q: %w", content, err)
	}
	var entries []string
	for _, block := range blocks {
		if block.Type == "text" {
			entries = append(entries, block.Text)
		}
		if block.Type == "tool_use" && block.Name == "AskUserQuestion" {
			var input askUserQuestionInput
			if err := json.Unmarshal(block.Input, &input); err != nil {
				return nil, fmt.Errorf("decode AskUserQuestion input %.80q: %w", block.Input, err)
			}
			for _, question := range input.Questions {
				var entry strings.Builder
				entry.WriteString(question.Question)
				for number, option := range question.Options {
					fmt.Fprintf(&entry, "\n  %d. %s", number+1, option.Label)
				}
				entries = append(entries, entry.String())
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
