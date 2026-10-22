package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"testing"
	"time"
)

// Exercise accepted casts rather than invoking the weapon healing helper directly.
func TestSpecialWeaponHealingActualCastSetIntegration(t *testing.T) {
	for _, pieces := range []int{2, 4} {
		for _, base := range []domain.ItemID{1323, 1325} {
			for _, active := range []bool{false, true} {
				t.Run(fmt.Sprintf("weapon%d/pieces%d/active%t", base, pieces, active), func(t *testing.T) {
					equipped := pieces
					if pieces == 4 {
						equipped = 5
					} // The enhanced staff replaces one fixture set piece.
					s, p, tid := casterSetFixture(t, 2, equipped)
					specialWeaponEquip(t, s, p, base)
					if n := s.classSetPieces(p, 2); n != int32(pieces) {
						t.Fatalf("actual set pieces %d", n)
					}
					join(t, s, 2, 101, "healing ally", 300, 100)
					ally := s.entities[2]
					p.Player.Party = 7
					ally.Player.Party = 7
					native, statuses := realV5SkillFixtures(t)
					s.statuses = statuses
					skill := domain.SkillID(10320)
					if base == 1325 {
						skill = 10316
					}
					def, ok := native.Get(skill, 5)
					if !ok {
						t.Fatal("missing native skill")
					}
					s.skills[domain.SkillKey{ID: skill, Level: 5}] = def
					p.Player.Char.Skills[skill] = 5
					s.invalidateCombatAttributes(p)
					if ally.Status == nil {
						ally.Status = domain.NewStatusSet()
					}
					if active {
						if base == 1323 {
							ally.Status.Apply(domain.StatusDef{ID: domain.StatusDamageShield, Level: 1, DurationSec: 30, DamageShield: 100, TypeKnown: true, Type: domain.StatusGood}, s.tick, p.ID)
						} else {
							ally.Status.Apply(domain.StatusDef{ID: 1006, Level: 1, DurationSec: 30, TypeKnown: true, Type: domain.StatusBad}, s.tick, p.ID)
						}
					}
					s.refreshStats(ally)
					ally.HP = 1
					p.MaxHP = 10000
					p.HP = 1
					p.MaxMP = 100000
					p.MP = 100000
					p.Stats.MAtk = 100
					target := s.entities[tid]
					target.MaxHP = 100000
					target.HP = 100000
					target.Stats.MDef = 0
					before := target.HP
					s.exec(UseSkill{ID: p.ID, Skill: skill, Target: ally.ID})
					if len(s.casts) != 1 {
						t.Fatalf("native cast not accepted: %d", len(s.casts))
					}
					if s.casts[0].def.SpecialWeaponBaseID != base {
						t.Fatal("accepted cast lost weapon modifier")
					}
					s.stepCastsAt(s.now().Add(2*time.Second), false)
					s.stepCastsAt(s.now().Add(3*time.Second), false)
					wantHeal := int32(0)
					if active {
						wantHeal = 60
						if base == 1325 {
							wantHeal = ally.MaxHP * 3 / 100
						}
					}
					if p.HP != 1+wantHeal {
						t.Fatalf("2-piece nominal self-copy got %d want %d; ally HP=%d/%d", p.HP-1, wantHeal, ally.HP, ally.MaxHP)
					}
					if active && ally.HP <= 1 {
						t.Fatal("original recipient received no healing")
					}
					wantDamage := int32(0)
					if active && pieces == 4 {
						wantDamage = 100
					}
					if got := before - target.HP; got != wantDamage {
						t.Fatalf("4-piece bolt damage got %d want %d", got, wantDamage)
					}
					if s.equipmentState().classSetTriggered != 0 || len(s.casts) != 0 {
						t.Fatal("generated payload recursed or left pending casts")
					}
					after := target.HP
					s.stepCastsAt(s.now().Add(4*time.Second), false)
					if target.HP != after {
						t.Fatal("one healing cast emitted another reciprocal bolt")
					}
				})
			}
		}
	}
}
