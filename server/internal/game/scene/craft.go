package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func (s *Scene) onOpenFurnace(cmd OpenFurnace) {
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil || cmd.MakeType > 4 || s.crafts.ByType == nil {
		return
	}
	p.Player.CraftType = cmd.MakeType
	s.emitTo(p.ID, s.craftRecipeSnapshot(p, cmd.MakeType))
}

func (s *Scene) craftRecipeSnapshot(p *entity.Entity, makeType uint8) event.CraftRecipeSnapshot {
	snap := event.CraftRecipeSnapshot{Who: p.ID, MakeType: makeType}
	for _, recipe := range s.crafts.Recipes(makeType) {
		// 客户端会为每条配方同步构建完整行与材料提示。一次下发数千条会
		// 卡死界面；只发送当前技能、熟练度、金钱、材料和背包都已满足，
		// 此刻确实可以执行的配方。
		if _, canMake := s.planCraft(p, recipe, true); !canMake {
			continue
		}
		product, ok := s.itemDef(recipe.Product)
		if !ok {
			continue
		}
		view := event.CraftRecipeView{
			Product: recipe.Product, Name: product.Name, Cost: int32(recipe.Cost),
		}
		for _, material := range recipe.Materials {
			def, ok := s.itemDef(material.Item)
			if !ok {
				continue
			}
			view.Materials = append(view.Materials, event.CraftMaterialView{
				Item: material.Item, Need: material.Qty,
				Have: p.Player.Bag.UsableCountOf(material.Item), Name: def.Name,
			})
		}
		view.CanMake = true
		snap.Recipes = append(snap.Recipes, view)
	}
	return snap
}

// planCraft 在背包副本上扣材料并放入产物。checkOutput=false 用于“失败但材料
// 消失”的路径，只验证并扣材料，不要求产物有空位。
func (s *Scene) planCraft(p *entity.Entity, recipe domain.CraftRecipe, checkOutput bool) (*domain.Bag, bool) {
	if p == nil || p.Player == nil || p.Player.Char == nil || p.Player.Bag == nil ||
		!craftSkillEligible(p.Player.Char, recipe) {
		return nil, false
	}
	money, err := inventoryMoney(p.Player.Char.Money)
	if err != nil || money < recipe.Cost {
		return nil, false
	}
	next := p.Player.Bag.Clone()
	if next == nil {
		return nil, false
	}
	for _, material := range recipe.Materials {
		if !next.Remove(material.Item, material.Qty) {
			return nil, false
		}
	}
	if checkOutput {
		product, ok := s.itemDef(recipe.Product)
		if !ok || next.Add(product, recipe.OutputQty) != 0 {
			return nil, false
		}
	}
	return next, true
}

func craftSkillEligible(ch *domain.Character, recipe domain.CraftRecipe) bool {
	if recipe.RequiredSkill == 0 {
		return true
	}
	value, learned := ch.Skills[recipe.RequiredSkill]
	if !learned || value < recipe.SkillLevelNeed || value < recipe.ProficiencyMin {
		return false
	}
	return recipe.ProficiencyMax <= 0 || value <= recipe.ProficiencyMax
}

func (s *Scene) onCraftItem(cmd CraftItem) {
	p := s.players[cmd.ID]
	if p == nil || !p.Alive() || p.Player == nil || p.Player.Char == nil ||
		cmd.MakeType != p.Player.CraftType || s.crafts.ByProduct == nil {
		return
	}
	var chosen domain.CraftRecipe
	var successBag *domain.Bag
	for _, recipe := range s.crafts.ProductRecipes(cmd.MakeType, cmd.Product) {
		if next, ok := s.planCraft(p, recipe, true); ok {
			chosen, successBag = recipe, next
			break
		}
	}
	if successBag == nil {
		s.emitTo(p.ID, event.ServerNotice{Who: p.ID, Text: "材料、金钱或背包空间不足。"})
		s.emitTo(p.ID, s.craftRecipeSnapshot(p, cmd.MakeType))
		return
	}

	roll := int32(s.rng.Intn(100))
	succeeded := roll < chosen.SuccessRate
	consumeMaterials := succeeded || roll < chosen.SuccessRate+chosen.DisappearRate
	next := p.Player.Bag
	if succeeded {
		next = successBag
	} else if consumeMaterials {
		if lost, ok := s.planCraft(p, chosen, false); ok {
			next = lost
		}
	}
	money, _ := inventoryMoney(p.Player.Char.Money)
	p.Player.Char.Money = moneyFromInventory(money - chosen.Cost)
	p.Player.Bag = next
	if succeeded && chosen.RequiredSkill != 0 && chosen.ProficiencyGain > 0 {
		value := p.Player.Char.Skills[chosen.RequiredSkill] + chosen.ProficiencyGain
		if chosen.ProficiencyMax > 0 && value > chosen.ProficiencyMax {
			value = chosen.ProficiencyMax
		}
		p.Player.Char.Skills[chosen.RequiredSkill] = value
	}
	p.Player.MarkDirty()
	_ = s.pushInventory(p)
	text := "合成失败。"
	if succeeded {
		text = "合成成功。"
	} else if consumeMaterials {
		text = "合成失败，材料消失。"
	}
	s.emitTo(p.ID, event.ServerNotice{Who: p.ID, Text: text})
	s.emitTo(p.ID, s.craftRecipeSnapshot(p, cmd.MakeType))
	if s.saver != nil {
		s.saver.Save(s.snapshotOf(p))
	}
}
