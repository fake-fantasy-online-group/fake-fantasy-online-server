package scene

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture"
)

// These rank-one native payloads keep their original costs, timings, damage and
// geometry (migrations 31/33). Status definitions/applications use migration 62,
// not the historical status fixture. The selected skills have no later native
// geometry corrections in migration 68. V5 rank/synergy coverage is separate.
func sharedTargetNativeTables(t *testing.T, s *Scene) {
	t.Helper()
	raw, err := os.ReadFile("../../../../docs/daoist-endgame/v5/skill-original-ranks.json")
	if err != nil {
		t.Fatal(err)
	}
	var defs []domain.SkillDef
	if err := json.Unmarshal(raw, &defs); err != nil {
		t.Fatal(err)
	}
	s.skills = domain.SkillTable{}
	for _, d := range defs {
		s.skills[domain.SkillKey{ID: d.ID, Level: d.Level}] = d
	}
	_, s.statuses = realV5SkillFixtures(t)
	raw, err = testfixture.ReadMigrationJSON("000000_init-62.sql", "$skill_config$")
	if err != nil {
		t.Fatal(err)
	}
	var seed struct {
		States       []domain.StatusDef
		Applications []struct {
			SkillID domain.SkillID
			Rank    int32
			App     domain.StatusApplication
		}
	}
	if err := json.Unmarshal(raw, &seed); err != nil {
		t.Fatal(err)
	}
	for _, st := range seed.States {
		s.statuses[domain.StatusKey{ID: st.ID, Level: st.Level}] = st
	}
	for _, app := range seed.Applications {
		key := domain.SkillKey{ID: app.SkillID, Level: app.Rank}
		d, ok := s.skills[key]
		if !ok {
			continue
		}
		replaced := false
		for i := range d.Statuses {
			if d.Statuses[i].ID == app.App.ID {
				d.Statuses[i] = app.App
				replaced = true
			}
		}
		if !replaced {
			d.Statuses = append(d.Statuses, app.App)
		}
		s.skills[key] = d
	}
}

func sharedTargetFinishCasts(t *testing.T, s *Scene) {
	t.Helper()
	// Follow actual queued deadlines, including native windup then projectile or
	// impact delay. Never zero the native definition to force an immediate hit.
	now := s.now()
	for len(s.casts) > 0 {
		next := now
		for _, c := range s.casts {
			if c.releaseAtWall.After(next) {
				next = c.releaseAtWall
			}
			if c.resolveAtWall.After(next) {
				next = c.resolveAtWall
			}
		}
		s.stepCastsAt(next, false)
		now = next.Add(time.Millisecond)
	}
}

func sharedTargetReadyPlayer(s *Scene, p *entity.Entity, skills ...domain.SkillID) {
	p.Player.Char.Skills = domain.Learned{}
	for _, id := range skills {
		p.Player.Char.Skills[id] = 1
	}
	p.MP, p.MaxMP = 10000, 10000
	p.Stats = domain.Stats{MinAtk: 100, MaxAtk: 100, MAtk: 100, Hit: 100000}
	if st := p.Player.Worn.At(domain.SlotWeapon); st.Item != 0 {
		item := s.items[st.Item]
		if item.Equip != nil {
			item.Equip.Type = 1
			item.InventoryTab, item.InventoryTabKnown = 2, true
			s.items[st.Item] = item
		}
	}
}

func TestLegendarySharedPreservePrimaryRangeAdmission(t *testing.T) {
	for _, tc := range []struct {
		power string
		skill domain.SkillID
	}{
		{"ZS-A02", 10002}, {"ZS-A08", 10009}, {"ZS-A16", 10003},
		{"ZS-A20", 10015}, {"ZS-A24", 10023}, {"ZS-A27", 10027},
	} {
		for _, inside := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/inside=%v", tc.power, inside), func(t *testing.T) {
				s, p, m := legendaryMeleeFixture(t, tc.power)
				sharedTargetNativeTables(t, s)
				sharedTargetReadyPlayer(s, p, tc.skill, 10005, 10009, 10022)
				d, _ := s.skills.Get(tc.skill, 1)
				distance := float64(d.Dist)
				if !inside {
					distance += 0.01
				}
				s.aoi.Move(m, domain.Pos{MapID: p.Pos.MapID, X: p.Pos.X + distance, Y: p.Pos.Y})
				hp, mp := m.HP, p.MP
				s.onUseSkill(UseSkill{ID: p.ID, Skill: tc.skill, Target: m.ID})
				if !inside {
					if len(s.casts) != 0 || p.MP != mp {
						t.Fatalf("out-of-range selected target accepted or charged: casts=%d mp=%d", len(s.casts), p.MP)
					}
					if m.HP != hp {
						t.Fatal("out-of-range target damaged")
					}
					return
				}
				if len(s.casts) != 1 {
					t.Fatal("exact cast-distance boundary rejected")
				}
				if s.casts[0].primary != m.ID {
					t.Fatalf("primary=%d want %d", s.casts[0].primary, m.ID)
				}
				sharedTargetFinishCasts(t, s)
				if m.HP >= hp {
					t.Fatal("legal preserved target not hit")
				}
			})
		}
	}
}

