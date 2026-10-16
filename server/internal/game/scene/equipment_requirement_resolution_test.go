package scene

import catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"

import (
	"encoding/json"
	"math/rand"
	"os"
	"reflect"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

func requirementResolutionFixture(t testing.TB) (*Scene, *entity.Entity) {
	t.Helper()
	b, err := os.ReadFile("../../../../docs/daoist-endgame/v5/skill-original-ranks.json")
	if err != nil {
		t.Fatal(err)
	}
	var defs []domain.SkillDef
	if err := json.Unmarshal(b, &defs); err != nil {
		t.Fatal(err)
	}
	skills := domain.SkillTable{}
	for _, d := range defs {
		skills[domain.SkillKey{ID: d.ID, Level: d.Level}] = d
	}
	if err := domain.InstallV5SkillProgression(skills, catalogfixture.SkillProgressionCatalog()); err != nil {
		t.Fatal(err)
	}
	s := &Scene{items: map[domain.ItemID]domain.ItemDef{}, skills: skills, endgameCatalog: catalogfixture.EndgameCatalog()}
	p := &entity.Entity{ID: 1, Player: &entity.Player{Char: &domain.Character{
		Level: 60, Race: domain.Warlock, Base: domain.Base{STR: 20, INT: 20, VIT: 20, AGI: 20, DEX: 20, SPI: 20},
		Skills: domain.Learned{10401: 20}, EmploymentKnown: true, Employed: true,
	}, Worn: domain.NewEquipSet()}}
	return s, p
}

func requirementResolutionItem(s *Scene, p *entity.Entity, slot domain.EquipSlot, need, bonus int32) {
	id := domain.ItemID(990000 + slot)
	d := domain.ItemDef{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: id, Equip: &domain.EquipDef{Slot: int32(slot), Durable: 100,
		Need: domain.Requirement{Base: domain.Base{STR: need}}, Base: domain.Base{STR: bonus}}}
	// Tests can replace templates; production scene catalogs are immutable.
	s.invalidateCombatAttributes(p)
	s.items[id] = d
	st := domain.NewStack(d, 1)
	st.UID = int64(id)
	p.Player.Worn.Set(slot, st)
}

func requirementResolutionStacks(worn *domain.EquipSet) map[domain.EquipSlot]domain.Stack {
	out := map[domain.EquipSlot]domain.Stack{}
	worn.Each(func(slot domain.EquipSlot, st domain.Stack) { out[slot] = st })
	return out
}

func TestEquipmentRequirementResolutionLegacyEquivalence(t *testing.T) {
	s, p := requirementResolutionFixture(t)
	rng := rand.New(rand.NewSource(74291))
	// nil resolution executes the original recursive algorithm with memoization
	// entirely disabled, including its original raw-skill cache behavior.
	for sample := 0; sample < 96; sample++ {
		p.Player.Char.Race, p.Player.Char.Skills = domain.Warlock, domain.Learned{10401: 20}
		if sample%2 == 1 {
			// Native passive with a required weapon type exercises removal of
			// an old weapon before replacement qualification.
			p.Player.Char.Race, p.Player.Char.Skills = domain.Race(2), domain.Learned{10202: 5}
		}
		hard := p.Player.Char.Skills.Clone()
		p.Player.Worn = domain.NewEquipSet()
		for _, slot := range []domain.EquipSlot{domain.SlotHead, domain.SlotWeapon, domain.SlotShield, domain.SlotTwoHand} {
			requirementResolutionItem(s, p, slot, int32(rng.Intn(35)), int32(rng.Intn(31)-10))
			d := s.items[domain.ItemID(990000+slot)]
			d.Equip.Type = 8
			switch rng.Intn(8) {
			case 0:
				d.Equip.Need.Sex = 1
			case 1:
				d.Equip.Need.Professions = 1 << uint8(domain.Warrior)
			case 2:
				d.Equip.Need.Level = 61
			case 3:
				st := p.Player.Worn.At(slot)
				st.Durability = 0
				st.MaxDurability = 100
				p.Player.Worn.Set(slot, st)
			}
		}
		s.skillRawCache = nil
		want := s.functionalWornResolved(p, nil)
		s.skillRawCache = nil
		ctx := newEquipmentRequirementResolution()
		got := s.functionalWornResolved(p, ctx)
		if !reflect.DeepEqual(requirementResolutionStacks(got), requirementResolutionStacks(want)) {
			t.Fatalf("sample %d worn differs", sample)
		}
		if len(ctx.bases) > 1<<p.Player.Worn.Count() || len(ctx.worn) > 1<<p.Player.Worn.Count() {
			t.Fatalf("subset bound exceeded: %+v", ctx)
		}
		if !reflect.DeepEqual(hard, p.Player.Char.Skills) {
			t.Fatal("permanent hard points mutated")
		}
		for _, slot := range []domain.EquipSlot{domain.SlotHead, domain.SlotWeapon, domain.SlotTwoHand} {
			s.skillRawCache = nil
			wantBase := s.equipmentRequirementBaseResolved(p, p.Player.Worn, slot, nil)
			s.skillRawCache = nil
			gotBase := s.equipmentRequirementBaseFromWorn(p, p.Player.Worn, slot)
			if gotBase != wantBase {
				t.Fatalf("sample %d slot %d base got %+v want %+v", sample, slot, gotBase, wantBase)
			}
		}
	}
}

func TestEquipmentRequirementResolutionNoSelfSupportOrStaleReuse(t *testing.T) {
	s, p := requirementResolutionFixture(t)
	requirementResolutionItem(s, p, domain.SlotHead, 30, 20)
	requirementResolutionItem(s, p, domain.SlotRing, 30, 20)
	if got := s.functionalWorn(p).Count(); got != 0 {
		t.Fatalf("mutual/self support activated %d", got)
	}
	// Later-slot independent support must allow a second pass to activate head.
	requirementResolutionItem(s, p, domain.SlotRing, 0, 20)
	if got := s.functionalWorn(p).Count(); got != 2 {
		t.Fatalf("legal dependency activated %d", got)
	}
	// Same slot count, changed stack: no memo survives the call.
	requirementResolutionItem(s, p, domain.SlotRing, 0, -10)
	if got := s.functionalWorn(p).Count(); got != 1 {
		t.Fatalf("stale support activated %d", got)
	}
	p.Player.Char.Base.STR = 40
	if got := s.functionalWorn(p).Count(); got != 2 {
		t.Fatalf("changed base activated %d", got)
	}
}

func TestEquipmentRequirementResolutionSoftRanksRemainSeparate(t *testing.T) {
	s, p := requirementResolutionFixture(t)
	// Real unique single-skill +2; the item must first pass its own requirement.
	d := domain.ItemDef{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: 2018, Equip: &domain.EquipDef{Slot: int32(domain.SlotHead), Durable: 100}}
	s.items[d.ID] = d
	st := domain.NewStack(d, 1)
	st.UID = 2018
	st.Endgame = domain.EndgameItemState{UniqueID: 2018, UniqueRollBP: 8000, UniqueContentVersion: domain.UniqueContentVersion}
	p.Player.Worn.Set(domain.SlotHead, st)
	for _, need := range []int32{0, 1000, 0} {
		d.Equip.Need.Base.STR = need
		// Definitions are immutable in production; clear the pre-existing unrelated
		// caches because this test deliberately edits the source definition.
		s.invalidateCombatAttributes(p)
		want := s.rawSkillLevelsResolved(p, nil)
		s.skillRawCache = nil
		got := s.rawSkillLevels(p)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("soft ranks got %v want %v", got, want)
		}
		expected := int32(22)
		if need > 0 {
			expected = 20
		}
		if got[10401] != expected || p.Player.Char.Skills[10401] != 20 {
			t.Fatalf("eligibility/hard points: %v hard %v", got, p.Player.Char.Skills)
		}
	}
}

