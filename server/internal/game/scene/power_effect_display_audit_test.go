package scene

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"
)

// Exercise real native/sidecar producers, not a sanitized copy of the catalog.
func TestPowerEffectDisplayAll330AtEveryRollBoundary(t *testing.T) {
	s, def, base := legendaryTooltipFixture(t, true)
	owner := s.players[1]
	wingDef := def
	wingEquip := *def.Equip
	wingEquip.Slot = int32(domain.SlotBag)
	wingDef.Equip = &wingEquip
	for _, power := range domain.LegendaryPowers() {
		for _, roll := range []int32{0, 7500, 8123, 10000} {
			if !domain.LegendaryPowerRollValid(power.ID, roll) {
				continue
			}
			t.Run(fmt.Sprintf("%s/%d", power.ID, roll), func(t *testing.T) {
				resolved, _ := domain.LegendaryPowerForRoll(power.ID, roll)
				want := resolved.Name + "：" + resolved.Effect
				if strings.Contains(want, "～") {
					t.Fatal("effect contains roll range", want)
				}
				st := base
				st.Legendary.PowerID, st.Legendary.PowerRollBP = power.ID, roll
				before := st
				for _, native := range []bool{false, true} {
					owner.Player.EndgameNativeStats = native
					for _, tip := range []string{s.ownedItemTooltip(def, st, owner), s.itemTooltipForViewer(def, st, owner, nil)} {
						if strings.Count(tip, "\x06"+want) != 1 {
							t.Fatal("native/inspection mismatch", tip, want)
						}
					}
				}
				for _, location := range []string{"bag", "equipped"} {
					view := s.endgameItem(st, location, 0)
					if view.Legendary == nil || view.Legendary.Description != want || view.Description != "" {
						t.Fatal("sidecar mismatch", view)
					}
				}
				scroll := domain.Stack{UID: 991, Item: domain.PowerScrollItemID, Count: 1, Bound: true, Endgame: domain.EndgameItemState{PowerScrollID: power.ID, PowerScrollRollBP: roll}}
				scrollBefore := scroll
				scrollDef := domain.ItemDef{ID: domain.PowerScrollItemID, Description: "将威能附着于器物。"}
				if tip := s.itemTooltip(scrollDef, scroll); strings.Count(tip, "\x06"+want) != 1 || !strings.Contains(tip, scrollDef.Description) {
					t.Fatal("scroll native fallback mismatch", tip, want)
				}
				view := s.endgameItem(scroll, "bag", 0)
				if view.PowerScroll == nil || view.PowerScroll.Description != want || view.Description != want {
					t.Fatal("scroll mismatch", view)
				}
				wing := base
				wing.Legendary = domain.LegendaryInstance{}
				index, ok := domain.WingPowerIndex(power)
				if !ok {
					t.Fatal("missing wing category")
				}
				wing.WingPowers[index] = domain.WingPower{PowerID: power.ID, PowerRollBP: roll, SourceTier: domain.LegendaryTierSecret, SourceLevel: 100}
				wingBefore := wing
				if tip := s.ownedItemTooltip(wingDef, wing, owner); strings.Count(tip, "\x06"+want) != 1 {
					t.Fatal("wing mismatch", tip, want)
				}
				if st != before || scroll != scrollBefore || wing != wingBefore {
					t.Fatal("display mutated identity, affixes, or rolls")
				}
			})
		}
	}
}

func TestEffectDisplayRuntimeAppendersDoNotExposeImplementationPolicy(t *testing.T) {
	s := newTestScene(t, nil)
	for _, unique := range domain.UniqueCatalog(catalogfixture.EndgameCatalog()) {
		def := domain.ItemDef{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: unique.BaseID, Name: unique.BaseName, InstanceKind: domain.ItemInstanceEquipment, Equip: &domain.EquipDef{Slot: unique.Slot, Durable: 100}}
		s.items = map[domain.ItemID]domain.ItemDef{def.ID: def}
		st := domain.NewStack(def, 1)
		st.UID = 987
		if !domain.ApplyUnique(&st, def, func(int) int { return 2000 }) {
			t.Fatal("unique fixture")
		}
		if text := s.itemTooltip(def, st); strings.Contains(text, "通道上限") || strings.Contains(text, "同名仙遗特性取最高值") {
			t.Fatal("unique policy leak", text)
		}
	}
	for class := int32(1); class <= 5; class++ {
		for _, pieces := range []int32{classSetFirstSkillPieces, classSetSecondSkillPieces} {
			if text := classSetSkillText(class, pieces); text == "" || strings.Contains(text, "同类不叠加") {
				t.Fatal("set policy leak", text)
			}
		}
	}
	for _, unique := range domain.UniqueCatalog(catalogfixture.EndgameCatalog()) {
		_, effects := s.equipmentV5Lines(unique.TypedEffects, unique.SkillBonuses)
		for _, text := range effects {
			if strings.Contains(text, "需新版客户端") || strings.Contains(text, "主形态不叠加") {
				t.Fatal("typed policy leak", text)
			}
		}
	}
}

