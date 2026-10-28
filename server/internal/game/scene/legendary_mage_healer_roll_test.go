package scene

import (
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func TestLegendaryMageHealerRollMetadataAndLegacy(t *testing.T) {
	ranged, fixed := 0, 0
	for _, spec := range domain.LegendaryPowerRollSpecs() {
		if !strings.HasPrefix(spec.ID, "SS-") && !strings.HasPrefix(spec.ID, "YS-") {
			continue
		}
		p, _ := domain.LegendaryPower(spec.ID)
		legacy, ok := domain.LegendaryPowerForRoll(spec.ID, 0)
		if !ok || legacy.Effect != p.Effect {
			t.Fatalf("legacy description changed %s", spec.ID)
		}
		if len(spec.Parameters) == 0 {
			fixed++
			if spec.FixedReason == "" || domain.LegendaryPowerHasRoll(spec.ID) {
				t.Fatal("unexplained fixed power", spec.ID)
			}
			continue
		}
		ranged++
		low, ok := domain.LegendaryPowerForRoll(spec.ID, 7500)
		if !ok || strings.Contains(low.Effect, "～") {
			t.Fatal("missing real range", spec.ID)
		}
		for _, parameter := range spec.Parameters {
			c := legendaryContext{Power: p, PowerRollBP: 7500}
			want := max(int32(1), parameter.Base*3/4)
			if got := c.RollValue(parameter.Key, parameter.Base); got != want {
				t.Fatalf("%s %s: %d != %d", spec.ID, parameter.Key, got, want)
			}
			c.PowerRollBP = 0
			if got := c.RollValue(parameter.Key, parameter.Base); got != parameter.Base {
				t.Fatal("legacy roll changed", spec.ID)
			}
			c.PowerRollBP = 10000
			if got := c.RollValue(parameter.Key, parameter.Base); got != parameter.Base {
				t.Fatal("max roll changed", spec.ID)
			}
		}
	}
	if ranged != 96 || fixed != 4 {
		t.Fatalf("coverage: ranged=%d fixed=%d", ranged, fixed)
	}
}

func TestLegendaryMageHealerRollActualModifierHooks(t *testing.T) {
	for _, tc := range []struct {
		id    string
		skill domain.SkillID
		field string
		base  int32
	}{
		{"SS-A01", 10402, "damage", 7000}, {"SS-A12", 10410, "damage", 2000}, {"SS-A18", 10405, "damage", 4000}, {"SS-A20", 10412, "damage", 5500},
		{"SS-A06", 10402, "radius", 6500}, {"SS-U01", 10432, "mana", 1500}, {"SS-U09", 10402, "cooldown", 3000},
		{"YS-A03", 10319, "damage", 6000}, {"YS-A08", 10317, "damage", 8000}, {"YS-A09", 10312, "damage", 4000}, {"YS-A11", 10312, "damage", 4500},
		{"YS-A12", 10312, "damage", 5000}, {"YS-A14", 10326, "cooldown", 4500}, {"YS-A16", 10326, "damage", 3500}, {"YS-A18", 10325, "damage", 6000},
		{"YS-A25", 10307, "buff", 6000}, {"YS-A26", 10321, "buff", 7000}, {"YS-D01", 10301, "healing", 6000}, {"YS-D03", 10320, "buff", 6000},
		{"YS-U01", 10303, "mana", 2000}, {"YS-U05", 10311, "buff", 3000},
	} {
		t.Run(tc.id, func(t *testing.T) {
			_, c := legendaryCasterFixture(t, tc.id)
			for _, roll := range []int32{7500, 8765, 10000, 0} {
				c.PowerRollBP = roll
				d := domain.SkillDef{ID: tc.skill, ProjectileSpeedPXPerSec: 100}
				legendaryHandlers[tc.id].PrepareSkill(c, &d)
				var got int32
				switch tc.field {
				case "damage":
					got = d.LegendaryModifiers.DamageBP
				case "radius":
					got = d.LegendaryModifiers.RadiusBP
				case "mana":
					got = d.LegendaryModifiers.ManaReductionBP
				case "cooldown":
					got = d.LegendaryModifiers.CooldownReductionBP
				case "buff":
					got = d.LegendaryModifiers.StatusPowerBP
				case "healing":
					got = d.LegendaryModifiers.HealingBP
				}
				factor := roll
				if factor == 0 {
					factor = 10000
				}
				want := tc.base * factor / 10000
				if got != want {
					t.Fatalf("roll%d actual hook=%d want%d", roll, got, want)
				}
				if tc.id == "SS-U01" && d.CooldownMS != 0 {
					t.Fatal("free cooldown changed")
				}
			}
		})
	}
}

func TestLegendaryMageHealerRollDefensesNeverInvert(t *testing.T) {
	for _, tc := range []struct {
		id        string
		reduction int32
	}{{"SS-D06", 35}, {"SS-D10", 35}} {
		_, c := legendaryCasterFixture(t, tc.id)
		c.Owner.Player.Char.Skills[10419] = 1
		for _, roll := range []int32{7500, 10000, 0} {
			c.PowerRollBP = roll
			wantReduction := tc.reduction
			if roll != 0 {
				wantReduction = tc.reduction * roll / 10000
			}
			got := legendaryHandlers[tc.id].BeforeIncoming(c, 10000)
			if got != 10000-wantReduction*100 {
				t.Fatalf("%s roll%d incoming%d reduction%d", tc.id, roll, got, wantReduction)
			}
		}
	}
}

func TestLegendaryMageHealerRollBurnPayloadAndFixedMechanics(t *testing.T) {
	for _, tc := range []struct {
		id    string
		skill domain.SkillID
		base  int32
	}{{"SS-A02", 10401, 80}, {"SS-A11", 10409, 100}, {"SS-A16", 10405, 160}} {
		s, c := legendaryCasterFixture(t, tc.id)
		c.PowerRollBP = 7500
		c.Skill = tc.skill
		burn := domain.StatusDef{ID: 1036, DurationSec: 10, IntervalSec: 1, TypeKnown: true, Type: domain.StatusBad, HPChanges: []domain.ResourceChange{{Value: -100, Mode: domain.ModeAbsolute}}}
		c.Target.Status.Apply(burn, s.tick, c.Owner.ID)
		before := legendaryCasterBurn(c, c.Target)
		hp := c.Target.HP
		legendaryHandlers[tc.id].SkillResolved(c)
		if got := hp - c.Target.HP; got != int32(before)*(tc.base*3/4)/100 {
			t.Fatalf("%s actual payload%d", tc.id, got)
		}
		if legendaryCasterBurn(c, c.Target) != before {
			t.Fatal("roll consumed or shortened native burn")
		}
	}
	_, c := legendaryCasterFixture(t, "YS-U07")
	c.PowerRollBP = 7500
	c.Skill = 10306
	legendaryHandlers[c.Power.ID].RootFinished(c)
	found := false
	c.Owner.Status.Each(func(a domain.Active) {
		if a.Def.ID == legendaryHealerEscortStatus {
			found = true
			if a.Def.Affixes[0].Value != 45 || a.Def.DurationSec != 8 || a.Def.PhysicalDamageReductionPct != 25 {
				t.Fatalf("wrong rolled/fixed escort %+v", a.Def)
			}
		}
	})
	if !found {
		t.Fatal("escort absent")
	}
}
