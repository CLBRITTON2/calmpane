package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// DraftFile holds the unsent text of the zen view over claudePaneID. A pane id holds a colon, which a Windows file
// name cannot.
func (m zenMode) DraftFile(claudePaneID string) string {
	return filepath.Join(m.stateDir, "draft."+m.session+"."+strings.ReplaceAll(claudePaneID, ":", "-"))
}

// MoveDraftIntoAgent types the draft into the Claude pane's input. herdr sends a newline as a line feed, which Claude
// Code's input takes as a new line rather than a submit.
func MoveDraftIntoAgent(herdr herdrCLI, draftFile string, claudePaneID string) error {
	draft, err := SavedDraft(draftFile)
	if err != nil {
		return err
	}
	if len(draft) > 0 {
		if err := herdr.SendText(claudePaneID, draft); err != nil {
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
func MoveAgentInputIntoDraft(herdr herdrCLI, claudePane herdrPane, draftFile string) error {
	if claudePane.AgentStatus == "blocked" {
		return nil
	}
	screen, err := herdr.StyledScreen(claudePane.PaneID)
	if err != nil {
		return err
	}
	box := ClaudeInputBoxOnScreen(screen)
	if box.Draft == "" {
		return nil
	}
	// Claude Code 2.1.329: ctrl+u deletes back to the start of the cursor's row or the line break before it, so twice
	// the rows empties the box. ctrl+c would interrupt a working turn, and ctrl+l only redraws.
	if err := herdr.SendKeys(claudePane.PaneID, slices.Repeat([]string{"ctrl+u"}, 2*box.Rows)); err != nil {
		return err
	}
	// Claude takes the keys after herdr returns
	deadline := time.Now().Add(2 * time.Second)
	for {
		screen, err := herdr.StyledScreen(claudePane.PaneID)
		if err != nil {
			return err
		}
		if ClaudeInputBoxOnScreen(screen).Draft == "" {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("Claude's input in pane %s still holds text after ctrl+u, so zen took none of it: ctrl+y "+
				"in Claude pastes back what ctrl+u deleted", claudePane.PaneID)
		}
		time.Sleep(100 * time.Millisecond)
	}
	savedDraft, err := SavedDraft(draftFile)
	if err != nil {
		return err
	}
	if savedDraft == "" {
		return WriteWhole(draftFile, box.Draft)
	}
	return WriteWhole(draftFile, savedDraft+"\n"+box.Draft)
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
	draft := v.zen.input.Value()
	if draft == v.savedDraft {
		return v, cmd
	}
	if err := WriteWhole(v.draftFile, draft); err != nil {
		v.zen.status = errorStyle.Render(err.Error())
		return v, cmd
	}
	v.savedDraft = draft
	return v, cmd
}
