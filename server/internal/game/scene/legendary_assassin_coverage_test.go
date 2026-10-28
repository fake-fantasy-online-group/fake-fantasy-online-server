package scene

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
)

// These are effect-case coverage tests, not a claim about Go statement coverage.
// Each CK subtest equips precisely one power; enabled=false preserves the same
// item and panel while removing only its legendary instance. Consumer fixtures
// intentionally use round coefficients to make the independent numeric oracle exact.
func assassinCoverageFixture(t *testing.T, id string, enabled bool) (*Scene, *entity.Entity, *entity.Entity) {
	t.Helper()
	s, p, m := legendaryAssassinFixture(t, id)
	if !enabled {
		p.Player.Worn.Each(func(slot domain.EquipSlot, st domain.Stack) {
			st.Legendary = domain.LegendaryInstance{}
			p.Player.Worn.Set(slot, st)
		})
	}
	s.rng = rand.New(rand.NewSource(7))
	return s, p, m
}
func assassinCoverageMob(s *Scene, id domain.EntityID, x, y float64) *entity.Entity {
	m := putMonster(s, id, 10000, domain.Stats{}, x, y)
	m.Status = domain.NewStatusSet()
	s.monsters[id] = m
	return m
}
func assassinCoverageSkill(id domain.SkillID) domain.SkillDef {
	return domain.SkillDef{ID: id, Level: 1, Prof: 3, Kind: domain.SkillSingle, TargetEnemy: true, Dist: 300, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}}
}
func assassinCoverageHit(s *Scene, p, m *entity.Entity, d domain.SkillDef) int32 {
	before := m.HP
	s.legendaryPrepareSkill(p, &d, m.ID)
	assassinNativeHits(s, p, d, m.ID, []domain.EntityID{m.ID})
	return before - m.HP
}
func assassinCoverageActive(t *testing.T, p *entity.Entity, id domain.StatusID) domain.Active {
	t.Helper()
	var out domain.Active
	p.Status.EachEffective(func(a domain.Active) {
		if a.Def.ID == id {
			out = a
		}
	})
	if out.Def.ID != id {
		t.Fatalf("status %d not committed", id)
	}
	return out
}
func assassinCoverageCast(t *testing.T, s *Scene, p, m *entity.Entity, d domain.SkillDef) domain.SkillDef {
	t.Helper()
	s.skills = domain.SkillTable{{ID: d.ID, Level: 1}: d}
	return assassinRegressionCast(t, s, p, UseSkill{Skill: d.ID, Target: m.ID})
}
func assassinCoverageCosts(t *testing.T, id string, skills []domain.SkillID, mp, cd int32) {
	t.Helper()
	for _, skill := range skills {
		for _, enabled := range []bool{false, true} {
			s, p, m := assassinCoverageFixture(t, id, enabled)
			d := assassinCoverageSkill(skill)
			d.MPCost = 100
			d.CooldownMS = 10000
			before := p.MP
			start := s.tick
			assassinCoverageCast(t, s, p, m, d)
			wantMP, wantCD := int32(100), int32(10000)
			if enabled {
				wantMP, wantCD = mp, cd
			}
			if before-p.MP != wantMP {
				t.Fatalf("skill%d enabled%v MP=%d want%d", skill, enabled, before-p.MP, wantMP)
			}
			at := start + domain.Ticks(int(wantCD))
			if !p.Player.SkillReadyAt(skill, at) || p.Player.SkillReadyAt(skill, at-1) {
				t.Fatalf("skill%d actual cooldown boundary != %d", skill, wantCD)
			}
		}
	}
}

