package scene

import (
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

func buffPassContext(t *testing.T, s *Scene, owner, target *entity.Entity, id string, skill domain.SkillID) legendaryContext {
	t.Helper()
	power, ok := domain.LegendaryPower(id)
	if !ok {
		t.Fatal("power missing", id)
	}
	return legendaryContext{Scene: s, Owner: owner, Target: target, Power: power, SourceUID: 991, Skill: skill, NowMS: s.now().UnixMilli()}
}

func buffPassStatus(e *entity.Entity, id string) (domain.Active, bool) {
	var out domain.Active
	found := false
	e.Status.EachEffective(func(a domain.Active) {
		if a.Def.LegendaryPowerID == id {
			out, found = a, true
		}
	})
	return out, found
}

// CK-U03: seal/frost trap radius +50% and +15% physical vulnerability for 8s.
func buffPassAssassinTrapBreaker(t *testing.T) {
	for _, skill := range []domain.SkillID{10215, 10216, 10213} {
		for _, enabled := range []bool{false, true} {
			s, p, m := assassinCoverageFixture(t, "CK-U03", enabled)
			d := assassinCoverageSkill(skill)
			d.Effects = []domain.SkillEffect{{Kind: domain.EffectPlaceTrap, TrapRadius: 100}, {Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}}
			s.legendaryPrepareSkill(p, &d, m.ID)
			want := int32(100)
			if enabled && skill != 10213 {
				want = 150
			}
			if got := d.DirectEffects()[0].TrapRadius; got != want {
				t.Fatal("trap radius", skill, enabled, got, want)
			}
		}
	}
	for _, skill := range []domain.SkillID{10215, 10216, 10213} {
		s, p, m := assassinCoverageFixture(t, "CK-U03", true)
		c := buffPassContext(t, s, p, m, "CK-U03", skill)
		legendaryHandlers["CK-U03"].SkillResolved(&c)
		a, ok := buffPassStatus(m, "CK-U03")
		if ok != (skill != 10213) {
			t.Fatal("vulnerability skill gate", skill, ok)
		}
		if ok && (a.Def.PhysicalDamageTakenPct != 15 || a.Def.MagicDamageTakenPct != 0 || a.ExpireAt-s.tick != domain.Ticks(8000)) {
			t.Fatal("vulnerability shape", a.Def.PhysicalDamageTakenPct, a.Def.MagicDamageTakenPct, a.ExpireAt-s.tick)
		}
	}
}

// CK-U04: shadow jump cooldown -50%, 3s +60% move speed, next backstab crits.
func buffPassAssassinShadowStep(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		s, p, m := assassinCoverageFixture(t, "CK-U04", enabled)
		p.Player.Char.Skills[10227] = 1
		d := assassinCoverageSkill(10227)
		d.CooldownMS = 10000
		s.legendaryPrepareSkill(p, &d, m.ID)
		wantCDR := int32(0)
		if enabled {
			wantCDR = 5000
		}
		if d.LegendaryModifiers.CooldownReductionBP != wantCDR {
			t.Fatal("shadow jump cooldown", d.LegendaryModifiers.CooldownReductionBP)
		}
		s.refreshStats(p)
		base := p.Stats.MoveSpeed
		s.legendaryCastReleased(p, domain.SkillDef{ID: 10227}, 0, nil)
		s.refreshStats(p)
		wantSpeed := base
		if enabled {
			wantSpeed = base * 160 / 100
		}
		if p.Stats.MoveSpeed != wantSpeed {
			t.Fatal("shadow step speed", base, p.Stats.MoveSpeed, wantSpeed)
		}
		back := assassinCoverageSkill(10209)
		s.legendaryPrepareSkill(p, &back, m.ID)
		if back.DirectEffects()[0].GuaranteedCrit != enabled {
			t.Fatal("next backstab crit", enabled)
		}
		s.legendaryCastReleased(p, domain.SkillDef{ID: 10209}, m.ID, []domain.EntityID{m.ID})
		again := assassinCoverageSkill(10209)
		s.legendaryPrepareSkill(p, &again, m.ID)
		if again.DirectEffects()[0].GuaranteedCrit {
			t.Fatal("guaranteed crit survived the backstab")
		}
		if enabled {
			p.Status.Expire(s.tick + domain.Ticks(3000))
			s.refreshStats(p)
			if p.Stats.MoveSpeed != base {
				t.Fatal("speed window outlived 3s", p.Stats.MoveSpeed, base)
			}
		}
	}
}
