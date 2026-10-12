package scene

import (
	"math/rand"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

func sourcePetFixtureV5(s *Scene, p *entity.Entity, chance int32) *domain.PetInstance {
	inst := &domain.PetInstance{ID: 7, Base: domain.Base{INT: 20}, Skills: []domain.PetSkill{{ID: 13001, Level: 1}}}
	s.petSkills = domain.PetSkillTable{13001: {ID: 13001, MaxLevel: 50, Effect: &domain.PetSkillEffect{Kind: "magic_extra", SourceAttr: domain.AttrINT, SourceStep: 1, Value: 1, ChanceBP: chance}}}
	pet := &entity.Entity{ID: 800, Kind: domain.KindPet, HP: 100, MaxHP: 100, Pet: &entity.Pet{Owner: p.ID, Inst: inst}}
	s.entities[pet.ID] = pet
	p.Player.Pet = pet.ID
	return inst
}
func sourceStatusFixtureV5(s *Scene, p *entity.Entity, flat int32, invisible bool) {
	p.Status = domain.NewStatusSet()
	p.Status.Apply(domain.StatusDef{ID: 999999, Level: 1, DurationSec: 100, OutgoingMagicDamageFlat: flat, Invisible: invisible}, s.tick, p.ID)
}
func sourceAcceptedCastV5(t *testing.T, s *Scene, p *entity.Entity, def domain.SkillDef, target domain.EntityID) func() {
	t.Helper()
	clock := time.Unix(1700000000, 0)
	s.now = func() time.Time { return clock }
	p.Player.Char.Skills = domain.Learned{def.ID: 1}
	s.skills = skillTable(def)
	s.onUseSkill(UseSkill{ID: p.ID, Skill: def.ID, Target: target})
	if len(s.casts) != 1 {
		t.Fatal("cast not accepted", def.ID)
	}
	return func() { clock = clock.Add(time.Second); s.stepCastsAt(clock, false) }
}

func TestCombatSourceV5AcceptedGearStatusPetAndSourceConditions(t *testing.T) {
	for _, initial := range []bool{false, true} {
		t.Run(map[bool]string{false: "equip_after_accept", true: "unequip_after_accept"}[initial], func(t *testing.T) {
			s, p, tr := uniqueTestGear(t, "healthy_assault")
			worn := p.Player.Worn
			if !initial {
				p.Player.Worn = domain.NewEquipSet()
			}
			p.Stats = domain.Stats{MAtk: 100}
			p.HP, p.MaxHP = 1000, 1000
			sourceStatusFixtureV5(s, p, 40, false)
			pet := sourcePetFixtureV5(s, p, 10000)
			m := putMonster(s, 900, 10000, domain.Stats{}, 110, 100)
			id := domain.SkillID(10401)
			if tr.SkillID != 0 {
				id = tr.SkillID
			}
			def := domain.SkillDef{ID: id, Level: 1, Kind: domain.SkillSingle, HurtType: domain.HurtFire, TargetEnemy: true, Dist: 450, PrepareMS: 100, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100}}
			x := domain.EvaluateUniqueTraits(s.uniqueTraits(p), domain.UniqueContext{SkillID: id, HPPercent: 100, MPPercent: 100, TargetHPPercent: 100, Magic: true, Skill: true, HurtType: domain.HurtFire})
			want := domain.ApplyUniqueBP(160, x.DamageBP)
			finish := sourceAcceptedCastV5(t, s, p, def, m.ID)
			if initial {
				p.Player.Worn = domain.NewEquipSet()
				p.Status = domain.NewStatusSet()
				delete(s.entities, p.Player.Pet)
			} else {
				p.Player.Worn = worn
				sourceStatusFixtureV5(s, p, 400, true)
				pet.Base.INT = 2000
			}
			p.HP, p.Stats.MAtk = 1, 10000
			finish()
			if m.HP != 10000-want {
				t.Fatalf("source changed accepted payload: damage%d want%d", 10000-m.HP, want)
			}
		})
	}
}

func TestCombatSourceV5AcceptedFalseStealthAndLiveTargetCondition(t *testing.T) {
	for _, kind := range []string{"invisible_focus", "execution"} {
		t.Run(kind, func(t *testing.T) {
			s, p, tr := uniqueTestGear(t, kind)
			p.Stats = domain.Stats{MAtk: 100}
			m := putMonster(s, 900, 10000, domain.Stats{}, 110, 100)
			id := domain.SkillID(10401)
			if tr.SkillID != 0 {
				id = tr.SkillID
			}
			def := domain.SkillDef{ID: id, Level: 1, Kind: domain.SkillSingle, HurtType: domain.HurtFire, TargetEnemy: true, Dist: 450, PrepareMS: 100, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100}}
			traits := append([]domain.UniqueTrait(nil), s.uniqueTraits(p)...)
			finish := sourceAcceptedCastV5(t, s, p, def, m.ID)
			sourceStatusFixtureV5(s, p, 0, true)
			if kind == "execution" {
				m.HP = m.MaxHP * tr.ThresholdPct / 100
			}
			before := m.HP
			x := domain.EvaluateUniqueTraits(traits, domain.UniqueContext{SkillID: id, HPPercent: 100, MPPercent: 100, TargetHPPercent: uniquePercent(m.HP, m.MaxHP), Magic: true, Skill: true, HurtType: domain.HurtFire})
			finish()
			if before-m.HP != domain.ApplyUniqueBP(100, x.DamageBP) {
				t.Fatal("source stealth or target condition wrong", before-m.HP, x)
			}
		})
	}
}

