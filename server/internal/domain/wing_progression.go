package domain

import (
	"encoding/json"
	"fmt"
	"sort"
)

const WingGuideQuestID QuestID = 1900030002
const WingPowerRecipeID int32 = 900000

// Explicit native identities/edges preserve appearance variants. AscensionOnly
// remains an equipment wear rule, never an extra crafting gate.
type WingItemRule struct {
	Item     ItemID
	Tier     int32
	Series   string
	Variant  string
	Ascended bool
}
type WingCubeRecipeDef struct {
	ID             int32
	Source, Target ItemID
	Operation      string
	Materials      []CubeMaterialDef
	Outputs        []CubeMaterialDef
	ProfessionMask uint8
}
type WingProgressionConfig struct {
	Version    int32
	Entrance   EndgameRiftEntranceView
	Quest      QuestID
	Starter    ItemID
	Items      []WingItemRule
	FeatherIDs []ItemID
	Recipes    []WingCubeRecipeDef
}
type WingProgressionRules struct {
	config  WingProgressionConfig
	items   map[ItemID]WingItemRule
	recipes map[int32]WingCubeRecipeDef
	ids     []int32
}

func NewWingProgressionRules(c WingProgressionConfig, items map[ItemID]ItemDef) (*WingProgressionRules, error) {
	if c.Version < 1 || c.Quest != WingGuideQuestID || c.Starter <= 0 || c.Entrance.MapID <= 0 || c.Entrance.NPCID <= 0 || c.Entrance.NPC == "" || c.Entrance.Script == "" || len(c.Items) < 2 || len(c.Items) > 128 || len(c.Recipes) > 1024 {
		return nil, fmt.Errorf("invalid wing progression identity")
	}
	r := &WingProgressionRules{items: map[ItemID]WingItemRule{}, recipes: map[int32]WingCubeRecipeDef{}}
	feathers := map[ItemID]bool{}
	for _, id := range c.FeatherIDs {
		d, ok := items[id]
		if !ok || !d.Stackable || d.InstanceKind != ItemInstanceNone || !d.InventoryTabKnown || d.Equip != nil || feathers[id] {
			return nil, fmt.Errorf("invalid wing feather %d", id)
		}
		feathers[id] = true
	}
	for _, w := range c.Items {
		d, ok := items[w.Item]
		if !ok || d.Equip == nil || d.Equip.Slot != int32(SlotBag) || d.InstanceKind != ItemInstanceEquipment || !d.InventoryTabKnown || w.Tier < 1 || w.Tier > 8 || w.Series == "" || r.items[w.Item].Item != 0 || w.Ascended != d.Equip.AscensionOnly {
			return nil, fmt.Errorf("invalid native wing %d", w.Item)
		}
		r.items[w.Item] = w
	}
	starter, ok := r.items[c.Starter]
	if !ok || starter.Tier != 1 || starter.Ascended {
		return nil, fmt.Errorf("invalid starter wing")
	}
	edges := map[[2]ItemID]bool{}
	for _, v := range c.Recipes {
		if v.ID <= WingPowerRecipeID || v.ID > 901024 || r.recipes[v.ID].ID != 0 || (v.Operation != "wing.feather" && (v.Source == v.Target || edges[[2]ItemID{v.Source, v.Target}])) || len(v.Materials) == 0 {
			return nil, fmt.Errorf("invalid wing recipe identity %d", v.ID)
		}
		from, fok := r.items[v.Source]
		to, tok := r.items[v.Target]
		if v.Operation != "wing.feather" && (!fok || !tok) {
			return nil, fmt.Errorf("unknown wing edge")
		}
		switch v.Operation {
		case "wing.feather":
			if len(v.Materials) != 1 || len(v.Outputs) != 1 {
				return nil, fmt.Errorf("invalid feather exchange shape")
			}
			a, b := v.Materials[0], v.Outputs[0]
			valid := (a.ItemID == 25027 && a.Count == 5 && b.ItemID == 25028 && b.Count == 1) || (a.ItemID == 25028 && a.Count == 1 && b.ItemID == 25027 && b.Count == 5) || (a.ItemID == 25028 && a.Count == 5 && b.ItemID == 25029 && b.Count == 1) || (a.ItemID == 25029 && a.Count == 1 && b.ItemID == 25028 && b.Count == 5)
			if !valid {
				return nil, fmt.Errorf("feather exchange must conserve approved common denominations")
			}
			if v.Source != 0 || v.Target != 0 || v.ProfessionMask != 0 || len(v.Materials) != 1 || len(v.Outputs) != 1 || !feathers[v.Outputs[0].ItemID] || v.Outputs[0].Count < 1 || v.Outputs[0].Count > RiftWingMaxRecipeCount || v.Outputs[0].ItemID == v.Materials[0].ItemID {
				return nil, fmt.Errorf("invalid feather exchange")
			}
		case "wing.upgrade":
			if to.Tier != from.Tier+1 || to.Series != from.Series {
				return nil, fmt.Errorf("invalid wing upgrade edge")
			}
		case "wing.change":
			if to.Tier != from.Tier || to.Series == from.Series || to.Variant != from.Variant || to.Ascended != from.Ascended {
				return nil, fmt.Errorf("invalid wing conversion edge")
			}
		default:
			return nil, fmt.Errorf("invalid wing operation")
		}
		if v.ProfessionMask > 31 || (v.Operation != "wing.feather" && len(v.Outputs) > 0) {
			return nil, fmt.Errorf("invalid wing recipe fields")
		}
		seen := map[ItemID]bool{}
		for _, m := range v.Materials {
			if !feathers[m.ItemID] || m.Count < 1 || m.Count > RiftWingMaxRecipeCount || seen[m.ItemID] {
				return nil, fmt.Errorf("invalid wing recipe material")
			}
			seen[m.ItemID] = true
		}
		// Identical ingredients may only branch on disjoint employed professions.
		// Never resolve conflicting authored recipes by map/order or random choice.
		for _, prior := range r.recipes {
			if prior.Source != v.Source || len(prior.Materials) != len(v.Materials) {
				continue
			}
			same := true
			for _, m := range v.Materials {
				found := false
				for _, n := range prior.Materials {
					if m == n {
						found = true
						break
					}
				}
				same = same && found
			}
			if same && (prior.ProfessionMask == 0 || v.ProfessionMask == 0 || prior.ProfessionMask&v.ProfessionMask != 0) {
				return nil, fmt.Errorf("ambiguous wing cube ingredients %d/%d", prior.ID, v.ID)
			}
		}
		edges[[2]ItemID{v.Source, v.Target}] = true
		v.Materials = append([]CubeMaterialDef(nil), v.Materials...)
		v.Outputs = append([]CubeMaterialDef(nil), v.Outputs...)
		r.recipes[v.ID] = v
		r.ids = append(r.ids, v.ID)
	}
	sort.Slice(r.ids, func(i, j int) bool { return r.ids[i] < r.ids[j] })
	raw, _ := json.Marshal(c)
	_ = json.Unmarshal(raw, &r.config)
	return r, nil
}

const RiftWingMaxRecipeCount int32 = 1000000

func (r *WingProgressionRules) Config() WingProgressionConfig {
	if r == nil {
		return WingProgressionConfig{}
	}
	raw, _ := json.Marshal(r.config)
	var c WingProgressionConfig
	_ = json.Unmarshal(raw, &c)
	return c
}
func (r *WingProgressionRules) Item(id ItemID) (WingItemRule, bool) {
	if r == nil {
		return WingItemRule{}, false
	}
	w, ok := r.items[id]
	return w, ok
}
func (r *WingProgressionRules) Recipe(id int32) (WingCubeRecipeDef, bool) {
	if r == nil {
		return WingCubeRecipeDef{}, false
	}
	v, ok := r.recipes[id]
	v.Materials = append([]CubeMaterialDef(nil), v.Materials...)
	v.Outputs = append([]CubeMaterialDef(nil), v.Outputs...)
	return v, ok
}
func (r *WingProgressionRules) Recipes() []WingCubeRecipeDef {
	if r == nil {
		return nil
	}
	var out []WingCubeRecipeDef
	for _, id := range r.ids {
		v, _ := r.Recipe(id)
		out = append(out, v)
	}
	return out
}
