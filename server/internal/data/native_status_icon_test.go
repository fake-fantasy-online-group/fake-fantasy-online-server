package data

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// Executes the actual SQL loader, optionally against an isolated idle copied
// cluster through the existing read-only adapter. The original resource is an
// optional local audit input, never a committed test asset.
func TestNativeStatusIconsActualReadOnlyLoader(t *testing.T) {
	var q Querier
	if path := os.Getenv("FANTASY_V5_SKILL_PG_DATA"); path != "" {
		if _, err := os.Stat(filepath.Join(path, "postmaster.pid")); err == nil {
			t.Fatal("use an idle disposable copied cluster")
		}
		bin := os.Getenv("PG_BIN")
		if bin == "" {
			t.Fatal("PG_BIN required")
		}
		db := os.Getenv("FANTASY_V5_SKILL_PG_DATABASE")
		if db == "" {
			db = "fresh_test"
		}
		q = &v5SingleUserQuerier{bin: bin, path: path, database: db}
	} else {
		q = testPool(t)
	}
	table, stats, err := LoadStatuses(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		id          domain.StatusID
		level, icon int32
	}{
		{1001, 1, 5030}, {1001, 30, 5030}, {1018, 1, 7037}, {1004, 1, 7087},
		{1023, 18, 4495}, {1023, 19, 3324}, {1036, 6, 7081}, {1036, 7, 10179},
		{1176, 1, 4419}, {1176, 2, 0},
	} {
		d, ok := table.Get(tc.id, tc.level)
		if !ok || !d.IconKnown || d.Icon != tc.icon {
			t.Fatalf("native %d/%d want icon%d, got %+v", tc.id, tc.level, tc.icon, d)
		}
	}
	checked := 0
	if root := os.Getenv("FANTASY_STATUS_ICON_SOURCE_DIR"); root != "" {
		original, err := LoadOvTable(filepath.Join(root, "ov_exceptdesc.bin"), "ov_exceptdesc")
		if err != nil {
			t.Fatal(err)
		}
		if original.RecordSize != 12324 || len(original.Rows) != 206 {
			t.Fatal("unexpected original status layout")
		}
		for _, r := range original.Rows {
			id := domain.StatusID(binary.LittleEndian.Uint32(r))
			if id <= 0 {
				continue
			}
			for level := int32(1); level <= 40; level++ {
				d, ok := table.Get(id, level)
				if !ok {
					continue
				}
				want := int32(binary.LittleEndian.Uint32(r[4+(level-1)*308+288:]))
				if !d.IconKnown || d.Icon != want {
					t.Fatalf("original status%d/%d want%d got%d known%v", id, level, want, d.Icon, d.IconKnown)
				}
				checked++
			}
		}
	}
	monsters, err := LoadMonsters(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	type source struct {
		MonsterID   domain.MonsterID
		MonsterName string
		Rule        domain.MonsterSkillRule
	}
	var sources []source
	seen := map[domain.SkillKey]bool{}
	ids := make([]int, 0, len(monsters))
	for id := range monsters {
		ids = append(ids, int(id))
	}
	sort.Ints(ids)
	references := 0
	for _, id := range ids {
		m := monsters[domain.MonsterID(id)]
		for _, rule := range m.Skills {
			poison := false
			for _, a := range rule.Skill.Statuses {
				poison = poison || a.ID == domain.StatusPoison
			}
			for _, e := range rule.Extra {
				for _, a := range e.Statuses {
					poison = poison || a.ID == domain.StatusPoison
				}
			}
			if !poison {
				continue
			}
			references++
			key := domain.SkillKey{ID: rule.Skill.ID, Level: rule.Skill.Level}
			if !seen[key] {
				sources = append(sources, source{m.ID, m.Name, rule})
				seen[key] = true
			}
		}
	}
	if len(sources) == 0 {
		t.Fatal("no actual monster poison rules found")
	}
	if path := os.Getenv("FANTASY_STATUS_ICON_RUNTIME_FIXTURE"); path != "" {
		statuses := make([]domain.StatusDef, 0, len(table))
		for _, d := range table {
			statuses = append(statuses, d)
		}
		sort.Slice(statuses, func(i, j int) bool {
			a, b := statuses[i], statuses[j]
			return a.ID < b.ID || a.ID == b.ID && a.Level < b.Level
		})
		payload := struct {
			Source   string
			Statuses []domain.StatusDef
			Sources  []source
		}{"Actual LoadStatuses and LoadMonsters from isolated initialized PostgreSQL", statuses, sources}
		b, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("PASS %d loaded definitions, %d original levels byte-compared; %d actual monster poison references / %d distinct skill-level rules", stats.Defs, checked, references, len(sources))
}
