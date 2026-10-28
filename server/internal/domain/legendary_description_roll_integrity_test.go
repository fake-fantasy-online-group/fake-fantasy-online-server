package domain

import (
	"strings"
	"testing"
)

// Check every display anchor, not just the primary codex column. Catalog prose
// may be shortened, but each authored beneficial value must still be rendered.
func TestLegendaryConciseDescriptionsAllParametersAtEveryQuality(t *testing.T) {
	for _, spec := range LegendaryPowerRollSpecs() {
		base, _ := LegendaryPower(spec.ID)
		for _, roll := range []int32{0, 7500, 8123, 10000} {
			if len(spec.Parameters) == 0 && roll != 0 {
				continue
			}
			got, ok := LegendaryPowerForRoll(spec.ID, roll)
			if !ok {
				t.Fatalf("%s/%d missing", spec.ID, roll)
			}
			if strings.Contains(got.Effect, "～") || strings.Contains(got.Effect, "基础值") || strings.Contains(got.Effect, "范围值") {
				t.Fatalf("%s/%d leaked range: %s", spec.ID, roll, got.Effect)
			}
			for _, p := range spec.Parameters {
				if !strings.Contains(base.Effect, p.Text) {
					t.Fatalf("%s missing anchor %s", spec.ID, p.Text)
				}
				old := powerRollNumber(p.Base, p.Divisor) + p.Unit
				value := powerRollNumber(LegendaryPowerRollValue(spec.ID, p.Key, p.Base, roll), p.Divisor) + p.Unit
				want := strings.Replace(p.Text, old, value, 1)
				if !strings.Contains(got.Effect, want) {
					t.Errorf("%s/%d missing rolled parameter %q in %s", spec.ID, roll, want, got.Effect)
				}
			}
		}
	}
}

func TestUniversalSweepDescriptionContainsOnlyItsEffect(t *testing.T) {
	for id, want := range map[string]string{
		"TY-A01": "普通攻击改为前方120度扇形攻击，范围等于普通攻击距离。选中目标保留100%伤害，范围内其他敌人各承受70%普通攻击伤害。",
		"TY-A02": "矩形技能的长度提高60%，宽度不变。",
	} {
		p, _ := LegendaryPower(id)
		if p.Effect != want {
			t.Errorf("%s: %s", id, p.Effect)
		}
	}
}