// Optional export is generated from the same native inventory message and
// sidecar snapshot used in production; it is not a live-server capture.
func TestPowerScrollNativeFallbackAndSnapshotShareActualEffect(t *testing.T) {
	s, _, _ := legendaryTooltipFixture(t, true)
	p := s.players[1]
	def := domain.ItemDef{ID: domain.PowerScrollItemID, Name: "威能卷轴", Description: "将卷轴拖到装备上，即可附着威能。", InventoryTab: 3, InventoryTabKnown: true}
	s.items[def.ID] = def
	st := domain.Stack{UID: 992, Item: def.ID, Count: 1, Bound: true, Endgame: domain.EndgameItemState{PowerScrollID: "TY-A02", PowerScrollRollBP: 8123}}
	p.Player.Bag = domain.NewBag(domain.DefaultBagSlots)
	p.Player.Bag.Set(300, st)
	snapshot, err := s.inventorySnapshot(p)
	if err != nil || len(snapshot.Items) != 1 {
		t.Fatal("native scroll inventory", err, snapshot)
	}
	item := snapshot.Items[0]
	view := s.endgameItem(st, "bag", 0)
	if view.PowerScroll == nil || !strings.Contains(view.PowerScroll.Description, "48.73%") || strings.Count(item.Desc, "\x06"+view.PowerScroll.Description) != 1 || strings.Contains(item.Desc, "～") {
		t.Fatal("actual scroll effect mismatch", item, view)
	}
	if path := os.Getenv("POWER_SCROLL_DISPLAY_FIXTURE_OUT"); path != "" {
		out := struct {
			Source   string                 `json:"source"`
			Tooltip  string                 `json:"tooltip"`
			Snapshot domain.EndgameResponse `json:"snapshot"`
		}{"Production native inventory and sidecar generated by deterministic test, not live capture.", item.Desc, domain.EndgameResponse{Kind: "snapshot", OK: true, SnapshotID: "scroll-display", Chunks: 1, Items: []domain.EndgameItemView{view}}}
		raw, e := json.MarshalIndent(out, "", "  ")
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(path, append(raw, '\n'), 0644); e != nil {
			t.Fatal(e)
		}
	}
}

func TestEquipmentRegistryDisplayAuditCoversEveryRegisteredSource(t *testing.T) {
	type row struct {
		ID          string                           `json:"id"`
		Kind        domain.EquipmentEffectSourceKind `json:"kind"`
		Consumer    string                           `json:"consumer"`
		Description string                           `json:"description"`
	}
	rows := []row{}
	for _, def := range domain.EquipmentEffectDefinitions() {
		consumer := ""
		switch def.SourceKind {
		case domain.EffectSourceLegendary:
			consumer = "LegendaryPowerForRoll -> native equipment/inspection/wings, snapshot gear/scroll, powerbook"
		case domain.EffectSourceSpecialWeapon:
			consumer = "DungeonWeaponEffectLines -> native specialEffects (registry text not appended)"
		case domain.EffectSourceUniqueTrait:
			consumer = "UniqueTrait.Describe -> native specialEffects and snapshot Trait (registry label not appended)"
		case domain.EffectSourceTyped:
			consumer = "equipmentV5Lines -> typed attributes/effects (internal registry English description not appended)"
		case domain.EffectSourceNative:
			consumer = "native affix/stat formatter (registry dispatch description not appended)"
		case domain.EffectSourceSet:
			consumer = "classSetSkillText / native set bonus formatter (registry dispatch description not appended)"
		default:
			t.Fatal("unaudited source", def)
		}
		rows = append(rows, row{def.ID, def.SourceKind, consumer, def.Description})
	}
	if len(rows) != 440 {
		t.Fatal("registry coverage changed", len(rows))
	}
	if path := os.Getenv("EFFECT_REGISTRY_DISPLAY_AUDIT_OUT"); path != "" {
		raw, e := json.MarshalIndent(rows, "", "  ")
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(path, append(raw, '\n'), 0644); e != nil {
			t.Fatal(e)
		}
	}
}
