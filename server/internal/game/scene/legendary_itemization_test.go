package scene

import (
	"encoding/json"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"testing"
)

func legendaryDropFixture(t *testing.T) (*Scene, domain.ItemDef) {
	t.Helper()
	def := domain.ItemDef{ID: 1001, Name: "底材", LegendaryEnabled: true, InstanceKind: domain.ItemInstanceEquipment, Equip: &domain.EquipDef{Slot: int32(domain.SlotWeapon), ResourceLevel: 3, Type: 1, LevelReq: 60, NativeDropEligible: true, Need: domain.Requirement{Professions: 31}}}
	table := domain.EquipmentRollTable{Options: map[domain.EquipmentRollKey][]domain.EquipmentRollOption{}, Counts: map[domain.ColorProfile][]domain.WeightedAffixCount{1: {{Count: 2, Weight: 1}}}}
	for i, a := range []int32{domain.AttrSTR, domain.AttrINT, domain.AttrVIT, domain.AttrDEX, domain.AttrCrit, domain.AttrPhysRes} {
		table.Options[domain.EquipmentRollKey{Level: 3, Type: 1}] = append(table.Options[domain.EquipmentRollKey{Level: 3, Type: 1}], domain.EquipmentRollOption{CardID: int32(i + 1), Affix: domain.Affix{Attr: a, Value: 10}, Weight: 1})
	}
	if len(domain.LegendaryCandidates(def)) == 0 {
		t.Fatal("runtime registered no eligible legendary powers")
	}
	return &Scene{equipmentRoll: table, items: map[domain.ItemID]domain.ItemDef{def.ID: def}}, def
}
func TestLegendaryNaturalDropAndView(t *testing.T) {
	s, def := legendaryDropFixture(t)
	st := domain.NewStack(def, 1)
	q, v, u, r := lootRandomV5{1}, lootRandomV5{2}, lootRandomV5{3}, lootRandomV5{4}
	rule := &domain.LootRuleSet{UniqueBasePPB: domain.LootProbabilityScaleV5, UniqueMFDivisor: 250}
	s.rollNaturalEquipmentQualityV5(def, &st, 1, true, 0, rule, &q, &v, &u, &r)
	if st.Legendary.PowerID == "" || st.Item != def.ID || st.RolledAffixCount != 0 {
		t.Fatalf("drop integration failed: %+v", st)
	}
	view := s.endgameItem(st, "bag", 0)
	if view.Legendary == nil || len(view.Legendary.Affixes) != 6 || len(view.Stats) != 6 {
		t.Fatal("missing six-line view")
	}
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if json.Unmarshal(raw, &doc) != nil {
		t.Fatal("bad json")
	}
	legend := doc["legendary"].(map[string]any)
	for _, a := range legend["affixes"].([]any) {
		if _, ok := a.(map[string]any)["mode"]; !ok {
			t.Fatal("zero mode omitted")
		}
	}
}
func TestLegendaryRiftNeverAdmitsHardDungeonBase(t *testing.T) {
	s, def := legendaryDropFixture(t)
	if !s.riftLegendaryBaseEligible(def) {
		t.Fatal("legal native base excluded")
	}
	def.HardDungeonOnly = true
	if s.riftLegendaryBaseEligible(def) {
		t.Fatal("hard dungeon base leaked")
	}
	def.HardDungeonOnly = false
	def.Equip.LevelReq = 61
	if s.riftLegendaryBaseEligible(def) {
		t.Fatal("rift level ceiling widened")
	}
}
