// Package showcase constructs the fixed, offline-reviewed showcase grant.
// It does not equip items, mutate characters, persist data, or contact a server.
package showcase

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

type selection struct {
	Slot   domain.EquipSlot
	Bases  []domain.ItemID // For clothing: male first, female second.
	Recipe int32
	Level  int32
	Runes  []domain.ItemID
}
type target struct {
	ID    int64
	Name  string
	Race  domain.Race
	Items []selection
}

// Deliberately compiled rather than loaded from a mutable operator-supplied plan.
var targets = []target{
	{ID: 42, Name: "春霞姐姐", Race: domain.Assassin, Items: []selection{
		{Slot: 1, Bases: []domain.ItemID{2707}, Recipe: 38, Level: 60, Runes: []domain.ItemID{1900028007, 1900028015, 1900028016, 1900028027, 1900028028}},
		{Slot: 2, Bases: []domain.ItemID{2049}, Recipe: 0, Level: 52, Runes: []domain.ItemID{}},
		{Slot: 3, Bases: []domain.ItemID{2509}, Recipe: 0, Level: 56, Runes: []domain.ItemID{}},
		{Slot: 6, Bases: []domain.ItemID{2410}, Recipe: 0, Level: 60, Runes: []domain.ItemID{}},
		{Slot: 7, Bases: []domain.ItemID{2590}, Recipe: 0, Level: 60, Runes: []domain.ItemID{}},
		{Slot: 8, Bases: []domain.ItemID{2895, 2896}, Recipe: 36, Level: 48, Runes: []domain.ItemID{1900028001, 1900028004, 1900028003, 1900028009, 1900028014}},
		{Slot: 9, Bases: []domain.ItemID{2108}, Recipe: 0, Level: 56, Runes: []domain.ItemID{}},
		{Slot: 10, Bases: []domain.ItemID{2604}, Recipe: 0, Level: 24, Runes: []domain.ItemID{}},
		{Slot: 13, Bases: []domain.ItemID{1810}, Recipe: 0, Level: 54, Runes: []domain.ItemID{}},
	}},
	{ID: 183, Name: "术士姐姐", Race: domain.Warlock, Items: []selection{
		{Slot: 1, Bases: []domain.ItemID{2707}, Recipe: 28, Level: 58, Runes: []domain.ItemID{1900028016, 1900028017, 1900028026, 1900028027}},
		{Slot: 2, Bases: []domain.ItemID{2059}, Recipe: 0, Level: 59, Runes: []domain.ItemID{}},
		{Slot: 3, Bases: []domain.ItemID{2506}, Recipe: 0, Level: 38, Runes: []domain.ItemID{}},
		{Slot: 6, Bases: []domain.ItemID{2409}, Recipe: 0, Level: 54, Runes: []domain.ItemID{}},
		{Slot: 7, Bases: []domain.ItemID{2558}, Recipe: 0, Level: 48, Runes: []domain.ItemID{}},
		{Slot: 8, Bases: []domain.ItemID{2829, 2830}, Recipe: 36, Level: 48, Runes: []domain.ItemID{1900028001, 1900028004, 1900028003, 1900028009, 1900028014}},
		{Slot: 9, Bases: []domain.ItemID{2108}, Recipe: 0, Level: 56, Runes: []domain.ItemID{}},
		{Slot: 10, Bases: []domain.ItemID{2602}, Recipe: 0, Level: 36, Runes: []domain.ItemID{}},
		{Slot: 13, Bases: []domain.ItemID{1310}, Recipe: 0, Level: 54, Runes: []domain.ItemID{}},
	}},
	{ID: 206, Name: "骚骚霞", Race: domain.Healer, Items: []selection{
		{Slot: 1, Bases: []domain.ItemID{2707}, Recipe: 35, Level: 60, Runes: []domain.ItemID{1900028010, 1900028011, 1900028013, 1900028014, 1900028028}},
		{Slot: 2, Bases: []domain.ItemID{2056}, Recipe: 0, Level: 59, Runes: []domain.ItemID{}},
		{Slot: 3, Bases: []domain.ItemID{2509}, Recipe: 0, Level: 56, Runes: []domain.ItemID{}},
		{Slot: 6, Bases: []domain.ItemID{2409}, Recipe: 0, Level: 54, Runes: []domain.ItemID{}},
		{Slot: 7, Bases: []domain.ItemID{2559}, Recipe: 0, Level: 54, Runes: []domain.ItemID{}},
		{Slot: 8, Bases: []domain.ItemID{2899, 2900}, Recipe: 36, Level: 48, Runes: []domain.ItemID{1900028001, 1900028004, 1900028003, 1900028009, 1900028014}},
		{Slot: 9, Bases: []domain.ItemID{2108}, Recipe: 0, Level: 56, Runes: []domain.ItemID{}},
		{Slot: 10, Bases: []domain.ItemID{2601}, Recipe: 0, Level: 30, Runes: []domain.ItemID{}},
		{Slot: 13, Bases: []domain.ItemID{1310}, Recipe: 40, Level: 60, Runes: []domain.ItemID{1900028001, 1900028008, 1900028015, 1900028022, 1900028028}},
	}},
}

