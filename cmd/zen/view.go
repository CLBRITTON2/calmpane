package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

type keysRead struct {
	keys []byte
	err  error
}

type entriesRead struct {
	entries []string
	err     error
}

type agentStatusRead struct {
	status string
	err    error
}

// A failure stays on screen until a key is pressed, since herdr closes the pane as soon as zen exits.
func RunZenView(herdr herdrCLI) error {
	restoreConsole, err := MakeConsoleRaw()
	if err != nil {
		return err
	}
	keys := make(chan keysRead)
	go readKeys(keys)
	viewErr := showDialogue(herdr, keys)
	_, writeErr := fmt.Fprintf(os.Stdout, "%s\r\n", viewErr)
	keyErr := (<-keys).err
	return errors.Join(viewErr, writeErr, keyErr, restoreConsole())
}

// keys is closed after a failed read, so a receive after the failure returns at once.
func readKeys(keys chan<- keysRead) {
	defer close(keys)
	for {
		buffer := make([]byte, 256)
		readCount, err := os.Stdin.Read(buffer)
		keys <- keysRead{keys: buffer[:readCount], err: err}
		if err != nil {
			return
		}
	}
}

func showDialogue(herdr herdrCLI, keys <-chan keysRead) (err error) {
	claudePaneID := os.Getenv(claudePaneEnv)
	if claudePaneID == "" {
		return fmt.Errorf("%s is not set: the toggle action opens the zen pane", claudePaneEnv)
	}
	if err := herdr.MarkZenPane(os.Getenv("HERDR_PANE_ID"), claudePaneID); err != nil {
		return err
	}
	claudePane, err := herdr.Pane(claudePaneID)
	if err != nil {
		return err
	}
	if claudePane.AgentSession == nil || claudePane.AgentSession.Agent != "claude" {
		return errors.New("No Claude session in this pane.")
	}
	transcript, err := openTranscript(claudePane.AgentSession.Value)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, transcript.Close())
	}()

	entries := make(chan entriesRead)
	go func() {
		err := followTranscript(transcript, entries)
		entries <- entriesRead{err: fmt.Errorf("Cannot read %s: %w", transcript.Name(), err)}
	}()
	statuses := make(chan agentStatusRead)
	go pollAgentStatus(herdr, claudePaneID, statuses)

	var screen zenScreen
	if err := screen.Draw(""); err != nil {
		return err
	}
	for {
		select {
		case read := <-entries:
			if read.err != nil {
				return read.err
			}
			for _, entry := range read.entries {
				if err := screen.Draw(entry); err != nil {
					return err
				}
			}
		case read := <-statuses:
			if read.err != nil {
				return read.err
			}
			screen.status = statusLine(read.status)
			if err := screen.Draw(""); err != nil {
				return err
			}
		case read := <-keys:
			if read.err != nil {
				return fmt.Errorf("read keys: %w", read.err)
			}
			if err := applyKeys(&screen, herdr, claudePaneID, read.keys); err != nil {
				return err
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

func followTranscript(transcript *os.File, entries chan<- entriesRead) error {
	info, err := transcript.Stat()
	if err != nil {
		return err
	}
	start, err := TailStart(transcript, info.Size(), 2*1024*1024)
	if err != nil {
		return err
	}
	if _, err := transcript.Seek(start, io.SeekStart); err != nil {
		return err
	}
	reader := bufio.NewReaderSize(transcript, 1024*1024)
	var partialLine []byte
	for {
		line, err := reader.ReadBytes('\n')
		if err == io.EOF {
			partialLine = append(partialLine, line...)
			time.Sleep(200 * time.Millisecond)
			continue
		}
		if err != nil {
			return err
		}
		line = append(partialLine, line...)
		partialLine = nil
		lineEntries, err := DialogueEntries(line)
		if err != nil {
			return err
		}
		if len(lineEntries) > 0 {
			entries <- entriesRead{entries: lineEntries}
		}
	}
}

func pollAgentStatus(herdr herdrCLI, claudePaneID string, statuses chan<- agentStatusRead) {
	lastStatus := ""
	for {
		pane, err := herdr.Pane(claudePaneID)
		if err != nil {
			statuses <- agentStatusRead{err: err}
			return
		}
		if pane.AgentStatus != lastStatus {
			lastStatus = pane.AgentStatus
			statuses <- agentStatusRead{status: pane.AgentStatus}
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func statusLine(agentStatus string) string {
	switch agentStatus {
	case "blocked":
		return "Waiting for you in Claude. Press " + toggleKey + " to answer."
	case "working":
		return "Working..."
	default:
		return ""
	}
}

// Escape sequences and control keys are dropped, since the input line takes one line of plain text.
func applyKeys(screen *zenScreen, herdr herdrCLI, claudePaneID string, keys []byte) error {
	for len(keys) > 0 {
		switch key := keys[0]; {
		case key == '\r':
			keys = keys[1:]
			if len(screen.input) == 0 {
				continue
			}
			err := herdr.PromptAgent(claudePaneID, string(screen.input))
			if errors.Is(err, errAgentBlocked) {
				// Shown as an entry, so it stays on screen after the status line changes.
				if err := screen.Draw("Claude is waiting for you. Press " + toggleKey + " to answer."); err != nil {
					return err
				}
				continue
			}
			if err != nil {
				return err
			}
			screen.input = nil
		case key == 0x7f || key == 0x08:
			keys = keys[1:]
			if len(screen.input) > 0 {
				screen.input = screen.input[:len(screen.input)-1]
			}
		case key == 0x1b:
			keys = keys[escapeSequenceLength(keys):]
		case key < 0x20:
			keys = keys[1:]
		default:
			typed, size := utf8.DecodeRune(keys)
			keys = keys[size:]
			screen.input = append(screen.input, typed)
		}
	}
	return screen.Draw("")
}

func escapeSequenceLength(keys []byte) int {
	if len(keys) < 2 {
		return len(keys)
	}
	switch keys[1] {
	case '[':
		for index := 2; index < len(keys); index++ {
			if keys[index] >= 0x40 && keys[index] <= 0x7e {
				return index + 1
			}
		}
		return len(keys)
	case 'O':
		return min(3, len(keys))
	default:
		return 2
	}
}

// zenScreen keeps the status line and the input line at the bottom while entries print above them.
type zenScreen struct {
	status     string
	input      []rune
	bottomRows int
}

func (s *zenScreen) Draw(entry string) error {
	columns, err := ConsoleColumns()
	if err != nil {
		return err
	}
	var output strings.Builder
	if s.bottomRows > 1 {
		fmt.Fprintf(&output, "\x1b[%dA", s.bottomRows-1)
	}
	output.WriteString("\r\x1b[J")
	if entry != "" {
		output.WriteString(strings.ReplaceAll(strings.ReplaceAll(entry, "\r\n", "\n"), "\n", "\r\n"))
		output.WriteString("\r\n\r\n")
	}
	s.bottomRows = 0
	if s.status != "" {
		output.WriteString(s.status + "\r\n")
		s.bottomRows += rowsOf(s.status, columns)
	}
	inputLine := "> " + string(s.input)
	output.WriteString(inputLine)
	s.bottomRows += rowsOf(inputLine, columns)
	_, err = io.WriteString(os.Stdout, output.String())
	return err
}

func rowsOf(line string, columns int) int {
	width := utf8.RuneCountInString(line)
	if width == 0 {
		return 1
	}
	return (width + columns - 1) / columns
}
