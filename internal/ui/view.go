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
	"unicode"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/CLBRITTON2/calmpane/claudecode"
	"github.com/CLBRITTON2/calmpane/internal/config"
)

// herdr turns each wheel notch over a pane that does not capture the mouse into one Up or Down key.
const arrowScrollRows = 3

// The input grows a row at a time up to inputMaxRows, then scrolls inside itself.
const inputMaxRows = 8

const inputPrompt = "> "

// placedQueueOperation counts in entriesBefore the entries of the same read that the transcript holds before it.
type placedQueueOperation struct {
	claudecode.QueueOperation
	entriesBefore int
}

type entriesReadMsg struct {
	entries         []claudecode.DialogueEntry
	queueOperations []placedQueueOperation
	turn            claudecode.CurrentTurn
}

type agentStatusMsg struct {
	status AgentStatus
}

// agentInputBoxMsg reports that the agent's input box came or went while it works.
type agentInputBoxMsg struct {
	shown bool
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

// sessionChangedMsg reports that the agent switched to another session, as /clear or an in-place /resume does.
type sessionChangedMsg struct{}

// slashCommandsLoadedMsg carries the commands and skills on disk and the history of the commands zen sent.
type slashCommandsLoadedMsg struct {
	commands []claudecode.SlashCommand
	uses     []SlashUse
	err      error
}

type slashUsesWrittenMsg struct {
	err error
}

// agentPickerShownMsg reports that the agent took its input box away after a slash command, for a picker or dialog.
type agentPickerShownMsg struct {
	prompt string
}

// agentPickerClosedMsg reports that the agent's input box is back.
type agentPickerClosedMsg struct {
	prompt string
	err    error
}

// slashCommandSettledMsg reports that a slash command has had its time to reach the transcript.
type slashCommandSettledMsg struct {
	prompt string
	err    error
}

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

// expandedPaste is the text of paste number, as the expand key put it into the input at byte offset start.
type expandedPaste struct {
	number int
	start  int
	text   string
}

// copyMode holds the picked and marked code blocks as indexes into zenModel.codeBlocks.
type copyMode struct {
	picked int
	marked []int
}

const copyModeHint = "Up/Down picks a block · Space marks · Enter copies · Esc cancels"

type zenModel struct {
	agent      Agent
	appearance appearance
	// claudeScreenKey shows Claude's own screen, and the waiting row names it.
	claudeScreenKey string
	expandPasteKey  string
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
	agentStatus   AgentStatus
	// interrupting is set by an Esc sent to a working Claude, until Claude stops working.
	interrupting bool
	turn         claudecode.CurrentTurn
	now          time.Time
	clockTicking bool
	// endedTurnRow reports the turn this view saw end, until Claude works or blocks again.
	endedTurnRow string
	// endedTurnPromptTime is the PromptTime of the last Done row's turn: work that records no new turn shows none.
	endedTurnPromptTime time.Time
	// status reports the latest problem in place of the agent status row, until the agent status changes.
	status string
	// copyStatus reports a copy in place of status until the next key or status.
	copyStatus string
	// draftStatus reports a draft that could not be read, in place of status until the next key or a newer error.
	draftStatus string
	// codeBlocks holds where each code block of the replies is, oldest first. The newest one's frame carries the hint.
	codeBlocks []codeBlockPosition
	// copyMode is nil while copy mode is closed.
	copyMode *copyMode
	// earlyPrompts holds the prompts Enter sent that the transcript has not shown yet, drawn as bands under the
	// entries until a prompt entry with the same text arrives.
	earlyPrompts []earlyPrompt
	input        textarea.Model
	// pastes holds each paste the input shows as a label, numbered from 1 in order, until the input is sent. An expanded
	// paste leaves an empty string, so the later labels keep their numbers.
	pastes []string
	// expandedPastes holds the expanded pastes the expand key can still turn back into their labels: an edit inside one
	// drops it.
	expandedPastes   []expandedPaste
	claudeStatusLine string
	// slashCommands holds the commands and skills on disk, none until they are read.
	slashCommands []claudecode.SlashCommand
	slashUses     []SlashUse
	// slashHistoryFile keeps slashUses across views.
	slashHistoryFile string
	// slashMenu is nil while the menu is closed.
	slashMenu *slashMenu
	// agentPickerShown is set while the agent shows a picker or dialog in place of its input box after a slash command.
	agentPickerShown bool
	// agentInputBoxGone is set while the agent works with no input box, as when a dialog herdr does not report as
	// blocked waits for the user.
	agentInputBoxGone bool
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

func newZenModel(agent Agent, appearance appearance, claudeScreenKey string, expandPasteKey string,
	slashHistoryFile string) zenModel {
	return zenModel{agent: agent, appearance: appearance, claudeScreenKey: claudeScreenKey, expandPasteKey: expandPasteKey,
		entries: []claudecode.DialogueEntry{}, entryRows: [][]string{}, input: NewPromptInput(),
		slashCommands: []claudecode.SlashCommand{}, slashUses: []SlashUse{}, slashHistoryFile: slashHistoryFile}
}

// RunZenView shows startErr alone until a key ends the view, when it is not nil, and then follows no session, since
// settings may not hold a valid config. A draftErr shows on the status line, over an empty input.
func RunZenView(agent Agent, settings config.Config, draft Draft, draftErr error, saveDraft func(draft Draft) error,
	slashHistoryFile string, startErr error) error {
	model := newZenModel(agent, appearance{}, settings.Keys.ClaudeScreen, settings.Keys.ExpandPaste, slashHistoryFile)
	model.fatal = startErr
	if startErr == nil {
		model.appearance, model.fatal = newAppearance(settings)
	}
	model = model.withDraftRestored(draft, draftErr)
	program := tea.NewProgram(draftSavingView{zen: model, saveDraft: saveDraft, savedDraft: draft})
	if model.fatal == nil {
		go followAgentSession(agent, settings.StatusLine.Dir, program.Send)
	}
	finalModel, runErr := program.Run()
	if finalModel, isDraftSavingView := finalModel.(draftSavingView); isDraftSavingView {
		return errors.Join(finalModel.zen.fatal, runErr)
	}
	return runErr
}

// followAgentSession sends the dialogue, status and status line of the session the agent reports, and of each session
// it switches to after, until a failure it sends as fatal. An empty statusLineDir sends no status line.
func followAgentSession(agent Agent, statusLineDir string, send func(tea.Msg)) {
	sessionID, err := agent.WaitForSessionID()
	if err != nil {
		send(fatalMsg{err: err})
		return
	}
	conversationSessionIDs := make(chan string)
	if statusLineDir == "" {
		go func() {
			for range conversationSessionIDs {
			}
		}()
	} else {
		go followClaudeStatusLine(statusLineDir, conversationSessionIDs, 300*time.Millisecond, send)
	}
	transcript, err := claudecode.WaitForTranscript(sessionID, conversationSessionIDs)
	if err != nil {
		send(fatalMsg{err: err})
		return
	}
	go pollAgentStatus(agent, send)
	for {
		nextSessionID, err := followTranscript(transcript, sessionID, agent.WaitForSessionID, send)
		if err != nil {
			send(fatalMsg{err: errors.Join(fmt.Errorf("Cannot read %s: %w", transcript.Name(), err), transcript.Close())})
			return
		}
		if err := transcript.Close(); err != nil {
			send(fatalMsg{err: fmt.Errorf("Cannot close %s: %w", transcript.Name(), err)})
			return
		}
		sessionID = nextSessionID
		send(sessionChangedMsg{})
		transcript, err = claudecode.WaitForTranscript(sessionID, conversationSessionIDs)
		if err != nil {
			send(fatalMsg{err: err})
			return
		}
	}
}

// sessionCheckInterval is how often followTranscript asks the agent for its session while the transcript is quiet.
const sessionCheckInterval = time.Second

// followTranscript sends the tail's entries in one message once it reaches the end of the file, then each new
// line's entries as they are written, until reportedSessionID gives a session other than sessionID, which it returns.
func followTranscript(transcript *os.File, sessionID string, reportedSessionID func() (string, error),
	send func(tea.Msg)) (string, error) {
	info, err := transcript.Stat()
	if err != nil {
		return "", err
	}
	start, err := claudecode.TailStart(transcript, info.Size(), 2*1024*1024)
	if err != nil {
		return "", err
	}
	if _, err := transcript.Seek(start, io.SeekStart); err != nil {
		return "", err
	}
	sessionChecked := time.Now()
	reader := bufio.NewReaderSize(transcript, 1024*1024)
	var partialLine []byte
	unsentEntries := []claudecode.DialogueEntry{}
	unsentQueueOperations := []placedQueueOperation{}
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
				unsentQueueOperations = []placedQueueOperation{}
				turnChanged = false
			}
			if time.Since(sessionChecked) >= sessionCheckInterval {
				sessionChecked = time.Now()
				reported, err := reportedSessionID()
				if err != nil {
					return "", fmt.Errorf("ask the agent for its session: %w", err)
				}
				if reported != sessionID {
					return reported, nil
				}
			}
			time.Sleep(200 * time.Millisecond)
			continue
		}
		if err != nil {
			return "", err
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
				return "", countErr
			}
			send(problemMsg{err: fmt.Errorf("Cannot read line %d of %s: %w", startLine+linesRead-1,
				transcript.Name(), err)})
			continue
		}
		for _, operation := range lineQueueOperations {
			unsentQueueOperations = append(unsentQueueOperations,
				placedQueueOperation{QueueOperation: operation, entriesBefore: len(unsentEntries)})
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

// pollAgentStatus keeps polling after a failure, since the view keeps running. While the agent works it also polls
// for the input box, since a dialog in place of the box can leave the agent reported as working.
func pollAgentStatus(agent Agent, send func(tea.Msg)) {
	lastStatus := AgentUnknown
	lastInputBoxShown := true
	for {
		agentStatus, err := agent.Status()
		if err != nil {
			send(problemMsg{err: err})
		} else if agentStatus != lastStatus {
			lastStatus = agentStatus
			send(agentStatusMsg{status: agentStatus})
		}
		inputBoxShown := true
		if lastStatus == AgentWorking {
			inputBoxShown, err = agent.InputBoxShown()
			if err != nil {
				send(problemMsg{err: err})
				inputBoxShown = lastInputBoxShown
			}
		}
		if inputBoxShown != lastInputBoxShown {
			lastInputBoxShown = inputBoxShown
			send(agentInputBoxMsg{shown: inputBoxShown})
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func statusLine(agentStatus AgentStatus, turn claudecode.CurrentTurn, now time.Time, width int, appearance appearance,
	claudeScreenKey string) string {
	switch agentStatus {
	case AgentBlocked:
		// wrapped, since the key to press comes last; each row keeps the one column of left margin
		waiting := "Waiting for you in Claude. Press " + claudeScreenKey + " to answer."
		return appearance.waitingStyle.Render(ansi.Wrap(waiting, max(width-1, 1), ""))
	case AgentWorking:
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
		return appearance.workingStyle.Render(ansi.Truncate(activity, activityWidth, "") + duration)
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
	return tea.Batch(textarea.Blink, loadSlashCommands(m.agent, m.slashHistoryFile))
}

// loadSlashCommands reads the commands and skills Claude loads from disk for its working directory, and the history.
func loadSlashCommands(agent Agent, slashHistoryFile string) tea.Cmd {
	return func() tea.Msg {
		uses, usesErr := SavedSlashUses(slashHistoryFile)
		directory, err := agent.WorkingDirectory()
		if err != nil {
			return slashCommandsLoadedMsg{commands: nil, uses: uses, err: errors.Join(usesErr, err)}
		}
		commands, err := claudecode.CustomSlashCommands(directory)
		return slashCommandsLoadedMsg{commands: commands, uses: uses, err: errors.Join(usesErr, err)}
	}
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
			m.status = m.appearance.errorStyle.Render(msg.err.Error())
			m.copyStatus = ""
			m.draftStatus = ""
		}
		return m, nil
	case agentStatusMsg:
		turnEnded := m.agentStatus == AgentWorking && (msg.status == AgentIdle || msg.status == AgentDone)
		m.agentStatus = msg.status
		m.status = ""
		m.copyStatus = ""
		m.endedTurnRow = ""
		if msg.status != AgentWorking && m.interrupting {
			m.interrupting = false
			return m, func() tea.Msg {
				text, err := m.agent.TakeInput()
				return agentInputTakenMsg{text: text, err: err}
			}
		}
		if turnEnded {
			return m, func() tea.Msg { return turnEndedMsg{now: time.Now()} }
		}
		if msg.status != AgentWorking || m.clockTicking {
			return m, nil
		}
		m.clockTicking = true
		return m, func() tea.Msg { return clockTickMsg{now: time.Now()} }
	case clockTickMsg:
		m.now = msg.now
		if m.agentStatus != AgentWorking {
			m.clockTicking = false
			return m, nil
		}
		return m, tea.Every(time.Second, func(now time.Time) tea.Msg { return clockTickMsg{now: now} })
	case turnEndedMsg:
		if m.agentStatus == AgentWorking || m.agentStatus == AgentBlocked {
			return m, nil
		}
		if m.turn.PromptTime.Equal(m.endedTurnPromptTime) {
			return m, nil
		}
		m.endedTurnPromptTime = m.turn.PromptTime
		m.endedTurnRow = "Done · " + TurnDuration(max(msg.now.Sub(m.turn.PromptTime), 0))
		return m.withVisibleRowsDrawn(), nil
	case agentInputTakenMsg:
		if msg.err != nil {
			m.status = m.appearance.errorStyle.Render(msg.err.Error())
			m.copyStatus = ""
			m.draftStatus = ""
			return m, nil
		}
		return m.withCancelledPromptRestored(msg.text), nil
	case problemMsg:
		m.status = m.appearance.errorStyle.Render(msg.err.Error())
		m.copyStatus = ""
		m.draftStatus = ""
		return m, nil
	case fatalMsg:
		m.fatal = msg.err
		return m, nil
	case claudeStatusLineMsg:
		m.claudeStatusLine = msg.line
		return m.withVisibleRowsDrawn(), nil
	case sessionChangedMsg:
		m.entries = []claudecode.DialogueEntry{}
		m.entryRows = [][]string{}
		m.codeBlocks = nil
		m.copyMode = nil
		m.earlyPrompts = nil
		m.rowsBelowView = 0
		m.turn = claudecode.CurrentTurn{}
		m.endedTurnRow = ""
		m.endedTurnPromptTime = time.Time{}
		return m.withVisibleRowsDrawn(), nil
	case slashCommandsLoadedMsg:
		if msg.err != nil {
			m.status = m.appearance.errorStyle.Render(msg.err.Error())
		}
		m.slashUses = msg.uses
		m.slashCommands = msg.commands
		return m.withVisibleRowsDrawn(), nil
	case slashUsesWrittenMsg:
		if msg.err != nil {
			m.status = m.appearance.errorStyle.Render(msg.err.Error())
		}
		return m, nil
	case agentInputBoxMsg:
		m.agentInputBoxGone = !msg.shown
		return m.withVisibleRowsDrawn(), nil
	case agentPickerShownMsg:
		m.agentPickerShown = true
		return m.withVisibleRowsDrawn(), waitForAgentInputBox(m.agent, msg.prompt)
	case agentPickerClosedMsg:
		m.agentPickerShown = false
		if msg.err != nil {
			m.status = m.appearance.errorStyle.Render(msg.err.Error())
		}
		prompt := msg.prompt
		return m.withVisibleRowsDrawn(), tea.Tick(time.Second, func(time.Time) tea.Msg {
			return slashCommandSettledMsg{prompt: prompt}
		})
	case slashCommandSettledMsg:
		if msg.err != nil {
			m.status = m.appearance.errorStyle.Render(msg.err.Error())
		}
		isUnqueuedWithText := func(early earlyPrompt) bool { return !early.queued && early.text == msg.prompt }
		return m.withEarlyPromptRemovedAt(slices.IndexFunc(m.earlyPrompts, isUnqueuedWithText)), nil
	case promptSentMsg:
		m.copyStatus = ""
		m.draftStatus = ""
		switch {
		case errors.Is(msg.err, ErrAgentBlocked):
			m.status = m.appearance.errorStyle.Render("Claude is waiting for you. Press " + m.claudeScreenKey + " to answer.")
		case msg.err != nil:
			m.status = m.appearance.errorStyle.Render(msg.err.Error())
		}
		if msg.err != nil {
			return m.withPromptPutBack(msg.prompt, msg.pastes), nil
		}
		if !strings.HasPrefix(msg.prompt, "/") {
			return m, nil
		}
		return m.withSlashCommandSent(strings.TrimSpace(msg.prompt))
	case tea.KeyPressMsg:
		if m.fatal != nil {
			return m, tea.Quit
		}
		m.copyStatus = ""
		m.draftStatus = ""
		return m.withKey(msg)
	case tea.PasteMsg:
		return m.withPasteInserted(msg.Content)
	}
	var inputCmd tea.Cmd
	m.input, inputCmd = m.input.Update(msg)
	return m, inputCmd
}

// withSlashCommandSent counts the command in the history under its name as typed, unless the name holds a / and so is
// a path such as /tmp/x, and watches for the picker or dialog the command may show in place of Claude's input box.
func (m zenModel) withSlashCommandSent(command string) (zenModel, tea.Cmd) {
	watchForPicker := watchForAgentPicker(m.agent, command)
	name := strings.TrimPrefix(strings.Fields(command)[0], "/")
	if name == "" || strings.Contains(name, "/") {
		return m, watchForPicker
	}
	m.slashUses = SlashUsesAfterUse(m.slashUses, name, time.Now())
	uses, historyFile := m.slashUses, m.slashHistoryFile
	writeUses := func() tea.Msg { return slashUsesWrittenMsg{err: WriteSlashUses(historyFile, uses)} }
	return m, tea.Batch(watchForPicker, writeUses)
}

// agentPickerWait is how long after a slash command a picker or dialog may take to replace Claude's input box.
const agentPickerWait = 3 * time.Second

// watchForAgentPicker reports the agent's input box gone within agentPickerWait of the command, or the command
// settled when it stays.
func watchForAgentPicker(agent Agent, command string) tea.Cmd {
	return func() tea.Msg {
		deadline := time.Now().Add(agentPickerWait)
		for time.Now().Before(deadline) {
			shown, err := agent.InputBoxShown()
			if err != nil {
				return slashCommandSettledMsg{prompt: command, err: err}
			}
			if !shown {
				return agentPickerShownMsg{prompt: command}
			}
			time.Sleep(200 * time.Millisecond)
		}
		return slashCommandSettledMsg{prompt: command}
	}
}

func waitForAgentInputBox(agent Agent, command string) tea.Cmd {
	return func() tea.Msg {
		for {
			shown, err := agent.InputBoxShown()
			if err != nil || shown {
				return agentPickerClosedMsg{prompt: command, err: err}
			}
			time.Sleep(300 * time.Millisecond)
		}
	}
}

func (m zenModel) withKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.copyMode != nil {
		return m.withCopyModeKey(msg)
	}
	if m.slashMenu != nil {
		if model, cmd, handled := m.withSlashMenuKey(msg); handled {
			return model, cmd
		}
	}
	switch msg.String() {
	case "pgup":
		return m.withRowsBelowViewChangedBy(m.entriesHeight()), nil
	case "pgdown":
		return m.withRowsBelowViewChangedBy(-m.entriesHeight()), nil
	case "up":
		if m.input.Line() == 0 && m.input.LineInfo().RowOffset == 0 {
			return m.withRowsBelowViewChangedBy(arrowScrollRows), nil
		}
	case "down":
		cursorRow := m.input.LineInfo()
		if m.input.Line() == m.input.LineCount()-1 && cursorRow.RowOffset == cursorRow.Height-1 {
			return m.withRowsBelowViewChangedBy(-arrowScrollRows), nil
		}
	case "end":
		return m.withRowsBelowViewChangedBy(-m.rowsBelowView), nil
	case "ctrl+y":
		if len(m.codeBlocks) == 0 {
			m.copyStatus = m.appearance.workingStyle.Render("No code block to copy.")
			return m, nil
		}
		newest := len(m.codeBlocks) - 1
		m.copyMode = &copyMode{picked: newest, marked: []int{}}
		return m.withEntryRedrawn(m.codeBlocks[newest].entry).withPickedCodeBlockInView(), nil
	case "enter":
		prompt := m.InputText()
		if prompt == "" {
			return m, nil
		}
		pastes := m.pastes
		sendPrompt := func() tea.Msg {
			return promptSentMsg{prompt: prompt, pastes: pastes, err: m.agent.SendPrompt(prompt)}
		}
		return m.withInputSent(prompt), sendPrompt
	case "ctrl+enter":
		prompt := m.InputText()
		pastes := m.pastes
		if prompt == "" {
			return m, func() tea.Msg {
				return claudeKeysSentMsg{err: m.agent.SendNow()}
			}
		}
		sendPromptAndSendNow := func() tea.Msg {
			if err := m.agent.SendPrompt(prompt); err != nil {
				return promptSentMsg{prompt: prompt, pastes: pastes, err: err}
			}
			return claudeKeysSentMsg{err: m.agent.SendNow()}
		}
		return m.withInputSent(prompt), sendPromptAndSendNow
	case "esc":
		// Claude takes Esc twice in a row while idle as the rewind menu, which would wait unseen behind the zen view
		if m.agentStatus != AgentWorking {
			m.pastes = nil
			m.expandedPastes = nil
			m.input.Reset()
			return m.withVisibleRowsDrawn(), nil
		}
		m.interrupting = true
		return m, func() tea.Msg {
			return claudeKeysSentMsg{err: m.agent.Interrupt()}
		}
	case m.expandPasteKey:
		cursor := m.inputCursorOffset()
		collapsible := slices.IndexFunc(m.expandedPastes, func(expanded expandedPaste) bool {
			return expanded.start <= cursor && cursor < expanded.start+len(expanded.text)
		})
		if collapsible < 0 {
			collapsible = slices.IndexFunc(m.expandedPastes, func(expanded expandedPaste) bool {
				return cursor == expanded.start+len(expanded.text)
			})
		}
		if collapsible < 0 {
			return m.withPasteLabelAtCursorExpanded(cursor), nil
		}
		return m.withExpandedPasteCollapsed(collapsible, cursor), nil
	}
	formerInput := m.input.Value()
	formerCursor := m.inputCursorOffset()
	var inputCmd tea.Cmd
	m.input, inputCmd = m.input.Update(msg)
	// the entries take the rows the input gives up
	return m.withExpandedPastesFollowingEdit(formerInput, formerCursor).withSlashMenuFollowingInput(formerInput).
		withVisibleRowsDrawn(), inputCmd
}

// withSlashMenuFollowingInput opens the menu on a / typed into the empty input when it has a command to list, and
// closes it once the input no longer starts with / or holds a space. A changed query selects the best match.
func (m zenModel) withSlashMenuFollowingInput(formerInput string) zenModel {
	input := m.input.Value()
	switch {
	case m.slashMenu == nil:
		hasCommands := len(m.slashCommands) > 0 || len(m.slashUses) > 0
		if formerInput == "" && input == "/" && hasCommands {
			m.slashMenu = &slashMenu{selected: 0, openedAt: time.Now()}
		}
	case !strings.HasPrefix(input, "/") || strings.ContainsFunc(input, unicode.IsSpace):
		m.slashMenu = nil
	case input != formerInput:
		m.slashMenu = &slashMenu{selected: 0, openedAt: m.slashMenu.openedAt}
	}
	return m
}

// withSlashMenuKey handles the keys the open menu takes, and reports false for the others, which go to the input.
func (m zenModel) withSlashMenuKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	matches := m.slashMatches()
	switch msg.String() {
	case "up":
		m.slashMenu = &slashMenu{selected: max(m.slashMenu.selected-1, 0), openedAt: m.slashMenu.openedAt}
		return m, nil, true
	case "down":
		m.slashMenu = &slashMenu{selected: max(min(m.slashMenu.selected+1, len(matches)-1), 0),
			openedAt: m.slashMenu.openedAt}
		return m, nil, true
	case "tab":
		if len(matches) == 0 {
			return m, nil, true
		}
		m.input.SetValue("/" + matches[m.slashMenu.selected].command.Name + " ")
		m.slashMenu = nil
		return m.withVisibleRowsDrawn(), nil, true
	case "enter":
		if len(matches) > 0 {
			m.input.SetValue("/" + matches[m.slashMenu.selected].command.Name)
		}
		m.slashMenu = nil
		model, cmd := m.withKey(msg)
		return model, cmd, true
	case "esc":
		m.slashMenu = nil
		return m.withVisibleRowsDrawn(), nil, true
	}
	return m, nil, false
}

// slashMatches are the commands the open menu lists for the input after its /.
func (m zenModel) slashMatches() []slashMatch {
	return RankedSlashCommands(m.slashCommands, m.slashUses, strings.TrimPrefix(m.input.Value(), "/"),
		m.slashMenu.openedAt)
}

// withCopyModeKey leaves the input and Claude alone: a key copy mode does not use does nothing.
func (m zenModel) withCopyModeKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	picked := m.copyMode.picked
	switch msg.String() {
	case "up":
		if picked > 0 {
			return m.withCodeBlockPicked(picked - 1), nil
		}
	case "down":
		if picked < len(m.codeBlocks)-1 {
			return m.withCodeBlockPicked(picked + 1), nil
		}
	case "space":
		marked := slices.Clone(m.copyMode.marked)
		if at := slices.Index(marked, picked); at >= 0 {
			marked = slices.Delete(marked, at, at+1)
		} else {
			marked = append(marked, picked)
		}
		m.copyMode = &copyMode{picked: picked, marked: marked}
		return m.withEntryRedrawn(m.codeBlocks[picked].entry), nil
	case "enter":
		return m.withCodeBlocksCopied()
	case "esc":
		return m.withCopyModeClosed(), nil
	}
	return m, nil
}

