package ui

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var slashTestNow = time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)

func TestSlashHistory(t *testing.T) {
	t.Run("useCountsOneMoreUnderItsNameAndPutsItFirst", useCountsOneMoreUnderItsNameAndPutsItFirst)
	t.Run("useAtThousandInAllKeepsEveryCount", useAtThousandInAllKeepsEveryCount)
	t.Run("usePastThousandInAllHalvesEveryCountAndDropsZeros", usePastThousandInAllHalvesEveryCountAndDropsZeros)
	t.Run("boostGrowsFourPerDoublingOfCountWeightedByAge", boostGrowsFourPerDoublingOfCountWeightedByAge)
	t.Run("missingFileGivesNoUses", missingFileGivesNoUses)
	t.Run("writtenUsesReadBack", writtenUsesReadBack)
}

func useCountsOneMoreUnderItsNameAndPutsItFirst(t *testing.T) {
	first, second, third := slashTestNow, slashTestNow.Add(time.Minute), slashTestNow.Add(2*time.Minute)
	uses := SlashUsesAfterUse([]SlashUse{}, "usage", first)
	uses = SlashUsesAfterUse(uses, "model", second)
	uses = SlashUsesAfterUse(uses, "usage", third)
	want := []SlashUse{{Name: "usage", Count: 2, Last: third}, {Name: "model", Count: 1, Last: second}}
	require.Equal(t, want, uses)
}

func useAtThousandInAllKeepsEveryCount(t *testing.T) {
	earlier := slashTestNow.Add(-time.Hour)
	atThousand := []SlashUse{{Name: "usage", Count: 998, Last: earlier}, {Name: "model", Count: 1, Last: earlier}}
	uses := SlashUsesAfterUse(atThousand, "effort", slashTestNow)
	require.Len(t, uses, 3)
	require.Equal(t, 998, uses[1].Count)
}

func usePastThousandInAllHalvesEveryCountAndDropsZeros(t *testing.T) {
	earlier := slashTestNow.Add(-time.Hour)
	pastThousand := []SlashUse{{Name: "usage", Count: 999, Last: earlier}, {Name: "model", Count: 1, Last: earlier}}
	uses := SlashUsesAfterUse(pastThousand, "effort", slashTestNow)
	want := []SlashUse{{Name: "usage", Count: 499, Last: earlier}}
	require.Equal(t, want, uses)
}

func boostGrowsFourPerDoublingOfCountWeightedByAge(t *testing.T) {
	uses := []SlashUse{
		{Name: "within-hour", Count: 4, Last: slashTestNow.Add(-30 * time.Minute)},
		{Name: "within-day", Count: 4, Last: slashTestNow.Add(-2 * time.Hour)},
		{Name: "within-week", Count: 4, Last: slashTestNow.Add(-48 * time.Hour)},
		{Name: "older", Count: 4, Last: slashTestNow.Add(-30 * 24 * time.Hour)},
		{Name: "older-twice", Count: 8, Last: slashTestNow.Add(-30 * 24 * time.Hour)},
	}
	// 4 times log2 of count times weight plus one, rounded down: 4×16+1, 4×8+1, 4×2+1, 4+1, 8+1
	want := map[string]int{"within-hour": 24, "within-day": 20, "within-week": 12, "older": 8, "older-twice": 12}
	boosts := SlashBoosts(uses, slashTestNow)
	require.Equal(t, want, boosts)
}

func missingFileGivesNoUses(t *testing.T) {
	historyFile := filepath.Join(t.TempDir(), "calmpane", "slash-history.json")
	missing, err := SavedSlashUses(historyFile)
	require.NoError(t, err)
	require.Empty(t, missing)
}

func writtenUsesReadBack(t *testing.T) {
	historyFile := filepath.Join(t.TempDir(), "calmpane", "slash-history.json")
	uses := []SlashUse{{Name: "usage", Count: 3, Last: slashTestNow}}
	writeErr := WriteSlashUses(historyFile, uses)
	require.NoError(t, writeErr)
	saved, err := SavedSlashUses(historyFile)
	require.NoError(t, err)
	require.Equal(t, uses, saved)
}
