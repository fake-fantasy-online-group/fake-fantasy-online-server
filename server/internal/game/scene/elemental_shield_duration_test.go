package scene

import (
	"fmt"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func TestElementalShieldDurationDisplayAndLifetime(t *testing.T) {
	for _, tc := range []struct {
		skill  domain.SkillID
		status domain.StatusID
	}{{10425, 1037}, {10435, 1038}} {
		for _, rank := range []int32{1, 2, 5, 15, 20, 21, 27, 100} {
			t.Run(fmt.Sprintf("%d/rank%d", tc.skill, rank), func(t *testing.T) {
				s, p := behaviorAcceptancePlayer(t, 5)
				def, ok := s.skills.Get(tc.skill, rank)
				if !ok {
					t.Fatal("missing skill")
				}
				want := 30 * rank
				lines := s.skillOriginalEffectLines(def)
				line := fmt.Sprintf("保护持续时间: %d秒", want)
				if strings.Count(strings.Join(lines, "\n"), line) != 1 {
					t.Fatalf("duration missing or duplicated: %v", lines)
				}
				state, resolved := s.skillDescriptionStatus(def.Statuses[0])
				if !resolved || !s.applyStatusDef(p, state, p.ID) {
					t.Fatal("apply failed")
				}
				a := universalSoloActive(t, p, tc.status)
				if a.Def.DurationSec != want || a.ExpireAt-s.tick != domain.Ticks(int(want)*1000) {
					t.Fatalf("wrong lifetime: %+v want %d", a, want)
				}
				reflect := p.Status.ReflectPct(false)
				_, _, freeze, _, _, _, _ := p.Status.CombatTriggers()
				if tc.status == 1037 && reflect == 0 || tc.status == 1038 && freeze == 0 {
					t.Fatal("fixture has no shield effect")
				}
				originalExpiry := a.ExpireAt
				s.tick += domain.Ticks(1000)
				if !s.applyStatusDef(p, state, p.ID) {
					t.Fatal("recast failed")
				}
				refreshed := universalSoloActive(t, p, tc.status)
				if refreshed.ExpireAt != s.tick+domain.Ticks(int(want)*1000) || refreshed.ExpireAt <= originalExpiry {
					t.Fatal("recast did not refresh", refreshed)
				}
				if gone := p.Status.Expire(refreshed.ExpireAt - 1); len(gone) != 0 || !p.Status.Has(tc.status) {
					t.Fatal("expired early", gone)
				}
				if p.Status.ReflectPct(false) != reflect {
					t.Fatal("reflect changed before expiry")
				}
				_, _, beforeFreeze, _, _, _, _ := p.Status.CombatTriggers()
				if beforeFreeze != freeze {
					t.Fatal("freeze changed before expiry")
				}
				gone := p.Status.Expire(refreshed.ExpireAt)
				if len(gone) != 1 || gone[0] != tc.status || p.Status.Has(tc.status) {
					t.Fatal("not removed at exact boundary", gone)
				}
				_, _, afterFreeze, _, _, _, _ := p.Status.CombatTriggers()
				if p.Status.ReflectPct(false) != 0 || afterFreeze != 0 {
					t.Fatal("leftover shield effects after expiry")
				}
			})
		}
	}
}

func TestElementalShieldDurationSoftRanks(t *testing.T) {
	for _, id := range []domain.SkillID{10425, 10435} {
		s, _ := behaviorAcceptancePlayer(t, 5)
		hard := domain.Learned{id: 20}
		ranks, err := domain.ResolveSkillLoadout(5, hard, []domain.SkillRankBonus{{SourceID: "equipped", Scope: "single", SkillID: id, Levels: 7}}, nil, s.skills.ProgressionCatalog())
		if err != nil {
			t.Fatal(err)
		}
		rank, err := ranks[id].EngineRank()
		if err != nil || rank != 27 {
			t.Fatal("soft ranks not resolved", rank, err)
		}
		def, ok := s.skills.Get(id, rank)
		if !ok {
			t.Fatal("skill unresolved")
		}
		st, ok := s.skillDescriptionStatus(def.Statuses[0])
		if !ok || st.DurationSec != 810 {
			t.Fatal("soft rank duration", st)
		}
	}
}

// Exercise native cast admission and the actual focus and legendary producers,
// not synthetic duration multipliers. Both bonuses follow the new base once.
func TestElementalShieldDurationNativeBonuses(t *testing.T) {
	for _, tc := range []struct {
		skill  domain.SkillID
		status domain.StatusID
	}{{10425, 1037}, {10435, 1038}} {
		for _, rank := range []int32{1, 5, 20, 21} {
			for _, focus := range []bool{false, true} {
				for _, unique := range []bool{false, true} {
					t.Run(fmt.Sprintf("%d/rank%d/focus%t/unique%t", tc.skill, rank, focus, unique), func(t *testing.T) {
						powers := []string{}
						if unique {
							powers = append(powers, "TY-U11")
						}
						s, p, _ := nativeRepairFixture(t, domain.Warlock, domain.Learned{tc.skill: min(rank, 20), 10418: 1}, powers...)
						// The production native fixture supplies the legacy skill/status rows;
						// install progression so the same native cast path resolves high ranks.
						s.skills, s.statuses = nativeRestoredSceneFixtures(t)
						p.Player.EndgameGrowthV5 = true
						if rank > 20 {
							items := s.items
							item, gear := growthAllSkillGear(s)
							items[item.ID] = item
							s.items = items
							p.Player.Worn.Set(domain.SlotGlove, gear)
						}
						s.refreshStats(p)
						if got := s.rawSkillLevels(p)[tc.skill]; got != rank {
							t.Fatalf("effective rank %d want %d", got, rank)
						}
						p.MP = p.MaxMP
						wantMS := int64(30*rank) * 1000
						if focus {
							assassinRegressionCast(t, s, p, UseSkill{Skill: 10418, Target: p.ID})
							if !p.Status.Has(domain.StatusFocus) {
								t.Fatal("focus was not applied")
							}
							focusState := universalSoloActive(t, p, domain.StatusFocus)
							wantMS = wantMS * (100 + int64(focusState.Def.NextStatusDurationPct)) / 100
						}
						p.MP = p.MaxMP
						assassinRegressionCast(t, s, p, UseSkill{Skill: tc.skill, Target: p.ID})
						if unique {
							wantMS = wantMS * 150 / 100
						}
						a := universalSoloActive(t, p, tc.status)
						if (a.ExpireAt - s.tick).Millis() != wantMS {
							t.Fatalf("got %dms want %dms: %+v", (a.ExpireAt - s.tick).Millis(), wantMS, a)
						}
						if focus && p.Status.Has(domain.StatusFocus) {
							t.Fatal("focus not consumed")
						}
					})
				}
			}
		}
	}
}

// Magic mirror retains its existing constant 30-second legacy duration and
// post-rank15 one-second continuation; it must not inherit elemental timing.
func TestElementalShieldDurationDoesNotChangeMagicMirror(t *testing.T) {
	s, _ := behaviorAcceptancePlayer(t, 5)
	for _, rank := range []int32{1, 2, 5, 15, 20, 21, 27} {
		def, ok := s.skills.Get(10417, rank)
		if !ok {
			t.Fatal("missing mirror")
		}
		state, ok := s.skillDescriptionStatus(def.Statuses[0])
		want := int32(30) + max(int32(0), rank-15)
		if !ok || state.ID != 1011 || state.DurationSec != want {
			t.Fatal("mirror duration changed", rank, state)
		}
		lines := strings.Join(s.skillOriginalEffectLines(def), "\n")
		if strings.Contains(lines, "保护持续时间:") != (rank > 15) {
			t.Fatal("mirror tooltip policy changed", rank, lines)
		}
	}
}
