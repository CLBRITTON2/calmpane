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

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
)

// herdr turns each wheel notch over a pane that does not capture the mouse into one Up or Down key.
const arrowScrollRows = 3

// The status line, the separator and the input line sit under the entries.
const rowsUnderEntries = 3

type entriesReadMsg struct {
	entries []dialogueEntry
}

type agentStatusMsg struct {
	status string
}

// problemMsg carries a failure the view keeps running through, shown on the status line.
type problemMsg struct {
	err error
}

// fatalMsg carries a failure that ends the view.
type fatalMsg struct {
	err error
}

type promptSentMsg struct {
	err error
}

type zenModel struct {
	herdr        herdrCLI
	claudePaneID string
	// fatal is shown alone until a key ends the view, since herdr closes the pane as soon as zen exits.
	fatal   error
	entries []dialogueEntry
	// entryRows holds each entry's rows at the current width, nil until drawn.
	entryRows     [][]string
	replyRenderer *glamour.TermRenderer
	width         int
	height        int
	// rowsBelowView counts the rows under the bottom of the view, 0 when it shows the newest entry.
	rowsBelowView int
	status        string
	// copyStatus reports a ctrl+y copy in place of status until the next key or status.
	copyStatus string
	// newestCodeBlock is the block ctrl+y copies and whose frame carries the hint, nil while no reply holds one.
	newestCodeBlock *codeBlockPosition
	input           textinput.Model
}

func RunZenView(herdr herdrCLI) error {
	claudePaneID := os.Getenv(claudePaneEnv)
	input := textinput.New()
	input.Focus()
	model := zenModel{herdr: herdr, claudePaneID: claudePaneID, entries: []dialogueEntry{}, entryRows: [][]string{},
		input: input}
	transcript, err := openClaudeTranscript(herdr, claudePaneID)
	model.fatal = err
	program := tea.NewProgram(model)
	if transcript != nil {
		go func() {
			err := followTranscript(transcript, program.Send)
			program.Send(fatalMsg{err: fmt.Errorf("Cannot read %s: %w", transcript.Name(), err)})
		}()
		go pollAgentStatus(herdr, claudePaneID, program.Send)
	}
	finalModel, runErr := program.Run()
	if transcript != nil {
		runErr = errors.Join(runErr, transcript.Close())
	}
	if finalModel, isZenModel := finalModel.(zenModel); isZenModel {
		return errors.Join(finalModel.fatal, runErr)
	}
	return runErr
}

