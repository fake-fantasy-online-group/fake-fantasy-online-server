package data

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// An opt-in audit export of actual LoadSkills/LoadStatuses output.
// It never replaces SQL, updates the baseline, or writes a reviewed delta.
func TestExportSkillConfigurationSnapshots(t *testing.T) {
	path := os.Getenv("FANTASY_SKILL_CONFIG_PG_DATA")
	out := os.Getenv("FANTASY_SKILL_CONFIG_EXPORT_OUTPUT")
	if out == "" {
		t.Skip("explicit FANTASY_SKILL_CONFIG_EXPORT_OUTPUT required")
	}
	if path == "" || os.Getenv("PG_BIN") == "" {
		t.Fatal("explicit idle disposable PostgreSQL data, PostgreSQL binary and output required")
	}
	if _, err := os.Stat(path + "/postmaster.pid"); !os.IsNotExist(err) {
		t.Fatal("cluster must be idle; postmaster.pid exists or could not be checked", err)
	}
	q := &v5SingleUserQuerier{bin: os.Getenv("PG_BIN"), path: path, database: "postgres"}
	table, _, err := LoadSkills(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	states, _, err := LoadStatuses(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		ID      domain.SkillID
		Rank    int32
		SHA256  string
		Payload json.RawMessage
	}
	var rows []row
	raw, err := os.ReadFile("testdata/skill-config-golden-v7.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1490 {
		t.Fatal("unexpected baseline coverage", len(rows))
	}
	seen := map[domain.SkillKey]bool{}
	for i := range rows {
		r := &rows[i]
		key := domain.SkillKey{ID: r.ID, Level: r.Rank}
		if seen[key] {
			t.Fatal("duplicate baseline key", key)
		}
		seen[key] = true
		d, ok := table.Get(r.ID, r.Rank)
		if !ok {
			t.Fatalf("missing %d/%d", r.ID, r.Rank)
		}
		var resolved []domain.StatusDef
		for j := range d.Statuses {
			a := &d.Statuses[j]
			a.SourceSkill = r.ID
			if !a.ChanceExplicit && a.ChanceBP == 0 {
				a.ChanceBP = 10000
			}
			a.ChanceExplicit = true
			s, ok := states.ResolveSkillApplication(*a)
			if !ok {
				t.Fatalf("unresolved %d/%d", r.ID, r.Rank)
			}
			resolved = append(resolved, s)
		}
		b, err := json.Marshal(struct {
			Skill    domain.SkillDef
			Statuses []domain.StatusDef
		}{d, resolved})
		if err != nil {
			t.Fatal(err)
		}
		h := sha256.Sum256(b)
		r.SHA256 = hex.EncodeToString(h[:])
		r.Payload = b
	}
	b, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	// Never overwrite a baseline, a reviewed delta, or an earlier export.
	f, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.Write(append(b, '\n')); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	t.Logf("exported %d production-resolved skill snapshots using %d read-only PostgreSQL queries", len(rows), q.queries)
}
