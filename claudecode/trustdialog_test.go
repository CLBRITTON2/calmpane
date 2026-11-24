package claudecode

import (
	"testing"

	"github.com/CLBRITTON2/calmpane/internal/herdr/herdrtest"
)

func TestTrustDialog(t *testing.T) {
	t.Run("trustDialogShownOnlyWhileScreenShowsTrustDialog", trustDialogShownOnlyWhileScreenShowsTrustDialog)
}

func trustDialogShownOnlyWhileScreenShowsTrustDialog(t *testing.T) {
	cases := []struct {
		capture string
		want    bool
	}{
		{capture: "claude-trust-dialog.ansi", want: true},
		{capture: "claude-input-empty.ansi", want: false},
		{capture: "claude-input-one-row.ansi", want: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.capture, func(t *testing.T) {
			if got := TrustDialogShown(herdrtest.CapturedScreen(t, testCase.capture)); got != testCase.want {
				t.Errorf("TrustDialogShown = %t, want %t", got, testCase.want)
			}
		})
	}
}
