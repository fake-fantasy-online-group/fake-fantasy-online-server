package scene

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"

// Compatibility entrypoint; membership is owned by the common effect table.
func specialWeaponSkillMatches(item domain.ItemID, skill domain.SkillID, profession int32) bool {
	return domain.SpecialWeaponSkillMatches(item, skill, profession)
}
