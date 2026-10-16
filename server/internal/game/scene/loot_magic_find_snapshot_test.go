package scene

import (
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"
)

// Resolve eligibility afresh so the oracle does not warm or reuse the snapshot
// whose invalidation the test is checking.
func uncachedMagicFindV5(s *Scene, p *entity.Entity) int64 {
	if p == nil || p.Player == nil || p.Player.Char == nil {
		return 0
	}
	var total int64
	for _, active := range domain.ActiveEquipmentEffects(s.functionalWornResolved(p, newEquipmentRequirementResolution()), s.itemDef, p.Player.Char) {
		if active.Effect.Kind == "magic_find_pct" {
			total += int64(active.Effect.Value)
		}
	}
	return max(int64(0), total)
}

func TestMagicFindSnapshotWearAndEligibilityParity(t *testing.T) {
	s, _ := lootScene(t, nil)
	join(t, s, 1, 100, "killer", 100, 100)
	p := s.players[1]
	slot, st, mf := magicFindGearV5(t, s, p)
	s.items[st.Item].Equip.Need.Level = 2
	p.Player.Char.Level = 2
	check := func(want int64) {
		t.Helper()
		if got := s.magicFindV5(p); got != want {
			t.Fatalf("snapshot MF = %d, want %d", got, want)
		}
		if got := uncachedMagicFindV5(s, p); got != want {
			t.Fatalf("uncached MF = %d, want %d", got, want)
		}
	}
	check(mf)
	state := s.combatAttributeState(p)
	for _, durability := range []int32{99, 1} {
		st.Durability = durability
		st.DurabilityWearRaw = 123
		p.Player.Worn.Set(slot, st)
		check(mf)
		if s.combatAttributeState(p) != state {
			t.Fatal("positive wear rebuilt the eligibility snapshot")
		}
	}
	if got := s.functionalWorn(p).At(slot); got.Durability != 1 || got.DurabilityWearRaw != 123 {
		t.Fatal("ordinary equipment views lost live wear values")
	}
	for _, durability := range []int32{0, 100} {
		st.Durability = durability
		p.Player.Worn.Set(slot, st)
		want := mf
		if durability == 0 {
			want = 0
		}
		check(want)
		if next := s.combatAttributeState(p); next == state {
			t.Fatal("break or repair did not rebuild eligibility")
		} else {
			state = next
		}
	}
	for _, level := range []int32{1, 2, 100, 1, 2} {
		p.Player.Char.Level = level
		want := mf
		if level < 2 {
			want = 0
		}
		check(want)
	}
	p.Player.Worn.Set(slot, domain.Stack{})
	check(0)
	p.Player.Worn.Set(slot, st)
	check(mf)
	// Replacing the item with a plain instance also invalidates typed effects.
	plain := domain.NewStack(s.items[st.Item], 1)
	p.Player.Worn.Set(slot, plain)
	check(0)
	p.Player.Worn.Set(slot, st)
	check(mf)
}

func TestMagicFindSnapshotPreviewPreservesOwner(t *testing.T) {
	s, _ := lootScene(t, nil)
	join(t, s, 1, 100, "killer", 100, 100)
	p := s.players[1]
	slot, _, mf := magicFindGearV5(t, s, p)
	if got := s.magicFindV5(p); got != mf {
		t.Fatalf("owner MF = %d, want %d", got, mf)
	}
	state := s.combatAttributeState(p)
	preview := *p
	player := *p.Player
	preview.Player = &player
	player.Worn = p.Player.Worn.Clone()
	player.Worn.Set(slot, domain.Stack{})
	if s.combatAttributeState(&preview) != nil {
		t.Fatal("preview admitted to owner cache")
	}
	if got, want := s.magicFindV5(&preview), uncachedMagicFindV5(s, &preview); got != 0 || got != want {
		t.Fatalf("preview MF = %d, want %d", got, want)
	}
	if s.combatAttributeState(p) != state || s.magicFindV5(p) != mf {
		t.Fatal("preview changed owner's MF snapshot")
	}
	for _, absent := range []*entity.Entity{nil, {}, {Player: &entity.Player{}}} {
		if got := s.magicFindV5(absent); got != 0 {
			t.Fatalf("absent character MF = %d", got)
		}
	}
}

// Only the MF payload changes here. This checks that the optimization still
// reads typed effects, not support for hot-reloading eligibility/catalog rules.
func TestMagicFindSnapshotReadsCurrentTypedEffect(t *testing.T) {
	s, _ := lootScene(t, nil)
	join(t, s, 1, 100, "killer", 100, 100)
	p := s.players[1]
	_, st, mf := magicFindGearV5(t, s, p)
	if got := s.magicFindV5(p); got != mf {
		t.Fatalf("initial MF = %d, want %d", got, mf)
	}
	state := s.combatAttributeState(p)
	data := catalogfixture.EndgameCatalogData()
	changed := false
	for i := range data.Uniques {
		u := &data.Uniques[i]
		if u.Version != st.Endgame.UniqueContentVersion || u.Definition.ID != st.Endgame.UniqueID {
			continue
		}
		for j := range u.Definition.TypedEffects {
			effect := &u.Definition.TypedEffects[j]
			if effect.Kind == "magic_find_pct" {
				effect.Value += 7
				changed = true
				break
			}
		}
	}
	if !changed {
		t.Fatal("fixture has no matching MF effect")
	}
	catalog, err := domain.NewEndgameCatalog(data)
	if err != nil {
		t.Fatal(err)
	}
	def := s.items[st.Item]
	def.EndgameCatalog = catalog
	s.items[st.Item] = def
	if got, want := s.magicFindV5(p), mf+7; got != want {
		t.Fatalf("current MF = %d, want %d", got, want)
	}
	if got := uncachedMagicFindV5(s, p); got != mf+7 {
		t.Fatalf("uncached MF = %d, want %d", got, mf+7)
	}
	if s.combatAttributeState(p) != state {
		t.Fatal("effect-only replacement rebuilt eligibility")
	}
}
