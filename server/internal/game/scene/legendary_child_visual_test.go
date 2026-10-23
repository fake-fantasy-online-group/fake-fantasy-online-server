package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
	"testing"
)

func TestLegendaryChildNativeVisualBoundaries(t *testing.T) {
	for _, mode := range []string{"landed", "immune", "missing", "dead", "recursive", "suppressed", "area", "native-table"} {
		t.Run(mode, func(t *testing.T) {
			s, p, m := legendaryMeleeFixture(t, "ZS-A03")
			d := domain.SkillDef{ID: 10011, Kind: domain.SkillSingle, Dist: 500, TargetEnemy: true, HurtType: domain.HurtPhysical, HitEffect: "native_hit", Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100, GuaranteedHit: true}}}
			c := s.legendaryContexts(p, m, combat.Blow{SkillID: 10011}, event.DamageDealt{})[0]
			if mode == "native-table" {
				d.HitEffect = ""
			}
			target := m.ID
			opt := legendaryChildOptions{TargetOnly: true}
			wantHits := 1
			if mode == "immune" {
				s.defs = spawn.MapDefs{m.Monster.TypeID: {ID: m.Monster.TypeID, ImmuneSkills: map[domain.SkillID]struct{}{d.ID: {}}}}
			}
			if mode == "missing" {
				target = 99999
			}
			if mode == "dead" {
				m.HP = 0
			}
			if mode == "recursive" {
				s.legendaryState().child = &legendaryChildFrame{Producers: map[string]bool{c.Power.ID: true}}
			}
			if mode == "suppressed" {
				opt.SuppressVisual = true
			}
			if mode == "area" {
				putMonster(s, 901, 100000, domain.Stats{}, 140, 100)
				d.Kind = domain.SkillArea
				d.Area = domain.SkillAreaTargeting{Shape: domain.SkillAreaCircle, Center: domain.SkillAreaCenterCaster, Radius: 150}
				opt.TargetOnly = false
				wantHits = 2
			}
			s.outbox = nil
			beforeMP := p.MP
			c.ChildSkill(target, d, opt)
			releases, hits, casts := 0, 0, 0
			seen := map[domain.EntityID]bool{}
			for _, q := range s.outbox {
				switch e := q.ev.(type) {
				case event.ChildSkillReleased:
					if e.Mode == 1 {
						hits++
						if mode != "native-table" || e.Skill != d.ID || e.Target != m.ID || e.Center != m.Pos {
							t.Fatalf("wrong native impact %+v", e)
						}
						continue
					}
					releases++
					center := m.Pos
					if mode == "area" {
						center = p.Pos
					}
					if e.Caster != p.ID || e.Target != m.ID || e.Skill != d.ID || e.Origin != p.Pos || e.Center != center {
						t.Fatalf("wrong release %+v", e)
					}
				case event.SkillHitEffect:
					hits++
					if seen[e.Target] || e.Effect != "native_hit" {
						t.Fatalf("duplicate/wrong hit %+v", e)
					}
					seen[e.Target] = true
				case event.SkillCastChanged:
					casts++
				}
			}
			wantRelease := 1
			switch mode {
			case "immune":
				wantHits = 0
			case "missing", "dead", "recursive", "suppressed":
				wantRelease = 0
				wantHits = 0
			}
			if releases != wantRelease || hits != wantHits || casts != 0 || p.MP != beforeMP {
				t.Fatalf("release/hit/cast=%d/%d/%d want%d/%d/0", releases, hits, casts, wantRelease, wantHits)
			}
		})
	}
}

func TestLegendaryEnemiesLivingBeforeLimit(t *testing.T) {
	s, p, dead := legendaryMeleeFixture(t, "ZS-A03")
	dead.HP = 0
	a := putMonster(s, 901, 100, domain.Stats{}, 140, 100)
	b := putMonster(s, 902, 100, domain.Stats{}, 150, 100)
	c := s.legendaryContexts(p, a, combat.Blow{}, event.DamageDealt{})[0]
	got := c.Enemies(dead.Pos, 200, 1)
	if len(got) != 1 || got[0].ID != a.ID {
		t.Fatalf("corpse occupied closest live slot: %+v", got)
	}
	got = c.Enemies(dead.Pos, 200, 2)
	if len(got) != 2 || got[1].ID != b.ID {
		t.Fatal("live limit/order lost")
	}
}
