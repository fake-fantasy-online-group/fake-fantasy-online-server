package scene

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"

func legendaryMeleeSkillDamage(skill domain.SkillID, pct int64) func(*legendaryContext, int32) int32 {
	return func(c *legendaryContext, n int32) int32 {
		if legendaryMeleeDirect(c) && c.Skill == skill {
			return legendaryMeleeScale(n, 100+int64(c.RollValue("damage_pct", int32(pct-100))))
		}
		return n
	}
}
func legendaryMeleeShieldRemaining(c *legendaryContext) int64 {
	var total int64
	if c.Target != nil && c.Target.Status != nil {
		c.Target.Status.EachEffective(func(a domain.Active) { total += int64(a.ShieldLeft) })
	}
	return total
}
