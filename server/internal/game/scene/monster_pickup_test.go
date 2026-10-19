package scene

import (
	"reflect"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
)

func monsterPickupFixture(t *testing.T) (*Scene, *entity.Entity, *entity.Entity) {
	t.Helper()
	s, mid := lootScene(t, nil)
	d := 必掉的怪
	d.Pickup = true
	s.defs = spawn.MapDefs{d.ID: d}
	m := s.monsters[mid]
	m.Monster.Aggressive = false
	item := 掉落树枝
	item.CanTrade = true
	s.items[item.ID] = item
	st := domain.NewStack(item, 3)
	s.spawnGeneratedLoot(item, st, m.Pos, 0)
	var drop *entity.Entity
	for _, e := range s.ground {
		drop = e
	}
	return s, m, drop
}

func TestMonsterPickupExactInstanceAndDeathOnce(t *testing.T) {
	s, m, e := monsterPickupFixture(t)
	// Include equipment-specific instance fields; the transfer must preserve them
	// even when the template is a stackable fixture.
	e.Drop.Count = 1
	e.Drop.Stack.Count = 1
	e.Drop.Stack.UID = 777
	e.Drop.Stack.Durability = 12
	e.Drop.Stack.MaxDurability = 71
	e.Drop.Stack.SocketCount = 2
	e.Drop.Stack.Sockets[0] = 201
	e.Drop.Stack.SocketUIDs[0] = 901
	before := *e.Drop
	originalID := e.ID
	if !s.tickMonsterPickup(m) || len(s.ground) != 0 || len(m.Monster.CarriedLoot) != 1 {
		t.Fatal("did not take ownership")
	}
	if s.entities[e.ID] != nil {
		t.Fatal("picked item still player-pickable")
	}
	join(t, s, 1, 100, "甲", 120, 100)
	killMonster(t, s, m.ID)
	if s.ground[originalID] != e || !reflect.DeepEqual(*e.Drop, before) {
		t.Fatal("death lost or rerolled carried instance")
	}
	s.releaseMonsterCarriedLoot(m)
	if len(s.ground) != 1 || len(m.Monster.CarriedLoot) != 0 {
		t.Fatal("repeated release duplicated loot")
	}
	s.onPickUp(PickUp{ID: 1, Target: originalID})
	if len(s.ground) != 0 || s.players[1].Player.Bag.CountOf(e.Drop.Item) != 1 {
		t.Fatal("returned drop not collectable")
	}
}

func TestMonsterPickupActorRaces(t *testing.T) {
	for _, playerFirst := range []bool{true, false} {
		t.Run(map[bool]string{true: "player_first", false: "monster_first"}[playerFirst], func(t *testing.T) {
			s, m, e := monsterPickupFixture(t)
			join(t, s, 1, 100, "甲", 120, 100)
			join(t, s, 2, 101, "乙", 120, 100)
			if playerFirst {
				s.onPickUp(PickUp{ID: 1, Target: e.ID})
			}
			s.tickMonsterPickup(m)
			s.onPickUp(PickUp{ID: 2, Target: e.ID})
			total := s.players[1].Player.Bag.CountOf(e.Drop.Item) + s.players[2].Player.Bag.CountOf(e.Drop.Item)
			for _, c := range m.Monster.CarriedLoot {
				total += c.Drop.Count
			}
			if total != 3 || len(s.ground) != 0 {
				t.Fatalf("ownership duplication/loss: %d", total)
			}
		})
	}
}

