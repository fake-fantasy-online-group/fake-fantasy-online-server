package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"testing"
	"time"
)

// Equip-compatible builds and actual monster attacks through the native queue.
func TestLegendaryManaAfterAllIncomingReductions(t *testing.T) {
	for _, tc := range []struct {
		race      domain.Race
		power     string
		remaining int32
	}{
		{domain.Warlock, "SS-D10", 650}, {domain.Assassin, "CK-D01", 800}, {domain.Swordsman, "JK-D05", 600},
	} {
		for _, mana := range []int32{0, 100, 1000} {
			for _, shield := range []int32{0, 100, 10000} {
				t.Run(fmt.Sprintf("%s/mana%d/shield%d", tc.power, mana, shield), func(t *testing.T) {
					learned := domain.Learned{}
					if tc.race == domain.Assassin {
						learned[10208] = 1
					}
					s, p, m := nativeRepairFixture(t, tc.race, learned, tc.power, "TY-D11")
					if tc.race == domain.Assassin {
						nativeRepairCast(t, s, p, 10208, p.ID)
					}
					if shield > 0 {
						p.Status.Apply(domain.StatusDef{ID: 199970, DurationSec: 30, DamageShield: shield}, s.tick, p.ID)
					}
					s.refreshStats(p)
					p.Stats = domain.Stats{}
					p.HP, p.MP = p.MaxHP, mana
					beforeHP := p.HP
					m.Stats = domain.Stats{MinAtk: 1000, MaxAtk: 1000, Hit: 100000}
					s.attacks = append(s.attacks, pendingAttack{src: m.ID, dst: p.ID, blow: combat.Blow{GuaranteedHit: true}})
					s.stepCombatAt(s.now().Add(time.Second), false)
					spent := min(mana, tc.remaining*30/100)
					remaining := tc.remaining - spent
					wantHP, wantShield := max(int32(0), remaining-shield), max(int32(0), shield-remaining)
					if hp, mp, left := beforeHP-p.HP, mana-p.MP, universalShieldValue(p); hp != wantHP || mp != spent || left != wantShield {
						t.Fatalf("HP/MP/shield=%d/%d/%d want%d/%d/%d", hp, mp, left, wantHP, spent, wantShield)
					}
				})
			}
		}
	}
}

// Native restored burn, actual applyStatusDef/stepStatus, and optional source
// disappearance. The authored DOT fields are never rewritten for the test.
func TestLegendaryManaAfterNativePeriodicReductions(t *testing.T) {
	for _, extra := range []bool{false, true} {
		for _, leaves := range []bool{false, true} {
			for _, mana := range []int32{0, 1, 1000} {
				for _, shield := range []int32{0, 5} {
					t.Run(fmt.Sprintf("extra%v/left%v/mana%d/shield%d", extra, leaves, mana, shield), func(t *testing.T) {
						powers := []string{"SS-D10", "TY-D11"}
						if extra {
							powers = append(powers, "TY-D19")
						}
						s, p, m := nativeRepairFixture(t, domain.Warlock, domain.Learned{}, powers...)
						var burn domain.StatusDef
						for _, d := range s.statuses {
							if d.ID == 1036 && (burn.ID == 0 || d.Level < burn.Level) {
								burn = d
							}
						}
						if burn.ID == 0 || !s.applyStatusDef(p, burn, m.ID) {
							t.Fatal("native burn unavailable")
						}
						if shield > 0 {
							p.Status.Apply(domain.StatusDef{ID: 199971, DurationSec: 30, DamageShield: shield}, s.tick, p.ID)
						}
						s.refreshStats(p)
						p.Stats = domain.Stats{}
						p.HP, p.MP = p.MaxHP, mana
						beforeHP := p.HP
						var raw int32
						for _, r := range burn.HPChanges {
							raw -= resourceDelta(r.Value, r.Mode, p.MaxHP)
						}
						for _, r := range burn.Affixes {
							if r.Attr == domain.AttrHPRegen {
								raw -= resourceDelta(r.Value, r.Mode, p.MaxHP)
							}
						}
						if raw <= 0 {
							t.Fatal("native burn has no damage")
						}
						if leaves {
							delete(s.entities, m.ID)
							delete(s.monsters, m.ID)
						}
						s.tick += domain.Ticks(int(burn.IntervalSec) * 1000)
						s.stepStatus()
						if extra {
							raw = raw * 75 / 100
						}
						reduced := raw * 65 / 100
						spent := min(mana, reduced*30/100)
						remaining := reduced - spent
						wantHP, wantShield := max(int32(0), remaining-shield), max(int32(0), shield-remaining)
						if hp, mp, left := beforeHP-p.HP, mana-p.MP, universalShieldValue(p); hp != wantHP || mp != spent || left != wantShield {
							t.Fatalf("HP/MP/shield=%d/%d/%d want%d/%d/%d", hp, mp, left, wantHP, spent, wantShield)
						}
					})
				}
			}
		}
	}
}

