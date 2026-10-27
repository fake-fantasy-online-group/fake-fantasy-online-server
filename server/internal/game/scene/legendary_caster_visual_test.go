package scene

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func casterVisualEvents(s *Scene) (hits []event.SkillHitEffect, ground []event.GroundEffect, releases []event.ChildSkillReleased) {
	for _, p := range s.outbox {
		switch e := p.ev.(type) {
		case event.SkillHitEffect:
			hits = append(hits, e)
		case event.GroundEffect:
			ground = append(ground, e)
		case event.ChildSkillReleased:
			releases = append(releases, e)
		}
	}
	return
}

func TestLegendaryCasterVisualSettlementsFollowActualDamage(t *testing.T) {
	for _, id := range []string{"SS-A02", "SS-A07", "SS-A11", "SS-A16", "SS-A19", "SS-A28", "YS-A02", "YS-A15"} {
		t.Run(id, func(t *testing.T) {
			s, c := legendaryCasterFixture(t, id)
			s.outbox = nil
			root := &equipmentRoot{id: c.RootID}
			s.equipmentState().root = root
			c.Damage(c.Target, 100, true, true)
			hits, _, _ := casterVisualEvents(s)
			if len(hits) != 0 {
				t.Fatal("queued damage emitted premature impact")
			}
			s.equipmentState().root = nil
			s.legendaryRootFinished(root)
			hits, _, _ = casterVisualEvents(s)
			fx, _, _ := legendaryVisualEffect(id, "stage-01")
			if len(hits) != 1 || hits[0].Target != c.Target.ID || hits[0].Effect != fx {
				t.Fatalf("wrong actual settlement effect: %+v", hits)
			}
			s.outbox = nil
			c.Target.Status.Apply(domain.StatusDef{ID: 99991, DurationSec: 10, Invulnerable: true}, s.tick, c.Target.ID)
			c.Damage(c.Target, 100, true, true)
			c.Damage(c.Target, 0, true, true)
			c.Preview = true
			c.Damage(c.Target, 100, true, true)
			hits, _, _ = casterVisualEvents(s)
			if len(hits) != 0 {
				t.Fatal("rejected/preview damage emitted impact", hits)
			}
		})
	}
}

func TestLegendaryCasterVisualBurnActualHandlersOnce(t *testing.T) {
	for _, tc := range []struct {
		id    string
		skill domain.SkillID
	}{{"SS-A02", legendaryFireBolt}, {"SS-A11", legendaryIceArrow}, {"SS-A16", legendaryFlame}} {
		t.Run(tc.id, func(t *testing.T) {
			s, c := legendaryCasterFixture(t, tc.id)
			c.Skill = tc.skill
			c.Target.Status.Apply(domain.StatusDef{ID: 1036, DurationSec: 10, IntervalSec: 1, HPChanges: []domain.ResourceChange{{Value: -10, Mode: domain.ModeAbsolute}}}, s.tick, c.Owner.ID)
			s.outbox = nil
			legendaryHandlers[tc.id].SkillResolved(c)
			legendaryHandlers[tc.id].SkillResolved(c)
			hits, _, releases := casterVisualEvents(s)
			if len(hits) != 1 || hits[0].Target != c.Target.ID || len(releases) != 0 {
				t.Fatalf("wanted one hit, no duplicate cast: %+v %+v", hits, releases)
			}
		})
	}
}

func TestLegendaryCasterVisualSyntheticAreaOnceAtCenter(t *testing.T) {
	for _, id := range []string{"SS-A04", "SS-A27"} {
		t.Run(id, func(t *testing.T) {
			s, c := legendaryCasterFixture(t, id)
			center := c.Target.Pos
			c.Center = center
			putMonster(s, 901, 10000, domain.Stats{}, center.X+40, center.Y)
			s.outbox = nil
			if id == "SS-A04" {
				c.Skill = legendaryFireball
				c.Def, _ = s.skills.Get(c.Skill, 1)
				legendaryHandlers[id].RootFinished(c)
			} else {
				c.Skill = 10432
				legendaryHandlers[id].Teleported(c, center)
			}
			_, ground, releases := casterVisualEvents(s)
			fx, _, _ := legendaryVisualEffect(id, "stage-01")
			if len(ground) != 1 || ground[0].Pos != center || ground[0].Effect != fx || ground[0].DurationMS != 0 || len(releases) != 0 {
				t.Fatalf("wrong area feedback: %+v releases %+v", ground, releases)
			}
			if c.Target.HP >= 100000 || s.entities[901].HP >= 10000 {
				t.Fatal("fixture did not settle both victims")
			}
		})
	}
}