func TestMonsterPickupProtectedAndUnsupported(t *testing.T) {
	cases := map[string]func(*Scene, *entity.Entity, *entity.Entity){
		"owner protection": func(s *Scene, m, e *entity.Entity) { e.Drop.Owner = 1; e.Drop.OwnerUntil = s.tick + 100 },
		"bound":            func(s *Scene, m, e *entity.Entity) { e.Drop.Stack.Bound = true },
		"locked":           func(s *Scene, m, e *entity.Entity) { e.Drop.Stack.Locked = true },
		"nontrade":         func(s *Scene, m, e *entity.Entity) { d := s.items[e.Drop.Item]; d.CanTrade = false; s.items[d.ID] = d },
		"unknown":          func(s *Scene, m, e *entity.Entity) { delete(s.items, e.Drop.Item) },
		"expired":          func(s *Scene, m, e *entity.Entity) { e.Drop.ExpireAt = s.tick },
		"quest": func(s *Scene, m, e *entity.Entity) {
			s.quests = domain.QuestTable{1: {Steps: []domain.QuestStep{{Collect: []domain.QuestItemGoal{{Item: e.Drop.Item, Sources: []domain.MonsterID{1001}}}}}}}
		},
		"combat":   func(s *Scene, m, e *entity.Entity) { m.Monster.Target = 1 },
		"summon":   func(s *Scene, m, e *entity.Entity) { m.Monster.Summoner = 1 },
		"over60":   func(s *Scene, m, e *entity.Entity) { m.Level = 61 },
		"disabled": func(s *Scene, m, e *entity.Entity) { d := 必掉的怪; s.defs = spawn.MapDefs{d.ID: d} },
		"rift":     func(s *Scene, m, e *entity.Entity) { s.rift = &endgameRiftState{} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			s, m, e := monsterPickupFixture(t)
			mutate(s, m, e)
			if s.tickMonsterPickup(m) || len(m.Monster.CarriedLoot) != 0 || s.ground[e.ID] != e {
				t.Fatal("ineligible loot stolen")
			}
		})
	}
}

func TestMonsterPickupExpiryPreserved(t *testing.T) {
	for _, carryAtExpiry := range []bool{true, false} {
		t.Run(map[bool]string{true: "held", false: "released"}[carryAtExpiry], func(t *testing.T) {
			s, m, e := monsterPickupFixture(t)
			expiry := e.Drop.ExpireAt
			s.tickMonsterPickup(m)
			if !carryAtExpiry {
				s.releaseMonsterCarriedLoot(m)
			}
			s.tick = expiry
			s.timer.advance(s.tick)
			s.releaseMonsterCarriedLoot(m)
			if len(s.ground) != 0 || len(m.Monster.CarriedLoot) != 0 {
				t.Fatal("expired loot resurrected")
			}
		})
	}
}

func TestMonsterPickupNonDeathRemovalAndShutdown(t *testing.T) {
	for _, kind := range []string{"capture", "despawn", "shutdown"} {
		t.Run(kind, func(t *testing.T) {
			s, m, e := monsterPickupFixture(t)
			s.tickMonsterPickup(m)
			switch kind {
			case "capture":
				s.removeMonsterCaptured(m)
			case "despawn":
				s.despawnMonster(m)
			case "shutdown":
				s.shutdown("test")
			}
			if len(m.Monster.CarriedLoot) != 0 {
				t.Fatal("removed monster retains item ownership")
			}
			if kind == "shutdown" {
				if len(s.ground) != 0 {
					t.Fatal("shutdown left world loot")
				}
			} else if s.ground[e.ID] != e {
				t.Fatal("nondeath removal lost loot")
			}
		})
	}
}

func TestMonsterPickupApproachAndCombatInterrupt(t *testing.T) {
	s, m, e := monsterPickupFixture(t)
	at := e.Pos
	at.X += 120
	s.aoi.Move(e, at)
	old := m.Pos
	if !s.tickMonsterPickup(m) || m.Pos == old || len(m.Monster.CarriedLoot) != 0 {
		t.Fatal("failed to approach")
	}
	m.Monster.Target = 9
	old = m.Pos
	if s.tickMonsterPickup(m) || m.Pos != old || m.Monster.PickupTarget != 0 {
		t.Fatal("pickup interrupted combat")
	}
	m.Monster.Target = 0
	m.Monster.NextPickupScan = 0
	for i := 0; i < 20 && len(m.Monster.CarriedLoot) == 0; i++ {
		s.tick++
		s.tickMonsterPickup(m)
	}
	if len(m.Monster.CarriedLoot) != 1 {
		t.Fatal("did not resume pickup after combat")
	}
}

