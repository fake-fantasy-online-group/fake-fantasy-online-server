package scene

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

func TestAttackSpeedPresentationSignsUnitsAndSources(t *testing.T) {
	for _, tc := range []struct {
		value, mode int32
		want        string
	}{
		{-20, domain.ModePercent, "攻击速度 +20%"},
		{15, domain.ModePercent, "攻击速度 -15%"},
		{0, domain.ModePercent, "攻击速度 +0%"},
		{-2147483648, domain.ModePercent, "攻击速度 +2147483648%"},
		{2147483647, domain.ModePercent, "攻击速度 -2147483647%"},
		{-200, domain.ModeAbsolute, "攻击间隔 -200毫秒"},
		{150, domain.ModeAbsolute, "攻击间隔 +150毫秒"},
		{0, domain.ModeAbsolute, "攻击间隔 +0毫秒"},
	} {
		t.Run(fmt.Sprintf("mode%d/value%d", tc.mode, tc.value), func(t *testing.T) {
			a := domain.Affix{Attr: domain.AttrAtkSpeed, Value: tc.value, Mode: tc.mode}
			before := a
			// Repeated rendering must not negate the durable value a second time.
			for repeat := 0; repeat < 2; repeat++ {
				equipment, ok := equipmentAffixLine(a)
				if !ok {
					t.Fatal("attack-speed equipment affix missing")
				}
				for source, got := range map[string]string{
					"equipment": equipment, "generic": affixText(a), "skill/status": skillAffixText(a),
					"ordinary-card": astralRuneAffixText(999, a), "astral-rune": astralRuneAffixText(domain.FirstAstralRune+19, a),
				} {
					if got != tc.want {
						t.Errorf("%s: got %q, want %q", source, got, tc.want)
					}
				}
				st := domain.Stack{Item: 999, WashCount: 1, WashAffixes: [4]domain.Affix{a}}
				pending := &entity.WashPending{Count: 1, Affixes: st.WashAffixes}
				for _, preview := range []*entity.WashPending{nil, pending} {
					info := washInfoOf(&entity.Entity{ID: 1}, socketTargetRef{}, st, preview)
					if len(info.Affixes) != 1 || info.Affixes[0] != tc.want {
						t.Errorf("wash current/pending: got %v, want %q", info.Affixes, tc.want)
					}
				}
			}
			if a != before {
				t.Fatal("rendering changed the stored attribute")
			}
		})
	}
	for _, attr := range []int32{domain.AttrMoveSpeed, domain.AttrAtk} {
		line, _ := equipmentAffixLine(domain.Affix{Attr: attr, Value: -20, Mode: domain.ModePercent})
		if !strings.HasSuffix(line, " -20%") || !strings.HasSuffix(affixText(domain.Affix{Attr: attr, Value: -20, Mode: domain.ModePercent}), " -20%") {
			t.Fatal("attack-speed sign inversion leaked to another attribute", line)
		}
	}
}

