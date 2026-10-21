package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"strings"
	"testing"
)

func TestClassSetPiecesUsesFunctionalGroupedEquipment(t *testing.T) {
	s, _ := equipScene(t)
	p := s.entities[1]
	p.Player.Char.Race = domain.Assassin
	p.Player.Char.Level = 40
	suit := &domain.EquipmentSuit{ID: 1, Groups: [][]domain.ItemID{{90001, 90002}, {90003}, {90004}}}
	for i, slot := range []domain.EquipSlot{domain.SlotWeapon, domain.SlotHead, domain.SlotBody, domain.SlotShoe} {
		id := domain.ItemID(90001 + i)
		need := domain.Requirement{}
		if i == 3 {
			need.Level = 90
		}
		s.items[id] = domain.ItemDef{ID: id, Equip: &domain.EquipDef{Slot: int32(slot), Durable: 100, Suit: suit, Need: need}}
		p.Player.Worn.Set(slot, domain.Stack{Item: id, Count: 1, Durability: 100, MaxDurability: 100})
	}
	if got := s.classSetPieces(p, 1); got != 2 {
		t.Fatalf("grouped functional count=%d, want2", got)
	}
	st := p.Player.Worn.At(domain.SlotBody)
	st.Durability = 0
	p.Player.Worn.Set(domain.SlotBody, st)
	if got := s.classSetPieces(p, 1); got != 1 {
		t.Fatalf("broken item counted: %d", got)
	}
	p.Player.Char.Race = domain.Warrior
	if got := s.classSetPieces(p, 1); got != 0 {
		t.Fatalf("wrong class activated: %d", got)
	}
	if got := s.classSetPieces(p, 6); got != 0 {
		t.Fatalf("unregistered suit activated: %d", got)
	}
}

func TestClassSetPiecesNilInputs(t *testing.T) {
	s, _ := equipScene(t)
	for _, p := range []*entity.Entity{nil, {}, {Player: &entity.Player{}}} {
		if got := s.classSetPieces(p, 1); got != 0 {
			t.Fatalf("invalid player=%d", got)
		}
	}
	if got := s.classSetPiecesInWorn(s.entities[1], nil, 1); got != 0 {
		t.Fatalf("nil worn=%d", got)
	}
}

func TestClassSetTooltipsExposeOnlyTwoFourSkillThresholds(t *testing.T) {
	for suitID := int32(1); suitID <= 5; suitID++ {
		for _, pieces := range []int32{0, 1, 2, 3, 4, 5, 6, 7, 8} {
			text := classSetSkillText(suitID, pieces)
			if (text != "") != (pieces == 2 || pieces == 4) {
				t.Fatalf("suit %d pieces %d text=%q", suitID, pieces, text)
			}
		}
	}
	s, _ := equipScene(t)
	suit := &domain.EquipmentSuit{ID: 1, Name: "夜色拥抱", Groups: [][]domain.ItemID{{90001}}, Bonuses: []domain.EquipmentSuitBonus{
		{Pieces: 4, Affix: domain.Affix{Attr: domain.AttrCrit, Value: 500}},
		{Pieces: 6, Affix: domain.Affix{Attr: domain.AttrMaxHP, Value: 2500}},
		{Pieces: 8, Affix: domain.Affix{Attr: domain.AttrMaxAtk, Value: 240}},
		{Pieces: 8, Affix: domain.Affix{Attr: domain.AttrHit, Value: 34}},
	}}
	var lines []string
	s.appendSuitTooltip(&lines, suit, nil)
	text := strings.Join(lines, "\n")
	for _, want := range []string{"2500", "240", "淬毒术总伤害额外增加物理攻击均值的1000%", "再次施加淬毒时，额外造成原淬毒剩余伤害的20%"} {
		if !strings.Contains(text, want) {
			t.Fatalf("tooltip missing %q: %s", want, text)
		}
	}
	if strings.Contains(text, "件]") {
		t.Fatal("old thresholds leaked")
	}
}

func TestClassSetTooltipSkillActivationMatchesFunctionalWorn(t *testing.T) {
	s, p, _ := classPoisonScene(t, 2)
	def := s.items[p.Player.Worn.At(domain.SlotHead).Item]
	suit := def.Equip.Suit
	// Runtime skill text must not need an artificial two-piece database row.
	suit.Bonuses = []domain.EquipmentSuitBonus{{Pieces: 8, Affix: domain.Affix{Attr: domain.AttrMaxAtk, Value: 240}}}
	render := func() string {
		var lines []string
		s.appendSuitTooltip(&lines, suit, p)
		return strings.Join(lines, "\n")
	}
	if !strings.Contains(render(), "\x05淬毒术") {
		t.Fatal("functional set should be active")
	}
	p.Player.Char.Race = domain.Warrior
	if !strings.Contains(render(), "\x02淬毒术") {
		t.Fatal("wrong profession must not show active skill")
	}
	p.Player.Char.Race = domain.Assassin
	def.Equip.Need.Level = 999
	s.invalidateCombatAttributes(p)
	if !strings.Contains(render(), "\x02淬毒术") {
		t.Fatal("unusable piece must not show active skill")
	}
}
