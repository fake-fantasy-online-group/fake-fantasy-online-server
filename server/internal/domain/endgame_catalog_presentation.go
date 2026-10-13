package domain

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// Shared with the bounded runtime observation. These are transport/resource
// limits, not authored balance values or catalog-size requirements.
const MaxEquipmentProcReadiness = 16
const endgameCatalogRowBytes = 61440 - 1024
const endgameClientStringUnits = 8192

func catalogTextValid(s string) bool {
	return utf8.ValidString(s) && len(utf16.Encode([]rune(s))) <= endgameClientStringUnits
}
func validateCatalogText(v reflect.Value) error {
	switch v.Kind() {
	case reflect.String:
		if !catalogTextValid(v.String()) {
			return fmt.Errorf("catalog text exceeds client UTF-16 string limit")
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if err := validateCatalogText(v.Field(i)); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			if err := validateCatalogText(v.Index(i)); err != nil {
				return err
			}
		}
	case reflect.Pointer:
		if !v.IsNil() {
			return validateCatalogText(v.Elem())
		}
	}
	return nil
}
func catalogJSONSize(v any) int { b, _ := json.Marshal(v); return len(b) }
func catalogStats(a []Affix) []EndgameStatView {
	out := make([]EndgameStatView, 0, len(a))
	for _, v := range a {
		out = append(out, EndgameStatView{Attr: v.Attr, Value: math.MinInt32, Mode: v.Mode})
	}
	return out
}
func catalogTraits(ts []UniqueTrait) string {
	out := make([]string, 0, len(ts))
	for _, t := range ts {
		out = append(out, t.Describe(UniqueRollMaxBP))
	}
	return strings.Join(out, "；")
}
func catalogFormation(r AstralRecipe) EndgameFormationView {
	v := EndgameFormationView{ID: r.ID, Name: r.Name, Description: r.Description(), Channel: r.Channel.String(), MinLevel: r.MinLevel, Professions: int32(r.Professions), Stats: catalogStats(r.Affixes), Active: true}
	for _, id := range r.Runes {
		v.Runes = append(v.Runes, int32(id))
	}
	for _, slot := range r.Slots {
		v.Slots = append(v.Slots, int32(slot))
	}
	return v
}
func (c *EndgameCatalog) validatePresentationRows() error {
	check := func(source any, view any) error {
		if err := validateCatalogText(reflect.ValueOf(source)); err != nil {
			return err
		}
		if catalogJSONSize(view) > endgameCatalogRowBytes {
			return fmt.Errorf("catalog row exceeds transport frame budget")
		}
		return nil
	}
	for _, defs := range c.uniques {
		for _, u := range defs {
			trait := catalogTraits(u.Traits)
			if !catalogTextValid(trait) {
				return fmt.Errorf("unique %d rendered trait exceeds client limit", u.ID)
			}
			if err := check(u, EndgameItemView{ItemID: int32(u.BaseID), UniqueID: u.ID, Name: u.Name, Description: u.Description(), Trait: trait, Stats: catalogStats(u.Affixes)}); err != nil {
				return fmt.Errorf("unique %d: %w", u.ID, err)
			}
		}
	}
	for _, defs := range c.formations {
		for _, r := range defs {
			if !catalogTextValid(catalogTraits(r.Traits)) {
				return fmt.Errorf("formation %d rendered trait exceeds client limit", r.ID)
			}
			if err := check(r, catalogFormation(r)); err != nil {
				return fmt.Errorf("formation %d: %w", r.ID, err)
			}
		}
	}
	for _, r := range c.runeRows {
		if err := check(r, EndgameRuneView{ItemID: int32(r.Item), Name: r.Name, MinLevel: r.MinLevel, Mansion: r.Mansion, Quadrant: r.Quadrant, Description: r.Description(), Stats: catalogStats(r.Affixes)}); err != nil {
			return fmt.Errorf("rune %d: %w", r.Item, err)
		}
	}
	for _, v := range c.variants {
		if err := check(v, v.Variant()); err != nil {
			return fmt.Errorf("variant %s: %w", v.ID, err)
		}
	}
	return nil
}

