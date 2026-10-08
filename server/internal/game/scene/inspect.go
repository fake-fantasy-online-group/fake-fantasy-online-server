package scene

import (
	"sort"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func (s *Scene) onInspectPlayer(cmd InspectPlayer) {
	requester := s.players[cmd.ID]
	target := s.players[cmd.Target]
	if requester == nil || requester.Player == nil || target == nil || target.Player == nil ||
		target.Player.Char == nil || target.Player.Worn == nil {
		return
	}
	ch := target.Player.Char
	attrs := s.attributeSnapshot(target).Total
	out := event.PlayerEquipmentInspected{
		Who: cmd.ID, Target: target.ID, Name: ch.Name, Level: ch.Level,
		Race: target.Look.Race, Gender: ch.Appear.Gender, Head: ch.Appear.Head,
		Hair: ch.Appear.Hair, Title: ch.Appear.Title, Honor: ch.Honor,
		Appearance: ch.Appear, Attrs: attrs,
		MaxAtk: target.Stats.MaxAtk, MinAtk: target.Stats.MinAtk, Def: target.Stats.Def,
		MAtk: target.Stats.MAtk, MDef: target.Stats.MDef, Hit: target.Stats.Hit,
	}
	type wornItem struct {
		slot domain.EquipSlot
		item domain.Stack
	}
	var worn []wornItem
	target.Player.Worn.Each(func(slot domain.EquipSlot, item domain.Stack) {
		worn = append(worn, wornItem{slot: slot, item: item})
	})
	sort.Slice(worn, func(i, j int) bool { return worn[i].slot < worn[j].slot })
	for _, row := range worn {
		def, ok := s.itemDef(row.item.Item)
		if !ok || def.Equip == nil {
			return
		}
		out.Equipped = append(out.Equipped, event.EquippedItemView{
			Slot: row.slot, Item: row.item.Item, Name: itemDisplayName(def, row.item), Desc: s.ownedItemTooltip(def, row.item, target),
			CurrentDurability: row.item.Durability,
			MaxDurability:     domain.MaxDurabilityOf(row.item, def),
			Avatar:            0,
			SoulAvatar:        int32(row.item.FusedSoul),
			Quality:           equipmentQuality(def, row.item),
		})
	}
	s.emitTo(requester.ID, out)
}
