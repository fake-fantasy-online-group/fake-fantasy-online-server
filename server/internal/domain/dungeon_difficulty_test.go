package domain

import "testing"

func TestNativeDungeonDifficultyCommands(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		id   int32
		diff DungeonDifficulty
	}{
		{"taoyu", 20502, DungeonNormal}, {"taoyu hard", 20502, DungeonHard},
		{"dungeonhard 20100", 20100, DungeonHard}, {"dungeonhard 20300", 20300, DungeonHard},
		{"dungeonhard 20600", 20600, DungeonHard}, {"dungeonhard 20701", 20701, DungeonHard},
		{"dungeonhard 20801", 20801, DungeonHard}, {"dungeonhard 20901", 20901, DungeonHard},
	} {
		id, d, _, ok := NativeDungeonCommand(tc.raw)
		if !ok || id != tc.id || d != tc.diff {
			t.Fatal(tc, id, d, ok)
		}
	}
	for _, raw := range []string{"/gm taoyu hard", "dungeonhard 20502", "dungeonhard 20802", "dungeonhard 020100", "taoyu hard ", "taoyu  hard", "taoyu Hard", "dungeonhard 7"} {
		if _, _, _, ok := NativeDungeonCommand(raw); ok {
			t.Fatal("untrusted variant accepted", raw)
		}
	}
}