func TestAssassinUnitCoverage(t *testing.T) {
	for _, prefix := range []string{"A", "D", "U"} {
		n := 10
		if prefix == "A" {
			n = 30
		}
		for i := 1; i <= n; i++ {
			id := fmt.Sprintf("CK-%s%02d", prefix, i)
			t.Run(id, func(t *testing.T) { assassinCoveragePower(t, id) })
		}
	}
}
func assassinCoveragePower(t *testing.T, id string) {
	switch id {
	case "CK-A01":
		assassinCoverageLine(t)
	case "CK-A02":
		TestLegendaryAssassinSynergyReplacementCritUsesNativeRules(t)
		assassinCoverageCrit(t)
	case "CK-A03", "CK-A04":
		skill, want := domain.SkillID(10221), int32(180)
		if id == "CK-A04" {
			skill, want = 10205, 150
		}
		for _, enabled := range []bool{false, true} {
			for _, distance := range []float64{30, 200} {
				s, p, m := assassinCoverageFixture(t, id, enabled)
				m.Pos.X = p.Pos.X + distance
				p.Look.AtkDist = 50
				s.aoi.Move(m, m.Pos)
				expected := int32(100)
				if enabled {
					expected = want
				}
				if got := assassinCoverageHit(s, p, m, assassinCoverageSkill(skill)); got != expected {
					t.Fatalf("distance%v damage%d want%d", distance, got, expected)
				}
			}
		}
	case "CK-A05":
		assassinCoverageSplash(t)
	case "CK-A06":
		assassinCoverageCircle(t)
	case "CK-A07", "CK-A21":
		assassinCoveragePrimary(t, id)
	case "CK-A08":
		assassinCoverageSpread(t)
	case "CK-A09", "CK-A15":
		assassinCoveragePoisonLifetime(t, id)
	case "CK-A10":
		assassinCoverageThrowPoison(t)
	case "CK-A11", "CK-A12", "CK-A14", "CK-A16", "CK-A17":
		assassinCoverageTrap(t, id)
	case "CK-A13":
		assassinCoverageTriple(t)
	case "CK-A18":
		assassinCoverageFrost(t)
	case "CK-A19":
		assassinCoverageFan(t)
	case "CK-A20":
		assassinCoverageBackstabGate(t)
	case "CK-A22":
		assassinCoverageLeech(t, id, 10209)
	case "CK-A23":
		assassinCoverageStun(t)
	case "CK-A24", "CK-A25", "CK-D03":
		assassinCoverageCounter(t, id)
	case "CK-A26":
		TestLegendaryAssassinSynergyTheftPalmPreservesAttempt(t)
		assassinCoveragePalm(t)
	case "CK-A27":
		assassinCoverageThrowRange(t)
	case "CK-A28", "CK-A30":
		assassinCoveragePull(t, id)
	case "CK-A29":
		assassinCoverageDeath(t)
	case "CK-D01", "CK-D04", "CK-D06", "CK-D07", "CK-D10":
		assassinCoverageDefense(t, id)
	case "CK-D02", "CK-U02":
		assassinCoverageSmoke(t, id)
	case "CK-D05":
		assassinCoverageControl(t)
	case "CK-D08":
		assassinCoverageCleanse(t)
	case "CK-D09":
		assassinCoverageMaxHP(t)
	case "CK-U01":
		assassinCoverageRefund(t)
	case "CK-U03":
		TestAssassinRegressionDisarmPreservesVisibleFriendlyTraps(t)
		assassinCoverageDisarm(t)
	case "CK-U04":
		assassinCoverageDetect(t)
	case "CK-U05":
		assassinCoverageRecovery(t)
	case "CK-U06":
		assassinCoverageCooldown(t)
	case "CK-U07":
		assassinCoverageCosts(t, id, []domain.SkillID{10213, 10214, 10215, 10216, 10217}, 100, 10000)
	case "CK-U08":
		assassinCoverageStealthBudget(t)
	case "CK-U09":
		assassinCoverageCosts(t, id, []domain.SkillID{10221, 10219}, 70, 8000)
	case "CK-U10":
		assassinCoverageStealthSpeed(t)
	default:
		t.Fatal("unimplemented effect case", id)
	}
}
func assassinCoverageLine(t *testing.T) {
	for _, weapon := range []int32{40, 300} {
		for _, enabled := range []bool{false, true} {
			s, p, m := assassinCoverageFixture(t, "CK-A01", enabled)
			p.Look.AtkDist = uint16(weapon)
			d := assassinCoverageSkill(10221)
			d.Dist = 200
			length, width := int32(200), int32(40)
			if weapon > length {
				length = weapon
			}
			if weapon/2 > width {
				width = weapon / 2
			}
			in := assassinCoverageMob(s, 901, p.Pos.X+float64(length)-1, p.Pos.Y+float64(width)/2-1)
			out := assassinCoverageMob(s, 902, p.Pos.X+float64(length)+1, p.Pos.Y)
			wide := assassinCoverageMob(s, 903, p.Pos.X+100, p.Pos.Y+float64(width)/2+1)
			immune := assassinCoverageMob(s, 904, p.Pos.X+60, p.Pos.Y)
			immune.Status.Apply(domain.StatusDef{ID: 990010, DurationSec: 30, BlockHarmful: true}, s.tick, immune.ID)
			s.statuses[domain.StatusKey{ID: 1034, Level: 1}] = domain.StatusDef{ID: 1034, Level: 1, TypeKnown: true, Type: domain.StatusBad, BlockMove: true, DurationSec: 10}
			d.Statuses = []domain.StatusApplication{{ID: 1034, Level: 1, ChanceExplicit: true, ChanceBP: 10000, SourceSkill: 10221}}
			s.legendaryPrepareSkill(p, &d, m.ID)
			ids := s.resolveTargets(p, d, m.ID, nil)
			assassinNativeHits(s, p, d, m.ID, ids)
			if m.HP != m.MaxHP-100 || !m.Status.Has(1034) {
				t.Fatal("primary full damage/root missing")
			}
			if enabled {
				if in.HP != 9900 || !in.Status.Has(1034) || immune.HP != 9900 || immune.Status.Has(1034) {
					t.Fatal("rectangle full damage or root immunity failed", in.HP, immune.HP)
				}
			} else if in.HP != 10000 || immune.HP != 10000 {
				t.Fatal("unequipped line leaked")
			}
			if out.HP != 10000 || wide.HP != 10000 {
				t.Fatal("rectangle boundary escaped")
			}
		}
	}
	assassinCoverageCosts(t, "CK-A01", []domain.SkillID{10221}, 100, 7000)
}
func assassinCoverageSplash(t *testing.T) {
	for _, radius := range []int32{0, 200} {
		for _, enabled := range []bool{false, true} {
			s, p, m := assassinCoverageFixture(t, "CK-A05", enabled)
			d := assassinCoverageSkill(10205)
			d.Radius = radius
			near := assassinCoverageMob(s, 901, m.Pos.X+99, m.Pos.Y)
			outer := assassinCoverageMob(s, 902, m.Pos.X+150, m.Pos.Y)
			far := assassinCoverageMob(s, 903, m.Pos.X+201, m.Pos.Y)
			s.legendaryPrepareSkill(p, &d, m.ID)
			ids := s.legendarySelectTargets(p, d, m.ID, []domain.EntityID{m.ID})
			assassinNativeHits(s, p, d, m.ID, ids)
			if (near.HP == 9900) != enabled || (outer.HP == 9900) != (enabled && radius == 200) || far.HP != 10000 {
				t.Fatal("splash minimum/preserved radius failed", near.HP, outer.HP, far.HP)
			}
			for j := 0; j < 7; j++ {
				assassinCoverageMob(s, domain.EntityID(910+j), m.Pos.X+float64(j+1), m.Pos.Y)
			}
			ids = s.legendarySelectTargets(p, d, m.ID, []domain.EntityID{m.ID})
			want := 1
			if enabled {
				want = 9
				if radius == 200 {
					want = 10
				}
			}
			if len(ids) != want || ids[0] != m.ID {
				t.Fatal("primary + 3 cap", ids)
			}
			before := map[domain.EntityID]int32{}
			for _, v := range ids {
				before[v] = s.entities[v].HP
			}
			assassinNativeHits(s, p, d, m.ID, ids)
			for _, v := range ids {
				if before[v]-s.entities[v].HP != 100 {
					t.Fatal("secondary damage was divided")
				}
			}
		}
	}
}
func assassinCoverageCircle(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		s, p, m := assassinCoverageFixture(t, "CK-A06", enabled)
		back := assassinCoverageMob(s, 901, -19, 100)
		far := assassinCoverageMob(s, 902, -21, 100)
		d := assassinCoverageSkill(10206)
		d.Kind = domain.SkillArea
		d.Radius = 100
		d.Area = domain.SkillAreaTargeting{Shape: domain.SkillAreaCone, Center: domain.SkillAreaCenterCaster, Radius: 100, Angle: 60}
		s.legendaryPrepareSkill(p, &d, m.ID)
		ids := s.resolveTargets(p, d, m.ID, nil)
		assassinNativeHits(s, p, d, m.ID, ids)
		want := int32(100)
		if enabled {
			want = 150
		}
		if m.MaxHP-m.HP != want || (back.HP == 9850) != enabled || far.HP != 10000 {
			t.Fatal("circle/radius/full damage", m.HP, back.HP, far.HP)
		}
	}
}
func assassinCoveragePrimary(t *testing.T, id string) {
	skill, want := domain.SkillID(10206), int32(150)
	if id == "CK-A21" {
		skill, want = 10209, 175
	}
	for _, n := range []int{1, 5} {
		for _, enabled := range []bool{false, true} {
			s, p, m := assassinCoverageFixture(t, id, enabled)
			ids := []domain.EntityID{m.ID}
			for j := 1; j < n; j++ {
				v := assassinCoverageMob(s, domain.EntityID(900+j), 140+float64(j), 100)
				ids = append(ids, v.ID)
			}
			d := assassinCoverageSkill(skill)
			s.legendaryPrepareSkill(p, &d, m.ID)
			assassinNativeHits(s, p, d, m.ID, ids)
			expected := int32(100)
			if enabled {
				expected = want
			}
			if m.MaxHP-m.HP != expected {
				t.Fatal("primary bonus depends on target count")
			}
			for _, v := range ids[1:] {
				if s.entities[v].HP != 9900 {
					t.Fatal("primary bonus leaked to secondary")
				}
			}
		}
	}
}
func assassinCoverageSpread(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		s, p, m := assassinCoverageFixture(t, "CK-A08", enabled)
		poison := domain.StatusDef{ID: domain.StatusPoison, Level: 21, ClassSetPoison: true, TypeKnown: true, Type: domain.StatusBad, DurationSec: 10, IntervalSec: 2, Affixes: []domain.Affix{{Attr: domain.AttrHPRegen, Value: -10}}}
		s.applyStatusDef(m, poison, p.ID)
		poisonTicks(s, 1)
		original := assassinCoverageActive(t, m, domain.StatusPoison)
		var victims []*entity.Entity
		for j := 0; j < 7; j++ {
			victims = append(victims, assassinCoverageMob(s, domain.EntityID(901+j), 140+float64(j), 100))
		}
		far := assassinCoverageMob(s, 920, m.Pos.X+201, m.Pos.Y)
		s.applyStatusDef(victims[0], poison, p.ID)
		m.HP = 0
		s.legendaryAssassinDeath(m)
		count := 0
		for _, v := range victims {
			if v.Status.Has(domain.StatusPoison) {
				count++
				a := assassinCoverageActive(t, v, domain.StatusPoison)
				if enabled && v != victims[0] && (a.ExpireAt != original.ExpireAt || a.NextTickAt != original.NextTickAt) {
					t.Fatal("remaining poison clock reset")
				}
				copies := 0
				v.Status.EachEffective(func(a domain.Active) {
					if a.Def.ID == domain.StatusPoison && a.Source == p.ID {
						copies++
					}
				})
				if copies != 1 {
					t.Fatal("same-source copies stacked")
				}
			}
		}
		want := 1
		if enabled {
			want = 7
		}
		if count != want || far.Status.Has(domain.StatusPoison) {
			t.Fatal("spread cap/radius", count)
		}
		if enabled {
			before := victims[1].HP
			poisonTicks(s, 1)
			if before-victims[1].HP != 10 {
				t.Fatal("copied poison potency changed")
			}
			fresh := assassinCoverageMob(s, 930, 150, 100)
			victims[1].HP = 0
			s.legendaryAssassinDeath(victims[1])
			s.legendaryAssassinDeath(m)
			if fresh.Status.Has(domain.StatusPoison) {
				t.Fatal("copy or same-death spread recurred")
			}
		}
	}
}
func assassinCoveragePoisonLifetime(t *testing.T, id string) {
	for _, eligible := range []bool{false, true} {
		for _, enabled := range []bool{false, true} {
			s, p, m := assassinCoverageFixture(t, id, enabled)
			d := domain.StatusDef{ID: domain.StatusPoison, Level: 1, DurationSec: 10, IntervalSec: 2, Affixes: []domain.Affix{{Attr: domain.AttrHPRegen, Value: -10}}}
			if id == "CK-A09" {
				d.ClassSetPoison = eligible
				d.SourceSkillID = 10211
			} else if eligible {
				d.SourceSkillID = 10214
			} else {
				d.SourceSkillID = 10216
			}
			s.applyStatusDef(m, d, p.ID)
			a := assassinCoverageActive(t, m, domain.StatusPoison)
			if a.ExpireAt-s.tick != domain.Ticks(10000) {
				t.Fatal("duration changed")
			}
			want := int32(10)
			if enabled && eligible {
				want = 20
			}
			for i := 0; i < 5; i++ {
				hp := m.HP
				poisonTicks(s, 1)
				if hp-m.HP != want {
					t.Fatalf("tick%d=%d want%d", i, hp-m.HP, want)
				}
			}
			hp := m.HP
			poisonTicks(s, 1)
			if m.HP != hp || m.Status.Has(domain.StatusPoison) {
				t.Fatal("extra damage tick or duration")
			}
			if id == "CK-A15" && assassinCoverageHit(s, p, m, assassinCoverageSkill(10214)) != 100 {
				t.Fatal("poison power changed direct burst")
			}
		}
	}
}
func assassinCoverageThrowPoison(t *testing.T) {
	for _, skill := range []domain.SkillID{10205, 10206} {
		for _, buff := range []bool{false, true} {
			for _, enabled := range []bool{false, true} {
				s, p, m := assassinCoverageFixture(t, "CK-A10", enabled)
				s.rng = rand.New(meleeAuditZeroSource{})
				if buff {
					p.Status.Apply(domain.StatusDef{ID: domain.StatusPoisonHit, Level: 1, DurationSec: 120, PoisonOnHitChance: 25, PoisonOnHitLevel: 21, SourceSkillID: 10211, SourceEffectHitKnown: true}, s.tick, p.ID)
				}
				s.statuses[domain.StatusKey{ID: domain.StatusPoison, Level: 21}] = domain.StatusDef{ID: domain.StatusPoison, Level: 21, DurationSec: 10, IntervalSec: 2, Affixes: []domain.Affix{{Attr: domain.AttrHPRegen, Value: -10}}}
				side := assassinCoverageMob(s, 901, 140, 100)
				d := assassinCoverageSkill(skill)
				s.legendaryPrepareSkill(p, &d, m.ID)
				// One native root with repeated hit on the primary and another target. A
				// second hit must not refresh the primary's poison clock after time advances.
				finish := s.beginEquipmentRoot(p.ID, m.ID, s.equipmentOffense(p))
				s.equipment.root.def = d
				s.applySkill(p, m, d, 100, false)
				first := domain.Active{}
				if m.Status.Has(domain.StatusPoison) {
					first = assassinCoverageActive(t, m, domain.StatusPoison)
				}
				s.tick++
				s.applySkill(p, m, d, 100, false)
				s.applySkill(p, side, d, 100, false)
				finish()
				want := buff && enabled
				if m.Status.Has(domain.StatusPoison) != want || side.Status.Has(domain.StatusPoison) != want {
					t.Fatal("throw poison activation gate")
				}
				if want {
					if assassinCoverageActive(t, m, domain.StatusPoison).ExpireAt != first.ExpireAt {
						t.Fatal("same-cast poison refreshed twice")
					}
					hp := m.HP
					poisonTicks(s, 1)
					if hp-m.HP != 10 {
						t.Fatal("poison potency not native")
					}
				}
				if buff && assassinCoverageActive(t, p, domain.StatusPoisonHit).Def.PoisonOnHitChance != 25 {
					t.Fatal("ordinary poison chance mutated")
				}
			}
		}
	}
}
func assassinCoverageTrap(t *testing.T, id string) {
	skill := map[string]domain.SkillID{"CK-A11": 10213, "CK-A12": 10213, "CK-A14": 10214, "CK-A16": 10215, "CK-A17": 10216}[id]
	for _, enabled := range []bool{false, true} {
		s, p, m := assassinCoverageFixture(t, id, enabled)
		s.rng = rand.New(meleeAuditZeroSource{})
		near := assassinCoverageMob(s, 901, m.Pos.X+129, m.Pos.Y)
		far := assassinCoverageMob(s, 902, m.Pos.X+131, m.Pos.Y)
		status := domain.StatusID(990020)
		sd := domain.StatusDef{ID: status, Level: 1, DurationSec: 10, TypeKnown: true, Type: domain.StatusBad}
		if id == "CK-A14" {
			status = domain.StatusPoison
			sd.ID = status
			sd.IntervalSec = 2
			sd.Affixes = []domain.Affix{{Attr: domain.AttrHPRegen, Value: -10}}
		}
		if id == "CK-A16" {
			sd.Control = domain.ControlSilence
		}
		if id == "CK-A17" {
			status = domain.StatusFreeze
			sd.ID = status
			sd.Control = domain.ControlStun
		}
		s.statuses[domain.StatusKey{ID: status, Level: 1}] = sd
		d := assassinCoverageSkill(skill)
		d.Effects = []domain.SkillEffect{{Kind: domain.EffectPlaceTrap, TrapRadius: 100}, {Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}}
		d.Statuses = []domain.StatusApplication{{ID: status, Level: 1, ChanceBP: 10000, ChanceExplicit: true, SourceSkill: skill}}
		s.legendaryPrepareSkill(p, &d, m.ID)
		s.placeTrap(p, d, m.Pos, d.DirectEffects()[0].TrapRadius, 0)
		instant := enabled && (id == "CK-A11" || id == "CK-A14")
		if (len(s.traps) == 0) != instant {
			t.Fatal("immediate trigger timing", len(s.traps))
		}
		if !instant && m.HP != m.MaxHP {
			t.Fatal("persistent trap hit before trigger")
		}
		s.stepTraps()
		want := int32(100)
		if enabled {
			switch id {
			case "CK-A11":
				want = 140
			case "CK-A12":
				want = 190
			case "CK-A16", "CK-A17":
				want = 150
			}
		}
		if m.MaxHP-m.HP != want || !m.Status.Has(status) || (near.HP == 10000-want) != enabled || far.HP != 10000 {
			t.Fatal("trap radius/damage/status", id, m.HP, near.HP, far.HP)
		}
		if enabled && !near.Status.Has(status) {
			t.Fatal("expanded trap target lost status")
		}
		if id == "CK-A14" {
			a := assassinCoverageActive(t, m, domain.StatusPoison)
			if a.ExpireAt-s.tick != domain.Ticks(10000) {
				t.Fatal("poison duration changed")
			}
			hp := m.HP
			poisonTicks(s, 1)
			if hp-m.HP != 10 {
				t.Fatal("mist poison altered")
			}
		}
		if id == "CK-A17" {
			expected := int32(100)
			if enabled {
				expected = 150
			}
			if assassinCoverageHit(s, p, m, assassinCoverageSkill(10209)) != expected {
				t.Fatal("backstab clause missing")
			}
		}
	}
	if id == "CK-A11" {
		for _, enabled := range []bool{false, true} {
			s, p, m := assassinCoverageFixture(t, id, enabled)
			d := assassinCoverageSkill(10213)
			d.Dist = 100
			m.Pos.X = 240
			s.aoi.Move(m, m.Pos)
			s.skills = domain.SkillTable{{ID: d.ID, Level: 1}: d}
			s.onUseSkill(UseSkill{ID: p.ID, Skill: d.ID, Target: m.ID})
			if (len(s.casts) == 1) != enabled {
				t.Fatal("50% throw range did not affect admission")
			}
		}
	}
}
func assassinCoverageTriple(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		powers := []string{}
		if enabled {
			powers = []string{"CK-A13"}
		}
		s, p, m := nativeRepairFixture(t, domain.Assassin, domain.Learned{10213: 1}, powers...)
		before := p.MP
		d := assassinRegressionCast(t, s, p, UseSkill{Skill: 10213, At: &m.Pos})
		if before-p.MP != d.MPCost {
			t.Fatal("triple paid more than once")
		}
		want := 1
		if enabled {
			want = 3
		}
		if len(s.traps) != want {
			t.Fatal("placement cardinality", len(s.traps))
		}
		native, _ := s.skills.Get(10213, 1)
		radius := native.DirectEffects()[0].TrapRadius
		positions := map[float64]bool{}
		for _, tr := range s.traps {
			positions[tr.Pos.Y-m.Pos.Y] = true
			if tr.Pos.X != m.Pos.X {
				t.Fatal("side placement direction")
			}
		}
		if !positions[0] || enabled && (!positions[float64(radius)*.75] || !positions[-float64(radius)*.75]) {
			t.Fatal("native radius side spacing", positions, radius)
		}
		for _, tr := range s.traps {
			s.triggerAssassinTrap(tr, false)
		}
		damage, hits := assassinRegressionDamage(s, m.ID, 10213)
		if damage != int32(want)*200 || hits != want || len(s.traps) != 0 {
			t.Fatal("full overlapping payload or recursive children", damage, hits, len(s.traps))
		}
	}
}
func assassinCoverageFrost(t *testing.T) {
	for _, mode := range []string{"learned", "unlearned", "freeze-blocked", "poison-resisted", "foreign-damage"} {
		learned := domain.Learned{10216: 1}
		if mode != "unlearned" {
			learned[10211] = 1
		}
		s, p, m := nativeRepairFixture(t, domain.Assassin, learned, "CK-A18")
		s.rng = rand.New(meleeAuditZeroSource{})
		if mode == "freeze-blocked" {
			m.Status.Apply(domain.StatusDef{ID: 990030, DurationSec: 100, BlockHarmful: true}, s.tick, m.ID)
		}
		if mode == "poison-resisted" {
			m.Status.Apply(domain.StatusDef{ID: 990031, DurationSec: 100}, s.tick, m.ID)
			s.statusOverlay = domain.StatusOverlay{{Existing: 990031, Incoming: domain.StatusPoison}: domain.OverlayBlock}
		}
		assassinRegressionCast(t, s, p, UseSkill{Skill: 10216, At: &m.Pos})
		for _, tr := range s.traps {
			s.triggerAssassinTrap(tr, false)
		}
		freeze := m.Status.Has(domain.StatusFreeze)
		poison := m.Status.Has(domain.StatusPoison)
		if freeze != (mode != "freeze-blocked") || poison != (mode != "unlearned" && mode != "freeze-blocked" && mode != "poison-resisted") {
			t.Fatal("native freeze/learned/poison immunity", mode, freeze, poison)
		}
		if p.Status.Has(domain.StatusPoisonHit) {
			t.Fatal("borrowed poison activated owner enchantment")
		}
		if poison {
			hp := m.HP
			poisonTicks(s, 1)
			if m.HP >= hp || !m.Status.Has(domain.StatusFreeze) {
				t.Fatal("own poison broke freeze")
			}
			if mode == "foreign-damage" {
				other := assassinCoverageMob(s, 950, 150, 100)
				other.Stats = domain.Stats{MinAtk: 100, MaxAtk: 100}
				ev, raw, refl := s.resolveStrikeStatuses(other, m, &combat.HitTracker{}, combat.Blow{GuaranteedHit: true})
				s.afterStrikeStatuses(other, m, ev, raw, refl)
				if m.Status.Has(domain.StatusFreeze) {
					t.Fatal("unrelated damage did not break freeze")
				}
			}
		}
	}
}
func assassinCoverageFan(t *testing.T) {
	for _, weapon := range []int32{40, 150} {
		for _, enabled := range []bool{false, true} {
			s, p, m := assassinCoverageFixture(t, "CK-A19", enabled)
			p.Look.AtkDist = uint16(weapon)
			d := assassinCoverageSkill(10209)
			d.Dist = 80
			edge := assassinCoverageMob(s, 901, 130, 150)
			behind := assassinCoverageMob(s, 902, 70, 100)
			outer := assassinCoverageMob(s, 903, 240, 100)
			far := assassinCoverageMob(s, 904, 251, 100)
			s.legendaryPrepareSkill(p, &d, m.ID)
			ids := s.legendarySelectTargets(p, d, m.ID, []domain.EntityID{m.ID})
			assassinNativeHits(s, p, d, m.ID, ids)
			if m.MaxHP-m.HP != 100 || (edge.HP == 9900) != enabled || (outer.HP == 9900) != (enabled && weapon == 150) || behind.HP != 10000 || far.HP != 10000 {
				t.Fatal("120 degree fan/radius/full coefficient", edge.HP, outer.HP)
			}
			for j := 0; j < 8; j++ {
				assassinCoverageMob(s, domain.EntityID(910+j), 140+float64(j), 100)
			}
			ids = s.legendarySelectTargets(p, d, m.ID, []domain.EntityID{m.ID})
			want := 1
			if enabled {
				want = 10
				if weapon == 150 {
					want = 11
				}
			}
			if len(ids) != want || ids[0] != m.ID {
				t.Fatal("fan primary and four cap")
			}
		}
	}
}
func assassinCoverageBackstabGate(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, stealth := range []bool{false, true} {
			for _, advanced := range []bool{false, true} {
				s, p, m := assassinCoverageFixture(t, "CK-A20", enabled)
				d := assassinCoverageSkill(10209)
				d.RequiresInvisible = true
				s.skills = domain.SkillTable{{ID: 10209, Level: 1}: d}
				if advanced {
					s.skills[domain.SkillKey{ID: 10201, Level: 1}] = domain.SkillDef{ID: 10201, Level: 1, Prof: 3, Kind: domain.SkillPassive, PassiveModifiers: []domain.PassiveModifier{{Kind: domain.PassiveSkillDamagePct, TargetSkill: 10209, Value: 30}}}
				}
				if stealth {
					p.Status.Apply(domain.StatusDef{ID: domain.StatusStealth, DurationSec: 10, Invisible: true, StealthDamagePct: 50}, s.tick, p.ID)
				}
				s.onUseSkill(UseSkill{ID: p.ID, Skill: 10209, Target: m.ID})
				allowed := enabled || stealth
				if (len(s.casts) == 1) != allowed {
					t.Fatal("backstab stealth gate")
				}
				if !allowed {
					continue
				}
				s.stepCastsAt(s.now().Add(time.Minute), false)
				s.stepCastsAt(s.now().Add(2*time.Minute), false)
				want := int32(100)
				if advanced {
					want = 130
				}
				if enabled {
					want = want * 125 / 100
				}
				if stealth {
					want = want * 150 / 100
				}
				if got := m.MaxHP - m.HP; got != want {
					t.Fatalf("enabled%v stealth%v advanced%v damage%d want%d", enabled, stealth, advanced, got, want)
				}
			}
		}
	}
}
func assassinCoverageLeech(t *testing.T, id string, skill domain.SkillID) {
	for _, enabled := range []bool{false, true} {
		for _, clamp := range []bool{false, true} {
			s, p, m := assassinCoverageFixture(t, id, enabled)
			side := assassinCoverageMob(s, 901, 140, 100)
			p.HP = 5000
			if clamp {
				p.HP = 9999
				m.HP = 10
			}
			before := p.HP
			d := assassinCoverageSkill(skill)
			assassinNativeHits(s, p, d, m.ID, []domain.EntityID{m.ID, side.ID})
			want := int32(0)
			if enabled {
				want = 30
				if clamp {
					want = 1
				}
			}
			if p.HP-before != want {
				t.Fatal("effective per-target healing/overkill/HP cap", p.HP-before, want)
			}
		}
	}
}
func assassinCoverageStun(t *testing.T) {
	for _, immune := range []bool{false, true} {
		for _, enabled := range []bool{false, true} {
			s, p, m := assassinCoverageFixture(t, "CK-A23", enabled)
			if immune {
				m.Status.Apply(domain.StatusDef{ID: 990040, DurationSec: 20, BlockHarmful: true}, s.tick, m.ID)
			}
			s.statuses[domain.StatusKey{ID: domain.StatusStun, Level: 1}] = domain.StatusDef{ID: domain.StatusStun, Level: 1, DurationSec: 5, Control: domain.ControlStun, TypeKnown: true, Type: domain.StatusBad}
			d := assassinCoverageSkill(10219)
			d.Statuses = []domain.StatusApplication{{ID: domain.StatusStun, Level: 1, ChanceBP: 10000, ChanceExplicit: true, SourceSkill: 10219}}
			got := assassinCoverageHit(s, p, m, d)
			want := int32(100)
			if enabled {
				want = 135
				if immune {
					want = 295
				}
			}
			if got != want || m.Status.Has(domain.StatusStun) == immune {
				t.Fatal("stun damage/immunity compensation/status", got, want)
			}
		}
	}
	assassinCoverageCosts(t, "CK-A23", []domain.SkillID{10219}, 100, 6000)
}
func assassinCoverageCounter(t *testing.T, id string) {
	for _, enabled := range []bool{false, true} {
		s, p, m := assassinCoverageFixture(t, id, enabled)
		p.HP = 5000
		s.skills = domain.SkillTable{{ID: 10203, Level: 1}: {ID: 10203, Level: 1, Prof: 3, Kind: domain.SkillPassive, PassiveCounter: &domain.PassiveCounterAttack{ChanceBP: 10000, DamagePct: 100}}}
		side := assassinCoverageMob(s, 901, m.Pos.X+99, m.Pos.Y)
		far := assassinCoverageMob(s, 902, m.Pos.X+101, m.Pos.Y)
		// The native counter's qualifying lifesteal remains functional with A24.
		if id == "CK-A24" {
			p.Status.Apply(domain.StatusDef{ID: 990041, DurationSec: 10, LifeStealPct: 10, LifeStealChance: 100}, s.tick, p.ID)
		}
		at, ok := s.passiveCounterPending(p, m)
		if !ok {
			t.Fatal("counter native chance rejected")
		}
		s.attacks = append(s.attacks, at)
		s.tick++
		s.stepCombatAt(s.now(), false)
		want := int32(100)
		if enabled && id == "CK-A24" {
			want = 150
		}
		if m.MaxHP-m.HP != want || (side.HP == 9900) != (enabled && id == "CK-A25") || far.HP != 10000 {
			t.Fatal("counter coefficient/radius", m.HP, side.HP, far.HP)
		}
		heal := int32(0)
		if id == "CK-A24" {
			heal = want / 10
		}
		if enabled && id == "CK-D03" {
			heal = 15
		}
		if p.HP != 5000+heal {
			t.Fatal("counter healing", p.HP, heal)
		}
		if len(s.attacks) != 0 {
			t.Fatal("counter recursively queued")
		}
		native, _ := s.skills.Get(10203, 1)
		native.PassiveCounter = &domain.PassiveCounterAttack{ChanceBP: 0, DamagePct: 100}
		s.skills[domain.SkillKey{ID: 10203, Level: 1}] = native
		if _, ok := s.passiveCounterPending(p, m); ok {
			t.Fatal("counter chance changed")
		}
	}
	if id == "CK-D03" {
		assassinCoverageLeech(t, id, 10203)
	}
}
func assassinCoveragePalm(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		s, p, m := assassinCoverageFixture(t, "CK-A26", enabled)
		s.items[掉落树枝.ID] = 掉落树枝
		s.lootTable = map[domain.MonsterID][]domain.DropEntry{338: {{Item: 掉落树枝.ID, RatePct: 100}}}
		edge := assassinCoverageMob(s, 901, p.Pos.X+180, p.Pos.Y)
		far := assassinCoverageMob(s, 902, p.Pos.X+181, p.Pos.Y)
		d := assassinCoverageSkill(10212)
		d.Effects = []domain.SkillEffect{{Kind: domain.EffectStealItem, ChanceBP: 0, ChanceExplicit: true, TheftMaxSuccesses: 1}}
		before, side := m.Pos, edge.Pos
		assassinNativeHits(s, p, d, m.ID, []domain.EntityID{m.ID})
		want := int32(0)
		if enabled {
			want = 240
		}
		if m.MaxHP-m.HP != want || 10000-edge.HP != want || far.HP != 10000 || m.Pos != before || edge.Pos != side {
			t.Fatal("palm exact radius/no displacement", enabled, m.MaxHP-m.HP, 10000-edge.HP, far.HP, m.Pos, before, edge.Pos, side)
		}
		m.Monster.TheftSuccesses = 1
		hp := edge.HP
		assassinNativeHits(s, p, d, m.ID, []domain.EntityID{m.ID})
		if edge.HP != hp {
			t.Fatal("ineligible theft caused palm")
		}
	}
}
func assassinCoverageThrowRange(t *testing.T) {
	for _, skill := range []domain.SkillID{10205, 10206} {
		for _, enabled := range []bool{false, true} {
			s, p, m := assassinCoverageFixture(t, "CK-A27", enabled)
			d := assassinCoverageSkill(skill)
			d.Dist = 100
			d.Kind = domain.SkillArea
			d.Radius = 100
			d.Area = domain.SkillAreaTargeting{Shape: domain.SkillAreaCircle, Center: domain.SkillAreaCenterTarget, Radius: 100}
			m.Pos.X = 240
			s.aoi.Move(m, m.Pos)
			edge := assassinCoverageMob(s, 901, m.Pos.X+149, m.Pos.Y)
			far := assassinCoverageMob(s, 902, m.Pos.X+151, m.Pos.Y)
			s.skills = domain.SkillTable{{ID: skill, Level: 1}: d}
			s.onUseSkill(UseSkill{ID: p.ID, Skill: skill, Target: m.ID})
			if (len(s.casts) == 1) != enabled {
				t.Fatal("50% cast range did not reach actual admission")
			}
			if !enabled {
				continue
			}
			s.stepCastsAt(s.now().Add(time.Minute), false)
			s.stepCastsAt(s.now().Add(2*time.Minute), false)
			if edge.HP != 9900 || far.HP != 10000 {
				t.Fatal("50% area range actual hits", edge.HP, far.HP)
			}
		}
	}
}
func assassinCoveragePull(t *testing.T, id string) {
	for _, learned := range []bool{false, true} {
		for _, mode := range []string{"open", "near", "immune", "wall", "dead"} {
			for _, enabled := range []bool{false, true} {
				s, p, m := assassinCoverageFixture(t, id, enabled)
				if !learned {
					delete(p.Player.Char.Skills, 10226)
				}
				m.Pos.X = 300
				if mode == "near" {
					m.Pos.X = 130
				}
				s.aoi.Move(m, m.Pos)
				before := m.Pos.X
				if mode == "immune" {
					m.Status.Apply(domain.StatusDef{ID: 990050, DurationSec: 30, SuppressHarmful: true}, s.tick, m.ID)
				}
				if mode == "wall" {
					s.walkable = func(at domain.Pos) bool { return at.X >= 250 }
				}
				if mode == "dead" {
					m.HP = 10
				}
				d := assassinCoverageSkill(10225)
				d.Kind = domain.SkillArea
				d.Area = domain.SkillAreaTargeting{Shape: domain.SkillAreaCircle, Center: domain.SkillAreaCenterCaster, Radius: 200}
				far := assassinCoverageMob(s, 901, 390, 100)
				s.legendaryPrepareSkill(p, &d, m.ID)
				ids := s.resolveTargets(p, d, m.ID, nil)
				hp := m.HP
				assassinNativeHits(s, p, d, m.ID, ids)
				wantDamage := int32(100)
				if enabled && id == "CK-A28" {
					wantDamage = 145
				}
				if hp-m.HP != min(hp, wantDamage) {
					t.Fatal("pull changed native damage", hp-m.HP, wantDamage)
				}
				active := enabled && (id == "CK-A28" || learned)
				wantX := before
				if active && mode == "open" {
					wantX = 180
					if id == "CK-A30" {
						wantX = 140
					}
				}
				if active && mode == "wall" {
					wantX = 250
				}
				if m.Pos.X != wantX {
					t.Fatal("pull immunity/terrain/near/death/learning", id, learned, mode, enabled, m.Pos.X, wantX)
				}
				if (far.HP < far.MaxHP) != (enabled && id == "CK-A30" && learned) {
					t.Fatal("A30 expanded radius not used")
				}
			}
		}
	}
}

