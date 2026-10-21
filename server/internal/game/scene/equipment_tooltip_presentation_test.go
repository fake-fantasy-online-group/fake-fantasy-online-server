package scene
import catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"



import (
	"fmt"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func TestEffectHitAffixUsesBasisPointsAndOriginalPalette(t *testing.T) {
	for _, tc := range []struct {
		value int32
		text  string
	}{
		{10000, "效果命中 +100%"}, {500, "效果命中 +5%"}, {1, "效果命中 +0.01%"}, {-50, "效果命中 -0.5%"},
	} {
		a := domain.Affix{Attr: domain.AttrEffectHitBP, Value: tc.value, Mode: domain.ModeAbsolute}
		line, ok := equipmentAffixLine(a)
		if !ok || line != tc.text || affixText(a) != tc.text {
			t.Fatalf("effect-hit unit mismatch: %v %q %q", a, line, affixText(a))
		}
		if got := qualityColoredLine(tooltipDarkGold, line); got != "\x035"+tc.text {
			t.Fatal("ordinary effect-hit affix changed palette", got)
		}
	}
}

func TestEquipmentTooltipTextPalettePreservesOrdinaryColors(t *testing.T) {
	for _, row := range []struct {
		color uint8
		want  string
	}{{0, "攻击 +1"}, {4, "\x034攻击 +1"}, {6, "\x036攻击 +1"}, {tooltipLightBlue, "\x05攻击 +1"}, {tooltipDarkGold, "\x035攻击 +1"}} {
		if got := qualityColoredLine(row.color, "攻击 +1"); got != row.want {
			t.Fatalf("color %d = %q, want %q", row.color, got, row.want)
		}
		if got := qualityColoredLine(row.color, ""); got != "" {
			t.Fatalf("empty line gained a color marker: %q", got)
		}
	}
}

// The native description must stand on its own. Neither color nor effect order
// may depend on the client's name-based overlay recognizing each new attribute.
func assertNativeEquipmentPresentation(t *testing.T, tip string, attributes, effects []string) {
	t.Helper()
	lines := strings.Split(tip, "\n")
	for _, want := range append([]string{"\x030命中 7", "\x030力量 3", "（绑定物品不可交易/摆摊/邮寄, 可卖NPC）"}, prefixedLines("\x035", attributes)...) {
		if !containsExactLine(lines, want) {
			t.Fatalf("missing native line %q in:\n%q", want, tip)
		}
	}
	for _, line := range lines {
		if strings.HasPrefix(line, "\x034") || strings.HasPrefix(line, "\x05") {
			t.Fatalf("native special-item attribute kept yellow/light-blue marker: %q", line)
		}
		if strings.Contains(line, "染色率") || strings.Contains(line, "强化") && strings.Contains(line, "本件") {
			t.Fatalf("old attribute wording: %q", line)
		}
	}
	if len(effects) == 0 {
		if strings.Contains(tip, "\x06") {
			t.Fatalf("ordinary attributes mislabeled as special effects: %q", tip)
		}
		return
	}
	wantSuffix := "\n" + strings.Join(prefixedLines("\x06", effects), "\n")
	if !strings.HasSuffix(tip, wantSuffix) {
		t.Fatalf("effects must be the final orange block after lore/binding: want suffix %q in %q", wantSuffix, tip)
	}
	if got := strings.Count(tip, "\x06"); got != len(effects) {
		t.Fatalf("special effects duplicated or ordinary attributes colored orange: %d, want %d", got, len(effects))
	}
}

func prefixedLines(prefix string, lines []string) []string {
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = prefix + line
	}
	return out
}

func containsExactLine(lines []string, want string) bool {
	for _, line := range lines {
		if line == want {
			return true
		}
	}
	return false
}

func nativePresentationEquip(slot int32) *domain.EquipDef {
	return &domain.EquipDef{Slot: slot, SocketLimit: 5, Durable: 100,
		Base: domain.Base{STR: 3}, Bonus: domain.Stats{MinAtk: 100, MaxAtk: 200, MAtk: 90, Def: 80, MDef: 60, Hit: 7}}
}

