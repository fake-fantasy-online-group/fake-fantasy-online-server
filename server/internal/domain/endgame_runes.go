package domain

import "fmt"

const (
	FirstAstralRune ItemID = 1900028001
	LastAstralRune  ItemID = 1900028028
)

type AstralChannel uint8

const (
	AstralBattle AstralChannel = iota + 1
	AstralGuard
	AstralSupport
)

func (c AstralChannel) String() string {
	switch c {
	case AstralBattle:
		return "战阵"
	case AstralGuard:
		return "守阵"
	case AstralSupport:
		return "辅阵"
	}
	return ""
}

type AstralRuneDef struct {
	RegenerationIntervalMS  int32
	Item                    ItemID
	MinLevel                int32
	Mansion, Name, Quadrant string
	Affixes                 []Affix
}
type AstralRecipe struct {
	ID                                     int32
	Name                                   string
	Channel                                AstralChannel
	MinLevel                               int32
	Professions                            uint8
	Runes                                  []ItemID
	Slots                                  []EquipSlot
	Affixes                                []Affix
	VariableAffixes                        []VariableAffix
	LocalEnhancements                      []ItemLocalEnhancement
	Lore                                   string
	Identity, Benefit, Tradeoff, PowerTier string
	Traits                                 []UniqueTrait
	ReservedSkills                         []ReservedSkillAttribute
	SkillBonuses                           []SkillRankBonus
	TypedEffects                           []EquipmentEffect
}
type AstralActivation struct {
	Slot   EquipSlot
	UID    int64
	Item   ItemID
	Recipe AstralRecipe
}

func cloneAstralRune(r AstralRuneDef) AstralRuneDef {
	r.Affixes = append([]Affix(nil), r.Affixes...)
	return r
}
func cloneAstralRecipe(r AstralRecipe) AstralRecipe {
	r.TypedEffects = CloneEquipmentEffects(r.TypedEffects)
	r.SkillBonuses = append([]SkillRankBonus(nil), r.SkillBonuses...)
	r.VariableAffixes = append([]VariableAffix(nil), r.VariableAffixes...)
	r.LocalEnhancements = append([]ItemLocalEnhancement(nil), r.LocalEnhancements...)
	r.Runes = append([]ItemID(nil), r.Runes...)
	r.Slots = append([]EquipSlot(nil), r.Slots...)
	r.Affixes = append([]Affix(nil), r.Affixes...)
	r.Traits = append([]UniqueTrait(nil), r.Traits...)
	r.ReservedSkills = append([]ReservedSkillAttribute(nil), r.ReservedSkills...)
	return r
}
func AstralRunes(catalog *EndgameCatalog) []AstralRuneDef { return catalog.Runes() }
func AstralRecipes(catalog *EndgameCatalog) []AstralRecipe {
	return catalog.Recipes(UniqueContentVersion)
}
func AstralRune(id ItemID, catalog *EndgameCatalog) (AstralRuneDef, bool) {
	return catalog.Rune(id)
}
func AstralRecipeByID(id int32, catalog *EndgameCatalog) (AstralRecipe, bool) {
	for _, r := range catalog.Recipes(UniqueContentVersion) {
		if r.ID == id {
			return r, true
		}
	}
	return AstralRecipe{}, false
}

// AstralBaseEligible checks the equipment's innate extra properties, not its
// display color. Intrinsic stats and later refinement, washing, fusions and
// socket cards do not turn a plain base into naturally affixed equipment.
// Unique and set gear have their own innate affixes/traits and cannot also form
// an array. Fail closed on leftover rolled-affix data even if its count is zero.
func AstralBaseEligible(def ItemDef, st Stack) bool {
	return AstralBaseIneligibility(def, st) == ""
}

// AstralBaseIneligibility distinguishes confirmed extra properties from source
// metadata that is not decoded well enough to certify a plain base.
func AstralBaseIneligibility(def ItemDef, st Stack) string {
	const hasExtras = "宿阵须使用天生无附加词条的普通装备"
	if def.Equip == nil || NaturalAttributeCount(def, st) != 0 || st.Endgame.UniqueID != 0 {
		return hasExtras
	}
	if suit := def.Equip.Suit; suit != nil && (suit.ID != 0 || len(suit.Groups) != 0 || len(suit.Bonuses) != 0) {
		return hasExtras
	}
	for _, a := range st.RolledAffixes {
		if a != (InstanceAffix{}) {
			return hasExtras
		}
	}
	if def.Equip.UnknownInnateProperties {
		return "这件装备的天生属性尚未确认，不能组成宿阵"
	}
	return ""
}