func (m zenModel) withCodeBlockPicked(index int) zenModel {
	formerEntry := m.codeBlocks[m.copyMode.picked].entry
	m.copyMode = &copyMode{picked: index, marked: m.copyMode.marked}
	return m.withEntryRedrawn(formerEntry).withEntryRedrawn(m.codeBlocks[index].entry).withPickedCodeBlockInView()
}

// withCodeBlocksCopied copies the marked blocks oldest first, or the picked block when none is marked, and closes copy
// mode.
func (m zenModel) withCodeBlocksCopied() (zenModel, tea.Cmd) {
	copiedBlocks := slices.Sorted(slices.Values(m.copyMode.marked))
	if len(copiedBlocks) == 0 {
		copiedBlocks = []int{m.copyMode.picked}
	}
	codes := []string{}
	for _, index := range copiedBlocks {
		position := m.codeBlocks[index]
		replyBlocks, err := CodeBlocks(m.entries[position.entry].Text)
		if err != nil {
			m.status = m.appearance.errorStyle.Render(err.Error())
			return m.withCopyModeClosed(), nil
		}
		codes = append(codes, replyBlocks[position.block].code)
	}
	if len(codes) == 1 {
		m.copyStatus = m.appearance.workingStyle.Render("Copied the code block.")
	} else {
		m.copyStatus = m.appearance.workingStyle.Render(fmt.Sprintf("Copied %d code blocks.", len(codes)))
	}
	return m.withCopyModeClosed(), tea.SetClipboard(strings.Join(codes, "\n\n"))
}