func TestLegendarySharedSelfCirclePrimaryAndAdditiveBuilds(t *testing.T) {
	for _, tc := range []struct {
		name          string
		selected      bool
		powers        []string
		primary, side int32
	}{
		{"native_no_power", true, nil, 155, 155},
		{"single_selected", true, []string{"ZS-A23"}, 439, 279},
		{"single_unselected", false, []string{"ZS-A23"}, 279, 439},
		{"two_selected", true, []string{"ZS-A23", "TY-A34"}, 563, 279},
		{"three_selected", true, []string{"ZS-A23", "ZS-A22", "TY-A34"}, 594, 310},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, p, m := legendaryMeleeFixture(t, "ZS-A23")
			p.Player.Worn = &domain.EquipSet{}
			for i, power := range tc.powers {
				warriorEquip(t, s, p, power, []domain.EquipSlot{domain.SlotWeapon, domain.SlotBody, domain.SlotHead}[i])
			}
			sharedTargetNativeTables(t, s)
			sharedTargetReadyPlayer(s, p, 10022)
			m.HP = 50000
			side := putMonster(s, 899, 100000, domain.Stats{}, 140, 100)
			side.HP = 90000
			hp, sideHP := m.HP, side.HP
			target := domain.EntityID(0)
			if tc.selected {
				target = m.ID
			}
			s.onUseSkill(UseSkill{ID: p.ID, Skill: 10022, Target: target})
			if len(s.casts) != 1 {
				t.Fatal("native circle rejected")
			}
			c := s.casts[0]
			if c.primary != target {
				t.Fatalf("semantic primary=%d want%d", c.primary, target)
			}
			if c.def.Area.Center == domain.SkillAreaCenterCaster && c.visualTarget != p.ID {
				t.Fatal("changed client caster-circle art")
			}
			sharedTargetFinishCasts(t, s)
			if hp-m.HP != tc.primary || sideHP-side.HP != tc.side {
				t.Fatalf("damage primary=%d side=%d want%d/%d", hp-m.HP, sideHP-side.HP, tc.primary, tc.side)
			}
		})
	}
}

func TestLegendarySharedAssassinPrimaryBuilds(t *testing.T) {
	for _, tc := range []struct {
		name          string
		powers        []string
		primary, side int32
	}{
		{"single", []string{"CK-A07"}, 217, 145},
		{"two", []string{"CK-A07", "CK-A06"}, 290, 217},
		{"three", []string{"CK-A07", "CK-A06", "TY-A34"}, 406, 217},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, p, m := legendaryAssassinFixture(t, tc.powers[0])
			for i, power := range tc.powers[1:] {
				assassinAddPower(s, p, power, []domain.EquipSlot{domain.SlotBody, domain.SlotHead}[i])
			}
			sharedTargetNativeTables(t, s)
			sharedTargetReadyPlayer(s, p, 10206)
			side := putMonster(s, 899, 100000, domain.Stats{}, 140, 100)
			hp, sideHP := m.HP, side.HP
			s.onUseSkill(UseSkill{ID: p.ID, Skill: 10206, Target: m.ID})
			if len(s.casts) != 1 {
				t.Fatal("native throw rejected")
			}
			if s.casts[0].primary != m.ID {
				t.Fatal("selected enemy replaced by art")
			}
			sharedTargetFinishCasts(t, s)
			if hp-m.HP != tc.primary || sideHP-side.HP != tc.side {
				t.Fatalf("damage primary=%d side=%d want%d/%d", hp-m.HP, sideHP-side.HP, tc.primary, tc.side)
			}
		})
	}
}

