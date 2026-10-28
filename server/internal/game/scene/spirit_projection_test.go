package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"strings"
	"testing"
)

func TestSpiritProjectionTooltipShowsInstanceNotTemplate(t *testing.T) {
	state := domain.SpiritState{Kind: 9, AttrCount: 2, Attrs: [4]int32{domain.AttrAtk, domain.AttrMAtk}}
	state.Rolls[0][0], state.Rolls[1][0] = 20, 11
	s := &Scene{}
	text := s.itemTooltip(domain.ItemDef{ID: 8038, Description: "obsolete999"}, domain.Stack{Item: 8038, Count: 1, Spirit: state})
	if strings.Contains(text, "obsolete999") || !strings.Contains(text, "加工等级: 1") || !strings.Contains(text, "完美 1") || !strings.Contains(text, "20") || !strings.Contains(text, "11") {
		t.Fatalf("wrong dynamic tooltip: %q", text)
	}
	sealed := s.itemTooltip(domain.ItemDef{ID: 8038, Description: "obsolete999"}, domain.Stack{Item: 8038, Count: 1, Spirit: domain.SpiritState{Kind: 9}})
	if sealed != "尚未解封" {
		t.Fatalf("sealed=%q", sealed)
	}
}

func TestSpiritProjectionFusedTooltipUsesDynamicValuesAndAppearanceOnce(t *testing.T) {
	state := domain.SpiritState{Kind: 9, AttrCount: 2, Attrs: [4]int32{domain.AttrAtk, domain.AttrMAtk}}
	state.Rolls[0][0], state.Rolls[1][0] = 20, 11
	s := &Scene{items: map[domain.ItemID]domain.ItemDef{
		8038: {ID: 8038, Name: "武器灵", EquipmentSoul: &domain.EquipmentSoulDef{Affixes: []domain.Affix{{Attr: domain.AttrAtk, Value: 999}}}},
		1080: {ID: 1080, Name: "灵外形"},
	}}
	st := domain.Stack{Item: 99901, Count: 1, FusedSoul: 8038, FusedSpirit: domain.SpiritAttachment{Item: 8038, State: state}, SpiritAppearance: 13701}
	text := s.fusedDescription(st)
	if strings.Contains(text, "999") || strings.Count(text, "变装: 灵外形") != 1 || !strings.Contains(text, "20") || !strings.Contains(text, "11") || !strings.Contains(text, "80") || !strings.Contains(text, "100") {
		t.Fatalf("incorrect fused tooltip: %q", text)
	}
	st.FusedSpirit, st.FusedSoul = domain.SpiritAttachment{}, 0
	text = s.fusedDescription(st)
	if !strings.Contains(text, "灵外形") || strings.Contains(text, "加工等级") {
		t.Fatalf("appearance must remain on separated equipment: %q", text)
	}
}

func TestSpiritProjectionLegacyTooltipKeepsTemplateValues(t *testing.T) {
	def := domain.ItemDef{ID: 8038, Name: "武器灵", EquipmentSoul: &domain.EquipmentSoulDef{Affixes: []domain.Affix{{Attr: domain.AttrAtk, Value: 77}}}}
	s := &Scene{items: map[domain.ItemID]domain.ItemDef{8038: def}}
	state := domain.SpiritState{Kind: 9, LegacyFixed: true}
	text := s.itemTooltip(def, domain.Stack{Item: 8038, Count: 1, Spirit: state})
	if !strings.Contains(text, "77") || strings.Contains(text, "尚未解封") {
		t.Fatalf("legacy standalone=%q", text)
	}
	text = s.fusedDescription(domain.Stack{FusedSoul: 8038, FusedSpirit: domain.SpiritAttachment{Item: 8038, State: state}})
	if !strings.Contains(text, "77") || strings.Contains(text, "尚未解封") {
		t.Fatalf("legacy fused=%q", text)
	}
}