type assassinCoverageRandom struct {
	source rand.Source
	calls  int
}

func (r *assassinCoverageRandom) Int63() int64    { r.calls++; return r.source.Int63() }
func (r *assassinCoverageRandom) Seed(seed int64) { r.source.Seed(seed) }
func assassinCoverageDeath(t *testing.T) {
	deaths, survived := 0, 0
	for seed := int64(1); seed <= 48; seed++ {
		var results [2]bool
		var calls [2]int
		for n, enabled := range []bool{false, true} {
			s, p, m := assassinCoverageFixture(t, "CK-A29", enabled)
			r := &assassinCoverageRandom{source: rand.NewSource(seed)}
			s.rng = rand.New(r)
			d := assassinCoverageSkill(10210)
			d.Effects = []domain.SkillEffect{d.Effect, {Kind: domain.EffectInstantDeath, ChanceBP: 2500, ChanceExplicit: true}}
			s.legendaryPrepareSkill(p, &d, m.ID)
			assassinNativeHits(s, p, d, m.ID, []domain.EntityID{m.ID})
			results[n] = !m.Alive()
			calls[n] = r.calls
			if m.Alive() {
				want := int32(100)
				if enabled {
					want = 180
				}
				if m.MaxHP-m.HP != want {
					t.Fatal("direct assassination damage")
				}
			}
			_, hits := assassinRegressionDamage(s, m.ID, 10210)
			wantHits := 1
			if !m.Alive() {
				wantHits = 2
			}
			if hits != wantHits {
				t.Fatal("instant-death repeated damage", hits)
			}
		}
		if results[0] != results[1] || calls[0] != calls[1] {
			t.Fatal("instant-death chance or draw count changed", seed, results, calls)
		}
		if results[0] {
			deaths++
		} else {
			survived++
		}
	}
	if deaths == 0 || survived == 0 {
		t.Fatal("seed corpus did not exercise both instant-death outcomes")
	}
}
func assassinCoverageDefense(t *testing.T, id string) {
	for _, enabled := range []bool{false, true} {
		for _, active := range []bool{false, true} {
			for _, kind := range []string{"native", "dot"} {
				s, p, m := assassinCoverageFixture(t, id, enabled)
				p.Stats = domain.Stats{}
				m.Stats = domain.Stats{MinAtk: 1000, MaxAtk: 1000}
				b := combat.Blow{SkillID: 199931, GuaranteedHit: true}
				switch id {
				case "CK-D01":
					if active {
						incomingAssassinCondition(s, p, m, id)
					}
				case "CK-D04":
					if active {
						m.Status.Apply(domain.StatusDef{ID: domain.StatusPoison, ClassSetPoison: true, DurationSec: 10}, s.tick, p.ID)
					}
				case "CK-D06":
					b.SourceSkillRange = 100
					if active {
						b.SourceSkillRange = 101
					}
					m.Pos.X = p.Pos.X + 1
				case "CK-D07":
					if active {
						s.legendaryCastReleased(p, domain.SkillDef{ID: 10219}, m.ID, []domain.EntityID{m.ID})
						incomingAssassinCondition(s, p, m, id)
					}
				case "CK-D10":
					b.Trap = true
					if !active {
						delete(p.Player.Char.Skills, 10229)
					}
				}
				hp := p.HP
				incomingAssassinHit(s, p, m, kind, b)
				want := int32(1000)
				if enabled && active && (id != "CK-D04" || kind == "native") {
					want = 800
					if id == "CK-D01" || id == "CK-D07" {
						want = 650
					}
				}
				if hp-p.HP != want {
					t.Fatal("conditional damage consumer", id, enabled, active, kind, hp-p.HP, want)
				}
				if id == "CK-D01" {
					p.Status.Remove(domain.StatusStealth)
				}
				if id == "CK-D04" || id == "CK-D07" {
					m.Status = domain.NewStatusSet()
				}
				if id == "CK-D01" || id == "CK-D04" || id == "CK-D07" {
					if id == "CK-D07" {
						s.tick += domain.Ticks(8000)
					}
					hp = p.HP
					incomingAssassinHit(s, p, m, kind, b)
					if hp-p.HP != 1000 {
						t.Fatal("condition survived removal")
					}
				}
			}
		}
	}
	if id == "CK-D06" {
		assassinCoverageCosts(t, id, []domain.SkillID{10213}, 70, 10000)
	}
	if id == "CK-D07" {
		assassinCoverageCosts(t, id, []domain.SkillID{10219}, 100, 7000)
		s, p, m := assassinCoverageFixture(t, id, true)
		m.Status.Apply(domain.StatusDef{ID: 990061, DurationSec: 10, BlockHarmful: true}, s.tick, m.ID)
		if s.applyStatusDef(m, domain.StatusDef{ID: domain.StatusStun, Control: domain.ControlStun, TypeKnown: true, Type: domain.StatusBad, SourceSkillID: 10219, DurationSec: 10}, p.ID) {
			t.Fatal("immune stun committed")
		}
		m.Stats = domain.Stats{MinAtk: 1000, MaxAtk: 1000}
		hp := p.HP
		incomingAssassinHit(s, p, m, "native", combat.Blow{GuaranteedHit: true})
		if hp-p.HP != 1000 {
			t.Fatal("immune stun granted defense")
		}
	}
	if id == "CK-D04" {
		s, p, m := assassinCoverageFixture(t, id, true)
		m.Stats = domain.Stats{MinAtk: 1000, MaxAtk: 1000}
		m.Status.Apply(domain.StatusDef{ID: domain.StatusPoison, ClassSetPoison: true, DurationSec: 10}, s.tick, 77)
		hp := p.HP
		incomingAssassinHit(s, p, m, "native", combat.Blow{GuaranteedHit: true})
		if hp-p.HP != 1000 {
			t.Fatal("foreign poison granted defense")
		}
	}
	if id == "CK-D10" {
		for _, enabled := range []bool{false, true} {
			s, p, m := assassinCoverageFixture(t, id, enabled)
			s.skills = domain.SkillTable{{ID: 10229, Level: 1}: {ID: 10229, Level: 1, Prof: 3, Kind: domain.SkillPassive, PassiveModifiers: []domain.PassiveModifier{{Kind: domain.PassiveTrapHalfDamageChance, Value: 100}}}}
			m.Stats = domain.Stats{MinAtk: 1000, MaxAtk: 1000}
			hp := p.HP
			incomingAssassinHit(s, p, m, "native", combat.Blow{GuaranteedHit: true, Trap: true})
			want := int32(500)
			if enabled {
				want = 400
			}
			if hp-p.HP != want {
				t.Fatal("native probabilistic half removed", hp-p.HP, want)
			}
		}
	}
}
func assassinCoverageSmoke(t *testing.T, id string) {
	for _, enabled := range []bool{false, true} {
		s, p, m := assassinCoverageFixture(t, id, enabled)
		d := assassinCoverageSkill(10224)
		d.Effect = domain.SkillEffect{}
		d.Dist = 100
		d.Radius = 0
		near := assassinCoverageMob(s, 901, m.Pos.X+99, m.Pos.Y)
		expanded := assassinCoverageMob(s, 902, m.Pos.X+129, m.Pos.Y)
		far := assassinCoverageMob(s, 903, m.Pos.X+131, m.Pos.Y)
		sd := domain.StatusDef{ID: domain.StatusBlind, Level: 1, SourceSkillID: 10224, DurationSec: 10, TypeKnown: true, Type: domain.StatusBad, Affixes: []domain.Affix{{Attr: domain.AttrHit, Mode: domain.ModePercent, Value: -20}, {Attr: domain.AttrMAtk, Mode: domain.ModePercent, Value: -30}}}
		s.statuses[domain.StatusKey{ID: sd.ID, Level: 1}] = sd
		d.Statuses = []domain.StatusApplication{{ID: sd.ID, Level: 1, SourceSkill: 10224, ChanceBP: 10000, ChanceExplicit: true}}
		s.legendaryPrepareSkill(p, &d, m.ID)
		ids := s.resolveTargets(p, d, m.ID, nil)
		assassinNativeHits(s, p, d, m.ID, ids)
		if !m.Status.Has(domain.StatusBlind) || near.Status.Has(domain.StatusBlind) != enabled || expanded.Status.Has(domain.StatusBlind) != (enabled && id == "CK-U02") || far.Status.Has(domain.StatusBlind) {
			t.Fatal("smoke real target radius")
		}
		a := assassinCoverageActive(t, m, domain.StatusBlind)
		hit, magic := int32(0), int32(0)
		for _, af := range a.Def.Affixes {
			if af.Attr == domain.AttrHit {
				hit += af.Value
			}
			if af.Attr == domain.AttrMAtk {
				magic += af.Value
			}
		}
		if hit != -20 || magic != -30 {
			t.Fatal("native blind/magic debuffs lost")
		}
		if id == "CK-D02" {
			m.Stats = domain.Stats{MinAtk: 1000, MaxAtk: 1000}
			hp := p.HP
			incomingAssassinHit(s, p, m, "native", combat.Blow{GuaranteedHit: true})
			want := int32(1000)
			if enabled {
				want = 800
			}
			if hp-p.HP != want {
				t.Fatal("own smoke mitigation", enabled, hp-p.HP, want, a)
			}
			m.Status.Remove(domain.StatusBlind)
			hp = p.HP
			incomingAssassinHit(s, p, m, "native", combat.Blow{GuaranteedHit: true})
			if hp-p.HP != 1000 {
				t.Fatal("smoke removal mitigation lingered")
			}
		}
	}
	if id == "CK-U02" {
		for _, enabled := range []bool{false, true} {
			s, p, m := assassinCoverageFixture(t, id, enabled)
			d := assassinCoverageSkill(10224)
			d.Dist = 100
			m.Pos.X = 240
			s.aoi.Move(m, m.Pos)
			s.skills = domain.SkillTable{{ID: d.ID, Level: 1}: d}
			s.onUseSkill(UseSkill{ID: p.ID, Skill: d.ID, Target: m.ID})
			if (len(s.casts) == 1) != enabled {
				t.Fatal("smoke cast distance")
			}
		}
	}
}
func assassinCoverageControl(t *testing.T) {
	for _, control := range []domain.StatusDef{{ID: domain.StatusStun, Control: domain.ControlStun}, {ID: domain.StatusFreeze, Control: domain.ControlStun}, {ID: 1034, BlockMove: true}} {
		for _, enabled := range []bool{false, true} {
			s, p, _ := assassinCoverageFixture(t, "CK-D05", enabled)
			control.DurationSec = 10
			control.TypeKnown = true
			control.Type = domain.StatusBad
			s.applyStatusDef(p, control, 999)
			a := assassinCoverageActive(t, p, control.ID)
			duration := 10000
			if enabled {
				duration = 7000
			}
			if a.ExpireAt-s.tick != domain.Ticks(duration) {
				t.Fatal("control duration", control.ID, a.ExpireAt-s.tick)
			}
			s.tick = a.ExpireAt - 1
			s.stepStatus()
			if !p.Status.Has(control.ID) {
				t.Fatal("control removed early")
			}
			s.tick++
			s.stepStatus()
			if p.Status.Has(control.ID) {
				t.Fatal("shortened control did not expire")
			}
		}
	}
	s, p, _ := assassinCoverageFixture(t, "CK-D05", false)
	d := domain.StatusDef{ID: domain.StatusStun, Control: domain.ControlStun, DurationSec: 10}
	s.applyStatusDef(p, d, 999)
	before := assassinCoverageActive(t, p, d.ID).ExpireAt
	assassinAddPower(s, p, "CK-D05", domain.SlotBody)
	s.refreshStats(p)
	if a := assassinCoverageActive(t, p, d.ID); a.ExpireAt != before {
		t.Fatal("equipping power retroactively shortened existing control")
	}
}
func assassinCoverageCleanse(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		s, p, _ := assassinCoverageFixture(t, "CK-D08", enabled)
		poison := domain.StatusDef{ID: domain.StatusPoison, Level: 21, ClassSetPoison: true, DurationSec: 10, IntervalSec: 2, Affixes: []domain.Affix{{Attr: domain.AttrHPRegen, Value: -100}}}
		p.Status.Apply(poison, s.tick, 900)
		hp := p.HP
		poisonTicks(s, 1)
		want := int32(100)
		if enabled {
			want = 70
		}
		if hp-p.HP != want {
			t.Fatal("poison 30% reduction", hp-p.HP)
		}
		for _, source := range []domain.EntityID{901, 902} {
			p.Status.ApplyWithOverlay(poison, s.tick, source, domain.StatusOverlay{{Existing: domain.StatusPoison, Incoming: domain.StatusPoison}: domain.OverlayCoexist})
		}
		permanent := poison
		permanent.Permanent = true
		p.Status.ApplyWithOverlay(permanent, s.tick, 903, domain.StatusOverlay{{Existing: domain.StatusPoison, Incoming: domain.StatusPoison}: domain.OverlayCoexist})
		before := 0
		p.Status.EachEffective(func(a domain.Active) {
			if a.Def.ID == domain.StatusPoison {
				before++
			}
		})
		if before != 4 {
			t.Fatalf("cleanse fixture expected four source poisons, got%d", before)
		}
		s.legendaryCastReleased(p, domain.SkillDef{ID: 10211}, p.ID, []domain.EntityID{p.ID}, 0)
		after, permanentCount := 0, 0
		p.Status.EachEffective(func(a domain.Active) {
			if a.Def.ID == domain.StatusPoison {
				after++
				if a.Def.Permanent {
					permanentCount++
				}
			}
		})
		wantCount := 4
		if enabled {
			wantCount = 2
		}
		if after != wantCount || permanentCount != 1 {
			t.Fatal("cleanse cap or non-clearable poison", after, permanentCount)
		}
	}
}
func assassinCoverageMaxHP(t *testing.T) {
	var maxima [2]int32
	for n, enabled := range []bool{false, true} {
		s, p, _ := assassinCoverageFixture(t, "CK-D09", enabled)
		s.refreshStats(p)
		maxima[n] = p.MaxHP
	}
	if maxima[0] <= 0 || maxima[1] != maxima[0]*120/100 {
		t.Fatal("actual maximum HP", maxima)
	}
	assassinCoverageCosts(t, "CK-D09", []domain.SkillID{10208}, 70, 10000)
}
func assassinCoverageRefund(t *testing.T) {
	for _, cost := range []int32{20, 100} {
		for _, enabled := range []bool{false, true} {
			s, p, m := assassinCoverageFixture(t, "CK-U01", enabled)
			d := assassinCoverageSkill(10208)
			d.MPCost = cost
			before := p.MP
			assassinCoverageCast(t, s, p, m, d)
			paid := cost
			refund := int32(0)
			if enabled {
				paid = cost * 70 / 100
				refund = 0
			}
			if p.MP != before-paid+refund {
				t.Fatal("native reduced payment and bounded refund", cost, p.MP, before-paid+refund)
			}
		}
	}
}
func assassinCoverageDisarm(t *testing.T) {
	for _, chance := range []int32{0, 7500, 10000} {
		for _, enabled := range []bool{false, true} {
			s, p, m := assassinCoverageFixture(t, "CK-U03", enabled)
			s.rng = rand.New(meleeAuditZeroSource{})
			ids := []domain.EntityID{}
			for _, x := range []float64{150, 299, 301} {
				s.placeTrap(m, domain.SkillDef{ID: 10213}, domain.Pos{MapID: p.Pos.MapID, X: x, Y: 100}, 50, 100)
				for id, tr := range s.traps {
					if tr.Pos.X == x {
						ids = append(ids, id)
						tr.Trap.VisibleTo[p.ID] = struct{}{}
					}
				}
			}
			s.disarmTrap(p, ids[0], chance)
			success := chance > 0 || enabled
			if (s.traps[ids[0]] == nil) != success || (s.traps[ids[1]] == nil) != (enabled && success) || s.traps[ids[2]] == nil {
				t.Fatal("disarm chance +25pp/cap/150 radius", chance, enabled, len(s.traps))
			}
		}
	}
	assassinCoverageCosts(t, "CK-U03", []domain.SkillID{10218}, 0, 10000)
}
func assassinCoverageDetect(t *testing.T) {
	// A single-power native detector fixture. Repeated seeded native detection
	// rolls make the radius boundary observable without changing its probability.
	for _, enabled := range []bool{false, true} {
		powers := []string{}
		if enabled {
			powers = []string{"CK-U04"}
		}
		s, p, m := nativeRepairFixture(t, domain.Assassin, domain.Learned{10228: 1}, powers...)
		for _, dist := range []float64{199, 250, 301} {
			s.placeTrap(m, domain.SkillDef{ID: 10213}, domain.Pos{MapID: p.Pos.MapID, X: p.Pos.X + dist, Y: p.Pos.Y}, 50, 100)
		}
		s.refreshStats(p)
		baseSpeed := p.Stats.MoveSpeed
		d := assassinRegressionCast(t, s, p, UseSkill{Skill: 10228})
		wantSpeed := baseSpeed
		if enabled {
			wantSpeed = baseSpeed * 160 / 100
		}
		if p.Stats.MoveSpeed != wantSpeed {
			t.Fatal("native detection release failed speed window", p.Stats.MoveSpeed, wantSpeed)
		}
		for i := 0; i < 50; i++ {
			s.detectTraps(p, d.Radius, d.DirectEffects()[0].ChanceBP)
		}
		want := float64(200)
		if enabled {
			want = 300
		}
		for _, tr := range s.traps {
			if tr.Trap.VisibleToPlayer(p.ID) != (tr.Pos.X-p.Pos.X <= want) {
				t.Fatal("actual detection radius")
			}
		}
	}
	var speed [2]int32
	for n, enabled := range []bool{false, true} {
		s, p, _ := assassinCoverageFixture(t, "CK-U04", enabled)
		s.refreshStats(p)
		speed[n] = p.Stats.MoveSpeed
	}
	if speed[0] <= 0 || speed[1] != speed[0] {
		t.Fatal("unconditional actual move speed", speed)
	}
}
func assassinCoverageRecovery(t *testing.T) {
	for _, learned := range []bool{false, true} {
		for _, skill := range []domain.SkillID{10221, 10205, 10206} {
			for _, enabled := range []bool{false, true} {
				s, p, m := assassinCoverageFixture(t, "CK-U05", enabled)
				if !learned {
					delete(p.Player.Char.Skills, 10222)
				}
				d := assassinCoverageSkill(skill)
				d.MPCost = 100
				before := p.MP
				assassinCoverageCast(t, s, p, m, d)
				want := int32(100)
				if enabled && learned {
					want = 55
				}
				if before-p.MP != want {
					t.Fatal("recovery learned gate/cost", skill, learned, enabled, before-p.MP)
				}
			}
		}
	}
	for _, enabled := range []bool{false, true} {
		s, p, _ := assassinCoverageFixture(t, "CK-U05", enabled)
		s.skills = domain.SkillTable{{ID: 10222, Level: 1}: {ID: 10222, Level: 1, Prof: 3, Kind: domain.SkillPassive, PassiveModifiers: []domain.PassiveModifier{{Kind: domain.PassiveAttackDurabilityReductionPct, Value: 50}}}}
		st := p.Player.Worn.At(domain.SlotWeapon)
		item := s.items[st.Item]
		item.Equip.AttackDurabilityCostRaw = 100
		s.items[st.Item] = item
		s.consumeAttackDurability(p)
		got := p.Player.Worn.At(domain.SlotWeapon)
		if got.Durability != st.Durability || got.DurabilityWearRaw != 50 {
			t.Fatal("native durability saving changed", got.Durability, got.DurabilityWearRaw)
		}
	}
}
func assassinCoverageCooldown(t *testing.T) {
	for _, fatal := range []bool{false, true} {
		for _, n := range []int{1, 3} {
			for _, enabled := range []bool{false, true} {
				s, p, m := assassinCoverageFixture(t, "CK-U06", enabled)
				p.Player.StartSkillCooldown(10208, s.tick, 100)
				ids := []domain.EntityID{m.ID}
				for j := 1; j < n; j++ {
					v := assassinCoverageMob(s, domain.EntityID(900+j), 140+float64(j), 100)
					ids = append(ids, v.ID)
				}
				if fatal {
					s.entities[ids[len(ids)-1]].HP = 10
				}
				d := assassinCoverageSkill(10209)
				assassinNativeHits(s, p, d, m.ID, ids)
				want := domain.Tick(100)
				if enabled {
					want = 65
					if fatal {
						want = 20
					}
				}
				if !p.Player.SkillReadyAt(10208, s.tick+want) || p.Player.SkillReadyAt(10208, s.tick+want-1) {
					t.Fatal("once-per-root hit/kill cooldown", fatal, n, enabled)
				}
			}
		}
	}
}
func assassinCoverageStealthBudget(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		s, p, m := assassinCoverageFixture(t, "CK-U08", enabled)
		stealth := domain.StatusDef{ID: domain.StatusStealth, DurationSec: 10, Invisible: true, SourceSkillID: 10208, BreakOnAttack: true, BreakOnDamage: true}
		s.applyStatusDef(p, stealth, p.ID)
		want := 10000
		if enabled {
			want = 15000
		}
		if assassinCoverageActive(t, p, domain.StatusStealth).ExpireAt-s.tick != domain.Ticks(want) {
			t.Fatal("stealth duration")
		}
		for n := 0; n < 3; n++ {
			d := assassinCoverageSkill(10213)
			s.legendaryPrepareSkill(p, &d, m.ID)
			s.prepareSkillStatuses(p, &d)
			s.legendaryCasterAccepted(p, d, m.ID, []domain.EntityID{m.ID}, 0)
			if entityInvisible(p) != enabled {
				t.Fatal("stealth trap budget", enabled, n)
			}
		}
		s.applyStatusDef(p, stealth, p.ID)
		s.prepareBasicAttackStatuses(p, &combat.Blow{})
		if entityInvisible(p) {
			t.Fatal("ordinary attack did not break stealth")
		}
		s.applyStatusDef(p, stealth, p.ID)
		m.Stats = domain.Stats{MinAtk: 100, MaxAtk: 100}
		ev, raw, refl := s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, combat.Blow{GuaranteedHit: true})
		s.afterStrikeStatuses(m, p, ev, raw, refl)
		if entityInvisible(p) {
			t.Fatal("incoming damage did not break stealth")
		}
	}
}
func assassinCoverageStealthSpeed(t *testing.T) {
	for _, nativePct := range []int32{-20, 20} {
		for _, enabled := range []bool{false, true} {
			s, p, _ := assassinCoverageFixture(t, "CK-U10", enabled)
			s.refreshStats(p)
			base := p.Stats.MoveSpeed
			d := domain.StatusDef{ID: domain.StatusStealth, Invisible: true, DurationSec: 10, SourceSkillID: 10208, Affixes: []domain.Affix{{Attr: domain.AttrMoveSpeed, Mode: domain.ModePercent, Value: nativePct}}}
			s.applyStatusDef(p, d, p.ID)
			wantPct := nativePct
			if enabled {
				wantPct += 40
			}
			if p.Stats.MoveSpeed != base*(100+wantPct)/100 {
				t.Fatal("actual native + extra stealth speed", base, p.Stats.MoveSpeed, wantPct)
			}
			removed := p.Status.RemoveWhere(func(d domain.StatusDef) bool { return d.ID == domain.StatusStealth })
			s.statusesRemoved(p, removed)
			if p.Stats.MoveSpeed != base {
				t.Fatal("stealth speed remained after exit")
			}
		}
	}
}

