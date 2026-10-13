package scene

import (
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
)

func TestEquipmentAuraConfiguredCadencesAndSourceCleanup(t *testing.T) {
	s := newCombatScene(t)
	join(t, s, 1, 100, "fast", 100, 100)
	join(t, s, 2, 200, "slow", 110, 100)
	join(t, s, 3, 300, "target", 120, 100)
	p, q, r := s.players[1], s.players[2], s.players[3]
	p.Player.Party, q.Player.Party, r.Player.Party = 1, 1, 1
	cacheTestAura(s, p, 991, 1200)
	cacheTestAura(s, q, 992, 800)
	fast := s.equipmentState().cache[p.ID].effects[0].Effect.Aura
	fast.ID, fast.Family = "configured_fast", "configured_fast"
	fast.RefreshMS, fast.LeaseMS, fast.RadiusPX, fast.PVPScaleBP = 200, 600, 180, 2500
	slow := s.equipmentState().cache[q.ID].effects[0].Effect.Aura
	slow.ID, slow.Family = "configured_slow", "configured_slow"
	slow.RefreshMS, slow.LeaseMS, slow.RadiusPX = 1000, 2000, 300
	slow.Modifier, slow.Element = "element_resist_bp", "fire"
	s.stepEquipmentAuras()
	rt := s.equipmentState()
	if len(rt.auraSchedules) != 2 || len(rt.auras[r.ID]) != 2 {
		t.Fatal("distinct sources not admitted")
	}
	fastExpiry, slowExpiry := rt.auras[r.ID][fast.Family].expires, rt.auras[r.ID][slow.Family].expires
	s.tick += domain.Ticks(200)
	s.stepEquipmentAuras()
	if rt.auras[r.ID][fast.Family].expires <= fastExpiry || rt.auras[r.ID][slow.Family].expires != slowExpiry {
		t.Fatal("short cadence refreshed another source's lease")
	}
	if got := s.equipmentDefense(r, true).PhysicalMitigationBP; got != 300 {
		t.Fatal("configured PvP coefficient ignored", got)
	}
	s.tick += domain.Ticks(800)
	s.stepEquipmentAuras()
	if rt.auras[r.ID][slow.Family].expires <= slowExpiry {
		t.Fatal("long configured cadence ignored")
	}
	// Source replacement must discard its prior timer instead of accumulating it.
	c := rt.cache[p.ID]
	c.effects[0].SourceUID = 993
	rt.cache[p.ID] = c
	s.stepEquipmentAuras()
	if len(rt.auraSchedules) != 2 || rt.auras[r.ID][fast.Family].uid != 993 {
		t.Fatal("source replacement retained old clock/grant")
	}
	for source := range rt.auraSchedules {
		if source.uid == 991 {
			t.Fatal("old source timer leaked")
		}
	}
	r.Player.Party = 0
	s.stepEquipmentAuras()
	if len(rt.auras[r.ID]) != 0 {
		t.Fatal("departed party member retained configured aura")
	}
	p.HP = 0
	s.stepEquipmentAuras()
	if len(rt.auraSchedules) != 1 {
		t.Fatal("dead source retained timer")
	}
	delete(s.players, q.ID)
	delete(s.entities, q.ID)
	s.stepEquipmentAuras()
	if len(rt.auraSchedules) != 0 || len(rt.auras) != 0 {
		t.Fatal("disconnected source retained timer/grants")
	}
}

func TestOffensiveAuraUsesItsConfiguredPVPScale(t *testing.T) {
	s := newCombatScene(t)
	join(t, s, 1, 100, "source", 100, 100)
	join(t, s, 2, 200, "target", 110, 100)
	p, q := s.players[1], s.players[2]
	p.Stats = domain.Stats{MAtk: 1000}
	q.Stats = domain.Stats{}
	q.HP, q.MaxHP = 10000, 10000
	cacheTestAura(s, p, 991, 2000)
	aura := s.equipmentState().cache[p.ID].effects[0].Effect.Aura
	aura.Modifier, aura.Element, aura.PVPScaleBP = "element_damage_bp", "fire", 2500
	s.stepEquipmentAuras()
	ev, _, _ := s.resolveStrikeStatuses(p, q, &combat.HitTracker{}, combat.Blow{Magic: true, HurtType: domain.HurtFire})
	if ev.Amount != 1050 {
		t.Fatal("offensive aura fell back to blanket 50% PvP scale", ev.Amount)
	}
	monster := putMonster(s, 900, 10000, domain.Stats{}, 120, 100)
	ev, _, _ = s.resolveStrikeStatuses(p, monster, &combat.HitTracker{}, combat.Blow{Magic: true, HurtType: domain.HurtFire})
	if ev.Amount != 1200 {
		t.Fatal("PvP scale leaked into PvE aura", ev.Amount)
	}
}
