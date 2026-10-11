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
	Lore                                   string
	Identity, Benefit, Tradeoff, PowerTier string
	Traits                                 []UniqueTrait
	ReservedSkills                         []ReservedSkillAttribute
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
	r.Runes = append([]ItemID(nil), r.Runes...)
	r.Slots = append([]EquipSlot(nil), r.Slots...)
	r.Affixes = append([]Affix(nil), r.Affixes...)
	r.Traits = append([]UniqueTrait(nil), r.Traits...)
	r.ReservedSkills = append([]ReservedSkillAttribute(nil), r.ReservedSkills...)
	return r
}
func AstralRunes() []AstralRuneDef {
	out := make([]AstralRuneDef, len(astralRuneCatalog))
	for i, r := range astralRuneCatalog {
		out[i] = cloneAstralRune(r)
	}
	return out
}
func AstralRecipes() []AstralRecipe {
	out := make([]AstralRecipe, len(astralRecipeCatalog))
	for i, r := range astralRecipeCatalog {
		out[i] = cloneAstralRecipe(r)
	}
	return out
}
func AstralRune(id ItemID) (AstralRuneDef, bool) {
	if id < FirstAstralRune || id > LastAstralRune {
		return AstralRuneDef{}, false
	}
	return cloneAstralRune(astralRuneCatalog[id-FirstAstralRune]), true
}
func AstralRecipeByID(id int32) (AstralRecipe, bool) {
	if id < 1 || int(id) > len(astralRecipeCatalog) {
		return AstralRecipe{}, false
	}
	return cloneAstralRecipe(astralRecipeCatalog[id-1]), true
}

// The icon is an existing, startup-verified asset. Every rune is a real durable
// SocketCard, with fixed effects snapshotted when its unique instance is created.
func AstralRuneItems(icon int32) []ItemDef {
	out := make([]ItemDef, 0, 28)
	for _, r := range astralRuneCatalog {
		card := CardInstanceState{Initialized: true, Count: uint8(len(r.Affixes))}
		for i, a := range r.Affixes {
			card.Effects[i] = CardEffect{Op: 1, Attr: a.Attr, Value: a.Value, Mode: a.Mode, Probability: 100}
		}
		out = append(out, ItemDef{ID: r.Item, Name: r.Name, Description: "按孔位顺序镶入星篆，可结成宿阵。", Level: r.MinLevel, UseLevel: r.MinLevel, Weight: 1, Icon: icon, InventoryTab: 1, InventoryTabKnown: true, InstanceKind: ItemInstanceSocketCard, CanMail: true, CanTrade: true, CardDefaults: card, SocketAffixes: append([]Affix(nil), r.Affixes...)})
	}
	return out
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
		if _, ok := AstralRune(st.Sockets[i]); !ok || st.SocketUIDs[i] <= 0 || !st.SocketCards[i].Initialized || seen[st.SocketUIDs[i]] {
			return AstralRecipe{}, false
		}
		seen[st.SocketUIDs[i]] = true
	}
	for _, r := range astralRecipeCatalog {
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
		out = append(out, a.Recipe.Affixes...)
	}
	return out
}
func ValidateAstralCatalog() error {
	if len(astralRuneCatalog) != 28 || len(astralRecipeCatalog) != 40 {
		return fmt.Errorf("astral catalog requires 28 runes and 40 formations")
	}
	used := map[ItemID]bool{}
	seen := map[string]bool{}
	for i, r := range astralRuneCatalog {
		if r.Item != FirstAstralRune+ItemID(i) || r.Name == "" || r.MinLevel < 12 || r.MinLevel > 60 || (i > 0 && r.MinLevel <= astralRuneCatalog[i-1].MinLevel) || len(r.Affixes) == 0 {
			return fmt.Errorf("invalid rune row %d", i)
		}
		for _, a := range r.Affixes {
			if !SupportedAstralRuneAffix(r.Item, a) {
				return fmt.Errorf("unsupported rune affix %d/%+v", r.Item, a)
			}
		}
	}
	for i, r := range astralRecipeCatalog {
		if r.ID != int32(i+1) || r.Name == "" || r.Channel < AstralBattle || r.Channel > AstralSupport || len(r.Runes) < 2 || len(r.Runes) > 5 || len(r.Slots) == 0 || len(r.Affixes) == 0 || r.Professions&^uint8(31) != 0 || r.MinLevel < 12 || r.MinLevel > 60 {
			return fmt.Errorf("invalid formation %d", r.ID)
		}
		for _, a := range r.Affixes {
			if !SupportedEndgameAffix(a) {
				return fmt.Errorf("unsupported formation affix %d/%+v", r.ID, a)
			}
		}
		for _, t := range r.Traits {
			if !ValidUniqueTrait(t) || !TraitMatchesProfessions(t, r.Professions) {
				return fmt.Errorf("invalid formation trait %d", r.ID)
			}
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
		for _, id := range r.Runes {
			if rune, ok := AstralRune(id); !ok {
				return fmt.Errorf("unknown rune %d", id)
			} else if r.MinLevel < rune.MinLevel {
				return fmt.Errorf("formation %d undercuts rune %d requirement", r.ID, id)
			}
			used[id] = true
		}
	}
	if len(used) != 28 {
		return fmt.Errorf("not all mansions used")
	}
	return nil
}

// EquipmentRequiredLevel is derived from current authoritative catalogs, never
// saved on an instance. Existing cards and saved equipment therefore inherit new
// requirements without rewriting their identities or snapshotted effects.
func EquipmentRequiredLevel(def ItemDef, st Stack) int32 {
	level := int32(0)
	if def.Equip != nil {
		level = def.Equip.Need.Level
	}
	for i := 0; i < int(st.SocketCount) && i < len(st.Sockets); i++ {
		if rune, ok := AstralRune(st.Sockets[i]); ok && rune.MinLevel > level {
			level = rune.MinLevel
		}
	}
	if recipe, ok := MatchAstralRecipe(def, st); ok && recipe.MinLevel > level {
		level = recipe.MinLevel
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
	if r.Item == LastAstralRune {
		return "有效穿戴时，每2秒按生命恢复数值恢复生命。也可按孔位顺序组成宿阵。"
	}
	return "按镶孔顺序与其他星篆组成宿阵。"
}