func (m zenModel) withCopyModeClosed() zenModel {
	framedEntries := []int{m.codeBlocks[m.copyMode.picked].entry}
	for _, index := range m.copyMode.marked {
		framedEntries = append(framedEntries, m.codeBlocks[index].entry)
	}
	m.copyMode = nil
	for _, entry := range framedEntries {
		m = m.withEntryRedrawn(entry)
	}
	return m.withVisibleRowsDrawn()
}

// withPickedCodeBlockInView scrolls the least that shows the picked block's whole frame, or puts its top rule at the
// top of the view when the frame is taller than the view.
func (m zenModel) withPickedCodeBlockInView() zenModel {
	entry := m.codeBlocks[m.copyMode.picked].entry
	rowsUnderEntry := len(m.earlyPromptRows())
	for index := len(m.entries) - 1; index > entry; index-- {
		m = m.withEntryDrawn(index)
		rowsUnderEntry += len(m.entryRows[index])
	}
	m = m.withEntryDrawn(entry)
	rows := m.entryRows[entry]
	// only the picked frame has heavy corners, and a frame's rules are the rows not indented
	pickedTopRule, pickedBottomRule := " "+pickedFrameRules.topLeft, " "+pickedFrameRules.bottomLeft
	topRule := slices.IndexFunc(rows, func(row string) bool { return strings.HasPrefix(ansi.Strip(row), pickedTopRule) })
	if topRule < 0 {
		return m.withVisibleRowsDrawn()
	}
	frameHeight := slices.IndexFunc(rows[topRule:], func(row string) bool {
		return strings.HasPrefix(ansi.Strip(row), pickedBottomRule)
	}) + 1
	rowsUnderFrame := rowsUnderEntry + len(rows) - topRule - frameHeight
	height := m.entriesHeight()
	switch {
	case frameHeight <= height && rowsUnderFrame < m.rowsBelowView:
		m.rowsBelowView = rowsUnderFrame
	case frameHeight > height || rowsUnderFrame+frameHeight > m.rowsBelowView+height:
		m.rowsBelowView = rowsUnderFrame + frameHeight - height
	}
	return m.withVisibleRowsDrawn()
}

