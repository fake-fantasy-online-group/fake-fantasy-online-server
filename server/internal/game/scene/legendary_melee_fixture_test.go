package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"testing"
)

func legendaryMeleeFixture(t *testing.T, id string) (*Scene, *entity.Entity, *entity.Entity) {
	t.Helper()
	s := newCombatScene(t)
	join(t, s, 1, 100, "melee", 100, 100)
	p := s.players[1]
	prof := uint8(1)
	if id[:2] == "JK" {
		prof = 2
	}
	p.Player.Char.Race = domain.Race(prof - 1)
	p.Player.Char.Level = 100
	p.Player.Char.Employed = true
	p.Player.Char.EmploymentKnown = true
	p.Player.Char.Skills = domain.Learned{}
	for i := domain.SkillID(10001); i < 10131; i++ {
		p.Player.Char.Skills[i] = 1
	}
	p.Stats = domain.Stats{MinAtk: 100, MaxAtk: 100, Hit: 100000}
	p.HP = 10000
	p.MaxHP = 10000
	if s.items == nil {
		s.items = map[domain.ItemID]domain.ItemDef{}
	}
	slot := domain.SlotWeapon
	if id[3] == 'D' {
		slot = domain.SlotBody
	}
	s.items[900001] = domain.ItemDef{ID: 900001, Equip: &domain.EquipDef{Slot: int32(slot), LevelReq: 100, Durable: 100, Need: domain.Requirement{Professions: 1 << uint(prof-1)}}}
	p.Player.Worn.Set(slot, domain.Stack{UID: 991, Item: 900001, Count: 1, Durability: 100, MaxDurability: 100, Legendary: domain.LegendaryInstance{PowerID: id}})
	m := putMonster(s, 900, 100000, domain.Stats{}, 130, 100)
	if len(domain.ActiveLegendaryPowers(p.Player.Worn, s.itemDef, p.Player.Char)) != 1 {
		t.Fatal("test equipment inactive")
	}
	return s, p, m
}

func legendaryMeleeStrike(s *Scene, p, m *entity.Entity, skill domain.SkillID) event.DamageDealt {
	end := s.beginEquipmentRoot(p.ID, m.ID, domain.EquipmentCombatStats{})
	defer end()
	ev, _, _ := s.resolveStrikeStatuses(p, m, &combat.HitTracker{}, combat.Blow{SkillID: int32(skill), GuaranteedHit: true, SkillPct: 100})
	return ev
}
