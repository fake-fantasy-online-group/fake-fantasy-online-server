package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"strings"
	"testing"
)

func TestGenericDOTDescriptionIsBriefAndSourceIndependent(t *testing.T) {
	a := domain.Active{Def: domain.StatusDef{DOTFamily: domain.DOTBleed, Desc: "连环枪法伤害提高200%", IntervalSec: 1, DurationSec: 6, ClassSetPoisonBonusTotal: 600, SynergyDOTCopy: true}}
	got := genericDOTDescription(a)
	if got != "持续物理伤害，每1秒结算一次。" {
		t.Fatal(got)
	}
	for _, forbidden := range []string{"枪法", "200%", "最近", "实际伤害", "快照", "防御", "抗性", "吸收", "增幅", "预算", "扩散"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("technical/source text leaked: %s", got)
		}
	}
}
func TestGenericDOTVisualDoesNotUseProducingSkillOrAnimation(t *testing.T) {
	s := &Scene{}
	for _, id := range []string{"ZS-A01", "JK-A01"} {
		d := domain.StatusDef{LegendaryPowerID: id, SynergyDOTNew: true, SynergyDOTVersion: 1, Name: "流血", IntervalSec: 1, DurationSec: 6, ClassSetPoisonBonusTotal: 600}
		effect, name, mode := s.statusVisual(d)
		if effect != "流血" || name != "流血" || mode != 0 {
			t.Fatalf("%s: %s %s %d", id, effect, name, mode)
		}
	}
	if !genericPublishedDOT([]event.EntityStatusEffect{{Effect: "流血"}}) || genericPublishedDOT([]event.EntityStatusEffect{{Effect: "刀光1"}}) {
		t.Fatal("dynamic clear classification")
	}
}

func TestGenericDOTRetainsNativePoisonAndBurnArt(t *testing.T) {
	s := &Scene{}
	poison := domain.StatusDef{ID: domain.StatusPoison, Name: "中毒", IntervalSec: 2, DurationSec: 60, HPChanges: []domain.ResourceChange{{Value: -28, Mode: domain.ModeAbsolute}}}
	fx, name, mode := s.statusVisual(poison)
	if fx != "中毒" || name != "中毒" || mode != domain.StatusPositionOrbit {
		t.Fatalf("native poison changed: %s %s %d", fx, name, mode)
	}
	burn := domain.StatusDef{ID: 1036, Name: "燃烧", IntervalSec: 2, DurationSec: 40, HPChanges: []domain.ResourceChange{{Value: -54, Mode: domain.ModeAbsolute}}}
	fx, name, mode = s.statusVisual(burn)
	if fx != "燃烧1" || name != "燃烧" || mode != 0 {
		t.Fatalf("native burn changed: %s %s %d", fx, name, mode)
	}
	if !genericPublishedDOT([]event.EntityStatusEffect{{Effect: "燃烧1"}}) {
		t.Fatal("burn clear tombstone missing")
	}
}