// withPromptPutBack shows the prompt's pastes and those typed since as labels again, renumbered in order.
func (m zenModel) withPromptPutBack(prompt string, pastes []string) zenModel {
	restored := prompt
	if typedSince := m.InputText(); typedSince != "" {
		restored += "\n" + typedSince
	}
	m.pastes = append(slices.Clone(pastes), m.pastes...)
	m.expandedPastes = nil
	m.input.SetValue(claudecode.TextWithPastesCollapsed(restored, m.pastes))
	return m.withEarlyPromptRemoved(strings.TrimSpace(prompt))
}

// inputCursorOffset is the byte offset of the input's cursor in its value.
func (m zenModel) inputCursorOffset() int {
	lines := strings.Split(m.input.Value(), "\n")
	cursor := len(string([]rune(lines[m.input.Line()])[:m.input.Column()]))
	for _, line := range lines[:m.input.Line()] {
		cursor += len(line) + 1
	}
	return cursor
}

// withPasteLabelAtCursorExpanded replaces the label the cursor is on, or else the one directly before it, with its
// paste as typed text, and puts the cursor after that text.
func (m zenModel) withPasteLabelAtCursorExpanded(cursor int) zenModel {
	value := m.input.Value()
	expanded := claudecode.TextPart{}
	labelStart, labelEnd := 0, 0
	partStart := 0
	for _, part := range claudecode.TextParts(value, m.pastes) {
		partEnd := partStart + len(part.Text)
		if part.IsPaste {
			partEnd = partStart + len(claudecode.PasteLabel(part.Number, part.Text))
		}
		cursorOnLabel := part.IsPaste && partStart <= cursor && cursor < partEnd
		if cursorOnLabel || (part.IsPaste && cursor == partEnd) {
			expanded, labelStart, labelEnd = part, partStart, partEnd
		}
		if cursorOnLabel {
			break
		}
		partStart = partEnd
	}
	if !expanded.IsPaste {
		return m
	}
	m.pastes = slices.Clone(m.pastes)
	m.pastes[expanded.Number-1] = ""
	m.input.SetValue(value[labelEnd:])
	m.input.MoveToBegin()
	m.input.InsertString(value[:labelStart] + expanded.Text)
	m = m.withExpandedPastesFollowingEdit(value, cursor)
	m.expandedPastes = append(m.expandedPastes, expandedPaste{number: expanded.Number, start: labelStart, text: expanded.Text})
	return m.withVisibleRowsDrawn()
}

