package data

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture"
	"github.com/jackc/pgx/v5"
)

type riftSlotFallbackQuery struct{ rules []domain.RiftRules }

func (q riftSlotFallbackQuery) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	rows := &v5SingleUserRows{at: -1}
	add := func(v any) { raw, _ := json.Marshal(v); rows.values = append(rows.values, []json.RawMessage{raw}) }
	switch {
	case sql == "SELECT version FROM game_rift_rule_active WHERE id=1":
		add(int32(11))
	case strings.Contains(sql, "FROM game_rift_rule_versions r"):
		if !strings.Contains(sql, "'EquipmentSlotFallbacks',to_jsonb(r)->'equipment_slot_fallbacks'") {
			return nil, fmt.Errorf("missing optional frozen fallback projection")
		}
		for _, r := range q.rules {
			raw, _ := json.Marshal(r)
			add(string(raw))
		}
	case strings.Contains(sql, "game_rift_entrances"):
		rows.values = [][]json.RawMessage{{json.RawMessage(`7`), json.RawMessage(`83`), json.RawMessage(`"entrance"`), json.RawMessage(`"Nbaxian"`)}}
	default:
		return nil, fmt.Errorf("unexpected query %s", sql)
	}
	return rows, nil
}

// This exercises the real loader's decoding and immutable table binding with a
// query double; it is not a PostgreSQL execution or SQL syntax validation.
func TestRiftSlotFallbackLoaderAndPersistedRun(t *testing.T) {
	var base domain.RiftRules
	var lootRule domain.LootRuleSet
	var itemRows []domain.ItemDef
	if e := testfixture.LoadRiftRules(&base); e != nil {
		t.Fatal(e)
	}
	if e := testfixture.LoadLootRules(&lootRule); e != nil {
		t.Fatal(e)
	}
	if e := testfixture.LoadEndgameItems(&itemRows); e != nil {
		t.Fatal(e)
	}
	items := map[domain.ItemID]domain.ItemDef{}
	for _, d := range itemRows {
		items[d.ID] = d
	}
	items[2703] = domain.ItemDef{ID: 2703, Equip: &domain.EquipDef{Slot: 1, LevelReq: 18, ResourceLevel: 3, NativeDropEligible: true}}
	items[2602] = domain.ItemDef{ID: 2602, Equip: &domain.EquipDef{Slot: 10, LevelReq: 36, ResourceLevel: 6, NativeDropEligible: true}}
	loot, e := domain.NewLootRuleTable(1, []domain.LootRuleSet{lootRule}, items)
	if e != nil {
		t.Fatal(e)
	}
	old := base
	old.Version = 10
	old.EquipmentPools = []domain.RiftEquipmentPool{{MonsterLevel: 43, SourceMonsters: []domain.MonsterID{4107}, Items: []domain.ItemID{2703}}}
	fresh := old
	fresh.Version = 11
	fresh.EquipmentSlotFallbacks = []domain.ItemID{2602, 2703}
	table, e := LoadRiftRules(context.Background(), riftSlotFallbackQuery{[]domain.RiftRules{base, old, fresh}}, loot, items)
	if e != nil {
		t.Fatal(e)
	}
	for _, version := range []int32{1, 10, 11} {
		r, ok := table.Version(version)
		if !ok {
			t.Fatal("missing version", version)
		}
		if version < 11 && r.EquipmentSlotFallbacks != nil {
			t.Fatal("historical NULL acquired fallback", version)
		}
		if version == 11 && !reflect.DeepEqual(r.EquipmentSlotFallbacks, []domain.ItemID{2602, 2703}) {
			t.Fatal("lost frozen IDs")
		}
		raw, _ := json.Marshal(&domain.RiftRun{RulesVersion: version, Rules: r})
		var saved domain.RiftRun
		if e = json.Unmarshal(raw, &saved); e != nil {
			t.Fatal(e)
		}
		if e = saved.BindRules(table); e != nil || !reflect.DeepEqual(saved.Rules, r) {
			t.Fatal("persisted run binding changed", version, e)
		}
	}
	active := table.Active()
	active.EquipmentSlotFallbacks[0] = 999
	if table.Active().EquipmentSlotFallbacks[0] != 2602 {
		t.Fatal("loaded rule alias")
	}
}
