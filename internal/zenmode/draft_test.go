package zenmode

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/CLBRITTON2/calmpane/claudecode"
	"github.com/CLBRITTON2/calmpane/internal/herdr"
	"github.com/CLBRITTON2/calmpane/internal/herdr/herdrtest"
	"github.com/CLBRITTON2/calmpane/internal/ui"
)

func TestDraft(t *testing.T) {
	t.Run("draftSavesLabelsInTextAndPastesAsJSONAndLoadsBackSame", draftSavesLabelsInTextAndPastesAsJSONAndLoadsBackSame)
	t.Run("missingOrEmptyDraftFileIsNoDraft", missingOrEmptyDraftFileIsNoDraft)
	t.Run("unparsableDraftIsErrorNamingFile", unparsableDraftIsErrorNamingFile)
	t.Run("moveDraftIntoAgentSendsTextBetweenLabelsAsTextAndEachPasteAsOneBracketedPasteInOrder",
		moveDraftIntoAgentSendsTextBetweenLabelsAsTextAndEachPasteAsOneBracketedPasteInOrder)
}

func draftSavesLabelsInTextAndPastesAsJSONAndLoadsBackSame(t *testing.T) {
	draftFile := filepath.Join(t.TempDir(), "draft")
	paste := strings.Repeat("a line of the pasted log\n", 4)
	draft := ui.Draft{Text: "look at " + claudecode.PasteLabel(1, paste) + " please", Pastes: []string{paste}}
	if err := WriteDraft(draftFile, draft); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(draftFile)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"text":"look at [Pasted text #1 +4 lines] please","pastes":["` + strings.Repeat(`a line of the pasted log\n`, 4) + `"]}`
	if string(content) != want {
		t.Errorf("draft file = %s, want %s", content, want)
	}
	loaded, err := SavedDraft(draftFile)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Text != draft.Text || !slices.Equal(loaded.Pastes, draft.Pastes) {
		t.Errorf("SavedDraft = %+q, want %+q", loaded, draft)
	}
}

func missingOrEmptyDraftFileIsNoDraft(t *testing.T) {
	emptyFile := filepath.Join(t.TempDir(), "empty")
	if err := os.WriteFile(emptyFile, []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, draftFile := range []string{emptyFile, filepath.Join(t.TempDir(), "missing")} {
		draft, err := SavedDraft(draftFile)
		if err != nil || draft.Text != "" || len(draft.Pastes) != 0 {
			t.Errorf("SavedDraft(%s) = %+q, %v, want no draft", draftFile, draft, err)
		}
	}
}

func unparsableDraftIsErrorNamingFile(t *testing.T) {
	draftFile := filepath.Join(t.TempDir(), "draft")
	if err := os.WriteFile(draftFile, []byte("a draft in the old plain-text form"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := SavedDraft(draftFile)
	if err == nil || !strings.HasPrefix(err.Error(), "Cannot read the draft "+draftFile+": ") {
		t.Errorf("SavedDraft = %v, want an error starting %q", err, "Cannot read the draft "+draftFile+": ")
	}
}

func moveDraftIntoAgentSendsTextBetweenLabelsAsTextAndEachPasteAsOneBracketedPasteInOrder(t *testing.T) {
	api, requests := herdrtest.FakeAPI(t, []herdr.Pane{herdrtest.ClaudePane("w1:p1", true)})
	draftFile := filepath.Join(t.TempDir(), "draft")
	first := strings.Repeat("first paste\n", 4)
	second := strings.Repeat("second paste\n", 4)
	text := "compare " + claudecode.PasteLabel(2, second) + " with " + claudecode.PasteLabel(1, first) + "\nplease"
	if err := WriteDraft(draftFile, ui.Draft{Text: text, Pastes: []string{first, second}}); err != nil {
		t.Fatal(err)
	}
	if err := MoveDraftIntoAgent(api, draftFile, "w1:p1"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"pane.send_text w1:p1 compare ",
		"pane.send_text w1:p1 \x1b[200~" + second + "\x1b[201~",
		"pane.send_text w1:p1  with ",
		"pane.send_text w1:p1 \x1b[200~" + first + "\x1b[201~",
		"pane.send_text w1:p1 \nplease",
	}
	if got := requests(); !reflect.DeepEqual(got, want) {
		t.Errorf("herdr API requests = %q, want %q", got, want)
	}
}
