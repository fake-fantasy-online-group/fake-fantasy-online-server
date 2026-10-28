package domain

import (
	"strconv"
	"strings"
	"testing"
)

func TestSpiritSoulEffectsCatalogAndLifecycle(t *testing.T) {
	for _, ap := range SpiritAppearances() {
		if ap.EffectID <= 0 {
			continue
		}
		worn := NewEquipSet()
		worn.Set(ap.Slot, Stack{Item: 999, Count: 1, SpiritAppearance: ap.ItemID})
		expected := "TFX" + strconv.Itoa(int(ap.EffectID))
		if ap.EffectID >= 8055 && ap.EffectID <= 8063 {
			expected += "_20"
		}
		got := SpiritSoulEffects("legacy", worn)
		if got != "legacy,"+expected {
			t.Fatalf("item%d: %s", ap.ItemID, got)
		}
		if got := SpiritSoulEffects(expected, worn); got != expected {
			t.Fatalf("duplicate: %s", got)
		}
		// Reproject from unchanged base after split (appearance stays) and unequip.
		if strings.Count(got, expected) != 1 {
			t.Fatal("repeated effect")
		}
		worn.Set(ap.Slot, Stack{})
		if got := SpiritSoulEffects("legacy", worn); got != "legacy" {
			t.Fatalf("stale effect: %s", got)
		}
	}
	if SpiritSoulEffects("legacy", nil) != "legacy" {
		t.Fatal("lost legacy effects")
	}
}
