package scene

import (
	"fmt"
	"math"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func (s *Scene) usePetPPItem(owner *entity.Entity, bagSlot int, item domain.ItemDef,
	startCooldown func(), reject func(event.RejectReason)) {
	pet := s.entities[owner.Player.Pet]
	if pet == nil || pet.Pet == nil || pet.Pet.Inst == nil || !pet.Alive() || !pet.Pet.Inst.Hatched {
		reject(event.RejectNoPet)
		return
	}
	inst, rule := pet.Pet.Inst, item.PetAffectionPP
	if inst.PPAiUsed >= rule.Limit {
		s.emitTo(owner.ID, event.ServerNotice{Who: owner.ID, Text: "该宠物已达宠爱PP果上限，物品未消耗"})
		return
	}
	chance, ok := rule.Chance(inst.PPAiUsed)
	if !ok || inst.PPAiUsed < 0 || inst.FreePoints == math.MaxInt32 {
		reject(event.RejectInvalid)
		return
	}
	if !owner.Player.Bag.RemoveAt(bagSlot, 1) {
		reject(event.RejectNoItem)
		return
	}
	success := s.rng.Intn(100) < int(chance)
	text := "宠爱PP果未成功增加点数"
	if success {
		inst.PPAiUsed++
		inst.FreePoints++
		text = fmt.Sprintf("宠物剩余点数增加1，宠爱PP果已成功使用%d/%d", inst.PPAiUsed, rule.Limit)
	}
	owner.Player.MarkDirty()
	startCooldown()
	s.pushInventory(owner)
	s.pushPetSnapshot(owner)
	s.emitTo(owner.ID, event.ServerNotice{Who: owner.ID, Text: text})
	s.log.Info("使用宠爱PP果", "char", owner.Name, "pet", inst.ID,
		"success", success, "used", inst.PPAiUsed, "chance", chance)
}
