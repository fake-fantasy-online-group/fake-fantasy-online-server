package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"strings"
	"testing"
)

func TestClassSetFinalTooltipThresholdsAndColors(t *testing.T) {
	for suitID := int32(1); suitID <= 5; suitID++ {
		for count := 0; count <= 8; count++ {
			t.Run(fmt.Sprintf("%d/%d", suitID, count), func(t *testing.T) {
				s, p := meleeSetFixture(t, suitID, 8)
				races := map[int32]domain.Race{1: domain.Assassin, 2: domain.Healer, 3: domain.Warlock, 4: domain.Swordsman, 5: domain.Warrior}
				p.Player.Char.Race = races[suitID]
				suit := s.items[p.Player.Worn.At(domain.SlotHead).Item].Equip.Suit
				// No numerical 2-piece row: pure runtime skill must still be displayed.
				suit.Bonuses = []domain.EquipmentSuitBonus{
					{Pieces: 4, Affix: domain.Affix{Attr: domain.AttrCrit, Value: 500}, Probability: 100},
					{Pieces: 6, Affix: domain.Affix{Attr: domain.AttrSTR, Value: 300}, Probability: 100},
					{Pieces: 8, Affix: domain.Affix{Attr: domain.AttrMaxAtk, Value: 540}, Probability: 100},
				}
				slots := []domain.EquipSlot{domain.SlotHead, domain.SlotNeck, domain.SlotWeapon, domain.SlotGlove, domain.SlotRing, domain.SlotBody, domain.SlotShoe, domain.SlotBag}
				for i := count; i < len(slots); i++ {
					p.Player.Worn.Set(slots[i], domain.Stack{})
				}
				s.invalidateCombatAttributes(p)
				var lines []string
				s.appendSuitTooltip(&lines, suit, p)
				text := strings.Join(lines, "\n")
				if strings.Contains(text, "件]") {
					t.Fatalf("piece prefix leaked %q", text)
				}
				want := []struct {
					tier int32
					text string
				}{{2, classSetSkillText(suitID, 2)}, {4, classSetSkillText(suitID, 4)}, {4, equipmentSuitBonusText(suit.Bonuses[0])}, {6, equipmentSuitBonusText(suit.Bonuses[1])}, {8, equipmentSuitBonusText(suit.Bonuses[2])}}
				last := -1
				for _, x := range want {
					if x.text == "" {
						t.Fatal("missing skill text")
					}
					color := "\x02"
					if int32(count) >= x.tier {
						color = "\x05"
					}
					idx := strings.Index(text, color+x.text)
					if idx < 0 || idx <= last {
						t.Fatalf("wrong color/order tier%d count%d text%q", x.tier, count, text)
					}
					last = idx
				}
			})
		}
	}
}

func TestClassSetHeaderUsesEquipmentAttributeColor(t *testing.T) {
	s, p := meleeSetFixture(t, 5, 8)
	suit := s.items[p.Player.Worn.At(domain.SlotHead).Item].Equip.Suit
	suit.Name = "勇气之刃"
	for _, quality := range []uint8{0, 1, 2, 3, 4, 6, tooltipLightBlue, tooltipDarkGold} {
		var lines []string
		s.appendSuitTooltip(&lines, suit, p, quality)
		joined := strings.Join(lines, "\n")
		if !strings.Contains(joined, qualityColoredLine(quality, "套装: 勇气之刃 (8/8)")) || !strings.Contains(joined, qualityColoredLine(quality, "套装属性")) {
			t.Fatalf("headers not equipment color %d: %q", quality, joined)
		}
		if !strings.Contains(joined, "\x05"+classSetSkillText(5, 2)) {
			t.Fatal("header color changed activated effect color")
		}
	}
}
