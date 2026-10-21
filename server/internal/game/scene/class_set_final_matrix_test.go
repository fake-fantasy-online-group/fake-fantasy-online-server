package scene

import (
	"encoding/json"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Fixture is exported from the real PostgreSQL migration95 acceptance database.
func TestClassSetFinalMigratedMatrixTooltip(t *testing.T) {
	var rows []struct {
		SuitID int32 `json:"suit_id"`
		Seq    int32 `json:"seq"`
		Attr   int32 `json:"attr_id"`
		Mode   int32 `json:"mode"`
		Prob   int32 `json:"prob"`
		Value  int32 `json:"value"`
	}
	data, err := os.ReadFile("testdata/class_set_final_matrix95.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 52 {
		t.Fatalf("matrix rows %d", len(rows))
	}
	names := map[int32]string{1: "夜色拥抱", 2: "信仰之翼", 3: "哲理之石", 4: "胜利王座", 5: "勇气之刃"}
	races := map[int32]domain.Race{1: domain.Assassin, 2: domain.Healer, 3: domain.Warlock, 4: domain.Swordsman, 5: domain.Warrior}
	snapshots := map[string][]string{}
	for id := int32(1); id <= 5; id++ {
		s, p := meleeSetFixture(t, id, 8)
		p.Player.Char.Race = races[id]
		suit := s.items[p.Player.Worn.At(domain.SlotHead).Item].Equip.Suit
		suit.Name = names[id]
		suit.Bonuses = nil
		for _, r := range rows {
			if r.SuitID == id {
				suit.Bonuses = append(suit.Bonuses, domain.EquipmentSuitBonus{Pieces: r.Seq, Affix: domain.Affix{Attr: r.Attr, Mode: r.Mode, Value: r.Value}, Probability: r.Prob})
			}
		}
		for _, n := range []int{8, 7, 6, 5, 4, 3, 2, 1, 0} {
			slots := []domain.EquipSlot{domain.SlotHead, domain.SlotNeck, domain.SlotWeapon, domain.SlotGlove, domain.SlotRing, domain.SlotBody, domain.SlotShoe, domain.SlotBag}
			if n < 8 {
				p.Player.Worn.Set(slots[n], domain.Stack{})
			}
			s.invalidateCombatAttributes(p)
			var lines []string
			s.appendSuitTooltip(&lines, suit, p)
			for _, b := range suit.Bonuses {
				expected := "\x02"
				if int32(n) >= b.Pieces {
					expected = "\x05"
				}
				needle := expected + equipmentSuitBonusText(b)
				found := false
				for _, line := range lines {
					if line == needle {
						found = true
					}
				}
				if !found {
					t.Fatalf("set%d count%d missing %q in %q", id, n, needle, lines)
				}
			}
			for _, line := range lines {
				if strings.Contains(line, "件]") {
					t.Fatalf("piece label %q", line)
				}
			}
			if n == 8 || n == 2 {
				snapshots[names[id]+string(rune('0'+n))] = lines
			}
		}
	}
	if out := os.Getenv("CLASS_SET_TOOLTIP_EXPORT"); out != "" {
		b, err := json.MarshalIndent(snapshots, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Clean(out), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
