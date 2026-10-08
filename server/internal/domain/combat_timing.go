package domain

// CombatTimingRule 是普通攻击命中时序与玩家前摇受击打断的服务端配置。
// 技能弹道速度由每个技能自己的最大距离与客户端飞行表现时长推导，不放在这里。
type CombatTimingRule struct {
	MeleeHitDelayMS          int32
	RangedBasicSpeedPXPerSec int32
	CastInterruptBaseBP      int32
	CastInterruptMinBP       int32
}

func (r CombatTimingRule) ValidCastInterrupt() bool {
	return r.CastInterruptMinBP > 0 && r.CastInterruptMinBP <= r.CastInterruptBaseBP &&
		r.CastInterruptBaseBP <= 10000
}

func (r CombatTimingRule) Valid() bool {
	return r.MeleeHitDelayMS >= 0 && r.RangedBasicSpeedPXPerSec > 0
}
