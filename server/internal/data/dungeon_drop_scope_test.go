package data

import (
	"context"
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/jackc/pgx/v5"
	"strings"
	"testing"
)

type dungeonDropScopeQuery struct{}

func (dungeonDropScopeQuery) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	switch {
	case strings.Contains(sql, "FROM gamedata.ov_arm"):
		return &cubeConfigRows{}, nil
	case strings.Contains(sql, "FROM gamedata.ov_item"):
		return &cubeConfigRows{records: [][]any{{int32(3716), int32(28), int32(0), int32(1)}}}, nil
	case strings.Contains(sql, "FROM game_monster_drops"):
		if !strings.Contains(sql, "item_src") {
			return nil, fmt.Errorf("loader omitted dungeon provenance")
		}
		return &cubeConfigRows{records: [][]any{
			{int32(1041), int32(4162), float32(28.758707), "", int32(0), int32(0), "item"},
			{int32(1041), int32(3034), float32(60), "", int32(0), int32(0), "dungeon_template:5019:small"},
			{int32(1041), int32(0), float32(0.4), "卡片", int32(1), int32(2), "dungeon_template:5019:small"},
			{int32(1041), int32(0), float32(0.2), "卡片", int32(1), int32(2), ""},
		}}, nil
	}
	return nil, fmt.Errorf("unexpected query %s", sql)
}
func TestLoadDropsPreservesDungeonScopeAndRates(t *testing.T) {
	items := ItemDefs{4162: domain.ItemDef{ID: 4162}, 3034: domain.ItemDef{ID: 3034}, 3716: domain.ItemDef{ID: 3716}}
	drops, stats, err := LoadDrops(context.Background(), dungeonDropScopeQuery{}, items)
	if err != nil {
		t.Fatal(err)
	}
	entries := drops[1041]
	if len(entries) != 4 || stats.Total != 4 {
		t.Fatalf("entries=%+v stats=%+v", entries, stats)
	}
	for i, want := range []bool{false, true, true, false} {
		if entries[i].DungeonOnly != want {
			t.Fatalf("row%d scope=%v want%v", i, entries[i].DungeonOnly, want)
		}
	}
	for i, rate := range []float32{28.758707, 60, 0.4, 0.2} {
		if entries[i].RatePct != float64(rate) {
			t.Fatalf("row%d rate changed: %v", i, entries[i].RatePct)
		}
	}
	if len(entries[2].Choices) != 1 || entries[2].Choices[0] != 3716 || !entries[2].RollAffixes {
		t.Fatalf("category pool lost: %+v", entries[2])
	}
}
