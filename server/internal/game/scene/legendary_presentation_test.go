package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"testing"
)

func TestLegendaryPresentationAllApprovedPowers(t *testing.T) {
	for _, p := range domain.LegendaryPowers() {
		name, desc, icon, _ := legendaryStatePresentation(p.ID, "tick-with-no-skill")
		if name != p.Name || desc != p.Effect || icon <= 0 {
			t.Fatalf("missing or stale presentation %s", p.ID)
		}
	}
	if _, _, _, ok := legendaryStatePresentation("future-unknown", ""); ok {
		t.Fatal("invented unknown power")
	}
	if len(legendaryPresentationIcons) != 330 {
		t.Fatal("incomplete icon map")
	}
}
func TestLegendaryPresentationStagesNeverFallback(t *testing.T) {
	a, _, ok := legendaryVisualEffect("ZS-A12", "stage-01")
	if !ok || a != "LP_ZS_A12_01" {
		t.Fatal(a)
	}
	b, _, ok := legendaryVisualEffect("ZS-A12", "stage-02")
	if !ok || b != "LP_ZS_A12_02" || a == b {
		t.Fatal(b)
	}
	if _, _, ok := legendaryVisualEffect("ZS-A12", "invalid-trigger"); ok {
		t.Fatal("must not infer trigger")
	}
	if _, _, ok := legendaryVisualEffect("ZS-A01", "stage-01"); ok {
		t.Fatal("UI-only must not invent VFX")
	}
}
