package ui

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
)

// The separator closing the input and the Claude status line row sit under the input.
const rowsBelowInputWithStatusLine = 2

type claudeStatusLineMsg struct {
	line string
}

// followClaudeStatusLine follows the latest path from paths, starting once the first arrives. statusline.ps1 rewrites
// the file in place, so content not ending in a newline is a write in progress.
func followClaudeStatusLine(paths <-chan string, interval time.Duration, send func(tea.Msg)) {
	path := <-paths
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
		select {
		case path = <-paths:
		case <-time.After(interval):
		}
	}
}
