package data

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
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
	// Each version is anchored to the preceding reviewed snapshot. In particular,
	// 10313 appears in both deltas, so v9 must use its v8 hash, not its v7 hash.
	// Migration73's v10 delta is restricted to Advanced Backstab (10201) at
	// exactly the same ten existing ranks; all earlier anchors remain intact.
	for _, delta := range []struct {
		file string
		ids  []domain.SkillID
	}{
		{"testdata/skill-config-golden-v8-delta.json", []domain.SkillID{10028, 10127, 10313, 10317, 10326}},
		{"testdata/skill-config-golden-v9-delta.json", []domain.SkillID{10102, 10212, 10312, 10313}},
		{"testdata/skill-config-golden-v10-delta.json", []domain.SkillID{10201}},
	} {
		var changes []struct {
			ID             domain.SkillID
			Rank           int32
			SHA256         string
			PreviousSHA256 string
		}
		raw, err = os.ReadFile(delta.file)
		if err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(raw, &changes); err != nil {
			t.Fatal(err)
		}
		if len(changes) != len(delta.ids)*10 {
			t.Fatal(delta.file, "reviewed delta coverage", len(changes))
		}
		// Exact identities and ranks are reviewed, not inferred from the changes.
		expected := map[domain.SkillKey]bool{}
		for _, id := range delta.ids {
			for _, rank := range []int32{1, 2, 3, 4, 5, 20, 21, 60, 255, 65536} {
				expected[domain.SkillKey{ID: id, Level: rank}] = true
			}
		}
		for _, c := range changes {
			k := domain.SkillKey{ID: c.ID, Level: c.Rank}
			i, ok := index[k]
			if !ok || !expected[k] {
				t.Fatal(delta.file, "unreviewed/missing/duplicate delta identity", k)
			}
			hash, err := hex.DecodeString(c.SHA256)
			if rows[i].SHA256 != c.PreviousSHA256 || c.SHA256 == c.PreviousSHA256 || err != nil || len(hash) != 32 {
				t.Fatal(delta.file, "invalid baseline-anchored delta", k)
			}
			delete(expected, k)
			rows[i].SHA256 = c.SHA256
		}
		if len(expected) != 0 {
			t.Fatal(delta.file, "missing reviewed rank coverage", expected)
		}
	}
	return rows
}
func TestSkillConfigurationGoldenDeltaIntegrity(t *testing.T) { loadSkillConfigurationGolden(t) }
