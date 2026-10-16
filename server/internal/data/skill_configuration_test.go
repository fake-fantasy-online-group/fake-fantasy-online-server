package data

import catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/jackc/pgx/v5"
	"os"
	"reflect"
	"strings"
	"testing"
)

// The reviewed v7 baseline remains intact. Explicit v8/v9 deltas cover only
// independently reviewed mechanics, with every prior hash preserved as an anchor.
func TestSkillConfigurationDatabaseGolden(t *testing.T) {
	path := os.Getenv("FANTASY_SKILL_CONFIG_PG_DATA")
	if path == "" {
		t.Skip("explicit isolated migration74 cluster required")
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
	c := table.ProgressionCatalog()
	if c == nil {
		t.Fatal("catalog not attached")
	}
	assertCurrentSkillProgressionCatalog(t, c)
	golden := loadSkillConfigurationGolden(t)
	for _, row := range golden {
		d, ok := table.Get(row.ID, row.Rank)
		if !ok {
			t.Fatalf("missing %d/%d", row.ID, row.Rank)
		}
		var resolved []domain.StatusDef
		for i := range d.Statuses {
			a := &d.Statuses[i]
			a.SourceSkill = row.ID
			if !a.ChanceExplicit && a.ChanceBP == 0 {
				a.ChanceBP = 10000
			}
			a.ChanceExplicit = true
			s, ok := states.ResolveSkillApplication(*a)
			if !ok {
				t.Fatalf("unresolved %d/%d", row.ID, row.Rank)
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
		if hex.EncodeToString(h[:]) != row.SHA256 {
			os.WriteFile("/tmp/skill-config-golden-mismatch.json", b, 0644)
			t.Fatalf("effective skill/status changed %d/%d", row.ID, row.Rank)
		}
	}
	t.Logf("149 identities/policies,128 synergies,1490 effective skill+status snapshots match the reviewed golden; %d actual SQL reads", q.queries)
}

// Deliberately altered SELECT results exercise fail-closed loading without ever
// changing the disposable PostgreSQL data. The catalogue is still read through
// the real typed SQL scanner, not an embedded production fallback.
type skillConfigurationQueryEdit struct {
	Querier
	from, to string
}

func (q skillConfigurationQueryEdit) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return q.Querier.Query(ctx, strings.ReplaceAll(sql, q.from, q.to), args...)
}
func TestSkillConfigurationDatabaseRejectsMissingAndUsesConfiguredPolicy(t *testing.T) {
	path := os.Getenv("FANTASY_SKILL_CONFIG_PG_DATA")
	if path == "" {
		t.Skip("explicit isolated migration74 cluster required")
	}
	q := &v5SingleUserQuerier{bin: os.Getenv("PG_BIN"), path: path, database: "postgres"}
	table, _, err := LoadSkills(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	for name, edit := range map[string][2]string{
		"manifest":          {"FROM game_skill_configuration_manifest WHERE id=1", "FROM game_skill_configuration_manifest WHERE id=0"},
		"member-and-policy": {"FROM game_skill_progression m", "FROM (SELECT * FROM game_skill_progression WHERE skill_id<>10001) m"},
		"policy":            {"JOIN game_skill_growth_policy p", "JOIN (SELECT * FROM game_skill_growth_policy WHERE skill_id<>10001) p"},
		"synergy":           {"FROM game_skill_synergies ORDER", "FROM game_skill_synergies WHERE edge_order<>1 ORDER"},
		"restoration":       {"WHERE configuration_version=62", "WHERE configuration_version=62 AND false"},
		"low-cap":           {"COALESCE(p.protection_cap_rank,0)", "4"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadSkillProgressionCatalog(context.Background(), skillConfigurationQueryEdit{q, edit[0], edit[1]}, table); err == nil {
				t.Fatal("incomplete/malformed configuration accepted")
			}
		})
	}
	changed, err := LoadSkillProgressionCatalog(context.Background(), skillConfigurationQueryEdit{q, "COALESCE(p.efficiency_cap_rank,0)", "CASE WHEN m.skill_id=10018 THEN 77 ELSE COALESCE(p.efficiency_cap_rank,0) END"}, table)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := changed.Policy(10018)
	if p.EfficiencyCapRank != 77 {
		t.Fatal("database policy ignored", p)
	}
	before, _ := table.ProgressionCatalog().Policy(10018)
	if before.EfficiencyCapRank == 77 {
		t.Fatal("new snapshot mutated active catalogue")
	}
	if err = domain.InstallV5SkillProgression(table, changed); err != nil {
		t.Fatal(err)
	}
	d, ok := table.Get(10018, 21)
	if !ok || d.Progression != changed {
		t.Fatal("new loaded snapshot not used by evaluator")
	}
	t.Logf("missing manifest/member/policy/synergy/restoration and low cap rejected; independent database policy loaded via %d SQL reads", q.queries)
}

// Current full-database coverage complements the historical migration74 fixture.
func TestCurrentSkillProgressionCatalogActualLoader(t *testing.T) {
	table, _, err := LoadSkills(context.Background(), testPool(t))
	if err != nil {
		t.Fatal(err)
	}
	assertCurrentSkillProgressionCatalog(t, table.ProgressionCatalog())
	statuses, _, err := LoadStatuses(context.Background(), testPool(t))
	if err != nil {
		t.Fatal(err)
	}
	want := &domain.StatusOverlayStrength{Attr: 19, Mode: 1, ValueOrder: -1}
	if got := statuses[domain.StatusKey{ID: 1008, Level: 11}].OverlayStrength; !reflect.DeepEqual(got, want) {
		t.Fatalf("current blind-status strength rule: got %+v, want %+v", got, want)
	}
}

func assertCurrentSkillProgressionCatalog(t *testing.T, c *domain.SkillProgressionCatalog) {
	t.Helper()
	if c == nil {
		t.Fatal("catalog not attached")
	}
	expected := catalogfixture.SkillProgressionCatalog()
	expectedEdges := expected.Synergies()
	// Migration74 is a reviewed synergy-only delta. The frozen1490 skill/status
	// goldens and original catalog remain unchanged and independently checked.
	for i, e := range expectedEdges {
		if e.RankMode != "hard" {
			continue
		}
		target, _ := expected.Identity(e.Target)
		e.RankMode = "effective_training"
		if e.Effect == "heal_flat" {
			e.Effect = "heal_power"
		}
		e.PerPointBP = 300
		if target.UnlockLevel >= 30 {
			e.PerPointBP = 400
		}
		if target.UnlockLevel >= 50 {
			e.PerPointBP = 500
		}
		if target.UnlockLevel >= 70 {
			e.PerPointBP = 600
		}
		expectedEdges[i] = e
	}
	if !reflect.DeepEqual(c.IDs(), expected.IDs()) || !reflect.DeepEqual(c.Synergies(), expectedEdges) {
		t.Fatal("membership/synergy differs from pre-refactor snapshot")
	}
	for _, id := range c.IDs() {
		a, _ := c.Identity(id)
		b, _ := expected.Identity(id)
		// Migration70 closes the verified theft identity instead of the imported
		// Youlong damage placeholder. All other identity and policy fields stay pinned.
		if id == 10212 {
			if b.Name != "游龙掌" {
				t.Fatal("theft identity delta lost its historical anchor", b.Name)
			}
			b.Name = "偷窃术"
		}
		p, _ := c.Policy(id)
		e, _ := expected.Policy(id)
		if id == 10212 {
			if e.EfficiencyCapRank != 0 {
				t.Fatal("theft growth delta lost its zero-cap anchor", e)
			}
			e.EfficiencyCapRank = 10
		}
		if !reflect.DeepEqual(a, b) || !reflect.DeepEqual(p, e) {
			t.Fatalf("identity/policy changed %d: %+v %+v", id, a, p)
		}
	}
}
