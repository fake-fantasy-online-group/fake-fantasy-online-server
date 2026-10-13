package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"strconv"
	"strings"
)

// Catalog numbers describe the frozen 100% reference, not an instance roll.
// Owned items below retain their actual 80–120% UniqueRollBP.
const uniqueCatalogDisplayRollBP int32 = 10000

func endgameStats(a []domain.Affix) []domain.EndgameStatView {
	out := make([]domain.EndgameStatView, 0, len(a))
	for _, v := range a {
		out = append(out, domain.EndgameStatView{Attr: int32(v.Attr), Value: v.Value, Mode: int32(v.Mode)})
	}
	return out
}
func (s *Scene) endgameItem(st domain.Stack, loc string, slot int32) domain.EndgameItemView {
	d, _ := s.itemDef(st.Item)
	v := domain.EndgameItemView{UID: strconv.FormatInt(st.UID, 10), ItemID: int32(st.Item), Location: loc, Slot: slot, Revision: strconv.FormatInt(st.Endgame.Revision, 10), UniqueID: st.Endgame.UniqueID, UniqueRollBP: st.Endgame.UniqueRollBP, Name: d.Name, Tab: int32(d.InventoryTab), SocketCount: int32(st.SocketCount), Bound: st.Bound, Locked: st.Locked}
	if d.Equip != nil {
		// Use the actual onEquip character-level gate, not the resource tier.
		v.Level = domain.EquipmentRequiredLevel(d, st)
		// Bit (1 << Character.Race): warrior/swordsman/assassin/healer/warlock.
		// This is not an ordinal or a complete requirement: beginner and sex
		// restrictions remain in the native equipment requirements/tooltip.
		v.Profession = int32(d.Equip.Need.Professions)
	}
	if rune, ok := domain.AstralRune(st.Item, s.endgameCatalog); ok {
		v.Level = rune.MinLevel
	}
	for i := 0; i < int(st.SocketCount) && i < len(st.Sockets); i++ {
		name := ""
		if rune, ok := domain.AstralRune(st.Sockets[i], s.endgameCatalog); ok {
			name = rune.Name
		} else if card, ok := s.itemDef(st.Sockets[i]); ok {
			name = card.Name
		}
		v.Sockets = append(v.Sockets, domain.EndgameSocketView{Name: name, Index: int32(i), ItemID: int32(st.Sockets[i]), UID: strconv.FormatInt(st.SocketUIDs[i], 10)})
	}
	if u, ok := domain.UniqueOf(st, s.endgameCatalog); ok {
		v.Name = u.Name
		v.Description = u.Description()
		v.Stats = endgameStats(domain.UniqueAffixes(st, s.endgameCatalog))
		ts := []string{}
		for _, t := range u.Traits {
			ts = append(ts, t.Describe(10000))
		}
		v.Trait = strings.Join(ts, "；")
	}
	if f, ok := domain.MatchAstralRecipe(d, st); ok {
		v.FormationID = f.ID
		// Reuse the existing owned-item effect field; older closed-schema clients
		// cannot accept a new formation field. This describes potential effects,
		// while Active/InactiveReason remains the authoritative activation state.
		if text := uniqueTraitDescription(f.Traits); text != "" {
			if v.Trait != "" {
				v.Trait += "；"
			}
			v.Trait += text
		}
	}
	return v
}
func (s *Scene) sendEndgameSnapshot(p *entity.Entity) {
	if p == nil || p.Player == nil {
		return
	}
	x := s.endgamePlayer(p)
	if !x.loaded || x.commit != nil {
		return
	}
	items := []domain.EndgameItemView{}
	active := map[int64]bool{}
	functional := s.functionalWorn(p)
	for _, a := range domain.ActiveAstralRecipes(functional, s.itemDef, p.Player.Char) {
		active[a.UID] = true
	}
	p.Player.Bag.Each(func(slot int, st domain.Stack) {
		if st.UID != 0 {
			clientSlot := slot
			if p.Player.Bag.Cap() >= domain.DefaultBagSlots {
				clientSlot %= domain.BagPageSlots
			}
			items = append(items, s.endgameItem(st, "bag", int32(clientSlot)))
		}
	})
	p.Player.Worn.Each(func(slot domain.EquipSlot, st domain.Stack) {
		v := s.endgameItem(st, "equipped", int32(slot))
		v.Active = active[st.UID]
		if v.FormationID != 0 && !v.Active {
			v.InactiveReason = s.astralInactiveReason(p, slot, st, functional)
		}
		items = append(items, v)
	})
	materials := []domain.EndgameMaterialView{}
	// Validated at startup against the same bounded client material contract.
	for _, id := range domain.CubeSnapshotMaterialIDs(s.cubeRules, s.endgameCatalog) {
		if d, ok := s.itemDef(id); ok {
			materials = append(materials, domain.EndgameMaterialView{ItemID: int32(id), Name: d.Name, Count: p.Player.Bag.CountOf(id)})
		}
	}
	formations := []domain.EndgameFormationView{}
	for _, a := range domain.ActiveAstralRecipes(functional, s.itemDef, p.Player.Char) {
		f := a.Recipe
		v := domain.EndgameFormationView{ID: f.ID, Name: f.Name, Channel: f.Channel.String(), MinLevel: f.MinLevel, Professions: int32(f.Professions), Active: true, Description: f.Description(), Stats: endgameStats(domain.ResolveVariableAffixes(f.Affixes, f.VariableAffixes, functional.At(a.Slot).Endgame.FormationRollBP))}
		for _, slot := range f.Slots {
			v.Slots = append(v.Slots, int32(slot))
		}
		for _, r := range f.Runes {
			v.Runes = append(v.Runes, int32(r))
		}
		formations = append(formations, v)
	}
	x.snapshotSequence++
	x.snapshotID = fmt.Sprintf("%d-%d-%d", p.ID, s.tick, x.snapshotSequence)
	s.stepEquipmentAuras()
	chunks, ok := splitEndgameSnapshot(domain.EndgameResponse{Kind: "snapshot", OK: true, SnapshotID: x.snapshotID, Materials: materials, Formations: formations, CombatStats: s.equipmentCombatViewV5(p)}, items)
	if !ok {
		s.endgameReply(p, domain.EndgameResponse{Kind: "snapshot", Error: "snapshot_size", Message: "器物资料超出传输限制，请检查配置"})
		return
	}
	x.snapshotItems = items
	for _, chunk := range chunks {
		s.endgameReply(p, chunk)
	}

}
func (s *Scene) sendEndgameCatalog(p *entity.Entity, req domain.EndgameRequest) {
	kind := req.Catalog
	if kind == "" {
		kind = strings.TrimPrefix(req.Op, "catalog.")
	}
	start := 0
	if req.Cursor != "" {
		n, e := strconv.Atoi(req.Cursor)
		if e != nil || n < 0 {
			s.endgameError(p, req, "cursor", "目录游标无效")
			return
		}
		start = n
	}
	const page = 32
	r := domain.EndgameResponse{Kind: "catalog." + kind, RequestID: req.RequestID, OK: true, CatalogVersion: domain.UniqueCatalogVersion, Cursor: req.Cursor}
	total := 0
	switch kind {
	case "uniques":
		all := domain.UniqueCatalog(s.endgameCatalog)
		total = len(all)
		for i := start; i < len(all) && i < start+page; i++ {
			u := all[i]
			r.Items = append(r.Items, domain.EndgameItemView{ItemID: int32(u.BaseID), UniqueID: u.ID, Name: u.Name, Description: u.Description(),
				Level: u.LevelRequirement, Profession: int32(u.Professions), UniqueRollBP: uniqueCatalogDisplayRollBP,
				Stats: endgameStats(domain.ResolveVariableAffixes(u.Affixes, u.VariableAffixes, uniqueCatalogDisplayRollBP)), Trait: uniqueTraitDescription(u.Traits)})
		}
	case "runes":
		all := domain.AstralRunes(s.endgameCatalog)
		total = len(all)
		for i := start; i < len(all) && i < start+page; i++ {
			u := all[i]
			r.Runes = append(r.Runes, domain.EndgameRuneView{MinLevel: u.MinLevel, ItemID: int32(u.Item), Name: u.Name, Mansion: u.Mansion, Description: u.Description(), Quadrant: u.Quadrant, Stats: endgameStats(u.Affixes)})
		}
	case "formations":
		all := domain.AstralRecipes(s.endgameCatalog)
		total = len(all)
		for i := start; i < len(all) && i < start+page; i++ {
			u := all[i]
			v := domain.EndgameFormationView{ID: u.ID, Name: u.Name, Description: u.Description(), Channel: u.Channel.String(), MinLevel: u.MinLevel, Professions: int32(u.Professions), Stats: endgameStats(domain.ResolveVariableAffixes(u.Affixes, u.VariableAffixes, 10000))}
			for _, a := range u.Runes {
				v.Runes = append(v.Runes, int32(a))
			}
			for _, a := range u.Slots {
				v.Slots = append(v.Slots, int32(a))
			}
			r.Formations = append(r.Formations, v)
		}
	case "recipes":
		all := s.catalogCubeRecipeViews()
		total = len(all)
		for i := start; i < len(all) && i < start+page; i++ {
			r.Recipes = append(r.Recipes, all[i])
		}
	default:
		s.endgameError(p, req, "catalog", "目录不存在")
		return
	}
	if start > total {
		s.endgameError(p, req, "cursor", "目录游标越界")
		return
	}
	if !fitEndgameCatalogPage(&r, start, total) {
		s.endgameError(p, req, "catalog_size", "目录条目超出传输限制，请检查配置")
		return
	}
	s.endgameReply(p, r)
}

