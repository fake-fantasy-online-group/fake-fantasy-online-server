package scene
import catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"



import (
	"fmt"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

const tooltipBaseLore = "旧底材的来历。\n另一段原装叙述。"

func assertSpecialTooltipOmitsBaseLore(t *testing.T, s *Scene, def domain.ItemDef, st domain.Stack, owner *entity.Entity) string {
	t.Helper()
	tip := s.ownedItemTooltip(def, st, owner)
	withoutLore := def
	withoutLore.Description = ""
	if tip != s.ownedItemTooltip(withoutLore, st, owner) {
		t.Fatalf("special tooltip retains the original base lore:\n%s", tip)
	}
	for _, line := range strings.Split(tooltipBaseLore, "\n") {
		if strings.Contains(tip, line) {
			t.Fatalf("base lore leaked: %s", line)
		}
	}
	for _, preserved := range []string{"耐久度:", "重量 5", "法宝技能: 原生技能", "技能效果: 原生技能效果。", "（绑定物品不可交易/摆摊/邮寄, 可卖NPC）"} {
		if !strings.Contains(tip, preserved) {
			t.Fatalf("functional tooltip section disappeared: %s\n%s", preserved, tip)
		}
	}
	return tip
}

func TestEveryUniqueTooltipReplacesBaseLoreOnly(t *testing.T) {
	for _, unique := range domain.UniqueCatalog(catalogfixture.EndgameCatalog()) {
		t.Run(unique.Name, func(t *testing.T) {
			s := newTestScene(t, nil)
			def := domain.ItemDef{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: unique.BaseID, Name: unique.BaseName, Description: tooltipBaseLore, Weight: 5,
				InstanceKind: domain.ItemInstanceEquipment,
				Equip: &domain.EquipDef{Slot: unique.Slot, Durable: 100, Need: domain.Requirement{Level: 12},
					Skill: &domain.EquipmentSkill{Name: "原生技能", Description: "原生技能效果。"},
					Suit: &domain.EquipmentSuit{ID: 123, Name: "原生套装", Groups: [][]domain.ItemID{{unique.BaseID}},
						Bonuses: []domain.EquipmentSuitBonus{{Pieces: 1, Affix: domain.Affix{Attr: domain.AttrSTR, Value: 4}}}}}}
			s.items = map[domain.ItemID]domain.ItemDef{def.ID: def}
			st := domain.NewStack(def, 1)
			st.UID, st.Bound = 987, true
			if !domain.ApplyUnique(&st, def, func(int) int { return 2000 }) {
				t.Fatal("unique fixture could not awaken")
			}
			tip := assertSpecialTooltipOmitsBaseLore(t, s, def, st, nil)
			for _, preserved := range []string{unique.Description(), "仙遗·" + unique.Name, "需要等级: 12", "套装: 原生套装", "[1件] 力量 +4"} {
				if !strings.Contains(tip, preserved) {
					t.Fatalf("unique lore or native section missing: %s", preserved)
				}
			}
			if view := s.endgameItem(st, "bag", 0); view.Description != unique.Description() {
				t.Fatal("authored unique lore changed in the client overlay payload")
			}
			ordinary := st
			ordinary.Endgame = domain.EndgameItemState{}
			if strings.Count(s.itemTooltip(def, ordinary), tooltipBaseLore) != 1 {
				t.Fatal("ordinary version lost its original description")
			}
		})
	}
}

func TestEveryFormationTooltipOmitsBaseLoreRegardlessOfActivation(t *testing.T) {
	s, _, _ := astralSceneFixture(t)
	p := s.players[1]
	for _, recipe := range domain.AstralRecipes(catalogfixture.EndgameCatalog()) {
		t.Run(recipe.Name, func(t *testing.T) {
			def := s.items[999001]
			equip := *def.Equip
			equip.Slot = int32(recipe.Slots[0])
			equip.Skill = &domain.EquipmentSkill{Name: "原生技能", Description: "原生技能效果。"}
			def.Equip, def.Description, def.Weight = &equip, tooltipBaseLore, 5
			s.items[def.ID] = def
			st := domain.NewStack(def, 1)
			st.UID, st.Bound, st.SocketCount = 987, true, uint8(len(recipe.Runes))
			for i, id := range recipe.Runes {
				st.Sockets[i], st.SocketUIDs[i], st.SocketCards[i] = id, int64(2000+i), s.items[id].CardDefaults
			}
			if matched, ok := domain.MatchAstralRecipe(def, st); !ok || matched.ID != recipe.ID {
				t.Fatal("formation fixture did not match", matched.ID)
			}
			for _, owner := range []*entity.Entity{nil, p} {
				for _, durability := range []int32{0, 100} {
					st.Durability = durability
					p.Player.Char.Level = 1 // Identity survives unmet activation requirements.
					tip := assertSpecialTooltipOmitsBaseLore(t, s, def, st, owner)
					if !strings.Contains(tip, fmt.Sprintf("孔数 %d/%d", len(recipe.Runes), len(recipe.Runes))) {
						t.Fatal("socket section disappeared")
					}
					if view := s.endgameItem(st, "bag", 0); view.FormationID != recipe.ID {
						t.Fatal("overlay no longer resolves authored formation lore")
					}
				}
			}
			// Extracting a constituent ends the identity; a plain socketed item must
			// immediately regain its base description without waiting for activation.
			st.Sockets[0], st.SocketUIDs[0], st.SocketCards[0] = 0, 0, domain.CardInstanceState{}
			if strings.Count(s.itemTooltip(def, st), tooltipBaseLore) != 1 {
				t.Fatal("incomplete formation lost ordinary equipment description")
			}
		})
	}
}

func TestOrdinaryItemTooltipKeepsBaseLore(t *testing.T) {
	s, _, _ := astralSceneFixture(t)
	def := s.items[999001]
	def.Description = tooltipBaseLore
	for _, sockets := range []uint8{0, 2, 5} {
		st := domain.NewStack(def, 1)
		st.UID, st.SocketCount = 987, sockets
		if strings.Count(s.itemTooltip(def, st), tooltipBaseLore) != 1 {
			t.Fatal("ordinary equipment description disappeared", sockets)
		}
	}
	material := domain.ItemDef{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: 999002, Description: tooltipBaseLore}
	if s.itemTooltip(material, domain.NewStack(material, 1)) != tooltipBaseLore {
		t.Fatal("ordinary non-equipment description changed")
	}
}
