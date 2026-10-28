package domain

import (
	"regexp"
	"strings"
	"testing"
)

// Player tooltips must describe playable effects, not implementation plans or
// retired revisions. Runtime semantics remain covered by the scene tests.
func TestLegendaryPlayerDescriptionsExcludeDevelopmentNotes(t *testing.T) {
	powers := LegendaryPowers()
	if len(powers) != 330 {
		t.Fatalf("catalog contains %d powers, want 330", len(powers))
	}
	internalID := regexp.MustCompile(`(?:[A-Z]{2}-)?[ADU][0-9]{2}`)
	for _, p := range powers {
		t.Run(p.ID, func(t *testing.T) {
			for _, phrase := range []string{
				"原生", "根施法", "生产器", "计时器", "账本", "实测", "暂定",
				"备选", "build", "三槽", "同桶", "快照", "需实现", "待实现",
				"需测试", "待修复", "实现时", "原提案", "开发", "旧版", "删除",
				"固定回蓝只结算一次", "按本次横扫的总伤害", "按职业威能改造",
				"不叠加", "共享冷却", "覆盖规则", "每次施法对同一敌人只触发一次",
				"不会再次触发本效果", "不再触发威能", "不先扣除再返还",
			} {
				if strings.Contains(p.Effect, phrase) {
					t.Errorf("player description contains developer wording %q: %s", phrase, p.Effect)
				}
			}
			if internalID.MatchString(p.Effect) {
				t.Errorf("player description refers to an internal power ID: %s", p.Effect)
			}
			if !strings.HasSuffix(p.Effect, "。") {
				t.Errorf("player description must end as a complete sentence: %s", p.Effect)
			}
		})
	}
}

func TestLegendaryPlayerDescriptionsPreserveCriticalConditions(t *testing.T) {
	cases := map[string][]string{
		"ZS-A08": {"未学穿刺之枪", "1级范围", "50%"},
		"ZS-A27": {"未学破防之枪", "1级范围", "80%"},
		"ZS-A30": {"未学飞翔之枪", "1级施法距离", "65%", "至少提高至"},
		"ZS-A07": {"已学破防之枪", "50%"},
		"ZS-A24": {"85%", "未学野战八荒", "保持战神之枪原有范围"},
		"ZS-D09": {"25%", "成功施放时"},
		"JK-U04": {"作用半径", "80%", "50%"},
		"JK-U08": {"免疫移动减速和定身", "装备时解除已有"},
		"JK-A14": {"命中有护盾的主目标", "120%", "50%"},
		"CK-A04": {"连续投掷", "50%"},
		"CK-A13": {"3枚", "75%", "完整伤害", "可命中同一敌人"},
		"CK-A18": {"学会淬毒术", "成功冻结", "不会打破这次冻结"},
		"CK-A28": {"环形爆发", "45%", "120", "40"},
		"CK-A30": {"学会爆发冲击后", "环形爆发", "50%", "40", "不再先击退"},
		"CK-U03": {"不消耗法力", "150", "25个百分点", "可见敌方陷阱"},
		"SS-A09": {"已学习冰系掌握", "必定附加", "100%"},
		"SS-A15": {"本次施法前", "60%"},
		"SS-A17": {"已学习燃烧", "必定附加", "82%"},
		"SS-A18": {"圆形作用范围扩大80%", "40%"},
		"SS-A24": {"已学习冰箭术", "150%", "冰封保留原冻结"},
		"SS-A27": {"未学火弹术时使用其1级伤害", "180%", "成功移动后"},
		"SS-A28": {"未学火弹术时使用其1级伤害", "220%", "50%", "每拥有一种防御魔法"},
		"YS-A29": {"50个百分点", "半径220", "圆形半径增加100", "矩形长度和宽度各增加100", "完整伤害"},
		"YS-A18": {"50%", "若枷锁术具有直接伤害", "60%"},
		"YS-D04": {"35个百分点", "物理和魔法减伤"},
		"TY-A16": {"100%", "破坏敌方吸收护盾的效率"},
		"TY-A24": {"1个百分点", "2%"},
		"TY-D11": {"30%", "每1点法力抵扣1点伤害", "法力不足"},
		"TY-U03": {"主动技能法力消耗降低30%"},
	}
	for id, required := range cases {
		t.Run(id, func(t *testing.T) {
			p, ok := LegendaryPower(id)
			if !ok {
				t.Fatal("missing power")
			}
			for _, phrase := range required {
				if !strings.Contains(p.Effect, phrase) {
					t.Errorf("missing required contract %q: %s", phrase, p.Effect)
				}
			}
		})
	}
}

func TestLegendaryHuijiUsesApprovedPlayerDescription(t *testing.T) {
	p, ok := LegendaryPower("TY-A05")
	const want = "单体伤害技能改为攻击选中目标及其周围半径120内的敌人。"
	if !ok || p.Effect != want {
		t.Fatalf("approved Huiji description changed: %q", p.Effect)
	}
}

// Interaction policy is covered by combat tests; descriptions must not advertise
// retired blanket exclusions after those effects become composable.
func TestLegendaryDescriptionsDoNotAdvertiseBlanketInteractionExclusions(t *testing.T) {
	for _, power := range LegendaryPowers() {
		for _, retired := range []string{
			"不触发命中威能", "不触发威能", "不能触发命中威能",
			"生效后不再获得“独锋”", "战吼半径与", "与「碎岩」",
			"新增副治疗不会再触发", "威能额外产生的护盾不触发",
		} {
			if strings.Contains(power.Effect, retired) {
				t.Errorf("%s contains retired interaction wording %q", power.ID, retired)
			}
		}
	}
}