// Reviewed V6 plan: counts are precise approval and UID-collision invariants.
const EquipmentCount = 27
const EmbeddedRuneCount = 34
const TotalInstanceCount = EquipmentCount + EmbeddedRuneCount

// ValidateTarget checks identity, actual employment, level and recorded gender.
// Account ownership must additionally be checked by the transactional caller.
func ValidateTarget(ch *domain.Character) error {
	if ch == nil {
		return fmt.Errorf("showcase: nil character")
	}
	for _, t := range targets {
		if t.ID != ch.ID {
			continue
		}
		if ch.Name != t.Name || ch.Race != t.Race || ch.Level != 60 || !ch.EmploymentKnown || !ch.Employed || ch.Appear.Gender > 1 {
			return fmt.Errorf("showcase: character %d identity/class/level/employment/gender mismatch", ch.ID)
		}
		return nil
	}
	return fmt.Errorf("showcase: character %d is not an approved target", ch.ID)
}

// Build returns nine independent new equipment instances. Six-dimensional stat
// requirements are intentionally left intact and are not an eligibility gate.
func Build(ch *domain.Character, defs map[domain.ItemID]domain.ItemDef) ([]domain.Stack, error) {
	if err := ValidateTarget(ch); err != nil {
		return nil, err
	}
	if err := validateCatalog(domain.EndgameCatalogOf(defs)); err != nil {
		return nil, err
	}
	var plan target
	for _, t := range targets {
		if t.ID == ch.ID {
			plan = t
			break
		}
	}
	canonicalRunes := map[domain.ItemID]domain.CardInstanceState{}
	for _, r := range domain.AstralRunes(domain.EndgameCatalogOf(defs)) {
		canonicalRunes[r.Item], _ = defs[r.Item].CardDefaults, true
	}
	out := make([]domain.Stack, 0, 9)
	seen := map[int64]bool{}
	record := func(uid int64) bool {
		if uid <= 0 || seen[uid] {
			return false
		}
		seen[uid] = true
		return true
	}
	worn := domain.NewEquipSet()
	expectedRecipes := map[int32]bool{}
	for _, item := range plan.Items {
		id := item.Bases[0]
		if len(item.Bases) == 2 && ch.Appear.Gender == 0 {
			id = item.Bases[1]
		}
		def, ok := defs[id]
		if !ok || def.ID != id || def.Equip == nil || def.InstanceKind != domain.ItemInstanceEquipment || def.Stackable {
			return nil, fmt.Errorf("showcase: missing/invalid equipment definition %d", id)
		}
		need := def.Equip.Need
		sex := uint8(0)
		if len(item.Bases) == 2 {
			sex = 2
			if ch.Appear.Gender == 1 {
				sex = 1
			}
		}
		if domain.EquipSlot(def.Equip.Slot) != item.Slot || need.Sex != sex || !need.AllowsCharacter(ch) || need.Level > ch.Level || def.Equip.LevelReq > ch.Level {
			return nil, fmt.Errorf("showcase: equipment %d slot/class/level/sex mismatch", id)
		}
		st := domain.NewStack(def, 1)
		if !record(st.UID) || st.Count != 1 || st.MaxDurability <= 0 || st.Durability != st.MaxDurability {
			return nil, fmt.Errorf("showcase: invalid new equipment instance %d", id)
		}
		if item.Recipe == 0 {
			if !domain.ApplyUnique(&st, def, func(n int) int { return n - 1 }) || st.Endgame.UniqueID != int32(id) || st.Endgame.UniqueContentVersion != 6 || st.Endgame.UniqueRollBP != 12000 || st.RolledAffixCount != 0 || st.SocketCount != 0 {
				return nil, fmt.Errorf("showcase: unique construction failed for %d", id)
			}

			unique, ok := domain.UniqueOf(st,domain.EndgameCatalogOf(defs))
			if !ok || unique.Slot != def.Equip.Slot || unique.Professions != need.Professions || unique.BeginnerAllowed != need.BeginnerAllowed || unique.Sex != need.Sex || unique.LevelRequirement != def.Equip.LevelReq || (unique.Professions != 0 && unique.Professions&(1<<uint8(ch.Race)) == 0) {
				return nil, fmt.Errorf("showcase: unique %d catalog/base metadata mismatch", id)
			}
		} else {
			recipe, ok := domain.AstralRecipeByID(item.Recipe,domain.EndgameCatalogOf(defs))
			if !ok || !reflect.DeepEqual(recipe.Runes, item.Runes) || recipe.MinLevel > ch.Level || (recipe.Professions != 0 && recipe.Professions&(1<<uint8(ch.Race)) == 0) || !domain.AstralBaseEligible(def, st) || domain.EquipmentSocketCapacity(def, st) < len(item.Runes) {
				return nil, fmt.Errorf("showcase: formation %d/base %d is not eligible", item.Recipe, id)
			}
			st.Endgame.FormationContentVersion, st.Endgame.FormationRollBP = 6, 12000
			st.SocketCount = uint8(len(item.Runes))
			for i, runeID := range item.Runes {
				rd, ok := defs[runeID]
				if !ok || rd.ID != runeID || rd.InstanceKind != domain.ItemInstanceSocketCard || rd.Equip != nil || rd.Level > ch.Level || rd.UseLevel > ch.Level || rd.CardDefaults != canonicalRunes[runeID] || !reflect.DeepEqual(rd.SocketAffixes, canonicalRunes[runeID].StaticAffixes()) {
					return nil, fmt.Errorf("showcase: missing/invalid rune definition %d", runeID)
				}
				rune := domain.NewStack(rd, 1)
				if !record(rune.UID) || !rune.Card.Initialized || rune.Card != rd.CardDefaults {
					return nil, fmt.Errorf("showcase: invalid rune instance %d", runeID)
				}
				st.Sockets[i], st.SocketUIDs[i], st.SocketCards[i] = runeID, rune.UID, rune.Card
			}
			matched, ok := domain.MatchAstralRecipe(def, st)
			if !ok || matched.ID != item.Recipe {
				return nil, fmt.Errorf("showcase: recipe %d failed production match", item.Recipe)
			}
			expectedRecipes[item.Recipe] = true
		}
		if domain.EquipmentRequiredLevel(def, st) != item.Level || item.Level > ch.Level {
			return nil, fmt.Errorf("showcase: equipment %d required level changed", id)
		}
		worn.Set(item.Slot, st)
		out = append(out, st)
	}
	active := domain.ActiveAstralRecipes(worn, func(id domain.ItemID) (domain.ItemDef, bool) { d, ok := defs[id]; return d, ok }, ch)
	if len(active) != len(expectedRecipes) {
		return nil, fmt.Errorf("showcase: expected exactly the selected non-conflicting formation channels")
	}
	for _, a := range active {
		if !expectedRecipes[a.Recipe.ID] {
			return nil, fmt.Errorf("showcase: unexpected active formation %d", a.Recipe.ID)
		}
	}
	return out, nil
}

