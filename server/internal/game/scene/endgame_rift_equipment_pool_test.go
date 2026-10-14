package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"reflect"
	"testing"
)

func TestRiftExplicitPoolLimitsEveryEquipmentSlot(t *testing.T) {
	s, b, now := riftFixture(t)
	activateTestRift(t, s, b, *now)
	const allowed domain.ItemID = 1610
	add := func(id domain.ItemID, level int32, ascended bool) {
		s.items[id] = domain.ItemDef{ID: id, InstanceKind: domain.ItemInstanceEquipment, Equip: &domain.EquipDef{NativeDropEligible: true, LevelReq: level, AscensionOnly: ascended}}
	}
	add(allowed, 54, false)
	add(15036, 60, true)
	add(15037, 66, false)
	add(1609, 48, false)
	r := s.rift.data.Run
	r.Rules.NormalBossEquipment = 10
	r.Rules.EquipmentPools = []domain.RiftEquipmentPool{{MonsterLevel: 55, SourceMonsters: []domain.MonsterID{101}, Items: []domain.ItemID{allowed, 15036, 15037}}}
	r.Timely = true
	r.Tier = 1
	for char, m := range r.Members {
		m.Level = 100
		m.Eligible = true
		r.Members[char] = m
	}
	s.generateEndgameRiftGrants()
	for _, g := range s.rift.data.Grants {
		n := 0
		for _, st := range g.Items {
			if st.InstanceKind == domain.ItemInstanceEquipment {
				n++
				if st.Item != allowed {
					t.Fatalf("equipment escaped explicit preascension pool: %+v", st)
				}
			}
		}
		want := domain.PlanRiftRewards(r.Tier, r.Timely, r.Rules)
		if n != int(want.Equipment+want.NormalBossEquipment) {
			t.Fatalf("got %d equipment want %d", n, want.Equipment+want.NormalBossEquipment)
		}
	}
	before := s.rift.data.clone().Grants
	s.generateEndgameRiftGrants()
	if !reflect.DeepEqual(before, s.rift.data.Grants) {
		t.Fatal("ready grants rerolled")
	}
}

func TestRiftNamedNativeEquipmentPreservesFixedProperties(t *testing.T) {
	s, b, now := riftFixture(t)
	activateTestRift(t, s, b, *now)
	const fixed domain.ItemID = 1022 // original 利器·飞鸟斩 named elite drop
	def := domain.ItemDef{ID: fixed, InstanceKind: domain.ItemInstanceEquipment, Equip: &domain.EquipDef{NoTypeDrop: true, LevelReq: 20, ResourceLevel: 4, Type: 1, Slot: 4, Affixes: []domain.Affix{{Attr: domain.AttrMaxHP, Value: 12}}}}
	s.items[fixed] = def
	s.equipmentRoll = domain.EquipmentRollTable{Counts: map[domain.ColorProfile][]domain.WeightedAffixCount{domain.DefaultColorProfile: {{Count: 5, Weight: 1}}}, Options: map[domain.EquipmentRollKey][]domain.EquipmentRollOption{{Level: 4, Type: 1}: {{CardID: 1, Affix: domain.Affix{Attr: domain.AttrMaxHP, Value: 100}, Weight: 1}}}}
	r := s.rift.data.Run
	r.Rules.NormalBossEquipment = 10
	r.Rules.EquipmentPools = []domain.RiftEquipmentPool{{MonsterLevel: 24, SourceMonsters: []domain.MonsterID{3049}, Items: []domain.ItemID{fixed}}}
	r.Tier = 10
	r.Timely = true
	for char, m := range r.Members {
		m.Level = 60
		m.Eligible = true
		r.Members[char] = m
	}
	s.generateEndgameRiftGrants()
	for _, g := range s.rift.data.Grants {
		n := 0
		for _, st := range g.Items {
			if st.InstanceKind != domain.ItemInstanceEquipment {
				continue
			}
			n++
			if st.Item != fixed || st.SocketCount != 0 || st.RolledAffixCount != 0 || st.Endgame.UniqueID != 0 {
				t.Fatal("named fixed equipment randomized/promoted", st)
			}
		}
		plan := domain.PlanRiftRewards(r.Tier, r.Timely, r.Rules)
		if n != int(plan.Equipment+plan.NormalBossEquipment) {
			t.Fatal("named native drop lost reward slots", n)
		}
	}
	if !reflect.DeepEqual(s.items[fixed].Equip.Affixes, def.Equip.Affixes) {
		t.Fatal("native fixed attributes changed")
	}
}