func TestMonsterPickupCompetingMonstersCapacityAndCooldown(t *testing.T) {
	s, m, e := monsterPickupFixture(t)
	copyEntity := *m
	copyMonster := *m.Monster
	copyEntity.Monster = &copyMonster
	copyEntity.ID = s.alloc.Monster()
	other := &copyEntity
	s.entities[other.ID] = other
	s.monsters[other.ID] = other
	s.aoi.Enter(other)
	s.tickMonsterPickup(m)
	s.tickMonsterPickup(other)
	if len(m.Monster.CarriedLoot)+len(other.Monster.CarriedLoot) != 1 {
		t.Fatal("competing monsters duplicated drop")
	}
	item := s.items[e.Drop.Item]
	for i := 0; i < 10; i++ {
		s.spawnGeneratedLoot(item, domain.NewStack(item, 1), m.Pos, 0)
	}
	if s.tickMonsterPickup(m) {
		t.Fatal("scan cooldown ignored")
	}
	for i := 0; i < 20; i++ {
		s.tick += 20
		s.tickMonsterPickup(m)
	}
	beforePos := m.Pos
	if s.tickMonsterPickup(m) || m.Pos != beforePos {
		t.Fatal("full carrier kept moving for loot")
	}
	if len(m.Monster.CarriedLoot) != monsterPickupCapacity || len(s.ground) != 3 {
		t.Fatal("capacity does not preserve remaining ground")
	}
}

func TestMonsterPickupCollisionDoesNotOverwriteOrExpireOtherItem(t *testing.T) {
	s, m, e := monsterPickupFixture(t)
	originalID := e.ID
	expiry := e.Drop.ExpireAt
	s.tickMonsterPickup(m)
	// Simulate allocator wrap/reuse while an earlier object is held.
	replacement := &entity.Entity{ID: originalID, Kind: domain.KindDrop, Pos: m.Pos, Drop: &entity.Drop{Item: e.Drop.Item, Count: 1, ExpireAt: expiry + 200}}
	s.entities[originalID] = replacement
	s.ground[originalID] = replacement
	s.aoi.Enter(replacement)
	s.releaseMonsterCarriedLoot(m)
	if e.ID == originalID || s.ground[originalID] != replacement || len(s.ground) != 2 {
		t.Fatal("ID collision overwritten")
	}
	s.tick = expiry
	s.timer.advance(s.tick)
	if s.ground[originalID] != replacement || len(s.ground) != 1 {
		t.Fatal("old expiry removed newer item or retained carried item")
	}
}

func TestMonsterPickupProtectedExpiryAndWall(t *testing.T) {
	s, m, e := monsterPickupFixture(t)
	e.Drop.Owner = 3
	e.Drop.OwnerUntil = 10
	if s.tickMonsterPickup(m) {
		t.Fatal("protected pickup")
	}
	s.tick = 20
	s.walkable = func(domain.Pos) bool { return false }
	if s.tickMonsterPickup(m) {
		t.Fatal("pickup through wall")
	}
	s.walkable = nil
	s.tick = 40
	if !s.tickMonsterPickup(m) {
		t.Fatal("public item not picked after protection ended")
	}
}

func TestMonsterPickupSceneAIIntegration(t *testing.T) {
	s, m, _ := monsterPickupFixture(t)
	s.stepAI()
	if len(m.Monster.CarriedLoot) != 1 {
		t.Fatal("scene AI did not invoke pickup")
	}
}

func TestMonsterPickupOrdinaryDungeonLifecycle(t *testing.T) {
	s, m, e := monsterPickupFixture(t)
	s.dungeon = &domain.DungeonDef{Solo: true}
	before := *e.Drop
	if !s.tickMonsterPickup(m) || len(m.Monster.CarriedLoot) != 1 {
		t.Fatal("eligible ordinary dungeon monster must pick up public loot")
	}
	join(t, s, 1, 100, "甲", 120, 100)
	killMonster(t, s, m.ID)
	if s.ground[e.ID] != e || !reflect.DeepEqual(*e.Drop, before) {
		t.Fatal("ordinary dungeon death must preserve item and original expiry")
	}
	s.onPickUp(PickUp{ID: 1, Target: e.ID})
	if len(s.ground) != 0 || s.players[1].Player.Bag.CountOf(e.Drop.Item) != before.Count {
		t.Fatal("ordinary dungeon returned loot must remain collectable")
	}
}
