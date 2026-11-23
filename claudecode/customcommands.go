package claudecode

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// SlashCommand is a command Claude Code's slash command menu lists, its Name without the slash.
type SlashCommand struct {
	Name        string
	Description string
}

type frontmatter struct {
	Description   string `yaml:"description"`
	UserInvocable *bool  `yaml:"user-invocable"`
}

type syncedSkillsManifest struct {
	Skills []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"skills"`
}

// CustomSlashCommands are the commands and skills Claude Code loads from disk for a Claude working in projectDir: the
// project's and the user's (`.claude\commands\*.md` and `.claude\skills\*\SKILL.md`), then the skills synced from
// claude.ai, first of a name only. Each description ends with its source, as Claude's menu shows it.
func CustomSlashCommands(projectDir string) ([]SlashCommand, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("find the home directory: %w", err)
	}
	userDir := filepath.Join(home, ".claude")
	commands := []SlashCommand{}
	for _, source := range []struct{ dir, label string }{
		{dir: filepath.Join(projectDir, ".claude"), label: "project"},
		{dir: userDir, label: "user"},
	} {
		fileCommands, err := commandFileCommands(filepath.Join(source.dir, "commands"), source.label)
		if err != nil {
			return nil, err
		}
		skills, err := skillCommands(filepath.Join(source.dir, "skills"), source.label)
		if err != nil {
			return nil, err
		}
		commands = append(append(commands, fileCommands...), skills...)
	}
	synced, err := syncedSkillCommands(filepath.Join(userDir, "skills", "synced"))
	if err != nil {
		return nil, err
	}
	unique := []SlashCommand{}
	for _, command := range append(commands, synced...) {
		if !slices.ContainsFunc(unique, func(other SlashCommand) bool { return other.Name == command.Name }) {
			unique = append(unique, command)
		}
	}
	return unique, nil
}

// commandFileCommands holds a command per Markdown file in dir, none when dir does not exist.
func commandFileCommands(dir string, sourceLabel string) ([]SlashCommand, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("Cannot read %s: %w", dir, err)
	}
	commands := []SlashCommand{}
	for _, entry := range entries {
		name, isMarkdown := strings.CutSuffix(entry.Name(), ".md")
		if !isMarkdown || entry.IsDir() {
			continue
		}
		fields, err := fileFrontmatter(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		commands = append(commands, SlashCommand{Name: name, Description: describedBySource(fields.Description, sourceLabel)})
	}
	return commands, nil
}

// skillCommands holds a command per folder of dir holding a SKILL.md, but a skill whose user-invocable is false, which
// Claude leaves out of its menu.
func skillCommands(dir string, sourceLabel string) ([]SlashCommand, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("Cannot read %s: %w", dir, err)
	}
	commands := []SlashCommand{}
	for _, entry := range entries {
		// a skill folder may be a link, which the entry does not report as a folder
		fields, err := fileFrontmatter(filepath.Join(dir, entry.Name(), "SKILL.md"))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if fields.UserInvocable != nil && !*fields.UserInvocable {
			continue
		}
		commands = append(commands, SlashCommand{Name: entry.Name(),
			Description: describedBySource(fields.Description, sourceLabel)})
	}
	return commands, nil
}

// syncedSkillCommands reads the manifest.json Claude Code keeps in each account's folder under dir.
func syncedSkillCommands(dir string) ([]SlashCommand, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("Cannot read %s: %w", dir, err)
	}
	commands := []SlashCommand{}
	for _, entry := range entries {
		manifestPath := filepath.Join(dir, entry.Name(), "manifest.json")
		content, err := os.ReadFile(manifestPath)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("Cannot read %s: %w", manifestPath, err)
		}
		var manifest syncedSkillsManifest
		if err := json.Unmarshal(content, &manifest); err != nil {
			return nil, fmt.Errorf("decode %s: %w", manifestPath, err)
		}
		for _, skill := range manifest.Skills {
			commands = append(commands, SlashCommand{Name: skill.Name,
				Description: describedBySource(skill.Description, "claude.ai sync")})
		}
	}
	return commands, nil
}

// fileFrontmatter is empty for a file that opens with no --- line.
func fileFrontmatter(path string) (frontmatter, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return frontmatter{}, fmt.Errorf("Cannot read %s: %w", path, err)
	}
	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	afterOpening, opens := strings.CutPrefix(text, "---\n")
	if !opens {
		return frontmatter{}, nil
	}
	block, _, closes := strings.Cut(afterOpening, "\n---")
	if !closes {
		return frontmatter{}, fmt.Errorf("%s: the frontmatter has no closing --- line", path)
	}
	var fields frontmatter
	if err := yaml.Unmarshal([]byte(block), &fields); err != nil {
		return frontmatter{}, fmt.Errorf("decode the frontmatter of %s: %w", path, err)
	}
	return fields, nil
}

func describedBySource(description string, sourceLabel string) string {
	return strings.TrimSpace(strings.Join(strings.Fields(description), " ") + " (" + sourceLabel + ")")
}
