package domain

// RestRule 是坐下休息的单行配置（game_rest_rule）。
//
// 数值由 PostgreSQL 在开服时加载；零值表示配置不可用，玩法保持关闭。
type RestRule struct {
	IntervalMS   int32
	HealMaxHPPct int32
	HealFlat     int32
}

// Enabled 报告这条规则能否产生有效的周期治疗。
func (r RestRule) Enabled() bool {
	return r.IntervalMS > 0 && r.HealMaxHPPct >= 0 && r.HealFlat >= 0 &&
		(r.HealMaxHPPct > 0 || r.HealFlat > 0)
}

// IntervalTicks 把数据库里的毫秒间隔换成场景逻辑帧。
func (r RestRule) IntervalTicks() Tick {
	if !r.Enabled() {
		return 0
	}
	return Ticks(int(r.IntervalMS))
}

// HealAmount 按“最大生命百分比 + 固定值”计算一跳的治疗量。
func (r RestRule) HealAmount(maxHP int32) int32 {
	if !r.Enabled() || maxHP <= 0 {
		return 0
	}
	amount := int64(maxHP)*int64(r.HealMaxHPPct)/100 + int64(r.HealFlat)
	if amount > 1<<31-1 {
		return 1<<31 - 1
	}
	return int32(amount)
}
