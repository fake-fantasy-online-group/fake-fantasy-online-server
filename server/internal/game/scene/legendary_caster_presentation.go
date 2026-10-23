package scene

type legendaryCasterUI struct {
	name, desc string
	good       bool
}

var legendaryCasterUIPhases = map[string]legendaryCasterUI{}

func legendaryCasterStatePresentation(key legendaryStateKey, st *legendaryPowerState) (name, desc string, icon int32, good, ok bool) {
	u, exists := legendaryCasterUIPhases[key.Power+"/"+key.Phase]
	if !exists || st.Stacks <= 0 {
		return "", "", 0, false, false
	}
	_, _, icon, _ = legendaryStatePresentation(key.Power, key.Phase)
	return u.name, u.desc, icon, u.good, true
}
func legendaryCasterIcon(c *legendaryContext) int32 {
	_, _, icon, _ := legendaryStatePresentation(c.Power.ID, "")
	return icon
}

var legendaryCasterNativeNames = map[string]string{
	"SS-A01": "火签",
	"SS-A03": "灼步印记",
	"SS-A07": "薪火",
	"SS-A09": "霜纹",
	"SS-A12": "冰封标记",
	"SS-A17": "续薪燃烧",
	"SS-A18": "追焰印记",
	"SS-A23": "雪后猎首",
	"SS-A25": "终章筹备",
	"SS-A26": "讨还标记",
	"SS-A29": "熔炉",
	"SS-A30": "复诵待发",
	"SS-D01": "烫手法衣",
	"SS-D03": "镜面偏转",
	"SS-D04": "幻步护盾",
	"SS-D05": "护界",
	"SS-D06": "寒封隔绝",
	"SS-D07": "换防护盾",
	"SS-D08": "枯蓝护盾",
	"SS-D10": "释能护息",
	"SS-U02": "夺法防御〔沿用原状态名〕",
	"SS-U03": "凝神回路",
	"SS-U04": "冰径游走",
	"SS-U07": "法力寄存",
	"SS-U08": "净镜通行",
	"SS-U09": "雪场驿步",
	"SS-U10": "瞬念远投",
	"YS-A03": "断愈药引",
	"YS-A04": "轮诊药签",
	"YS-A11": "协诊封印",
	"YS-A13": "神威印记",
	"YS-A17": "分科会诊",
	"YS-A18": "步步药痕",
	"YS-A19": "同病锁链",
	"YS-A21": "药爆待发",
	"YS-A23": "救急反攻",
	"YS-A25": "祝福战签",
	"YS-A26": "问罪标记",
	"YS-A27": "蓄光",
	"YS-A28": "复法余烬",
	"YS-A29": "释能记账",
	"YS-A30": "瞬发透支",
	"YS-D01": "急救护盾",
	"YS-D02": "解毒保护",
	"YS-D03": "残盾",
	"YS-D04": "后排交班",
	"YS-D05": "稳手保护",
	"YS-D06": "群疗掩护",
	"YS-D07": "清身余裕",
	"YS-D08": "救人护盾",
	"YS-D09": "针衣退手",
	"YS-D10": "锁外护持",
	"YS-U01": "回春术〔长流〕",
	"YS-U02": "专注圣契",
	"YS-U05": "加速术〔巡诊〕",
	"YS-U06": "祝福接诊",
	"YS-U08": "鹰眼会诊",
	"YS-U09": "复苏接力",
	"YS-U10": "解咒验方",
}

func legendaryCasterNativeName(c *legendaryContext) string {
	if n := legendaryCasterNativeNames[c.Power.ID]; n != "" {
		return n
	}
	return c.Power.Name
}
