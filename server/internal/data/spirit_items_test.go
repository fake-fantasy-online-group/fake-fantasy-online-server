package data

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"testing"
)

func TestFinalizeDynamicSpiritItemsAfterEquipmentPass(t *testing.T) {
	items := ItemDefs{}
	for _, sp := range domain.SpiritDefinitions() {
		items[sp.ItemID] = domain.ItemDef{ID: sp.ItemID, Name: sp.Name, Icon: 123, InventoryTab: 2, InventoryTabKnown: true, InstanceKind: domain.ItemInstanceEquipment, Equip: &domain.EquipDef{Slot: int32(sp.Slot)}, EquipmentSoul: &domain.EquipmentSoulDef{TargetSlot: sp.Slot, Affixes: []domain.Affix{{Attr: domain.AttrSTR, Value: 20}}}}
	}
	ring := domain.ItemID(23473)
	ringEquip := &domain.EquipDef{Slot: int32(domain.SlotRing)}
	items[ring] = domain.ItemDef{ID: ring, InstanceKind: domain.ItemInstanceEquipment, Equip: ringEquip}
	finalizeDynamicSpiritItems(items)
	for _, sp := range domain.SpiritDefinitions() {
		d := items[sp.ItemID]
		if d.InstanceKind != domain.ItemInstanceSpirit || d.Equip != nil || d.Stackable {
			t.Fatalf("still wearable/equipment-kind: %+v", d)
		}
		if d.EquipmentSoul == nil || d.EquipmentSoul.TargetSlot != sp.Slot || len(d.EquipmentSoul.Affixes) != 1 || d.EquipmentSoul.Affixes[0].Value != 20 {
			t.Fatal("legacy fixed soul metadata lost")
		}
		if d.Icon != 123 || d.InventoryTab != 2 || !d.InventoryTabKnown {
			t.Fatal("native presentation changed")
		}
	}
	if items[ring].Equip != ringEquip || items[ring].InstanceKind != domain.ItemInstanceEquipment {
		t.Fatal("ring was changed")
	}
	finalizeDynamicSpiritItems(items)
	if len(items) != 10 {
		t.Fatal("normalization added templates")
	}
}