func TestLegendaryManaDoesNotHideLargeHitThreshold(t *testing.T) {
	s, p, m := nativeRepairFixture(t, domain.Warlock, domain.Learned{10435: 1}, "SS-D02", "TY-D11")
	nativeRepairCast(t, s, p, 10435, p.ID)
	s.refreshStats(p)
	p.HP, p.MP = p.MaxHP, 1000
	beforeHP := p.HP
	p.Stats, m.Stats = domain.Stats{}, domain.Stats{MAtk: 1000}
	// A real native monster spell uses the same damage consumer as AI casts;
	// magic avoids the shield's separate physical reduction.
	d, ok := s.skills.Get(10401, 1)
	if !ok {
		t.Fatal("native fire spell missing")
	}
	var raw int32
	for _, effect := range d.DirectEffects() {
		if effect.Kind != domain.EffectDamage {
			continue
		}
		if effect.DamagePctDenominator > 0 {
			raw = int32(int64(m.Stats.MAtk) * effect.DamagePctNumerator / (100 * effect.DamagePctDenominator))
		} else {
			raw = m.Stats.MAtk * effect.DamagePct / 100
		}
	}
	if raw <= 800 {
		t.Fatal("native spell does not cross large-hit threshold", raw)
	}
	if !s.applySkill(m, p, d, 0, false) {
		t.Fatal("native monster spell did not land")
	}
	reduced := raw * 45 / 100
	spent := min(int32(1000), reduced*30/100)
	if hp, mp := beforeHP-p.HP, int32(1000)-p.MP; hp != reduced-spent || mp != spent {
		t.Fatalf("large-hit damage/mana=%d/%d want%d/%d", hp, mp, reduced-spent, spent)
	}
}

// These rows validate the shared proc/child/set-consumer boundary only. Player
// legendary children against equipped players are not currently reachable PvP.
func TestLegendaryManaDerivedConsumerBoundaries(t *testing.T) {
	for _, kind := range []string{"proc", "child", "set_proc"} {
		for _, mana := range []int32{0, 100, 1000} {
			t.Run(fmt.Sprintf("%s/mana%d", kind, mana), func(t *testing.T) {
				s, p, m := nativeRepairFixture(t, domain.Warlock, domain.Learned{}, "SS-D10", "TY-D11")
				p.Status.Apply(domain.StatusDef{ID: 199972, DurationSec: 30, DamageShield: 100}, s.tick, p.ID)
				s.refreshStats(p)
				p.HP, p.MP = p.MaxHP, mana
				p.Stats, m.Stats = domain.Stats{}, domain.Stats{MinAtk: 1000, MaxAtk: 1000}
				b := combat.Blow{Origin: domain.DamageProcOrigin, ProcDepth: 1, GuaranteedHit: true, LegendaryChild: kind == "child"}
				if kind == "set_proc" {
					s.equipmentState().classSetTriggered = 1
				}
				ev, _, _ := s.resolveStrikeStatuses(m, p, &combat.HitTracker{}, b)
				spent := min(mana, int32(195))
				want := int32(650) - spent - 100
				if ev.Amount != want || mana-p.MP != spent || universalShieldValue(p) != 0 {
					t.Fatalf("derived damage/mana/shield=%d/%d/%d want%d/%d/0", ev.Amount, mana-p.MP, universalShieldValue(p), want, spent)
				}
			})
		}
	}
}

// Both actors receive the current native fire shield. A real player attack
// triggers the monster's native reflection, which must not reflect recursively.
func TestLegendaryManaActualNativeReflection(t *testing.T) {
	for _, reduction := range []bool{false, true} {
		for _, mana := range []int32{0, 1, 1000} {
			for _, shield := range []int32{0, 10, 100} {
				t.Run(fmt.Sprintf("reduction%v/mana%d/shield%d", reduction, mana, shield), func(t *testing.T) {
					powers := []string{"TY-D11"}
					if reduction {
						powers = append(powers, "SS-D10")
					}
					s, p, m := nativeRepairFixture(t, domain.Warlock, domain.Learned{10425: 1}, powers...)
					d, ok := s.skills.Get(10425, 1)
					if !ok || !s.applySkill(m, m, d, 0, false) {
						t.Fatal("native monster fire shield unavailable")
					}
					nativeRepairCast(t, s, p, 10425, p.ID)
					if m.Status.ReflectPct(false) <= 0 || p.Status.ReflectPct(false) <= 0 {
						t.Fatal("native fire shields have no reflection")
					}
					if shield > 0 {
						p.Status.Apply(domain.StatusDef{ID: 199973, DurationSec: 30, DamageShield: shield}, s.tick, p.ID)
					}
					s.refreshStats(p)
					p.HP, p.MP = p.MaxHP, mana
					p.Stats = domain.Stats{MinAtk: 1000, MaxAtk: 1000, Hit: 100000}
					p.Look.AtkDist = 200
					beforeHP := p.HP
					s.outbox = nil
					s.onAttack(Attack{ID: p.ID, Target: m.ID, Blow: combat.Blow{GuaranteedHit: true}})
					s.stepCombatAt(s.now().Add(time.Second), false)
					var direct int32
					reflections := 0
					for _, out := range s.outbox {
						if e, ok := out.ev.(event.DamageDealt); ok {
							if e.Dst == m.ID && e.Origin == domain.DamageNormalOrigin {
								direct += e.Amount
							}
							if e.Origin == domain.DamageReflectionOrigin {
								reflections++
							}
						}
					}
					if direct <= 0 {
						t.Fatal("native player attack did not reach monster")
					}
					reduced := direct * m.Status.ReflectPct(false) / 100
					// Native mitigation subtracts its truncated reduction; preserve that rounding.
					reduced -= reduced * p.Status.DamageReductionPct(false) / 100
					if reduction {
						reduced = reduced * 65 / 100
					}
					spent := min(mana, reduced*30/100)
					afterMana := reduced - spent
					wantHP, wantShield := max(int32(0), afterMana-shield), max(int32(0), shield-afterMana)
					wantEvents := 0
					if wantHP > 0 {
						wantEvents = 1
					}
					if hp, mp, left := beforeHP-p.HP, mana-p.MP, universalShieldValue(p); hp != wantHP || mp != spent || left != wantShield || reflections != wantEvents {
						t.Fatalf("reflection HP/MP/shield/events=%d/%d/%d/%d want%d/%d/%d/%d", hp, mp, left, reflections, wantHP, spent, wantShield, wantEvents)
					}
				})
			}
		}
	}
}