func assassinCoverageCrit(t *testing.T) {
	for _, bonus := range []int32{0, 1500} {
		for _, enabled := range []bool{false, true} {
			s, p, m := assassinCoverageFixture(t, "CK-A02", enabled)
			st := p.Player.Worn.At(domain.SlotWeapon)
			item := s.items[st.Item]
			item.Equip.Suit = &domain.EquipmentSuit{ID: 1, EnhancedClass: 1, Groups: [][]domain.ItemID{{st.Item}}, Bonuses: []domain.EquipmentSuitBonus{{Pieces: 1, Probability: 100, Affix: domain.Affix{Attr: domain.AttrSuitPhysicalCritDamageBP, Value: bonus}}}}
			s.items[st.Item] = item
			s.invalidateCombatAttributes(p)
			p.Stats.CritRate = 0
			d := assassinCoverageSkill(10221)
			got := assassinCoverageHit(s, p, m, d)
			want := int32(100)
			if enabled {
				want = 200 + bonus/100
			}
			if got != want {
				t.Fatal("native critical multiplier was replaced", got, want)
			}
		}
	}
}

// Legal combinations are deliberately outside the 50-single-power denominator.
func TestAssassinUnitCoverageCombinations(t *testing.T) {
	t.Run("A13_A11_A12_instant_triple", TestLegendaryAssassinSynergyTripleInstantBomb)
	t.Run("A05_A27_A04_throw", TestAssassinRegressionThrowFinalSplashNativeCast)
	t.Run("A28_A30_pull", TestAssassinFinalReviewPullLearningAndGeometryCombinations)
	t.Run("A06_A27_A07_circle", func(t *testing.T) {
		s, p, m := nativeRepairFixture(t, domain.Assassin, domain.Learned{10206: 1}, "CK-A06", "CK-A27", "CK-A07")
		p.Stats = domain.Stats{MinAtk: 100, MaxAtk: 100, Hit: 100000}
		back := assassinCoverageMob(s, 901, -69, 100)
		out := assassinCoverageMob(s, 902, -71, 100)
		d := assassinCoverageSkill(10206)
		d.Kind = domain.SkillArea
		d.Radius = 100
		d.Area = domain.SkillAreaTargeting{Shape: domain.SkillAreaCone, Center: domain.SkillAreaCenterCaster, Radius: 100, Angle: 60}
		s.legendaryPrepareSkill(p, &d, m.ID)
		ids := s.resolveTargets(p, d, m.ID, nil)
		assassinNativeHits(s, p, d, m.ID, ids)
		if m.MaxHP-m.HP != 200 || back.HP != 9850 || out.HP != 10000 {
			t.Fatal("additive 20%+50% transformed circle or primary classification", m.HP, back.HP, out.HP)
		}
	})
	t.Run("U08_A13_triple_one_charge", func(t *testing.T) {
		s, p, m := nativeRepairFixture(t, domain.Assassin, domain.Learned{10208: 1, 10213: 1}, "CK-U08", "CK-A13")
		assassinRegressionCast(t, s, p, UseSkill{Skill: 10208, Target: p.ID})
		for i := 0; i < 3; i++ {
			d := assassinRegressionCast(t, s, p, UseSkill{Skill: 10213, At: &m.Pos})
			if len(s.traps) != 3 {
				t.Fatal("triple placement cardinality")
			}
			if !entityInvisible(p) {
				t.Fatal("one cast consumed more than one stealth opportunity", i)
			}
			if i < 2 {
				a := assassinCoverageActive(t, p, domain.StatusStealth)
				if a.Def.LegendaryStealthTrapCharges != 0 {
					t.Fatal("triple consumed wrong budget")
				}
			}
			for id := range s.traps {
				s.removeTrap(id)
			}
			s.tick += domain.Ticks(int(d.CooldownMS))
		}
	})
	t.Run("A19_A20_A21_fan", func(t *testing.T) {
		s, p, m := nativeRepairFixture(t, domain.Assassin, domain.Learned{10209: 1}, "CK-A19", "CK-A20", "CK-A21")
		side := assassinCoverageMob(s, 901, 140, 110)
		d := assassinCoverageSkill(10209)
		d.RequiresInvisible = true
		d.Dist = 80
		s.legendaryPrepareSkill(p, &d, m.ID)
		if d.RequiresInvisible {
			t.Fatal("no-stealth gate")
		}
		ids := s.legendarySelectTargets(p, d, m.ID, []domain.EntityID{m.ID})
		assassinNativeHits(s, p, d, m.ID, ids)
		if m.MaxHP-m.HP != 200 || side.HP != 9875 {
			t.Fatal("fan primary-only additive bonus", m.HP, side.HP)
		}
	})
}

