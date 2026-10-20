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

// ManaAmount 沿用休息规则的百分比恢复法力；HealFlat 是生命专用加成，
// 不套给法力，避免低等级角色每跳都直接回满。
func (r RestRule) ManaAmount(maxMP int32) int32 {
	if !r.Enabled() || maxMP <= 0 {
		return 0
	}
	return SaturateInt32(int64(maxMP) * int64(r.HealMaxHPPct) / 100)
}
