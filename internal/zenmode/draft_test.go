package zenmode

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/CLBRITTON2/calmpane/claudecode"
	"github.com/CLBRITTON2/calmpane/internal/herdr"
	"github.com/CLBRITTON2/calmpane/internal/herdr/herdrtest"
	"github.com/CLBRITTON2/calmpane/internal/ui"
	"github.com/stretchr/testify/require"
)

func TestDraft(t *testing.T) {
	t.Run("draftSavesLabelsInTextAndPastesAsJSONAndLoadsBackSame", draftSavesLabelsInTextAndPastesAsJSONAndLoadsBackSame)
	t.Run("emptyDraftFileIsNoDraft", emptyDraftFileIsNoDraft)
	t.Run("missingDraftFileIsNoDraft", missingDraftFileIsNoDraft)
	t.Run("unparsableDraftIsErrorNamingFile", unparsableDraftIsErrorNamingFile)
	t.Run("moveDraftIntoAgentSendsTextBetweenLabelsAsTextAndEachPasteAsOneBracketedPasteInOrder",
		moveDraftIntoAgentSendsTextBetweenLabelsAsTextAndEachPasteAsOneBracketedPasteInOrder)
}

func draftSavesLabelsInTextAndPastesAsJSONAndLoadsBackSame(t *testing.T) {
	draftFile := filepath.Join(t.TempDir(), "draft")
	paste := strings.Repeat("a line of the pasted log\n", 4)
	label := claudecode.PasteLabel(1, paste)
	draft := ui.Draft{Text: "look at " + label + " please", Pastes: []string{paste}}
	writeErr := WriteDraft(draftFile, draft)
	require.NoError(t, writeErr)
	content, readErr := os.ReadFile(draftFile)
	require.NoError(t, readErr)
	savedJSON := string(content)
	pastedJSON := strings.Repeat(`a line of the pasted log\n`, 4)
	wantJSON := `{"text":"look at [Pasted text #1 +4 lines] please","pastes":["` + pastedJSON + `"]}`
	require.Equal(t, wantJSON, savedJSON)
	loaded, loadErr := SavedDraft(draftFile)
	require.NoError(t, loadErr)
	require.Equal(t, draft, loaded)
}

func emptyDraftFileIsNoDraft(t *testing.T) {
	draftFile := filepath.Join(t.TempDir(), "empty")
	err := os.WriteFile(draftFile, []byte{}, 0o644)
	require.NoError(t, err)
	assertDraft(t, draftFile, ui.Draft{})
}

func missingDraftFileIsNoDraft(t *testing.T) {
	draftFile := filepath.Join(t.TempDir(), "missing")
	assertDraft(t, draftFile, ui.Draft{})
}

func unparsableDraftIsErrorNamingFile(t *testing.T) {
	draftFile := filepath.Join(t.TempDir(), "draft")
	writeErr := os.WriteFile(draftFile, []byte("a draft in the old plain-text form"), 0o644)
	require.NoError(t, writeErr)
	_, err := SavedDraft(draftFile)
	require.Error(t, err)
	errText := err.Error()
	wantPrefix := regexp.QuoteMeta("Cannot read the draft " + draftFile + ": ")
	require.Regexp(t, "^"+wantPrefix, errText)
}

func moveDraftIntoAgentSendsTextBetweenLabelsAsTextAndEachPasteAsOneBracketedPasteInOrder(t *testing.T) {
	api, requests := herdrtest.FakeAPI(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", true)})
	draftFile := filepath.Join(t.TempDir(), "draft")
	first := strings.Repeat("first paste\n", 4)
	second := strings.Repeat("second paste\n", 4)
	firstLabel := claudecode.PasteLabel(1, first)
	secondLabel := claudecode.PasteLabel(2, second)
	text := "compare " + secondLabel + " with " + firstLabel + "\nplease"
	writeErr := WriteDraft(draftFile, ui.Draft{Text: text, Pastes: []string{first, second}})
	require.NoError(t, writeErr)
	moveErr := MoveDraftIntoAgent(api, draftFile, "w1:p1")
	require.NoError(t, moveErr)
	apiRequests := requests()
	wantRequests := []string{
		"pane.send_text w1:p1 compare ",
		"pane.send_text w1:p1 \x1b[200~" + second + "\x1b[201~",
		"pane.send_text w1:p1  with ",
		"pane.send_text w1:p1 \x1b[200~" + first + "\x1b[201~",
		"pane.send_text w1:p1 \nplease",
	}
	require.Equal(t, wantRequests, apiRequests)
}