func openClaudeTranscript(herdr herdrCLI, claudePaneID string) (*os.File, error) {
	if claudePaneID == "" {
		return nil, fmt.Errorf("%s is not set: the toggle action opens the zen pane", claudePaneEnv)
	}
	if err := herdr.MarkZenPane(os.Getenv("HERDR_PANE_ID"), claudePaneID); err != nil {
		return nil, err
	}
	claudePane, err := herdr.Pane(claudePaneID)
	if err != nil {
		return nil, err
	}
	if claudePane.AgentSession == nil || claudePane.AgentSession.Agent != "claude" {
		return nil, errors.New("No Claude session in this pane.")
	}
	return openTranscript(claudePane.AgentSession.Value)
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

// followTranscript sends the tail's entries in one message once it reaches the end of the file, then each new
// line's entries as they are written.
func followTranscript(transcript *os.File, send func(tea.Msg)) error {
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
	unsentEntries := []dialogueEntry{}
	linesRead := 0
	for {
		line, err := reader.ReadBytes('\n')
		if err == io.EOF {
			partialLine = append(partialLine, line...)
			if len(unsentEntries) > 0 {
				send(entriesReadMsg{entries: unsentEntries})
				unsentEntries = []dialogueEntry{}
			}
			time.Sleep(200 * time.Millisecond)
			continue
		}
		if err != nil {
			return err
		}
		line = append(partialLine, line...)
		partialLine = nil
		linesRead++
		lineEntries, err := DialogueEntries(line)
		if err != nil {
			startLine, countErr := LineNumberAt(transcript, start)
			if countErr != nil {
				return countErr
			}
			send(problemMsg{err: fmt.Errorf("Cannot read line %d of %s: %w", startLine+linesRead-1,
				transcript.Name(), err)})
			continue
		}
		unsentEntries = append(unsentEntries, lineEntries...)
	}
}

// LineNumberAt is the 1-based number of the line that starts at offset.
func LineNumberAt(file io.ReaderAt, offset int64) (int, error) {
	reader := bufio.NewReaderSize(io.NewSectionReader(file, 0, offset), 1024*1024)
	newlines := 0
	for {
		chunk, err := reader.ReadSlice('\n')
		if len(chunk) > 0 && chunk[len(chunk)-1] == '\n' {
			newlines++
		}
		if err == io.EOF {
			return newlines + 1, nil
		}
		if err != nil && !errors.Is(err, bufio.ErrBufferFull) {
			return 0, fmt.Errorf("count lines before offset %d: %w", offset, err)
		}
	}
}

// pollAgentStatus keeps polling after a failure, since the view keeps running.
func pollAgentStatus(herdr herdrCLI, claudePaneID string, send func(tea.Msg)) {
	lastStatus := ""
	for {
		pane, err := herdr.Pane(claudePaneID)
		if err != nil {
			send(problemMsg{err: err})
		} else if pane.AgentStatus != lastStatus {
			lastStatus = pane.AgentStatus
			send(agentStatusMsg{status: pane.AgentStatus})
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func statusLine(agentStatus string) string {
	switch agentStatus {
	case "blocked":
		return waitingStyle.Render("Waiting for you in Claude. Press " + toggleKey + " to answer.")
	case "working":
		return workingStyle.Render("Working...")
	default:
		return ""
	}
}

func (m zenModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m zenModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.withSize(msg.Width, msg.Height), nil
	case entriesReadMsg:
		return m.withEntries(msg.entries), nil
	case agentStatusMsg:
		m.status = statusLine(msg.status)
		m.copyStatus = ""
		return m, nil
	case problemMsg:
		m.status = errorStyle.Render(msg.err.Error())
		m.copyStatus = ""
		return m, nil
	case fatalMsg:
		m.fatal = msg.err
		return m, nil
	case promptSentMsg:
		m.copyStatus = ""
		switch {
		case errors.Is(msg.err, errAgentBlocked):
			m.status = errorStyle.Render("Claude is waiting for you. Press " + toggleKey + " to answer.")
		case msg.err != nil:
			m.status = errorStyle.Render(msg.err.Error())
		default:
			m.input.SetValue("")
		}
		return m, nil
	case tea.KeyPressMsg:
		if m.fatal != nil {
			return m, tea.Quit
		}
		m.copyStatus = ""
		return m.withKey(msg)
	}
	var inputCmd tea.Cmd
	m.input, inputCmd = m.input.Update(msg)
	return m, inputCmd
}

func (m zenModel) withKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "pgup":
		return m.withRowsBelowViewChangedBy(m.entriesHeight()), nil
	case "pgdown":
		return m.withRowsBelowViewChangedBy(-m.entriesHeight()), nil
	case "up":
		return m.withRowsBelowViewChangedBy(arrowScrollRows), nil
	case "down":
		return m.withRowsBelowViewChangedBy(-arrowScrollRows), nil
	case "end":
		return m.withRowsBelowViewChangedBy(-m.rowsBelowView), nil
	case "ctrl+y":
		if m.newestCodeBlock == nil {
			m.copyStatus = workingStyle.Render("No code block to copy.")
			return m, nil
		}
		replyBlocks, err := CodeBlocks(m.entries[m.newestCodeBlock.entry].text)
		if err != nil {
			m.status = errorStyle.Render(err.Error())
			return m, nil
		}
		m.copyStatus = workingStyle.Render("Copied the last code block.")
		return m, tea.SetClipboard(replyBlocks[m.newestCodeBlock.block].code)
	case "enter":
		prompt := m.input.Value()
		if prompt == "" {
			return m, nil
		}
		return m, func() tea.Msg {
			return promptSentMsg{err: m.herdr.PromptAgent(m.claudePaneID, prompt)}
		}
	}
	var inputCmd tea.Cmd
	m.input, inputCmd = m.input.Update(msg)
	return m, inputCmd
}

// withSize redraws every entry when the width changes, since prompts and markdown wrap to it.
func (m zenModel) withSize(width int, height int) zenModel {
	if width != m.width {
		replyRenderer, err := NewReplyRenderer(width)
		if err != nil {
			m.fatal = err
			return m
		}
		m.replyRenderer = replyRenderer
		m.entryRows = make([][]string, len(m.entries))
	}
	m.width = width
	m.height = height
	// textinput draws its end-of-text cursor one cell past its width
	m.input.SetWidth(max(width-1-len(m.input.Prompt)-1, 1))
	return m.withVisibleRowsDrawn()
}

// withEntries keeps a view scrolled up on the rows it shows, by counting the new entries' rows under it.
func (m zenModel) withEntries(entries []dialogueEntry) zenModel {
	firstNew := len(m.entries)
	m.entries = append(m.entries, entries...)
	m.entryRows = append(m.entryRows, make([][]string, len(entries))...)
	newPosition, err := NewestCodeBlockPosition(entries)
	if err != nil {
		m.status = errorStyle.Render(err.Error())
	}
	if newPosition != nil {
		formerPosition := m.newestCodeBlock
		m.newestCodeBlock = &codeBlockPosition{entry: firstNew + newPosition.entry, block: newPosition.block}
		// the former newest block's frame loses the copy hint, in the same number of rows
		if formerPosition != nil && m.entryRows[formerPosition.entry] != nil {
			m.entryRows[formerPosition.entry] = nil
			m = m.withEntryDrawn(formerPosition.entry)
		}
	}
	if m.rowsBelowView > 0 {
		for index := firstNew; index < len(m.entries); index++ {
			m = m.withEntryDrawn(index)
			m.rowsBelowView += len(m.entryRows[index])
		}
	}
	return m.withVisibleRowsDrawn()
}

func (m zenModel) withRowsBelowViewChangedBy(rows int) zenModel {
	m.rowsBelowView = max(m.rowsBelowView+rows, 0)
	return m.withVisibleRowsDrawn()
}

func (m zenModel) entriesHeight() int {
	return max(m.height-rowsUnderEntries, 0)
}

// withVisibleRowsDrawn draws entries from the newest back until the view is covered, so a start on a long tail
// draws only what the screen shows, and stops a view scrolled past the oldest entry at it.
func (m zenModel) withVisibleRowsDrawn() zenModel {
	if m.width == 0 {
		return m
	}
	rowsNeeded := m.rowsBelowView + m.entriesHeight()
	rowsDrawn := 0
	for index := len(m.entries) - 1; index >= 0 && rowsDrawn < rowsNeeded; index-- {
		m = m.withEntryDrawn(index)
		rowsDrawn += len(m.entryRows[index])
	}
	if rowsDrawn < rowsNeeded {
		m.rowsBelowView = max(rowsDrawn-m.entriesHeight(), 0)
	}
	return m
}

// withEntryDrawn shows a failed draw on the status line and leaves the entry without rows, so it is not retried.
func (m zenModel) withEntryDrawn(index int) zenModel {
	if m.entryRows[index] != nil {
		return m
	}
	copyHintBlock := -1
	if m.newestCodeBlock != nil && m.newestCodeBlock.entry == index {
		copyHintBlock = m.newestCodeBlock.block
	}
	rows, err := EntryRows(m.entries[index], copyHintBlock, m.width, m.replyRenderer)
	if err != nil {
		m.status = errorStyle.Render(err.Error())
		rows = []string{}
	}
	m.entryRows[index] = rows
	return m
}

func (m zenModel) View() tea.View {
	var view tea.View
	switch {
	case m.fatal != nil:
		view = tea.NewView(m.fatal.Error())
	case m.width == 0:
		view = tea.NewView("")
	default:
		view = tea.NewView(m.screen())
	}
	view.AltScreen = true
	return view
}

func (m zenModel) screen() string {
	height := m.entriesHeight()
	rowsNeeded := m.rowsBelowView + height
	rowsOldestFirst := []string{}
	for index := len(m.entries) - 1; index >= 0 && len(rowsOldestFirst) < rowsNeeded; index-- {
		rowsOldestFirst = append(append([]string{}, m.entryRows[index]...), rowsOldestFirst...)
	}
	end := max(len(rowsOldestFirst)-m.rowsBelowView, 0)
	visible := rowsOldestFirst[max(end-height, 0):end]
	rows := make([]string, height-len(visible), height+rowsUnderEntries)
	rows = append(rows, visible...)
	shownStatus := m.status
	if m.copyStatus != "" {
		shownStatus = m.copyStatus
	}
	rows = append(rows, " "+shownStatus, separatorStyle.Render(strings.Repeat("─", m.width)), " "+m.input.View())
	return strings.Join(rows, "\n")
}
