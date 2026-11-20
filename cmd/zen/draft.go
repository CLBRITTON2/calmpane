package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	draft, err := os.ReadFile(draftFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read the zen draft: %w", err)
	}
	if len(draft) > 0 {
		if err := herdr.SendText(claudePaneID, string(draft)); err != nil {
			return err
		}
	}
	if err := os.Remove(draftFile); err != nil {
		return fmt.Errorf("remove the zen draft: %w", err)
	}
	return nil
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
	// written whole under another name and renamed over, so the toggle never reads half a draft
	staging := v.draftFile + ".tmp"
	if err := os.WriteFile(staging, []byte(draft), 0o644); err != nil {
		v.zen.status = errorStyle.Render(err.Error())
		return v, cmd
	}
	if err := os.Rename(staging, v.draftFile); err != nil {
		v.zen.status = errorStyle.Render(err.Error())
		return v, cmd
	}
	v.savedDraft = draft
	return v, cmd
}
