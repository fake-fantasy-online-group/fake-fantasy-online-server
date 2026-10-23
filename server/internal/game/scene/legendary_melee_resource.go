package scene

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"

func legendaryMeleeRestoreMP(c *legendaryContext, amount int32) {
	if amount <= 0 {
		return
	}
	c.Owner.MP = min(c.Owner.MaxMP, c.Owner.MP+amount)
	c.Scene.emitAttributesIfPlayer(c.Owner)
}
func legendaryMeleeSpearActive(d domain.SkillDef) bool {
	return d.ID >= 10000 && d.ID < 10100 && d.Kind != domain.SkillPassive && (d.RequiredWeaponType == 1 || legendaryMeleeSpear(d.ID) || d.ID == 10028 || d.ID == 10016)
}