func TestCombatSourceV5AcceptedHealingTraits(t *testing.T) {
	for _, initial := range []bool{false, true} {
		s, p, tr := uniqueTestGear(t, "healing_grace")
		worn := p.Player.Worn
		if !initial {
			p.Player.Worn = domain.NewEquipSet()
		}
		p.HP = 100
		id := domain.SkillID(10301)
		if tr.SkillID != 0 {
			id = tr.SkillID
		}
		def := domain.SkillDef{ID: id, Level: 1, Kind: domain.SkillSingle, TargetSelf: true, PrepareMS: 100, Effect: domain.SkillEffect{Kind: domain.EffectHeal, HealFlat: 100}}
		want := s.uniqueHealing(p, p, 100, def)
		finish := sourceAcceptedCastV5(t, s, p, def, p.ID)
		if initial {
			p.Player.Worn = domain.NewEquipSet()
		} else {
			p.Player.Worn = worn
		}
		finish()
		if p.HP != 100+want {
			t.Fatal("heal trait changed after acceptance", initial, p.HP, want)
		}
	}
}

func TestCombatSourceV5AcceptedTrapPlacementAndLowerSources(t *testing.T) {
	for _, tc := range []struct {
		name                           string
		castGear, finalGear            bool
		finalStat, finalFlat, finalPet int32
	}{
		{"new_gear_before_placement", false, true, 10000, 400, 2000},
		{"remove_after_placement", true, false, 10000, 0, 0},
		{"retain_cast_ceiling", true, true, 10000, 400, 2000},
		{"lower_current_values", true, true, 50, 10, 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, p, tr := uniqueTestGear(t, "fire_mastery")
			s.alloc = domain.NewEntityAlloc()
			worn := p.Player.Worn
			if !tc.castGear {
				p.Player.Worn = domain.NewEquipSet()
			}
			p.Stats = domain.Stats{MAtk: 100}
			sourceStatusFixtureV5(s, p, 40, false)
			pet := sourcePetFixtureV5(s, p, 10000)
			id := domain.SkillID(10401)
			if tr.SkillID != 0 {
				id = tr.SkillID
			}
			def := domain.SkillDef{ID: id, Level: 1, Kind: domain.SkillSingle, HurtType: domain.HurtFire, Dist: 450, PrepareMS: 100, Effects: []domain.SkillEffect{{Kind: domain.EffectPlaceTrap, TrapRadius: 100}, {Kind: domain.EffectDamage, DamagePct: 100}}}
			m := putMonster(s, 900, 10000, domain.Stats{}, 110, 100)
			s.monsters[m.ID] = m
			traits := append([]domain.UniqueTrait(nil), s.uniqueTraits(p)...)
			finish := sourceAcceptedCastV5(t, s, p, def, p.ID)
			// Raise every source between accepted cast and actual placement.
			p.Player.Worn = worn
			p.Stats.MAtk = 10000
			sourceStatusFixtureV5(s, p, 400, false)
			pet.Base.INT = 2000
			finish()
			if len(s.traps) != 1 {
				t.Fatal("trap not placed")
			}
			if !tc.finalGear {
				p.Player.Worn = domain.NewEquipSet()
			}
			p.Stats.MAtk = tc.finalStat
			sourceStatusFixtureV5(s, p, tc.finalFlat, false)
			pet.Base.INT = tc.finalPet
			base := min(int32(100), tc.finalStat) + min(int32(40), tc.finalFlat) + min(int32(20), tc.finalPet)
			if !tc.finalGear {
				traits = nil
			}
			x := domain.EvaluateUniqueTraits(traits, domain.UniqueContext{SkillID: id, HPPercent: 100, MPPercent: 100, TargetHPPercent: 100, Magic: true, Skill: true, Trap: true, Area: true, HurtType: domain.HurtFire})
			s.stepTraps()
			if got, want := 10000-m.HP, domain.ApplyUniqueBP(base, x.DamageBP); got != want {
				t.Fatal("trap source ceiling", got, want)
			}
		})
	}
}

func TestCombatSourceV5PetRollRemainsPerHit(t *testing.T) {
	s := newCombatScene(t)
	join(t, s, 1, 100, "宠物来源", 100, 100)
	p := s.players[1]
	p.Stats = domain.Stats{MAtk: 100}
	pet := sourcePetFixtureV5(s, p, 5000)
	source := s.captureCombatSourceV5(p, false)
	pet.Base.INT = 9999
	s.rng = rand.New(rand.NewSource(7))
	counts := map[int32]int{}
	for i := 0; i < 40; i++ {
		m := putMonster(s, domain.EntityID(900+i), 1000, domain.Stats{}, 110, 100)
		ev, _, _ := s.resolveStrikeStatuses(p, m, &combat.HitTracker{}, combat.Blow{Magic: true, SourceOffense: source})
		counts[ev.Amount]++
	}
	if counts[100] == 0 || counts[120] == 0 || len(counts) != 2 {
		t.Fatal("pet rolled once or inherited live power", counts)
	}
}
