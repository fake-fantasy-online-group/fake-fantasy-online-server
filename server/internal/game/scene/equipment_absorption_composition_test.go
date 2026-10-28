package scene

import (
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func absorptionCompositionFixture(t *testing.T, boosted bool) (*Scene, *entity.Entity, *entity.Entity) {
	powers := []string{}
	if boosted {
		powers = append(powers, "TY-D12")
	}
	s, p, m := nativeRepairFixture(t, domain.Healer, domain.Learned{}, powers...)
	p.HP, p.MaxHP = 500, 10000
	p.Stats = domain.Stats{MAtk: 100, Hit: 100000}
	m.Stats = domain.Stats{MAtk: 100, Hit: 100000}
	var equipment domain.EquipmentCombatStats
	equipment.Elements[domain.HurtElementIndex(domain.HurtFire)].AbsorbBP = 2000
	cacheElementV5(s, p, equipment)
	return s, p, m
}

func TestElementAbsorptionActualBurnTickAndReceivedPower(t *testing.T) {
	for _, boosted := range []bool{false, true} {
		for _, element := range []int32{domain.HurtFire, domain.HurtIce} {
			s, p, m := absorptionCompositionFixture(t, boosted)
			skill := domain.SkillID(10421)
			var equipment domain.EquipmentCombatStats
			equipment.Elements[domain.HurtElementIndex(element)].AbsorbBP = 2000
			cacheElementV5(s, p, equipment)
			active := domain.Active{Source: m.ID, Def: domain.StatusDef{ID: 1036, SourceSkillID: int32(skill), SkillSnapshotID: skill, SkillSnapshotRank: 1, DurationSec: 10, IntervalSec: 1, HPChanges: []domain.ResourceChange{{Value: -100, Mode: domain.ModeAbsolute}}}}
			s.tickStatus(p, active)
			want := int32(400)
			if element == domain.HurtFire {
				heal := int32(20)
				if boosted {
					heal = 28
				}
				want = 500 - 80 + heal
			}
			if p.HP != want {
				t.Fatalf("real periodic element%d boosted%v hp%d want%d", element, boosted, p.HP, want)
			}
			count := 0
			for _, out := range s.outbox {
				if h, ok := out.ev.(event.HealDone); ok && h.Dst == p.ID {
					count++
				}
			}
			if element == domain.HurtFire && count != 1 || element == domain.HurtIce && count != 0 {
				t.Fatal("periodic absorption restore emitted wrong count", count)
			}
		}
	}
}

func TestElementAbsorptionActualReflectionRetainsIncomingElement(t *testing.T) {
	for _, element := range []int32{domain.HurtFire, domain.HurtIce} {
		s, p, m := absorptionCompositionFixture(t, true)
		m.Status.Apply(domain.StatusDef{ID: 199991, DurationSec: 30, MagicReflectPct: 100}, s.tick, m.ID)
		ev, raw, reflected := s.resolveStrikeStatuses(p, m, &combat.HitTracker{}, combat.Blow{Magic: true, HurtType: element, SkillPct: 100, GuaranteedHit: true})
		s.emit(ev)
		s.afterStrikeStatuses(p, m, ev, raw, reflected, strikeReactionContext{HurtType: element})
		want := int32(400)
		if element == domain.HurtFire {
			want = 448
		}
		if p.HP != want {
			t.Fatalf("actual reflection element%d hp%d want%d", element, p.HP, want)
		}
		count := 0
		for _, out := range s.outbox {
			if e, ok := out.ev.(event.DamageDealt); ok && e.Origin == domain.DamageReflectionOrigin {
				count++
				if e.Src != m.ID || e.Dst != p.ID {
					t.Fatal("reflection identity changed")
				}
			}
		}
		if count != 1 {
			t.Fatal("reflection repeated", count)
		}
	}
}

func TestElementAbsorptionActualRiftHazard(t *testing.T) {
	s, m, p, _ := riftAbilityFixture(t, riftEruption)
	p.Player.Char.Level = 100
	incomingEquipPower(t, s, p, "TY-D12", domain.SlotBody)
	p.HP, p.MaxHP = 500, 10000
	p.Stats = domain.Stats{}
	var equipment domain.EquipmentCombatStats
	equipment.Elements[domain.HurtElementIndex(domain.HurtFire)].AbsorbBP = 2000
	cacheElementV5(s, p, equipment)
	h := &endgameRiftHazard{Source: m.ID, SourceLevel: m.Level, Attack: domain.Stats{MAtk: 100}, Pos: p.Pos}
	s.hitRiftHazard(h, p, 100)
	if p.HP != 448 {
		t.Fatalf("actual aura hazard bypassed matching absorption/received healing: %d", p.HP)
	}
}

func TestElementAbsorptionCallbacksGuardGenerationNotPureAbsorption(t *testing.T) {
	s, p, m := absorptionCompositionFixture(t, true)
	old := legendaryHandlers["TY-D12"]
	handler := old
	healed, resolved := 0, 0
	handler.Healed = func(c *legendaryContext, before, effective int32) {
		healed++
		if effective != 28 || c.Skill != 0 {
			t.Fatal("absorption callback fabricated skill or double-applied healing", effective, c.Skill)
		}
	}
	handler.HealResolved = func(c *legendaryContext, raw, effective, overflow int32) {
		resolved++
		// A different effect may react with damage, but the nested absorb must not
		// re-enter the same absorption producer. Its damage prevention/heal still work.
		ev, damage, reflect := s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{Magic: true, HurtType: domain.HurtFire, SkillPct: 100, GuaranteedHit: true})
		s.afterStrikeStatuses(m, p, ev, damage, reflect, strikeReactionContext{HurtType: domain.HurtFire})
	}
	legendaryHandlers["TY-D12"] = handler
	t.Cleanup(func() { legendaryHandlers["TY-D12"] = old })
	ev, raw, reflect := s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{Magic: true, HurtType: domain.HurtFire, SkillPct: 100, GuaranteedHit: true})
	s.afterStrikeStatuses(m, p, ev, raw, reflect, strikeReactionContext{HurtType: domain.HurtFire})
	if p.HP != 500-2*(80-28) || healed != 1 || resolved != 1 {
		t.Fatal("absorption loop or pure modifier suppression", p.HP, healed, resolved)
	}
}