func (m zenModel) withExpandedPasteCollapsed(index int, cursor int) zenModel {
	collapsed := m.expandedPastes[index]
	value := m.input.Value()
	m.pastes = slices.Clone(m.pastes)
	m.pastes[collapsed.number-1] = collapsed.text
	m.expandedPastes = slices.Delete(slices.Clone(m.expandedPastes), index, index+1)
	m.input.SetValue(value[collapsed.start+len(collapsed.text):])
	m.input.MoveToBegin()
	m.input.InsertString(value[:collapsed.start] + claudecode.PasteLabel(collapsed.number, collapsed.text))
	return m.withExpandedPastesFollowingEdit(value, cursor).withVisibleRowsDrawn()
}

// withExpandedPastesFollowingEdit moves each expanded paste after the edit that turned formerInput into the input, and
// drops the ones the edit reached into. The edit is what lies between the text the two values share before and after
// the cursor, since a key edits the input at its cursor and the shared text alone can match either side of a repeat.
func (m zenModel) withExpandedPastesFollowingEdit(formerInput string, formerCursor int) zenModel {
	input := m.input.Value()
	if input == formerInput || len(m.expandedPastes) == 0 {
		return m
	}
	cursor := m.inputCursorOffset()
	sharedPrefix := 0
	for sharedPrefix < min(formerCursor, cursor) && formerInput[sharedPrefix] == input[sharedPrefix] {
		sharedPrefix++
	}
	sharedSuffix := 0
	for sharedSuffix < min(len(formerInput)-formerCursor, len(input)-cursor) &&
		formerInput[len(formerInput)-1-sharedSuffix] == input[len(input)-1-sharedSuffix] {
		sharedSuffix++
	}
	editStart, editEnd := sharedPrefix, len(formerInput)-sharedSuffix
	followed := []expandedPaste{}
	for _, expanded := range m.expandedPastes {
		end := expanded.start + len(expanded.text)
		editReachesIn := editStart < end && editEnd > expanded.start
		if editReachesIn {
			continue
		}
		if editEnd <= expanded.start {
			expanded.start += len(input) - len(formerInput)
		}
		followed = append(followed, expanded)
	}
	m.expandedPastes = followed
	return m
}

