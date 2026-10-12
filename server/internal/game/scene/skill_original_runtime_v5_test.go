package scene

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

func originalRuntimeRowsV5(t *testing.T) []domain.SkillDef {
	t.Helper()
	b, err := os.ReadFile("../../../../docs/daoist-endgame/v5/skill-original-ranks.json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []domain.SkillDef
	if err = json.Unmarshal(b, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 745 {
		t.Fatalf("original rows=%d, want745", len(rows))
	}
	return rows
}

// Every original damage segment is checked at the real combat seam. This catches
// both the previous rank1..5-to20 compression and invented fixed-damage carriers.
func TestV5AllOriginalDamageRanksReachRealStrike(t *testing.T) {
	skills, statuses := realV5SkillFixtures(t)
	seen := map[domain.SkillID]int{}
	for _, original := range originalRuntimeRowsV5(t) {
		seen[original.ID]++
		def, ok := skills.Get(original.ID, original.Level)
		if !ok {
			t.Fatal("original rank unresolved", original.ID, original.Level)
		}
		wantEffects := original.DirectEffects()
		if !reflect.DeepEqual(def.DirectEffects(), wantEffects) {
			t.Fatal("original ordered effects changed", original.ID, original.Level)
		}
		for i, want := range wantEffects {
			if want.Kind != domain.EffectDamage {
				continue
			}
			t.Run(fmt.Sprintf("%d/%d/%d", original.ID, original.Level, i), func(t *testing.T) {
				s := newTestScene(t, nil)
				s.skills, s.statuses = skills, statuses
				join(t, s, 1, 100, "原始伤害", 100, 100)
				p := s.players[1]
				p.Stats = domain.Stats{MinAtk: 1000, MaxAtk: 1000, MAtk: 1000}
				target := &entity.Entity{ID: 999, Kind: domain.KindMonster, HP: 1000000, MaxHP: 1000000, Monster: &entity.Monster{}}
				s.entities[target.ID] = target
				effect := def.DirectEffects()[i]
				blow := combat.Blow{Magic: def.HurtType != domain.HurtNone && def.HurtType != domain.HurtPhysical, HurtType: def.HurtType, SkillPct: effect.DamagePct, SkillPctNumerator: effect.DamagePctNumerator, SkillPctDenominator: effect.DamagePctDenominator, FlatDamage: effect.DamageFlat, GuaranteedHit: true}
				ev, _, _ := s.resolveStrikeStatuses(p, target, &combat.HitTracker{}, blow)
				wantDamage := 10*want.DamagePct + want.DamageFlat
				if ev.Amount != wantDamage {
					t.Fatal("original real damage mismatch", ev.Amount, wantDamage, effect)
				}
			})
		}
	}
	if len(seen) != 149 {
		t.Fatal("wrong class-battle catalogue size", len(seen))
	}
	for id, count := range seen {
		if count != 5 {
			t.Fatal("missing original ranks", id, count)
		}
	}
}

// Original target sets are the oracle, never the new synergy graph. At every
// sampled rank only those skills may change; each advertised native target must
// still receive the modifier in the actual cast preparation path.
func TestV5OriginalPassiveScopeThroughRuntime(t *testing.T) {
	skills, statuses := realV5SkillFixtures(t)
	for _, original := range originalRuntimeRowsV5(t) {
		if original.Level != 5 || original.Kind != domain.SkillPassive {
			continue
		}
		targets := map[domain.SkillID]bool{}
		globalCast := false
		globalInterrupt := false
		for _, m := range original.PassiveModifiers {
			if m.TargetSkill != 0 {
				targets[m.TargetSkill] = true
			}
			globalCast = globalCast || m.Kind == domain.PassiveCastTimeReductionPct
			globalInterrupt = globalInterrupt || m.Kind == domain.PassiveCastInterruptReductionPct
		}
		for _, m := range original.PassiveTriggers {
			targets[m.TargetSkill] = true
		}
		for _, m := range original.PassiveTriggerEffects {
			targets[m.TargetSkill] = true
		}
		for _, m := range original.PassiveStatusAffixes {
			targets[m.TargetSkill] = true
		}
		for _, rank := range []int32{1, 2, 3, 4, 5, 20, 100} {
			t.Run(fmt.Sprintf("%d/%d", original.ID, rank), func(t *testing.T) {
				s := newTestScene(t, nil)
				s.skills, s.statuses = skills, statuses
				join(t, s, 1, 100, "被动范围", 100, 100)
				p := s.players[1]
				p.Player.Char.Race = domain.Race(original.Prof - 1)
				for _, id := range domain.V5SkillIDs() {
					base, ok := skills.Get(id, 5)
					if !ok || base.Prof != original.Prof || !base.Kind.Active() {
						continue
					}
					a, b := base, base
					s.applyPassiveSkillModifiersWithLevels(p, &a, domain.Learned{})
					s.applyPassiveSkillModifiersWithLevels(p, &b, domain.Learned{original.ID: rank})
					changed := !reflect.DeepEqual(a, b)
					wantChange := targets[id] || (globalCast || globalInterrupt && rank > 5) && base.PrepareMS > 0
					if changed != wantChange {
						t.Fatalf("passive%d rank%d target%d changed=%t, original scope=%t", original.ID, rank, id, changed, wantChange)
					}
					if b.PreserveInvisibleChanceBP < 0 || b.PreserveInvisibleChanceBP > 10000 || b.WeaponDurabilityReductionPct < 0 || b.WeaponDurabilityReductionPct > 100 {
						t.Fatal("unbounded passive chance/reduction", b)
					}
					for _, app := range b.Statuses {
						if app.ChanceBP < 0 || app.ChanceBP > 10000 {
							t.Fatal("unbounded native trigger", app)
						}
					}
				}
			})
		}
	}
}

func TestV5OriginalRankOneHealingSurvivesTrainingFractions(t *testing.T) {
	skills, statuses := realV5SkillFixtures(t)
	for _, tc := range []struct {
		name string
		hard domain.Learned
		want int32
	}{
		{"base", domain.Learned{10301: 1}, 200},
		{"flat", domain.Learned{10301: 1, 10302: 20}, 240},
		{"power", domain.Learned{10301: 1, 10304: 20}, 240},
		{"flat_then_power", domain.Learned{10301: 1, 10302: 20, 10304: 20}, 288},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestScene(t, nil)
			join(t, s, 1, 100, "治疗协同", 100, 100)
			p := s.players[1]
			p.Player.Char.Race = domain.Healer
			p.Player.Char.Skills = tc.hard
			s.skills, s.statuses = skills, statuses
			def, ok := skills.Get(10301, 1)
			if !ok {
				t.Fatal("rank1 heal missing")
			}
			s.applyTrainingSynergies(p, &def)
			s.applyPassiveSkillModifiers(p, &def)
			target := &entity.Entity{ID: 999, Kind: domain.KindMonster, HP: 1, MaxHP: 1000, Monster: &entity.Monster{}}
			s.entities[target.ID] = target
			if !s.applySkill(p, target, def, 0, false) || target.HP != 1+tc.want {
				t.Fatal("training erased original heal or recipient percentage", target.HP, tc.want, def.Effect)
			}
		})
	}
}

