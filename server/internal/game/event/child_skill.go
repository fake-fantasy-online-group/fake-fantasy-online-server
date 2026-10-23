package event

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"

// ChildSkillReleased plays native child art without actor cast animation or
// cooldown. Release and actual-landed impact are separate explicit modes.
type ChildSkillReleased struct {
	Caster domain.EntityID
	Skill  domain.SkillID
	Target domain.EntityID
	Origin domain.Pos
	Center domain.Pos
	Mode   uint8 // 0 release-only; 1 actual-landed native impact-only
}

func (e ChildSkillReleased) Subject() domain.EntityID { return e.Caster }