func TestAssassinUnitCoverageImmunityBoundaries(t *testing.T) {
	for _, id := range []string{"CK-A26", "CK-A28", "CK-A30"} {
		t.Run(id, func(t *testing.T) {
			s, p, m := assassinCoverageFixture(t, id, true)
			skill := domain.SkillID(10225)
			if id == "CK-A26" {
				skill = 10212
				s.items[掉落树枝.ID] = 掉落树枝
				s.lootTable = map[domain.MonsterID][]domain.DropEntry{338: {{Item: 掉落树枝.ID, RatePct: 100}}}
			}
			m.Monster.Kind = domain.MonsterBoss
			s.defs = spawn.MapDefs{m.Monster.TypeID: {ID: m.Monster.TypeID, Kind: domain.MonsterBoss, ImmuneSkills: map[domain.SkillID]struct{}{skill: {}}}}
			m.Pos.X = 250
			s.aoi.Move(m, m.Pos)
			d := assassinCoverageSkill(skill)
			if skill == 10212 {
				d.Effects = []domain.SkillEffect{{Kind: domain.EffectStealItem, ChanceBP: 10000, ChanceExplicit: true, TheftMaxSuccesses: 1}}
			}
			before := m.Pos
			hp := m.HP
			assassinCoverageHit(s, p, m, d)
			if m.Pos != before || m.HP != hp || len(s.ground) != 0 {
				t.Fatal("boss skill immunity bypassed")
			}
		})
	}
}

