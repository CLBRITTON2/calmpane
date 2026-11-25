package claudecode

import (
	"testing"

	"github.com/CLBRITTON2/calmpane/internal/herdr/herdrtest"
	"github.com/stretchr/testify/require"
)

func TestTrustDialog(t *testing.T) {
	t.Run("trustDialogShownWhileScreenShowsTrustDialog", trustDialogShownWhileScreenShowsTrustDialog)
	t.Run("trustDialogNotShownForEmptyInputBox", trustDialogNotShownForEmptyInputBox)
	t.Run("trustDialogNotShownForInputBoxHoldingDraft", trustDialogNotShownForInputBoxHoldingDraft)
}

func trustDialogShownWhileScreenShowsTrustDialog(t *testing.T) {
	screen := herdrtest.CapturedScreen(t, "claude-trust-dialog.ansi")
	shown := TrustDialogShown(screen)
	require.True(t, shown)
}

func trustDialogNotShownForEmptyInputBox(t *testing.T) {
	screen := herdrtest.CapturedScreen(t, "claude-input-empty.ansi")
	shown := TrustDialogShown(screen)
	require.False(t, shown)
}

func trustDialogNotShownForInputBoxHoldingDraft(t *testing.T) {
	screen := herdrtest.CapturedScreen(t, "claude-input-one-row.ansi")
	shown := TrustDialogShown(screen)
	require.False(t, shown)
}
