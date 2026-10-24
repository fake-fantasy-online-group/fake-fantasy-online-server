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
)

// Advance actual native admission/release/impact without rewriting costs,
// cooldowns, geometry, status chances, skill coefficients or projectile timing.
func assassinRegressionCast(t *testing.T, s *Scene, p *entity.Entity, cmd UseSkill) domain.SkillDef {
	t.Helper()
	cmd.ID = p.ID
	s.onUseSkill(cmd)
	if len(s.casts) != 1 {
		t.Fatalf("native admission failed skill=%d casts=%d events=%+v", cmd.Skill, len(s.casts), s.outbox)
	}
	def := s.casts[0].def
	now := s.now()
	s.stepCastsAt(now.Add(time.Minute), false)
	s.stepCastsAt(now.Add(2*time.Minute), false)
	if len(s.casts) != 0 {
		t.Fatal("cast did not settle")
	}
	return def
}
func assassinRegressionDamage(s *Scene, target domain.EntityID, skill int32) (int32, int) {
	var n int32
	count := 0
	for _, o := range s.outbox {
		if e, ok := o.ev.(event.DamageDealt); ok && e.Dst == target && e.SkillID == skill {
			n += e.Amount
			count++
		}
	}
	return n, count
}
func TestAssassinRegressionInstantBombNativeCombinations(t *testing.T) {
	for _, weapon := range []bool{false, true} {
		for _, n := range []int{1, 2, 3} {
			t.Run(fmt.Sprintf("1822=%v/powers=%d", weapon, n), func(t *testing.T) {
				powers := []string{"CK-A13", "CK-A11", "CK-A12"}
				s, p, m := nativeRepairFixture(t, domain.Assassin, domain.Learned{10213: 1}, powers[:n]...)
				if weapon {
					p.Player.Worn = domain.NewEquipSet()
					assassinRegressionEnhancedBombWeapon(t, s, p)
					assassinAddPower(s, p, "CK-A13", domain.SlotBody)
					if n >= 2 {
						st := p.Player.Worn.At(domain.SlotWeapon)
						st.Legendary = domain.LegendaryInstance{PowerID: "CK-A11"}
						p.Player.Worn.Set(domain.SlotWeapon, st)
					}
					if n >= 3 {
						assassinAddPower(s, p, "CK-A12", domain.SlotHead)
					}
					if got := len(domain.ActiveLegendaryPowers(s.functionalWorn(p), s.itemDef, p.Player.Char)); got != n {
						t.Fatalf("inactive enhanced combination %d/%d", got, n)
					}
				}
				p.Stats = domain.Stats{MinAtk: 100, MaxAtk: 100, Hit: 100000}
				p.HP, p.MaxHP, p.MP, p.MaxMP = 10000, 10000, 10000, 10000
				before := p.MP
				s.outbox = nil
				def := assassinRegressionCast(t, s, p, UseSkill{Skill: 10213, At: &m.Pos})
				if before-p.MP != def.MPCost {
					t.Fatal("cast did not pay exactly once", before-p.MP, def.MPCost)
				}
				if !weapon && n == 1 {
					if len(s.traps) != 3 {
						t.Fatal("persistent triple count", len(s.traps))
					}
					for _, tr := range s.traps {
						s.triggerAssassinTrap(tr, false)
					}
				}
				damage, hits := assassinRegressionDamage(s, m.ID, 10213)
				want := []int32{600, 840, 1380}[n-1]
				if damage != want || hits != 3 || len(s.traps) != 0 {
					t.Fatalf("triple=%d/%d traps=%d want%d/3 effects=%+v", damage, hits, len(s.traps), want, def.DirectEffects())
				}
			})
		}
	}
}
func TestAssassinRegressionThrowFinalSplashNativeCast(t *testing.T) {
	for n := 1; n <= 3; n++ {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			powers := []string{"CK-A05", "CK-A27", "CK-A04"}
			s, p, m := nativeRepairFixture(t, domain.Assassin, domain.Learned{10205: 1}, powers[:n]...)
			near := putMonster(s, 901, 10000, domain.Stats{}, m.Pos.X+125, m.Pos.Y)
			s.monsters[near.ID] = near
			far := putMonster(s, 902, 10000, domain.Stats{}, m.Pos.X+151, m.Pos.Y)
			s.monsters[far.ID] = far
			d := assassinRegressionCast(t, s, p, UseSkill{Skill: 10205, Target: m.ID})
			radius := int32(100)
			if n >= 2 {
				radius = 150
			}
			if d.Radius != radius || (near.HP < near.MaxHP) != (n >= 2) || far.HP != far.MaxHP {
				t.Fatalf("splash radius=%d near=%d far=%d", d.Radius, near.HP, far.HP)
			}
			damage, hits := assassinRegressionDamage(s, m.ID, 10205)
			want := int32(150)
			if n == 3 {
				want = 225
			}
			if damage != want || hits != 1 {
				t.Fatalf("native hit=%d/%d want%d/1", damage, hits, want)
			}
		})
	}
}
func TestAssassinRegressionDisarmPreservesVisibleFriendlyTraps(t *testing.T) {
	s, p, m := nativeRepairFixture(t, domain.Assassin, domain.Learned{10218: 20}, "CK-U03")
	join(t, s, 2, 200, "teammate", 150, 100)
	ally := s.players[2]
	p.Player.Party = 7
	ally.Player.Party = 7
	var ids []domain.EntityID
	for i, owner := range []*entity.Entity{p, ally, m, m} {
		before := map[domain.EntityID]bool{}
		for id := range s.traps {
			before[id] = true
		}
		at := domain.Pos{MapID: p.Pos.MapID, X: 150 + float64(i*5), Y: 100}
		s.placeTrap(owner, domain.SkillDef{ID: 10213}, at, 50, 100)
		for id, tr := range s.traps {
			if !before[id] {
				ids = append(ids, id)
				if i != 3 {
					tr.Trap.VisibleTo[p.ID] = struct{}{}
				}
			}
		}
	}
	before := p.MP
	assassinRegressionCast(t, s, p, UseSkill{Skill: 10218, Target: ids[2]})
	if p.MP != before || s.traps[ids[0]] == nil || s.traps[ids[1]] == nil || s.traps[ids[2]] != nil || s.traps[ids[3]] == nil {
		t.Fatal("wrong disarm survivors", s.traps)
	}
	count := len(s.traps)
	s.disarmTrap(p, ids[1], 10000)
	if len(s.traps) != count {
		t.Fatal("friendly trap accepted as area center")
	}
}
func TestAssassinRegressionDetectNativeFinalRadius(t *testing.T) {
	for _, wide := range []bool{false, true} {
		t.Run(fmt.Sprint(wide), func(t *testing.T) {
			powers := []string{"CK-U04"}
			if wide {
				powers = append(powers, "TY-A04")
			}
			s, p, m := nativeRepairFixture(t, domain.Assassin, domain.Learned{10228: 1}, powers...)
			for _, dist := range []float64{250, 301, 381} {
				s.placeTrap(m, domain.SkillDef{ID: 10213}, domain.Pos{MapID: p.Pos.MapID, X: p.Pos.X + dist, Y: p.Pos.Y}, 50, 100)
			}
			d := assassinRegressionCast(t, s, p, UseSkill{Skill: 10228})
			want := int32(300)
			if wide {
				want = 380
			}
			if d.Radius != want || d.Area.Radius != want || d.DirectEffects()[0].ChanceBP != 4500 {
				t.Fatalf("native geometry/chance %+v %v", d.Area, d.DirectEffects())
			}
			for i := 0; i < 50; i++ {
				s.detectTraps(p, d.Radius, d.DirectEffects()[0].ChanceBP)
			}
			for _, tr := range s.traps {
				if tr.Trap.VisibleToPlayer(p.ID) != (tr.Pos.X-p.Pos.X <= float64(want)) {
					t.Fatal("detection boundary", tr.Pos)
				}
			}
		})
	}
}
func TestUniversalRegressionTargetBonusRealBasicAndTrap(t *testing.T) {
	for _, tc := range []struct {
		id     string
		status domain.StatusID
		level  int32
		want   int32
	}{{"TY-A30", 1002, 11, 150}, {"TY-A31", 1001, 1, 140}, {"TY-A32", 1036, 1, 140}} {
		for _, owned := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/owned=%v", tc.id, owned), func(t *testing.T) {
				race := domain.Assassin
				learned := domain.Learned{10213: 1}
				if tc.id == "TY-A32" {
					race = domain.Warlock
					learned = domain.Learned{10421: 1}
				}
				s, p, m := nativeRepairFixture(t, race, learned, tc.id)
				p.Look.AtkDist = 200
				st, ok := s.statuses.Get(tc.status, tc.level)
				if !ok {
					t.Fatal("native status missing")
				}
				source := p.ID
				if !owned {
					source = 77
				}
				m.Status.Apply(st, s.tick, source)
				s.onAttack(Attack{ID: p.ID, Target: m.ID, Blow: combat.Blow{GuaranteedHit: true}})
				if len(s.attacks) != 1 {
					t.Fatal("basic not admitted")
				}
				s.stepCombatAt(s.now().Add(time.Second), false)
				want := tc.want
				if !owned && tc.id != "TY-A30" {
					want = 100
				}
				damage, hits := assassinRegressionDamage(s, m.ID, 0)
				if damage != want || hits != 1 {
					t.Fatalf("real basic=%d/%d want%d/1", damage, hits, want)
				}
				if race == domain.Warlock {
					return
				} // No impossible warlock/assassin trap combination.
				m.Status.Apply(st, s.tick, source)
				s.outbox = nil
				assassinRegressionCast(t, s, p, UseSkill{Skill: 10213, At: &m.Pos})
				if len(s.traps) != 1 {
					t.Fatal("native trap missing")
				}
				for _, tr := range s.traps {
					s.triggerAssassinTrap(tr, false)
				}
				damage, hits = assassinRegressionDamage(s, m.ID, 10213)
				if damage != want*2 || hits != 1 {
					t.Fatalf("real trap=%d/%d want%d/1", damage, hits, want*2)
				}
			})
		}
	}
}
func TestUniversalRegressionPhysicalTrapEquipmentBaseAndGeometry(t *testing.T) {
	s, p, m := nativeRepairFixture(t, domain.Assassin, domain.Learned{10213: 1}, "TY-A25", "TY-A04", "TY-A21")
	item := s.items[p.Player.Worn.At(domain.SlotWeapon).Item]
	item.Equip.Bonus = domain.Stats{MinAtk: 100, MaxAtk: 100}
	s.items[item.ID] = item
	d := assassinRegressionCast(t, s, p, UseSkill{Skill: 10213, At: &m.Pos})
	if d.HurtType != domain.HurtNone || d.DirectEffects()[0].TrapRadius != 190 {
		t.Fatalf("native physical circle %+v %v", d.HurtType, d.DirectEffects())
	}
	for _, tr := range s.traps {
		s.triggerAssassinTrap(tr, false)
	}
	damage, hits := assassinRegressionDamage(s, m.ID, 10213)
	if damage != 300 || hits != 1 {
		t.Fatalf("equipment base before native coefficient=%d/%d", damage, hits)
	}
}
func TestUniversalRegressionHealingStatusManaNativeCast(t *testing.T) {
	for _, extra := range []bool{false, true} {
		t.Run(fmt.Sprint(extra), func(t *testing.T) {
			powers := []string{"TY-U04"}
			if extra {
				powers = append(powers, "TY-U03")
			}
			s, p, _ := nativeRepairFixture(t, domain.Healer, domain.Learned{10303: 1}, powers...)
			s.refreshStats(p)
			p.MP = p.MaxMP
			native, _ := s.skills.Get(10303, 1)
			before := p.MP
			d := assassinRegressionCast(t, s, p, UseSkill{Skill: 10303, Target: p.ID})
			pct := int32(70)
			if extra {
				pct = 40
			}
			want := (native.MPCost*pct + 99) / 100
			if before-p.MP != want || d.MPCost != want {
				t.Fatalf("HoT paid=%d prepared=%d native=%d want%d", before-p.MP, d.MPCost, native.MPCost, want)
			}
			if p.Status == nil || !p.Status.Has(10303) {
				t.Fatal("native HoT missing")
			}
		})
	}
}
func TestAssassinRegressionPullNativePassiveAndA30(t *testing.T) {
	for _, passive := range []bool{false, true} {
		for _, gather := range []bool{false, true} {
			for _, immune := range []bool{false, true} {
				t.Run(fmt.Sprintf("passive=%v/gather=%v/immune=%v", passive, gather, immune), func(t *testing.T) {
					powers := []string{"CK-A28"}
					if gather {
						powers = append(powers, "CK-A30")
					}
					learned := domain.Learned{10225: 1}
					if passive {
						learned[10226] = 1
					}
					s, p, m := nativeRepairFixture(t, domain.Assassin, learned, powers...)
					m.Pos.X = 250
					s.aoi.Move(m, m.Pos)
					if immune {
						m.Status.Apply(domain.StatusDef{ID: 9999, TypeKnown: true, Type: domain.StatusGood, DurationSec: 60, SuppressHarmful: true}, s.tick, m.ID)
					}
					// Drive the RNG, not the passive payload: keep the real10% knockback chance.
					seed := int64(0)
					for ; seed < 10000; seed++ {
						if rand.New(rand.NewSource(seed)).Intn(10000) < 1000 {
							break
						}
					}
					s.rng = rand.New(rand.NewSource(seed))
					d := assassinRegressionCast(t, s, p, UseSkill{Skill: 10225, Target: m.ID})
					want := int32(4500)
					if d.LegendaryModifiers.AppliedDamageBP != want {
						t.Fatal("A28 incorrectly requires the knockback passive", d.LegendaryModifiers)
					}
					var moves []float64
					for _, o := range s.outbox {
						if e, ok := o.ev.(event.EntityMoved); ok && e.ID == m.ID {
							moves = append(moves, e.To.X)
						}
					}
					if immune {
						if len(moves) != 0 {
							t.Fatal("immune target moved", moves)
						}
						return
					}
					if !passive {
						if len(moves) != 1 || m.Pos.X != 140 {
							t.Fatal("unlearned A28 must pull, even with inactive A30 equipped", moves)
						}
					} else if gather {
						if len(moves) != 1 || m.Pos.X != 140 {
							t.Fatal("A30 must suppress push and gather once", moves)
						}
					} else {
						if len(moves) != 2 || moves[0] != 350 || moves[1] != 230 {
							t.Fatal("A28 must retain native100 push then bounded120 pull", moves)
						}
						t.Logf("native push and bounded pull=%v", moves)
					}
				})
			}
		}
	}
}

