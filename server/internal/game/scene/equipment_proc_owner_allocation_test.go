package scene

import (
	"fmt"
	"reflect"
	"slices"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func legacyEquipmentRootOwners(hits []equipmentHit) []domain.EntityID {
	owners := map[domain.EntityID]bool{}
	for _, h := range hits {
		owners[h.src] = true
		owners[h.dst] = true
	}
	return sortedEquipmentEntities(owners)
}

func TestEquipmentRootOwnersMatchesMapOrder(t *testing.T) {
	for _, n := range []int{0, 1, 2, 15, 16, 17, 32, 64, 256} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			hits := make([]equipmentHit, n)
			for i := range hits {
				hits[i] = equipmentHit{src: domain.EntityID((n - i) % 7), dst: domain.EntityID((i * 13) % 43)}
			}
			before := slices.Clone(hits)
			var buf [32]domain.EntityID
			got := equipmentRootOwners(hits, buf[:0])
			want := legacyEquipmentRootOwners(hits)
			if !slices.Equal(got, want) {
				t.Fatalf("owners=%v want=%v", got, want)
			}
			if !reflect.DeepEqual(hits, before) && n > 0 {
				t.Fatal("mutated hit order or contents")
			}
			saved := slices.Clone(got)
			equipmentRootOwners([]equipmentHit{{src: 91, dst: 92}}, nil)
			if !slices.Equal(got, saved) {
				t.Fatal("owner storage shared across roots")
			}
		})
	}
	// More than 32 distinct endpoints exercises growth beyond the stack buffer.
	hits := make([]equipmentHit, 33)
	for i := range hits {
		hits[i] = equipmentHit{src: domain.EntityID(i * 2), dst: domain.EntityID(i*2 + 1)}
	}
	var buf [32]domain.EntityID
	if got, want := equipmentRootOwners(hits, buf[:0]), legacyEquipmentRootOwners(hits); !slices.Equal(got, want) {
		t.Fatal(got, want)
	}
}

func TestEquipmentRootOwnersRetainsLiveLookup(t *testing.T) {
	s := newCombatScene(t)
	join(t, s, 1, 100, "alive", 100, 100)
	join(t, s, 2, 200, "dead", 100, 100)
	s.players[2].HP = 0
	putMonster(s, 3, 100, domain.Stats{}, 110, 100)
	rt := s.equipmentState()
	clear(rt.cache)
	root := &equipmentRoot{hits: []equipmentHit{{src: 3, dst: 2}, {src: 4, dst: 1}, {src: 0, dst: 1}}}
	s.executeEquipmentRoot(root)
	if _, ok := rt.cache[1]; !ok {
		t.Fatal("live player was not evaluated")
	}
	for _, id := range []domain.EntityID{0, 2, 3, 4} {
		if _, ok := rt.cache[id]; ok {
			t.Fatalf("non-live player %d evaluated", id)
		}
	}
	s.players[2].HP = 100
	s.executeEquipmentRoot(root)
	if _, ok := rt.cache[2]; !ok {
		t.Fatal("revived player was not read live")
	}
	s.executeEquipmentRoot(nil)
	s.executeEquipmentRoot(&equipmentRoot{})
}

func BenchmarkEquipmentRootOwnerSettlement(b *testing.B) {
	for _, n := range []int{1, 10, 64, 256} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			s, p, _, _ := newCombatAttributeBatch(b)
			root := &equipmentRoot{id: 1, src: p.ID}
			for i := 0; i < n; i++ {
				root.hits = append(root.hits, equipmentHit{src: p.ID, dst: domain.EntityID(10000 + i), ev: event.DamageDealt{Amount: 10}})
			}
			s.equipmentEffects(p)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				s.executeEquipmentRoot(root)
			}
		})
	}
}
