package scene

import (
	"sort"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// onUpgradeSkill 升级技能(学习或提升等级)。
//
// 技能点是独立于六维自由点的持久化资源：每次升级获得 1 点，每学一级消耗 1 点。
func (s *Scene) onUpgradeSkill(cmd UpgradeSkill) {
	p, ok := s.players[cmd.ID]
	if !ok || !p.Alive() {
		return
	}
	reject := func(r event.RejectReason) {
		s.emitTo(p.ID, event.Rejected{Who: p.ID, Cmd: "UpgradeSkill", Reason: r})
	}

	if s.skills == nil {
		reject(event.RejectUnknown)
		return
	}

	ch := p.Player.Char
	def, nextLv, reason, ok := s.skillUpgradeTarget(p, cmd.Skill)
	if !ok {
		reject(reason)
		return
	}
	if ch.SkillPointsKnown && ch.SkillPoints <= 0 {
		reject(event.RejectNotEnough)
		return
	}

	// 学会 / 升级
	if ch.Skills == nil {
		ch.Skills = make(domain.Learned)
	}
	ch.Skills[cmd.Skill] = nextLv
	// SkillPointsKnown=false 只兼容旧的内存构造调用；生产角色从建角开始及每次
	// PostgreSQL 加载后都为 true，必定执行消耗。
	if ch.SkillPointsKnown {
		ch.SkillPoints--
	}
	p.Player.MarkDirty()
	if def.Kind == domain.SkillPassive {
		s.refreshStats(p)
	}

	// 下发技能快照(0x8015): 客户端技能书界面需要这个才能刷新
	s.emitTo(p.ID, s.skillSnapshot(p))
	if nextLv > 1 {
		s.requestNewbieTip(p.ID, 9)
	}

	s.log.Debug("升级技能", "char", ch.Name, "技能", def.Name, "等级", nextLv)
}

// skillUpgradeTarget 汇总界面加点与技能书共用的技能准入。资源消耗留给各自
// 调用方处理：界面升级扣技能点，技能书路径扣书。
func (s *Scene) skillUpgradeTarget(p *entity.Entity, skill domain.SkillID) (domain.SkillDef, int32, event.RejectReason, bool) {
	if p == nil || p.Player == nil || p.Player.Char == nil || s.skills == nil {
		return domain.SkillDef{}, 0, event.RejectUnknown, false
	}
	ch := p.Player.Char
	nextLv := ch.Skills.LevelOf(skill) + 1
	def, ok := s.skills.Get(skill, nextLv)
	if !ok {
		if s.skills.MaxLevelOf(skill) == 0 {
			return domain.SkillDef{}, 0, event.RejectUnknown, false
		}
		return domain.SkillDef{}, 0, event.RejectMaxLevel, false
	}
	if !ch.AllowsSkill(skill, def.Prof) {
		return domain.SkillDef{}, 0, event.RejectWrongProf, false
	}
	if ch.Level < def.LevelNeed {
		return domain.SkillDef{}, 0, event.RejectLevelTooLow, false
	}
	for _, dependency := range def.Prerequisites {
		if ch.Skills.LevelOf(dependency) == 0 {
			return domain.SkillDef{}, 0, event.RejectSkillPrerequisite, false
		}
	}
	return def, nextLv, event.RejectUnknown, true
}

// skillSnapshot 构造技能快照(0x8015)。
func (s *Scene) skillSnapshot(e *entity.Entity) event.SkillSnapshot {
	ch := e.Player.Char
	levels := s.clientSkillLevels(e)
	skills := make([]event.SkillView, 0, len(levels))
	for id, lv := range levels {
		skills = append(skills, event.SkillView{ID: id, Level: lv})
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].ID < skills[j].ID })
	bonuses := make(map[domain.SkillID]int32)
	for id, lv := range ch.Skills {
		passive, ok := s.skills.Get(id, lv)
		if !ok || passive.Kind != domain.SkillPassive || !ch.AllowsSkill(passive.ID, passive.Prof) {
			continue
		}
		for _, m := range passive.PassiveModifiers {
			if m.Kind == domain.PassiveSkillDamagePct && m.TargetSkill > 0 {
				bonuses[m.TargetSkill] += m.Value
			}
		}
	}
	return event.SkillSnapshot{Who: e.ID, SkillPoints: ch.SkillPoints, Skills: skills, DamageBonuses: bonuses}
}
