package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// lifePanelSkills 是正式客户端 skillconfig.lua 的生活页九项。11005 炼丹士
// 属于药师职业技能，不在生活技能界面；此前用连续区间误把它塞进面板，同时
// 漏掉了真正的第九项 11010 念力造物Ⅲ。
var lifePanelSkills = [...]domain.SkillID{
	11001, 11002, 11003, 11004, 11006, 11007, 11008, 11009, 11010,
}

func isLifePanelSkill(id domain.SkillID) bool {
	for _, skill := range lifePanelSkills {
		if id == skill {
			return true
		}
	}
	return false
}

func (s *Scene) lifeSkillSnapshot(p *entity.Entity) event.LifeSkillSnapshot {
	if p == nil || p.Player == nil || p.Player.Char == nil || s.skills == nil {
		return event.LifeSkillSnapshot{}
	}
	out := event.LifeSkillSnapshot{Who: p.ID, Skills: make([]event.LifeSkillView, 0, 9)}
	ch := p.Player.Char
	for _, id := range lifePanelSkills {
		def, ok := s.skills.Get(id, 1)
		if !ok {
			continue
		}
		level, learned := ch.Skills[id]
		if !learned {
			// Win05.SetLifeSkills 把 0 当成“已学、熟练度为 0”，这正是刚学会的
			// 钓鱼/采矿状态。未学项必须用 -1，再由 canLearn 控制学习按钮。
			level = -1
		}
		canLearn := !learned && s.canLearnLifeSkill(p, def)
		out.Skills = append(out.Skills, event.LifeSkillView{
			ID: id, Level: level, Name: def.Name, CanLearn: canLearn,
		})
	}
	return out
}

func (s *Scene) canLearnLifeSkill(p *entity.Entity, def domain.SkillDef) bool {
	if p == nil || p.Player == nil || p.Player.Char == nil || p.Player.Bag == nil || def.Book == 0 {
		return false
	}
	ch := p.Player.Char
	if ch.Level < def.LevelNeed ||
		(def.Prof != 0 && (!ch.HasProfession() || def.Prof != int32(ch.Race)+1)) ||
		p.Player.Bag.UsableCountOf(def.Book) < 1 {
		return false
	}
	money, err := inventoryMoney(ch.Money)
	return err == nil && money >= def.LearnMoney
}

func (s *Scene) onLearnLife(cmd LearnLife) {
	p := s.players[cmd.ID]
	if p == nil || !p.Alive() || p.Player == nil || p.Player.Char == nil ||
		p.Player.Bag == nil || !isLifePanelSkill(cmd.Skill) || s.skills == nil {
		return
	}
	notice := func(text string) {
		s.emitTo(p.ID, event.ServerNotice{Who: p.ID, Text: text})
	}
	if p.Player.Stall != nil || p.Player.Work != nil || p.Player.Riding {
		notice("学习生活技能失败：摆摊、打工或骑乘状态下不能学习")
		return
	}
	ch := p.Player.Char
	if _, learned := ch.Skills[cmd.Skill]; learned {
		notice("该生活技能已经学会")
		s.emitTo(p.ID, s.lifeSkillSnapshot(p))
		return
	}
	def, ok := s.skills.Get(cmd.Skill, 1)
	if !ok || ch.Level < def.LevelNeed ||
		(def.Prof != 0 && (!ch.HasProfession() || def.Prof != int32(ch.Race)+1)) {
		notice("学习生活技能失败：等级或职业不满足要求")
		s.emitTo(p.ID, s.lifeSkillSnapshot(p))
		return
	}
	book, bookOK := s.itemDef(def.Book)
	if def.Book == 0 || !bookOK || p.Player.Bag.UsableCountOf(def.Book) < 1 {
		name := "对应技能书"
		if bookOK && book.Name != "" {
			name = book.Name
		}
		notice("学习生活技能失败：需要" + name + "×1")
		s.emitTo(p.ID, s.lifeSkillSnapshot(p))
		return
	}
	money, err := inventoryMoney(ch.Money)
	if err != nil || money < def.LearnMoney {
		notice("学习生活技能失败：随身金钱不足")
		return
	}
	bag := p.Player.Bag.Clone()
	if bag == nil || !bag.Remove(def.Book, 1) {
		notice("学习生活技能失败：技能书状态已经变化")
		return
	}
	if ch.Skills == nil {
		ch.Skills = make(domain.Learned)
	}
	level := int32(1)
	if domain.IsLifeSkill(cmd.Skill) {
		level = 0
	}
	p.Player.Bag = bag
	ch.Money = moneyFromInventory(money - def.LearnMoney)
	ch.Skills[cmd.Skill] = level
	p.Player.MarkDirty()
	_ = s.pushInventory(p)
	notice("已学习" + def.Name + "，消耗" + book.Name + "×1")
	if s.saver != nil {
		s.saver.Save(s.snapshotOf(p))
	}
}
