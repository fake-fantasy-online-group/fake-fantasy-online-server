package domain

import "testing"

func TestLegendaryDisplayNameKeepsIndependentBase(t *testing.T) {
	p, ok := LegendaryPower("TY-A01")
	if !ok {
		t.Fatal("missing power")
	}
	if a, b := LegendaryDisplayName("青铜戒指", p), LegendaryDisplayName("铁制手套", p); a == b || a != p.Name+"之青铜戒指" || b != p.Name+"之铁制手套" {
		t.Fatal(a, b)
	}
	if LegendaryDisplayName("原底材", LegendaryPowerDefinition{}) != "原底材" {
		t.Fatal("unknown definition corrupted name")
	}
}
