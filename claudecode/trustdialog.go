package claudecode

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// TrustDialogShown reports whether the screen shows the dialog Claude Code opens in a folder it does not trust yet,
// before it starts a session. Its default answer, No, exit, ends Claude. The texts are Claude Code 2.1.331's.
func TrustDialogShown(screen string) bool {
	accessingWorkspace := false
	offersTrust := false
	for _, row := range strings.Split(ansi.Strip(screen), "\n") {
		text := strings.TrimSpace(row)
		accessingWorkspace = accessingWorkspace || text == "Accessing workspace:"
		offersTrust = offersTrust || strings.HasSuffix(text, "Yes, I trust this folder")
	}
	return accessingWorkspace && offersTrust
}