func TestV5OriginalAcceptedCastKeepsRankAndSynergySnapshot(t *testing.T) {
	for _, acceptedRank := range []int32{1, 5, 20} {
		t.Run(fmt.Sprint(acceptedRank), func(t *testing.T) {
			s := newTestScene(t, nil)
			join(t, s, 1, 100, "已受理施法", 100, 100)
			p := s.players[1]
			ch := p.Player.Char
			ch.Race, ch.Level, ch.SkillAllocationVersion = domain.Warlock, 100, 5
			ch.Skills = domain.Learned{10401: acceptedRank, 10402: 20}
			s.skills, s.statuses = realV5SkillFixtures(t)
			p.Stats = domain.Stats{MAtk: 1000}
			p.MP, p.MaxMP = 10000, 10000
			target := putMonster(s, 900, 100000, domain.Stats{}, 110, 100)
			now := time.Unix(1700000000, 0)
			s.now = func() time.Time { return now }
			s.onUseSkill(UseSkill{ID: p.ID, Skill: 10401, Target: target.ID})
			if len(s.casts) != 1 {
				t.Fatal("original cast not accepted", acceptedRank)
			}
			// Neither a later rank/source loss nor stronger live offense may rewrite an
			// already accepted one-off cast; persistent statuses use separate down-only tests.
			ch.Skills = domain.Learned{10401: 1}
			p.Stats.MAtk = 10000
			now = now.Add(2 * time.Second)
			s.stepCastsAt(now, false)
			now = now.Add(time.Second)
			s.stepCastsAt(now, false)
			want := (208 + 8*(acceptedRank-1)) * 12
			if len(s.casts) != 0 || target.HP != 100000-want {
				t.Fatal("accepted original payload changed after source loss", len(s.casts), target.HP, want)
			}
		})
	}
}
