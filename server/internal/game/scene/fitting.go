package scene

import (
	"strings"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func (s *Scene) onOpenFitting(cmd OpenFitting) {
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil || p.Player.Char == nil || !s.rack.Valid() {
		return
	}
	items := s.fittingCatalog(cmd.Kind, cmd.Part, cmd.Search)
	if !cmd.Paged {
		s.emitTo(cmd.ID, event.FittingCatalogSnapshot{Who: cmd.ID, Items: items, Total: int32(len(items))})
		return
	}
	start := int64(cmd.Page) * int64(cmd.PerPage)
	end := start + int64(cmd.PerPage)
	if start > int64(len(items)) {
		start = int64(len(items))
	}
	if end > int64(len(items)) {
		end = int64(len(items))
	}
	s.emitTo(cmd.ID, event.FittingCatalogSnapshot{Who: cmd.ID,
		Items: items[int(start):int(end)], Total: int32(len(items)), Page: cmd.Page, Paged: true})
}

func (s *Scene) onFittingDetails(cmd FittingDetails) {
	p := s.players[cmd.ID]
	if p == nil || p.Player == nil || p.Player.Char == nil || !s.rack.Valid() {
		return
	}
	seen := make(map[domain.ItemID]struct{}, len(cmd.Items))
	out := event.FittingDetailsSnapshot{Who: cmd.ID}
	for _, itemID := range cmd.Items {
		if _, duplicate := seen[itemID]; duplicate {
			continue
		}
		seen[itemID] = struct{}{}
		good, exists := s.rack.Good(itemID)
		def, known := s.itemDef(itemID)
		view, fit := fittingItem(good, def)
		if !exists || !known || !fit {
			continue
		}
		out.Items = append(out.Items, event.FittingDetailView{Item: view.Item, Name: view.Name,
			Price: view.Price, Desc: view.Desc})
	}
	s.emitTo(cmd.ID, out)
}

func (s *Scene) fittingCatalog(kind uint8, part, search string) []event.FittingItemView {
	part = strings.TrimSpace(strings.ToLower(part))
	search = strings.TrimSpace(strings.ToLower(search))
	out := make([]event.FittingItemView, 0, len(s.rack.Goods))
	for _, good := range s.rack.Goods {
		def, ok := s.itemDef(good.Item)
		if !ok {
			continue
		}
		view, ok := fittingItem(good, def)
		if !ok || view.Kind != kind || part != "" && strings.ToLower(view.Part) != part {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(view.Name), search) &&
			!strings.Contains(strings.ToLower(view.Desc), search) {
			continue
		}
		out = append(out, view)
	}
	return out
}

func fittingItem(good domain.RackGood, def domain.ItemDef) (event.FittingItemView, bool) {
	view := event.FittingItemView{Item: good.Item, Name: def.Name, Desc: def.Description,
		Price: int32(good.Price)}
	var appearance domain.EquipAppearance
	switch {
	case def.PetTransmog != nil && def.PetTransmog.Valid():
		view.Kind, view.Model, view.Part = 2, def.PetTransmog.Model, good.Part
		return view, view.Model > 0
	case def.AvatarFusion != nil && def.AvatarFusion.Appearance.Known:
		appearance = def.AvatarFusion.Appearance
	case def.Equip != nil && def.Equip.Appearance.Known:
		appearance = def.Equip.Appearance
	default:
		return event.FittingItemView{}, false
	}
	view.Model = int32(appearance.Model)
	view.Part = good.Part
	if view.Part == "" {
		view.Part = fittingPart(appearance.Part)
	}
	return view, view.Model > 0 && view.Part != ""
}

func fittingPart(part domain.AppearancePart) string {
	switch part {
	case domain.AppearanceBody:
		return "服装"
	case domain.AppearanceCap:
		return "头盔"
	case domain.AppearanceBackpack:
		return "背包"
	case domain.AppearanceWeaponR:
		return "武器"
	case domain.AppearanceWeaponL:
		return "武器"
	case domain.AppearanceFace:
		return "面饰"
	default:
		return ""
	}
}
