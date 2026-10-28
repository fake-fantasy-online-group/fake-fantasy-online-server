package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"strings"
	"testing"
)

func TestSpiritWorldEffectsProjectionNeverPersists(t *testing.T) {
	s, p := meleeSetFixture(t, 5, 4)
	var ap domain.SpiritAppearanceDefinition
	for _, v := range domain.SpiritAppearances() {
		if v.Kind == 2 && v.EffectID > 0 {
			ap = v
			break
		}
	}
	if ap.ItemID == 0 {
		t.Fatal("missing glove appearance")
	}
	st := p.Player.Worn.At(domain.SlotGlove)
	st.SpiritAppearance = ap.ItemID
	p.Player.Worn.Set(domain.SlotGlove, st)
	base := p.Player.Char.Appear
	base.SoulFX = "legacy"
	p.Player.Char.Appear = base
	got := s.classSetAppearance(p, base)
	if !strings.HasPrefix(got.SoulFX, "legacy,TFX") {
		t.Fatalf("no public spirit effect: %q", got.SoulFX)
	}
	if p.Player.Char.Appear != base {
		t.Fatal("persisted runtime effects")
	}
	st.SpiritAppearance = 0
	p.Player.Worn.Set(domain.SlotGlove, st)
	if got := s.classSetAppearance(p, base); got.SoulFX != "legacy" {
		t.Fatalf("stale world effect: %s", got.SoulFX)
	}
}
