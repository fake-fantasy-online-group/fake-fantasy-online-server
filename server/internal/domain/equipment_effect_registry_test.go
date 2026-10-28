package domain

import "testing"

func TestEquipmentEffectRegistryStableDefinitionsAndCollisions(t *testing.T) {
	defs := EquipmentEffectDefinitions()
	counts := map[EquipmentEffectSourceKind]int{}
	for i, d := range defs {
		if i > 0 && defs[i-1].ID >= d.ID {
			t.Fatal("unstable or duplicate IDs")
		}
		got, ok := EquipmentEffectDefinitionByID(d.ID)
		if !ok || got != d || d.Description == "" || d.Trigger == "" {
			t.Fatalf("definition %+v", d)
		}
		counts[d.SourceKind]++
	}
	if counts[EffectSourceLegendary] != 330 || counts[EffectSourceUniqueTrait] < 29 || counts[EffectSourceTyped] != 9 {
		t.Fatal(counts)
	}
	if len(uniqueTraitNames) != 29 {
		t.Fatal("unique semantic catalog changed")
	}
	for kind := range uniqueTraitNames {
		if d, ok := EquipmentEffectDefinitionByID("unique:" + kind); !ok || d.SourceKind != EffectSourceUniqueTrait {
			t.Fatal(kind)
		}
	}
	for _, d := range []EquipmentEffectDefinition{{}, defs[0]} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("invalid/duplicate registration accepted")
				}
			}()
			RegisterEquipmentEffectDefinition(d)
		}()
	}
}
func TestEquipmentEffectRuntimeTypedParameterIdentityAndIsolation(t *testing.T) {
	for _, e := range []EquipmentEffect{
		{Kind: "element_absorb_bp", Element: "fire", Value: 100},
		{Kind: "proc", Proc: &EquipmentProc{ID: "test", Trigger: "on_attack", ChanceBP: 100}},
		{Kind: "aura", Aura: &EquipmentAura{ID: "test", Value: 200}},
		{Kind: "skill_variant", VariantID: "healer_guarded_mend"},
	} {
		a := RuntimeTypedEquipmentEffect(ActiveEquipmentEffect{SourceUID: 10, Item: 20, Effect: e}, true)
		b := RuntimeTypedEquipmentEffect(ActiveEquipmentEffect{SourceUID: 11, Item: 21, Effect: e}, true)
		if !a.Registered() || a.ID != b.ID || a.SourceUID == b.SourceUID || a.Item == b.Item {
			t.Fatalf("identity %+v %+v", a, b)
		}
		if a.Parameters.Typed.Proc != nil {
			a.Parameters.Typed.Proc.ChanceBP++
			if e.Proc.ChanceBP != 100 {
				t.Fatal("proc aliases input")
			}
		}
		if a.Parameters.Typed.Aura != nil {
			a.Parameters.Typed.Aura.Value++
			if e.Aura.Value != 200 {
				t.Fatal("aura aliases input")
			}
		}
	}
	p, _ := LegendaryPower("ZS-A03")
	a := RuntimeLegendaryEquipmentEffect(ActiveLegendaryPower{Power: p, PowerRollBP: 12345, SourceUID: 8, Item: 9}, true)
	if !a.Registered() || a.ID != p.ID || a.Parameters.Legendary.PowerRollBP != 12345 {
		t.Fatal(a)
	}
	u := RuntimeUniqueEquipmentEffect(UniqueTrait{Kind: "execution", PowerBP: 700, ThresholdPct: 30, SkillID: 10002})
	if !u.Registered() || u.Parameters.Unique.ThresholdPct != 30 || u.Parameters.Unique.SkillID != 10002 {
		t.Fatal(u)
	}
}
func TestEquipmentEffectDispatcherRejectsUnknownAndMismatchedSources(t *testing.T) {
	p, _ := LegendaryPower("ZS-A03")
	valid := RuntimeLegendaryEquipmentEffect(ActiveLegendaryPower{Power: p}, false)
	calls := 0
	if !DispatchEquipmentEffect(valid, func() { calls++ }) || calls != 1 {
		t.Fatal("payload not executed once")
	}
	if DispatchEquipmentEffect(valid, nil) {
		t.Fatal("nil callback accepted")
	}
	for _, mutate := range []func(*RuntimeEquipmentEffect){func(e *RuntimeEquipmentEffect) { e.DefinitionID = "missing" }, func(e *RuntimeEquipmentEffect) { e.SourceKind = EffectSourceNative }, func(e *RuntimeEquipmentEffect) { e.ID = "" }} {
		e := valid
		mutate(&e)
		if DispatchEquipmentEffect(e, func() { calls++ }) {
			t.Fatal("invalid descriptor accepted")
		}
	}
	if calls != 1 {
		t.Fatal("invalid callback executed")
	}
}
