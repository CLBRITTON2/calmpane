package ui

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

var slashTestNow = time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)

func TestSlashHistory(t *testing.T) {
	t.Run("useCountsOneMoreUnderItsNameAndPutsItFirst", useCountsOneMoreUnderItsNameAndPutsItFirst)
	t.Run("usePastThousandInAllHalvesEveryCountAndDropsZeros", usePastThousandInAllHalvesEveryCountAndDropsZeros)
	t.Run("boostGrowsFourPerDoublingOfCountWeightedByAge", boostGrowsFourPerDoublingOfCountWeightedByAge)
	t.Run("writtenUsesReadBackAndMissingFileGivesNone", writtenUsesReadBackAndMissingFileGivesNone)
}

func useCountsOneMoreUnderItsNameAndPutsItFirst(t *testing.T) {
	first, second, third := slashTestNow, slashTestNow.Add(time.Minute), slashTestNow.Add(2*time.Minute)
	uses := SlashUsesAfterUse([]SlashUse{}, "usage", first)
	uses = SlashUsesAfterUse(uses, "model", second)
	uses = SlashUsesAfterUse(uses, "usage", third)
	want := []SlashUse{{Name: "usage", Count: 2, Last: third}, {Name: "model", Count: 1, Last: second}}
	if !reflect.DeepEqual(uses, want) {
		t.Errorf("uses = %+v, want %+v", uses, want)
	}
}

func usePastThousandInAllHalvesEveryCountAndDropsZeros(t *testing.T) {
	earlier := slashTestNow.Add(-time.Hour)
	atThousand := []SlashUse{{Name: "usage", Count: 998, Last: earlier}, {Name: "model", Count: 1, Last: earlier}}
	if uses := SlashUsesAfterUse(atThousand, "effort", slashTestNow); len(uses) != 3 || uses[1].Count != 998 {
		t.Errorf("uses at a total of 1000 = %+v, want the counts kept", uses)
	}
	pastThousand := []SlashUse{{Name: "usage", Count: 999, Last: earlier}, {Name: "model", Count: 1, Last: earlier}}
	uses := SlashUsesAfterUse(pastThousand, "effort", slashTestNow)
	want := []SlashUse{{Name: "usage", Count: 499, Last: earlier}}
	if !reflect.DeepEqual(uses, want) {
		t.Errorf("uses past a total of 1000 = %+v, want %+v", uses, want)
	}
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
	if boosts := SlashBoosts(uses, slashTestNow); !reflect.DeepEqual(boosts, want) {
		t.Errorf("boosts = %v, want %v", boosts, want)
	}
}

func writtenUsesReadBackAndMissingFileGivesNone(t *testing.T) {
	historyFile := filepath.Join(t.TempDir(), "calmpane", "slash-history.json")
	missing, err := SavedSlashUses(historyFile)
	if err != nil {
		t.Fatalf("SavedSlashUses of a missing file: %v", err)
	}
	if len(missing) != 0 {
		t.Errorf("uses of a missing file = %+v, want none", missing)
	}
	uses := []SlashUse{{Name: "usage", Count: 3, Last: slashTestNow}}
	if err := WriteSlashUses(historyFile, uses); err != nil {
		t.Fatalf("WriteSlashUses: %v", err)
	}
	saved, err := SavedSlashUses(historyFile)
	if err != nil {
		t.Fatalf("SavedSlashUses: %v", err)
	}
	if !reflect.DeepEqual(saved, uses) {
		t.Errorf("saved uses = %+v, want %+v", saved, uses)
	}
}
