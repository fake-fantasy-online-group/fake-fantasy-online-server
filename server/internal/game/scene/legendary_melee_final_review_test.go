package scene

import (
	"fmt"
	"math/rand"
	"reflect"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// A28 never promised a learned-passive requirement. A30 did. Exercise both
// through native admission and actual movement, with legal three-slot gear.
func TestAssassinFinalReviewPullLearningAndGeometryCombinations(t *testing.T) {
	for _, rank := range []int32{1, 5, 20} {
		for _, passive := range []bool{false, true} {
			for _, mode := range []string{"a28", "a30", "pair", "triple"} {
				t.Run(fmt.Sprintf("rank=%d/passive=%v/%s", rank, passive, mode), func(t *testing.T) {
					powers := []string{"CK-A28"}
					if mode == "a30" {
						powers = []string{"CK-A30"}
					} else if mode == "pair" || mode == "triple" {
						powers = append(powers, "CK-A30")
					}
					if mode == "triple" {
						powers = append(powers, "TY-A04")
					}
					learned := domain.Learned{10225: rank}
					if passive {
						learned[10226] = 1
					}
					s, p, m := nativeRepairFixture(t, domain.Assassin, learned, powers...)
					m.Pos.X = 290 // 190 from owner: A28 is bounded to120; A30 reaches40.
					s.aoi.Move(m, m.Pos)
					far := putMonster(s, 901, 10000, domain.Stats{}, 401, 100)
					s.monsters[far.ID] = far
					// Native chance remains10%; use a non-triggering RNG branch here.
					s.rng = rand.New(rand.NewSource(4))
					native, ok := s.skills.Get(10225, rank)
					if !ok {
						t.Fatal("missing native skill rank")
					}
					s.applyResolvedSkillModifiers(p, &native, learned, s.rawSkillLevels(p))
					d := assassinRegressionCast(t, s, p, UseSkill{Skill: 10225, Target: m.ID})
					wantBP := int32(4500)
					if mode == "a30" {
						wantBP = 0
					}
					if d.LegendaryModifiers.AppliedDamageBP != wantBP {
						t.Fatalf("damage bucket=%d want%d", d.LegendaryModifiers.AppliedDamageBP, wantBP)
					}
					radiusBP := int32(0)
					a30 := passive && mode != "a28"
					if a30 {
						radiusBP += 5000
					}
					if mode == "triple" {
						radiusBP += 4000
					}
					wantRadius := native.Area.Radius * (10000 + radiusBP) / 10000
					if d.Area.Radius != wantRadius {
						t.Fatalf("radius=%d want%d, native=%d", d.Area.Radius, wantRadius, native.Area.Radius)
					}
					activePull := mode != "a30" || passive
					if activePull && (m.Pos.X >= 290 || m.Pos.X < 140) {
						t.Fatalf("pull not applied within native safe bounds: %+v", m.Pos)
					}
					if !activePull && m.Pos.X != 290 {
						t.Fatal("A30's explicit learning prerequisite was relaxed", m.Pos)
					}
					if a30 && m.Pos.X != 140 {
						t.Fatal("active A30 did not replace bounded pull", m.Pos)
					}
					if !passive && learned[10226] != 0 {
						t.Fatal("power learned the passive")
					}
					if (far.HP < far.MaxHP) != (wantRadius >= 301) {
						t.Fatalf("final-radius hit admission disagrees: radius=%d hp=%d", wantRadius, far.HP)
					}
				})
			}
		}
	}
}

func TestWarriorFinalReviewChainSharesReplacedBasicCoefficient(t *testing.T) {
	for _, ranks := range [][2]int32{{1, 1}, {5, 1}, {1, 20}, {20, 1}, {1, 0}} {
		for _, mode := range []string{"single", "pair", "third-flat", "third-damage"} {
			for _, set := range []bool{false, true} {
				t.Run(fmt.Sprintf("chain=%d/eye=%d/%s/set=%v", ranks[0], ranks[1], mode, set), func(t *testing.T) {
					powers := []string{"ZS-A03"}
					if mode != "single" {
						powers = append(powers, "ZS-A21")
					}
					if mode == "third-flat" {
						powers = append(powers, "ZS-A20")
					}
					if mode == "third-damage" {
						powers = append(powers, "ZS-A01")
					}
					learned := domain.Learned{10002: ranks[0]}
					if ranks[1] > 0 {
						learned[10015] = ranks[1]
					}
					s, p, m := meleeAuditNativeScene(t, domain.Warrior, learned, powers...)
					if set {
						meleeAuditEquipSet(t, s, p, 5)
					}
					c := legendaryContext{Scene: s, Owner: p, Target: m, Primary: m.ID}
					chain, _ := warriorLearned(&c, 10002, false)
					want := chain.DirectEffects()[0]
					if want.DamagePctDenominator <= 0 {
						want.DamagePctNumerator, want.DamagePctDenominator = int64(want.DamagePct), 1
					}
					if mode != "single" && ranks[1] > 0 {
						eye, _ := warriorLearned(&c, 10015, false)
						e := eye.DirectEffects()[0]
						if e.DamagePctDenominator <= 0 {
							e.DamagePctNumerator, e.DamagePctDenominator = int64(e.DamagePct), 1
						}
						if e.DamagePctNumerator*want.DamagePctDenominator > want.DamagePctNumerator*e.DamagePctDenominator {
							want = e
						}
					}
					baseNum, baseDen := want.DamagePctNumerator, want.DamagePctDenominator
					if mode == "third-damage" {
						domain.ScaleSkillDamageFraction(&want, 180, 100)
					}
					hp, mp := m.HP, p.MP
					d := meleeAuditCast(t, s, p, 10002, m.ID)
					effect := d.DirectEffects()[0]
					n, den := effect.DamagePctNumerator, effect.DamagePctDenominator
					if den <= 0 {
						n, den = int64(effect.DamagePct), 1
					}
					if n*want.DamagePctDenominator != want.DamagePctNumerator*den {
						t.Fatalf("chain coefficient=%d/%d want%d/%d; A21 must share its base, not its active+100%% or A20 flat", n, den, want.DamagePctNumerator, want.DamagePctDenominator)
					}
					if effect.BaseDamageFlat != 0 {
						t.Fatal("eye flat payload leaked into chain", effect)
					}
					if !set && hp-m.HP != int32(n/den) {
						t.Fatalf("actual chain damage=%d want%d", hp-m.HP, n/den)
					}
					if set && (p.MP != mp || !s.classSetDragonActive(p)) {
						t.Fatal("native dragon cost/set composition lost")
					}
					hp, mp = m.HP, p.MP
					s.onAttack(Attack{ID: p.ID, Target: m.ID})
					s.stepCombatAt(s.now().Add(time.Second), false)
					wantBasic := domain.SkillEffect{DamagePctNumerator: baseNum, DamagePctDenominator: baseDen}
					damageScale := int64(220)
					if mode != "single" && ranks[1] == 0 {
						damageScale += 100
					}
					if mode == "third-flat" && ranks[1] > 0 {
						wantBasic.BaseDamageFlat = 240
					}
					domain.ScaleSkillDamageFraction(&wantBasic, damageScale, 100)
					if !set {
						damage := int32(wantBasic.DamagePctNumerator/wantBasic.DamagePctDenominator) + wantBasic.BaseDamageFlat
						if hp-m.HP != damage {
							t.Fatalf("actual basic=%d want%d", hp-m.HP, damage)
						}
					}
					if m.HP >= hp || p.MP != mp {
						t.Fatal("basic was not a resource-free damaging swing")
					}
				})
			}
		}
	}
}

func TestAssassinFinalReviewFrostBorrowedPoisonEffectiveRank(t *testing.T) {
	for _, hard := range []int32{1, 5, 20} {
		for _, soft := range []bool{false, true} {
			t.Run(fmt.Sprintf("hard=%d/soft=%v", hard, soft), func(t *testing.T) {
				var poisons []domain.StatusDef
				for _, prebuff := range []bool{false, true} {
					s, p, m := nativeRepairFixture(t, domain.Assassin, domain.Learned{10211: hard, 10216: 1}, "CK-A18", "CK-A09")
					if soft {
						items := s.items
						item, gear := growthAllSkillGear(s)
						s.items = items
						s.items[item.ID] = item
						p.Player.Worn.Set(domain.SlotGlove, gear)
						s.invalidateCombatAttributes(p)
						if got, _ := s.skillLevelForUse(p, 10211); got != hard+1 {
							t.Fatalf("real+skill gear inactive: %d", got)
						}
					}
					s.rng = rand.New(meleeAuditZeroSource{})
					if prebuff {
						assassinRegressionCast(t, s, p, UseSkill{Skill: 10211, Target: p.ID})
					}
					assassinRegressionCast(t, s, p, UseSkill{Skill: 10216, At: &m.Pos})
					for _, trap := range s.traps {
						s.triggerAssassinTrap(trap, false)
					}
					c := legendaryContext{Scene: s, Owner: p}
					poison, ok := legendaryAssassinNativePoison(&c, m)
					if !ok || !m.Status.Has(domain.StatusFreeze) {
						t.Fatal("native frost and poison not committed", prebuff)
					}
					poisons = append(poisons, poison.Def)
				}
				if !reflect.DeepEqual(poisons[0].HPChanges, poisons[1].HPChanges) || !reflect.DeepEqual(poisons[0].Affixes, poisons[1].Affixes) || poisons[0].DurationSec != poisons[1].DurationSec {
					t.Fatalf("borrowed poison skipped current native modifiers: absent carrier=%+v, existing carrier=%+v", poisons[0], poisons[1])
				}
			})
		}
	}
}

func TestAssassinFinalReviewFrostBorrowedPoisonAcceptedRanks(t *testing.T) {
	for _, mode := range []string{"later-soft-rank", "removed-soft-rank", "later-learned", "unlearned"} {
		t.Run(mode, func(t *testing.T) {
			learned := domain.Learned{10211: 1, 10216: 1}
			if mode == "later-learned" || mode == "unlearned" {
				delete(learned, 10211)
			}
			s, p, m := nativeRepairFixture(t, domain.Assassin, learned, "CK-A18", "CK-A09", "CK-A17")
			items := s.items
			item, gear := growthAllSkillGear(s)
			s.items = items
			s.items[item.ID] = item
			if mode == "removed-soft-rank" {
				p.Player.Worn.Set(domain.SlotGlove, gear)
				s.invalidateCombatAttributes(p)
			}
			s.rng = rand.New(meleeAuditZeroSource{})
			assassinRegressionCast(t, s, p, UseSkill{Skill: 10216, At: &m.Pos})
			switch mode {
			case "later-soft-rank":
				p.Player.Worn.Set(domain.SlotGlove, gear)
			case "removed-soft-rank":
				p.Player.Worn.Set(domain.SlotGlove, domain.Stack{})
			case "later-learned":
				p.Player.Char.Skills[10211] = 1
			}
			s.invalidateCombatAttributes(p)
			s.refreshSkillPersistentEffects(p)
			for _, trap := range s.traps {
				s.triggerAssassinTrap(trap, false)
			}
			c := legendaryContext{Scene: s, Owner: p}
			poison, ok := legendaryAssassinNativePoison(&c, m)
			if mode == "later-learned" || mode == "unlearned" {
				if ok {
					t.Fatal("trap borrowed a skill not learned at acceptance")
				}
				return
			}
			if !ok {
				t.Fatal("accepted learned poison disappeared")
			}
			var base int32
			for _, a := range poison.Def.Affixes {
				if a.Attr == domain.AttrHPRegen {
					base -= a.Value
				}
			}
			if base != 10 || poison.Def.LegendaryModifiers.DamageTickBP != 10000 {
				t.Fatalf("bounded rank/A09 composition=%d/%d want10/10000", base, poison.Def.LegendaryModifiers.DamageTickBP)
			}
		})
	}
}

func TestWarriorFinalReviewChainAcceptedCoefficientAndOffense(t *testing.T) {
	s, p, m := meleeAuditNativeScene(t, domain.Warrior, domain.Learned{10002: 1, 10015: 1}, "ZS-A03", "ZS-A21", "ZS-A20")
	s.onUseSkill(UseSkill{ID: p.ID, Skill: 10002, Target: m.ID})
	if len(s.casts) != 1 {
		t.Fatal("native chain not accepted")
	}
	p.Player.Char.Skills[10015] = 20
	p.Stats.MinAtk, p.Stats.MaxAtk = 9000, 9000
	s.invalidateCombatAttributes(p)
	before := m.HP
	now := s.now()
	s.stepCastsAt(now.Add(time.Minute), false)
	s.stepCastsAt(now.Add(2*time.Minute), false)
	if got := before - m.HP; got != 165 {
		t.Fatalf("accepted chain gained later eye rank or attack: %d", got)
	}
}

func TestWarriorFinalReviewUnlearnedChainDoesNotGateBasicCoefficient(t *testing.T) {
	for _, replaced := range []bool{false, true} {
		t.Run(fmt.Sprint(replaced), func(t *testing.T) {
			powers := []string{"ZS-A03"}
			if replaced {
				powers = append(powers, "ZS-A21")
			}
			// The right-hand Pierce path does not require the Chain skill.
			// Do not construct the unreachable learned-Eye/unlearned-Chain pair.
			s, p, m := meleeAuditNativeScene(t, domain.Warrior, domain.Learned{10005: 1}, powers...)
			before, mp := m.HP, p.MP
			s.onAttack(Attack{ID: p.ID, Target: m.ID})
			s.stepCombatAt(s.now().Add(time.Second), false)
			want := int32(286)
			if replaced {
				want = 416
			} // A21's explicitly unlearned fallback adds100%.
			if before-m.HP != want || p.MP != mp {
				t.Fatalf("borrowed rank1 basic=%d want%d; MP=%d", before-m.HP, want, p.MP)
			}
			s.onUseSkill(UseSkill{ID: p.ID, Skill: 10002, Target: m.ID})
			if len(s.casts) != 0 || p.Player.Char.Skills[10002] != 0 {
				t.Fatal("coefficient borrowing learned or unlocked Chain")
			}
		})
	}
}

func TestAssassinFinalReviewFrostBorrowedPoisonEffectAccuracy(t *testing.T) {
	for _, ratings := range [][2]int32{{2000, 2000}, {2000, 5000}, {2000, 700}, {0, 2000}} {
		for _, prebuff := range []bool{false, true} {
			t.Run(fmt.Sprintf("accepted=%d/current=%d/carrier=%v", ratings[0], ratings[1], prebuff), func(t *testing.T) {
				s, p, m := nativeRepairFixture(t, domain.Assassin, domain.Learned{10211: 1, 10216: 1}, "CK-A18", "CK-A09", "CK-A17")
				s.rng = rand.New(meleeAuditZeroSource{})
				// Computed-stat boundary: effect accuracy is an equipment-supported
				// attribute, distinct from the ordinary attack's Hit stat.
				p.Stats.EffectHitBP = ratings[0]
				if prebuff {
					assassinRegressionCast(t, s, p, UseSkill{Skill: 10211, Target: p.ID})
					p.Stats.EffectHitBP = ratings[0]
				}
				assassinRegressionCast(t, s, p, UseSkill{Skill: 10216, At: &m.Pos})
				p.Stats.EffectHitBP = ratings[1]
				for _, trap := range s.traps {
					s.triggerAssassinTrap(trap, false)
				}
				poison, ok := legendaryAssassinNativePoison(&legendaryContext{Scene: s, Owner: p}, m)
				if !ok || !m.Status.Has(domain.StatusFreeze) {
					t.Fatal("native frost/poison did not commit")
				}
				if got, want := poison.Def.SourceEffectHitBP, min(ratings[0], ratings[1]); !poison.Def.SourceEffectHitKnown || got != want {
					t.Fatalf("poison accepted effect accuracy=%d/known=%v want%d", got, poison.Def.SourceEffectHitKnown, want)
				}
				if !prebuff && p.Status.Has(domain.StatusPoisonHit) {
					t.Fatal("virtual poison carrier leaked a real owner buff")
				}
			})
		}
	}
}

func TestAssassinFinalReviewFrostExistingPoisonCannotUpgradeTrap(t *testing.T) {
	for _, accepted := range []int32{0, 1, 5} {
		t.Run(fmt.Sprint(accepted), func(t *testing.T) {
			learned := domain.Learned{10216: 1}
			if accepted > 0 {
				learned[10211] = accepted
			}
			s, p, m := nativeRepairFixture(t, domain.Assassin, learned, "CK-A18", "CK-A09", "CK-A17")
			s.rng = rand.New(meleeAuditZeroSource{})
			assassinRegressionCast(t, s, p, UseSkill{Skill: 10216, At: &m.Pos})
			p.Player.Char.Skills[10211] = 20
			s.invalidateCombatAttributes(p)
			assassinRegressionCast(t, s, p, UseSkill{Skill: 10211, Target: p.ID})
			var realCarrier domain.Active
			p.Status.EachEffective(func(a domain.Active) {
				if a.Def.ID == domain.StatusPoisonHit {
					realCarrier = a
				}
			})
			for _, trap := range s.traps {
				s.triggerAssassinTrap(trap, false)
			}
			var afterCarrier domain.Active
			p.Status.EachEffective(func(a domain.Active) {
				if a.Def.ID == domain.StatusPoisonHit {
					afterCarrier = a
				}
			})
			if !reflect.DeepEqual(realCarrier, afterCarrier) {
				t.Fatal("trap changed real poison carrier or its clock")
			}
			poison, ok := legendaryAssassinNativePoison(&legendaryContext{Scene: s, Owner: p}, m)
			if accepted == 0 {
				if ok {
					t.Fatal("newly learned active carrier upgraded an unlearned trap")
				}
				return
			}
			if !ok {
				t.Fatal("accepted learned poison disappeared")
			}
			var base int32
			for _, a := range poison.Def.Affixes {
				if a.Attr == domain.AttrHPRegen {
					base -= a.Value
				}
			}
			want := map[int32]int32{1: 10, 5: 24}[accepted]
			if base != want {
				t.Fatalf("later active carrier upgraded trap poison: %d want%d", base, want)
			}
			// A subsequent independent native strike may use the new level20 buff.
			other := putMonster(s, 902, 100000, domain.Stats{}, 140, 100)
			s.onAttack(Attack{ID: p.ID, Target: other.ID})
			s.stepCombatAt(s.now().Add(time.Second), false)
			newPoison, ok := legendaryAssassinNativePoison(&legendaryContext{Scene: s, Owner: p}, other)
			var newBase int32
			for _, a := range newPoison.Def.Affixes {
				if a.Attr == domain.AttrHPRegen {
					newBase -= a.Value
				}
			}
			if !ok || newBase != 76 {
				t.Fatalf("independent new attack retained old trap rank: %d", newBase)
			}
		})
	}
}

func TestAssassinFinalReviewFrostPoisonSetAttackSnapshot(t *testing.T) {
	for _, attack := range []int32{20, 100, 9000} {
		for _, prebuff := range []bool{false, true} {
			t.Run(fmt.Sprintf("attack=%d/carrier=%v", attack, prebuff), func(t *testing.T) {
				s, p, m := nativeRepairFixture(t, domain.Assassin, domain.Learned{10211: 1, 10216: 1}, "CK-A18", "CK-A09", "CK-A17")
				meleeAuditEquipSet(t, s, p, 1)
				s.rng = rand.New(meleeAuditZeroSource{})
				if prebuff {
					assassinRegressionCast(t, s, p, UseSkill{Skill: 10211, Target: p.ID})
				}
				p.Stats.MinAtk, p.Stats.MaxAtk = 100, 100
				assassinRegressionCast(t, s, p, UseSkill{Skill: 10216, At: &m.Pos})
				p.Stats.MinAtk, p.Stats.MaxAtk = attack, attack
				for _, trap := range s.traps {
					s.triggerAssassinTrap(trap, false)
				}
				poison, ok := legendaryAssassinNativePoison(&legendaryContext{Scene: s, Owner: p}, m)
				if !ok {
					t.Fatal("frost poison missing")
				}
				if got, want := poison.Def.ClassSetPoisonBonusTotal, int64(min(attack, 100))*10; got != want {
					t.Fatalf("poison set attack escaped trap snapshot: %d want%d", got, want)
				}
			})
		}
	}
}
