package scene

import (
	"fmt"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// ReqUseSkill permits both TargetID and Flag=1 coordinates. An aim ignored by
// caster-centered replacement geometry must not turn an illegal original
// single-target command into a legal primary (or its primary-only bonus).
func TestSharedReviewSelectedSingleAimRange(t *testing.T) {
	for _, tc := range []struct {
		name   string
		race   domain.Race
		skill  domain.SkillID
		powers []string
	}{
		{"circle", domain.Warrior, 10002, []string{"ZS-A02", "TY-A04", "TY-A34"}},
		{"rectangle", domain.Warrior, 10003, []string{"ZS-A16", "TY-A02", "TY-A34"}},
		{"holy_rectangle", domain.Healer, 10317, []string{"YS-A08", "TY-A02", "TY-A34"}},
	} {
		for _, aim := range []bool{false, true} {
			for _, inside := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/aim=%t/inside=%t", tc.name, aim, inside), func(t *testing.T) {
					s, p, m := nativeRepairFixture(t, tc.race, domain.Learned{tc.skill: 1}, tc.powers...)
					d, _ := s.skills.Get(tc.skill, 1)
					if d.RequiredWeaponType > 0 {
						stack := p.Player.Worn.At(domain.SlotWeapon)
						item := s.items[stack.Item]
						item.Equip.Type = d.RequiredWeaponType
						item.InventoryTab, item.InventoryTabKnown = 2, true
						s.items[stack.Item] = item
					}
					s.applyResolvedSkillModifiers(p, &d, p.Player.Char.Skills, s.rawSkillLevels(p))
					s.legendaryPrepareSkill(p, &d, m.ID)
					distance := float64(d.Dist)
					if !inside {
						distance++
					}
					s.aoi.Move(m, domain.Pos{MapID: p.Pos.MapID, X: p.Pos.X + distance, Y: p.Pos.Y})
					mp, hp := p.MP, m.HP
					cmd := UseSkill{ID: p.ID, Skill: tc.skill, Target: m.ID}
					if aim {
						at := m.Pos
						cmd.At = &at
					}
					s.onUseSkill(cmd)
					if !inside {
						if len(s.casts) != 0 || p.MP != mp {
							t.Fatalf("out-of-range single accepted: casts=%d mp=%d/%d", len(s.casts), p.MP, mp)
						}
						return
					}
					if len(s.casts) != 1 || s.casts[0].primary != m.ID {
						t.Fatal("legal boundary selected target rejected or primary lost")
					}
					sharedTargetFinishCasts(t, s)
					if m.HP >= hp {
						t.Fatal("legal selected target did not receive damage")
					}
				})
			}
		}
	}
}

func TestSharedReviewNativeAreaAndGroundAimRemainLegal(t *testing.T) {
	t.Run("native_area_uses_geometry", func(t *testing.T) {
		s, p, m := nativeRepairFixture(t, domain.Assassin, domain.Learned{10206: 1}, "CK-A06", "CK-A07", "TY-A34")
		s.aoi.Move(m, domain.Pos{MapID: p.Pos.MapID, X: p.Pos.X + 300, Y: p.Pos.Y})
		at := m.Pos
		s.onUseSkill(UseSkill{ID: p.ID, Skill: 10206, Target: m.ID, At: &at})
		if len(s.casts) != 1 || s.casts[0].primary != m.ID {
			t.Fatal("native area victim incorrectly constrained to unused Dist")
		}
		hp := m.HP
		sharedTargetFinishCasts(t, s)
		if m.HP >= hp {
			t.Fatal("native area lost damage")
		}
	})
	t.Run("target_centered_ground_point", func(t *testing.T) {
		s, p, m := nativeRepairFixture(t, domain.Warrior, domain.Learned{10002: 1}, "TY-A05", "TY-A04")
		d, _ := s.skills.Get(10002, 1)
		at := domain.Pos{MapID: p.Pos.MapID, X: p.Pos.X + float64(d.Dist), Y: p.Pos.Y}
		s.aoi.Move(m, domain.Pos{MapID: p.Pos.MapID, X: at.X + 20, Y: at.Y})
		s.onUseSkill(UseSkill{ID: p.ID, Skill: 10002, Target: m.ID, At: &at})
		if len(s.casts) != 1 {
			t.Fatal("legal ground point with a collateral enemy rejected")
		}
		hp := m.HP
		sharedTargetFinishCasts(t, s)
		if m.HP >= hp {
			t.Fatal("ground-centered collateral victim not hit")
		}
	})
}
