package claudecode

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCustomSlashCommands(t *testing.T) {
	t.Run("customSlashCommandsListProjectThenUserThenSyncedWithSourceAndFirstOfName",
		customSlashCommandsListProjectThenUserThenSyncedWithSourceAndFirstOfName)
	t.Run("customSlashCommandsLeaveOutSkillNotUserInvocable", customSlashCommandsLeaveOutSkillNotUserInvocable)
	t.Run("customSlashCommandsAreEmptyWithoutClaudeFolders", customSlashCommandsAreEmptyWithoutClaudeFolders)
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	dir := filepath.Dir(path)
	err := os.MkdirAll(dir, 0o755)
	require.NoError(t, err)
	err = os.WriteFile(path, []byte(content), 0o600)
	require.NoError(t, err)
}

func customSlashCommandsListProjectThenUserThenSyncedWithSourceAndFirstOfName(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	project := t.TempDir()
	projectDeployPath := filepath.Join(project, ".claude", "commands", "deploy.md")
	writeTestFile(t, projectDeployPath, "---\r\ndescription: Deploy the\r\n  staging site\r\n---\r\nDeploy it.\r\n")
	userDeployPath := filepath.Join(home, ".claude", "commands", "deploy.md")
	writeTestFile(t, userDeployPath, "---\ndescription: Deploy anywhere\n---\n")
	notesPath := filepath.Join(home, ".claude", "commands", "notes.txt")
	writeTestFile(t, notesPath, "not a command")
	tidySkillPath := filepath.Join(home, ".claude", "skills", "tidy", "SKILL.md")
	writeTestFile(t, tidySkillPath, "---\nname: tidy\ndescription: Tidy the imports\n---\nBody.\n")
	bareSkillPath := filepath.Join(home, ".claude", "skills", "bare", "SKILL.md")
	writeTestFile(t, bareSkillPath, "No frontmatter.\n")
	manifestPath := filepath.Join(home, ".claude", "skills", "synced", "account", "manifest.json")
	writeTestFile(t, manifestPath,
		`{"skills":[{"name":"pdf","description":"Read PDFs"},{"name":"tidy","description":"Synced tidy"}]}`)

	commands, err := CustomSlashCommands(project)
	require.NoError(t, err)
	want := []SlashCommand{
		{Name: "deploy", Description: "Deploy the staging site (project)"},
		{Name: "bare", Description: "(user)"},
		{Name: "tidy", Description: "Tidy the imports (user)"},
		{Name: "pdf", Description: "Read PDFs (claude.ai sync)"},
	}
	require.Equal(t, want, commands)
}

func customSlashCommandsLeaveOutSkillNotUserInvocable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	hiddenSkillPath := filepath.Join(home, ".claude", "skills", "hidden", "SKILL.md")
	writeTestFile(t, hiddenSkillPath, "---\ndescription: Only Claude runs this\nuser-invocable: false\n---\n")
	project := t.TempDir()

	commands, err := CustomSlashCommands(project)
	require.NoError(t, err)
	require.Empty(t, commands)
}

func customSlashCommandsAreEmptyWithoutClaudeFolders(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	project := t.TempDir()

	commands, err := CustomSlashCommands(project)
	require.NoError(t, err)
	require.Empty(t, commands)
}
