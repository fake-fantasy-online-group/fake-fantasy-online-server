package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"strings"
	"testing"
)

func TestLegendaryMeleeRollCoverage(t *testing.T) {
	ranged, fixed := 0, 0
	for _, s := range domain.LegendaryPowerRollSpecs() {
		if !strings.HasPrefix(s.ID, "ZS-") && !strings.HasPrefix(s.ID, "JK-") && !strings.HasPrefix(s.ID, "CK-") {
			continue
		}
		p, _ := domain.LegendaryPower(s.ID)
		legacy, _ := domain.LegendaryPowerForRoll(s.ID, 0)
		if legacy.Effect != p.Effect {
			t.Fatal("legacy changed", s.ID)
		}
		if len(s.Parameters) == 0 {
			fixed++
			if s.FixedReason == "" {
				t.Fatal("unexplained fixed", s.ID)
			}
			continue
		}
		ranged++
		for _, roll := range []int32{7500, 8765, 10000, 0} {
			c := legendaryContext{Power: p, PowerRollBP: roll}
			factor := roll
			if factor == 0 {
				factor = 10000
			}
			for _, v := range s.Parameters {
				if got := c.RollValue(v.Key, v.Base); got != max(int32(1), v.Base*factor/10000) {
					t.Fatalf("%s %s=%d", s.ID, v.Key, got)
				}
			}
		}
	}
	if ranged != 142 || fixed != 8 {
		t.Fatalf("coverage %d ranged %d fixed", ranged, fixed)
	}
}
func TestLegendaryMeleeRollActualModifiers(t *testing.T) {
	for _, tc := range []struct {
		id    string
		skill domain.SkillID
		field string
		base  int32
	}{
		{"ZS-A01", 10002, "damage", 8000}, {"ZS-A04", 10005, "damage", 1500}, {"ZS-A05", 10005, "damage", 6000}, {"ZS-A07", 10009, "damage", 5000}, {"ZS-A11", 10017, "damage", 6000}, {"ZS-A13", 10006, "damage", 4500}, {"ZS-A15", 10006, "damage", 6000}, {"ZS-A17", 10003, "damage", 3000}, {"ZS-A19", 10021, "damage", 7000}, {"ZS-A22", 10022, "damage", 2000}, {"ZS-A24", 10023, "damage", 8500}, {"ZS-A25", 10022, "damage", 3500}, {"ZS-A27", 10027, "damage", 8000}, {"ZS-A30", 10017, "damage", 6500},
		{"ZS-U02", 10013, "mana", 6000}, {"ZS-U05", 10006, "mana", 4500}, {"ZS-U08", 10005, "mana", 3000}, {"ZS-U10", 10023, "mana", 6000}, {"JK-U04", 10127, "mana", 5000},
		{"CK-A01", 10221, "cooldown", 3000}, {"CK-A03", 10221, "damage", 8000}, {"CK-A04", 10205, "damage", 5000}, {"CK-A06", 10206, "damage", 5000}, {"CK-A11", 10213, "damage", 4000}, {"CK-A12", 10213, "damage", 9000}, {"CK-A14", 10214, "radius", 3000}, {"CK-A16", 10215, "damage", 5000}, {"CK-A17", 10209, "damage", 5000}, {"CK-A20", 10209, "damage", 2500}, {"CK-A23", 10219, "damage", 3500}, {"CK-A24", 10203, "damage", 5000}, {"CK-A29", 10210, "damage", 8000}, {"CK-U09", 10221, "mana", 3000},
	} {
		t.Run(tc.id, func(t *testing.T) {
			_, c := legendaryCasterFixture(t, tc.id)
			for _, roll := range []int32{7500, 8765, 10000, 0} {
				c.PowerRollBP = roll
				d := domain.SkillDef{ID: tc.skill}
				legendaryHandlers[tc.id].PrepareSkill(c, &d)
				var got int32
				switch tc.field {
				case "damage":
					got = d.LegendaryModifiers.DamageBP
				case "mana":
					got = d.LegendaryModifiers.ManaReductionBP
				case "radius":
					got = d.LegendaryModifiers.RadiusBP
				case "cooldown":
					got = d.LegendaryModifiers.CooldownReductionBP
				}
				f := roll
				if f == 0 {
					f = 10000
				}
				if got != tc.base*f/10000 {
					t.Fatalf("roll%d got%d want%d", roll, got, tc.base*f/10000)
				}
			}
		})
	}
}
func TestLegendaryMeleeRollDurationsAndReduction(t *testing.T) {
	_, c := legendaryCasterFixture(t, "JK-U05")
	c.Owner.Player.Char.Skills[10103] = 1
	c.Skill = 10103
	c.PowerRollBP = 7500
	legendaryHandlers[c.Power.ID].CastReleased(c)
	if got := c.State(0, "functional-window").ExpiresMS; got != c.NowMS+6000 {
		t.Fatalf("rolled duration expiry %d", got)
	}
	// Native functional helper owns the exact expiry and its learning gate.
	if !warriorFunctionalActive(c, 10103) {
		t.Fatal("rolled immunity did not start")
	}
	for _, tc := range []struct {
		id        string
		remaining int32
	}{{"ZS-D05", 70}, {"ZS-D07", 67}, {"JK-D02", 59}} {
		_, c := legendaryCasterFixture(t, tc.id)
		c.PowerRollBP = 7500
		h := legendaryHandlers[tc.id]
		var got int32
		if tc.id == "JK-D02" {
			c.Blow.Origin = domain.DamageDOTOrigin
			got = h.BeforeIncoming(c, 10000)
		} else {
			got = h.ModifyIncoming(c, 10000)
		}
		if got != tc.remaining*100 {
			t.Fatalf("%s got%d", tc.id, got)
		}
	}
}
func TestLegendaryMeleeRollPoisonPayload(t *testing.T) {
	for _, id := range []string{"CK-A09", "CK-A15"} {
		_, c := legendaryCasterFixture(t, id)
		c.PowerRollBP = 7500
		d := domain.StatusDef{ID: domain.StatusPoison, ClassSetPoison: true, SourceSkillID: 10214}
		legendaryHandlers[id].StatusApplied(c, &d)
		if d.LegendaryModifiers.DamageTickBP != 7500 {
			t.Fatal(id, d.LegendaryModifiers.DamageTickBP)
		}
		if id == "CK-A09" {
			legendaryHandlers[id].StatusApplied(c, &d)
			if d.LegendaryModifiers.DamageTickBP != 7500 {
				t.Fatal("duplicate poison scaling")
			}
		}
	}
}

