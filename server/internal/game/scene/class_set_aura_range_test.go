package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"strings"
	"testing"
)

func TestClassSetAuraApprovedRadiusBoundary(t *testing.T) {
	for suit := int32(1); suit <= 5; suit++ {
		for _, distance := range []float64{299, 300, 301} {
			t.Run(fmt.Sprintf("suit%d/distance%.0f", suit, distance), func(t *testing.T) {
				s := newCombatScene(t)
				source := partySetMember(t, s, 1, suit)
				join(t, s, 2, 200, "队友", source.Pos.X+distance, source.Pos.Y)
				target := s.players[2]
				target.Player.Party = source.Player.Party
				s.stepEquipmentAuras()
				a, ok := classSetAura(suit)
				if !ok || a.RadiusPX != 300 || a.RefreshMS != 500 || a.LeaseMS != 1000 {
					t.Fatalf("unexpected contract %+v", a)
				}
				got := s.classSetAuraValue(target, a.Modifier)
				want := int32(0)
				if distance <= 300 {
					want = a.Value
				}
				if got != want {
					t.Fatalf("distance %.0f benefit=%d want=%d", distance, got, want)
				}
				// Leaving the inclusive boundary invalidates the source without waiting
				// for a spatial refresh or visual lease timeout.
				if distance <= 300 {
					s.aoi.Move(target, domain.Pos{MapID: source.Pos.MapID, X: source.Pos.X + 301, Y: source.Pos.Y})
					if got := s.classSetAuraValue(target, a.Modifier); got != 0 {
						t.Fatalf("left range retained %d", got)
					}
				}
			})
		}
		text := classSetSkillText(suit, 6)
		if !strings.Contains(text, "自身及300范围内队友") || strings.Contains(text, "240范围") {
			t.Fatalf("stale suit%d tooltip: %s", suit, text)
		}
	}
}

func TestClassSetAuraDisplayNameKeepsVisualWireKey(t *testing.T) {
	s, players := partySetFive(t)
	s.stepEquipmentAuras()
	names := map[int32]string{1: "夜色拥抱", 2: "信仰之翼", 3: "哲理之石", 4: "胜利王座", 5: "勇气之刃"}
	keys := map[int32]string{1: "套装光环·刺客", 2: "套装光环·药师", 3: "套装光环·术士", 4: "套装光环·剑客", 5: "套装光环·战士"}
	for _, p := range players {
		views := s.equipmentAuraStatuses(p)
		if len(views) != 5 {
			t.Fatalf("missing auras %d", len(views))
		}
		for suit, name := range names {
			found := false
			for _, v := range views {
				if v.effect.Effect != keys[suit] {
					continue
				}
				found = true
				if v.name != name || v.buff.Name != name {
					t.Fatalf("display=%q buff=%q want=%q", v.name, v.buff.Name, name)
				}
				if !strings.Contains(v.desc, name+"套装光环") || !strings.Contains(v.desc, "来源：") {
					t.Fatalf("missing set/source: %s", v.desc)
				}
			}
			if !found {
				t.Fatalf("visual key changed: %s", keys[suit])
			}
		}
	}
	// Legacy aura names and wire keys still share their existing original label.
	old := newCombatScene(t)
	join(t, old, 1, 100, "原光环", 100, 100)
	p := old.players[1]
	cacheTestAura(old, p, 991, 1200)
	old.stepEquipmentAuras()
	views := old.equipmentAuraStatuses(p)
	if len(views) != 1 || views[0].name != "白虎卫身" || views[0].buff.Name != "白虎卫身" || views[0].effect.Effect != "白虎卫身" {
		t.Fatalf("ordinary aura changed %+v", views)
	}
}