func validateCatalog(c *domain.EndgameCatalog) error {
	if domain.UniqueCatalogVersion != "endgame-v6-scope259" || domain.UniqueContentVersion != 6 || len(domain.UniqueCatalog(c)) != 259 || len(domain.AstralRecipes(c)) != 40 || len(domain.AstralRunes(c)) != 28 {
		return fmt.Errorf("showcase: catalog version or coverage changed")
	}
	return nil
}

// PlanDigest binds an operator approval to the fixed selection, both clothing
// variants, relevant loaded item definitions, and the compiled effect catalogs.
// It is deterministic and never allocates item instances.
func PlanDigest(defs map[domain.ItemID]domain.ItemDef) (string, error) {
	if err := validateCatalog(domain.EndgameCatalogOf(defs)); err != nil {
		return "", err
	}
	ids := map[domain.ItemID]bool{}
	for _, t := range targets {
		for _, item := range t.Items {
			for _, id := range item.Bases {
				ids[id] = true
			}
			for _, id := range item.Runes {
				ids[id] = true
			}
		}
	}
	ordered := make([]domain.ItemID, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	relevant := make([]domain.ItemDef, 0, len(ordered))
	for _, id := range ordered {
		d, ok := defs[id]
		if !ok || d.ID != id {
			return "", fmt.Errorf("showcase: missing definition %d", id)
		}
		d.Icon = 0
		relevant = append(relevant, d)
	}
	raw, err := json.Marshal(struct {
		Targets     []target
		Definitions []domain.ItemDef
		Uniques     any
		Recipes     any
		Runes       any
	}{targets, relevant, domain.UniqueCatalog(domain.EndgameCatalogOf(defs)), domain.AstralRecipes(domain.EndgameCatalogOf(defs)), domain.AstralRunes(domain.EndgameCatalogOf(defs))})
	if err != nil {
		return "", fmt.Errorf("showcase: digest: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
