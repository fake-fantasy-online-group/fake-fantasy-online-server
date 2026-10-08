package scene

import (
	"fmt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func (s *Scene) onNianliCreation(caster *entity.Entity, def domain.SkillDef,
	rule domain.NianliCreationRule, reject func(event.RejectReason)) {
	if caster == nil || caster.Player == nil || caster.Player.Char == nil ||
		caster.Player.Bag == nil || !domain.IsNianliCreationSkill(def.ID) || rule.Skill != def.ID {
		reject(event.RejectSkillNotUsable)
		return
	}
	if caster.Player.Work != nil {
		s.nianliCreationNotice(caster, "打工、采矿或钓鱼时不能使用念力造物。")
		return
	}
	if !canAct(caster) {
		reject(event.RejectStunned)
		return
	}
	if !canCast(caster) {
		reject(event.RejectSilenced)
		return
	}
	ch := caster.Player.Char
	if ch.EffectiveNianli() < rule.NianliCost {
		s.nianliCreationNotice(caster, fmt.Sprintf("念力不足，需要 %d 点念力。", rule.NianliCost))
		return
	}
	bag := caster.Player.Bag.Clone()
	for _, material := range rule.Materials {
		if !bag.Remove(material.Item, material.Qty) {
			item, _ := s.itemDef(material.Item)
			s.nianliCreationNotice(caster, fmt.Sprintf("缺少%s×%d。", item.Name, material.Qty))
			return
		}
	}
	product, ok := s.itemDef(rule.Product)
	if !ok || bag.Add(product, rule.ProductQty) != 0 {
		reject(event.RejectBagFull)
		return
	}

	caster.Player.StopResting()
	s.emit(event.SkillCastChanged{Caster: caster.ID, Skill: def.ID, Phase: event.SkillCastChant})
	s.emit(event.SkillCastChanged{Caster: caster.ID, Skill: def.ID, Phase: event.SkillCastRelease,
		Target: caster.ID})
	caster.Player.Bag = bag
	ch.SetNianli(ch.EffectiveNianli() - rule.NianliCost)
	caster.Player.MarkDirty()
	_ = s.pushInventory(caster)
	s.emitAttributesIfPlayer(caster)
	s.nianliCreationNotice(caster, fmt.Sprintf("念力造物成功，获得%s×%d。", product.Name, rule.ProductQty))
	if s.saver != nil {
		s.saver.Save(s.snapshotOf(caster))
	}
	s.log.Debug("念力造物完成", "char", ch.Name, "skill", def.ID,
		"product", rule.Product, "qty", rule.ProductQty, "nianli_cost", rule.NianliCost)
}

func (s *Scene) nianliCreationNotice(caster *entity.Entity, text string) {
	if caster != nil {
		s.emitTo(caster.ID, event.ServerNotice{Who: caster.ID, Text: text})
	}
}
