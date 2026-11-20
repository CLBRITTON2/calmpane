// Command zen shows the dialogue of the Claude session in a herdr pane: the user's prompts, Claude's reply text and
// its questions.
package main

import (
	"errors"
	"fmt"
	"os"
)

const (
	// toggleKey is the key windows-dev's herdr/config.toml binds to the windows-dev.zen.toggle action.
	toggleKey     = "alt+x"
	claudePaneEnv = "ZEN_CLAUDE_PANE"
)

func main() {
	herdr := herdrCLI{binary: os.Getenv("HERDR_BIN_PATH")}
	if herdr.binary == "" {
		fmt.Fprintln(os.Stderr, "HERDR_BIN_PATH is not set: zen runs from a herdr plugin")
		os.Exit(2)
	}
	var err error
	switch {
	case len(os.Args) == 1:
		err = RunZenView(herdr)
	case len(os.Args) == 2 && os.Args[1] == "toggle":
		err = ToggleZenPane(herdr)
	default:
		err = errors.New("usage: zen [toggle]")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
