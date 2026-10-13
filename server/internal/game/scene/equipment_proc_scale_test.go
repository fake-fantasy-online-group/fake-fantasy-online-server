package scene

import (
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func TestEquipmentProcTinyPVPScaleDoesNotBecomeUnscaledDamage(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		scale, pvpScale, damage int32
	}{
		{"explicit_zero_pvp", 10000, 0, 1},
		{"tiny_pvp_truncates_to_zero", 1, 3500, 1},
		{"ordinary_pvp", 10000, 3500, 350},
	} {
		s := newCombatScene(t)
		join(t, s, 1, 100, "source", 100, 100)
		p := s.players[1]
		p.Stats = domain.Stats{MAtk: 1000}
		p.HP, p.MaxHP = 1000, 1000
		m := putMonster(s, 900, 10000, domain.Stats{}, 110, 100)
		s.skills = skillTable(domain.SkillDef{ID: 77777, Level: 1, HurtType: domain.HurtFire, Dist: 450, Effect: domain.SkillEffect{Kind: domain.EffectDamage, DamagePct: 100}})
		proc := &domain.EquipmentProc{ID: "configured", SkillID: 77777, FixedRank: 1, Payload: "damage", TargetRule: "hit_target", ScaleBP: tc.scale, PVPScaleBP: tc.pvpScale}
		c := s.equipmentEffects(p)
		c.effects = []domain.ActiveEquipmentEffect{{SourceUID: 99, Effect: domain.EquipmentEffect{Kind: "proc", Proc: proc}}}
		s.equipmentState().cache[p.ID] = c
		s.executeEquipmentProc(p, equipmentProcCandidate{active: c.effects[0], owner: p.ID, pvp: true, hit: equipmentHit{src: p.ID, dst: m.ID}})
		if dealt := 10000 - m.HP; dealt != tc.damage {
			t.Fatalf("%s dealt %d want %d", tc.name, dealt, tc.damage)
		}
	}
}
