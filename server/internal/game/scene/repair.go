package scene

import (
	"fmt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

const (
	repairOrdinary uint8 = iota
	repairSpecial
	repairAll
)

const (
	// 正式客户端 RepairDlg.PickBag 固定发送 0，PickSlot 固定发送 1。
	repairTargetBag uint8 = iota
	repairTargetWorn
)

func (s *Scene) onRepair(cmd Repair) {
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil {
		return
	}
	worn := p.Player.Worn.At(domain.EquipSlot(cmd.Slot))
	bag := p.Player.Bag.At(int(cmd.Slot))
	s.log.Debug("收到 NPC 修理报价", "char", p.Name, "mode", cmd.Mode, "slot", cmd.Slot,
		"targetKind", cmd.TargetKind, "tab", cmd.Tab, "wornItem", worn.Item, "bagItem", bag.Item)
	p.Player.RepairQuote = nil
	shop, ok := s.repairShop(p)
	if !p.Alive() || !ok || cmd.Mode > repairAll || cmd.TargetKind > repairTargetWorn ||
		(cmd.Mode != repairAll && cmd.Slot < 0) {
		s.repairMessage(p, "当前无法修理装备。")
		return
	}

	q := &entity.RepairQuote{
		Mode: cmd.Mode, TargetKind: cmd.TargetKind, Tab: cmd.Tab, Slot: cmd.Slot,
		ShopID: shop.ID,
	}
	var firstDef domain.ItemDef
	quoteValid := true
	if cmd.Mode == repairAll {
		p.Player.Worn.Each(func(slot domain.EquipSlot, st domain.Stack) {
			def, ok := s.repairable(st, repairOrdinary)
			if !ok {
				return
			}
			ref := repairRef(repairTargetWorn, 0, int32(slot), st, def)
			fee, ok := repairItemFee(def, st, shop.RepairPriceBP)
			if !ok {
				quoteValid = false
				return
			}
			if q.Fee > int64(^uint64(0)>>1)-fee {
				quoteValid = false
				return
			}
			q.Items = append(q.Items, ref)
			q.Fee += fee
			if firstDef.ID == 0 {
				firstDef = def
			}
		})
	} else {
		st, def, ok := s.repairTarget(p, cmd.TargetKind, cmd.Tab, cmd.Slot)
		if !ok {
			s.repairMessage(p, "没有找到要修理的装备。")
			return
		}
		if _, ok = s.repairable(st, cmd.Mode); !ok {
			s.repairMessage(p, "这件装备不能使用所选方式修理，或当前无需修理。")
			return
		}
		bp := shop.RepairPriceBP
		if cmd.Mode == repairSpecial {
			bp = shop.SpecialRepairPriceBP
			q.Nianli = shop.SpecialRepairNianli
		}
		fee, feeOK := repairItemFee(def, st, bp)
		if !feeOK {
			s.repairMessage(p, "修理费用计算失败。")
			return
		}
		q.Fee, firstDef = fee, def
		q.Items = append(q.Items, repairRef(cmd.TargetKind, cmd.Tab, cmd.Slot, st, def))
	}

	if !quoteValid {
		s.repairMessage(p, "修理费用计算失败。")
		return
	}
	if len(q.Items) == 0 {
		s.emitTo(p.ID, event.RepairQuoted{
			Who: p.ID, Mode: cmd.Mode, Slot: repairClientSlot(cmd.TargetKind, cmd.Slot), TargetKind: cmd.TargetKind,
			Tab: cmd.Tab, Info: "没有需要修理的装备。",
		})
		return
	}
	if q.Fee < 0 {
		s.repairMessage(p, "修理费用计算失败。")
		return
	}
	p.Player.RepairQuote = q
	info := fmt.Sprintf("%s：修理 %d 件装备", firstDef.Name, len(q.Items))
	if cmd.Mode == repairSpecial {
		if q.Nianli == 0 {
			info = fmt.Sprintf("%s：特殊修理，不消耗念力", firstDef.Name)
		} else {
			info = fmt.Sprintf("%s：特殊修理，消耗 %d 点念力", firstDef.Name, q.Nianli)
		}
	} else if cmd.Mode == repairOrdinary {
		info = fmt.Sprintf("%s：普通修理会降低耐久上限", firstDef.Name)
	}
	s.emitTo(p.ID, event.RepairQuoted{
		Who: p.ID, Mode: cmd.Mode, Slot: repairClientSlot(cmd.TargetKind, cmd.Slot), Fee: q.Fee,
		Count: int32(len(q.Items)), Item: firstDef.ID, Info: info,
		TargetKind: cmd.TargetKind, Tab: cmd.Tab,
	})
}

func (s *Scene) onRepairConfirm(cmd RepairConfirm) {
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil {
		return
	}
	q := p.Player.RepairQuote
	p.Player.RepairQuote = nil // 每份报价只能确认一次，失败后也必须重新报价。
	shop, ok := s.repairShop(p)
	if !p.Alive() || !ok || q == nil || q.ShopID != shop.ID || q.Mode != cmd.Mode ||
		q.Slot != cmd.Slot || q.TargetKind != cmd.TargetKind || q.Tab != cmd.Tab || len(q.Items) == 0 {
		s.repairMessage(p, "修理报价已失效，请重新选择。")
		return
	}

	money, err := inventoryMoney(p.Player.Char.Money)
	if err != nil || money < q.Fee {
		s.repairMessage(p, "金钱不足，无法修理。")
		return
	}
	if p.Player.Char.EffectiveNianli() < q.Nianli {
		s.repairMessage(p, "念力不足，无法特殊修理。")
		return
	}

	bag := p.Player.Bag.Clone()
	worn := p.Player.Worn.Clone()
	if bag == nil || worn == nil {
		s.repairMessage(p, "修理失败，请稍后重试。")
		return
	}
	wornChanged := false
	for _, ref := range q.Items {
		st, def, valid := repairStackFrom(bag, worn, ref, s.itemDef)
		if !valid || !repairFingerprintMatches(ref, st, def) {
			s.repairMessage(p, "装备状态已变化，请重新报价。")
			return
		}
		maxDurability := domain.MaxDurabilityOf(st, def)
		rawLoss := def.Equip.RepairDurabilityCostRaw
		if q.Mode == repairSpecial {
			rawLoss = def.Equip.SpecRepairDurabilityCostRaw
		}
		maxLoss := (rawLoss + domain.DurabilityRawPerPoint - 1) / domain.DurabilityRawPerPoint
		maxDurability -= maxLoss
		if maxDurability < 1 {
			maxDurability = 1
		}
		st.MaxDurability = maxDurability
		st.Durability = maxDurability
		st.DurabilityWearRaw = 0
		if ref.TargetKind == repairTargetWorn {
			worn.Set(domain.EquipSlot(ref.Slot), st)
			wornChanged = true
		} else if !bag.Set(int(ref.Slot), st) {
			s.repairMessage(p, "装备状态已变化，请重新报价。")
			return
		}
	}

	oldBag, oldWorn, oldMoney := p.Player.Bag, p.Player.Worn, p.Player.Char.Money
	oldTreasureSkill := s.equippedTreasureSkill(p)
	oldNianli := p.Player.Char.EffectiveNianli()
	p.Player.Bag, p.Player.Worn = bag, worn
	p.Player.Char.Money = moneyFromInventory(money - q.Fee)
	p.Player.Char.SetNianli(oldNianli - q.Nianli)
	treasureSkillChanged := equipmentSkillID(oldTreasureSkill) != equipmentSkillID(s.equippedTreasureSkill(p))
	if wornChanged {
		s.refreshStats(p)
		s.refreshAppearance(p)
		if treasureSkillChanged {
			s.emitTo(p.ID, s.skillSnapshot(p))
		}
	} else if q.Nianli > 0 {
		s.emitTo(p.ID, s.attributeSnapshot(p))
	}
	if err := s.pushInventory(p); err != nil {
		p.Player.Bag, p.Player.Worn, p.Player.Char.Money = oldBag, oldWorn, oldMoney
		p.Player.Char.SetNianli(oldNianli)
		if wornChanged {
			s.refreshStats(p)
			s.refreshAppearance(p)
			if treasureSkillChanged {
				s.emitTo(p.ID, s.skillSnapshot(p))
			}
		}
		s.repairMessage(p, "修理失败，请稍后重试。")
		return
	}
	p.Player.MarkDirty()
	if s.saver != nil {
		s.saver.Save(s.snapshotOf(p))
	}
	s.log.Debug("NPC 修理完成", "char", p.Name, "shop", shop.ID,
		"mode", q.Mode, "count", len(q.Items), "fee", q.Fee, "nianli", q.Nianli)
}

func (s *Scene) repairShop(p *entity.Entity) (domain.Shop, bool) {
	if p == nil || p.Player == nil {
		return domain.Shop{}, false
	}
	shop, ok := s.shops[p.Player.ShopID]
	return shop, ok && shop.AllowRepair && s.hasShopNPC(shop.ID)
}

func (s *Scene) repairTarget(p *entity.Entity, targetKind, tab uint8, slot int32) (domain.Stack, domain.ItemDef, bool) {
	var st domain.Stack
	if targetKind == repairTargetWorn {
		st = p.Player.Worn.At(domain.EquipSlot(slot))
	} else if targetKind == repairTargetBag {
		st = p.Player.Bag.At(int(slot))
	} else {
		return domain.Stack{}, domain.ItemDef{}, false
	}
	def, ok := s.itemDef(st.Item)
	if !ok || st.Empty() || def.Equip == nil || (targetKind == repairTargetBag &&
		(!def.InventoryTabKnown || def.InventoryTab != tab)) {
		return domain.Stack{}, domain.ItemDef{}, false
	}
	return st, def, true
}

func (s *Scene) repairable(st domain.Stack, mode uint8) (domain.ItemDef, bool) {
	def, ok := s.itemDef(st.Item)
	if !ok || st.Empty() || def.Equip == nil || def.Equip.Durable <= 0 ||
		def.Equip.NoLimitDurability || st.Durability >= domain.MaxDurabilityOf(st, def) {
		return domain.ItemDef{}, false
	}
	if (mode == repairSpecial && !def.Equip.CanSpecialRepair) ||
		(mode != repairSpecial && !def.Equip.CanRepair) {
		return domain.ItemDef{}, false
	}
	return def, true
}

func repairRef(targetKind, tab uint8, slot int32, st domain.Stack, def domain.ItemDef) entity.RepairQuoteItem {
	return entity.RepairQuoteItem{
		TargetKind: targetKind, Tab: tab, Slot: slot, UID: st.UID, Item: st.Item,
		Durability: st.Durability, MaxDurability: domain.MaxDurabilityOf(st, def),
		DurabilityWearRaw: st.DurabilityWearRaw,
	}
}

func repairItemFee(def domain.ItemDef, st domain.Stack, priceBP int32) (int64, bool) {
	maxDurability := domain.MaxDurabilityOf(st, def)
	missing := maxDurability - st.Durability
	// 任务/赠送装备可能买价为 0，但仍由 CanRepair/CanSpecialRepair 明确
	// 允许修理。零价继续走下面已有的最低 1 铜币规则，不能当成无效配置。
	if def.Price < 0 || maxDurability <= 0 || missing <= 0 || priceBP <= 0 {
		return 0, false
	}
	numerator, ok := checkedMul64(def.Price, int64(missing))
	if !ok {
		return 0, false
	}
	numerator, ok = checkedMul64(numerator, int64(priceBP))
	if !ok {
		return 0, false
	}
	denominator := int64(maxDurability) * 10_000
	fee := numerator / denominator
	if numerator%denominator != 0 {
		fee++
	}
	if fee < 1 {
		fee = 1
	}
	return fee, true
}

func repairStackFrom(bag *domain.Bag, worn *domain.EquipSet, ref entity.RepairQuoteItem,
	defs func(domain.ItemID) (domain.ItemDef, bool)) (domain.Stack, domain.ItemDef, bool) {
	var st domain.Stack
	if ref.TargetKind == repairTargetWorn {
		st = worn.At(domain.EquipSlot(ref.Slot))
	} else if ref.TargetKind == repairTargetBag {
		st = bag.At(int(ref.Slot))
	} else {
		return domain.Stack{}, domain.ItemDef{}, false
	}
	def, ok := defs(st.Item)
	return st, def, ok && def.Equip != nil
}

func repairFingerprintMatches(ref entity.RepairQuoteItem, st domain.Stack, def domain.ItemDef) bool {
	return !st.Empty() && st.UID == ref.UID && st.Item == ref.Item &&
		st.Durability == ref.Durability && domain.MaxDurabilityOf(st, def) == ref.MaxDurability &&
		st.DurabilityWearRaw == ref.DurabilityWearRaw
}

func (s *Scene) repairMessage(p *entity.Entity, text string) {
	if p != nil {
		s.log.Debug("NPC 修理提示", "char", p.Name, "text", text)
		s.emitTo(p.ID, event.NpcDialogText{Who: p.ID, Text: text})
	}
}

func repairClientSlot(kind uint8, slot int32) int32 {
	if kind == repairTargetBag {
		return domain.ClientBagPosition(slot)
	}
	return slot
}
