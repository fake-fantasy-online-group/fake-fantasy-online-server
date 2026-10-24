package scene

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
)

// A deterministic instance from the production affix roller, not a live item.
func legendaryTooltipFixture(t *testing.T, high bool) (*Scene, domain.ItemDef, domain.Stack) {
	t.Helper()
	roller, def := legendaryDropFixture(t)
	def.Name, def.Description = "青锋剑", "青锋出鞘，寒光照影。"
	def.InventoryTab, def.InventoryTabKnown = 2, true
	def.Equip = nativePresentationEquip(int32(domain.SlotWeapon))
	def.Equip.Type, def.Equip.ResourceLevel, def.Equip.LevelReq = 2, 3, 60
	def.Equip.Need = domain.Requirement{Level: 60, Professions: 31}
	// A template extra is replaced by the instance's six rolled affixes.
	def.Equip.Affixes = []domain.Affix{{Attr: domain.AttrSTR, Value: 999}}
	roller.equipmentRoll.Options[domain.EquipmentRollKey{Level: 3, Type: 2}] =
		roller.equipmentRoll.Options[domain.EquipmentRollKey{Level: 3, Type: 1}]
	st := domain.NewStack(def, 1)
	st.UID, st.Bound = 42, true
	if !roller.equipmentRoll.ApplyLegendaryLegacy(&st, def, func(n int) int {
		if high {
			return n - 1
		}
		return 0
	}) {
		t.Fatal("legendary tooltip fixture could not roll six affixes")
	}
	st.Legendary.PowerID = "JK-A03"
	if err := st.Legendary.Validate(); err != nil {
		t.Fatal(err)
	}
	s, _, _ := astralSceneFixture(t)
	s.items[def.ID] = def
	return s, def, st
}

func TestLegendaryTooltipUsesExistingAttributeAndEffectBlocks(t *testing.T) {
	for _, high := range []bool{false, true} {
		s, def, st := legendaryTooltipFixture(t, high)
		p := s.players[1]
		for _, native := range []bool{false, true} {
			p.Player.EndgameNativeStats = native
			for _, power := range domain.LegendaryPowers() {
				t.Run(fmt.Sprintf("high=%t/native=%t/%s", high, native, power.ID), func(t *testing.T) {
					st.Legendary.PowerID = power.ID
					before := st
					tip := s.ownedItemTooltip(def, st, p)
					var attrs []string
					for _, a := range st.Legendary.Affixes {
						attrs = append(attrs, affixText(a.Affix))
					}
					assertNativeEquipmentPresentation(t, tip, attrs, []string{power.Name + "：" + power.Effect})
					view := s.endgameItem(st, "bag", 0)
					if view.Description != "" || view.Legendary == nil || !strings.HasSuffix(tip, "\x06"+view.Legendary.Description) {
						t.Fatal("legacy client requires identical native and sidecar effect text", view)
					}
					for _, forbidden := range []string{"传奇·", "基底：", "力量 +999", domain.LegendaryDisplayName(def.Name, power) + "："} {
						if strings.Contains(tip, forbidden) {
							t.Fatalf("extra heading, equipment-name prefix, or template affix leaked: %q", forbidden)
						}
					}
					if strings.Count(tip, "\x06"+power.Name+"：") != 1 || strings.Count(tip, power.Effect) != 1 || !strings.Contains(tip, def.Description) {
						t.Fatal("power name/effect duplicated or native lore removed", tip)
					}
					if got := equipmentQuality(def, st); got != 5 {
						t.Fatalf("legendary name quality=%d, want native dark gold 5", got)
					}
					if got, want := itemDisplayName(def, st), domain.LegendaryDisplayName(def.Name, power); got != want {
						t.Fatalf("native name=%q, want %q", got, want)
					}
					if st != before {
						t.Fatal("presentation changed immutable instance rolls")
					}
				})
			}
		}
	}
}