func TestLegendaryCasterVisualSupportWavesOneImpactPerVictim(t *testing.T) {
	for _, tc := range []struct {
		id    string
		skill domain.SkillID
	}{{"YS-A22", 10306}, {"YS-A24", 10304}, {"YS-A28", 10302}} {
		t.Run(tc.id, func(t *testing.T) {
			s, c := legendaryCasterFixture(t, tc.id)
			c.Skill = tc.skill
			c.Def, _ = s.skills.Get(tc.skill, 1)
			c.Center = c.Target.Pos
			putMonster(s, 901, 10000, domain.Stats{}, 160, 100)
			s.outbox = nil
			legendaryHandlers[tc.id].RootFinished(c)
			legendaryHandlers[tc.id].RootFinished(c)
			hits, _, releases := casterVisualEvents(s)
			if tc.id == "YS-A28" {
				if len(hits) != 0 || len(releases) != 2 {
					t.Fatalf("native impact count: hits=%+v native=%+v", hits, releases)
				}
				seen := map[domain.EntityID]bool{}
				for _, e := range releases {
					if e.Mode != 1 || e.Skill != legendaryLightBolt || e.Caster != c.Owner.ID || e.Center != s.entities[e.Target].Pos || seen[e.Target] {
						t.Fatal("not exact native impact-only", releases)
					}
					seen[e.Target] = true
				}
				return
			}
			fx, _, _ := legendaryVisualEffect(tc.id, "stage-01")
			if fx == "" || len(hits) != 2 || len(releases) != 0 {
				t.Fatalf("missing/duplicate wave visuals: %+v releases %+v native=%q", hits, releases, fx)
			}
			seen := map[domain.EntityID]bool{}
			for _, hit := range hits {
				if hit.Effect != fx || seen[hit.Target] || hit.Target == c.Owner.ID {
					t.Fatal("wrong victim/duplicate impact", hits)
				}
				seen[hit.Target] = true
			}
		})
	}
}

func TestLegendaryCasterVisualLightFieldsUseLiveSlotsAndTicks(t *testing.T) {
	s, c := legendaryCasterFixture(t, "YS-A07")
	c.Skill = legendaryHolyLight
	c.Center = c.Target.Pos
	h := legendaryHandlers[c.Power.ID]
	s.outbox = nil
	h.RootFinished(c)
	_, ground, _ := casterVisualEvents(s)
	if len(ground) != 1 || ground[0].Mode != 1 || ground[0].DurationMS != 4000 || ground[0].Pos != c.Center {
		t.Fatal("wrong light field", ground)
	}
	firstKey := ground[0].Key
	for i := 1; i <= 2; i++ {
		c.RootID++
		c.NowMS++
		c.Center.X += 20
		h.RootFinished(c)
	}
	if len(s.groundEffects) != 2 {
		t.Fatal("more than two visual zones", s.groundEffects)
	}
	if got := s.groundEffects[groundEffectKey{c.Owner.ID, firstKey}]; got == nil || got.event.Pos != c.Center {
		t.Fatal("third field did not replace first slot")
	}
	s.outbox = nil
	c.NowMS += 1000
	h.Tick(c)
	_, ground, _ = casterVisualEvents(s)
	if len(ground) != 2 {
		t.Fatalf("each live zone needs one pulse, not per victim: %+v", ground)
	}
	for _, g := range ground {
		if g.Mode != 0 || g.DurationMS != 0 {
			t.Fatal("pulse invented lifetime", g)
		}
	}
	s.outbox = nil
	h.Tick(c)
	_, ground, _ = casterVisualEvents(s)
	if len(ground) != 0 {
		t.Fatal("same tick repeated pulse", ground)
	}
	s.outbox = nil
	c.DeleteState(0, "light-field-0")
	c.DeleteState(0, "light-field-1")
	_, ground, _ = casterVisualEvents(s)
	if len(s.groundEffects) != 0 || len(ground) != 2 {
		t.Fatal("deleting fields did not stop visuals", ground)
	}
	for _, g := range ground {
		if g.Mode != 2 {
			t.Fatal("missing stop", g)
		}
	}
}

