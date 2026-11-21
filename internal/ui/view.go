package ui

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/CLBRITTON2/herdr-zen/internal/claudecode"
	"github.com/CLBRITTON2/herdr-zen/internal/herdr"
	"github.com/CLBRITTON2/herdr-zen/internal/zenmode"
)

// toggleKey is the key windows-dev's herdr/config.toml binds to the windows-dev.zen.toggle action.
const toggleKey = "alt+x"

// herdr turns each wheel notch over a pane that does not capture the mouse into one Up or Down key.
const arrowScrollRows = 3

// The input grows a row at a time up to inputMaxRows, then scrolls inside itself.
const inputMaxRows = 8

const inputPrompt = "> "

type entriesReadMsg struct {
	entries         []claudecode.DialogueEntry
	queueOperations []claudecode.QueueOperation
	turn            claudecode.CurrentTurn
}

type agentStatusMsg struct {
	status string
}

type clockTickMsg struct {
	now time.Time
}

type turnEndedMsg struct {
	now time.Time
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
	prompt string
	pastes []string
	err    error
}

type claudeKeysSentMsg struct {
	err error
}

// Claude Code's default chat:sendNow keys. Not Ctrl+Enter, which a terminal without extended keys sends as Enter.
var sendNowKeys = []string{"ctrl+x", "ctrl+s"}

// agentInputTakenMsg carries the prompt Claude put back in its input box when an Esc cancelled the turn before a reply.
type agentInputTakenMsg struct {
	text string
	err  error
}

type earlyPrompt struct {
	text   string
	pastes []string
	queued bool
}

type zenModel struct {
	claude       zenmode.ClaudeInput
	claudePaneID string
	// fatal is shown alone until a key ends the view, since herdr closes the pane as soon as zen exits.
	fatal   error
	entries []claudecode.DialogueEntry
	// entryRows holds each entry's rows at the current width, nil until drawn.
	entryRows     [][]string
	replyRenderer *glamour.TermRenderer
	width         int
	height        int
	// rowsBelowView counts the rows under the bottom of the view, 0 when it shows the newest entry.
	rowsBelowView int
	agentStatus   string
	// interrupting is set by an Esc sent to a working Claude, until Claude stops working.
	interrupting bool
	turn         claudecode.CurrentTurn
	now          time.Time
	clockTicking bool
	// endedTurnRow reports the turn this view saw end, until Claude works or blocks again.
	endedTurnRow string
	// status reports the latest problem in place of the agent status row, until the agent status changes.
	status string
	// copyStatus reports a ctrl+y copy in place of status until the next key or status.
	copyStatus string
	// newestCodeBlock is the block ctrl+y copies and whose frame carries the hint, nil while no reply holds one.
	newestCodeBlock *codeBlockPosition
	// earlyPrompts holds the prompts Enter sent that the transcript has not shown yet, drawn as bands under the
	// entries until a prompt entry with the same text arrives.
	earlyPrompts []earlyPrompt
	input        textarea.Model
	// pastes holds each paste the input shows as a label, numbered from 1 in order, until the input is sent.
	pastes           []string
	claudeStatusLine string
}

// Shift+Enter inserts a newline, since Enter sends.
func NewPromptInput() textarea.Model {
	input := textarea.New()
	input.ShowLineNumbers = false
	input.DynamicHeight = true
	input.MaxHeight = inputMaxRows
	// Left at 0, MaxContentHeight would stop the typing at MaxHeight lines instead of scrolling.
	input.MaxContentHeight = math.MaxInt
	input.MaxWidth = 0
	input.SetPromptFunc(len(inputPrompt), func(info textarea.PromptInfo) string {
		if info.LineNumber == 0 {
			return inputPrompt
		}
		return ""
	})
	input.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("shift+enter"))
	styles := input.Styles()
	styles.Focused.CursorLine = lipgloss.NewStyle()
	input.SetStyles(styles)
	input.Focus()
	return input
}