func (m zenModel) InputText() string {
	return claudecode.TextWithPastesExpanded(m.input.Value(), m.pastes)
}

// withDraftRestored shows the draft's pastes as their labels, as when they were pasted, and draftErr on the status line.
func (m zenModel) withDraftRestored(draft Draft, draftErr error) zenModel {
	if draftErr != nil {
		m.draftStatus = m.appearance.errorStyle.Render(draftErr.Error())
	}
	m.input.SetValue(draft.Text)
	m.pastes = draft.Pastes
	return m
}

func (m zenModel) withPasteInserted(content string) (zenModel, tea.Cmd) {
	paste := claudecode.NormalizedPaste(content)
	formerInput := m.input.Value()
	formerCursor := m.inputCursorOffset()
	if !claudecode.IsShownAsLabel(paste) {
		var inputCmd tea.Cmd
		m.input, inputCmd = m.input.Update(tea.PasteMsg{Content: content})
		return m.withExpandedPastesFollowingEdit(formerInput, formerCursor).withVisibleRowsDrawn(), inputCmd
	}
	m.pastes = append(slices.Clone(m.pastes), paste)
	m.input.InsertString(claudecode.PasteLabel(len(m.pastes), paste))
	return m.withExpandedPastesFollowingEdit(formerInput, formerCursor).withVisibleRowsDrawn(), nil
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
	m.expandedPastes = nil
	m.input.Reset()
	m = m.withVisibleRowsDrawn()
	bandText := strings.TrimSpace(prompt)
	if bandText == "" {
		return m
	}
	return m.withEarlyPromptAdded(earlyPrompt{text: bandText, pastes: pastes})
}