func TestAssassinUnitCoverageAdditional(t *testing.T) {
	t.Run("CK-A10_basic_poison_probability", func(t *testing.T) {
		successes, failures := 0, 0
		for seed := int64(1); seed <= 32; seed++ {
			var results [2]bool
			for n, enabled := range []bool{false, true} {
				s, p, m := assassinCoverageFixture(t, "CK-A10", enabled)
				s.rng = rand.New(rand.NewSource(seed))
				p.Status.Apply(domain.StatusDef{ID: domain.StatusPoisonHit, Level: 1, DurationSec: 120, PoisonOnHitChance: 25, PoisonOnHitLevel: 21, SourceSkillID: 10211, SourceEffectHitKnown: true}, s.tick, p.ID)
				s.statuses[domain.StatusKey{ID: domain.StatusPoison, Level: 21}] = domain.StatusDef{ID: domain.StatusPoison, Level: 21, DurationSec: 10, IntervalSec: 2, Affixes: []domain.Affix{{Attr: domain.AttrHPRegen, Value: -10}}}
				ev, raw, refl := s.resolveStrikeStatuses(p, m, &combat.HitTracker{}, combat.Blow{GuaranteedHit: true})
				s.afterStrikeStatuses(p, m, ev, raw, refl)
				results[n] = m.Status.Has(domain.StatusPoison)
			}
			if results[0] != results[1] {
				t.Fatal("A10 changed ordinary-attack poison outcome", seed, results)
			}
			if results[0] {
				successes++
			} else {
				failures++
			}
		}
		if successes == 0 || failures == 0 {
			t.Fatal("ordinary poison corpus did not exercise success and failure")
		}
	})
	t.Run("CK-U03_exact_probability", func(t *testing.T) {
		for _, base := range []int32{0, 1000, 7499, 7500, 9000} {
			for seed := int64(1); seed <= 24; seed++ {
				for _, enabled := range []bool{false, true} {
					s, p, m := assassinCoverageFixture(t, "CK-U03", enabled)
					s.placeTrap(m, domain.SkillDef{ID: 10213}, m.Pos, 50, 100)
					var target domain.EntityID
					for id, tr := range s.traps {
						target = id
						tr.Trap.VisibleTo[p.ID] = struct{}{}
					}
					s.rng = rand.New(rand.NewSource(seed))
					effective := base
					if enabled {
						effective = min(int32(10000), base+2500)
					}
					want := effective >= 10000 || effective > 0 && int32(rand.New(rand.NewSource(seed)).Intn(10000)) < effective
					s.disarmTrap(p, target, base)
					if (s.traps[target] == nil) != want {
						t.Fatal("disarm not exact +2500BP", base, seed, enabled)
					}
				}
			}
		}
	})
	for _, id := range []string{"CK-A28", "CK-A30"} {
		t.Run(id+"_native_knockback_order", func(t *testing.T) {
			for _, enabled := range []bool{false, true} {
				s, p, m := assassinCoverageFixture(t, id, enabled)
				m.Pos.X = 300
				s.aoi.Move(m, m.Pos)
				d := assassinCoverageSkill(10225)
				d.Effects = []domain.SkillEffect{d.Effect, {Kind: domain.EffectKnockback, ChanceBP: 10000, ChanceExplicit: true, KnockbackDistance: 120}}
				s.legendaryPrepareSkill(p, &d, m.ID)
				s.outbox = nil
				assassinNativeHits(s, p, d, m.ID, []domain.EntityID{m.ID})
				wantX, wantMoves := float64(420), 1
				if enabled {
					wantX, wantMoves = 300, 2
					if id == "CK-A30" {
						wantX, wantMoves = 140, 1
					}
				}
				moves := 0
				for _, o := range s.outbox {
					if ev, ok := o.ev.(event.EntityMoved); ok && ev.ID == m.ID {
						moves++
					}
				}
				if m.Pos.X != wantX || moves != wantMoves {
					t.Fatal("native knockback/pull order", id, enabled, m.Pos.X, moves, wantX, wantMoves)
				}
			}
		})
	}
}

