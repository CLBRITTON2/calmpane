package herdrtest

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/CLBRITTON2/calmpane/internal/herdr"
)

// Main runs the tests, or acts as herdr's CLI when a test started the test binary as herdr.
func Main(m *testing.M) {
	if fakeDir := os.Getenv(FakeDirEnv); fakeDir != "" {
		if err := runFakeHerdr(fakeDir, os.Args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// FakeDirEnv makes the test binary act as herdr's CLI on the panes in panes.json under the directory it names,
// logging each command line to calls.log there. A pane's screen is the file ScreenFile names, and send-keys
// replaces it with the file named with .after-keys added when there is one.
const FakeDirEnv = "ZEN_FAKE_HERDR_DIR"

func runFakeHerdr(fakeDir string, arguments []string) error {
	callLog, err := os.OpenFile(filepath.Join(fakeDir, "calls.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(callLog, strings.Join(arguments, " ")); err != nil {
		return errors.Join(err, callLog.Close())
	}
	if err := callLog.Close(); err != nil {
		return err
	}
	panesFile := filepath.Join(fakeDir, "panes.json")
	panesJSON, err := os.ReadFile(panesFile)
	if err != nil {
		return err
	}
	var panes []herdr.Pane
	if err := json.Unmarshal(panesJSON, &panes); err != nil {
		return err
	}
	command := strings.Join(arguments[:2], " ")
	switch command {
	case "pane read":
		screen, err := os.ReadFile(ScreenFile(fakeDir, arguments[2]))
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		_, err = os.Stdout.Write(screen)
		return err
	case "pane send-keys":
		screenFile := ScreenFile(fakeDir, arguments[2])
		if err := os.Rename(screenFile+".after-keys", screenFile); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	case "pane get":
		for _, pane := range panes {
			if pane.PaneID == arguments[2] {
				reply, err := json.Marshal(map[string]map[string]herdr.Pane{"result": {"pane": pane}})
				if err != nil {
					return err
				}
				_, err = os.Stdout.Write(reply)
				return err
			}
		}
		return fmt.Errorf("pane %s not found", arguments[2])
	case "pane list":
		reply, err := json.Marshal(map[string]map[string][]herdr.Pane{"result": {"panes": panes}})
		if err != nil {
			return err
		}
		_, err = os.Stdout.Write(reply)
		return err
	case "agent list":
		agents := []herdr.Pane{}
		for _, pane := range panes {
			if pane.Agent != "" {
				agents = append(agents, pane)
			}
		}
		reply, err := json.Marshal(map[string]map[string][]herdr.Pane{"result": {"agents": agents}})
		if err != nil {
			return err
		}
		_, err = os.Stdout.Write(reply)
		return err
	case "plugin pane":
		if arguments[2] != "open" {
			return nil
		}
		zenPane := herdr.Pane{PaneID: fmt.Sprintf("w1:z%d", len(panes)), Label: herdr.ZenPaneTitle, Focused: true}
		for index := range panes {
			panes[index].Focused = false
		}
		panes = append(panes, zenPane)
		if _, err := fmt.Printf(`{"result":{"plugin_pane":{"pane":{"pane_id":%q}}}}`, zenPane.PaneID); err != nil {
			return err
		}
	case "pane report-metadata":
		token := strings.SplitN(arguments[len(arguments)-1], "=", 2)
		for index := range panes {
			if panes[index].PaneID == arguments[2] {
				panes[index].Tokens = map[string]string{token[0]: token[1]}
			}
		}
	default:
		return nil
	}
	panesJSON, err = json.Marshal(panes)
	if err != nil {
		return err
	}
	return os.WriteFile(panesFile, panesJSON, 0o644)
}

func Fake(t *testing.T, panes []herdr.Pane) (herdr.CLI, func() []string) {
	fakeDir := t.TempDir()
	panesJSON, err := json.Marshal(panes)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fakeDir, "panes.json"), panesJSON, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(FakeDirEnv, fakeDir)
	testBinary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	calls := func() []string {
		callLog, err := os.ReadFile(filepath.Join(fakeDir, "calls.log"))
		if os.IsNotExist(err) {
			return []string{}
		}
		if err != nil {
			t.Fatal(err)
		}
		return strings.Split(strings.TrimSpace(string(callLog)), "\n")
	}
	return herdr.CLI{Binary: testBinary, PluginID: "zen-test"}, calls
}

func ScreenFile(fakeDir string, paneID string) string {
	return filepath.Join(fakeDir, "screen."+strings.ReplaceAll(paneID, ":", "-"))
}

// ShowOnScreen puts the testdata capture on paneID's screen, and afterKeysCapture there once keys are sent. An
// empty afterKeysCapture leaves the screen as it is.
func ShowOnScreen(t *testing.T, paneID string, capture string, afterKeysCapture string) {
	screenFile := ScreenFile(os.Getenv(FakeDirEnv), paneID)
	for file, capture := range map[string]string{screenFile: capture, screenFile + ".after-keys": afterKeysCapture} {
		if capture == "" {
			continue
		}
		if err := os.WriteFile(file, []byte(CapturedScreen(t, capture)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func ClaudePane(paneID string, focused bool) herdr.Pane {
	return herdr.Pane{PaneID: paneID, Agent: "claude", Focused: focused}
}

func ZenPane(paneID string, claudePaneID string, focused bool) herdr.Pane {
	return herdr.Pane{PaneID: paneID, Label: herdr.ZenPaneTitle, Focused: focused,
		Tokens: map[string]string{herdr.ClaudePaneToken: claudePaneID}}
}

// CapturedScreen reads testdata next to this file, not next to the calling test's package.
func CapturedScreen(t *testing.T, capture string) string {
	t.Helper()
	_, fakeFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller reports no file for the herdrtest package")
	}
	screen, err := os.ReadFile(filepath.Join(filepath.Dir(fakeFile), "testdata", capture))
	if err != nil {
		t.Fatal(err)
	}
	return string(screen)
}
