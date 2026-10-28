package domain

import "testing"

func TestLifeSalvageNativeOreWeightsAndLevelBands(t *testing.T) {
	for tier := 1; tier <= 10; tier++ {
		got := map[ItemID]int32{}
		for roll := 0; roll < tier+1; roll++ {
			out := RollEquipmentSalvageOres(int32((tier-1)*15+1), int32(SlotRing), 0, func(n int) int {
				if n != tier+1 {
					t.Fatalf("bound %d", n)
				}
				return roll
			})
			for _, v := range out {
				got[v.ItemID] += v.Count
			}
		}
		if len(got) != tier || got[4001] != 8 {
			t.Fatalf("tier %d: %v", tier, got)
		}
		for i := 1; i < tier; i++ {
			if got[ItemID(4001+3*i)] != 4 {
				t.Fatalf("weights: %v", got)
			}
		}
	}
}
func TestLifeSalvageQualityQuantityAndBounds(t *testing.T) {
	for q, want := range []int32{8, 16, 24, 32, 48} {
		if got := EquipmentSalvageOreCount(int32(SlotWeapon), int32(q)); got != want {
			t.Fatal(got, want)
		}
	}
	if EquipmentSalvageOreCount(int32(SlotRing), 4) != 24 || EquipmentSalvageOreCount(int32(SlotHead), 4) != 36 {
		t.Fatal("slot quantity")
	}
	if out := RollEquipmentSalvageOres(150, 4, 4, func(int) int { return -1 }); out != nil {
		t.Fatal("invalid rng accepted")
	}
	def := ItemDef{Equip: &EquipDef{}}
	st := Stack{SocketCount: 5, RefineLevel: 20, WashCount: 4}
	if EquipmentSalvageQuality(st, def) != 0 {
		t.Fatal("paid upgrades amplify salvage")
	}
	st.Endgame.UniqueID = 1
	if EquipmentSalvageQuality(st, def) != 4 {
		t.Fatal("legacy darkgold")
	}
}
