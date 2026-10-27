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
		"ZS-A08": {"未学穿刺之枪", "1级范围", "50%", "减防上限不变"},
		"ZS-A27": {"未学破防之枪", "1级范围", "80%", "控制"},
		"ZS-A30": {"未学飞翔之枪", "1级施法距离", "65%", "不会缩短"},
		"ZS-A07": {"已学破防之枪", "50%", "未学破防之枪"},
		"ZS-A24": {"85%", "未学野战八荒", "保持战神之枪原有范围"},
		"ZS-D09": {"25%", "目标免疫", "不叠加", "只取一次50%"},
		"JK-U04": {"作用半径", "80%", "50%"},
		"JK-U08": {"免疫移动减速和定身", "装备时解除已有", "不免疫眩晕、冻结、石化", "攻击速度降低", "混合效果"},
		"JK-A14": {"主目标有吸收护盾", "120%", "400%", "其他目标不会", "50%"},
		"CK-A04": {"连续投掷", "50%"},
		"CK-A13": {"3枚", "75%", "只消耗一次", "完整伤害", "可命中同一敌人"},
		"CK-A18": {"学会淬毒术", "免疫仍有效", "不会打破这次冻结"},
		"CK-A28": {"无需学会爆发冲击", "已学爆发冲击时", "45%", "120", "40", "收网聚势", "位移免疫"},
		"CK-A30": {"学会爆发冲击后", "环形爆发", "50%", "40", "只拉拽一次"},
		"CK-U03": {"不消耗法力", "150", "25个百分点", "100%", "不会拆除自己或队友"},
		"SS-A09": {"已学习冰系掌握", "必定附加", "免疫", "100%"},
		"SS-A15": {"本次施法前", "60%", "不移除或缩短"},
		"SS-A17": {"已学习燃烧", "必定附加", "82%", "免疫", "先施加或刷新燃烧"},
		"SS-A18": {"圆形作用范围扩大80%", "40%"},
		"SS-A24": {"已学习冰箭术", "150%", "不会打碎", "免疫"},
		"SS-A27": {"未学火弹术时使用其1级伤害", "180%", "不会因此学会", "每次成功移动只触发一次"},
		"SS-A28": {"未学火弹术时使用其1级伤害", "220%", "50%", "3种", "不会因此学会"},
		"YS-A29": {"50个百分点", "半径220", "最终形状", "圆形半径增加100", "矩形长度和宽度各增加100", "完整伤害"},
		"YS-A18": {"50%", "聚敌枷锁", "60%", "不会单独赋予"},
		"YS-D04": {"35个百分点", "持续时间", "覆盖规则"},
		"TY-A16": {"100%", "每1点伤害可消耗2点护盾", "不把破盾增幅带入生命伤害"},
		"TY-A24": {"1个百分点", "2%", "100%减耗", "200%伤害加成"},
		"TY-D11": {"30%", "每1点法力抵扣1点伤害", "法力不足", "之后消耗吸收护盾"},
		"TY-U03": {"30%", "至少消耗1点", "100%时可零消耗"},
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
	const want = "单体伤害技能改为攻击选中目标及其周围半径120内的敌人。技能原有伤害和附加效果不变。普通攻击、地面技能和已经变为范围攻击的技能不受影响；生效后不再获得“独锋”的伤害加成。"
	if !ok || p.Effect != want {
		t.Fatalf("approved Huiji description changed: %q", p.Effect)
	}
}
