package zenmode

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/CLBRITTON2/herdr-zen/claudecode"
	"github.com/CLBRITTON2/herdr-zen/internal/herdr"
)

// DraftFile holds the unsent text of the zen view over claudePaneID. A pane id holds a colon, which a Windows file
// name cannot.
func (m Mode) DraftFile(claudePaneID string) string {
	return filepath.Join(m.stateDir, "draft."+m.session+"."+strings.ReplaceAll(claudePaneID, ":", "-"))
}

// MoveDraftIntoAgent types the draft into the Claude pane's input. herdr sends a newline as a line feed, which Claude
// Code's input takes as a new line rather than a submit.
func MoveDraftIntoAgent(cli herdr.CLI, draftFile string, claudePaneID string) error {
	draft, err := SavedDraft(draftFile)
	if err != nil {
		return err
	}
	if len(draft) > 0 {
		if err := cli.SendText(claudePaneID, draft); err != nil {
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
	if savedDraft == "" {
		return WriteWhole(draftFile, agentInput)
	}
	return WriteWhole(draftFile, savedDraft+"\n"+agentInput)
}

// TakeAgentInput empties the unsent text of the input box in Claude pane claudePaneID and returns that text.
func TakeAgentInput(cli herdr.CLI, claudePaneID string) (string, error) {
	screen, err := cli.StyledScreen(claudePaneID)
	if err != nil {
		return "", err
	}
	box := claudecode.InputBoxOnScreen(screen)
	if box.Draft == "" {
		return "", nil
	}
	// Claude Code 2.1.329: ctrl+u deletes back to the start of the cursor's row or the line break before it, so twice
	// the rows empties the box. ctrl+c would interrupt a working turn, and ctrl+l only redraws.
	if err := cli.SendKeys(claudePaneID, slices.Repeat([]string{"ctrl+u"}, 2*box.Rows)); err != nil {
		return "", err
	}
	// Claude takes the keys after herdr returns
	deadline := time.Now().Add(2 * time.Second)
	for {
		screen, err := cli.StyledScreen(claudePaneID)
		if err != nil {
			return "", err
		}
		if claudecode.InputBoxOnScreen(screen).Draft == "" {
			return box.Draft, nil
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("Claude's input in pane %s still holds text after ctrl+u, so zen took none of it: ctrl+y "+
				"in Claude pastes back what ctrl+u deleted", claudePaneID)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// SavedDraft is empty when there is no draft file.
func SavedDraft(draftFile string) (string, error) {
	draft, err := os.ReadFile(draftFile)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read the zen draft: %w", err)
	}
	return string(draft), nil
}

// WriteWhole writes text under another name and renames it over file, so a reader never sees half of it.
func WriteWhole(file string, text string) error {
	staging := file + ".tmp"
	if err := os.WriteFile(staging, []byte(text), 0o644); err != nil {
		return err
	}
	return os.Rename(staging, file)
}