func TestLegendarySharedFriendlyCenterAdmissionAndVictims(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		distance               float64
		party, alive, accepted bool
	}{
		{"same_party", 500, true, true, true}, {"exact_boundary", 600, true, true, true},
		{"outside", 600.01, true, true, false}, {"stranger", 500, false, true, false}, {"dead", 500, true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c := legendaryCasterFixture(t, "YS-A05")
			p := c.Owner
			sharedTargetNativeTables(t, s)
			sharedTargetReadyPlayer(s, p, 10317)
			join(t, s, 2, 200, "ally", p.Pos.X+tc.distance, p.Pos.Y)
			ally := s.players[2]
			p.Player.Party = 1
			if tc.party {
				ally.Player.Party = 1
			}
			if !tc.alive {
				ally.HP = 0
			}
			enemy := putMonster(s, 901, 100000, domain.Stats{}, ally.Pos.X+50, ally.Pos.Y)
			hp, allyHP, mp := enemy.HP, ally.HP, p.MP
			s.onUseSkill(UseSkill{ID: p.ID, Skill: 10317, Target: ally.ID})
			if !tc.accepted {
				if len(s.casts) != 0 || p.MP != mp {
					t.Fatal("illegal friendly center accepted or charged")
				}
				return
			}
			if len(s.casts) != 1 {
				t.Fatal("legal friendly center rejected")
			}
			if s.casts[0].primary != ally.ID {
				t.Fatal("friendly anchor not preserved")
			}
			sharedTargetFinishCasts(t, s)
			if hp-enemy.HP != 288 || ally.HP != allyHP {
				t.Fatalf("enemy damage=%d ally delta=%d", hp-enemy.HP, allyHP-ally.HP)
			}
		})
	}
}

func TestLegendarySharedImplicitSelfRetainsSupportCenter(t *testing.T) {
	for _, tc := range []struct {
		power    string
		skill    domain.SkillID
		distance float64
		withAim  bool
	}{
		{"YS-U04", 10301, 200, false}, {"YS-A26", 10321, 240, false},
		{"YS-A26", 10321, 240, true},
	} {
		name := tc.power
		if tc.withAim {
			name += "/ignored-ground-aim"
		}
		t.Run(name, func(t *testing.T) {
			s, c := legendaryCasterFixture(t, tc.power)
			p := c.Owner
			sharedTargetNativeTables(t, s)
			sharedTargetReadyPlayer(s, p, tc.skill)
			join(t, s, 2, 200, "ally", p.Pos.X+tc.distance, p.Pos.Y)
			ally := s.players[2]
			p.Player.Party, ally.Player.Party = 1, 1
			ally.HP = 1
			cmd := UseSkill{ID: p.ID, Skill: tc.skill}
			if tc.withAim {
				// This caster-centered native area must ignore an unrelated aim,
				// including for A26's additional ally selection and buff impact.
				cmd.At = &domain.Pos{MapID: p.Pos.MapID, X: p.Pos.X + 400, Y: p.Pos.Y + 400}
			}
			s.onUseSkill(cmd)
			if len(s.casts) != 1 || s.casts[0].primary != p.ID {
				t.Fatal("implicit self has no semantic center")
			}
			found := false
			for _, id := range s.casts[0].targets {
				if id == ally.ID {
					found = true
				}
			}
			if !found {
				t.Fatal("additional ally absent from actual admission")
			}
			sharedTargetFinishCasts(t, s)
			if tc.skill == 10301 && ally.HP <= 1 {
				t.Fatal("additional ally not healed")
			}
			if tc.skill == 10321 && (ally.Status == nil || !ally.Status.Has(10321)) {
				t.Fatal("additional ally not buffed")
			}
		})
	}
}