// Exact length and order are mandatory: an empty extra hole does not activate a
// shorter prefix. Legacy equipment hydrates kind=None; positive UID plus the
// authoritative equipment template remains sufficient for that compatibility.
func MatchAstralRecipe(def ItemDef, st Stack) (AstralRecipe, bool) {
	if st.Empty() || st.UID <= 0 || (st.InstanceKind != ItemInstanceEquipment && st.InstanceKind != ItemInstanceNone) || !AstralBaseEligible(def, st) || st.SocketCount < 2 || st.SocketCount > 5 || int(st.SocketCount) > EquipmentSocketCapacity(def, st) {
		return AstralRecipe{}, false
	}
	seen := map[int64]bool{}
	for i := range st.Sockets {
		if i >= int(st.SocketCount) {
			if st.Sockets[i] != 0 || st.SocketUIDs[i] != 0 {
				return AstralRecipe{}, false
			}
			continue
		}
		if _, ok := AstralRune(st.Sockets[i], def.EndgameCatalog); !ok || st.SocketUIDs[i] <= 0 || !st.SocketCards[i].Initialized || seen[st.SocketUIDs[i]] {
			return AstralRecipe{}, false
		}
		seen[st.SocketUIDs[i]] = true
	}
	catalog := def.EndgameCatalog.Recipes(st.Endgame.FormationContentVersion)
	for _, r := range catalog {
		if len(r.Runes) != int(st.SocketCount) {
			continue
		}
		slotOK := false
		for _, slot := range r.Slots {
			if int32(slot) == def.Equip.Slot {
				slotOK = true
				break
			}
		}
		if !slotOK {
			continue
		}
		equal := true
		for i, id := range r.Runes {
			if st.Sockets[i] != id {
				equal = false
				break
			}
		}
		if equal {
			return cloneAstralRecipe(r), true
		}
	}
	return AstralRecipe{}, false
}

// One winner per channel: most holes, then lower recipe ID, then lower slot.
// Standalone rune affixes remain active even when their formation is suppressed.
func ActiveAstralRecipes(worn *EquipSet, defs func(ItemID) (ItemDef, bool), ch *Character) []AstralActivation {
	if worn == nil || defs == nil || ch == nil {
		return nil
	}
	var chosen [4]AstralActivation
	worn.Each(func(slot EquipSlot, st Stack) {
		def, ok := defs(st.Item)
		if !ok || def.Equip == nil || int32(slot) != def.Equip.Slot || !def.Equip.Need.AllowsCharacter(ch) || ch.Level < EquipmentRequiredLevel(def, st) || !EquipmentFunctional(st, def) {
			return
		}
		r, ok := MatchAstralRecipe(def, st)
		if !ok || ch.Level < r.MinLevel {
			return
		}
		if r.Professions != 0 && (!ch.HasProfession() || !ch.Race.Valid() || r.Professions&(1<<uint8(ch.Race)) == 0) {
			return
		}
		old := chosen[r.Channel]
		if old.Recipe.ID != 0 && (len(old.Recipe.Runes) > len(r.Runes) || len(old.Recipe.Runes) == len(r.Runes) && (old.Recipe.ID < r.ID || old.Recipe.ID == r.ID && old.Slot <= slot)) {
			return
		}
		chosen[r.Channel] = AstralActivation{Slot: slot, UID: st.UID, Item: st.Item, Recipe: r}
	})
	var out []AstralActivation
	for c := AstralBattle; c <= AstralSupport; c++ {
		if chosen[c].Recipe.ID != 0 {
			out = append(out, chosen[c])
		}
	}
	return out
}
func AstralBonuses(worn *EquipSet, defs func(ItemID) (ItemDef, bool), ch *Character) []Affix {
	var out []Affix
	for _, a := range ActiveAstralRecipes(worn, defs, ch) {
		def, ok := defs(a.Item)
		if !ok {
			continue
		}
		out = append(out, AstralItemBonuses(def, worn.At(a.Slot), a.Recipe)...)
	}
	return out
}
func ValidateAstralCatalog(catalog *EndgameCatalog) error {
	return validateAstralCatalogVersion(catalog, UniqueContentVersion)
}
func validateAstralCatalogVersion(c *EndgameCatalog, version int32) error {
	astralRuneCatalog := c.Runes()
	astralRecipeCatalog := c.Recipes(version)
	if len(astralRuneCatalog) == 0 || len(astralRecipeCatalog) == 0 {
		return fmt.Errorf("astral catalog requires rune and formation definitions")
	}
	used := map[ItemID]bool{}
	seen := map[string]bool{}
	for i, r := range astralRuneCatalog {
		if r.Item <= 0 || r.Name == "" || r.MinLevel <= 0 || len(r.Affixes) == 0 || len(r.Affixes) > len((CardInstanceState{}).Effects) {
			return fmt.Errorf("invalid rune row %d", i)
		}
		for _, a := range r.Affixes {
			if !SupportedAstralRuneDefinitionAffix(r, a) {
				return fmt.Errorf("unsupported rune affix %d/%+v", r.Item, a)
			}
		}
	}
	for _, r := range astralRecipeCatalog {
		if r.Name == "" || r.Channel < AstralBattle || r.Channel > AstralSupport || len(r.Runes) < 2 || len(r.Runes) > 5 || len(r.Slots) == 0 || len(r.Slots) > 32 || len(r.Traits) > 3 || !validEndgamePowerTier(r.PowerTier) || r.Professions&^uint8(31) != 0 || r.MinLevel <= 0 {
			return fmt.Errorf("invalid formation %d", r.ID)
		}
		seenSlots := map[EquipSlot]bool{}
		for _, slot := range r.Slots {
			if slot < SlotFace || slot > maxEquipSlot || seenSlots[slot] {
				return fmt.Errorf("invalid formation %d slot %d", r.ID, slot)
			}
			seenSlots[slot] = true
		}
		if err := validateEndgameExtraAffixes(r.Affixes, false); err != nil {
			return fmt.Errorf("formation %d: %w", r.ID, err)
		}
		if err := validateEndgameRollModel(r.Affixes, r.VariableAffixes, r.LocalEnhancements); err != nil {
			return fmt.Errorf("formation %d: %w", r.ID, err)
		}
		for _, t := range r.Traits {
			if !ValidUniqueTrait(t) || !TraitMatchesProfessions(t, r.Professions) {
				return fmt.Errorf("invalid formation trait %d", r.ID)
			}
		}
		if err := ValidateEquipmentEffects(r.TypedEffects, c); err != nil {
			return fmt.Errorf("formation %d: %w", r.ID, err)
		}
		if err := ValidateSkillRankBonuses(r.SkillBonuses, c.skills); err != nil {
			return fmt.Errorf("formation %d: %w", r.ID, err)
		}
		for _, b := range r.ReservedSkills {
			if !b.Valid() {
				return fmt.Errorf("invalid reserved skill attribute %d", r.ID)
			}
		}
		key := fmt.Sprint(r.Runes)
		if seen[key] {
			return fmt.Errorf("duplicate formation %d", r.ID)
		}
		seen[key] = true
		maxRuneLevel := int32(0)
		for _, id := range r.Runes {
			if rune, ok := AstralRune(id, c); !ok {
				return fmt.Errorf("unknown rune %d", id)
			} else if rune.MinLevel > maxRuneLevel {
				maxRuneLevel = rune.MinLevel
			}
			used[id] = true
		}
		if version == UniqueContentVersion && r.MinLevel != maxRuneLevel {
			return fmt.Errorf("formation %d requirement must equal highest rune level %d", r.ID, maxRuneLevel)
		}
	}
	if len(used) != len(astralRuneCatalog) {
		return fmt.Errorf("not all mansions used")
	}
	return nil
}