func TestAssassinUnitCoverageCounterProbability(t *testing.T) {
	successes, failures := 0, 0
	for seed := int64(1); seed <= 32; seed++ {
		var results [2]bool
		for n, enabled := range []bool{false, true} {
			s, p, m := assassinCoverageFixture(t, "CK-A25", enabled)
			s.skills = domain.SkillTable{{ID: 10203, Level: 1}: {ID: 10203, Level: 1, Prof: 3, Kind: domain.SkillPassive, PassiveCounter: &domain.PassiveCounterAttack{ChanceBP: 5000, DamagePct: 100}}}
			s.rng = rand.New(rand.NewSource(seed))
			_, results[n] = s.passiveCounterPending(p, m)
		}
		if results[0] != results[1] {
			t.Fatal("A25 changed native probability", seed, results)
		}
		if results[0] {
			successes++
		} else {
			failures++
		}
	}
	if successes == 0 || failures == 0 {
		t.Fatal("native probability corpus lacks both outcomes")
	}
}
func TestAssassinUnitCoveragePullCombinationIndependentEffects(t *testing.T) {
	for _, learned := range []bool{false, true} {
		for _, immune := range []bool{false, true} {
			for _, powers := range [][]string{nil, {"CK-A28"}, {"CK-A30"}, {"CK-A28", "CK-A30"}, {"CK-A30", "CK-A28"}} {
				t.Run(fmt.Sprintf("learned=%v/immune=%v/powers=%v", learned, immune, powers), func(t *testing.T) {
					skills := domain.Learned{10225: 1}
					if learned {
						skills[10226] = 1
					}
					s, p, m := nativeRepairFixture(t, domain.Assassin, skills, powers...)
					m.Pos.X = 300
					s.aoi.Move(m, m.Pos)
					if immune {
						m.Status.Apply(domain.StatusDef{ID: 9999, TypeKnown: true, Type: domain.StatusGood, DurationSec: 60, SuppressHarmful: true}, s.tick, m.ID)
					}
					d := assassinCoverageSkill(10225)
					if learned {
						d.Effects = []domain.SkillEffect{d.Effect, {Kind: domain.EffectKnockback, ChanceBP: 10000, ChanceExplicit: true, KnockbackDistance: 120}}
					}
					s.legendaryPrepareSkill(p, &d, m.ID)
					s.outbox = nil
					assassinNativeHits(s, p, d, m.ID, []domain.EntityID{m.ID})
					hasLimited, hasGather := false, false
					for _, power := range powers {
						hasLimited = hasLimited || power == "CK-A28"
						hasGather = hasGather || power == "CK-A30"
					}
					// Independent oracle: learned native knockback adds120 unless
					// A30 transforms it; A28 pulls120, then A30 pulls to distance40.
					wantX := float64(300)
					var wantMoves []float64
					if !immune {
						if learned && !hasGather {
							wantX += 120
							wantMoves = append(wantMoves, wantX)
						}
						if hasLimited {
							wantX = max(p.Pos.X+40, wantX-120)
							wantMoves = append(wantMoves, wantX)
						}
						if hasGather && learned && wantX > p.Pos.X+40 {
							wantX = p.Pos.X + 40
							wantMoves = append(wantMoves, wantX)
						}
					}
					var gotMoves []float64
					for _, o := range s.outbox {
						if ev, ok := o.ev.(event.EntityMoved); ok && ev.ID == m.ID {
							gotMoves = append(gotMoves, ev.To.X)
						}
					}
					if len(gotMoves) != len(wantMoves) {
						t.Fatalf("trajectory %v want %v", gotMoves, wantMoves)
					}
					for i := range wantMoves {
						if gotMoves[i] != wantMoves[i] {
							t.Fatalf("trajectory %v want %v", gotMoves, wantMoves)
						}
					}
					wantDamage := int32(100)
					if hasLimited {
						wantDamage = 145
					}
					if m.Pos.X != wantX || m.MaxHP-m.HP != wantDamage {
						t.Fatalf("position/damage=(%v,%d) want(%v,%d)", m.Pos.X, m.MaxHP-m.HP, wantX, wantDamage)
					}
				})
			}
		}
	}
}
