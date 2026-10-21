package scene

import (
	"fmt"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// Fixed V1 two-piece additions are multiplied by ten; percentage additions retain
// their original V1 values per the latest user correction. These are
// ordinary native card affixes, not hard-coded production bonuses.
func TestClassSetTwoApprovedAttributesReachPanel(t *testing.T) {
	cases := []struct {
		id        int32
		race      domain.Race
		add       domain.Base
		secondary domain.Affix
		text      []string
	}{
		{5, domain.Warrior, domain.Base{STR: 300, VIT: 180}, domain.Affix{Attr: domain.AttrAtkSpeed, Mode: domain.ModePercent, Value: -8}, []string{"力量 +300", "体质 +180", "攻击速度 +8%"}},
		{4, domain.Swordsman, domain.Base{STR: 240, VIT: 240}, domain.Affix{Attr: domain.AttrHit, Mode: domain.ModePercent, Value: 10}, []string{"力量 +240", "体质 +240", "命中 +10%"}},
		{1, domain.Assassin, domain.Base{STR: 300, DEX: 180}, domain.Affix{}, []string{"力量 +300", "灵巧 +180"}},
		{3, domain.Warlock, domain.Base{INT: 180, SPI: 120}, domain.Affix{}, []string{"智慧 +180", "精神 +120"}},
		{2, domain.Healer, domain.Base{INT: 160, VIT: 120}, domain.Affix{Attr: domain.AttrMaxMP, Mode: domain.ModePercent, Value: 10}, []string{"智慧 +160", "体质 +120", "最大法力 +10%"}},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprint(tc.id), func(t *testing.T) {
			s, p := meleeSetFixture(t, tc.id, 2)
			p.Player.Char.Race = tc.race
			def := s.items[p.Player.Worn.At(domain.SlotHead).Item]
			suit := def.Equip.Suit
			resource := domain.AttrMaxHP
			if tc.id == 2 || tc.id == 3 {
				resource = domain.AttrMaxMP
			}
			affixes := []domain.Affix{{Attr: int32(resource), Value: 2500}}
			for _, x := range []struct{ attr, value int32 }{{domain.AttrSTR, tc.add.STR}, {domain.AttrVIT, tc.add.VIT}, {domain.AttrINT, tc.add.INT}, {domain.AttrSPI, tc.add.SPI}, {domain.AttrDEX, tc.add.DEX}} {
				if x.value != 0 {
					affixes = append(affixes, domain.Affix{Attr: x.attr, Value: x.value})
				}
			}
			if tc.secondary.Attr != 0 {
				affixes = append(affixes, tc.secondary)
			}
			for _, a := range affixes {
				if !domain.SupportsEquipmentSuitAffix(a) {
					t.Fatalf("unsupported %+v", a)
				}
				suit.Bonuses = append(suit.Bonuses, domain.EquipmentSuitBonus{Pieces: 2, Affix: a, Probability: 100})
			}
			s.invalidateCombatAttributes(p)
			s.refreshStats(p)
			want := p.Player.Char.EffectiveBase().Add(tc.add)
			attrs := s.attributeSnapshot(p).Total
			for _, x := range []struct {
				index int
				value int32
			}{{10, want.STR}, {11, want.VIT}, {13, want.INT}, {14, want.SPI}, {21, want.DEX}} {
				if attrs[x.index] != x.value {
					t.Fatalf("panel attr%d=%d want%d", x.index, attrs[x.index], x.value)
				}
			}
			expected := domain.DeriveFromBase(want)
			if resource == domain.AttrMaxHP {
				expected.MaxHP += 2500
			} else {
				expected.MaxMP += 2500
			}
			if tc.id == 2 {
				expected.MaxMP += expected.MaxMP / 10
			}
			if tc.id == 4 {
				expected.Hit += expected.Hit / 10
			}
			if p.MaxHP != expected.MaxHP || p.MaxMP != expected.MaxMP || p.Stats.Hit != expected.Hit {
				t.Fatalf("resources/hit got%d/%d/%d want%d/%d/%d", p.MaxHP, p.MaxMP, p.Stats.Hit, expected.MaxHP, expected.MaxMP, expected.Hit)
			}
			if tc.id == 5 {
				if attrs[33] != 108 {
					t.Fatalf("panel speed=%d want108", attrs[33])
				}
				p.Look.AtkDist = 75
				p.Look.WeaponCType = 0
				p.DidBasicAttack(0)
				if p.ReadyToAttack(8) || !p.ReadyToAttack(9) {
					t.Fatal("+8% speed must produce 1000/1.08ms rounded to nine ticks")
				}
			}
			var lines []string
			s.appendSuitTooltip(&lines, suit, p)
			tooltip := strings.Join(lines, "\n")
			for _, text := range tc.text {
				if !strings.Contains(tooltip, text) {
					t.Fatalf("missing %q in %q", text, tooltip)
				}
			}
			// One broken item crosses 2 -> 1 and removes every new affix, including %.
			st := p.Player.Worn.At(domain.SlotNeck)
			st.Durability = 0
			p.Player.Worn.Set(domain.SlotNeck, st)
			s.invalidateCombatAttributes(p)
			s.refreshStats(p)
			baseline := domain.DeriveFromBase(p.Player.Char.EffectiveBase())
			if p.MaxHP != baseline.MaxHP || p.MaxMP != baseline.MaxMP || p.Stats.Hit != baseline.Hit || p.Stats.PlayerAttackSpeedPercent() != 100 {
				t.Fatal("broken 2-piece retained an attribute")
			}
		})
	}
}