func TestLegendaryCasterVisualManaOnlyActualRecipients(t *testing.T) {
	s, c := legendaryCasterFixture(t, "YS-U03")
	c.Skill = 10302
	c.Target = c.Owner
	c.Owner.Player.Party = 1
	for i := 2; i <= 4; i++ {
		join(t, s, domain.EntityID(i), int64(100+i), fmt.Sprint(i), float64(100+i*20), 100)
		p := s.players[domain.EntityID(i)]
		p.Player.Party = 1
		p.MaxMP = 1000
		p.MP = 0
	}
	s.outbox = nil
	legendaryHandlers[c.Power.ID].ManaRestored(c, 0, 500)
	hits, _, _ := casterVisualEvents(s)
	if len(hits) != 3 || hits[0].Target != 2 || hits[1].Target != 3 || hits[2].Target != 4 {
		t.Fatal("not all actual mana recipients", hits)
	}
	s.outbox = nil
	legendaryHandlers[c.Power.ID].ManaRestored(c, 0, 0)
	hits, _, _ = casterVisualEvents(s)
	if len(hits) != 0 {
		t.Fatal("zero overflow impact", hits)
	}
}

func TestLegendaryCasterVisualShieldBlastsAtActualRecipients(t *testing.T) {
	s, c := legendaryCasterFixture(t, "YS-A21")
	c.Skill = 10320
	join(t, s, 2, 102, "ally", 180, 100)
	ally := s.players[2]
	ally.Status = domain.NewStatusSet()
	c.Targets = []domain.EntityID{c.Owner.ID, ally.ID}
	c.Target = c.Owner
	legendaryHandlers[c.Power.ID].StatusCommitted(c, &domain.StatusDef{DamageShield: 100})
	c.Target = ally
	legendaryHandlers[c.Power.ID].StatusCommitted(c, &domain.StatusDef{DamageShield: 200})
	c.Target = c.Owner
	s.outbox = nil
	hp := s.entities[900].HP
	legendaryHandlers[c.Power.ID].RootFinished(c)
	legendaryHandlers[c.Power.ID].RootFinished(c)
	hits, ground, releases := casterVisualEvents(s)
	if len(ground) != 2 || len(hits) != 0 || len(releases) != 0 {
		t.Fatal("shield blast is per center, not per victim", hits, ground, releases)
	}
	if ground[0].Pos != c.Owner.Pos || ground[1].Pos != ally.Pos || ground[0].DurationMS != 0 || ground[1].DurationMS != 0 {
		t.Fatal("wrong shield blast centers", ground)
	}
	if lost := hp - s.entities[900].HP; lost != 160 {
		t.Fatalf("visual changed strongest-only overlap: %d", lost)
	}
}

func TestLegendaryCasterVisualPullOnlyAfterMovement(t *testing.T) {
	for _, landed := range []bool{true, false} {
		s, c := legendaryCasterFixture(t, "SS-A14")
		s.aoi.Move(c.Target, domain.Pos{MapID: c.Owner.Pos.MapID, X: 280, Y: 100})
		finish := s.beginEquipmentRoot(c.Owner.ID, c.Target.ID, domain.EquipmentCombatStats{})
		root := s.equipmentState().root
		root.def = sorcererRemainingDamage(legendaryIceBurst)
		root.center = c.Owner.Pos
		if landed {
			root.hits = []equipmentHit{{src: c.Owner.ID, dst: c.Target.ID, ev: event.DamageDealt{Amount: 100}}}
		}
		s.outbox = nil
		finish()
		_, ground, _ := casterVisualEvents(s)
		if !landed {
			if len(ground) != 0 {
				t.Fatal("no pull emitted swirl", ground)
			}
		} else if len(ground) != 1 || ground[0].Pos != c.Owner.Pos {
			t.Fatal("pull swirl center", ground)
		}
	}
}

func TestLegendaryCasterVisualCoverageHasExactlyCurrentHundred(t *testing.T) {
	data, err := os.ReadFile("../../../docs/legendary/caster-visual-coverage.json")
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		Items []struct {
			ID             string  `json:"id"`
			Classification string  `json:"classification"`
			Unmatched      *string `json:"unmatched_issue"`
		}
	}
	if err = json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, r := range report.Items {
		if seen[r.ID] {
			t.Fatal("duplicate coverage", r.ID)
		}
		seen[r.ID] = true
		switch r.Classification {
		case "custom_wired", "native_fx_reused", "no_world_fx_by_design":
		default:
			t.Fatal("unknown classification", r)
		}
	}
	count := 0
	for _, p := range domain.LegendaryPowers() {
		if p.ID[:2] != "SS" && p.ID[:2] != "YS" {
			continue
		}
		count++
		if !seen[p.ID] {
			t.Fatal("missing coverage", p.ID)
		}
		name, desc, icon, _ := legendaryStatePresentation(p.ID, "")
		if name != p.Name || desc != p.Effect || icon <= 0 {
			t.Fatal("missing current icon/name/description", p.ID)
		}
	}
	if count != 100 || len(seen) != 100 {
		t.Fatal("scope drift", count, len(seen))
	}
}
