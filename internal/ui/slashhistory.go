package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math/bits"
	"os"
	"path/filepath"
	"time"
)

// SlashUse counts the times zen sent a slash command, under the command's name.
type SlashUse struct {
	Name  string    `json:"name"`
	Count int       `json:"count"`
	Last  time.Time `json:"last"`
}

// slashUsesMaxTotal is the sum of counts past which every count is halved, so old favourites fade.
const slashUsesMaxTotal = 1000

// slashBoostPerDoubling is the boost a command gains each time its weighted count doubles.
const slashBoostPerDoubling = 4

// SlashUsesAfterUse puts name first. Past slashUsesMaxTotal uses in all, every count halves so old favourites fade.
func SlashUsesAfterUse(uses []SlashUse, name string, now time.Time) []SlashUse {
	used := SlashUse{Name: name, Count: 1, Last: now}
	others := []SlashUse{}
	total := 1
	for _, use := range uses {
		if use.Name == name {
			used.Count = use.Count + 1
			total += use.Count
			continue
		}
		others = append(others, use)
		total += use.Count
	}
	updated := append([]SlashUse{used}, others...)
	if total <= slashUsesMaxTotal {
		return updated
	}
	halved := []SlashUse{}
	for _, use := range updated {
		if use.Count/2 > 0 {
			halved = append(halved, SlashUse{Name: use.Name, Count: use.Count / 2, Last: use.Last})
		}
	}
	return halved
}

// SlashBoosts weights each count by how lately the command was used.
func SlashBoosts(uses []SlashUse, now time.Time) map[string]int {
	boosts := map[string]int{}
	for _, use := range uses {
		age := now.Sub(use.Last)
		weight := 1
		switch {
		case age < time.Hour:
			weight = 16
		case age < 24*time.Hour:
			weight = 8
		case age < 7*24*time.Hour:
			weight = 2
		}
		boosts[use.Name] = slashBoostPerDoubling * (bits.Len(uint(use.Count*weight+1)) - 1)
	}
	return boosts
}

// SavedSlashUses is empty when historyFile does not exist.
func SavedSlashUses(historyFile string) ([]SlashUse, error) {
	content, err := os.ReadFile(historyFile)
	if errors.Is(err, fs.ErrNotExist) {
		return []SlashUse{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("Cannot read the slash command history %s: %w", historyFile, err)
	}
	var uses []SlashUse
	if err := json.Unmarshal(content, &uses); err != nil {
		return nil, fmt.Errorf("Cannot read the slash command history %s: %w", historyFile, err)
	}
	return uses, nil
}

func WriteSlashUses(historyFile string, uses []SlashUse) error {
	content, err := json.MarshalIndent(uses, "", "  ")
	if err != nil {
		return fmt.Errorf("encode the slash command history: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(historyFile), 0o755); err != nil {
		return fmt.Errorf("Cannot write the slash command history %s: %w", historyFile, err)
	}
	if err := os.WriteFile(historyFile, content, 0o600); err != nil {
		return fmt.Errorf("Cannot write the slash command history %s: %w", historyFile, err)
	}
	return nil
}