func TestUniversalRegressionTrapConditionalDoubleAndTriple(t *testing.T) {
	for _, instant := range []bool{false, true} {
		for _, powerCount := range []int{2, 3} {
			t.Run(fmt.Sprintf("instant=%v/powers=%d", instant, powerCount), func(t *testing.T) {
				powers := []string{"TY-A30", "TY-A31", "CK-A13"}
				s, p, m := nativeRepairFixture(t, domain.Assassin, domain.Learned{10213: 1}, powers[:powerCount]...)
				if instant {
					p.Player.Worn = domain.NewEquipSet()
					assassinRegressionEnhancedBombWeapon(t, s, p)
					ordered := []string{"TY-A30", "TY-A31"}
					if powerCount == 3 {
						ordered = []string{"CK-A13", "TY-A30", "TY-A31"}
					}
					nativeRepairEquip(t, s, p, ordered...)
				}
				p.Stats = domain.Stats{MinAtk: 100, MaxAtk: 100, Hit: 100000}
				p.MP, p.MaxMP = 10000, 10000
				for _, sk := range []domain.StatusKey{{ID: 1002, Level: 11}, {ID: 1001, Level: 1}} {
					st, ok := s.statuses.Get(sk.ID, sk.Level)
					if !ok {
						t.Fatal("native status missing")
					}
					m.Status.Apply(st, s.tick, p.ID)
				}
				s.outbox = nil
				assassinRegressionCast(t, s, p, UseSkill{Skill: 10213, At: &m.Pos})
				if !instant {
					for _, tr := range s.traps {
						s.triggerAssassinTrap(tr, false)
					}
				}
				n := int32(1)
				if powerCount == 3 {
					n = 3
				}
				damage, hits := assassinRegressionDamage(s, m.ID, 10213)
				if damage != 380*n || int32(hits) != n {
					t.Fatalf("target bonuses must join one additive bucket per native blast: %d/%d want%d/%d", damage, hits, 380*n, n)
				}
			})
		}
	}
}

