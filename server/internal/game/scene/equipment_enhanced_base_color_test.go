package scene

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// Fixtures contain the actual inventory/equipment descriptions and sidecar views,
// so the managed client contract exercises the same text through both entrances.
type enhancedBaseTooltipFixture struct {
	Name      string                       `json:"name"`
	Tooltip   string                       `json:"tooltip"`
	Item      domain.EndgameItemView       `json:"item"`
	Formation *domain.EndgameFormationView `json:"formation,omitempty"`
	Intrinsic []string                     `json:"intrinsic"`
}

func expectedEnhancedBaseRows(e *domain.EquipDef, st domain.Stack, local []domain.ItemLocalEnhancement, roll int32, explicitWhite bool) []string {
	base, stats := domain.EnhancedIntrinsic(e, st.RefineLevel, local, roll)
	// Resolve actual integer increments independently of the renderer's comparison.
	gains := map[int32]int32{}
	for _, a := range domain.ResolveItemLocalEnhancements(e, local, roll) {
		gains[a.Attr] += a.Value
	}
	var rows []string
	add := func(text string, changed bool) {
		if changed {
			text = "\x035" + text
		} else if explicitWhite {
			text = "\x030" + text
		}
		rows = append(rows, text)
	}
	if stats.MinAtk != 0 || stats.MaxAtk != 0 {
		add(fmt.Sprintf("攻击 %d~%d", stats.MinAtk, stats.MaxAtk), gains[domain.AttrMinAtk] != 0 || gains[domain.AttrMaxAtk] != 0)
	}
	for _, r := range []struct {
		name        string
		value, attr int32
	}{
		{"防御", stats.Def, domain.AttrDef}, {"魔法攻击", stats.MAtk, domain.AttrMAtk}, {"魔法防御", stats.MDef, domain.AttrMDef},
		{"命中", stats.Hit, 0}, {"负重上限", stats.MaxWeight, 0},
		{"力量", base.STR, 0}, {"体质", base.VIT, 0}, {"敏捷", base.AGI, 0}, {"智慧", base.INT, 0}, {"精神", base.SPI, 0}, {"灵巧", base.DEX, 0},
	} {
		if r.value != 0 {
			add(fmt.Sprintf("%s %d", r.name, r.value), gains[r.attr] != 0)
		}
	}
	return rows
}

func TestIntrinsicBaseColorUsesActualLocalIncrement(t *testing.T) {
	for _, tc := range []struct {
		name     string
		local    []domain.ItemLocalEnhancement
		small    bool
		wantGold string
	}{
		{name: "no_local_refinement_only"},
		{name: "attack", local: []domain.ItemLocalEnhancement{{Attr: domain.AttrAtk, Percent: 50}}, wantGold: "攻击"},
		{name: "defense", local: []domain.ItemLocalEnhancement{{Attr: domain.AttrDef, Percent: 50}}, wantGold: "防御"},
		{name: "magic_attack", local: []domain.ItemLocalEnhancement{{Attr: domain.AttrMAtk, Percent: 50}}, wantGold: "魔法攻击"},
		{name: "magic_defense", local: []domain.ItemLocalEnhancement{{Attr: domain.AttrMDef, Percent: 50}}, wantGold: "魔法防御"},
		{name: "zero_percent", local: []domain.ItemLocalEnhancement{{Attr: domain.AttrAtk, Percent: 0}}},
		{name: "rounded_zero", local: []domain.ItemLocalEnhancement{{Attr: domain.AttrAtk, Percent: 1}}, small: true},
		{name: "only_max_attack_changes", local: []domain.ItemLocalEnhancement{{Attr: domain.AttrAtk, Percent: 50}}, small: true, wantGold: "攻击"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := nativePresentationEquip(4)
			e.RefineEffects = map[int32]domain.RefineEffect{1: {Bonuses: []domain.Affix{{Attr: domain.AttrMinAtk, Value: 40}, {Attr: domain.AttrMaxAtk, Value: 60}, {Attr: domain.AttrDef, Value: 30}}}}
			if tc.small {
				e.Bonus.MinAtk, e.Bonus.MaxAtk = 1, 2
			}
			st := domain.Stack{RefineLevel: 1}
			for _, native := range []bool{false, true} {
				b, stats := domain.EnhancedIntrinsic(e, 1, tc.local, 10000)
				_, refined := e.RefinedIntrinsic(1)
				var rows []string
				appendIntrinsicEquipmentStats(&rows, stats, b, refined, native)
				want := expectedEnhancedBaseRows(e, st, tc.local, 10000, native)
				if strings.Join(rows, "\n") != strings.Join(want, "\n") {
					t.Fatalf("native=%v rows=%q want=%q", native, rows, want)
				}
				var gold []string
				for _, row := range rows {
					if strings.HasPrefix(row, "\x035") {
						gold = append(gold, strings.Fields(row[2:])[0])
					}
				}
				if strings.Join(gold, ",") != tc.wantGold {
					t.Fatalf("colored unrelated or inert local field: %q want %q", gold, tc.wantGold)
				}
			}
		})
	}
}

