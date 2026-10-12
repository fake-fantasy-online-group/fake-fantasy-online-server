package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"strconv"
)

// Upgrades share the equipment/reward aggregate receipt and persistence lease.
// No permanent mutation or point deduction becomes visible before commit succeeds.
func (s *Scene) onEndgameSkillUpgrade(p *entity.Entity, r domain.EndgameRequest) {
	if p == nil || p.Player == nil || !p.Alive() || !canAct(p) || s.entityMapLoading(p) {
		s.endgameError(p, r, "busy", "当前状态不能分配技能点")
		return
	}
	ch := p.Player.Char
	id := domain.SkillID(r.SkillID)
	if !p.Player.EndgameGrowthV5 || !s.skills.V5Enabled(id) {
		s.endgameError(p, r, "capability", "请更新客户端后使用新版技能加点")
		return
	}
	if ch.SkillAllocationVersion != domain.SkillFormulaVersion {
		s.endgameError(p, r, "allocation_migration", "技能存档尚未完成新版迁移")
		return
	}
	if err := domain.ValidateSkillAllocation(ch.Level, int32(ch.Race)+1, ch.Skills, ch.SkillPoints); err != nil {
		s.endgameError(p, r, "allocation_invalid", err.Error())
		return
	}
	def, next, reason, ok := s.skillUpgradeTarget(p, id)
	if !ok {
		s.endgameError(p, r, "skill_unavailable", fmt.Sprintf("无法提升技能：%v", reason))
		return
	}
	if ch.SkillPoints < 1 {
		s.endgameError(p, r, "skill_points", "技能点不足")
		return
	}
	candidate := s.snapshotOf(p).Clone()
	if candidate.Char.Skills == nil {
		candidate.Char.Skills = domain.Learned{}
	}
	candidate.Char.Skills[id] = next
	candidate.Char.SkillPoints--
	if err := domain.ValidateSkillAllocation(candidate.Char.Level, int32(candidate.Char.Race)+1, candidate.Char.Skills, candidate.Char.SkillPoints); err != nil {
		s.endgameError(p, r, "allocation_invalid", err.Error())
		return
	}
	response := domain.EndgameResponse{Kind: "skill.upgrade", Message: fmt.Sprintf("%s投入等级提升至%d", def.Name, next), Result: map[string]string{"skillId": strconv.Itoa(int(id)), "hardRank": strconv.Itoa(int(next)), "skillPoints": strconv.Itoa(int(candidate.Char.SkillPoints))}}
	s.endgameCommit(p, r, candidate, response, nil, 0, func() { s.emitTo(p.ID, s.skillSnapshot(p)); s.sendGrowthV5Skills(p,domain.EndgameRequest{Op:"skill.ranks"}) })
}
