package zenmode

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/CLBRITTON2/calmpane/claudecode"
	"github.com/CLBRITTON2/calmpane/internal/herdr"
	"github.com/CLBRITTON2/calmpane/internal/ui"
)

// DraftFile holds the unsent text of the zen view over claudePaneID. A pane id holds a colon, which a Windows file
// name cannot.
func (m Mode) DraftFile(claudePaneID string) string {
	return filepath.Join(m.stateDir, "draft."+m.session+"."+strings.ReplaceAll(claudePaneID, ":", "-"))
}

// MoveDraftIntoAgent types the draft into the Claude pane's input. herdr sends a newline as a line feed, which Claude
// Code's input takes as a new line rather than a submit. Each paste goes as a bracketed paste, so Claude shows its own
// label for it.
func MoveDraftIntoAgent(api herdr.API, draftFile string, claudePaneID string) error {
	draft, err := SavedDraft(draftFile)
	if err != nil {
		return err
	}
	for _, part := range claudecode.TextParts(draft.Text, draft.Pastes) {
		text := part.Text
		if part.IsPaste {
			text = "\x1b[200~" + text + "\x1b[201~"
		}
		if err := api.SendText(claudePaneID, text); err != nil {
			return err
		}
	}
	if err := os.Remove(draftFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove the zen draft: %w", err)
	}
	return nil
}

// MoveAgentInputIntoDraft appends the unsent text of Claude's input box to draftFile and empties the box. A blocked
// Claude's input holds a question or a permission prompt, not a draft.
func MoveAgentInputIntoDraft(cli herdr.CLI, claudePane herdr.Pane, draftFile string) error {
	if claudePane.AgentStatus == "blocked" {
		return nil
	}
	agentInput, err := TakeAgentInput(cli, claudePane.PaneID)
	if err != nil || agentInput == "" {
		return err
	}
	savedDraft, err := SavedDraft(draftFile)
	if err != nil {
		return err
	}
	if savedDraft.Text == "" {
		return WriteDraft(draftFile, ui.Draft{Text: agentInput, Pastes: savedDraft.Pastes})
	}
	return WriteDraft(draftFile, ui.Draft{Text: savedDraft.Text + "\n" + agentInput, Pastes: savedDraft.Pastes})
}

// TakeAgentInput empties the unsent text of the input box in Claude pane claudePaneID and returns that text.
func TakeAgentInput(cli herdr.CLI, claudePaneID string) (string, error) {
	styledScreen := func() (string, error) { return cli.StyledScreen(claudePaneID) }
	sendCtrlU := func(count int) error { return cli.SendKeys(claudePaneID, slices.Repeat([]string{"ctrl+u"}, count)) }
	return ui.TakeClaudeInput("Claude's input in pane "+claudePaneID, styledScreen, sendCtrlU)
}

// SavedDraft is empty when the draft file is missing or empty.
func SavedDraft(draftFile string) (ui.Draft, error) {
	content, err := os.ReadFile(draftFile)
	if errors.Is(err, os.ErrNotExist) {
		return ui.Draft{}, nil
	}
	if err != nil {
		return ui.Draft{}, fmt.Errorf("Cannot read the draft %s: %w", draftFile, err)
	}
	if len(content) == 0 {
		return ui.Draft{}, nil
	}
	var draft ui.Draft
	if err := json.Unmarshal(content, &draft); err != nil {
		return ui.Draft{}, fmt.Errorf("Cannot read the draft %s: %w", draftFile, err)
	}
	return draft, nil
}

// WriteDraft writes the draft under another name and renames it over draftFile, so a reader never sees half of it.
func WriteDraft(draftFile string, draft ui.Draft) error {
	content, err := json.Marshal(draft)
	if err != nil {
		return fmt.Errorf("encode the draft %s: %w", draftFile, err)
	}
	staging := draftFile + ".tmp"
	if err := os.WriteFile(staging, content, 0o644); err != nil {
		return err
	}
	return os.Rename(staging, draftFile)
}