func RunZenView(cli herdr.CLI) error {
	claudePaneID := os.Getenv(herdr.ClaudePaneEnv)
	model := zenModel{claude: cli, claudePaneID: claudePaneID, entries: []claudecode.DialogueEntry{}, entryRows: [][]string{},
		input: NewPromptInput()}
	mode, modeErr := zenmode.ModeOfPlugin(cli.PluginID)
	draft, draftErr := zenmode.SavedDraft(mode.DraftFile(claudePaneID))
	model.fatal = errors.Join(modeErr, draftErr)
	model.input.SetValue(draft)
	program := tea.NewProgram(draftSavingView{zen: model, draftFile: mode.DraftFile(claudePaneID), savedDraft: draft})
	go func() {
		transcript, statusLinePath, err := claudecode.WaitForTranscriptAndStatusLinePath(cli, claudePaneID)
		if err != nil {
			program.Send(fatalMsg{err: err})
			return
		}
		go pollAgentStatus(cli, claudePaneID, program.Send)
		go followClaudeStatusLine(statusLinePath, 300*time.Millisecond, program.Send)
		err = followTranscript(transcript, program.Send)
		program.Send(fatalMsg{err: errors.Join(fmt.Errorf("Cannot read %s: %w", transcript.Name(), err),
			transcript.Close())})
	}()
	finalModel, runErr := program.Run()
	if finalModel, isDraftSavingView := finalModel.(draftSavingView); isDraftSavingView {
		return errors.Join(finalModel.zen.fatal, runErr)
	}
	return runErr
}