func uniqueTraitDescription(traits []domain.UniqueTrait) string {
	out := []string{}
	for _, t := range traits {
		out = append(out, t.Describe(uniqueCatalogDisplayRollBP))
	}
	return strings.Join(out, "；")
}

func (s *Scene) astralItemIDs() []domain.ItemID {
	ids := []domain.ItemID{}
	for _, r := range domain.AstralRunes(s.endgameCatalog) {
		ids = append(ids, r.Item)
	}
	return ids
}

// Only label channel suppression after all independent eligibility checks pass.
func (s *Scene) astralInactiveReason(p *entity.Entity, slot domain.EquipSlot, st domain.Stack, functional *domain.EquipSet) string {
	def, ok := s.itemDef(st.Item)
	if !ok || def.Equip == nil || int32(slot) != def.Equip.Slot {
		return "装备状态不符合要求"
	}
	ch := p.Player.Char
	if ch == nil || ch.Level < domain.EquipmentRequiredLevel(def, st) {
		return fmt.Sprintf("需要等级: %d", domain.EquipmentRequiredLevel(def, st))
	}
	if !domain.EquipmentFunctional(st, def) {
		return "装备耐久不足"
	}
	recipe, matched := domain.MatchAstralRecipe(def, st)
	if !matched {
		return "孔数、宿序或部位未匹配"
	}
	if !def.Equip.Need.AllowsCharacter(ch) || recipe.Professions != 0 && (!ch.HasProfession() || !ch.Race.Valid() || recipe.Professions&(1<<uint8(ch.Race)) == 0) {
		return "职业要求未满足"
	}
	if functional.At(slot).UID != st.UID {
		return "装备属性或性别要求未满足"
	}
	return "同类宿阵优先级未满足"
}
