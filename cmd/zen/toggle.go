package main

import (
	"encoding/json"
	"fmt"
	"os"
)

func ToggleZenPane(herdr herdrCLI) error {
	contextJSON := os.Getenv("HERDR_PLUGIN_CONTEXT_JSON")
	var context struct {
		FocusedPaneID string `json:"focused_pane_id"`
	}
	if err := json.Unmarshal([]byte(contextJSON), &context); err != nil {
		return fmt.Errorf("decode HERDR_PLUGIN_CONTEXT_JSON %q: %w", contextJSON, err)
	}
	if context.FocusedPaneID == "" {
		return fmt.Errorf("HERDR_PLUGIN_CONTEXT_JSON has no focused_pane_id: %q", contextJSON)
	}
	focusedPane, err := herdr.Pane(context.FocusedPaneID)
	if err != nil {
		return err
	}
	if claudePaneID, isZenPane := focusedPane.Tokens[claudePaneToken]; isZenPane {
		return herdr.CloseZenPaneAndFocusAgent(focusedPane.PaneID, claudePaneID)
	}
	return herdr.OpenZenPaneZoomed(focusedPane.PaneID)
}