func TestLegendaryMeleeRollLiveSourceHelpers(t *testing.T) {
	for _, id := range []string{"CK-A09", "CK-D08", "CK-A25"} {
		t.Run(id, func(t *testing.T) {
			s, p, m := legendaryAssassinFixture(t, id)
			slot := domain.SlotWeapon
			if id == "CK-D08" {
				slot = domain.SlotBody
			}
			st := p.Player.Worn.At(slot)
			st.Legendary.PowerRollBP = 7500
			p.Player.Worn.Set(slot, st)
			s.invalidateCombatAttributes(p)
			c, ok := legendaryAssassinPowerContext(s, p, id)
			if !ok || c.PowerRollBP != 7500 {
				t.Fatalf("lookup lost roll: %v %d", ok, c.PowerRollBP)
			}
			switch id {
			case "CK-A09":
				d := domain.StatusDef{ID: domain.StatusPoison, ClassSetPoison: true}
				s.legendaryAssassinPreparePoison(p, &d)
				if d.LegendaryModifiers.DamageTickBP != 7500 {
					t.Fatal("native poison reverted to fixed full value")
				}
			case "CK-D08":
				if n := s.legendaryAssassinPoisonIncoming(p, 10000); n != 7800 {
					t.Fatal("native poison resistance lost roll", n)
				}
			case "CK-A25":
				c.Skill, c.Target = 10203, m
				near := assassinCoverageMob(s, 950, m.Pos.X+74, m.Pos.Y)
				far := assassinCoverageMob(s, 951, m.Pos.X+76, m.Pos.Y)
				ids := legendaryAssassinCounterTargets(&c, nil)
				foundNear, foundFar := false, false
				for _, v := range ids {
					foundNear = foundNear || v == near.ID
					foundFar = foundFar || v == far.ID
				}
				if !foundNear || foundFar {
					t.Fatal("counter radius not actual rolled75", ids)
				}
			}
		})
	}
}
func TestLegendaryMeleeRollFractionalDuration(t *testing.T) {
	s, c := legendaryCasterFixture(t, "CK-U08")
	c.PowerRollBP = 8765
	d := domain.StatusDef{ID: 10208, SourceSkillID: 10208, Invisible: true, DurationSec: 30}
	legendaryHandlers[c.Power.ID].StatusApplied(c, &d)
	s.legendaryFinalizeStatus(&d)
	if d.LegendaryDurationMS != 34382 {
		t.Fatal("fractional bonus duration lost", d.LegendaryDurationMS)
	}
}