func TestUniversalRegressionOwnPoisonBasicWithAssassinSet(t *testing.T) {
	for _, pieces := range []int{2, 4} {
		t.Run(fmt.Sprint(pieces), func(t *testing.T) {
			s, p, m := nativeRepairFixture(t, domain.Assassin, domain.Learned{10211: 1}, "TY-A31")
			// Attach native assassin set identity to functional items, preserving the power.
			suit := &domain.EquipmentSuit{ID: 1, EnhancedClass: 1}
			slots := []domain.EquipSlot{domain.SlotWeapon, domain.SlotHead, domain.SlotBody, domain.SlotGlove}
			for i, slot := range slots[:pieces] {
				item := p.Player.Worn.At(slot).Item
				if item == 0 {
					item = domain.ItemID(980900 + i)
					s.items[item] = domain.ItemDef{ID: item, Equip: &domain.EquipDef{Slot: int32(slot), Durable: 100}}
					p.Player.Worn.Set(slot, domain.Stack{UID: int64(item), Item: item, Count: 1, Durability: 100, MaxDurability: 100})
				}
				suit.Groups = append(suit.Groups, []domain.ItemID{item})
				d := s.items[item]
				d.Equip.Suit = suit
				s.items[item] = d
			}
			if s.classSetPieces(p, 1) != int32(pieces) {
				t.Fatal("inactive assassin set")
			}
			// Use current migration poison. Its extra set payload is periodic and must not
			// suppress or multiply the independent direct-hit weakness bonus.
			st, ok := s.statuses.Get(domain.StatusPoison, 1)
			if !ok {
				t.Fatal("poison missing")
			}
			m.Status.Apply(st, s.tick, p.ID)
			p.Look.AtkDist = 200
			s.outbox = nil
			s.onAttack(Attack{ID: p.ID, Target: m.ID, Blow: combat.Blow{GuaranteedHit: true}})
			s.stepCombatAt(s.now().Add(time.Second), false)
			damage, hits := assassinRegressionDamage(s, m.ID, 0)
			if damage != 140 || hits != 1 {
				t.Fatalf("set+poison basic=%d/%d", damage, hits)
			}
		})
	}
}