func TestEveryUniqueNativeTooltipAttributeAndEffectPresentation(t *testing.T) {
	s, _, _ := astralSceneFixture(t)
	p := s.players[1]
	p.Player.EndgameNativeStats = true
	for _, version := range []int32{0, domain.UniqueContentVersion} {
		for _, entry := range domain.UniqueCatalog(catalogfixture.EndgameCatalog()) {
			t.Run(fmt.Sprintf("v%d/%d", version, entry.ID), func(t *testing.T) {
				def := domain.ItemDef{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: entry.BaseID, Name: entry.BaseName, InstanceKind: domain.ItemInstanceEquipment, Equip: nativePresentationEquip(entry.Slot)}
				s.items[def.ID] = def
				st := domain.NewStack(def, 1)
				st.UID, st.Bound, st.SocketCount = 42, true, 1
				st.Endgame = domain.EndgameItemState{UniqueID: entry.ID, UniqueRollBP: 12000, UniqueContentVersion: version}
				u, ok := domain.UniqueOf(st, catalogfixture.EndgameCatalog())
				if !ok {
					t.Fatal("invalid unique fixture")
				}
				attributes, effects := s.equipmentV5Lines(u.TypedEffects, u.SkillBonuses)
				traits := make([]string, 0, len(u.Traits))
				for _, tr := range u.Traits {
					traits = append(traits, tr.Describe(10000))
				}
				effects = append(traits, effects...)
				attributes = append(attributes, localEnhancementLines(def, u.LocalEnhancements, st.Endgame.UniqueRollBP)...)
				attributes = append(attributes, "孔数 0/1", "仙遗·"+u.Name)
				for _, durability := range []int32{0, 100} {
					st.Durability = durability
					tip := s.ownedItemTooltip(def, st, p)
					assertNativeEquipmentPresentation(t, tip, attributes, effects)
					if !strings.Contains(tip, u.Description()) {
						t.Fatal("authored lore disappeared")
					}
				}
			})
		}
	}
}

func TestEveryFormationNativeTooltipAttributeAndEffectPresentation(t *testing.T) {
	s, _, _ := astralSceneFixture(t)
	p := s.players[1]
	p.Player.EndgameNativeStats = true
	p.Player.Char.Level = 1 // Potential effects remain visible when ineligible.
	for _, version := range []int32{0, domain.UniqueContentVersion} {
		for _, entry := range domain.AstralRecipes(catalogfixture.EndgameCatalog()) {
			t.Run(fmt.Sprintf("v%d/%d", version, entry.ID), func(t *testing.T) {
				def := s.items[999001]
				def.Equip = nativePresentationEquip(int32(entry.Slots[0]))
				s.items[def.ID] = def
				st := domain.NewStack(def, 1)
				st.UID, st.Bound, st.SocketCount = 42, true, uint8(len(entry.Runes))
				st.Endgame.FormationContentVersion, st.Endgame.FormationRollBP = version, 12000
				for i, id := range entry.Runes {
					st.Sockets[i], st.SocketUIDs[i], st.SocketCards[i] = id, int64(50+i), s.items[id].CardDefaults
				}
				r, ok := domain.MatchAstralRecipe(def, st)
				if !ok || r.ID != entry.ID {
					t.Fatal("invalid formation fixture")
				}
				attributes, effects := s.equipmentV5Lines(r.TypedEffects, r.SkillBonuses)
				for _, tr := range r.Traits {
					effects = append(effects, tr.Describe(10000))
				}
				attributes = append(attributes, localEnhancementLines(def, r.LocalEnhancements, st.Endgame.FormationRollBP)...)
				attributes = append(attributes, fmt.Sprintf("孔数 %d/%d", len(r.Runes), len(r.Runes)))
				for _, durability := range []int32{0, 100} {
					st.Durability = durability
					assertNativeEquipmentPresentation(t, s.ownedItemTooltip(def, st, p), attributes, effects)
				}
				st.Endgame.UniqueID = int32(st.Item)
				if _, ok := domain.MatchAstralRecipe(def, st); ok {
					t.Fatal("presentation changed unique/formation exclusion")
				}
			})
		}
	}
}
