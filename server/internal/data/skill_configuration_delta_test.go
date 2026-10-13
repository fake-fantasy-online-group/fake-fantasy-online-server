package data

import (
	"encoding/json"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"os"
	"testing"
)

type skillConfigurationGoldenRow struct {
	ID     domain.SkillID
	Rank   int32
	SHA256 string
}

// Apply a small, reviewed delta rather than silently regenerating the full
// baseline. PreviousSHA256 anchors every change to its preserved old snapshot.
func loadSkillConfigurationGolden(t *testing.T) []skillConfigurationGoldenRow {
	t.Helper()
	var rows []skillConfigurationGoldenRow
	raw, err := os.ReadFile("testdata/skill-config-golden-v7.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1490 {
		t.Fatal("golden baseline coverage", len(rows))
	}
	index := map[domain.SkillKey]int{}
	for i, r := range rows {
		k := domain.SkillKey{ID: r.ID, Level: r.Rank}
		if _, ok := index[k]; ok {
			t.Fatal("duplicate baseline", k)
		}
		index[k] = i
	}
	var changes []struct {
		ID             domain.SkillID
		Rank           int32
		SHA256         string
		PreviousSHA256 string
	}
	raw, err = os.ReadFile("testdata/skill-config-golden-v8-delta.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &changes); err != nil {
		t.Fatal(err)
	}
	if len(changes) != 50 {
		t.Fatal("reviewed delta coverage", len(changes))
	}
	seen := map[domain.SkillKey]bool{}
	counts := map[domain.SkillID]int{}
	for _, c := range changes {
		switch c.ID {
		case 10028, 10127, 10313, 10317, 10326:
		default:
			t.Fatal("unreviewed changed identity", c.ID)
		}
		k := domain.SkillKey{ID: c.ID, Level: c.Rank}
		i, ok := index[k]
		if !ok || seen[k] {
			t.Fatal("missing/duplicate delta identity", k)
		}
		if rows[i].SHA256 != c.PreviousSHA256 || c.SHA256 == c.PreviousSHA256 || len(c.SHA256) != 64 {
			t.Fatal("invalid baseline-anchored delta", k)
		}
		seen[k] = true
		counts[c.ID]++
		rows[i].SHA256 = c.SHA256
	}
	for id, n := range counts {
		if n != 10 {
			t.Fatal("reviewed rank coverage", id, n)
		}
	}
	return rows
}
func TestSkillConfigurationGoldenDeltaIntegrity(t *testing.T) { loadSkillConfigurationGolden(t) }
