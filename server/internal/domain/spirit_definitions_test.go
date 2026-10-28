package domain

import "testing"

func TestSpiritRegistrySourceIdentities(t *testing.T) {
	defs := SpiritDefinitions()
	if len(defs) != 9 {
		t.Fatal(len(defs))
	}
	seen := map[ItemID]bool{}
	for _, d := range defs {
		if seen[d.ItemID] || d.Slot == SlotRing || d.MapID != 7 || d.NPCID == 130 {
			t.Fatalf("bad definition %+v", d)
		}
		seen[d.ItemID] = true
		got, ok := SpiritDefinitionForItem(d.ItemID)
		if !ok || got != d {
			t.Fatal("lookup")
		}
		got, ok = SpiritDefinitionForKind(d.Kind)
		if !ok || got != d {
			t.Fatal("kind lookup")
		}
		weights := d.Weights()
		sum := int32(0)
		attrs := map[int32]bool{}
		for _, w := range weights {
			if w.Weight <= 0 || attrs[w.Attr] {
				t.Fatal("weights")
			}
			attrs[w.Attr] = true
			sum += w.Weight
		}
		if sum != 10000 || len(attrs) != 12 {
			t.Fatal(sum, len(attrs))
		}
		weights[0].Weight = 0
		if d.Weights()[0].Weight == 0 {
			t.Fatal("shared weights")
		}
		fee := int64(100000)
		if d.Kind == 8 {
			fee = 50000
		}
		if d.ProcessMoney != fee {
			t.Fatal("money units")
		}
	}
	defs[0].Name = "mutated"
	if SpiritDefinitions()[0].Name == "mutated" {
		t.Fatal("shared definitions")
	}
}
func TestSpiritAppearanceSourceCoverage(t *testing.T) {
	all := SpiritAppearances()
	if len(all) != 92 {
		t.Fatal(len(all))
	}
	effects := 0
	weapons := 0
	seen := map[ItemID]bool{}
	for _, a := range all {
		if seen[a.ItemID] || a.Slot == SlotRing || a.AttributeCount != 4 {
			t.Fatalf("bad appearance %+v", a)
		}
		seen[a.ItemID] = true
		if a.EffectID != 0 {
			effects++
		} else if a.Kind == 9 {
			weapons++
		} else {
			t.Fatal("missing effect", a.ItemID)
		}
		if a.Extra[0].Attr == 0 || a.Extra[1].Attr == 0 || a.Extra[0].Value <= 0 || a.Extra[1].Value <= 0 {
			t.Fatal("bonus", a)
		}
		if _, ok := SpiritDefinitionForKind(a.Kind); !ok {
			t.Fatal("unknown kind")
		}
	}
	if effects != 82 || weapons != 10 {
		t.Fatal(effects, weapons)
	}
	for _, id := range []ItemID{43021, 43022, 43023, 43024, 43025, 43026, 43028, 43029, 43030, 43031, 43032, 43033, 43034, 43035, 43036, 43037, 43038, 43041, 43042, 43043, 43044} {
		if _, ok := SpiritAppearanceForItem(id); ok {
			t.Fatal("unsupported cloth", id)
		}
	}
}
func TestSpiritAppearanceInclusiveThreshold(t *testing.T) {
	d, _ := SpiritAppearanceForItem(13707)
	s := SpiritState{Kind: 9, AttrCount: 4, Attrs: [4]int32{AttrSTR, AttrAGI, AttrVIT, AttrINT}}
	for i := range s.Rolls {
		for j := range s.Rolls[i] {
			s.Rolls[i][j] = 18
		}
	}
	if !d.Meets(s) {
		t.Fatal("270 must qualify")
	}
	s.Rolls[0][0] = 17
	if d.Meets(s) {
		t.Fatal("269 cannot qualify")
	}
	d, _ = SpiritAppearanceForItem(25096)
	s.Kind = d.Kind
	for i := range s.Rolls {
		for j := range s.Rolls[i] {
			s.Rolls[i][j] = 0
		}
		for j := 0; j < 10; j++ {
			s.Rolls[i][j] = 20
		}
	}
	if !d.Meets(s) {
		t.Fatal("sum 800 must qualify")
	}
	s.Rolls[0][0] = 19
	if d.Meets(s) {
		t.Fatal("799 cannot qualify")
	}
}
