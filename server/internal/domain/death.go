package domain

// DeathRule 是普通场景玩家死亡时的经验、随身金钱损失规则。
//
// 百分比使用万分比保存，便于在 PostgreSQL 中只调整配置而不改战斗代码。
// 复活技能返还的是本次实际扣除值，不会根据复活时的余额重新计算。
type DeathRule struct {
	MinLevel    int32
	ExpLossBP   int32
	MoneyLossBP int32
}

// DeathLoss 是一具玩家尸体尚可被复活技能取回的实际损失。
// Money 使用客户端背包统一口径的铜币总数。
type DeathLoss struct {
	Exp   int64
	Money int64
}

func (l DeathLoss) Empty() bool { return l.Exp <= 0 && l.Money <= 0 }

// Enabled 报告死亡规则是否包含至少一种有效损失。
func (r DeathRule) Enabled() bool {
	return r.MinLevel >= 0 && r.ExpLossBP >= 0 && r.ExpLossBP <= 10000 &&
		r.MoneyLossBP >= 0 && r.MoneyLossBP <= 10000 &&
		(r.ExpLossBP > 0 || r.MoneyLossBP > 0)
}

// Loss 按死亡瞬间的当前经验和随身金钱计算实际损失。只要余额大于零且配置
// 了损失比例，至少扣除一个最小单位，避免低余额角色永远绕过死亡规则。
func (r DeathRule) Loss(level int32, exp, money int64) DeathLoss {
	if !r.Enabled() || level < r.MinLevel {
		return DeathLoss{}
	}
	return DeathLoss{
		Exp:   basisPointAmount(exp, r.ExpLossBP),
		Money: basisPointAmount(money, r.MoneyLossBP),
	}
}

// Refund 返回复活技能按百分比取回的实际值。100% 必定精确返还；较小的
// 正损失至少返还一个最小单位，同时永远不会超过原损失。
func (l DeathLoss) Refund(pct int32) DeathLoss {
	if pct <= 0 {
		return DeathLoss{}
	}
	if pct > 100 {
		pct = 100
	}
	return DeathLoss{
		Exp:   percentAmount(l.Exp, pct),
		Money: percentAmount(l.Money, pct),
	}
}

func basisPointAmount(value int64, bp int32) int64 {
	if value <= 0 || bp <= 0 {
		return 0
	}
	amount := value/10000*int64(bp) + value%10000*int64(bp)/10000
	if amount < 1 {
		return 1
	}
	if amount > value {
		return value
	}
	return amount
}

func percentAmount(value int64, pct int32) int64 {
	if value <= 0 || pct <= 0 {
		return 0
	}
	amount := value/100*int64(pct) + value%100*int64(pct)/100
	if amount < 1 {
		return 1
	}
	if amount > value {
		return value
	}
	return amount
}
