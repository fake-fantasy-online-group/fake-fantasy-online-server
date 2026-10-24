package scene

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"
)

func requireGroundPresentationWire(t *testing.T, observer domain.EntityID, ev event.EntitySpawned, quality uint8, identity int32, fresh bool, token int64) {
	t.Helper()
	if ev.Ground == nil || ev.Ground.Quality != quality || ev.Ground.UniqueID != identity || ev.Ground.FreshDrop != fresh || ev.Ground.DropToken != token || token <= 0 {
		t.Fatalf("ground presentation: %+v; want quality=%d identity=%d fresh=%t token=%d", ev.Ground, quality, identity, fresh, token)
	}
	legacy := protocol.Encode(observer, ev)[0]
	wire := protocol.GroundSpawnPresentation(observer, ev)
	if len(wire) != len(legacy)+14 || !bytes.Equal(wire[:len(legacy)], legacy) || legacy[len(legacy)-1] != quality {
		t.Fatal("ground presentation changed the legacy prefix or quality")
	}
	tail := wire[len(legacy):]
	flags := byte(0)
	if fresh {
		flags = 1
	}
	if tail[0] != 1 || tail[1] != flags || int32(binary.LittleEndian.Uint32(tail[2:6])) != identity || int64(binary.LittleEndian.Uint64(tail[6:14])) != token {
		t.Fatalf("wrong ground presentation wire tail: %x", tail)
	}
}

func groundPresentationEvent(t *testing.T, events []event.Event, id domain.EntityID) event.EntitySpawned {
	t.Helper()
	for _, ev := range events {
		if spawned, ok := ev.(event.EntitySpawned); ok && spawned.ID == id && spawned.Ground != nil {
			return spawned
		}
	}
	t.Fatalf("missing ground spawn for entity %d", id)
	return event.EntitySpawned{}
}

func TestLegendaryNaturalGroundPresentationWire(t *testing.T) {
	fixture, def := legendaryDropFixture(t)
	s, mid := lootScene(t, []domain.DropEntry{{Item: def.ID, RatePct: 100, RollAffixes: true}})
	s.items[def.ID], s.equipmentRoll = def, fixture.equipmentRoll
	rules := *lootRulesFixture()
	rules.UniqueBasePPB, rules.UniqueMFBoost = domain.LootProbabilityScaleV5, 0
	var err error
	s.lootRules, err = domain.NewLootRuleTable(rules.Version, []domain.LootRuleSet{rules}, balanceItemsFixture())
	if err != nil {
		t.Fatal(err)
	}
	sink := join(t, s, 1, 100, "甲", 100, 100)
	s.step()
	sink.take()
	killMonster(t, s, mid)
	if len(s.ground) != 1 {
		t.Fatalf("natural kill created %d drops, want 1", len(s.ground))
	}
	for id, drop := range s.ground {
		st := drop.Drop.Stack
		if st.Legendary.PowerID == "" || st.Legendary.Validate() != nil || st.Endgame.UniqueID != 0 {
			t.Fatalf("natural kill did not produce an independent legendary: %+v", st)
		}
		ev := groundPresentationEvent(t, sink.take(), id)
		requireGroundPresentationWire(t, 1, ev, 5, int32(def.ID), true, drop.Drop.PresentationToken)
		if drop.Drop.Stack != st {
			t.Fatal("presentation mutated the naturally generated instance")
		}
	}
}

