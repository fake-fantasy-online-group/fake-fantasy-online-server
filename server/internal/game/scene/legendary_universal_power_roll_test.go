package scene

import (
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func TestLegendaryUniversalPowerRollSkillBenefitsAndGuaranteedMechanic(t *testing.T) {
	for _, roll := range []int32{0, 7500, 8765, 10000} {
		for _, tc := range []struct {
			id    string
			base  int32
			field func(domain.LegendarySkillModifiers) int32
		}{
			{"TY-A02", 6000, func(m domain.LegendarySkillModifiers) int32 { return m.LengthBP }},
			{"TY-U03", 3000, func(m domain.LegendarySkillModifiers) int32 { return m.ManaReductionBP }},
			{"TY-U15", 2000, func(m domain.LegendarySkillModifiers) int32 { return m.CooldownReductionBP }},
			{"TY-U16", 6000, func(m domain.LegendarySkillModifiers) int32 { return m.HealingBP }},
		} {
			p, _ := domain.LegendaryPowerForRoll(tc.id, roll)
			c := legendaryContext{Power: p, PowerRollBP: roll}
			d := domain.SkillDef{Kind: domain.SkillSingle, MPCost: 100, CooldownMS: 1000, Area: domain.SkillAreaTargeting{Shape: domain.SkillAreaRectangle, Center: domain.SkillAreaCenterCaster, Length: 100, Width: 100}}
			legendaryHandlers[tc.id].PrepareSkill(&c, &d)
			want := domain.LegendaryPowerRollValue(tc.id, "benefit", tc.base, roll)
			if got := tc.field(d.LegendaryModifiers); got != want {
				t.Fatalf("%s roll%d got%d want%d", tc.id, roll, got, want)
			}
			(&Scene{}).legendaryFinalizeSkill(&d)
			if tc.id == "TY-U15" && d.CooldownMS != int32((1000*int64(10000-want)+9999)/10000) {
				t.Fatal("lower roll made cooldown stronger", d.CooldownMS, want)
			}
			if tc.id == "TY-U03" && d.MPCost != int32((100*int64(10000-want)+9999)/10000) {
				t.Fatal("lower roll made mana benefit stronger", d.MPCost, want)
			}
		}
	}
	c := legendaryContext{Power: domain.LegendaryPowerDefinition{ID: "TY-A09"}}
	d := domain.SkillDef{LegendaryBasic: true, Effect: domain.SkillEffect{Kind: domain.EffectDamage}}
	legendaryHandlers["TY-A09"].PrepareSkill(&c, &d)
	if !d.DirectEffects()[0].GuaranteedHit || domain.LegendaryPowerHasRoll("TY-A09") {
		t.Fatal("guaranteed hit randomized")
	}
}

func TestLegendaryUniversalPowerRollActualIncomingStatusAndStats(t *testing.T) {
	for _, roll := range []int32{0, 7500, 8765, 10000} {
		s, p, m := universalSoloFixture(t, "TY-D07", domain.Warrior, domain.Learned{})
		var slot domain.EquipSlot
		p.Player.Worn.Each(func(at domain.EquipSlot, st domain.Stack) {
			if st.Legendary.PowerID == "TY-D07" {
				slot = at
			}
		})
		gear := p.Player.Worn.At(slot)
		gear.Legendary.PowerRollBP = roll
		p.Player.Worn.Set(slot, gear)
		contexts := s.legendaryIncomingContexts(m, p, combat.Blow{}, event.DamageDealt{Src: m.ID, Dst: p.ID})
		if len(contexts) != 1 || contexts[0].PowerRollBP != roll {
			t.Fatal("active roll missing", contexts)
		}
		got := legendaryHandlers["TY-D07"].ModifyIncoming(&contexts[0], 10000)
		want := 10000 - domain.LegendaryPowerRollValue("TY-D07", "benefit", 2000, roll)
		if got != want {
			t.Fatal("actual mitigation", got, want)
		}
		// Chance bonuses are weakened, not the unconditional 100% cap.
		c := legendaryContext{Power: domain.LegendaryPowerDefinition{ID: "TY-A23"}, PowerRollBP: roll}
		status := domain.StatusDef{SourceSkillID: 10211, PoisonOnHitChance: 10}
		legendaryHandlers["TY-A23"].StatusApplied(&c, &status)
		s.legendaryFinalizeStatus(&status)
		if status.PoisonOnHitChance != 10+domain.LegendaryPowerRollValue("TY-A23", "benefit", 40, roll) {
			t.Fatal("poison chance", status.PoisonOnHitChance)
		}
		c.Power.ID = "TY-A10"
		stats := domain.Stats{AtkSpeedMS: 1000}
		legendaryHandlers["TY-A10"].PassiveStats(&c, &stats)
		bp := domain.LegendaryPowerRollValue("TY-A10", "benefit", 2000, roll)
		if stats.AtkSpeedMS != 1000*(10000-bp)/10000 || stats.AtkSpeedPct != int32(1000000/int64(10000-bp)-100) {
			t.Fatal("attack interval inconsistent", stats, bp)
		}
	}
}

func TestLegendaryPowerRollAcceptedSnapshotAndTooltipAgree(t *testing.T) {
	s, p, m := universalSoloFixture(t, "TY-A02", domain.Warrior, domain.Learned{})
	var slot domain.EquipSlot
	p.Player.Worn.Each(func(at domain.EquipSlot, st domain.Stack) {
		if st.Legendary.PowerID == "TY-A02" {
			slot = at
		}
	})
	st := p.Player.Worn.At(slot)
	st.Legendary.PowerRollBP = 7500
	p.Player.Worn.Set(slot, st)
	accepted := &domain.CombatSourceSnapshotV5{LegendaryPowers: domain.ActiveLegendaryPowers(s.functionalWorn(p), s.itemDef, p.Player.Char)}
	st.Legendary.PowerRollBP = 10000
	p.Player.Worn.Set(slot, st)
	contexts := s.legendaryContextsFor(p, m, combat.Blow{SourceOffense: accepted}, event.DamageDealt{}, true)
	if len(contexts) != 1 || contexts[0].PowerRollBP != 7500 || !strings.Contains(contexts[0].Power.Effect, "45%（45%～60%）") {
		t.Fatal("inflight attack upgraded", contexts)
	}
	view := s.endgameItem(st, "equipment", int32(slot))
	if view.Legendary == nil || !strings.Contains(view.Legendary.Description, "60%（45%～60%）") {
		t.Fatal("equipped value/range missing", view.Legendary)
	}
}
