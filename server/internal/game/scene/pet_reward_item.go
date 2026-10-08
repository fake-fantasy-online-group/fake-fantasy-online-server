package scene

import (
	"fmt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func (s *Scene) usePetRewardItem(owner *entity.Entity, slot int, item domain.ItemDef,
	startCooldown func(), reject func(event.RejectReason)) {
	if !s.petItemRoom(owner, 1) {
		s.emitTo(owner.ID, event.ServerNotice{Who: owner.ID, Text: "宠物栏已满，礼盒未消耗"})
		return
	}
	// 整个奖池均须能兑现，不能抽中未配置项后重抽或扣除礼盒。
	var total int
	for _, reward := range item.PetRewards {
		pet, ok := s.petDefs[reward.Pet]
		_, prefixOK := s.petPrefixes.Find(reward.Prefix)
		if !ok || !pet.RealPet || pet.ModelID() <= 0 || !prefixOK || s.petSkills == nil || reward.Weight <= 0 {
			reject(event.RejectInvalid)
			return
		}
		total += int(reward.Weight)
	}
	if total != 10000 {
		reject(event.RejectInvalid)
		return
	}
	roll := s.rng.Intn(total)
	var chosen domain.PetItemReward
	for _, reward := range item.PetRewards {
		roll -= int(reward.Weight)
		if roll < 0 {
			chosen = reward
			break
		}
	}
	pet := s.petDefs[chosen.Pet]
	inst := domain.NewPetInstance(pet)
	inst.Prefix, inst.Hatched = chosen.Prefix, true
	inst.Gender = uint8(s.rng.Intn(2))
	s.assignInitialPetSkills(&inst, pet)
	inst.ID, inst.Slot = s.nextPetInstID(owner.Player.Char), s.nextPetSlot(owner.Player.Char)
	if !owner.Player.Bag.RemoveAt(slot, 1) {
		reject(event.RejectNoItem)
		return
	}
	s.appendPet(owner, inst)
	owner.Player.MarkDirty()
	startCooldown()
	s.pushInventory(owner)
	s.pushPetSnapshot(owner)
	s.emitTo(owner.ID, event.ServerNotice{Who: owner.ID,
		Text: fmt.Sprintf("获得%s%s，已进入宠物栏", s.petPrefixName(inst.Prefix), pet.Name)})
	s.log.Info("打开宠物礼盒", "char", owner.Name, "item", item.ID, "pet", inst.Def, "prefix", inst.Prefix)
}