func TestEquipmentRequirementResolutionSubsetBound(t *testing.T) {
	s, p := requirementResolutionFixture(t)
	for _, slot := range []domain.EquipSlot{1, 2, 3, 4, 6, 7, 8, 9, 10} {
		requirementResolutionItem(s, p, slot, 0, 1)
	}
	ctx := newEquipmentRequirementResolution()
	if got := s.functionalWornResolved(p, ctx).Count(); got != 9 {
		t.Fatal(got)
	}
	// Independent gear generates only sorted prefixes, not exponential subtrees.
	if len(ctx.bases) > 9 || len(ctx.worn) > 10 || ctx.baseHits == 0 {
		t.Fatalf("prefix bound: bases=%d worn=%d hits=%d", len(ctx.bases), len(ctx.worn), ctx.baseHits)
	}
	t.Logf("9 slots: base evaluations=%d, worn evaluations=%d, base cache hits=%d", len(ctx.bases), len(ctx.worn), ctx.baseHits)
}

func BenchmarkEquipmentRequirementResolution(b *testing.B) {
	for _, n := range []int{4, 9} {
		name := "nine"
		if n == 4 {
			name = "four"
		}
		b.Run(name, func(b *testing.B) {
			s, p := requirementResolutionFixture(b)
			slots := []domain.EquipSlot{1, 2, 3, 4, 6, 7, 8, 9, 10}
			for _, slot := range slots[:n] {
				requirementResolutionItem(s, p, slot, 0, 1)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				s.skillRawCache = nil
				s.functionalWorn(p)
			}
		})
	}
	b.Run("legacy_four", func(b *testing.B) {
		s, p := requirementResolutionFixture(b)
		for _, slot := range []domain.EquipSlot{1, 2, 3, 4} {
			requirementResolutionItem(s, p, slot, 0, 1)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			s.skillRawCache = nil
			s.functionalWornResolved(p, nil)
		}
	})
}

func TestEquipmentRequirementResolutionStatusLifetime(t *testing.T) {
	s, p := requirementResolutionFixture(t)
	requirementResolutionItem(s, p, domain.SlotHead, 40, 0)
	p.Status = domain.NewStatusSet()
	if s.functionalWorn(p).Count() != 0 {
		t.Fatal("unqualified gear active")
	}
	p.Status.Apply(domain.StatusDef{ID: 999991, Level: 1, DurationSec: 60, Affixes: []domain.Affix{{Attr: domain.AttrSTR, Value: 30}}}, 0, p.ID)
	if s.functionalWorn(p).Count() != 1 {
		t.Fatal("status support ignored")
	}
	p.Status.RemoveWhere(func(d domain.StatusDef) bool { return d.ID == 999991 })
	if s.functionalWorn(p).Count() != 0 {
		t.Fatal("expired status support cached")
	}
}

func BenchmarkEquipmentRequirementResolutionLegacyNine(b *testing.B) {
	s, p := requirementResolutionFixture(b)
	for _, slot := range []domain.EquipSlot{1, 2, 3, 4, 6, 7, 8, 9, 10} {
		requirementResolutionItem(s, p, slot, 0, 1)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.skillRawCache = nil
		s.functionalWornResolved(p, nil)
	}
}
