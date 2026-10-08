package scene

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"

func (s *Scene) onResolveChatShares(cmd ResolveChatShares) {
	result := ChatShareResult{}
	defer func() { cmd.Reply <- result }()
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil || p.Player.Char == nil || p.Player.Bag == nil || len(cmd.Refs) > 4 {
		result.Reason = "分享引用无效"
		return
	}
	for _, ref := range cmd.Refs {
		var st domain.Stack
		if ref.Kind == 1 {
			// 宠物面板传的是 0x800b 可见宠物槽位，不是背包格。
			visible := int32(0)
			var uid int64
			for _, i := range orderedPetIndices(p.Player.Char) {
				inst := &p.Player.Char.Pets[i]
				if _, ok := s.petDefs[inst.Def]; !ok {
					continue
				}
				if visible == ref.Slot {
					uid = inst.ItemUID
					break
				}
				visible++
			}
			if uid > 0 {
				p.Player.Bag.Each(func(_ int, candidate domain.Stack) {
					if candidate.UID == uid {
						st = candidate
					}
				})
			}
		} else {
			st = p.Player.Bag.At(int(ref.Slot))
		}
		def, ok := s.itemDef(st.Item)
		if !ok || st.Empty() || ref.Kind != 1 && (!def.InventoryTabKnown || def.InventoryTab != ref.Tab) {
			result.Reason = "分享物品已经不在该背包格"
			return
		}
		share := domain.ChatShare{Kind: ref.Kind, Item: st.Item, Quality: equipmentQuality(def, st),
			Name: itemDisplayName(def, st), Desc: s.itemTooltip(def, st)}
		if def.PetCarrierSpecies > 0 || ref.Kind == 1 {
			pet := s.petItemInfo(p, st)
			if def.PetCarrierSpecies == 0 || pet == nil {
				result.Reason = "分享宠物已经不在背包中"
				return
			}
			share.Kind = 1
			share.Pet = pet
			share.Name = s.ownedItemName(p, def, st)
			share.Desc = share.Name
			share.PPAiUsed, share.PPAiCap = share.Pet.PPAiUsed, share.Pet.PPAiCap
		}
		result.Shares = append(result.Shares, share)
	}
}
