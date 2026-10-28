package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"testing"
)

func TestEquipmentSharedAttackProcActualTriggerBoundary(t *testing.T) {
	for _, skill := range []int32{0, 10402} {
		t.Run(map[bool]string{true: "basic-positive", false: "skill-negative"}[skill == 0], func(t *testing.T) {
			s := newCombatScene(t)
			join(t, s, 1, 100, "attack proc", 100, 100)
			p := s.players[1]
			st := wearEquipmentV5(t, s, p, 1306, 9001)
			st.Endgame.UniqueContentVersion = 5
			p.Player.Worn.Set(domain.SlotTwoHand, st)
			p.Stats = domain.Stats{MinAtk: 100, MaxAtk: 100, MAtk: 100, Hit: 100000}
			s.skills = skillTable(domain.SkillDef{ID: 10401, Level: 10, HurtType: domain.HurtFire, Dist: 450, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 200}})
			cache := s.equipmentEffects(p)
			found := false
			for i, a := range cache.effects {
				if a.Effect.Proc == nil {
					continue
				}
				proc := *a.Effect.Proc
				proc.Trigger = "on_attack"
				proc.ChanceBP = 10000
				proc.RequireMagic = false
				cache.effects[i].Effect.Proc = &proc
				found = true
			}
			if !found {
				t.Fatal("fixture missing proc")
			}
			s.equipmentState().cache[p.ID] = cache
			m := putMonster(s, 900, 10000, domain.Stats{}, 110, 100)
			done := s.beginEquipmentRoot(p.ID, m.ID, domain.EquipmentCombatStats{})
			s.resolveStrikeStatuses(p, m, &combat.HitTracker{}, combat.Blow{SkillID: skill, GuaranteedHit: true})
			done()
			count := 0
			for _, out := range s.outbox {
				if hit, ok := out.ev.(event.DamageDealt); ok && hit.Origin == domain.DamageProcOrigin {
					count++
					if hit.Amount != 200 || hit.Dst != m.ID {
						t.Fatalf("wrong actual proc %+v", hit)
					}
				}
			}
			want := 0
			if skill == 0 {
				want = 1
			}
			if count != want {
				t.Fatalf("skill=%d actual proc count=%d want=%d", skill, count, want)
			}
		})
	}
}
