package scene

import (
	"math/rand"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func socketlessDropDef() domain.ItemDef {
	return domain.ItemDef{ID: 999002, InstanceKind: domain.ItemInstanceEquipment, Equip: &domain.EquipDef{NativeDropEligible: true, Slot: int32(domain.SlotWeapon), LevelReq: 50, Affixes: []domain.Affix{{Attr: domain.AttrAtk, Value: 10}}}}
}

func TestNaturalWorldAndFixedDropsHaveNoSockets(t *testing.T) {
	for _, fixed := range []bool{false, true} {
		for _, rollAffixes := range []bool{false, true} {
			s, mid := lootScene(t, []domain.DropEntry{{Item: 999002, RatePct: 100, RollAffixes: rollAffixes}})
			def := socketlessDropDef()
			s.items[def.ID] = def
			s.equipmentRoll = magicFindRollTableV5()
			s.equipmentRoll.Counts[domain.DefaultColorProfile] = []domain.WeightedAffixCount{{Count: 5, Weight: 100}}
			if fixed {
				s.entities[mid].Monster.Kind = domain.MonsterBoss
				s.bossFixedDrops = domain.BossFixedDropTable{{MapID: s.id.MapID, Monster: s.entities[mid].Monster.TypeID}: {Minimum: 1, Pool: []domain.BossFixedDropChoice{{Item: def.ID, Weight: 1, RollAffixes: rollAffixes}}}}
			}
			for i := 0; i < 30; i++ {
				s.tick = domain.Tick(i)
				s.rollLoot(s.entities[mid], 0)
			}
			seen := 0
			for _, drop := range s.ground {
				if drop.Drop.Item != def.ID {
					continue
				}
				seen++
				st := drop.Drop.Stack
				want := uint8(0)
				if rollAffixes {
					want = 4
				}
				if st.SocketCount != 0 || st.RolledAffixCount != want {
					t.Fatalf("fixed=%v roll=%v: holes=%d affixes=%d", fixed, rollAffixes, st.SocketCount, st.RolledAffixCount)
				}
			}
			if seen != 30 {
				t.Fatalf("fixed=%v roll=%v: got %d equipment", fixed, rollAffixes, seen)
			}
		}
	}
}

func TestNaturalTheftDropsHaveNoSockets(t *testing.T) {
	for _, rollAffixes := range []bool{false, true} {
		for seed := int64(0); seed < 30; seed++ {
			s, src, dst, _ := theftTestScene(t, 10000)
			def := socketlessDropDef()
			s.items[def.ID] = def
			s.lootTable[dst.Monster.TypeID] = []domain.DropEntry{{Item: def.ID, RatePct: 100, RollAffixes: rollAffixes}}
			s.equipmentRoll = magicFindRollTableV5()
			s.equipmentRoll.Counts[domain.DefaultColorProfile] = []domain.WeightedAffixCount{{Count: 5, Weight: 100}}
			s.rng = rand.New(rand.NewSource(seed))
			castTheft(t, s, src, dst)
			st := onlyTheftDrop(t, s).Drop.Stack
			want := uint8(0)
			if rollAffixes {
				want = 4
			}
			if st.SocketCount != 0 || st.RolledAffixCount != want {
				t.Fatalf("roll=%v seed=%d: holes=%d affixes=%d", rollAffixes, seed, st.SocketCount, st.RolledAffixCount)
			}
		}
	}
}

func TestNaturalRiftSpecialAndNormalBossRewardsHaveNoSockets(t *testing.T) {
	s, b, now := riftFixture(t)
	activateTestRift(t, s, b, *now)
	s.riftRules = rebalanceRiftRulesFixture(t)
	s.rift.data.Version = 2
	s.rift.data.Run.Rules = s.riftRules.Active()
	s.rift.data.Run.RulesVersion = 4
	s.rift.data.Run.Members[100].Eligible = true
	// One non-unique eligible base makes both special and normal rewards observable.
	for id, def := range s.items {
		if def.Equip != nil {
			delete(s.items, id)
		}
	}
	def := socketlessDropDef()
	s.items[def.ID] = def
	s.equipmentRoll = magicFindRollTableV5()
	s.equipmentRoll.Counts[domain.DefaultColorProfile] = []domain.WeightedAffixCount{{Count: 5, Weight: 100}}
	s.generateEndgameRiftGrants()
	seen := 0
	for _, st := range s.rift.data.Grants[100].Items {
		if st.Item != def.ID {
			continue
		}
		seen++
		if st.SocketCount != 0 || st.RolledAffixCount != 4 {
			t.Fatalf("rift holes=%d affixes=%d", st.SocketCount, st.RolledAffixCount)
		}
	}
	plan := domain.PlanRiftRewards(s.rift.data.Run.Tier, s.rift.data.Run.Timely, s.rift.data.Run.Rules)
	want := int(plan.Equipment + plan.NormalBossEquipment)
	if want < 11 || seen != want {
		t.Fatalf("special and ten normal slots: got %d want %d", seen, want)
	}
}

func TestGeneratedLootPreservesAuthoredSocketInstance(t *testing.T) {
	s, mid := lootScene(t, nil)
	def := socketlessDropDef()
	s.items[def.ID] = def
	st := domain.NewStack(def, 1)
	st.SocketCount = 2
	st.Sockets[0] = 12345
	st.SocketUIDs[0] = 777
	s.spawnGeneratedLoot(def, st, s.entities[mid].Pos, 0)
	if got := onlyTheftDrop(t, s).Drop.Stack; got != st {
		t.Fatal("publication rewrote an existing socket instance")
	}
}