func TestEquipmentEnhancedBaseTooltipCatalogAndEntrances(t *testing.T) {
	s, _, _ := astralSceneFixture(t)
	p := s.players[1]
	p.Player.EndgameNativeStats = true
	p.Player.Bag = domain.NewBag(domain.DefaultBagSlots)
	var fixtures []enhancedBaseTooltipFixture
	seen := map[string]bool{}
	checked := 0
	verify := func(name string, def domain.ItemDef, st domain.Stack, local []domain.ItemLocalEnhancement, roll int32, native bool) {
		t.Helper()
		s.items[def.ID] = def
		want := expectedEnhancedBaseRows(def.Equip, st, local, roll, native)
		// Generate both native packet entrances with the very same instance, moved
		// between locations. This deliberately includes broken-item potential previews.
		p.Player.Bag.Set(200, st)
		bag, err := s.inventorySnapshot(p)
		if err != nil || len(bag.Items) != 1 {
			t.Fatalf("%s bag snapshot: %+v %v", name, bag, err)
		}
		p.Player.Bag.Set(200, domain.Stack{})
		slot := domain.EquipSlot(def.Equip.Slot)
		p.Player.Worn.Set(slot, st)
		worn, err := s.inventorySnapshot(p)
		p.Player.Worn.Set(slot, domain.Stack{})
		if err != nil || len(worn.Equipped) != 1 {
			t.Fatalf("%s worn snapshot: %+v %v", name, worn, err)
		}
		tip := bag.Items[0].Desc
		if tip != worn.Equipped[0].Desc {
			t.Fatalf("%s bag and worn tooltip differ", name)
		}
		if !strings.Contains(tip, "\n"+strings.Join(want, "\n")+"\n") {
			t.Fatalf("%s intrinsic values/colors/order changed: want %q in %q", name, want, tip)
		}
		for _, row := range want {
			if strings.Count("\n"+tip+"\n", "\n"+row+"\n") != 1 {
				t.Fatalf("%s duplicated intrinsic %q", name, row)
			}
		}
		checked++
		key := strings.Split(name, "/")[0]
		if !seen[key] {
			seen[key] = true
			f := enhancedBaseTooltipFixture{Name: name, Tooltip: tip, Item: s.endgameItem(st, "bag", 0), Intrinsic: want}
			if recipe, ok := domain.MatchAstralRecipe(def, st); ok {
				f.Formation = &domain.EndgameFormationView{ID: recipe.ID, Name: recipe.Name, Description: recipe.Description(), Channel: recipe.Channel.String(), Runes: []int32{}}
				for _, id := range recipe.Runes {
					f.Formation.Runes = append(f.Formation.Runes, int32(id))
				}
			}
			fixtures = append(fixtures, f)
		}
	}
	makeDef := func(id domain.ItemID, slot int32) domain.ItemDef {
		e := nativePresentationEquip(slot)
		e.Affixes = []domain.Affix{{Attr: domain.AttrAtk, Value: 50, Mode: domain.ModePercent}, {Attr: domain.AttrDef, Value: 11}}
		e.RefineEffects = map[int32]domain.RefineEffect{1: {Bonuses: []domain.Affix{{Attr: domain.AttrMinAtk, Value: 40}, {Attr: domain.AttrMaxAtk, Value: 60}, {Attr: domain.AttrDef, Value: 30}}}}
		return domain.ItemDef{ID: id, Name: "颜色回归底材", InventoryTab: 2, InventoryTabKnown: true, InstanceKind: domain.ItemInstanceEquipment, Equip: e, Weight: 5}
	}
	for _, version := range []int32{0, 5, domain.UniqueContentVersion} {
		for _, entry := range domain.UniqueCatalog() {
			def := makeDef(entry.BaseID, entry.Slot)
			for _, roll := range []int32{8000, 10000, 12000} {
				st := domain.NewStack(def, 1)
				st.UID, st.RefineLevel, st.RolledAffixCount = 42, 1, 1
				st.RolledAffixes[0].Affix = domain.Affix{Attr: domain.AttrMAtk, Value: 70, Mode: domain.ModePercent}
				st.Endgame = domain.EndgameItemState{UniqueID: int32(st.Item), UniqueRollBP: roll, UniqueContentVersion: version}
				u, ok := domain.UniqueOf(st)
				if !ok {
					continue
				} // A new base need not exist in the frozen old catalog.
				key := "unique-no-local"
				if len(u.LocalEnhancements) > 0 {
					key = fmt.Sprintf("unique-local-%d", u.LocalEnhancements[0].Attr)
				}
				for _, durability := range []int32{0, 100} {
					st.Durability = durability
					verify(fmt.Sprintf("%s/v%d-%d-%d-%d", key, version, entry.ID, roll, durability), def, st, u.LocalEnhancements, roll, true)
				}
				if len(u.LocalEnhancements) > 0 && roll == 10000 && version == domain.UniqueContentVersion {
					zero := def
					equip := *def.Equip
					zero.Equip = &equip
					equip.Bonus.MinAtk, equip.Bonus.MaxAtk, equip.Bonus.Def, equip.Bonus.MAtk, equip.Bonus.MDef = 0, 0, 0, 0, 0
					verify("unique-zero-base/"+fmt.Sprint(entry.ID), zero, st, u.LocalEnhancements, roll, true)
				}
			}
		}
		for _, entry := range domain.AstralRecipes() {
			def := makeDef(999001, int32(entry.Slots[0]))
			def.Equip.Affixes = nil // Formations require a naturally plain base.
			st := domain.NewStack(def, 1)
			st.UID, st.RefineLevel, st.SocketCount = 42, 1, uint8(len(entry.Runes))
			st.Endgame.FormationContentVersion, st.Endgame.FormationRollBP = version, 12000
			for i, id := range entry.Runes {
				st.Sockets[i], st.SocketUIDs[i], st.SocketCards[i] = id, int64(50+i), s.items[id].CardDefaults
			}
			r, ok := domain.MatchAstralRecipe(def, st)
			if !ok {
				t.Fatalf("recipe fixture %d", entry.ID)
			}
			key := "formation-no-local"
			if len(r.LocalEnhancements) > 0 {
				key = fmt.Sprintf("formation-local-%d", r.LocalEnhancements[0].Attr)
			}
			for _, durability := range []int32{0, 100} {
				st.Durability = durability
				verify(fmt.Sprintf("%s/v%d-%d-%d", key, version, r.ID, durability), def, st, r.LocalEnhancements, 12000, true)
			}
		}
	}
	def := makeDef(999001, 4)
	plain := domain.NewStack(def, 1)
	plain.UID, plain.RefineLevel = 42, 1
	verify("ordinary-affixes-refinement/no-local", def, plain, nil, 10000, false)
	def.Equip.Affixes = nil
	plain.RefineLevel = 0
	verify("plain/no-local", def, plain, nil, 10000, false)
	if path := os.Getenv("FFO_ENHANCED_TOOLTIP_FIXTURES"); path != "" {
		data, err := json.MarshalIndent(fixtures, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("verified %d actual inventory/equipment tooltip pairs; %d cross-client fixtures", checked, len(fixtures))
}