// Migration24 ov_arm index1822 has native level_need26, category13/type8 and
// assassin permission. The hard version inherits these restrictions unchanged.
// Only A11(min25), not A13(min40), can be carried on this particular weapon.
func assassinRegressionEnhancedBombWeapon(t *testing.T, s *Scene, p *entity.Entity) {
	specialWeaponEquip(t, s, p, 1822)
	st := p.Player.Worn.At(domain.SlotWeapon)
	d := s.items[st.Item]
	d.Equip.LevelReq = 26
	d.Equip.Need.Professions = 4
	s.items[d.ID] = d
}

func TestUniversalRegressionTrapAttackBaseCannotUpgradeAfterPlacement(t *testing.T) {
	s, p, m := nativeRepairFixture(t, domain.Assassin, domain.Learned{10213: 1}, "TY-A25")
	item := s.items[p.Player.Worn.At(domain.SlotWeapon).Item]
	item.Equip.Bonus = domain.Stats{MinAtk: 100, MaxAtk: 100}
	s.items[item.ID] = item
	assassinRegressionCast(t, s, p, UseSkill{Skill: 10213, At: &m.Pos})
	item.Equip.Bonus = domain.Stats{MinAtk: 200, MaxAtk: 200}
	s.items[item.ID] = item
	s.stepTraps()
	damage, hits := assassinRegressionDamage(s, m.ID, 10213)
	if damage != 300 || hits != 1 {
		t.Fatalf("placed trap borrowed upgraded equipment: %d/%d", damage, hits)
	}
}

