package domain

import (
	"fmt"
	"sort"
)

// EndgameCatalog is an immutable startup snapshot. Its maps are private and every
// returned slice-bearing definition is copied. No process-global catalog exists.
type EndgameCatalog struct {
	skills     *SkillProgressionCatalog
	uniques    map[int32]map[ItemID]UniqueDefinition
	formations map[int32][]AstralRecipe
	runes      map[ItemID]int
	runeRows   []AstralRuneDef
	variants   map[string]EquipmentSkillVariantRule
}
type VersionedUniqueDefinition struct {
	Version    int32
	Definition UniqueDefinition
}
type VersionedAstralRecipe struct {
	Version    int32
	Definition AstralRecipe
}
type EndgameCatalogManifest struct {
	Kind    string
	Version int32
	Count   int
}
type EndgameCatalogData struct {
	Skills     *SkillProgressionCatalog `json:"-"`
	Manifest   []EndgameCatalogManifest
	Uniques    []VersionedUniqueDefinition
	Formations []VersionedAstralRecipe
	Runes      []AstralRuneDef
	Variants   []EquipmentSkillVariantRule
}

func NewEndgameCatalog(in EndgameCatalogData) (*EndgameCatalog, error) {
	if in.Skills == nil {
		return nil, fmt.Errorf("endgame catalog requires skill progression catalog")
	}
	c := &EndgameCatalog{skills: in.Skills, uniques: map[int32]map[ItemID]UniqueDefinition{}, formations: map[int32][]AstralRecipe{}, runes: map[ItemID]int{}, variants: map[string]EquipmentSkillVariantRule{}}
	for _, v := range in.Variants {
		if err := v.Validate(); err != nil {
			return nil, err
		}
		if _, ok := c.variants[v.ID]; ok {
			return nil, fmt.Errorf("duplicate variant %s", v.ID)
		}
		c.variants[v.ID] = v
	}
	for _, r := range in.Runes {
		if _, ok := c.runes[r.Item]; ok {
			return nil, fmt.Errorf("duplicate rune %d", r.Item)
		}
		if r.RegenerationIntervalMS != 0 && (r.RegenerationIntervalMS < 100 || r.RegenerationIntervalMS > 60000 || r.RegenerationIntervalMS%int32(TickMS) != 0) {
			return nil, fmt.Errorf("unsupported rune regeneration interval %d", r.Item)
		}
		c.runes[r.Item] = len(c.runeRows)
		c.runeRows = append(c.runeRows, cloneAstralRune(r))
	}
	for _, v := range in.Uniques {
		if v.Version != 0 && v.Version != 5 && v.Version != UniqueContentVersion {
			return nil, fmt.Errorf("unsupported unique version %d", v.Version)
		}
		if c.uniques[v.Version] == nil {
			c.uniques[v.Version] = map[ItemID]UniqueDefinition{}
		}
		if _, ok := c.uniques[v.Version][v.Definition.BaseID]; ok {
			return nil, fmt.Errorf("duplicate unique %d/%d", v.Version, v.Definition.BaseID)
		}
		if err := validateUniqueDefinition(v.Definition, c); err != nil {
			return nil, err
		}
		c.uniques[v.Version][v.Definition.BaseID] = cloneUnique(v.Definition)
	}
	for _, v := range in.Formations {
		if v.Version != 0 && v.Version != 5 && v.Version != UniqueContentVersion {
			return nil, fmt.Errorf("unsupported formation version %d", v.Version)
		}
		c.formations[v.Version] = append(c.formations[v.Version], cloneAstralRecipe(v.Definition))
	}
	for version, recipes := range c.formations {
		sort.Slice(recipes, func(i, j int) bool { return recipes[i].ID < recipes[j].ID })
		for i, r := range recipes {
			if r.ID <= 0 || i > 0 && r.ID == recipes[i-1].ID {
				return nil, fmt.Errorf("formation version %d duplicate identity %d", version, r.ID)
			}
		}
	}
	if len(in.Manifest) == 0 {
		return nil, fmt.Errorf("missing endgame catalog manifest")
	}
	seen := map[string]bool{}
	for _, m := range in.Manifest {
		key := fmt.Sprintf("%s/%d", m.Kind, m.Version)
		if seen[key] || m.Count <= 0 {
			return nil, fmt.Errorf("invalid catalog manifest %s", key)
		}
		if (m.Kind == "rune" || m.Kind == "variant") && m.Version != 0 {
			return nil, fmt.Errorf("unsupported manifest %s", key)
		}
		if (m.Kind == "unique" || m.Kind == "formation") && m.Version != 0 && m.Version != 5 && m.Version != UniqueContentVersion {
			return nil, fmt.Errorf("unsupported manifest %s", key)
		}
		seen[key] = true
		actual := 0
		switch m.Kind {
		case "unique":
			actual = len(c.uniques[m.Version])
		case "formation":
			actual = len(c.formations[m.Version])
		case "rune":
			actual = len(c.runes)
		case "variant":
			actual = len(c.variants)
		default:
			return nil, fmt.Errorf("unknown manifest %s", key)
		}
		if actual != m.Count {
			return nil, fmt.Errorf("catalog %s got %d want %d", key, actual, m.Count)
		}
	}
	for _, v := range []int32{0, 5, UniqueContentVersion} {
		for _, kind := range []string{"unique", "formation"} {
			if !seen[fmt.Sprintf("%s/%d", kind, v)] {
				return nil, fmt.Errorf("missing manifest %s/%d", kind, v)
			}
		}
	}
	if !seen["rune/0"] || !seen["variant/0"] {
		return nil, fmt.Errorf("missing rune or variant manifest")
	}
	for v := range c.formations {
		if err := validateAstralCatalogVersion(c, v); err != nil {
			return nil, fmt.Errorf("formation version %d: %w", v, err)
		}
	}
	if err := c.validatePresentationRows(); err != nil {
		return nil, err
	}
	return c, nil
}
func (c *EndgameCatalog) Unique(version int32, id ItemID) (UniqueDefinition, bool) {
	if c == nil {
		return UniqueDefinition{}, false
	}
	d, ok := c.uniques[version][id]
	return cloneUnique(d), ok
}
func (c *EndgameCatalog) LegacyUniqueForBase(id ItemID) (UniqueDefinition, bool) {
	return c.Unique(0, id)
}
func (c *EndgameCatalog) Uniques(version int32) []UniqueDefinition {
	if c == nil {
		return nil
	}
	out := make([]UniqueDefinition, 0, len(c.uniques[version]))
	for _, d := range c.uniques[version] {
		out = append(out, cloneUnique(d))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].BaseID < out[j].BaseID })
	return out
}
func (c *EndgameCatalog) Recipes(version int32) []AstralRecipe {
	if c == nil {
		return nil
	}
	out := make([]AstralRecipe, len(c.formations[version]))
	for i, d := range c.formations[version] {
		out[i] = cloneAstralRecipe(d)
	}
	return out
}
func (c *EndgameCatalog) Rune(id ItemID) (AstralRuneDef, bool) {
	if c == nil {
		return AstralRuneDef{}, false
	}
	i, ok := c.runes[id]
	if !ok {
		return AstralRuneDef{}, false
	}
	return cloneAstralRune(c.runeRows[i]), true
}
func (c *EndgameCatalog) Runes() []AstralRuneDef {
	if c == nil {
		return nil
	}
	out := make([]AstralRuneDef, 0, len(c.runes))
	for _, d := range c.runeRows {
		out = append(out, cloneAstralRune(d))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Item < out[j].Item })
	return out
}
func (c *EndgameCatalog) Variant(id string) (EquipmentSkillVariant, bool) {
	if c == nil {
		return EquipmentSkillVariant{}, false
	}
	d, ok := c.variants[id]
	return d.Variant(), ok
}
func AttachEndgameCatalog(items map[ItemID]ItemDef, c *EndgameCatalog) {
	for id, d := range items {
		d.EndgameCatalog = c
		items[id] = d
	}
}
func EndgameCatalogOf(items map[ItemID]ItemDef) *EndgameCatalog {
	for _, d := range items {
		if d.EndgameCatalog != nil {
			return d.EndgameCatalog
		}
	}
	return nil
}
