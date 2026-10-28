package store

import (
	"context"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"strings"
	"testing"
)

func legacyWingScrollFixture(t *testing.T) (domain.Stack, domain.Stack) {
	t.Helper()
	items := map[domain.ItemID]domain.ItemDef{}
	cfg := domain.WingProgressionConfig{Version: 1, Quest: domain.WingGuideQuestID, Starter: 20609, Entrance: domain.EndgameRiftEntranceView{MapID: 7, NPCID: 96, NPC: "幻翼域使者", Script: "N2619LC"}}
	for i, id := range []domain.ItemID{20609, 20611} {
		items[id] = domain.ItemDef{ID: id, InventoryTab: 2, InventoryTabKnown: true, InstanceKind: domain.ItemInstanceEquipment, Equip: &domain.EquipDef{Slot: int32(domain.SlotBag)}}
		cfg.Items = append(cfg.Items, domain.WingItemRule{Item: id, Tier: int32(i + 1), Series: "light"})
	}
	rules, err := domain.NewWingProgressionRules(cfg, items)
	if err != nil {
		t.Fatal(err)
	}
	domain.RegisterLegendaryPowerExecutable("TY-A02")
	legacy := domain.Stack{Item: 20609, UID: 42, Count: 1, Bound: true, RefineLevel: 3, InstanceKind: domain.ItemInstanceNone}
	ch := &domain.Character{Race: domain.Warrior, EmploymentKnown: true, Employed: true}
	got, err := (domain.LegendaryPowerBook{"TY-A02": 8123}).ImprintWingPower(legacy, items[legacy.Item], ch, "TY-A02", rules)
	if err != nil {
		t.Fatal(err)
	}
	return legacy, got
}

func TestWingScrollLegacyLocationKindPassesPersistenceValidator(t *testing.T) {
	legacy, got := legacyWingScrollFixture(t)
	if got.InstanceKind != domain.ItemInstanceEquipment || got.WingPowers[0].PowerID != "TY-A02" || got.WingPowers[0].PowerRollBP != 8123 {
		t.Fatal("legacy wing not normalized", got)
	}
	if err := validateWingPowerItem(got); err != nil {
		t.Fatal("valid scroll result cannot persist", err)
	}
	broken := got
	broken.InstanceKind = domain.ItemInstanceNone
	if validateWingPowerItem(broken) == nil {
		t.Fatal("fixture no longer exercises actual store kind gate")
	}
	restored := got
	restored.InstanceKind = legacy.InstanceKind
	restored.WingPowers = legacy.WingPowers
	if restored != legacy {
		t.Fatal("normalization changed unrelated instance state")
	}
}

func TestWingScrollLegacySQLShapeRoundTrip(t *testing.T) {
	_, got := legacyWingScrollFixture(t)
	q := &powerScrollQuerier{}
	if err := saveWingPowerItem(context.Background(), q, got); err != nil {
		t.Fatal(err)
	}
	if len(q.args) != 2 || !strings.Contains(q.statement, "equipment_wing_power_state") {
		t.Fatal("wrong SQL write", q.args)
	}
	q.rows = &powerScrollRows{values: []any{got.UID, []byte(q.args[1].(string))}}
	cached := got
	cached.WingPowers = domain.WingPowers{}
	items := map[int64]domain.Stack{got.UID: cached}
	if err := loadWingPowerItems(context.Background(), q, items); err != nil {
		t.Fatal(err)
	}
	if items[got.UID] != got {
		t.Fatal("SQL reload lost normalized kind or power roll")
	}
}

func TestWingScrollLegacyAtomicFaultsAndReplay(t *testing.T) {
	for _, stage := range []string{"state", "snapshot", "receipt", "lost_ack"} {
		t.Run(stage, func(t *testing.T) {
			legacy, after := legacyWingScrollFixture(t)
			m, s := endgameFixture(t)
			scroll := domain.Stack{UID: 92, Item: domain.PowerScrollItemID, Count: 1, Bound: true, InstanceKind: domain.ItemInstanceEquipment, Endgame: domain.EndgameItemState{PowerScrollID: "TY-A02", PowerScrollRollBP: 8123}}
			s.Bag.Set(0, legacy)
			s.Bag.Set(1, scroll)
			if err := m.SaveSnapshot(context.Background(), s); err != nil {
				t.Fatal(err)
			}
			plan := endgamePlan(s)
			plan.Snapshot.Char.Caiyu = s.Char.Caiyu
			plan.Receipt.Operation = "powerbook.apply"
			plan.Run = nil
			plan.Snapshot.Bag.Set(0, after)
			plan.Snapshot.Bag.Set(1, domain.Stack{})
			f := &endgameFaultStore{Memory: m, stage: stage, lostAck: stage == "lost_ack"}
			backend := NewEndgameBackend(f, nil)
			result := awaitEndgame(t, backend.Commit(plan))
			durable := durableSnapshot(t, m, s)
			if stage != "lost_ack" {
				if result.Err == nil || durable.Bag.At(0) != legacy || durable.Bag.At(1) != scroll {
					t.Fatal("failed imprint lost wing or scroll", result)
				}
			} else {
				if result.Err != nil || !result.Replayed || durable.Bag.At(0) != after || !durable.Bag.At(1).Empty() {
					t.Fatal("lost ACK roundtrip lost normalized wing", result)
				}
				again := awaitEndgame(t, backend.Commit(plan))
				if again.Err != nil || !again.Replayed || durableSnapshot(t, m, s).Bag.At(0) != after {
					t.Fatal("replay changed wing or consumed twice", again)
				}
			}
		})
	}
}