func TestUniversalRegressionMagicAttackBaseNativeCombination(t *testing.T) {
	for count := 1; count <= 3; count++ {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			powers := []string{"TY-A26", "TY-A08", "TY-A30"}
			s, p, m := nativeRepairFixture(t, domain.Warlock, domain.Learned{10409: 1}, powers[:count]...)
			item := s.items[p.Player.Worn.At(domain.SlotWeapon).Item]
			item.Equip.Bonus = domain.Stats{MAtk: 100}
			s.items[item.ID] = item
			controlled, _ := s.statuses.Get(1002, 11)
			m.Status.Apply(controlled, s.tick, p.ID)
			native, _ := s.skills.Get(10409, 1)
			d := assassinRegressionCast(t, s, p, UseSkill{Skill: 10409, Target: m.ID})
			var effect domain.SkillEffect
			for _, e := range native.DirectEffects() {
				if e.Kind == domain.EffectDamage {
					effect = e
				}
			}
			bp := int64(10000)
			if count >= 2 {
				bp += 6000
			}
			if count >= 3 {
				bp += 5000
			}
			num, den := effect.DamagePctNumerator, effect.DamagePctDenominator
			if den <= 0 {
				num, den = int64(effect.DamagePct), 1
			}
			want := int32(150 * num * bp / (100 * den * 10000))
			damage, hits := assassinRegressionDamage(s, m.ID, 10409)
			if damage != want || hits != 1 {
				t.Fatalf("magic attack base=%d/%d want%d native=%+v prepared=%+v", damage, hits, want, effect, d.DirectEffects())
			}
		})
	}
}