func TestElementAbsorptionRealGeneratedRoutesDispatchBothHealHooks(t *testing.T) {
	for _, route := range []string{"burn", "reflection", "full_reflection", "aura"} {
		t.Run(route, func(t *testing.T) {
			s, p, m := absorptionCompositionFixture(t, true)
			if route == "aura" {
				s, m, p, _ = riftAbilityFixture(t, riftEruption)
				p.Player.Char.Level = 100
				incomingEquipPower(t, s, p, "TY-D12", domain.SlotBody)
				p.HP, p.MaxHP = 500, 10000
				p.Stats = domain.Stats{}
				var equipment domain.EquipmentCombatStats
				equipment.Elements[0].AbsorbBP = 2000
				cacheElementV5(s, p, equipment)
			}
			old := legendaryHandlers["TY-D12"]
			h := old
			healed, resolved := 0, 0
			h.Healed = func(c *legendaryContext, before, effective int32) {
				healed++
				if effective != 28 || c.Skill != 0 {
					t.Fatal("wrong heal-hook amount/skill", effective, c.Skill)
				}
			}
			h.HealResolved = func(c *legendaryContext, raw, effective, overflow int32) {
				resolved++
				if raw != 28 || effective != 28 || overflow != 0 {
					t.Fatal("wrong resolved amount", raw, effective, overflow)
				}
			}
			legendaryHandlers["TY-D12"] = h
			defer func() { legendaryHandlers["TY-D12"] = old }()
			switch route {
			case "burn":
				s.tickStatus(p, domain.Active{Source: m.ID, Def: domain.StatusDef{ID: 1036, SourceSkillID: 10421, SkillSnapshotID: 10421, HPChanges: []domain.ResourceChange{{Value: -100, Mode: domain.ModeAbsolute}}}})
			case "reflection", "full_reflection":
				reflectStatus := domain.StatusDef{ID: 199992, DurationSec: 30, MagicReflectPct: 100}
				if route == "full_reflection" {
					reflectStatus.MagicReflectPct = 0
					reflectStatus.MagicFullReflectChancePct = 100
				}
				m.Status.Apply(reflectStatus, s.tick, m.ID)
				ev, raw, reflected := s.resolveStrikeStatuses(p, m, &combat.HitTracker{}, combat.Blow{Magic: true, HurtType: domain.HurtFire, SkillPct: 100, GuaranteedHit: true})
				s.afterStrikeStatuses(p, m, ev, raw, reflected, strikeReactionContext{HurtType: domain.HurtFire})
			case "aura":
				s.hitRiftHazard(&endgameRiftHazard{Source: m.ID, SourceLevel: m.Level, Attack: domain.Stats{MAtk: 100}, Pos: p.Pos}, p, 100)
			}
			if p.HP != 448 || healed != 1 || resolved != 1 {
				t.Fatal("generated route failed one HP transaction/one reaction", p.HP, healed, resolved)
			}
		})
	}
}

func TestElementAbsorptionIncomingBonusStillRespectsActualQuota(t *testing.T) {
	s, p, _ := absorptionCompositionFixture(t, true)
	p.HP, p.MaxHP = 500, 1000
	first := s.equipmentAbsorb(p, 100, 2000, false)
	// 20 nominal ×1.4 cannot exceed the authored 2% MaxHP rolling quota.
	if first.Damage != 80 || first.Heal != 20 || first.HP != 440 {
		t.Fatal("received bonus exceeded quota or applied HP twice", first)
	}
	p.HP = first.HP
	second := s.equipmentAbsorb(p, 100, 2000, false)
	if second.Damage != 80 || second.Heal != 0 || second.HP != 360 {
		t.Fatal("quota suppressed pure absorption or granted repeated heal", second)
	}
}
