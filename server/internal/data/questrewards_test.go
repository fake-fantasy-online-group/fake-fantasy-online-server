package data

import (
	"context"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/jackc/pgx/v5"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

type questWeaponQuery struct{ records [][]any }

func (q questWeaponQuery) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return &cubeConfigRows{records: q.records}, nil
}
func TestQuestProfessionRewardsMultipleAndLegacy(t *testing.T) {
	for _, task := range []int32{42, 86} {
		rows := [][]any{}
		for race := int32(0); race < 5; race++ {
			rows = append(rows, []any{task, race, int32(8000 + race), int32(3), true})
			if task == 42 && (race == 1 || race == 2) {
				rows = append(rows, []any{task, race, int32(9000 + race), int32(0), true})
			}
		}
		quests := domain.QuestTable{domain.QuestID(task): {ID: domain.QuestID(task)}}
		if err := loadQuestProfessionRewards(context.Background(), questWeaponQuery{rows}, quests); err != nil {
			t.Fatal(err)
		}
		for race := domain.Race(0); race < 5; race++ {
			want := 1
			if task == 42 && (race == 1 || race == 2) {
				want = 2
			}
			rs := quests[domain.QuestID(task)].ProfessionRewards[race]
			if len(rs) != want || rs[0].RefineLevel != 3 {
				t.Fatalf("task %d race %d: %+v", task, race, rs)
			}
		}
	}
}
func TestQuestProfessionRewardsRejectInvalid(t *testing.T) {
	for _, mode := range []string{"duplicate", "missing_route", "not_equipment", "unknown_task"} {
		rows := [][]any{}
		for race := int32(0); race < 5; race++ {
			rows = append(rows, []any{int32(42), race, int32(8000 + race), int32(0), true})
		}
		switch mode {
		case "duplicate":
			rows = append(rows, rows[0])
		case "missing_route":
			rows = rows[:4]
		case "not_equipment":
			rows[0][4] = false
		case "unknown_task":
			rows[0][0] = int32(99)
		}
		if err := loadQuestProfessionRewards(context.Background(), questWeaponQuery{rows}, domain.QuestTable{42: {ID: 42}}); err == nil {
			t.Fatalf("accepted %s", mode)
		}
	}
}

func TestQuestProfessionRewardMigrationManifest(t *testing.T) {
	init, err := os.ReadFile("../../migrations/000000_init-100.sql")
	if err != nil {
		t.Fatal(err)
	}
	upgrade, err := os.ReadFile("../../migrations/upgrades/000100_20261022_quest_special_weapons.sql")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(upgrade), string(init)) {
		t.Fatal("fresh and upgrade diverged")
	}
	matches := regexp.MustCompile(`\((\d+),(\d+),(\d+),0\)`).FindAllStringSubmatch(string(init), -1)
	if len(matches) != 35 {
		t.Fatalf("got %d grants", len(matches))
	}
	offsets := map[int]int{85: 0, 87: 0, 36: 1, 37: 1, 42: 2}
	base := map[int][]int{0: {1618}, 1: {1019, 1518}, 2: {1818, 1920}, 3: {1318}, 4: {1318}}
	seen := map[string]bool{}
	for _, m := range matches {
		task, _ := strconv.Atoi(m[1])
		race, _ := strconv.Atoi(m[2])
		item, _ := strconv.Atoi(m[3])
		offset, ok := offsets[task]
		if !ok {
			t.Fatalf("unexpected task %d", task)
		}
		valid := false
		for _, id := range base[race] {
			if item == id+offset {
				valid = true
			}
		}
		if !valid || seen[m[0]] {
			t.Fatalf("invalid or repeated reward %s", m[0])
		}
		seen[m[0]] = true
	}
}
