package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"math/rand"
	"testing"
)

func partySetMember(t *testing.T, s *Scene, id domain.EntityID, suitID int32) *entity.Entity {
	t.Helper()
	join(t, s, id, int64(id)*100, fmt.Sprint(id), 100+float64(id)*10, 100)
	p := s.players[id]
	races := map[int32]domain.Race{1: domain.Assassin, 2: domain.Healer, 3: domain.Warlock, 4: domain.Swordsman, 5: domain.Warrior}
	p.Player.Char.Race = races[suitID]
	p.Player.Char.Level = 60
	p.Player.Char.EmploymentKnown = true
	p.Player.Char.Employed = true
	p.Player.Party = 1
	p.Player.Char.Base = domain.Base{STR: 100, VIT: 100, INT: 100, SPI: 100, AGI: 100, DEX: 100}
	suit := &domain.EquipmentSuit{ID: suitID}
	slots := []domain.EquipSlot{domain.SlotHead, domain.SlotNeck, domain.SlotWeapon, domain.SlotGlove, domain.SlotRing, domain.SlotBody}
	if s.items == nil {
		s.items = map[domain.ItemID]domain.ItemDef{}
	}
	for i, slot := range slots {
		item := domain.ItemID(870000 + int(id)*10 + i)
		suit.Groups = append(suit.Groups, []domain.ItemID{item})
		s.items[item] = domain.ItemDef{ID: item, Equip: &domain.EquipDef{Slot: int32(slot), Durable: 100, Suit: suit}}
		p.Player.Worn.Set(slot, domain.Stack{Item: item, Count: 1, Durability: 100, MaxDurability: 100})
	}
	s.refreshStats(p)
	p.HP = 100
	return p
}
func partySetFive(t *testing.T) (*Scene, []*entity.Entity) {
	s := newCombatScene(t)
	ps := []*entity.Entity{}
	for i := int32(1); i <= 5; i++ {
		ps = append(ps, partySetMember(t, s, domain.EntityID(i), i))
	}
	return s, ps
}
func TestClassSetPartyAurasFiveAndDuplicate(t *testing.T) {
	s, ps := partySetFive(t)
	base := map[domain.EntityID]domain.Stats{}
	for _, p := range ps {
		base[p.ID] = p.Stats
	}
	s.stepEquipmentAuras()
	for _, p := range ps {
		if len(s.equipmentAuraViews(p)) != 5 {
			t.Fatalf("id%d missing grants %+v", p.ID, s.equipmentAuraViews(p))
		}
		b := base[p.ID]
		if p.MaxHP != b.MaxHP*115/100 || p.Stats.CritRate != b.CritRate+500 || p.Stats.MCritRate != b.MCritRate+500 || p.HP != 100 {
			t.Fatalf("stats %+v base%+v hp%d", p.Stats, b, p.HP)
		}
		for _, v := range s.equipmentAuraStatuses(p) {
			if v.effect.PositionMode != 0 {
				t.Fatal("class aura must use base position")
			}
		}
	}
	six := partySetMember(t, s, 6, 5)
	s.tick += domain.Ticks(500)
	s.stepEquipmentAuras()
	if len(s.equipmentAuraViews(six)) != 5 {
		t.Fatal("sixth player wrongly excluded")
	}
	if ps[0].MaxHP != base[1].MaxHP*115/100 {
		t.Fatal("duplicate warrior stacked")
	}
	s.refreshStats(ps[0])
	if ps[0].HP != 100 {
		t.Fatal("refresh healed")
	}
}
func TestClassSetPartyAuraLifecycleAndResourceNeutral(t *testing.T) {
	for _, mode := range []string{"range", "party", "map", "death", "unequip", "broken"} {
		t.Run(mode, func(t *testing.T) {
			s := newCombatScene(t)
			p := partySetMember(t, s, 1, 5)
			q := partySetMember(t, s, 2, 1)
			s.stepEquipmentAuras()
			base := int32(900)
			if q.MaxHP != 1035 {
				t.Fatalf("max %d", q.MaxHP)
			}
			switch mode {
			case "range":
				s.aoi.Move(q, domain.Pos{MapID: 7, X: 900, Y: 100})
			case "party":
				q.Player.Party = 2
			case "map":
				q.Pos.MapID = 8
			case "death":
				p.HP = 0
			case "unequip":
				p.Player.Worn.Set(domain.SlotBody, domain.Stack{})
			case "broken":
				st := p.Player.Worn.At(domain.SlotBody)
				st.Durability = 0
				p.Player.Worn.Set(domain.SlotBody, st)
			}
			if s.classSetAuraValue(q, "class_set_max_hp_bp") != 0 {
				t.Fatal("invalid source retained live bonus before rescan")
			}
			s.stepEquipmentAuras()
			if q.MaxHP != base || q.HP != 100 {
				t.Fatalf("withdraw max%d hp%d", q.MaxHP, q.HP)
			}
		})
	}
	s := newCombatScene(t)
	p := partySetMember(t, s, 1, 5)
	q := partySetMember(t, s, 2, 1)
	s.stepEquipmentAuras()
	if p.MaxHP != 2070 {
		t.Fatalf("dragon x aura got%d", p.MaxHP)
	}
	for i := 0; i < 10; i++ {
		s.aoi.Move(q, domain.Pos{MapID: 7, X: 900, Y: 100})
		s.stepEquipmentAuras()
		s.aoi.Move(q, domain.Pos{MapID: 7, X: 120, Y: 100})
		s.tick += domain.Ticks(500)
		s.stepEquipmentAuras()
		if q.MaxHP != 1035 || q.HP != 100 {
			t.Fatalf("cycle%d max%d hp%d", i, q.MaxHP, q.HP)
		}
	}
}
func TestClassSetPartyAuraCriticalAndDamageTypes(t *testing.T) {
	s, ps := partySetFive(t)
	s.stepEquipmentAuras()
	p := ps[2]
	dst := putMonster(s, 900, 100000, domain.Stats{}, 200, 100)
	p.Stats.MinAtk, p.Stats.MaxAtk, p.Stats.MAtk = 100, 100, 100
	p.Stats.CritRate, p.Stats.MCritRate = 10000, 10000
	for _, magic := range []bool{false, true} {
		b := s.equipmentBlow(p, combat.Blow{Magic: magic, GuaranteedHit: true})
		if b.CriticalBonusBP != 2000 {
			t.Fatal("wrong critical units")
		}
		ev := combat.CalculateStrike(p, dst, &combat.HitTracker{}, b, rand.New(rand.NewSource(1)))
		if !ev.Flag.Has(event.DamageCrit) || ev.Amount != 220 {
			t.Fatalf("crit magic%v amount%d", magic, ev.Amount)
		}
	}
	for _, magic := range []bool{false, true} {
		if got := s.classSetAuraIncomingDamage(p, magic, 1000); got != 920 {
			t.Fatalf("mitigation magic%v=%d", magic, got)
		}
	}
	// No elemental key is involved: neutral magic receives the same eight percent.
	attacker := putMonster(s, 901, 100000, domain.Stats{MinAtk: 1000, MaxAtk: 1000, MAtk: 1000}, 210, 100)
	p.Stats.MDef = 0
	p.Stats.MagicResist = 0
	p.HP, p.MaxHP = 100000, 100000
	ev, _, _ := s.resolveStrikeStatuses(attacker, p, &combat.HitTracker{}, combat.Blow{Magic: true, HurtType: 0})
	if ev.Amount != 920 {
		t.Fatalf("neutral magic %d", ev.Amount)
	}
}

