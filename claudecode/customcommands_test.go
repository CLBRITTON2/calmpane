package claudecode

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCustomSlashCommands(t *testing.T) {
	t.Run("customSlashCommandsListProjectThenUserThenSyncedWithSourceAndFirstOfName",
		customSlashCommandsListProjectThenUserThenSyncedWithSourceAndFirstOfName)
	t.Run("customSlashCommandsLeaveOutSkillNotUserInvocable", customSlashCommandsLeaveOutSkillNotUserInvocable)
	t.Run("customSlashCommandsAreEmptyWithoutClaudeFolders", customSlashCommandsAreEmptyWithoutClaudeFolders)
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func customSlashCommandsListProjectThenUserThenSyncedWithSourceAndFirstOfName(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	project := t.TempDir()
	writeTestFile(t, filepath.Join(project, ".claude", "commands", "deploy.md"),
		"---\r\ndescription: Deploy the\r\n  staging site\r\n---\r\nDeploy it.\r\n")
	writeTestFile(t, filepath.Join(home, ".claude", "commands", "deploy.md"), "---\ndescription: Deploy anywhere\n---\n")
	writeTestFile(t, filepath.Join(home, ".claude", "commands", "notes.txt"), "not a command")
	writeTestFile(t, filepath.Join(home, ".claude", "skills", "tidy", "SKILL.md"),
		"---\nname: tidy\ndescription: Tidy the imports\n---\nBody.\n")
	writeTestFile(t, filepath.Join(home, ".claude", "skills", "bare", "SKILL.md"), "No frontmatter.\n")
	writeTestFile(t, filepath.Join(home, ".claude", "skills", "synced", "account", "manifest.json"),
		`{"skills":[{"name":"pdf","description":"Read PDFs"},{"name":"tidy","description":"Synced tidy"}]}`)
	commands, err := CustomSlashCommands(project)
	if err != nil {
		t.Fatalf("CustomSlashCommands: %v", err)
	}
	want := []SlashCommand{
		{Name: "deploy", Description: "Deploy the staging site (project)"},
		{Name: "bare", Description: "(user)"},
		{Name: "tidy", Description: "Tidy the imports (user)"},
		{Name: "pdf", Description: "Read PDFs (claude.ai sync)"},
	}
	if !reflect.DeepEqual(commands, want) {
		t.Errorf("commands = %+v, want %+v", commands, want)
	}
}

func customSlashCommandsLeaveOutSkillNotUserInvocable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	writeTestFile(t, filepath.Join(home, ".claude", "skills", "hidden", "SKILL.md"),
		"---\ndescription: Only Claude runs this\nuser-invocable: false\n---\n")
	commands, err := CustomSlashCommands(t.TempDir())
	if err != nil {
		t.Fatalf("CustomSlashCommands: %v", err)
	}
	if len(commands) != 0 {
		t.Errorf("commands = %+v, want none", commands)
	}
}

func customSlashCommandsAreEmptyWithoutClaudeFolders(t *testing.T) {
	t.Setenv("USERPROFILE", t.TempDir())
	commands, err := CustomSlashCommands(t.TempDir())
	if err != nil {
		t.Fatalf("CustomSlashCommands: %v", err)
	}
	if len(commands) != 0 {
		t.Errorf("commands = %+v, want none", commands)
	}
}