// EquipmentRequiredLevel is derived from current authoritative catalogs, never
// saved on an instance. Existing cards and saved equipment therefore inherit new
// requirements without rewriting their identities or snapshotted effects.
// A formation adds no independent level tier: keep the higher of the base
// equipment requirement and every installed rune requirement.
func EquipmentRequiredLevel(def ItemDef, st Stack) int32 {
	level := int32(0)
	if def.Equip != nil {
		level = def.Equip.Need.Level
	}
	for i := 0; i < int(st.SocketCount) && i < len(st.Sockets); i++ {
		if rune, ok := AstralRune(st.Sockets[i], def.EndgameCatalog); ok && rune.MinLevel > level {
			level = rune.MinLevel
		}
	}
	return level
}

// AstralTraits observes exactly the same channel winner, broken-equipment and
// class/level gates as stat bonuses. Suppressed arrays grant no formation trait.
func AstralTraits(worn *EquipSet, defs func(ItemID) (ItemDef, bool), ch *Character) []UniqueTrait {
	var out []UniqueTrait
	for _, a := range ActiveAstralRecipes(worn, defs, ch) {
		out = append(out, a.Recipe.Traits...)
	}
	return out
}
func (r AstralRecipe) Description() string { return r.Lore }

func (r AstralRuneDef) Description() string {
	if r.RegenerationIntervalMS > 0 {
		return fmt.Sprintf("有效穿戴时，每%g秒按生命恢复数值恢复生命。也可按孔位顺序组成宿阵。", float64(r.RegenerationIntervalMS)/1000)
	}
	return "单枚镶嵌即可提供自身属性，使用等级以装备与已镶星篆的最高要求为准。孔位填满且底材、孔数、排列符合配方时，额外组成宿阵。"
}

// adoptCurrentFormationOnCompletion only runs as part of an explicit socket
// mutation. Reading an existing complete V4 array never upgrades it. Removing
// a rune and subsequently completing an array is a new formation operation and
// opts that equipment into V5; its saved quality roll is never regenerated.
func adoptCurrentFormationOnCompletion(def ItemDef, before Stack, after *Stack) bool {
	if after == nil || before.UID != after.UID || before.Item != after.Item || before.Endgame.FormationContentVersion != 0 || after.Endgame.FormationContentVersion != 0 {
		return false
	}
	if _, matched := MatchAstralRecipe(def, before); matched {
		return false
	}
	candidate := *after
	candidate.Endgame.FormationContentVersion = UniqueContentVersion
	if _, matched := MatchAstralRecipe(def, candidate); !matched {
		return false
	}
	after.Endgame.FormationContentVersion = UniqueContentVersion
	return true
}
