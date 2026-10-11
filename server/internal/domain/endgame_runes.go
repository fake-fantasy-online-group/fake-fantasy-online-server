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
	Mansion, Name, Quadrant string
	Affixes                 []Affix
}
type AstralRecipe struct {
	ID          int32
	Name        string
	Channel     AstralChannel
	MinLevel    int32
	Professions uint8
	Runes       []ItemID
	Slots       []EquipSlot
	Affixes     []Affix
	Lore        string
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
		out = append(out, ItemDef{ID: r.Item, Name: r.Name, Description: "按孔位顺序镶入星篆，可结成宿阵。", Level: 10, UseLevel: 10, Weight: 1, Icon: icon, InventoryTab: 1, InventoryTabKnown: true, InstanceKind: ItemInstanceSocketCard, CanMail: true, CanTrade: true, CardDefaults: card, SocketAffixes: append([]Affix(nil), r.Affixes...)})
	}
	return out
}

// Exact length and order are mandatory: an empty extra hole does not activate a
// shorter prefix. Legacy equipment hydrates kind=None; positive UID plus the
// authoritative equipment template remains sufficient for that compatibility.
func MatchAstralRecipe(def ItemDef, st Stack) (AstralRecipe, bool) {
	if st.Empty() || st.UID <= 0 || (st.InstanceKind != ItemInstanceEquipment && st.InstanceKind != ItemInstanceNone) || def.Equip == nil || st.SocketCount < 2 || st.SocketCount > 5 || int(st.SocketCount) > EquipmentSocketCapacity(def, st) {
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
		if !ok || def.Equip == nil || int32(slot) != def.Equip.Slot || !def.Equip.Need.AllowsCharacter(ch) || !EquipmentFunctional(st, def) {
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
		return fmt.Errorf("astral catalog requires28 runes and40 formations")
	}
	used := map[ItemID]bool{}
	seen := map[string]bool{}
	for i, r := range astralRuneCatalog {
		if r.Item != FirstAstralRune+ItemID(i) || r.Name == "" || len(r.Affixes) == 0 {
			return fmt.Errorf("invalid rune row %d", i)
		}
	}
	for i, r := range astralRecipeCatalog {
		if r.ID != int32(i+1) || r.Name == "" || r.Channel < AstralBattle || r.Channel > AstralSupport || len(r.Runes) < 2 || len(r.Runes) > 5 || len(r.Slots) == 0 || len(r.Affixes) == 0 || r.Professions&^uint8(31) != 0 {
			return fmt.Errorf("invalid formation %d", r.ID)
		}
		key := fmt.Sprint(r.Runes)
		if seen[key] {
			return fmt.Errorf("duplicate formation %d", r.ID)
		}
		seen[key] = true
		for _, id := range r.Runes {
			if _, ok := AstralRune(id); !ok {
				return fmt.Errorf("unknown rune %d", id)
			}
			used[id] = true
		}
	}
	if len(used) != 28 {
		return fmt.Errorf("not all mansions used")
	}
	return nil
}
