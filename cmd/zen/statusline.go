package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
)

// The blank row and the Claude status line row sit under the input.
const rowsBelowInputWithStatusLine = 2

type claudeStatusLineMsg struct {
	line string
}

// statusline.ps1 rewrites the file in place, so content not ending in a newline is a write in progress.
func followClaudeStatusLine(path string, interval time.Duration, send func(tea.Msg)) {
	sentLine := ""
	for {
		content, err := os.ReadFile(path)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			if sentLine != "" {
				sentLine = ""
				send(claudeStatusLineMsg{line: ""})
			}
		case err != nil:
			send(problemMsg{err: fmt.Errorf("Cannot read %s: %w", path, err)})
		default:
			line, whole := bytes.CutSuffix(content, []byte("\n"))
			if whole && string(line) != sentLine {
				sentLine = string(line)
				send(claudeStatusLineMsg{line: sentLine})
			}
		}
		time.Sleep(interval)
	}
}