// followTranscript sends the tail's entries in one message once it reaches the end of the file, then each new
// line's entries as they are written.
func followTranscript(transcript *os.File, send func(tea.Msg)) error {
	info, err := transcript.Stat()
	if err != nil {
		return err
	}
	start, err := claudecode.TailStart(transcript, info.Size(), 2*1024*1024)
	if err != nil {
		return err
	}
	if _, err := transcript.Seek(start, io.SeekStart); err != nil {
		return err
	}
	reader := bufio.NewReaderSize(transcript, 1024*1024)
	var partialLine []byte
	unsentEntries := []claudecode.DialogueEntry{}
	unsentQueueOperations := []claudecode.QueueOperation{}
	turn := claudecode.CurrentTurn{}
	turnChanged := false
	linesRead := 0
	for {
		line, err := reader.ReadBytes('\n')
		if err == io.EOF {
			partialLine = append(partialLine, line...)
			if len(unsentEntries) > 0 || len(unsentQueueOperations) > 0 || turnChanged {
				send(entriesReadMsg{entries: unsentEntries, queueOperations: unsentQueueOperations, turn: turn})
				unsentEntries = []claudecode.DialogueEntry{}
				unsentQueueOperations = []claudecode.QueueOperation{}
				turnChanged = false
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
		lineEntries, err := claudecode.DialogueEntries(line)
		var lineQueueOperations []claudecode.QueueOperation
		if err == nil {
			lineQueueOperations, err = claudecode.QueueOperations(line)
		}
		lineTurn := turn
		if err == nil {
			lineTurn, err = claudecode.TurnAfterLine(turn, line, lineEntries)
		}
		if err != nil {
			startLine, countErr := LineNumberAt(transcript, start)
			if countErr != nil {
				return countErr
			}
			send(problemMsg{err: fmt.Errorf("Cannot read line %d of %s: %w", startLine+linesRead-1,
				transcript.Name(), err)})
			continue
		}
		for _, operation := range lineQueueOperations {
			operation.EntriesBefore = len(unsentEntries)
			unsentQueueOperations = append(unsentQueueOperations, operation)
		}
		unsentEntries = append(unsentEntries, lineEntries...)
		turnChanged = turnChanged || lineTurn != turn
		turn = lineTurn
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
func pollAgentStatus(cli herdr.CLI, claudePaneID string, send func(tea.Msg)) {
	lastStatus := ""
	for {
		pane, err := cli.Pane(claudePaneID)
		if err != nil {
			send(problemMsg{err: err})
		} else if pane.AgentStatus != lastStatus {
			lastStatus = pane.AgentStatus
			send(agentStatusMsg{status: pane.AgentStatus})
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func statusLine(agentStatus string, turn claudecode.CurrentTurn, now time.Time, width int) string {
	switch agentStatus {
	case "blocked":
		return waitingStyle.Render("Waiting for you in Claude. Press " + toggleKey + " to answer.")
	case "working":
		activity := turn.ToolCallActivity
		if activity == "" {
			activity = "Working..."
		}
		duration := ""
		if !turn.PromptTime.IsZero() {
			duration = " · " + TurnDuration(max(now.Sub(turn.PromptTime), 0))
		}
		// the row's left margin takes one column
		activityWidth := max(width-1-ansi.StringWidth(duration), 0)
		return workingStyle.Render(ansi.Truncate(activity, activityWidth, "") + duration)
	default:
		return ""
	}
}

func TurnDuration(elapsed time.Duration) string {
	seconds := int(elapsed / time.Second)
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	return fmt.Sprintf("%dm %ds", seconds/60, seconds%60)
}

func (m zenModel) Init() tea.Cmd {
	return textarea.Blink
}

func (m zenModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.withSize(msg.Width, msg.Height), nil
	case entriesReadMsg:
		m.turn = msg.turn
		return m.withEntries(msg.entries, msg.queueOperations), nil
	case claudeKeysSentMsg:
		if msg.err != nil {
			m.status = errorStyle.Render(msg.err.Error())
			m.copyStatus = ""
		}
		return m, nil
	case agentStatusMsg:
		turnEnded := m.agentStatus == "working" && (msg.status == "idle" || msg.status == "done")
		m.agentStatus = msg.status
		m.status = ""
		m.copyStatus = ""
		m.endedTurnRow = ""
		if msg.status != "working" && m.interrupting {
			m.interrupting = false
			return m, func() tea.Msg {
				text, err := zenmode.TakeAgentInput(m.claude, m.claudePaneID)
				return agentInputTakenMsg{text: text, err: err}
			}
		}
		if turnEnded {
			return m, func() tea.Msg { return turnEndedMsg{now: time.Now()} }
		}
		if msg.status != "working" || m.clockTicking {
			return m, nil
		}
		m.clockTicking = true
		return m, func() tea.Msg { return clockTickMsg{now: time.Now()} }
	case clockTickMsg:
		m.now = msg.now
		if m.agentStatus != "working" {
			m.clockTicking = false
			return m, nil
		}
		return m, tea.Every(time.Second, func(now time.Time) tea.Msg { return clockTickMsg{now: now} })
	case turnEndedMsg:
		if m.agentStatus == "working" || m.agentStatus == "blocked" {
			return m, nil
		}
		m.endedTurnRow = "Done"
		if !m.turn.PromptTime.IsZero() {
			m.endedTurnRow += " · " + TurnDuration(max(msg.now.Sub(m.turn.PromptTime), 0))
		}
		return m.withVisibleRowsDrawn(), nil
	case agentInputTakenMsg:
		if msg.err != nil {
			m.status = errorStyle.Render(msg.err.Error())
			m.copyStatus = ""
			return m, nil
		}
		return m.withCancelledPromptRestored(msg.text), nil
	case problemMsg:
		m.status = errorStyle.Render(msg.err.Error())
		m.copyStatus = ""
		return m, nil
	case fatalMsg:
		m.fatal = msg.err
		return m, nil
	case claudeStatusLineMsg:
		m.claudeStatusLine = msg.line
		return m.withVisibleRowsDrawn(), nil
	case promptSentMsg:
		m.copyStatus = ""
		switch {
		case errors.Is(msg.err, herdr.ErrAgentBlocked):
			m.status = errorStyle.Render("Claude is waiting for you. Press " + toggleKey + " to answer.")
		case msg.err != nil:
			m.status = errorStyle.Render(msg.err.Error())
		}
		if msg.err == nil {
			return m, nil
		}
		return m.withPromptPutBack(msg.prompt, msg.pastes), nil
	case tea.KeyPressMsg:
		if m.fatal != nil {
			return m, tea.Quit
		}
		m.copyStatus = ""
		return m.withKey(msg)
	case tea.PasteMsg:
		return m.withPasteInserted(msg.Content)
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
		if m.input.Height() == 1 {
			return m.withRowsBelowViewChangedBy(arrowScrollRows), nil
		}
	case "down":
		if m.input.Height() == 1 {
			return m.withRowsBelowViewChangedBy(-arrowScrollRows), nil
		}
	case "end":
		return m.withRowsBelowViewChangedBy(-m.rowsBelowView), nil
	case "ctrl+y":
		if m.newestCodeBlock == nil {
			m.copyStatus = workingStyle.Render("No code block to copy.")
			return m, nil
		}
		replyBlocks, err := CodeBlocks(m.entries[m.newestCodeBlock.entry].Text)
		if err != nil {
			m.status = errorStyle.Render(err.Error())
			return m, nil
		}
		m.copyStatus = workingStyle.Render("Copied the last code block.")
		return m, tea.SetClipboard(replyBlocks[m.newestCodeBlock.block].code)
	case "enter":
		prompt := m.InputText()
		if prompt == "" {
			return m, nil
		}
		pastes := m.pastes
		sendPrompt := func() tea.Msg {
			return promptSentMsg{prompt: prompt, pastes: pastes, err: m.claude.PromptAgent(m.claudePaneID, prompt)}
		}
		return m.withInputSent(prompt), sendPrompt
	case "ctrl+enter":
		prompt := m.InputText()
		pastes := m.pastes
		if prompt == "" {
			return m, func() tea.Msg {
				return claudeKeysSentMsg{err: m.claude.SendKeys(m.claudePaneID, sendNowKeys)}
			}
		}
		// herdr returns from agent prompt once it has written the prompt and its Enter, so the keys reach Claude after them.
		sendPromptAndSendNow := func() tea.Msg {
			if err := m.claude.PromptAgent(m.claudePaneID, prompt); err != nil {
				return promptSentMsg{prompt: prompt, pastes: pastes, err: err}
			}
			return claudeKeysSentMsg{err: m.claude.SendKeys(m.claudePaneID, sendNowKeys)}
		}
		return m.withInputSent(prompt), sendPromptAndSendNow
	case "esc":
		// Claude takes Esc twice in a row while idle as the rewind menu, which would wait unseen behind the zen view
		if m.agentStatus != "working" {
			return m, nil
		}
		m.interrupting = true
		return m, func() tea.Msg {
			return claudeKeysSentMsg{err: m.claude.SendKeys(m.claudePaneID, []string{"esc"})}
		}
	}
	var inputCmd tea.Cmd
	m.input, inputCmd = m.input.Update(msg)
	// the entries take the rows the input gives up
	return m.withVisibleRowsDrawn(), inputCmd
}

// withPromptPutBack shows the prompt's pastes and those typed since as labels again, renumbered in order.
func (m zenModel) withPromptPutBack(prompt string, pastes []string) zenModel {
	restored := prompt
	if typedSince := m.InputText(); typedSince != "" {
		restored += "\n" + typedSince
	}
	m.pastes = append(slices.Clone(pastes), m.pastes...)
	m.input.SetValue(claudecode.TextWithPastesCollapsed(restored, m.pastes))
	return m.withEarlyPromptRemoved(strings.TrimSpace(prompt))
}

func (m zenModel) InputText() string {
	return claudecode.TextWithPastesExpanded(m.input.Value(), m.pastes)
}

func (m zenModel) withPasteInserted(content string) (zenModel, tea.Cmd) {
	paste := claudecode.NormalizedPaste(content)
	if !claudecode.IsShownAsLabel(paste) {
		var inputCmd tea.Cmd
		m.input, inputCmd = m.input.Update(tea.PasteMsg{Content: content})
		return m.withVisibleRowsDrawn(), inputCmd
	}
	m.pastes = append(slices.Clone(m.pastes), paste)
	m.input.InsertString(claudecode.PasteLabel(len(m.pastes), paste))
	return m.withVisibleRowsDrawn(), nil
}

// Claude's own view drops a prompt an Esc cancelled before the reply.
func (m zenModel) withCancelledPromptRestored(prompt string) zenModel {
	if prompt == "" {
		return m
	}
	m = m.withPromptPutBack(prompt, nil)
	newest := len(m.entries) - 1
	newestIsCancelledPrompt := newest >= 0 && m.entries[newest].Kind == claudecode.PromptEntry &&
		strings.TrimSpace(m.entries[newest].Text) == strings.TrimSpace(prompt)
	if !newestIsCancelledPrompt {
		return m.withVisibleRowsDrawn()
	}
	if m.rowsBelowView > 0 {
		m.rowsBelowView = max(m.rowsBelowView-len(m.entryRows[newest]), 0)
	}
	m.entries = slices.Delete(slices.Clone(m.entries), newest, newest+1)
	m.entryRows = slices.Delete(slices.Clone(m.entryRows), newest, newest+1)
	return m.withVisibleRowsDrawn()
}

func (m zenModel) withInputSent(prompt string) zenModel {
	pastes := m.pastes
	m.pastes = nil
	m.input.Reset()
	m = m.withVisibleRowsDrawn()
	// Claude records a slash command as a hidden <command-name> record, which would never replace the band.
	bandText := strings.TrimSpace(prompt)
	if bandText == "" || strings.HasPrefix(bandText, "/") {
		return m
	}
	return m.withEarlyPromptAdded(earlyPrompt{text: bandText, pastes: pastes})
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
	// one column of margin on each side
	m.input.SetWidth(max(width-2, len(inputPrompt)+1))
	return m.withVisibleRowsDrawn()
}

// withEntries keeps a view scrolled up on the rows it shows, by counting the new entries' rows under it.
func (m zenModel) withEntries(entries []claudecode.DialogueEntry, queueOperations []claudecode.QueueOperation) zenModel {
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
	nextOperation := 0
	for index, entry := range entries {
		for ; nextOperation < len(queueOperations) && queueOperations[nextOperation].EntriesBefore <= index; nextOperation++ {
			m = m.withQueueOperation(queueOperations[nextOperation])
		}
		if entry.Kind != claudecode.PromptEntry {
			continue
		}
		if bandIndex := m.earlyPromptIndex(entry.Text); bandIndex >= 0 {
			m = m.withEntryPastes(firstNew+index, m.earlyPrompts[bandIndex].pastes).withEarlyPromptRemovedAt(bandIndex)
		}
	}
	for _, operation := range queueOperations[nextOperation:] {
		m = m.withQueueOperation(operation)
	}
	return m.withVisibleRowsDrawn()
}

// withEntryPastes gives a prompt entry the pastes of the early band it replaces, since herdr sends a zen prompt as one
// paste, which Claude Code wraps whole in a pasted_content tag.
func (m zenModel) withEntryPastes(index int, pastes []string) zenModel {
	formerRows := m.entryRows[index]
	m.entries[index].Pastes = pastes
	if formerRows == nil {
		return m
	}
	m.entryRows[index] = nil
	m = m.withEntryDrawn(index)
	if m.rowsBelowView > 0 {
		m.rowsBelowView = max(m.rowsBelowView+len(m.entryRows[index])-len(formerRows), 0)
	}
	return m
}

func (m zenModel) withQueueOperation(operation claudecode.QueueOperation) zenModel {
	isQueued := func(early earlyPrompt) bool { return early.queued }
	isQueuedWithText := func(early earlyPrompt) bool { return early.queued && early.text == operation.Text }
	switch operation.Operation {
	case "enqueue":
		isUnqueuedWithText := func(early earlyPrompt) bool { return !early.queued && early.text == operation.Text }
		return m.withEarlyPromptMarkedQueuedAt(slices.IndexFunc(m.earlyPrompts, isUnqueuedWithText))
	case "dequeue":
		return m.withEarlyPromptQueuedMarkClearedAt(slices.IndexFunc(m.earlyPrompts, isQueued))
	case "remove":
		switch operation.Reason {
		case "absorbed_mid_turn":
			return m.withEarlyPromptQueuedMarkClearedAt(slices.IndexFunc(m.earlyPrompts, isQueuedWithText))
		case "cleared_on_cancel", "not_carried_to_background":
			return m.withEarlyPromptRemovedAt(slices.IndexFunc(m.earlyPrompts, isQueuedWithText))
		case "dropped_by_hook":
			return m.withEarlyPromptRemovedAt(slices.IndexFunc(m.earlyPrompts, isQueued))
		}
	}
	return m
}

func (m zenModel) withEarlyPromptAdded(early earlyPrompt) zenModel {
	return m.withEarlyPrompts(append(slices.Clone(m.earlyPrompts), early))
}

// withEarlyPromptRemoved removes the oldest early band with the prompt's text, if there is one.
func (m zenModel) withEarlyPromptRemoved(prompt string) zenModel {
	return m.withEarlyPromptRemovedAt(m.earlyPromptIndex(prompt))
}

func (m zenModel) earlyPromptIndex(prompt string) int {
	return slices.IndexFunc(m.earlyPrompts, func(early earlyPrompt) bool { return early.text == prompt })
}

func (m zenModel) withEarlyPromptRemovedAt(index int) zenModel {
	if index < 0 {
		return m
	}
	return m.withEarlyPrompts(slices.Delete(slices.Clone(m.earlyPrompts), index, index+1))
}

func (m zenModel) withEarlyPromptMarkedQueuedAt(index int) zenModel {
	if index < 0 {
		return m
	}
	earlyPrompts := slices.Clone(m.earlyPrompts)
	earlyPrompts[index].queued = true
	return m.withEarlyPrompts(earlyPrompts)
}

func (m zenModel) withEarlyPromptQueuedMarkClearedAt(index int) zenModel {
	if index < 0 {
		return m
	}
	earlyPrompts := slices.Clone(m.earlyPrompts)
	earlyPrompts[index].queued = false
	return m.withEarlyPrompts(earlyPrompts)
}

// withEarlyPrompts keeps a view scrolled up on the rows it shows, as withEntries does.
func (m zenModel) withEarlyPrompts(earlyPrompts []earlyPrompt) zenModel {
	formerRowCount := len(m.earlyPromptRows())
	m.earlyPrompts = earlyPrompts
	if m.rowsBelowView > 0 {
		m.rowsBelowView = max(m.rowsBelowView+len(m.earlyPromptRows())-formerRowCount, 0)
	}
	return m.withVisibleRowsDrawn()
}

func (m zenModel) earlyPromptRows() []string {
	rows := []string{}
	for _, early := range m.earlyPrompts {
		note := ""
		if early.queued {
			note = queuedNote
		}
		rows = append(rows, PromptEntryRows(claudecode.TextWithPastesCollapsed(early.text, early.pastes), note, m.width)...)
	}
	return rows
}

func (m zenModel) withRowsBelowViewChangedBy(rows int) zenModel {
	m.rowsBelowView = max(m.rowsBelowView+rows, 0)
	return m.withVisibleRowsDrawn()
}

func (m zenModel) entriesHeight() int {
	return max(m.height-m.rowsAboveInput()-m.input.Height()-m.rowsBelowInput(), 0)
}

// A shown status gets a blank row above it, between it and the newest entry.
func (m zenModel) rowsAboveInput() int {
	if m.shownStatus() == "" {
		return 2
	}
	return 3
}

func (m zenModel) shownStatus() string {
	if m.copyStatus != "" {
		return m.copyStatus
	}
	if m.status != "" {
		return m.status
	}
	if m.endedTurnRow != "" {
		return doneStyle.Render(m.endedTurnRow)
	}
	return statusLine(m.agentStatus, m.turn, m.now, m.width)
}

func (m zenModel) rowsBelowInput() int {
	if m.claudeStatusLine == "" {
		return 0
	}
	return rowsBelowInputWithStatusLine
}

// withVisibleRowsDrawn draws entries from the newest back until the view is covered, so a start on a long tail
// draws only what the screen shows, and stops a view scrolled past the oldest entry at it.
func (m zenModel) withVisibleRowsDrawn() zenModel {
	if m.width == 0 {
		return m
	}
	rowsNeeded := m.rowsBelowView + m.entriesHeight()
	rowsDrawn := len(m.earlyPromptRows())
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
	rowsOldestFirst := m.earlyPromptRows()
	for index := len(m.entries) - 1; index >= 0 && len(rowsOldestFirst) < rowsNeeded; index-- {
		rowsOldestFirst = append(append([]string{}, m.entryRows[index]...), rowsOldestFirst...)
	}
	end := max(len(rowsOldestFirst)-m.rowsBelowView, 0)
	visible := rowsOldestFirst[max(end-height, 0):end]
	rows := make([]string, height-len(visible), height+m.rowsAboveInput()+m.input.Height()+m.rowsBelowInput())
	rows = append(rows, visible...)
	shownStatus := m.shownStatus()
	if shownStatus != "" {
		rows = append(rows, "")
	}
	separator := separatorStyle.Render(strings.Repeat("─", m.width))
	rows = append(rows, " "+shownStatus, separator)
	for _, inputRow := range strings.Split(m.input.View(), "\n") {
		rows = append(rows, " "+inputRow)
	}
	if m.claudeStatusLine != "" {
		rows = append(rows, separator, " "+ansi.Truncate(m.claudeStatusLine, max(m.width-1, 0), "")+ansi.ResetStyle)
	}
	return strings.Join(rows, "\n")
}

// draftSavingView saves the zen input's unsent text to draftFile after each update, so turning zen mode off can move
// it into Claude's input after closing the zen pane.
type draftSavingView struct {
	zen        zenModel
	draftFile  string
	savedDraft string
}

func (v draftSavingView) Init() tea.Cmd {
	return v.zen.Init()
}

func (v draftSavingView) View() tea.View {
	return v.zen.View()
}

func (v draftSavingView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := v.zen.Update(msg)
	v.zen = updated.(zenModel)
	draft := v.zen.InputText()
	if draft == v.savedDraft {
		return v, cmd
	}
	if err := zenmode.WriteWhole(v.draftFile, draft); err != nil {
		v.zen.status = errorStyle.Render(err.Error())
		return v, cmd
	}
	v.savedDraft = draft
	return v, cmd
}
