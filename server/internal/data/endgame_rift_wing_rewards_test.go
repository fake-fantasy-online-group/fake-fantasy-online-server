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

type riftWingQuery struct{ riftSlotFallbackQuery }

func (q riftWingQuery) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if sql == "SELECT version FROM game_rift_rule_active WHERE id=1" {
		return &v5SingleUserRows{at: -1, values: [][]json.RawMessage{{json.RawMessage(`14`)}}}, nil
	}
	if strings.Contains(sql, "FROM game_rift_rule_versions r") && !strings.Contains(sql, "'WingRewards',to_jsonb(r)->'wing_rewards'") {
		return nil, fmt.Errorf("missing backwards-compatible nullable wing policy projection")
	}
	return q.riftSlotFallbackQuery.Query(ctx, sql, args...)
}

// Query-double coverage of the actual loader and immutable snapshots. SQL itself
// is exercised separately by the migration PostgreSQL integration suite.
func TestRiftWingLoaderFrozenPolicyAndLegacyNull(t *testing.T) {
	var base domain.RiftRules
	var l1 domain.LootRuleSet
	var itemRows []domain.ItemDef
	for _, err := range []error{testfixture.LoadRiftRules(&base), testfixture.LoadLootRules(&l1), testfixture.LoadEndgameItems(&itemRows)} {
		if err != nil {
			t.Fatal(err)
		}
	}
	items := map[domain.ItemID]domain.ItemDef{}
	for _, item := range itemRows {
		items[item.ID] = item
	}
	items[2703] = domain.ItemDef{ID: 2703, Equip: &domain.EquipDef{Slot: 1, LevelReq: 18, ResourceLevel: 3, NativeDropEligible: true}}
	items[2602] = domain.ItemDef{ID: 2602, Equip: &domain.EquipDef{Slot: 10, LevelReq: 36, ResourceLevel: 6, NativeDropEligible: true}}
	items[25029] = domain.ItemDef{ID: 25029, Stackable: true}
	l2 := l1
	l2.Version = 2
	l2.OutdoorEquipment = &domain.OutdoorEquipmentRule{AffixWeights: [6]int64{20, 25, 20, 15, 10, 5}, LegendaryBasePPB: 50000000}
	loot, err := domain.NewLootRuleTable(2, []domain.LootRuleSet{l1, l2}, items)
	if err != nil {
		t.Fatal(err)
	}
	old := base
	old.Version, old.LootVersion, old.CumulativeLegendaryPerTier, old.NormalBossEquipment = 12, 2, 10, 10
	old.Equipment, old.TimelyEquipment, old.UniqueBaseChance, old.UniquePerTier, old.UniqueCap = 0, 0, 0, 0, 0
	old.EquipmentPools = []domain.RiftEquipmentPool{{MonsterLevel: 43, SourceMonsters: []domain.MonsterID{4107}, Items: []domain.ItemID{2703}}}
	old.EquipmentSlotFallbacks = []domain.ItemID{2602, 2703}
	fresh := old
	fresh.GuardianHPDivisor = 10
	fresh.Version = 14
	fresh.WingRewards = &domain.RiftWingRewardRules{Milestones: []domain.RiftWingMilestone{{Tier: 30, Item: 25029, Count: 1200}}, RepeatBands: []domain.RiftWingRepeatBand{{MinTier: 1, Item: 25029, Count: 1}}}
	q := riftWingQuery{riftSlotFallbackQuery{[]domain.RiftRules{base, old, fresh}}}
	table, err := LoadRiftRules(context.Background(), q, loot, items)
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []int32{1, 12, 14} {
		rules, ok := table.Version(version)
		if !ok || (version < 14 && rules.WingRewards != nil) {
			t.Fatal("historical rewards changed", version)
		}
		if version == 14 && !reflect.DeepEqual(rules.WingRewards, fresh.WingRewards) {
			t.Fatal("new configuration lost")
		}
		raw, _ := json.Marshal(domain.RiftRun{RulesVersion: version, Rules: rules})
		var run domain.RiftRun
		if err = json.Unmarshal(raw, &run); err != nil {
			t.Fatal(err)
		}
		if err = run.BindRules(table); err != nil {
			t.Fatal("frozen snapshot cannot reconnect", err)
		}
	}
	table.Active().WingRewards.Milestones[0].Count = 1
	if table.Active().WingRewards.Milestones[0].Count != 1200 {
		t.Fatal("loaded policy alias")
	}
	q.rules[2].WingRewards.Milestones[0].Count = 0
	if _, err = LoadRiftRules(context.Background(), q, loot, items); err == nil {
		t.Fatal("invalid loaded policy accepted")
	}
}
