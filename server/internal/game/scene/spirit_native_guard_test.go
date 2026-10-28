package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"testing"
)

func TestSpiritNativeFusionCannotDropDynamicAttachment(t *testing.T) {
	s, _, _, d := spiritFixture(t)
	p := s.players[1]
	gear := p.Player.Bag.At(200)
	gd := s.items[gear.Item]
	gd.Equip.Slot = int32(domain.SlotWeapon)
	s.items[gear.Item] = gd
	spirit := p.Player.Bag.At(5)
	spirit.Spirit = domain.SpiritState{Kind: 9, AttrCount: 2, Attrs: [4]int32{domain.AttrSTR, domain.AttrINT}}
	gear.FusedSoul = spirit.Item
	gear.FusedSpirit = domain.SpiritAttachmentOf(spirit)
	gear.SpiritAppearance = 13698
	p.Player.Worn.Set(domain.SlotWeapon, gear)
	s.items[99997] = domain.ItemDef{ID: 99997, InventoryTabKnown: true, EquipmentSoul: &domain.EquipmentSoulDef{TargetSlot: domain.SlotWeapon}}
	p.Player.Bag.Set(8, domain.Stack{Item: 99997, Count: 1})
	for _, slot := range []int32{5, 8} {
		before := p.Player.Bag.At(int(slot))
		s.onApplyAvatar(ApplyAvatar{ID: p.ID, BagSlot: slot, TargetSlot: int32(domain.SlotWeapon), Kind: 1, TargetKind: 1})
		if p.Player.Worn.At(domain.SlotWeapon) != gear || p.Player.Bag.At(int(slot)) != before {
			t.Fatal("native path replaced nested spirit or consumed tool")
		}
	}
	_ = d
}