func TestLegendarySharedPrimaryControlAttachmentBoundary(t *testing.T) {
	// Native Chain has no control. This explicit compatibility attachment verifies
	// preservation without pretending the added stun is a native skill payload.
	s, p, m := legendaryMeleeFixture(t, "ZS-A02")
	sharedTargetNativeTables(t, s)
	sharedTargetReadyPlayer(s, p, 10002)
	d, _ := s.skills.Get(10002, 1)
	d.Statuses = []domain.StatusApplication{{ID: domain.StatusStun, Level: 1, ChanceBP: 10000, ChanceExplicit: true}}
	s.statuses[domain.StatusKey{ID: domain.StatusStun, Level: 1}] = domain.StatusDef{ID: domain.StatusStun, Level: 1, DurationSec: 6, TypeKnown: true, Type: domain.StatusBad, Control: domain.ControlStun}
	s.skills[domain.SkillKey{ID: d.ID, Level: 1}] = d
	side := putMonster(s, 899, 100000, domain.Stats{}, 140, 100)
	s.onUseSkill(UseSkill{ID: p.ID, Skill: d.ID, Target: m.ID})
	if len(s.casts) != 1 || len(s.casts[0].targets) != 2 || s.casts[0].targets[0] != m.ID {
		t.Fatal("primary not first in actual circle selection")
	}
	sharedTargetFinishCasts(t, s)
	if m.Status == nil || !m.Status.Has(domain.StatusStun) {
		t.Fatal("primary lost original attached control")
	}
	if side.Status != nil && side.Status.Has(domain.StatusStun) {
		t.Fatal("secondary copied control")
	}
}

func TestLegendarySharedFinalGeometryBuckets(t *testing.T) {
	for _, area := range []bool{true, false} {
		t.Run(fmt.Sprint(area), func(t *testing.T) {
			s := newCombatScene(t)
			d := domain.SkillDef{Kind: domain.SkillSingle, Radius: 100, LegendaryModifiers: domain.LegendarySkillModifiers{RadiusFlat: 100, RadiusBP: 9000}}
			if area {
				d.Kind = domain.SkillArea
				d.Area = domain.SkillAreaTargeting{Shape: domain.SkillAreaCircle, Center: domain.SkillAreaCenterCaster, Radius: 200}
			}
			// The final geometry may be replaced after a flat modifier was authored.
			s.legendaryFinalizeSkill(&d)
			want := int32(380)
			if area {
				want = 570
			}
			if d.Radius != want || area && d.Area.Radius != want {
				t.Fatalf("geometry %+v radius%d want%d", d.Area, d.Radius, want)
			}
			s.legendaryFinalizeSkill(&d)
			if d.Radius != want {
				t.Fatal("range modifier reapplied")
			}
		})
	}
}

func TestLegendarySharedNativeWeaponCircleKeepsSelection(t *testing.T) {
	s, p, m := legendaryAssassinFixture(t, "CK-A07")
	assassinAddPower(s, p, "CK-A07", domain.SlotBody)
	assassinAddPower(s, p, "TY-A34", domain.SlotHead)
	specialWeaponEquip(t, s, p, 1926)
	sharedTargetNativeTables(t, s)
	sharedTargetReadyPlayer(s, p, 10206)
	side := putMonster(s, 899, 100000, domain.Stats{}, 140, 100)
	hp, sideHP := m.HP, side.HP
	s.onUseSkill(UseSkill{ID: p.ID, Skill: 10206, Target: m.ID})
	if len(s.casts) != 1 {
		t.Fatal("native enhanced-weapon cast rejected")
	}
	d := s.casts[0].def
	if d.SpecialWeaponBaseID != 1926 || d.Area.Shape != domain.SkillAreaCircle || d.Area.Radius != 400 {
		t.Fatalf("wrong native weapon geometry %+v", d.Area)
	}
	if s.casts[0].primary != m.ID || s.casts[0].visualTarget != p.ID {
		t.Fatal("weapon circle conflated selection and art")
	}
	sharedTargetFinishCasts(t, s)
	if hp-m.HP != 400 || sideHP-side.HP != 174 {
		t.Fatalf("native weapon+two powers damage=%d/%d want400/174", hp-m.HP, sideHP-side.HP)
	}
}

