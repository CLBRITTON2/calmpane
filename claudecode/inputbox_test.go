package claudecode

import (
	"testing"

	"github.com/CLBRITTON2/calmpane/internal/herdr/herdrtest"
	"github.com/stretchr/testify/require"
)

func TestClaudeInputBox(t *testing.T) {
	t.Run("inputBoxOnScreenGivesDraftOfOneRowBox", inputBoxOnScreenGivesDraftOfOneRowBox)
	t.Run("inputBoxOnScreenGivesDraftUnderRuleCarryingSessionName", inputBoxOnScreenGivesDraftUnderRuleCarryingSessionName)
	t.Run("inputBoxOnScreenJoinsDraftWrappedOverTwoRows", inputBoxOnScreenJoinsDraftWrappedOverTwoRows)
	t.Run("inputBoxOnScreenGivesDraftTypedWhileClaudeWorks", inputBoxOnScreenGivesDraftTypedWhileClaudeWorks)
	t.Run("inputBoxOnScreenGivesNoDraftForPlaceholder", inputBoxOnScreenGivesNoDraftForPlaceholder)
	t.Run("inputBoxOnScreenGivesNoDraftForEmptyBox", inputBoxOnScreenGivesNoDraftForEmptyBox)
	t.Run("inputBoxOnScreenGivesNoDraftWhenBoxScrolledPastItsTop", inputBoxOnScreenGivesNoDraftWhenBoxScrolledPastItsTop)
	t.Run("inputBoxShownForBoxHoldingDraft", inputBoxShownForBoxHoldingDraft)
	t.Run("inputBoxShownForEmptyBox", inputBoxShownForEmptyBox)
	t.Run("inputBoxNotShownWhenBoxScrolledPastItsTop", inputBoxNotShownWhenBoxScrolledPastItsTop)
}

func inputBoxOnScreenGivesDraftOfOneRowBox(t *testing.T) {
	screen := herdrtest.CapturedScreen(t, "claude-input-one-row.ansi")
	box := InputBoxOnScreen(screen)
	want := InputBox{Draft: "move this draft", Rows: 1}
	require.Equal(t, want, box)
}

func inputBoxOnScreenGivesDraftUnderRuleCarryingSessionName(t *testing.T) {
	screen := herdrtest.CapturedScreen(t, "claude-input-titled-rule.ansi")
	box := InputBoxOnScreen(screen)
	want := InputBox{Draft: "move this draft", Rows: 1}
	require.Equal(t, want, box)
}

func inputBoxOnScreenJoinsDraftWrappedOverTwoRows(t *testing.T) {
	screen := herdrtest.CapturedScreen(t, "claude-input-wrapped.ansi")
	box := InputBoxOnScreen(screen)
	want := InputBox{Draft: "this unsent draft is long enough to wrap across two rows of the Claude input " +
		"box, so pane read drops the space at the wrap", Rows: 2}
	require.Equal(t, want, box)
}

func inputBoxOnScreenGivesDraftTypedWhileClaudeWorks(t *testing.T) {
	screen := herdrtest.CapturedScreen(t, "claude-input-working.ansi")
	box := InputBoxOnScreen(screen)
	want := InputBox{Draft: "while that ping runs I am drafting the next request so it should move into " +
		"the zen input untouched", Rows: 2}
	require.Equal(t, want, box)
}

func inputBoxOnScreenGivesNoDraftForPlaceholder(t *testing.T) {
	screen := herdrtest.CapturedScreen(t, "claude-input-placeholder.ansi")
	box := InputBoxOnScreen(screen)
	require.Zero(t, box)
}

func inputBoxOnScreenGivesNoDraftForEmptyBox(t *testing.T) {
	screen := herdrtest.CapturedScreen(t, "claude-input-empty.ansi")
	box := InputBoxOnScreen(screen)
	require.Zero(t, box)
}

func inputBoxOnScreenGivesNoDraftWhenBoxScrolledPastItsTop(t *testing.T) {
	screen := herdrtest.CapturedScreen(t, "claude-input-scrolled.ansi")
	box := InputBoxOnScreen(screen)
	require.Zero(t, box)
}

func inputBoxShownForBoxHoldingDraft(t *testing.T) {
	screen := herdrtest.CapturedScreen(t, "claude-input-one-row.ansi")
	shown := InputBoxShown(screen)
	require.True(t, shown)
}

func inputBoxShownForEmptyBox(t *testing.T) {
	screen := herdrtest.CapturedScreen(t, "claude-input-empty.ansi")
	shown := InputBoxShown(screen)
	require.True(t, shown)
}

func inputBoxNotShownWhenBoxScrolledPastItsTop(t *testing.T) {
	screen := herdrtest.CapturedScreen(t, "claude-input-scrolled.ansi")
	shown := InputBoxShown(screen)
	require.False(t, shown)
}
