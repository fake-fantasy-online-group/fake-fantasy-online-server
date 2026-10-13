package data

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func TestMonsterFamiliesPostgresActualLoader(t *testing.T) {
	path := os.Getenv("FANTASY_MONSTER_FAMILIES_PG_DATA")
	if path == "" {
		t.Skip("isolated migration71 database required")
	}
	if _, err := os.Stat(filepath.Join(path, "postmaster.pid")); err == nil {
		t.Fatal("refusing active cluster")
	}
	if os.Getenv("PG_BIN") == "" {
		t.Fatal("PG_BIN required")
	}
	q := &v5SingleUserQuerier{bin: os.Getenv("PG_BIN"), path: path, database: "postgres"}
	monsters, err := LoadMonsters(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[domain.MonsterFamily]int{}
	for _, m := range monsters {
		if m.Kind.Hostile() {
			counts[m.Family]++
		} else if m.Family != domain.MonsterFamilyUnknown {
			t.Fatal("noncombat row classified", m.ID)
		}
	}
	for family, want := range map[domain.MonsterFamily]int{domain.MonsterFamilyUndead: 90, domain.MonsterFamilyDemon: 87, domain.MonsterFamilyUnknown: 244, domain.MonsterFamilyOther: 857} {
		if counts[family] != want {
			t.Fatalf("family %s got%d want%d", family, counts[family], want)
		}
	}
	for _, tc := range []struct {
		id     domain.MonsterID
		family domain.MonsterFamily
	}{
		{1025, domain.MonsterFamilyUndead}, {4100, domain.MonsterFamilyUndead}, {5013, domain.MonsterFamilyUndead},
		{3173, domain.MonsterFamilyUnknown}, {1209, domain.MonsterFamilyUndead}, {4158, domain.MonsterFamilyUndead},
		{6016, domain.MonsterFamilyDemon}, {6114, domain.MonsterFamilyDemon}, {6214, domain.MonsterFamilyDemon},
		{6080, domain.MonsterFamilyUnknown}, {31009, domain.MonsterFamilyUnknown}, {31736, domain.MonsterFamilyOther},
		{4131, domain.MonsterFamilyOther}, {3086, domain.MonsterFamilyUnknown},
	} {
		if m, ok := monsters[tc.id]; !ok || m.Family != tc.family {
			t.Fatalf("monster%d: %+v", tc.id, m)
		}
	}
	t.Logf("actual LoadMonsters: %d total, 1278 reviewed combat templates, 177 undead/demon", len(monsters))
}