func TestAttackSpeedNativeEquipmentAndRolledTooltips(t *testing.T) {
	s, _, _ := astralSceneFixture(t)
	p := s.players[1]
	for _, native := range []bool{false, true} {
		p.Player.EndgameNativeStats = native
		def := s.items[999001]
		def.Equip.Affixes = []domain.Affix{{Attr: domain.AttrAtkSpeed, Value: -20, Mode: domain.ModePercent}}
		st := domain.NewStack(def, 1)
		st.RolledAffixCount, st.WashCount = 1, 1
		st.RolledAffixes[0].Affix = domain.Affix{Attr: domain.AttrAtkSpeed, Value: 15, Mode: domain.ModePercent}
		st.WashAffixes[0] = domain.Affix{Attr: domain.AttrAtkSpeed, Value: -200, Mode: domain.ModeAbsolute}
		before := st
		for _, worn := range []bool{false, true} {
			if worn {
				p.Player.Worn.Set(domain.SlotWeapon, st)
			}
			tip := s.ownedItemTooltip(def, st, p)
			for _, want := range []string{"攻击速度 +20%", "攻击速度 -15%", "攻击间隔 -200毫秒"} {
				if !strings.Contains(tip, want) {
					t.Fatalf("native%v/worn%v missing %q in %q", native, worn, want, tip)
				}
			}
		}
		if st != before {
			t.Fatal("equipment preview mutated an instance")
		}
	}
	// The negotiated native path formerly used affixText for its merged gold
	// block, whereas the client fallback already inverted the same attr25.
	checked := 0
	for _, unique := range domain.UniqueCatalog() {
		st := domain.Stack{UID: 42, Item: unique.BaseID, Count: 1, Durability: 100,
			Endgame: domain.EndgameItemState{UniqueID: unique.ID, UniqueRollBP: 10000, UniqueContentVersion: domain.UniqueContentVersion}}
		for _, a := range domain.UniqueAffixes(st) {
			if a.Attr != domain.AttrAtkSpeed {
				continue
			}
			def := domain.ItemDef{ID: unique.BaseID, Name: unique.BaseName, InstanceKind: domain.ItemInstanceEquipment, Equip: nativePresentationEquip(unique.Slot)}
			s.items[def.ID] = def
			for _, native := range []bool{false, true} {
				p.Player.EndgameNativeStats = native
				prefix := "\x034"
				if native {
					prefix = "\x035"
				}
				want := prefix + fmt.Sprintf("攻击速度 %+d%%", -int64(a.Value))
				if tip := s.ownedItemTooltip(def, st, p); !containsExactLine(strings.Split(tip, "\n"), want) {
					t.Fatalf("unique%d/native%v missing colored speed %q in %q", unique.ID, native, want, tip)
				}
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatal("no authored attack-speed unique covered")
	}
}

func TestAttackSpeedSkillStatusAndCharacterPresentationPreserveRuntime(t *testing.T) {
	s := newTestScene(t, nil)
	for _, tc := range []struct {
		value, mode, interval, speed int32
		want                         string
	}{
		{-20, domain.ModePercent, 1200, 120, "攻击速度 +20%"},
		{15, domain.ModePercent, 1725, 85, "攻击速度 -15%"},
		{0, domain.ModePercent, 1500, 100, "攻击速度 +0%"},
		{-200, domain.ModeAbsolute, 1300, 100, "攻击间隔 -200毫秒"},
		{150, domain.ModeAbsolute, 1650, 100, "攻击间隔 +150毫秒"},
		{0, domain.ModeAbsolute, 1500, 100, "攻击间隔 +0毫秒"},
	} {
		a := domain.Affix{Attr: domain.AttrAtkSpeed, Value: tc.value, Mode: tc.mode}
		state := domain.StatusDef{ID: 19999, Level: 1, Name: "攻速状态", Desc: "原状态说明", DurationSec: 30, Affixes: []domain.Affix{a}}
		s.statuses = domain.StatusTable{{ID: state.ID, Level: 1}: state}
		def := domain.SkillDef{ID: 99999, Level: 1, Statuses: []domain.StatusApplication{{ID: state.ID, Level: 1}}}
		if lines := s.skillEffectLines(def); !containsExactLine(lines, tc.want) {
			t.Errorf("skill status details missing %q in %v", tc.want, lines)
		}
		actor := &entity.Entity{ID: 1, Status: domain.NewStatusSet()}
		actor.Status.Apply(state, 0, 1)
		if views := s.visibleStatuses(actor); len(views) != 1 || views[0].desc != state.Desc || views[0].buff.Desc != state.Desc {
			t.Fatal("status presentation replaced authored prose", views)
		}
		stats := domain.ApplyStatusStats(domain.Stats{AtkSpeedMS: 1500}, []domain.Affix{a})
		attrs := make([]int32, 40)
		writeKnownClientAttributes(attrs, domain.Base{}, stats)
		if stats.AtkSpeedMS != tc.interval || attrs[33] != tc.speed {
			t.Fatalf("runtime/character attr33 changed: %+v attr33=%d", stats, attrs[33])
		}
		if !reflect.DeepEqual(s.statuses[domain.StatusKey{ID: state.ID, Level: 1}], state) {
			t.Fatal("status display mutated source data")
		}
	}
}
