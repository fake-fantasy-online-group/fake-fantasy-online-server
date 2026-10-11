package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"math"
)

func (s *Scene) nativeEndgameStats(viewer *entity.Entity) bool {
	return viewer != nil && viewer.Player != nil && viewer.Player.EndgameNativeStats
}

func localEnhancementLine(v domain.ItemLocalEnhancement, roll int32) string {
	name := map[int32]string{domain.AttrAtk: "攻击", domain.AttrMAtk: "魔法攻击", domain.AttrDef: "防御", domain.AttrMDef: "魔法防御"}[v.Attr]
	return fmt.Sprintf("%s强化 +%d%%（本件）", name, v.ValueAt(roll))
}
func localEnhancementLines(def domain.ItemDef, local []domain.ItemLocalEnhancement, roll int32) []string {
	var out []string
	for _, v := range local {
		// A mixed-slot or zero-base fixture must never advertise an inert local line.
		if len(domain.ResolveItemLocalEnhancements(def.Equip, []domain.ItemLocalEnhancement{v}, roll)) > 0 {
			out = append(out, localEnhancementLine(v, roll))
		}
	}
	return out
}

// Merge only the same attribute, mode and scope. All inputs here are ordinary
// global affixes; item-local percentages never enter this aggregation.
func mergedEndgameAffixes(groups ...[]domain.Affix) []domain.Affix {
	type key struct{ attr, mode int32 }
	order := []key{}
	totals := map[key]int64{}
	for _, rows := range groups {
		for _, a := range rows {
			k := key{a.Attr, a.Mode}
			if _, ok := totals[k]; !ok {
				order = append(order, k)
			}
			totals[k] += int64(a.Value)
		}
	}
	var out []domain.Affix
	for _, k := range order {
		value := totals[k]
		if value != 0 {
			out = append(out, domain.Affix{Attr: k.attr, Mode: k.mode, Value: int32(max(int64(math.MinInt32), min(int64(math.MaxInt32), value)))})
		}
	}
	return out
}
func (s *Scene) tooltipFormation(def domain.ItemDef, st domain.Stack, owner *entity.Entity) (domain.AstralRecipe, bool) {
	recipe, matched := domain.MatchAstralRecipe(def, st)
	if !matched {
		return domain.AstralRecipe{}, false
	}
	// All equipment descriptions are potential-item previews, like broken-item
	// white stats. Active/InactiveReason controls actual combat and the badge.
	return recipe, true
}
