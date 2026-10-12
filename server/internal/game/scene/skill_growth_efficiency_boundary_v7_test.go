package scene

import (
	"fmt"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// Exercise the accepted cast, actual debit, queued cooldown and ready-at clock.
// The former cleanse fixture is not evidence that healing crosses its cap.
func TestSixGrowthEfficiencyCapsPreserveActualCostAndCooldown(t *testing.T) {
	for _, tc := range []struct {
		id  domain.SkillID
		cap int32
	}{{10018, 5}, {10227, 5}, {10432, 5}, {10301, 19}, {10305, 11}, {10309, 10}} {
		for _, zero := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/zero=%t", tc.id, zero), func(t *testing.T) {
				var previousCost, previousCooldown int32
				for _, rank := range []int32{tc.cap, tc.cap + 1, tc.cap + 2} {
					identity, _ := domain.V5SkillIdentity(tc.id)
					s, p := behaviorAcceptancePlayer(t, identity.Profession)
					join(t, s, 2, 200, "效率边界目标", 110, 100)
					target := s.players[2]
					p.Player.Party, target.Player.Party = 7, 7
					if zero {
						// Zero-valued original resources must remain absent after the handoff.
						for r := int32(1); r <= 5; r++ {
							key := domain.SkillKey{ID: tc.id, Level: r}
							d := s.skills[key]
							d.MPCost, d.CooldownMS = 0, 0
							s.skills[key] = d
						}
					}
					p.Player.Char.Skills = domain.Learned{tc.id: min(rank, 20)}
					s.skillRawCache = map[domain.EntityID]skillRawCacheEntry{p.ID: {key: skillRawFingerprint(p), levels: domain.Learned{tc.id: rank}}}
					d, ok := s.skills.Get(tc.id, rank)
					if !ok {
						t.Fatal("missing skill", rank)
					}
					s.applyResolvedSkillModifiers(p, &d, p.Player.Char.Skills, s.rawSkillLevels(p))
					if rank > tc.cap && (d.MPCost > previousCost || d.CooldownMS > previousCooldown) {
						t.Fatalf("cap handoff increased resources at rank%d: MP%d→%d CD%d→%d", rank, previousCost, d.MPCost, previousCooldown, d.CooldownMS)
					}
					if zero && (d.MPCost != 0 || d.CooldownMS != 0) {
						t.Fatal("invented absent cost or cooldown", d.MPCost, d.CooldownMS)
					}
					cmd := UseSkill{ID: p.ID, Skill: tc.id, Target: p.ID}
					if tc.id == 10305 {
						target.HP = 0
						cmd.Target = target.ID
					}
					if tc.id == 10227 {
						cmd.Target = target.ID
					}
					if tc.id == 10432 {
						point := domain.Pos{MapID: s.id.MapID, X: 110, Y: 100}
						cmd.At = &point
					}
					p.MP, p.MaxMP = 100000, 100000
					before := p.MP
					s.onUseSkill(cmd)
					if len(s.casts) != 1 || before-p.MP != d.MPCost {
						t.Fatalf("rank%d admitted=%d actual MP=%d want%d", rank, len(s.casts), before-p.MP, d.MPCost)
					}
					if s.casts[0].cooldownMS != d.CooldownMS || s.casts[0].def.CooldownMS != d.CooldownMS {
						t.Fatal("queued cooldown mismatch", rank, s.casts[0].cooldownMS, d.CooldownMS)
					}
					ticks := domain.Ticks(int(d.CooldownMS))
					if !p.Player.SkillReadyAt(tc.id, s.tick+ticks) {
						t.Fatal("cooldown exceeds actual payload", rank)
					}
					if ticks > 0 && p.Player.SkillReadyAt(tc.id, s.tick+ticks-1) {
						t.Fatal("cooldown shorter than payload", rank)
					}
					previousCost, previousCooldown = d.MPCost, d.CooldownMS
					t.Logf("rank%d actual MP=%d cooldown=%dms (%d ticks)", rank, d.MPCost, d.CooldownMS, ticks)
				}
			})
		}
	}
}
