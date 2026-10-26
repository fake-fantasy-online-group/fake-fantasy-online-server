package domain

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
)

// CubeRuleTable is an immutable startup snapshot. Accessors return deep copies.
// No built-in recipes are substituted when database configuration is absent.
type CubeRuleTable struct {
	recipes map[int32]CubeRecipeDef
	ids     []int32
}
type CubeMaterialDef struct {
	ItemID ItemID
	Count  int32
}
type CubeOutputDef struct {
	ItemID                                    ItemID
	Count, LevelDivisor, LevelCap, Multiplier int32
}
type CubeRecipeDef struct {
	ID                           int32
	Name, Operation, Description string
	TargetRequired               bool
	Money                        int64
	SourceCount, QualityStepBP   int32
	Materials                    []CubeMaterialDef
	Outputs                      []CubeOutputDef
}

func cloneCubeRecipe(r CubeRecipeDef) CubeRecipeDef {
	r.Materials = append([]CubeMaterialDef(nil), r.Materials...)
	r.Outputs = append([]CubeOutputDef(nil), r.Outputs...)
	return r
}
func (t *CubeRuleTable) Recipe(id int32) (CubeRecipeDef, bool) {
	if t == nil {
		return CubeRecipeDef{}, false
	}
	r, ok := t.recipes[id]
	return cloneCubeRecipe(r), ok
}
func (t *CubeRuleTable) Recipes() []CubeRecipeDef {
	if t == nil {
		return nil
	}
	out := make([]CubeRecipeDef, 0, len(t.ids))
	for _, id := range t.ids {
		out = append(out, cloneCubeRecipe(t.recipes[id]))
	}
	return out
}
func NewCubeRuleTable(rows []CubeRecipeDef, items map[ItemID]ItemDef) (*CubeRuleTable, error) {
	if len(rows) == 0 || len(rows) > 4096 {
		return nil, fmt.Errorf("missing cube rules")
	}
	t := &CubeRuleTable{recipes: make(map[int32]CubeRecipeDef, len(rows))}
	for _, r := range rows {
		if _, exists := t.recipes[r.ID]; exists {
			return nil, fmt.Errorf("duplicate cube recipe %d", r.ID)
		}
		if err := validateCubeRecipe(r, items); err != nil {
			return nil, fmt.Errorf("cube recipe %d: %w", r.ID, err)
		}
		t.recipes[r.ID] = cloneCubeRecipe(r)
		t.ids = append(t.ids, r.ID)
	}
	sort.Slice(t.ids, func(i, j int) bool { return t.ids[i] < t.ids[j] })
	// Any accepted cursor may start a 32-row page. Verify the actual JSON shape
	// with bounded worst-case response metadata, rather than per-field bytes.
	views := CubeRecipes(t, items)
	for start := 0; start < len(views); start++ {
		end := min(start+32, len(views))
		v := EndgameResponse{Kind: "catalog.recipes", RequestID: strings.Repeat("x", 64), OK: true, CatalogVersion: strings.Repeat("x", 128), Cursor: strings.Repeat("9", 64), NextCursor: strings.Repeat("9", 64), Recipes: views[start:end]}
		raw, err := json.Marshal(v)
		if err != nil || len(raw) > EndgameMaxJSONBytes {
			return nil, fmt.Errorf("cube catalog page exceeds protocol limit at %d", start)
		}
	}
	return t, nil
}
func validateCubeRecipe(r CubeRecipeDef, items map[ItemID]ItemDef) error {
	if r.ID <= 0 || r.Name == "" || r.Description == "" || r.Money < 0 || cubeTextUnits(r.Name) > 8192 || cubeTextUnits(r.Description) > 8192 || cubeTextUnits(r.Operation) > 64 || len(r.Materials) > 64 {
		return fmt.Errorf("invalid identity or fee")
	}
	suffix := "完成。"
	if r.Operation == "socket.open" {
		suffix = "完成：开出5孔。"
	}
	if cubeTextUnits(r.Name)+cubeTextUnits(suffix) > 8192 {
		return fmt.Errorf("cube completion message exceeds client limit")
	}
	target := true
	switch r.Operation {
	case "salvage.normal", "salvage.unique", "socket.open", "socket.extract", "socket.reorder", "socket.array", "unique.awaken", "unique.reforge", "unique.temper":
	case "salvage.rune", "rune.inscribe", "rune.convert":
		target = false
	default:
		return fmt.Errorf("unknown operation %q", r.Operation)
	}
	if r.TargetRequired != target {
		return fmt.Errorf("target contract differs from operation")
	}
	if r.Operation == "rune.convert" {
		if r.SourceCount < 1 || r.SourceCount > 5 {
			return fmt.Errorf("invalid source count")
		}
	} else if r.SourceCount != 0 {
		return fmt.Errorf("unexpected source count")
	}
	if r.Operation == "unique.temper" {
		if r.QualityStepBP < 1 || r.QualityStepBP > UniqueRollMaxBP-UniqueRollMinBP {
			return fmt.Errorf("invalid quality step")
		}
	} else if r.QualityStepBP != 0 {
		return fmt.Errorf("unexpected quality step")
	}
	seen := map[ItemID]bool{}
	for _, m := range r.Materials {
		d, ok := items[m.ItemID]
		if !ok || cubeTextUnits(d.Name) > 8192 || !d.InventoryTabKnown || !d.Stackable || d.InstanceKind != ItemInstanceNone || m.Count <= 0 || seen[m.ItemID] {
			return fmt.Errorf("invalid material %d", m.ItemID)
		}
		seen[m.ItemID] = true
	}
	runeOutput := r.Operation == "rune.inscribe" || r.Operation == "rune.convert"
	salvage := r.Operation == "salvage.normal" || r.Operation == "salvage.unique" || r.Operation == "salvage.rune"
	if runeOutput && len(r.Outputs) != 1 || salvage && len(r.Outputs) == 0 || !runeOutput && !salvage && len(r.Outputs) != 0 {
		return fmt.Errorf("invalid output count")
	}
	seen = map[ItemID]bool{}
	for _, o := range r.Outputs {
		d, ok := items[o.ItemID]
		if !ok || !d.InventoryTabKnown || o.Count <= 0 || o.Multiplier <= 0 || o.LevelDivisor < 0 || o.LevelCap < 0 || seen[o.ItemID] {
			return fmt.Errorf("invalid output %d", o.ItemID)
		}
		seen[o.ItemID] = true
		if runeOutput {
			if d.InstanceKind != ItemInstanceSocketCard || !d.Stackable || !d.CardDefaults.Initialized || o.ItemID < FirstAstralRune || o.ItemID > LastAstralRune || o.Count != 1 || o.Multiplier != 1 || o.LevelDivisor != 0 || o.LevelCap != 0 {
				return fmt.Errorf("invalid rune output %d", o.ItemID)
			}
		} else if !d.Stackable || d.InstanceKind != ItemInstanceNone {
			return fmt.Errorf("invalid salvage output %d", o.ItemID)
		}
		if o.LevelDivisor == 0 && o.LevelCap != 0 || o.LevelDivisor > 0 && (o.LevelCap == 0 || r.Operation == "salvage.rune") {
			return fmt.Errorf("invalid level-dependent yield")
		}
		if _, err := o.Quantity(o.LevelCap); err != nil {
			return err
		}
	}
	return nil
}
func (o CubeOutputDef) Quantity(level int32) (int32, error) {
	n := int64(o.Count)
	if o.LevelDivisor > 0 {
		n += int64(max(int32(0), min(o.LevelCap, level)) / o.LevelDivisor)
	}
	n *= int64(o.Multiplier)
	if n < 1 || n > math.MaxInt32 {
		return 0, fmt.Errorf("cube output quantity out of range")
	}
	return int32(n), nil
}
func cubeRecipeView(r CubeRecipeDef, items map[ItemID]ItemDef) EndgameRecipeView {
	v := EndgameRecipeView{ID: r.ID, Name: r.Name, Operation: r.Operation, Description: r.Description, TargetRequired: r.TargetRequired, Money: strconv.FormatInt(r.Money, 10)}
	for _, m := range r.Materials {
		v.Materials = append(v.Materials, EndgameMaterialView{ItemID: int32(m.ItemID), Name: items[m.ItemID].Name, Count: m.Count})
	}
	if r.Operation == "rune.inscribe" || r.Operation == "rune.convert" {
		v.OutputItemID = int32(r.Outputs[0].ItemID)
	}
	return v
}
func CubeRecipes(rules *CubeRuleTable, items map[ItemID]ItemDef) []EndgameRecipeView {
	out := make([]EndgameRecipeView, 0)
	for _, r := range rules.Recipes() {
		out = append(out, cubeRecipeView(r, items))
	}
	return out
}

func cubeTextUnits(s string) int { return len(utf16.Encode([]rune(s))) }