func TestClassSetPartyAuraDuplicateWithdrawalAndExpiry(t *testing.T) {
	s := newCombatScene(t)
	a := partySetMember(t, s, 1, 5)
	b := partySetMember(t, s, 2, 5)
	q := partySetMember(t, s, 3, 1)
	s.stepEquipmentAuras()
	a.HP = 0
	s.stepEquipmentAuras()
	if q.MaxHP != 1035 || q.HP != 100 {
		t.Fatal("second source did not take over without healing")
	}
	views := s.equipmentAuraViews(q)
	found := false
	for _, v := range views {
		if v.AuraID == "class_set_5" {
			found = true
			if v.SourceEntityID != b.ID {
				t.Fatal("dead source retained")
			}
		}
	}
	if !found {
		t.Fatal("missing replacement")
	}
	b.Player.Worn.Set(domain.SlotBody, domain.Stack{})
	s.stepEquipmentAuras()
	if q.MaxHP != 900 || q.HP != 100 {
		t.Fatal("last source withdrawal failed")
	}
}
func TestClassSetPartyAuraMitigationBeyondLegacyCap(t *testing.T) {
	s, ps := partySetFive(t)
	s.stepEquipmentAuras()
	p := ps[2]
	// The legacy bucket may already reach its cap; the approved 8% still applies.
	b := domain.CombatDamageBuckets{MitigationBP: 10000}
	legacy := b.IncomingDamage(10000)
	if legacy != 5500 {
		t.Fatalf("legacy cap changed %d", legacy)
	}
	if got := s.classSetAuraIncomingDamage(p, false, legacy); got != 5060 {
		t.Fatalf("new mitigation swallowed by cap: %d", got)
	}
	// Magic ward does not reduce the physical/resource-damage route when alone.
	s2 := newCombatScene(t)
	healer := partySetMember(t, s2, 1, 2)
	s2.stepEquipmentAuras()
	if got := s2.classSetAuraIncomingDamage(healer, false, 1000); got != 1000 {
		t.Fatalf("magic aura reduced nonmagic %d", got)
	}
	if got := s2.mitigateSecondaryStatusDamage(healer, true, 1000); got != 920 {
		t.Fatalf("magic reflection %d", got)
	}
}
