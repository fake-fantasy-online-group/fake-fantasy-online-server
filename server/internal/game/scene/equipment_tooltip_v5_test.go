package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"strings"
	"testing"
)

func TestV5EquipmentTooltipUsesNativeNamesAndRuntimeConditions(t *testing.T) {
	s := newTestScene(t, nil)
	lines := s.equipmentV5Lines([]domain.EquipmentEffect{
		{Kind: "magic_find_pct", Value: 35},
		{Kind: "element_absorb_bp", Element: "fire", Value: 1250},
		{Kind: "proc", Proc: &domain.EquipmentProc{Trigger: "on_direct_damage_taken", SkillID: 10301, FixedRank: 5, ChanceBP: 2000, ICDMS: 15000, HPBelowPct: 30, TargetRule: "self", ScaleBP: 10000}},
		{Kind: "aura", Aura: &domain.EquipmentAura{FixedRank: 1, VisualKey: "白虎卫身", Modifier: "physical_mitigation_bp", Value: 400, RadiusPX: 240, MaxPlayers: 6, IncludeActivePet: true}},
	}, []domain.SkillRankBonus{{Scope: "branch", Profession: 1, Branch: "left", Levels: 2}, {Scope: "school", School: "flame", Levels: 1}})
	got := strings.Join(lines, "\n")
	for _, want := range []string{"战士左系主修技能 +2级", "火系技能 +1级", "染色率 +35%", "火系伤害吸收 12.5%", "生命不高于30%", "受到伤害时有20%概率对自身释放5级", "冷却15秒", "白虎卫身", "物理伤害减免 4%", "范围240", "最多6人", "出战宠物"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q: %s", want, got)
		}
	}
	if strings.Contains(got, "效力100%") {
		t.Fatal("redundant scale description")
	}
}
