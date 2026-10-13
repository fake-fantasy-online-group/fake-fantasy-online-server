package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"strings"
	"testing"
)

func TestV5EquipmentTooltipUsesNativeNamesAndRuntimeConditions(t *testing.T) {
	s := newTestScene(t, nil)
	attributes, special := s.equipmentV5Lines([]domain.EquipmentEffect{
		{Kind: "magic_find_pct", Value: 35},
		{Kind: "element_absorb_bp", Element: "fire", Value: 1250},
		{Kind: "proc", Proc: &domain.EquipmentProc{Trigger: "on_direct_damage_taken", SkillID: 10301, FixedRank: 5, ChanceBP: 2000, ICDMS: 15000, HPBelowPct: 30, TargetRule: "self", ScaleBP: 10000}},
		{Kind: "aura", Aura: &domain.EquipmentAura{FixedRank: 1, VisualKey: "白虎卫身", Modifier: "physical_mitigation_bp", Value: 400, RadiusPX: 240, MaxPlayers: 6, IncludeActivePet: true}},
	}, []domain.SkillRankBonus{{Scope: "branch", Profession: 1, Branch: "left", Levels: 2}, {Scope: "school", School: "flame", Levels: 1}})
	got := strings.Join(append(attributes, special...), "\n")
	for _, want := range []string{"战士左系主修技能 +2级", "火系技能 +1级", "魔法装备获取几率提升 +35%", "火系伤害吸收 12.5%", "生命不高于30%", "受到伤害时有20%概率对自身释放5级", "冷却15秒", "白虎卫身"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q: %s", want, got)
		}
	}
	if special[1] != "赋予1级白虎卫身光环" {
		t.Fatalf("aura equipment line must stay concise: %q", special[1])
	}
	if len(attributes) != 4 || len(special) != 2 {
		t.Fatalf("skill ranks, MF and absorb must be attributes; only proc/aura are special: %v / %v", attributes, special)
	}
	if strings.Contains(got, "染色率") {
		t.Fatal("legacy MF wording leaked")
	}
	if strings.Contains(got, "效力100%") {
		t.Fatal("redundant scale description")
	}
}

func TestV5TooltipSectionsDoNotDependOnEffectInputOrder(t *testing.T) {
	s := newTestScene(t, nil)
	s.skills, s.statuses = realV5SkillFixtures(t)
	attributes, effects := s.equipmentV5Lines([]domain.EquipmentEffect{
		{Kind: "proc", Proc: &domain.EquipmentProc{Trigger: "on_attack", SkillID: 10002, FixedRank: 1, ChanceBP: 500, ScaleBP: 10000}},
		{Kind: "magic_find_pct", Value: 20},
		{Kind: "element_damage_bp", Element: "fire", Value: 1000},
		{Kind: "aura", Aura: &domain.EquipmentAura{FixedRank: 1, VisualKey: "白虎卫身", Modifier: "physical_mitigation_bp", Value: 400}},
		{Kind: "enemy_element_pierce_bp", Element: "ice", Value: 1000},
		{Kind: "element_resist_bp", Element: "holy", Value: 1250},
		{Kind: "element_mitigation_bp", Element: "dark", Value: 1500},
		{Kind: "element_absorb_bp", Element: "fire", Value: 1750},
	}, []domain.SkillRankBonus{
		{Scope: "all", Levels: 1}, {Scope: "class", Profession: 1, Levels: 2},
		{Scope: "branch", Profession: 1, Branch: "left", Levels: 3},
		{Scope: "school", School: "flame", Levels: 4}, {Scope: "single", SkillID: 10002, Levels: 5},
	})
	want := []string{"所有战斗技能 +1级", "战士技能 +2级", "战士左系主修技能 +3级", "火系技能 +4级", "连环枪法 +5级",
		"魔法装备获取几率提升 +20%", "火系伤害 +10%", "敌人冰系抗性 -10%", "圣系抗性 +12.5%", "暗系伤害减免 15%", "火系伤害吸收 17.5%"}
	if strings.Join(attributes, "\n") != strings.Join(want, "\n") {
		t.Fatalf("ordinary skill/MF/element rows changed sections or order: %v", attributes)
	}
	if len(effects) != 2 || !strings.HasPrefix(effects[0], "攻击时有5%概率") || !strings.HasPrefix(effects[1], "赋予1级白虎卫身光环") {
		t.Fatalf("proc/aura special order changed: %v", effects)
	}
}