// ValidateEndgameCatalogPresentation checks the largest possible configured
// owned-item row and snapshot header before publication. Pagination alone cannot
// rescue an oversized single item, material/formation header or combat view.
// All 0/5/6 rows participate because already-owned equipment keeps its version.
func ValidateEndgameCatalogPresentation(items map[ItemID]ItemDef, c *EndgameCatalog) error {
	if c == nil {
		return fmt.Errorf("missing catalog for presentation validation")
	}
	header := EndgameResponse{Kind: "snapshot", OK: true, SnapshotID: strings.Repeat("9", 64), Chunks: 128, CombatStats: &EndgameCombatStatsV5{MagicFind: math.MaxInt64, PhysicalMitigationBP: math.MinInt32}}
	materialIDs := []ItemID{RiftKeyItemID, CubeStarMarrow, CubePurifiedDust, CubeImmortalEcho}
	for _, r := range c.Runes() {
		materialIDs = append(materialIDs, r.Item)
	}
	for _, id := range materialIDs {
		d, ok := items[id]
		if !ok || !catalogTextValid(d.Name) {
			return fmt.Errorf("invalid material presentation %d", id)
		}
		header.Materials = append(header.Materials, EndgameMaterialView{ItemID: int32(id), Name: d.Name, Count: math.MaxInt32})
	}
	// The client limits the materials list independently of byte size.
	if len(header.Materials) > 64 {
		return fmt.Errorf("material presentation exceeds client list limit")
	}
	largestSocket := EndgameSocketView{Index: math.MaxInt32, UID: strings.Repeat("9", 20), ItemID: math.MaxInt32}
	for _, d := range items {
		if d.InstanceKind == ItemInstanceSocketCard {
			if !catalogTextValid(d.Name) {
				return fmt.Errorf("invalid socket card presentation %d", d.ID)
			}
			v := largestSocket
			v.Name = d.Name
			if catalogJSONSize(v) > catalogJSONSize(largestSocket) {
				largestSocket = v
			}
		}
	}
	formations := map[AstralChannel]EndgameFormationView{}
	longestTrait := ""
	largestTraitUnits := 0
	procViews := map[string]EquipmentProcReadinessView{}
	auraViews := map[string]EquipmentAuraView{}
	effects := func(es []EquipmentEffect) {
		for _, e := range es {
			if p := e.Proc; p != nil {
				v := EquipmentProcReadinessView{EffectID: p.ID, SourceUID: math.MinInt64, Trigger: p.Trigger, SkillID: p.SkillID, FixedRank: p.FixedRank, ChanceBP: p.ChanceBP, ICDMS: p.ICDMS, RemainingMS: math.MaxInt64, GlobalRemainingMS: math.MaxInt64}
				if catalogJSONSize(v) > catalogJSONSize(procViews[p.ID]) {
					procViews[p.ID] = v
				}
			}
			if a := e.Aura; a != nil {
				v := EquipmentAuraView{AuraID: a.ID, Family: a.Family, SourceEntityID: math.MaxUint32, SourceUID: math.MinInt64, Rank: a.FixedRank, AssetKey: a.VisualKey, Modifier: a.Modifier, Element: a.Element, Value: a.Value, LeaseMS: a.LeaseMS}
				if catalogJSONSize(v) > catalogJSONSize(auraViews[a.Family]) {
					auraViews[a.Family] = v
				}
			}
		}
	}
	for _, defs := range c.formations {
		for _, r := range defs {
			v := catalogFormation(r)
			if catalogJSONSize(v) > catalogJSONSize(formations[r.Channel]) {
				formations[r.Channel] = v
			}
			trait := catalogTraits(r.Traits)
			largestTraitUnits = max(largestTraitUnits, len(utf16.Encode([]rune(trait))))
			if len(trait) > len(longestTrait) {
				longestTrait = trait
			}
			effects(r.TypedEffects)
		}
	}
	for _, defs := range c.uniques {
		for _, u := range defs {
			effects(u.TypedEffects)
			trait := catalogTraits(u.Traits)
			if len(utf16.Encode([]rune(trait)))+largestTraitUnits+1 > endgameClientStringUnits {
				return fmt.Errorf("unique %d combined formation trait exceeds client limit", u.ID)
			}
			if longestTrait != "" {
				if trait != "" {
					trait += "；"
				}
				trait += longestTrait
			}
			if !catalogTextValid(trait) {
				return fmt.Errorf("unique %d combined formation trait exceeds client limit", u.ID)
			}
			v := EndgameItemView{UID: strings.Repeat("9", 20), ItemID: int32(u.BaseID), UniqueID: u.ID, Name: u.Name, Description: u.Description(), Trait: trait, Stats: catalogStats(u.Affixes), Revision: strings.Repeat("9", 20), Location: "equipped", Level: math.MaxInt32, Profession: 31, Slot: 25, UniqueRollBP: UniqueRollMaxBP, FormationID: math.MaxInt32, InactiveReason: "装备状态不符合要求", SocketCount: int32(len((Stack{}).Sockets))}
			for i := 0; i < len((Stack{}).Sockets); i++ {
				v.Sockets = append(v.Sockets, largestSocket)
			}
			if catalogJSONSize(v) > endgameCatalogRowBytes {
				return fmt.Errorf("unique %d owned row exceeds transport frame budget", u.ID)
			}
		}
	}
	for _, v := range formations {
		header.Formations = append(header.Formations, v)
	}
	for _, v := range auraViews {
		header.CombatStats.EquipmentAuras = append(header.CombatStats.EquipmentAuras, v)
	}
	if len(auraViews) > 128 {
		return fmt.Errorf("configured aura families exceed client list limit")
	}
	for _, v := range procViews {
		header.CombatStats.ProcReadiness = append(header.CombatStats.ProcReadiness, v)
	}
	sort.Slice(header.CombatStats.ProcReadiness, func(i, j int) bool {
		return catalogJSONSize(header.CombatStats.ProcReadiness[i]) > catalogJSONSize(header.CombatStats.ProcReadiness[j])
	})
	if len(header.CombatStats.ProcReadiness) > MaxEquipmentProcReadiness {
		header.CombatStats.ProcReadiness = header.CombatStats.ProcReadiness[:MaxEquipmentProcReadiness]
	}
	for _, element := range []string{"fire", "ice", "holy", "dark"} {
		header.CombatStats.Elements = append(header.CombatStats.Elements, EndgameElementStatsV5{Element: element, DamageBP: math.MinInt32, ResistBP: math.MinInt32, PierceBP: math.MinInt32, MitigationBP: math.MinInt32, AbsorbBP: math.MinInt32})
	}
	if catalogJSONSize(header) > endgameCatalogRowBytes {
		return fmt.Errorf("configured snapshot header exceeds transport frame budget")
	}
	return nil
}