func TestUniversalRegressionPointTrapDoesNotInventPrimaryVictim(t *testing.T) {
	s, p, m := nativeRepairFixture(t, domain.Assassin, domain.Learned{10213: 1}, "TY-A34", "TY-A36")
	second := putMonster(s, 901, 10000, domain.Stats{}, 140, 100)
	s.monsters[second.ID] = second
	assassinRegressionCast(t, s, p, UseSkill{Skill: 10213, At: &m.Pos})
	for _, tr := range s.traps {
		s.triggerAssassinTrap(tr, false)
	}
	for _, id := range []domain.EntityID{m.ID, second.ID} {
		damage, hits := assassinRegressionDamage(s, id, 10213)
		if damage != 200 || hits != 1 {
			t.Fatalf("point trap invented primary/splash bonus for %d: %d/%d", id, damage, hits)
		}
	}
}

func TestAssassinRegressionAcceptedBombDoesNotBorrowNewTriplePower(t *testing.T) {
	for _, instant := range []bool{false, true} {
		t.Run(fmt.Sprint(instant), func(t *testing.T) {
			s, p, m := nativeRepairFixture(t, domain.Assassin, domain.Learned{10213: 1})
			if instant {
				assassinRegressionEnhancedBombWeapon(t, s, p)
			}
			p.Stats = domain.Stats{MinAtk: 100, MaxAtk: 100, Hit: 100000}
			p.MP, p.MaxMP = 10000, 10000
			s.onUseSkill(UseSkill{ID: p.ID, Skill: 10213, At: &m.Pos})
			if len(s.casts) != 1 {
				t.Fatal("native bomb not accepted")
			}
			assassinAddPower(s, p, "CK-A13", domain.SlotBody)
			now := s.now()
			s.stepCastsAt(now.Add(time.Minute), false)
			s.stepCastsAt(now.Add(2*time.Minute), false)
			if !instant {
				if len(s.traps) != 1 {
					t.Fatal("pending cast borrowed new triple power", len(s.traps))
				}
				for _, tr := range s.traps {
					s.triggerAssassinTrap(tr, false)
				}
			}
			damage, hits := assassinRegressionDamage(s, m.ID, 10213)
			if damage != 200 || hits != 1 {
				t.Fatalf("late power borrowed at release: %d/%d", damage, hits)
			}
		})
	}
}
