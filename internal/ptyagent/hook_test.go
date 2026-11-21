package ptyagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHook(t *testing.T) {
	t.Run("writeHookStateWritesSessionAndStatusOfEachEvent", writeHookStateWritesSessionAndStatusOfEachEvent)
	t.Run("writeHookStateFailsForEventZenDoesNotHook", writeHookStateFailsForEventZenDoesNotHook)
	t.Run("writeHookStateFailsForHookWithNoSessionID", writeHookStateFailsForHookWithNoSessionID)
	t.Run("parseHookStateIsWholeOnlyForKnownStatusAndSessionEndingInLF", parseHookStateIsWholeOnlyForKnownStatusAndSessionEndingInLF)
	t.Run("writeHookSettingsRunsZenHookOnEachEventAndOnlyPromptNotifications",
		writeHookSettingsRunsZenHookOnEachEventAndOnlyPromptNotifications)
}

func writeHookStateWritesSessionAndStatusOfEachEvent(t *testing.T) {
	for hookEvent, want := range map[string]string{
		"SessionStart":     "idle session-1\n",
		"UserPromptSubmit": "working session-1\n",
		"PostToolUse":      "working session-1\n",
		"Stop":             "done session-1\n",
		"Notification":     "blocked session-1\n",
	} {
		t.Run(hookEvent, func(t *testing.T) {
			stateFile := filepath.Join(t.TempDir(), "state")
			if err := os.WriteFile(stateFile, []byte("working an-earlier-session-with-a-longer-id\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			hookStdin := `{"session_id":"session-1","hook_event_name":"` + hookEvent + `","cwd":"C:\\dev"}`
			if err := WriteHookState(stateFile, strings.NewReader(hookStdin)); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(stateFile)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != want {
				t.Errorf("state file = %q, want %q", got, want)
			}
		})
	}
}

func writeHookStateFailsForEventZenDoesNotHook(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "state")
	err := WriteHookState(stateFile, strings.NewReader(`{"session_id":"session-1","hook_event_name":"PreToolUse"}`))
	if err == nil || !strings.Contains(err.Error(), `"PreToolUse"`) {
		t.Errorf("WriteHookState error = %v, want one naming PreToolUse", err)
	}
	if _, err := os.Stat(stateFile); !os.IsNotExist(err) {
		t.Errorf("state file stat error = %v, want it not written", err)
	}
}

func writeHookStateFailsForHookWithNoSessionID(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "state")
	err := WriteHookState(stateFile, strings.NewReader(`{"hook_event_name":"Stop"}`))
	if err == nil || !strings.Contains(err.Error(), "no session_id") {
		t.Errorf("WriteHookState error = %v, want one saying the hook has no session_id", err)
	}
	if _, err := os.Stat(stateFile); !os.IsNotExist(err) {
		t.Errorf("state file stat error = %v, want it not written", err)
	}
}

func parseHookStateIsWholeOnlyForKnownStatusAndSessionEndingInLF(t *testing.T) {
	for stateFileContent, wantWhole := range map[string]bool{
		"done session-1\n": true, "done session-1": false, "done\n": false, "finished session-1\n": false, "": false,
	} {
		if _, whole := parseHookState(stateFileContent); whole != wantWhole {
			t.Errorf("parseHookState(%q) whole = %v, want %v", stateFileContent, whole, wantWhole)
		}
	}
}

func writeHookSettingsRunsZenHookOnEachEventAndOnlyPromptNotifications(t *testing.T) {
	settingsFile := filepath.Join(t.TempDir(), "settings.json")
	if err := WriteHookSettings(settingsFile, `C:\bin\zen.exe`, `C:\Temp\zen-claude-1\state`); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(settingsFile)
	if err != nil {
		t.Fatal(err)
	}
	var settings hookSettings
	if err := json.Unmarshal(content, &settings); err != nil {
		t.Fatal(err)
	}
	wantCommand := `"C:/bin/zen.exe" hook "C:/Temp/zen-claude-1/state"`
	for hookEvent, wantMatcher := range map[string]string{
		"SessionStart": "", "UserPromptSubmit": "", "PostToolUse": "", "Stop": "",
		"Notification": "permission_prompt|elicitation_dialog",
	} {
		matchers := settings.Hooks[hookEvent]
		if len(matchers) != 1 || matchers[0].Matcher != wantMatcher || len(matchers[0].Hooks) != 1 ||
			matchers[0].Hooks[0].Command != wantCommand || matchers[0].Hooks[0].Type != "command" {
			t.Errorf("%s hooks = %+v, want matcher %q running %s", hookEvent, matchers, wantMatcher, wantCommand)
		}
	}
	if len(settings.Hooks) != 5 {
		t.Errorf("hooked events = %d, want 5", len(settings.Hooks))
	}
}