func TestLegendaryTooltipSnapshotPreservesSixTypedRolls(t *testing.T) {
	s, def, st := legendaryTooltipFixture(t, true)
	power, _ := domain.LegendaryPower(st.Legendary.PowerID)
	view := s.endgameItem(st, "bag", 0)
	if view.Name != itemDisplayName(def, st) || view.Description != "" || view.Legendary == nil || view.Legendary.Description != power.Name+"："+power.Effect || view.Legendary.Name != power.Name {
		t.Fatal("named effect must match the native row without being copied into item lore", view)
	}
	wire := protocol.EncodeEndgame(domain.EndgameResponse{Kind: "snapshot", OK: true, SnapshotID: "legendary-tooltip", Chunks: 1, Items: []domain.EndgameItemView{view}})
	if len(wire) < 6 {
		t.Fatal("legendary tooltip snapshot did not encode")
	}
	var decoded domain.EndgameResponse
	if err := json.Unmarshal(wire[6:], &decoded); err != nil {
		t.Fatal(err)
	}
	got := decoded.Items[0]
	if !reflect.DeepEqual(got, view) || len(got.Stats) != domain.LegendaryAffixCount || len(got.Legendary.Affixes) != domain.LegendaryAffixCount {
		t.Fatal("snapshot changed the existing six-affix serialization contract")
	}
	for i, a := range st.Legendary.Affixes {
		if got.Stats[i] != (domain.EndgameStatView{Attr: a.Affix.Attr, Value: a.Affix.Value, Mode: a.Affix.Mode}) ||
			got.Legendary.Affixes[i] != (domain.LegendaryAffixView{Attr: a.Affix.Attr, Value: a.Affix.Value, Mode: a.Affix.Mode, Min: st.Legendary.Bounds[i].Min, Max: st.Legendary.Bounds[i].Max}) {
			t.Fatalf("rolled value, unit, or bounds changed at affix %d", i)
		}
	}
	st.RefineLevel = 3
	if got, want := itemDisplayName(def, st), view.Name+"+3"; got != want {
		t.Fatalf("refinement suffix changed: %q, want %q", got, want)
	}
}

func TestLegendaryTooltipNativeInventoryFixture(t *testing.T) {
	s, def, st := legendaryTooltipFixture(t, false)
	p := s.players[1]
	p.Player.EndgameNativeStats = true
	p.Player.Bag = domain.NewBag(domain.DefaultBagSlots)
	p.Player.Bag.Set(200, st)
	snapshot, err := s.inventorySnapshot(p)
	if err != nil || len(snapshot.Items) != 1 {
		t.Fatal("native inventory fixture failed", snapshot, err)
	}
	item := snapshot.Items[0]
	if item.Name != itemDisplayName(def, st) || item.Quality != 5 || item.Desc != s.ownedItemTooltip(def, st, p) {
		t.Fatal("inventory does not use the fixed tooltip payload", item)
	}
	// Optional paired-client export invokes the real native snapshot and sidecar
	// generators. It never contacts a database, game server, or client process.
	if path := os.Getenv("LEGENDARY_TOOLTIP_FIXTURE_OUT"); path != "" {
		fixture := struct {
			Source   string                 `json:"source"`
			Name     string                 `json:"name"`
			Quality  uint8                  `json:"quality"`
			Tooltip  string                 `json:"tooltip"`
			Snapshot domain.EndgameResponse `json:"snapshot"`
		}{"Deterministic test item emitted by production inventory/sidecar code; not a live game capture.", item.Name, item.Quality, item.Desc,
			domain.EndgameResponse{Kind: "snapshot", OK: true, SnapshotID: "legendary-tooltip-preview", Chunks: 1, Items: []domain.EndgameItemView{s.endgameItem(st, "bag", 0)}}}
		raw, err := json.MarshalIndent(fixture, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(raw, '\n'), 0644); err != nil {
			t.Fatal(err)
		}
	}
}
