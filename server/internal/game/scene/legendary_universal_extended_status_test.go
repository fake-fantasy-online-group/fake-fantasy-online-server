package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"testing"
)

func TestLegendaryUniversalExtendedStatusFamilies(t *testing.T) {
	for _, tc := range []struct {
		id   string
		want int32
	}{{"TY-A13", 7000}, {"TY-A14", -60}, {"TY-A17", 16}, {"TY-A22", 7500}, {"TY-A23", 60}, {"TY-D18", 7000}, {"TY-U11", 7500}} {
		t.Run(tc.id, func(t *testing.T) {
			s, _, _, c := universalTestContext(t, tc.id)
			d := domain.StatusDef{ID: 998, SourceSkillID: 123, DurationSec: 5}
			switch tc.id {
			case "TY-A13":
				d.BlockMove = true
			case "TY-A14":
				d.Affixes = []domain.Affix{{Attr: domain.AttrMoveSpeed, Value: -40, Mode: domain.ModePercent}}
			case "TY-A17":
				d.LifeStealPct = 10
			case "TY-A22":
				d.ID = domain.StatusPoison
			case "TY-A23":
				d.PoisonOnHitChance = 20
			case "TY-D18":
				d.DamageShield = 100
			}
			universalExtendedStatus(c, &d)
			s.legendaryFinalizeStatus(&d)
			got := d.LegendaryDurationMS
			switch tc.id {
			case "TY-A14":
				got = d.Affixes[0].Value
			case "TY-A17":
				got = d.LifeStealPct
			case "TY-A23":
				got = d.PoisonOnHitChance
			}
			if got != tc.want {
				t.Fatalf("got%d want%d", got, tc.want)
			}
		})
	}
}
func TestLegendaryUniversalReceiverStatusFamilies(t *testing.T) {
	for _, tc := range []struct {
		id   string
		want int32
	}{{"TY-D13", 140}, {"TY-D14", -60}, {"TY-D15", 60}, {"TY-D20", 3000}} {
		t.Run(tc.id, func(t *testing.T) {
			s, _, _, c := universalTestContext(t, tc.id)
			d := domain.StatusDef{ID: 998, SourceSkillID: 123, DurationSec: 5}
			switch tc.id {
			case "TY-D13":
				d.DamageShield = 100
			case "TY-D14":
				d.Affixes = []domain.Affix{{Attr: domain.AttrDef, Value: -100}}
			case "TY-D15":
				d.TypeKnown = true
				d.Type = domain.StatusBad
				d.Affixes = []domain.Affix{{Attr: domain.AttrAtkSpeed, Value: 100, Mode: domain.ModePercent}}
			case "TY-D20":
				d.ID = domain.StatusPoison
				d.TypeKnown = true
				d.Type = domain.StatusBad
			}
			universalExtendedReceiving(c, &d)
			s.legendaryFinalizeStatus(&d)
			got := d.LegendaryDurationMS
			if tc.id == "TY-D13" {
				got = d.DamageShield
			}
			if tc.id == "TY-D14" || tc.id == "TY-D15" {
				got = d.Affixes[0].Value
			}
			if got != tc.want {
				t.Fatalf("got%d want%d", got, tc.want)
			}
		})
	}
}
func TestLegendaryUniversalPeriodicCritSnapshot(t *testing.T) {
	for _, tc := range []struct {
		id     string
		status domain.StatusID
	}{{"TY-A28", domain.StatusPoison}, {"TY-A29", 1036}} {
		t.Run(tc.id, func(t *testing.T) {
			s, p, _, c := universalTestContext(t, tc.id)
			p.Stats.CritRate = 10000
			p.Stats.MCritRate = 10000
			d := domain.StatusDef{ID: tc.status, SourceSkillID: 123, DurationSec: 5, IntervalSec: 1}
			universalExtendedStatus(c, &d)
			s.legendaryFinalizeStatus(&d)
			p.Stats.CritRate = 0
			p.Stats.MCritRate = 0
			if got := s.legendaryPeriodicAmount(domain.Active{Def: d}, -100); got != -200 {
				t.Fatalf("snapshot tick%d", got)
			}
			if s.legendaryPeriodicAmount(domain.Active{Def: d}, 100) != 100 {
				t.Fatal("healing crit introduced")
			}
		})
	}
}
func TestLegendaryUniversalSourceAccuracyAndReceivedHealing(t *testing.T) {
	s, p, m, _ := universalTestContext(t, "TY-A15")
	d := domain.StatusDef{ID: domain.StatusPoison, SourceSkillID: 10211, TypeKnown: true, Type: domain.StatusBad}
	s.legendaryStatusAccuracy(p, m, &d)
	if d.EffectHitBP != 3000 {
		t.Fatal("accuracy not at pre-roll hook")
	}
	s, p, _, _ = universalTestContext(t, "TY-D12")
	if s.legendaryIncomingHeal(p, 100) != 140 {
		t.Fatal("received healing missing")
	}
	s.legendaryState().depth = 1
	if s.legendaryIncomingHeal(p, 100) != 100 {
		t.Fatal("recursive heal amplification")
	}
}
func TestLegendaryUniversalNativeCritAndPenetration(t *testing.T) {
	for _, tc := range []struct {
		id     string
		crit   int32
		resist int32
		want   int32
	}{{"TY-A11", 8000, 0, 200}, {"TY-A12", 0, 100, 30}, {"TY-A33", 10000, 0, 250}} {
		t.Run(tc.id, func(t *testing.T) {
			s, p, m, _ := universalTestContext(t, tc.id)
			p.Stats.CritRate = tc.crit
			m.Stats.PhysResist = tc.resist
			before := m.HP
			end := s.beginEquipmentRoot(p.ID, m.ID, domain.EquipmentCombatStats{})
			ev, _, _ := s.resolveStrikeStatuses(p, m, &combat.HitTracker{}, combat.Blow{SkillID: 1, SkillPct: 100, GuaranteedHit: true})
			end()
			if before-m.HP != tc.want {
				t.Fatalf("damage%d flag%v", before-m.HP, ev.Flag)
			}
			if m.Stats.PhysResist != tc.resist {
				t.Fatal("target defense mutated")
			}
		})
	}
}
func TestLegendaryUniversalExtendedIncomingKinds(t *testing.T) {
	for _, tc := range []struct {
		id   string
		want int32
	}{{"TY-D06", 75}, {"TY-D07", 80}, {"TY-D08", 80}, {"TY-D10", 70}, {"TY-D17", 75}} {
		t.Run(tc.id, func(t *testing.T) {
			s, p, _, c := universalTestContext(t, tc.id)
			c.IsBasic = true
			if tc.id == "TY-D08" {
				c.Blow.Magic = true
			}
			if tc.id == "TY-D10" {
				c.Hit.Flag |= event.DamageCrit
			}
			if tc.id == "TY-D17" {
				p.Status.Apply(domain.StatusDef{ID: 998, BlockMove: true, DurationSec: 5}, s.tick, 2)
			}
			if got := universalExtendedIncoming(c, 100); got != tc.want {
				t.Fatalf("damage%d", got)
			}
			c.Target = p
			if universalExtendedIncoming(c, 100) != 100 {
				t.Fatal("self damage benefit")
			}
		})
	}
}
func TestLegendaryUniversalExtendedTargetSynergies(t *testing.T) {
	for _, id := range []string{"TY-A30", "TY-A31", "TY-A32", "TY-A36"} {
		t.Run(id, func(t *testing.T) {
			s, p, m, c := universalTestContext(t, id)
			d := domain.SkillDef{Kind: domain.SkillArea, Effects: []domain.SkillEffect{{Kind: domain.EffectDamage, DamagePct: 100}}}
			st := domain.StatusDef{ID: 998, DurationSec: 5}
			if id == "TY-A30" {
				st.BlockMove = true
			}
			if id == "TY-A31" {
				st.ID = domain.StatusPoison
			}
			if id == "TY-A32" {
				st.ID = 1036
			}
			m.Status.Apply(st, s.tick, p.ID)
			c.Primary = 999
			s.legendaryFinalizeSkill(&d)
			universalExtendedTarget(c, &d)
			want := int32(140)
			if id == "TY-A30" {
				want = 150
			}
			if d.Effects[0].DamagePct != want {
				t.Fatalf("damage%d", d.Effects[0].DamagePct)
			}
		})
	}
}
func TestLegendaryUniversalOpposedNativeModifiersShareBucket(t *testing.T) {
	s, _, _, _ := universalTestContext(t, "TY-A13")
	d := domain.StatusDef{DurationSec: 10, Affixes: []domain.Affix{{Attr: domain.AttrMoveSpeed, Mode: domain.ModePercent, Value: -40}}, LegendaryModifiers: domain.LegendaryStatusModifiers{DurationBP: 4000, ControlDurationReductionBP: 3000, SlowMagnitudeBP: 5000, SlowMagnitudeReductionBP: 4000}}
	s.legendaryFinalizeStatus(&d)
	if d.LegendaryDurationMS != 11000 || d.Affixes[0].Value != -44 {
		t.Fatalf("duration%d slow%d", d.LegendaryDurationMS, d.Affixes[0].Value)
	}
}
