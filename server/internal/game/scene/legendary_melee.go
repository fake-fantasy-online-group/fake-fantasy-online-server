package scene

// Melee power handlers deliberately use explicit skill identities and event
// boundaries. The approved Chinese descriptions are presentation, never code.
import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

const (
	legendarySpearChain   domain.SkillID = 10002
	legendarySpearBreak   domain.SkillID = 10009
	legendarySpearEye     domain.SkillID = 10015
	legendarySpearFly     domain.SkillID = 10017
	legendarySpearRainbow domain.SkillID = 10021
	legendarySpearGod     domain.SkillID = 10023
	legendarySwordAura    domain.SkillID = 10102
	legendarySwordBlood   domain.SkillID = 10104
	legendarySwordBind    domain.SkillID = 10111
	legendarySwordLion    domain.SkillID = 10116
	legendarySwordVacuum  domain.SkillID = 10117
	legendarySwordSoul    domain.SkillID = 10119
	legendarySwordMeteor  domain.SkillID = 10123
)

func legendaryMeleeDirect(c *legendaryContext) bool {
	return c.Owner != nil && c.Target != nil && c.Blow.Origin == domain.DamageNormalOrigin && c.Blow.ProcDepth == 0
}
func legendaryMeleeLanded(c *legendaryContext) bool {
	return legendaryMeleeDirect(c) && !c.Hit.Flag.Has(event.DamageMiss)
}
func legendaryMeleeScale(n int32, pct int64) int32 { return domain.SaturateInt32(int64(n) * pct / 100) }
func legendaryMeleeSpear(id domain.SkillID) bool {
	switch id {
	case 10002, 10003, 10004, 10005, 10006, 10007, 10009, 10013, 10015, 10017, 10021, 10022, 10023, 10027:
		return true
	}
	return false
}

func legendaryMeleeOnce(c *legendaryContext, target domain.EntityID, phase string) bool {
	st := c.State(target, phase)
	if st.RootID == c.RootID && st.Stacks != 0 {
		return false
	}
	st.RootID = c.RootID
	st.Stacks = 1
	return true
}
