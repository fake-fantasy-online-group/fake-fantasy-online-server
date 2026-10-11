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
		v.Level = d.Equip.Need.Level
		// Bit (1 << Character.Race): warrior/swordsman/assassin/healer/warlock.
		// This is not an ordinal or a complete requirement: beginner and sex
		// restrictions remain in the native equipment requirements/tooltip.
		v.Profession = int32(d.Equip.Need.Professions)
	}
	for i := 0; i < int(st.SocketCount) && i < len(st.Sockets); i++ {
		v.Sockets = append(v.Sockets, domain.EndgameSocketView{Index: int32(i), ItemID: int32(st.Sockets[i]), UID: strconv.FormatInt(st.SocketUIDs[i], 10)})
	}
	if u, ok := domain.UniqueOf(st); ok {
		v.Name = u.Name
		v.Description = u.Lore
		v.Stats = endgameStats(domain.UniqueAffixes(st))
		ts := []string{}
		for _, t := range u.Traits {
			ts = append(ts, t.Describe(st.Endgame.UniqueRollBP))
		}
		v.Trait = strings.Join(ts, "；")
	}
	if f, ok := domain.MatchAstralRecipe(d, st); ok {
		v.FormationID = f.ID
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
	for _, a := range domain.ActiveAstralRecipes(p.Player.Worn, s.itemDef, p.Player.Char) {
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
			v.InactiveReason = "职业、等级、耐久或同类宿阵优先级未满足"
		}
		items = append(items, v)
	})
	materials := []domain.EndgameMaterialView{}
	for _, id := range append([]domain.ItemID{domain.RiftKeyItemID, domain.CubeStarMarrow, domain.CubePurifiedDust, domain.CubeImmortalEcho}, astralItemIDs()...) {
		if d, ok := s.itemDef(id); ok {
			materials = append(materials, domain.EndgameMaterialView{ItemID: int32(id), Name: d.Name, Count: p.Player.Bag.CountOf(id)})
		}
	}
	formations := []domain.EndgameFormationView{}
	for _, a := range domain.ActiveAstralRecipes(p.Player.Worn, s.itemDef, p.Player.Char) {
		f := a.Recipe
		v := domain.EndgameFormationView{ID: f.ID, Name: f.Name, Channel: f.Channel.String(), Active: true, Description: f.Lore, Stats: endgameStats(f.Affixes)}
		for _, r := range f.Runes {
			v.Runes = append(v.Runes, int32(r))
		}
		formations = append(formations, v)
	}
	const size = 32
	n := (len(items) + size - 1) / size
	if n == 0 {
		n = 1
	}
	x.snapshotSequence++
	x.snapshotID = fmt.Sprintf("%d-%d-%d", p.ID, s.tick, x.snapshotSequence)
	x.snapshotItems = items
	for i := 0; i < n; i++ {
		end := (i + 1) * size
		if end > len(items) {
			end = len(items)
		}
		s.endgameReply(p, domain.EndgameResponse{Kind: "snapshot", OK: true, SnapshotID: x.snapshotID, Chunk: int32(i), Chunks: int32(n), Items: items[i*size : end], Materials: materials, Formations: formations})
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
		all := domain.UniqueCatalog()
		total = len(all)
		for i := start; i < len(all) && i < start+page; i++ {
			u := all[i]
			r.Items = append(r.Items, domain.EndgameItemView{ItemID: int32(u.BaseID), UniqueID: u.ID, Name: u.Name, Description: u.Lore,
				Level: u.LevelRequirement, Profession: int32(u.Professions), UniqueRollBP: uniqueCatalogDisplayRollBP,
				Stats: endgameStats(u.Affixes), Trait: uniqueTraitDescription(u.Traits)})
		}
	case "runes":
		all := domain.AstralRunes()
		total = len(all)
		for i := start; i < len(all) && i < start+page; i++ {
			u := all[i]
			r.Runes = append(r.Runes, domain.EndgameRuneView{ItemID: int32(u.Item), Name: u.Name, Description: u.Mansion, Quadrant: u.Quadrant, Stats: endgameStats(u.Affixes)})
		}
	case "formations":
		all := domain.AstralRecipes()
		total = len(all)
		for i := start; i < len(all) && i < start+page; i++ {
			u := all[i]
			v := domain.EndgameFormationView{ID: u.ID, Name: u.Name, Description: u.Lore, Channel: u.Channel.String(), MinLevel: u.MinLevel, Professions: int32(u.Professions), Stats: endgameStats(u.Affixes)}
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
	if start+page < total {
		r.NextCursor = strconv.Itoa(start + page)
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

func astralItemIDs() []domain.ItemID {
	ids := []domain.ItemID{}
	for _, r := range domain.AstralRunes() {
		ids = append(ids, r.Item)
	}
	return ids
}
