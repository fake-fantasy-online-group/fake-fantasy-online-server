package scene

import (
	"fmt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// saddleForPet 同时用于使用和重登校验。合体鞍只借用两只已孵化宠物，
// 不改种族、不合并或删除实例；组合只改变骑乘模型。
func (s *Scene) saddleForPet(ch *domain.Character, inst *domain.PetInstance, id domain.ItemID) *domain.SaddleDef {
	item, ok := s.itemDef(id)
	if ch == nil || !ok || item.Saddle == nil || inst == nil || !inst.Hatched || !item.Saddle.Allows(inst.Def) {
		return nil
	}
	d := item.Saddle
	if d.CombinationModel != 0 {
		found := false
		for i := range ch.Pets {
			other := &ch.Pets[i]
			if other.ID != inst.ID && other.Hatched && other.HP > 0 && d.Allows(other.Def) && (!d.DistinctPets || other.Def != inst.Def) {
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}
	return d
}

func (s *Scene) useSaddle(owner *entity.Entity, item domain.ItemDef, reject func(event.RejectReason)) {
	pet := s.entities[owner.Player.Pet]
	if pet == nil || pet.Pet == nil || pet.Pet.Inst == nil || !pet.Alive() {
		reject(event.RejectNoPet)
		return
	}
	inst := pet.Pet.Inst
	// 鞍具等同于特殊宠物的骑乘技能：每次切换上下马，始终保留道具。
	// 下马不重新校验合体伙伴，避免伙伴变化后无法结束骑乘。
	if owner.Player.Riding && inst.RidingSaddle == item.ID {
		s.setRiding(owner, false, true)
	} else {
		if s.saddleForPet(owner.Player.Char, inst, item.ID) == nil ||
			(item.Saddle.UsePetMaxSpeed && pet.Pet.Def.HorseSpeedLimit <= 0) {
			s.emitTo(owner.ID, event.ServerNotice{Who: owner.ID, Text: "当前宠物不适用此鞍具；合体鞍还需要携带另一只符合条件的已孵化宠物"})
			return
		}
		if owner.Player.Riding {
			s.setRiding(owner, false, true)
		}
		inst.ActiveSkill = 0
		inst.RidingSaddle = item.ID
		s.setRiding(owner, true, true)
	}
	owner.Player.MarkDirty()
	s.pushPetSnapshot(owner)
	state := "已下马"
	if owner.Player.Riding {
		state = "已骑乘"
	}
	s.emitTo(owner.ID, event.ServerNotice{Who: owner.ID, Text: fmt.Sprintf("%s：%s", item.Name, state)})
	s.log.Info("使用骑乘鞍具", "char", owner.Name, "item", item.ID, "pet", inst.ID,
		"riding", owner.Player.Riding, "speed_px", s.playerMoveSpeedPX(owner))
}

func (s *Scene) ridingModel(owner *entity.Entity, inst *domain.PetInstance, def domain.PetDef) int32 {
	if inst.RidingSaddle != 0 {
		if d := s.saddleForPet(owner.Player.Char, inst, inst.RidingSaddle); d != nil && d.CombinationModel > 0 {
			return d.CombinationModel
		}
	}
	return inst.RenderModel(def, s.now())
}