func TestLegendarySharedClassSetAndCirclePrimary(t *testing.T) {
	run := func(primaryBonus bool) (int32, int32) {
		s, p := meleeSetFixture(t, 5, 4)
		p.Player.Char.Level = 100
		// Add powers without replacing the four set pieces, so the native blood-cost
		// and missing-health consumers remain active on the same accepted cast.
		for i, slot := range []domain.EquipSlot{domain.SlotWeapon, domain.SlotHead} {
			st := p.Player.Worn.At(slot)
			item := s.items[st.Item]
			item.Equip.LevelReq = 100
			item.Equip.Need.Professions = 1
			s.items[st.Item] = item
			st.UID = 10000 + int64(i)
			if i == 0 {
				st.Legendary.PowerID = "ZS-A02"
			} else if primaryBonus {
				st.Legendary.PowerID = "TY-A34"
			}
			p.Player.Worn.Set(slot, st)
		}
		sharedTargetNativeTables(t, s)
		sharedTargetReadyPlayer(s, p, 10002)
		p.HP, p.MaxHP = 10000, 10000
		p.MP = 0
		primary := putMonster(s, 901, 100000, domain.Stats{}, 200, 100)
		side := putMonster(s, 900, 100000, domain.Stats{}, 130, 100)
		s.onUseSkill(UseSkill{ID: p.ID, Skill: 10002, Target: primary.ID})
		if len(s.casts) != 1 || s.casts[0].primary != primary.ID {
			t.Fatal("set+circle cast rejected or primary lost")
		}
		if p.MP != 0 || p.HP != 9000 || !s.classSetDragonActive(p) {
			t.Fatalf("native set payment changed hp%d mp%d", p.HP, p.MP)
		}
		sharedTargetFinishCasts(t, s)
		return 100000 - primary.HP, 100000 - side.HP
	}
	primary, side := run(false)
	boosted, boostedSide := run(true)
	if primary <= 0 || side != primary || boosted <= primary || boostedSide != side {
		t.Fatalf("set primary modifier leaked or lost: base%d/%d bonus%d/%d", primary, side, boosted, boostedSide)
	}
}

func TestLegendarySharedFriendlyCenterDoesNotRestoreFriendlyVictim(t *testing.T) {
	s, c := legendaryCasterFixture(t, "YS-A05")
	sharedTargetNativeTables(t, s)
	sharedTargetReadyPlayer(s, c.Owner, 10317)
	d, _ := s.skills.Get(10317, 1)
	// Exercise the generic preserving interface used by other shape powers. A
	// friendly center must never be appended back after the damage selector strips it.
	d.LegendaryPreservePrimary = true
	s.skills[domain.SkillKey{ID: d.ID, Level: 1}] = d
	hp := c.Owner.HP
	s.onUseSkill(UseSkill{ID: c.Owner.ID, Skill: d.ID, Target: c.Owner.ID})
	if len(s.casts) != 1 {
		t.Fatal("self-centered holy light rejected")
	}
	for _, id := range s.casts[0].targets {
		if id == c.Owner.ID {
			t.Fatal("friendly anchor added as victim")
		}
	}
	sharedTargetFinishCasts(t, s)
	if c.Owner.HP != hp {
		t.Fatal("preserved friendly center was damaged")
	}
}

func TestLegendarySharedFriendlyLoadingCenterRejected(t *testing.T) {
	s, c := legendaryCasterFixture(t, "YS-A05")
	sharedTargetNativeTables(t, s)
	sharedTargetReadyPlayer(s, c.Owner, 10317)
	join(t, s, 2, 200, "loading ally", 200, 100)
	ally := s.players[2]
	ally.Player.Party, c.Owner.Player.Party = 1, 1
	s.mapLoads = map[domain.EntityID]*mapLoad{ally.ID: {}}
	mp := c.Owner.MP
	s.onUseSkill(UseSkill{ID: c.Owner.ID, Skill: 10317, Target: ally.ID})
	if len(s.casts) != 0 || c.Owner.MP != mp {
		t.Fatal("loading friendly anchor accepted")
	}
}

func TestLegendarySharedNativeAreaPrimaryUsesAdmittedGeometry(t *testing.T) {
	for _, circle := range []bool{false, true} {
		t.Run(fmt.Sprint(circle), func(t *testing.T) {
			s, p, m := legendaryAssassinFixture(t, "CK-A07")
			if circle {
				assassinAddPower(s, p, "CK-A06", domain.SlotBody)
			}
			sharedTargetNativeTables(t, s)
			sharedTargetReadyPlayer(s, p, 10206)
			// Native multi-throw uses its 500-radius cone to accept victims. Its Dist=50
			// is not a reach limit for a victim already inside the native area.
			s.aoi.Move(m, domain.Pos{MapID: p.Pos.MapID, X: p.Pos.X + 300, Y: p.Pos.Y})
			before := m.HP
			s.onUseSkill(UseSkill{ID: p.ID, Skill: 10206, Target: m.ID})
			if len(s.casts) != 1 || s.casts[0].primary != m.ID {
				t.Fatal("native area victim lost selected identity due to unused Dist")
			}
			sharedTargetFinishCasts(t, s)
			want := int32(217)
			if circle {
				want = 290
			}
			if before-m.HP != want {
				t.Fatalf("native area primary damage%d want%d", before-m.HP, want)
			}
		})
	}
}
