package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

// Preview is pure. Failed requests and UI inspection never advance this ledger.
func previewGrowthRounding(p *entity.Entity, def *domain.SkillDef) domain.SkillRoundingState {
	if p == nil || p.Player == nil || p.Player.Char == nil || def == nil {
		return domain.SkillRoundingState{}
	}
	return domain.PreviewSkillRounding(def, p.Player.Char.SkillRoundingCredits[def.ID])
}

func commitGrowthRounding(p *entity.Entity, def domain.SkillDef) {
	if p == nil || p.Player == nil || p.Player.Char == nil || !def.RoundingPreviewReady {
		return
	}
	old := p.Player.Char.SkillRoundingCredits[def.ID]
	next := def.RoundingPreviewNext
	if next == old {
		return
	}
	if p.Player.Char.SkillRoundingCredits == nil {
		p.Player.Char.SkillRoundingCredits = map[domain.SkillID]domain.SkillRoundingState{}
	}
	p.Player.Char.SkillRoundingCredits[def.ID] = next
	p.Player.MarkDirty()
}

// The existing typed sidecar carries the next exact integer preview. Only push
// when its observable cost changes, rather than publishing fractional metadata.
func (s *Scene) refreshGrowthRoundingPreview(p *entity.Entity, accepted domain.SkillDef) {
	if !accepted.RoundingPreviewReady {
		return
	}
	next := accepted
	previewGrowthRounding(p, &next)
	if next.MPCost != accepted.MPCost || next.CooldownMS != accepted.CooldownMS {
		s.maybeSendSkillRanks(p)
	}
}