func TestLegendaryGroundPresentationLifetimesAndControls(t *testing.T) {
	fixture, def := legendaryDropFixture(t)
	legendary := domain.NewStack(def, 1)
	if !fixture.equipmentRoll.ApplyLegendary(&legendary, def, func(int) int { return 0 }) {
		t.Fatal("failed to generate legendary fixture")
	}
	old := domain.UniqueCatalog(catalogfixture.EndgameCatalog())[0]
	oldDef := domain.ItemDef{ID: old.BaseID, Name: "旧暗金", EndgameCatalog: catalogfixture.EndgameCatalog(), Equip: &domain.EquipDef{}}
	unique := domain.Stack{UID: 999, Item: old.BaseID, Count: 1, Endgame: domain.EndgameItemState{UniqueID: old.ID, UniqueRollBP: 10000, UniqueContentVersion: domain.UniqueContentVersion}}
	yellow := domain.NewStack(def, 1)
	yellow.RolledAffixCount = 5
	for _, tc := range []struct {
		name     string
		def      domain.ItemDef
		stack    domain.Stack
		quality  uint8
		identity int32
	}{
		{"new legendary", def, legendary, 5, int32(def.ID)},
		{"legacy unique", oldDef, unique, 4, old.ID},
		{"ordinary yellow", def, yellow, 4, 0},
	} {
		for _, mapID := range []int32{14, 920, 20001} {
			t.Run(fmt.Sprintf("%s/map_%d", tc.name, mapID), func(t *testing.T) {
				s := New(Config{ID: domain.SceneID{MapID: mapID}, EndgameCatalog: catalogfixture.EndgameCatalog(), Alloc: domain.NewEntityAlloc(), Items: map[domain.ItemID]domain.ItemDef{tc.def.ID: tc.def}})
				sink := join(t, s, 1, 100, "甲", 100, 100)
				s.step()
				sink.take()
				s.spawnGeneratedLoot(tc.def, tc.stack, s.players[1].Pos, 1)
				s.flush()
				var drop *entity.Entity
				for _, e := range s.ground {
					drop = e
				}
				if drop == nil {
					t.Fatal("generated loot not placed on ground")
				}
				token := drop.Drop.PresentationToken
				requireGroundPresentationWire(t, 1, groundPresentationEvent(t, sink.take(), drop.ID), tc.quality, tc.identity, true, token)

				// A real late observer receives the existing beam, never a new sound.
				late := join(t, s, 2, 200, "乙", 110, 100)
				s.step()
				requireGroundPresentationWire(t, 2, groundPresentationEvent(t, late.take(), drop.ID), tc.quality, tc.identity, false, token)
				sink.take()

				// Returning carried loot is an actual visibility refresh, not a new lifetime.
				s.removeLoot(drop.ID, event.DespawnPickedUp)
				carrier := &entity.Entity{Pos: drop.Pos, Monster: &entity.Monster{CarriedLoot: []*entity.Entity{drop}}}
				s.releaseMonsterCarriedLoot(carrier)
				s.flush()
				requireGroundPresentationWire(t, 1, groundPresentationEvent(t, sink.take(), drop.ID), tc.quality, tc.identity, false, token)
				if drop.Drop.Stack != tc.stack {
					t.Fatal("refresh changed persisted instance data")
				}

				// Dropping the same inventory UID again gets a separate sound receipt.
				s.removeLoot(drop.ID, event.DespawnPickedUp)
				s.spawnPlayerDrop(tc.def, tc.stack, drop.Pos, 1)
				s.flush()
				for _, e := range s.ground {
					drop = e
				}
				if drop.Drop.PresentationToken <= token {
					t.Fatal("new drop lifetime reused the old sound token")
				}
				requireGroundPresentationWire(t, 1, groundPresentationEvent(t, sink.take(), drop.ID), tc.quality, tc.identity, true, drop.Drop.PresentationToken)
				if drop.Drop.Stack != tc.stack {
					t.Fatal("redrop changed persisted instance data")
				}
			})
		}
	}
}

func TestLegendaryGroundPresentationRejectsMalformedMarkers(t *testing.T) {
	s, def := legendaryDropFixture(t)
	st := domain.NewStack(def, 1)
	if !s.equipmentRoll.ApplyLegendary(&st, def, func(int) int { return 0 }) {
		t.Fatal("failed legendary fixture")
	}
	for name, change := range map[string]func(*domain.Stack){
		"missing UID":    func(st *domain.Stack) { st.UID = 0 },
		"negative UID":   func(st *domain.Stack) { st.UID = -1 },
		"multiple items": func(st *domain.Stack) { st.Count = 2 },
		"missing item":   func(st *domain.Stack) { st.Item = 0 },
		"negative item":  func(st *domain.Stack) { st.Item = -1 },
		"unknown power":  func(st *domain.Stack) { st.Legendary.PowerID = "unknown" },
		"missing power":  func(st *domain.Stack) { st.Legendary.PowerID = "" },
		"missing rolls":  func(st *domain.Stack) { st.Legendary = domain.LegendaryInstance{PowerID: st.Legendary.PowerID} },
		"out of bounds":  func(st *domain.Stack) { st.Legendary.Affixes[0].Affix.Value = st.Legendary.Bounds[0].Max + 1 },
	} {
		t.Run(name, func(t *testing.T) {
			bad := st
			change(&bad)
			before := bad
			if got := s.groundUniqueID(bad); got != 0 {
				t.Fatalf("malformed legendary gained a beam: %d", got)
			}
			if bad != before {
				t.Fatal("presentation mutated malformed instance")
			}
		})
	}
}
