package ptyagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHook(t *testing.T) {
	t.Run("writeHookStateWritesIdleOnSessionStart", writeHookStateWritesIdleOnSessionStart)
	t.Run("writeHookStateWritesWorkingOnUserPromptSubmit", writeHookStateWritesWorkingOnUserPromptSubmit)
	t.Run("writeHookStateWritesWorkingOnPostToolUse", writeHookStateWritesWorkingOnPostToolUse)
	t.Run("writeHookStateWritesDoneOnStop", writeHookStateWritesDoneOnStop)
	t.Run("writeHookStateWritesBlockedOnNotification", writeHookStateWritesBlockedOnNotification)
	t.Run("writeHookStateFailsForEventZenDoesNotHook", writeHookStateFailsForEventZenDoesNotHook)
	t.Run("writeHookStateFailsForHookWithNoSessionID", writeHookStateFailsForHookWithNoSessionID)
	t.Run("parseHookStateIsWholeForKnownStatusAndSessionEndingInLF", parseHookStateIsWholeForKnownStatusAndSessionEndingInLF)
	t.Run("parseHookStateIsNotWholeWithoutEndingLF", parseHookStateIsNotWholeWithoutEndingLF)
	t.Run("parseHookStateIsNotWholeWithoutSession", parseHookStateIsNotWholeWithoutSession)
	t.Run("parseHookStateIsNotWholeForUnknownStatus", parseHookStateIsNotWholeForUnknownStatus)
	t.Run("parseHookStateIsNotWholeForEmptyState", parseHookStateIsNotWholeForEmptyState)
	t.Run("writeHookSettingsRunsZenHookOnEachEventAndOnlyPromptNotifications",
		writeHookSettingsRunsZenHookOnEachEventAndOnlyPromptNotifications)
}

func assertHookEventOverwritesState(t *testing.T, hookEvent string, wantState string) {
	stateFile := filepath.Join(t.TempDir(), "state")
	writeErr := os.WriteFile(stateFile, []byte("working an-earlier-session-with-a-longer-id\n"), 0o600)
	require.NoError(t, writeErr)
	hookStdin := strings.NewReader(`{"session_id":"session-1","hook_event_name":"` + hookEvent + `","cwd":"C:\\dev"}`)
	hookErr := WriteHookState(stateFile, hookStdin)
	require.NoError(t, hookErr)
	state, readErr := os.ReadFile(stateFile)
	require.NoError(t, readErr)
	require.Equal(t, wantState, string(state))
}

func writeHookStateWritesIdleOnSessionStart(t *testing.T) {
	assertHookEventOverwritesState(t, "SessionStart", "idle session-1\n")
}

func writeHookStateWritesWorkingOnUserPromptSubmit(t *testing.T) {
	assertHookEventOverwritesState(t, "UserPromptSubmit", "working session-1\n")
}

func writeHookStateWritesWorkingOnPostToolUse(t *testing.T) {
	assertHookEventOverwritesState(t, "PostToolUse", "working session-1\n")
}

func writeHookStateWritesDoneOnStop(t *testing.T) {
	assertHookEventOverwritesState(t, "Stop", "done session-1\n")
}

func writeHookStateWritesBlockedOnNotification(t *testing.T) {
	assertHookEventOverwritesState(t, "Notification", "blocked session-1\n")
}

func writeHookStateFailsForEventZenDoesNotHook(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "state")
	hookStdin := strings.NewReader(`{"session_id":"session-1","hook_event_name":"PreToolUse"}`)
	hookErr := WriteHookState(stateFile, hookStdin)
	require.ErrorContains(t, hookErr, `"PreToolUse"`)
	_, statErr := os.Stat(stateFile)
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func writeHookStateFailsForHookWithNoSessionID(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "state")
	hookStdin := strings.NewReader(`{"hook_event_name":"Stop"}`)
	hookErr := WriteHookState(stateFile, hookStdin)
	require.ErrorContains(t, hookErr, "no session_id")
	_, statErr := os.Stat(stateFile)
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func parseHookStateIsWholeForKnownStatusAndSessionEndingInLF(t *testing.T) {
	_, whole := parseHookState("done session-1\n")
	require.True(t, whole)
}

func parseHookStateIsNotWholeWithoutEndingLF(t *testing.T) {
	_, whole := parseHookState("done session-1")
	require.False(t, whole)
}

func parseHookStateIsNotWholeWithoutSession(t *testing.T) {
	_, whole := parseHookState("done\n")
	require.False(t, whole)
}

func parseHookStateIsNotWholeForUnknownStatus(t *testing.T) {
	_, whole := parseHookState("finished session-1\n")
	require.False(t, whole)
}

func parseHookStateIsNotWholeForEmptyState(t *testing.T) {
	_, whole := parseHookState("")
	require.False(t, whole)
}

func writeHookSettingsRunsZenHookOnEachEventAndOnlyPromptNotifications(t *testing.T) {
	settingsFile := filepath.Join(t.TempDir(), "settings.json")
	writeErr := WriteHookSettings(settingsFile, `C:\bin\calmpane.exe`, `C:\Temp\zen-claude-1\state`)
	require.NoError(t, writeErr)
	content, readErr := os.ReadFile(settingsFile)
	require.NoError(t, readErr)
	var settings hookSettings
	unmarshalErr := json.Unmarshal(content, &settings)
	require.NoError(t, unmarshalErr)
	zenHook := []hookCommand{{Type: "command", Command: `"C:/bin/calmpane.exe" hook "C:/Temp/zen-claude-1/state"`}}
	everyRun := []hookMatcher{{Matcher: "", Hooks: zenHook}}
	promptNotificationRun := []hookMatcher{{Matcher: "permission_prompt|elicitation_dialog", Hooks: zenHook}}
	want := hookSettings{Hooks: map[string][]hookMatcher{
		"SessionStart":     everyRun,
		"UserPromptSubmit": everyRun,
		"PostToolUse":      everyRun,
		"Stop":             everyRun,
		"Notification":     promptNotificationRun,
	}}
	require.Equal(t, want, settings)
}