// withSize redraws every entry when the width changes, since prompts and markdown wrap to it.
func (m zenModel) withSize(width int, height int) zenModel {
	if width != m.width {
		replyRenderer, err := NewReplyRenderer(m.appearance, width)
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
func (m zenModel) withEntries(entries []claudecode.DialogueEntry, queueOperations []placedQueueOperation) zenModel {
	firstNew := len(m.entries)
	m.entries = append(m.entries, entries...)
	m.entryRows = append(m.entryRows, make([][]string, len(entries))...)
	newPositions, err := CodeBlockPositions(entries)
	if err != nil {
		m.status = m.appearance.errorStyle.Render(err.Error())
	}
	if len(newPositions) > 0 {
		formerCodeBlocks := m.codeBlocks
		for _, position := range newPositions {
			m.codeBlocks = append(m.codeBlocks, codeBlockPosition{entry: firstNew + position.entry, block: position.block})
		}
		// the former newest block's frame loses the copy hint, in the same number of rows
		if len(formerCodeBlocks) > 0 {
			m = m.withEntryRedrawn(formerCodeBlocks[len(formerCodeBlocks)-1].entry)
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
		for ; nextOperation < len(queueOperations) && queueOperations[nextOperation].entriesBefore <= index; nextOperation++ {
			m = m.withQueueOperation(queueOperations[nextOperation].QueueOperation)
		}
		if entry.Kind != claudecode.PromptEntry && entry.Kind != claudecode.CommandEntry {
			continue
		}
		if bandIndex := m.earlyPromptIndex(entry.Text); bandIndex >= 0 {
			m = m.withEntryPastes(firstNew+index, m.earlyPrompts[bandIndex].pastes).withEarlyPromptRemovedAt(bandIndex)
		}
	}
	for _, operation := range queueOperations[nextOperation:] {
		m = m.withQueueOperation(operation.QueueOperation)
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
		rows = append(rows, PromptEntryRows(claudecode.TextWithPastesCollapsed(early.text, early.pastes), note, m.width,
			m.appearance)...)
	}
	return rows
}

func (m zenModel) withRowsBelowViewChangedBy(rows int) zenModel {
	m.rowsBelowView = max(m.rowsBelowView+rows, 0)
	return m.withVisibleRowsDrawn()
}

func (m zenModel) entriesHeight() int {
	return max(m.height-m.rowsAboveInput()-m.input.Height()-len(m.rowsBelowInput()), 0)
}

// A shown status gets a blank row above it, between it and the newest entry.
func (m zenModel) rowsAboveInput() int {
	shownStatus := m.shownStatus()
	if shownStatus == "" {
		return 2
	}
	return 2 + len(strings.Split(shownStatus, "\n"))
}

func (m zenModel) shownStatus() string {
	if m.copyMode != nil {
		return m.appearance.workingStyle.Render(copyModeHint)
	}
	if m.copyStatus != "" {
		return m.copyStatus
	}
	waitingOnDialogReportedAsWorking := m.agentStatus == AgentWorking && m.agentInputBoxGone
	if m.agentPickerShown || waitingOnDialogReportedAsWorking {
		return statusLine(AgentBlocked, m.turn, m.now, m.width, m.appearance, m.claudeScreenKey)
	}
	if m.draftStatus != "" {
		return m.draftStatus
	}
	if m.status != "" {
		return m.status
	}
	if m.endedTurnRow != "" {
		return m.appearance.doneStyle.Render(m.endedTurnRow)
	}
	return statusLine(m.agentStatus, m.turn, m.now, m.width, m.appearance, m.claudeScreenKey)
}

// rowsBelowInput are the open slash menu's rows, or Claude's status line, under the separator that closes the input.
func (m zenModel) rowsBelowInput() []string {
	separator := m.appearance.separatorStyle.Render(strings.Repeat(m.appearance.separatorLine, m.width))
	if m.slashMenu != nil {
		return append([]string{separator}, slashMenuRows(m.slashMatches(), m.slashMenu.selected, m.width,
			m.appearance)...)
	}
	if m.claudeStatusLine == "" {
		return []string{}
	}
	return []string{separator, " " + ansi.Truncate(m.claudeStatusLine, max(m.width-1, 0), "") + ansi.ResetStyle}
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
	rows, err := EntryRows(m.entries[index], m.codeFrames(index), m.width, m.replyRenderer, m.appearance)
	if err != nil {
		m.status = m.appearance.errorStyle.Render(err.Error())
		rows = []string{}
	}
	m.entryRows[index] = rows
	return m
}

// withEntryRedrawn redraws an entry drawn before, in the same number of rows, and leaves one not drawn yet to be drawn
// when it shows.
func (m zenModel) withEntryRedrawn(index int) zenModel {
	if m.entryRows[index] == nil {
		return m
	}
	m.entryRows = slices.Clone(m.entryRows)
	m.entryRows[index] = nil
	return m.withEntryDrawn(index)
}

// codeFrames holds how each code frame of the entry at index is drawn, by its number in the entry's reply.
func (m zenModel) codeFrames(index int) map[int]codeFrame {
	frames := map[int]codeFrame{}
	for blockIndex, position := range m.codeBlocks {
		if position.entry != index {
			continue
		}
		frame := codeFrame{copyHint: blockIndex == len(m.codeBlocks)-1}
		if m.copyMode != nil {
			frame.picked = blockIndex == m.copyMode.picked
			frame.marked = slices.Contains(m.copyMode.marked, blockIndex)
		}
		frames[position.block] = frame
	}
	return frames
}

func (m zenModel) View() tea.View {
	var view tea.View
	switch {
	case m.fatal != nil:
		// wrapped, since a terminal cuts a row at the pane's edge, and a config error names the field after the file
		view = tea.NewView(lipgloss.NewStyle().Width(m.width).Render(m.fatal.Error()))
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
	rows := make([]string, height-len(visible), height+m.rowsAboveInput()+m.input.Height()+len(m.rowsBelowInput()))
	rows = append(rows, visible...)
	shownStatus := m.shownStatus()
	if shownStatus != "" {
		rows = append(rows, "")
	}
	separator := m.appearance.separatorStyle.Render(strings.Repeat(m.appearance.separatorLine, m.width))
	for _, statusRow := range strings.Split(shownStatus, "\n") {
		rows = append(rows, " "+statusRow)
	}
	rows = append(rows, separator)
	for _, inputRow := range strings.Split(m.input.View(), "\n") {
		rows = append(rows, " "+inputRow)
	}
	rows = append(rows, m.rowsBelowInput()...)
	return strings.Join(rows, "\n")
}

// Draft is the zen input's unsent text as shown, with each paste as its label, and the pastes, numbered from 1 in
// order.
type Draft struct {
	Text   string   `json:"text"`
	Pastes []string `json:"pastes"`
}

// draftSavingView saves the zen input's unsent text with saveDraft after each update, so turning zen mode off can move
// it into Claude's input after closing the zen pane.
type draftSavingView struct {
	zen        zenModel
	saveDraft  func(draft Draft) error
	savedDraft Draft
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
	draft := Draft{Text: v.zen.input.Value(), Pastes: v.zen.pastes}
	if draft.Text == v.savedDraft.Text && slices.Equal(draft.Pastes, v.savedDraft.Pastes) {
		return v, cmd
	}
	if err := v.saveDraft(draft); err != nil {
		v.zen.status = v.zen.appearance.errorStyle.Render(err.Error())
		return v, cmd
	}
	v.savedDraft = draft
	return v, cmd
}
