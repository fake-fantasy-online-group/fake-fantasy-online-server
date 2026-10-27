package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"testing"
	"time"
)

func TestPowerBookSameUIDRollRefreshesPermanentSource(t *testing.T) {
	s, p, _ := nativeRepairFixture(t, domain.Healer, domain.Learned{10322: 2, 10320: 1}, "YS-U02")
	initial := p.Player.Worn.At(domain.SlotWeapon)
	for _, roll := range []int32{7500, 8750, 10000, 0, 7500} {
		st := initial
		st.Legendary.PowerRollBP = roll
		p.Player.Worn.Set(domain.SlotWeapon, st)
		s.invalidateCombatAttributes(p)
		s.syncLegendaryPermanentBuffs(p)
		count := 0
		p.Status.Each(func(a domain.Active) {
			if a.Def.LegendaryPermanentPower != "YS-U02" {
				return
			}
			count++
			if a.Def.SkillSnapshotRank != 2 || a.Def.LegendarySourceUID != initial.UID || a.ExpireAt != 0 {
				t.Fatal("permanent rank/identity changed", a.Def.SkillSnapshotRank, a.Def.LegendarySourceUID, a.ExpireAt)
			}
			found := false
			for _, source := range a.Def.LegendarySources {
				if source.PowerID == "YS-U02" && source.SourceUID == initial.UID {
					found = true
					if source.PowerRollBP != roll {
						t.Fatal("same UID kept stale power roll", source.PowerRollBP, roll)
					}
				}
			}
			if !found {
				t.Fatal("permanent source provenance disappeared")
			}
		})
		if count != 1 {
			t.Fatal("permanent source duplicated or missing", count)
		}
		def, _ := s.skills.Get(10320, 1)
		s.legendaryPreviewSkill(p, &def)
		factor := roll
		if factor == 0 {
			factor = 10000
		}
		if def.LegendaryModifiers.StatusPowerBP != 5000*factor/10000 {
			t.Fatal("actual buff modifier did not use current roll", roll, def.LegendaryModifiers.StatusPowerBP)
		}
		if saved := p.Status.SaveForLogout(s.tick, time.Now(), p.ID); len(saved) != 0 {
			t.Fatal("equipment permanent roll persisted as a free status")
		}
	}
}
func TestPowerBookAcceptedRollRemainsFrozenAcrossSameUIDUpgrade(t *testing.T) {
	s, p, m := nativeRepairFixture(t, domain.Warlock, domain.Learned{10402: 1}, "SS-A01")
	st := p.Player.Worn.At(domain.SlotWeapon)
	st.Legendary.PowerRollBP = 7500
	p.Player.Worn.Set(domain.SlotWeapon, st)
	s.invalidateCombatAttributes(p)
	def, _ := s.skills.Get(10402, 1)
	s.legendaryPreviewSkill(p, &def)
	accepted := append([]domain.LegendarySourceRef(nil), def.LegendarySources...)
	if len(accepted) != 1 || accepted[0].PowerRollBP != 7500 {
		t.Fatal("cast failed to snapshot roll", accepted)
	}
	offense := s.captureCombatSourceV5(p, false)
	st.Legendary.PowerRollBP = 10000
	p.Player.Worn.Set(domain.SlotWeapon, st)
	s.invalidateCombatAttributes(p)
	current, _ := s.skills.Get(10402, 1)
	s.legendaryPreviewSkill(p, &current)
	if current.LegendaryModifiers.DamageBP != 7000 {
		t.Fatal("new cast missed improved roll", current.LegendaryModifiers.DamageBP)
	}
	prior, _ := s.skills.Get(10402, 1)
	s.legendaryComposeSkill(p, &prior, accepted, true, false, m.ID)
	if prior.LegendaryModifiers.DamageBP != 5250 {
		t.Fatal("accepted cast retroactively gained new roll", prior.LegendaryModifiers.DamageBP)
	}
	contexts := s.legendaryContexts(p, m, combat.Blow{SkillID: 10402, SourceOffense: offense}, event.DamageDealt{Src: p.ID, Dst: m.ID})
	if len(contexts) != 1 || contexts[0].PowerRollBP != 7500 {
		t.Fatal("projectile source lost accepted roll", contexts)
	}
	// Pre-range persisted casts have zero, which retains the original full
	// strength rather than borrowing the current equipment's lower value.
	accepted[0].PowerRollBP = 0
	st.Legendary.PowerRollBP = 7500
	p.Player.Worn.Set(domain.SlotWeapon, st)
	s.invalidateCombatAttributes(p)
	legacy, _ := s.skills.Get(10402, 1)
	s.legendaryComposeSkill(p, &legacy, accepted, true, false, m.ID)
	if legacy.LegendaryModifiers.DamageBP != 7000 {
		t.Fatal("legacy snapshot was weakened", legacy.LegendaryModifiers.DamageBP)
	}
}
