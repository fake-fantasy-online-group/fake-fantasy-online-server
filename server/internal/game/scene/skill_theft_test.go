package scene

import (
	"math/rand"
	"reflect"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
)

func theftTestScene(t *testing.T, chance int32) (*Scene, *entity.Entity, *entity.Entity, *fakeSink) {
	t.Helper()
	s := newTestScene(t, nil)
	s.alloc = domain.NewEntityAlloc()
	s.items = map[domain.ItemID]domain.ItemDef{掉落树枝.ID: 掉落树枝, 掉落牛肉面.ID: 掉落牛肉面}
	s.lootTable = map[domain.MonsterID][]domain.DropEntry{338: {{Item: 掉落树枝.ID, RatePct: 0.00045}}}
	s.skills = domain.SkillTable{{ID: 10212, Level: 1}: {
		ID: 10212, Level: 1, Name: "偷窃术", Kind: domain.SkillSingle, TargetEnemy: true,
		Dist: 75, MPCost: 69, CooldownMS: 5000,
		Effects: []domain.SkillEffect{{Kind: domain.EffectStealItem, ChanceBP: chance, ChanceExplicit: true, TheftMaxSuccesses: 1}},
	}}
	sink := join(t, s, 1, 101, "thief", 100, 100)
	src := s.players[1]
	src.Player.Char.Race, src.Player.Char.Level = domain.Assassin, 100
	src.Player.Char.SkillAllocationVersion = domain.SkillFormulaVersion
	src.Player.Char.Skills = domain.Learned{10212: 1}
	src.MP, src.MaxMP = 10000, 10000
	dst := putMonster(s, 900, 100000, domain.Stats{}, 120, 100)
	s.defs = spawn.MapDefs{338: {ID: 338, Kind: domain.MonsterNormal, Level: 1}}
	s.flush()
	sink.take()
	return s, src, dst, sink
}

func castTheft(t *testing.T, s *Scene, src, dst *entity.Entity) {
	t.Helper()
	s.onUseSkill(UseSkill{ID: src.ID, Target: dst.ID, Skill: 10212})
	s.stepCasts()
	s.flush()
}

func onlyTheftDrop(t *testing.T, s *Scene) *entity.Entity {
	t.Helper()
	if len(s.ground) != 1 {
		t.Fatalf("got %d ground drops, want exactly one", len(s.ground))
	}
	for _, drop := range s.ground {
		return drop
	}
	return nil
}

func TestMonsterTheftRealEntrySingleGateAndLifecycle(t *testing.T) {
	s, src, dst, sink := theftTestScene(t, 10000)
	hp, mp := dst.HP, src.MP
	castTheft(t, s, src, dst)
	drop := onlyTheftDrop(t, s)
	if dst.HP != hp || src.MP != mp-69 || dst.Monster.TheftSuccesses != 1 || drop.Drop.Count != 1 || drop.Drop.Owner != src.ID ||
		drop.Drop.Item != 掉落树枝.ID || drop.Drop.Stack.Count != 1 || drop.Pos != dst.Pos {
		t.Fatalf("wrong theft settlement: HP%d MP%d count%d drop%+v", dst.HP, src.MP, dst.Monster.TheftSuccesses, drop.Drop)
	}
	if !reflect.DeepEqual(s.lootTable[338], []domain.DropEntry{{Item: 掉落树枝.ID, RatePct: 0.00045}}) {
		t.Fatal("theft mutated normal kill pool")
	}
	if countOf[event.DamageDealt](sink.take()) != 0 {
		t.Fatal("theft retained Youlong damage")
	}
	// Repeated packets during cooldown and after cooldown both leave one item.
	for _, advance := range []domain.Tick{0, 100} {
		s.tick += advance
		mp = src.MP
		castTheft(t, s, src, dst)
		if len(s.ground) != 1 || src.MP != mp {
			t.Fatal("duplicate packet spent mana or granted another item")
		}
	}
	// The same monster is shared by all players, including reconnecting owners.
	s.onLeave(Leave{ID: src.ID})
	join(t, s, 2, 101, "thief-reconnected", 100, 100)
	src = s.players[2]
	src.Player.Char.Skills = domain.Learned{10212: 1}
	src.MP, src.MaxMP = 10000, 10000
	mp = src.MP
	castTheft(t, s, src, dst)
	if len(s.ground) != 1 || src.MP != mp || dst.Monster.TheftSuccesses != 1 {
		t.Fatal("reconnection reset monster theft allowance")
	}
	// A fresh spawned instance of the same template is a new lifecycle.
	fresh := putMonster(s, 901, 100000, domain.Stats{}, 120, 100)
	castTheft(t, s, src, fresh)
	if len(s.ground) != 2 || fresh.Monster.TheftSuccesses != 1 {
		t.Fatal("new monster life did not permit theft")
	}
}

func TestMonsterTheftFailureCanRetryAndDoesNotRollDropChance(t *testing.T) {
	s, src, dst, _ := theftTestScene(t, 5000)
	var seed int64
	for rand.New(rand.NewSource(seed)).Intn(10000) < 5000 {
		seed++
	}
	s.rng = rand.New(rand.NewSource(seed))
	mp := src.MP
	castTheft(t, s, src, dst)
	if len(s.ground) != 0 || dst.Monster.TheftSuccesses != 0 || src.MP != mp-69 || src.Player.SkillReadyAt(10212, s.tick) {
		t.Fatal("failed gate consumed opportunity, created loot, or omitted normal cost/cooldown")
	}
	// Find a succeeding gate whose following draw would fail the tiny original
	// 0.00045% drop chance. A successful theft must still produce its one item.
	seed = 0
	for {
		r := rand.New(rand.NewSource(seed))
		if r.Intn(10000) < 5000 && r.Intn(1_000_000) > 5 {
			break
		}
		seed++
	}
	s.rng = rand.New(rand.NewSource(seed))
	s.tick += 100
	castTheft(t, s, src, dst)
	onlyTheftDrop(t, s)
	if dst.Monster.TheftSuccesses != 1 || src.MP != mp-138 {
		t.Fatal("normal retry failed or mischarged")
	}
}

func TestMonsterTheftAdmissionBoundaries(t *testing.T) {
	for _, mode := range []string{"normal", "elite", "boss", "range-edge", "gather", "prop", "summon", "player", "pet", "dead", "empty", "bad-item", "nil-allocator", "immune", "out-of-range", "wrong-map", "stale-target", "already-stolen"} {
		t.Run(mode, func(t *testing.T) {
			s, src, dst, _ := theftTestScene(t, 10000)
			allowed := mode == "normal" || mode == "elite" || mode == "boss" || mode == "range-edge"
			switch mode {
			case "elite":
				dst.Monster.Kind = domain.MonsterElite
			case "boss":
				dst.Monster.Kind = domain.MonsterBoss
				s.bossFixedDrops = domain.BossFixedDropTable{{MapID: s.id.MapID, Monster: dst.Monster.TypeID}: {Minimum: 2, Pool: []domain.BossFixedDropChoice{{Item: 掉落牛肉面.ID, Weight: 1, RatePct: 100}}}}
			case "range-edge":
				dst.Pos.X = src.Pos.X + 75
			case "gather":
				dst.Monster.Kind = domain.MonsterGather
			case "prop":
				dst.Monster.Kind = domain.MonsterProp
			case "summon":
				dst.Monster.Summoner = src.ID
			case "player":
				dst.Kind = domain.KindPlayer
			case "pet":
				dst.Kind = domain.KindPet
			case "dead":
				dst.HP = 0
			case "empty":
				s.lootTable = nil
			case "bad-item":
				s.items = nil
			case "nil-allocator":
				s.alloc = nil
			case "immune":
				s.defs = spawn.MapDefs{338: {ID: 338, ImmuneSkills: map[domain.SkillID]struct{}{10212: {}}}}
			case "out-of-range":
				dst.Pos.X = src.Pos.X + 76
			case "wrong-map":
				dst.Pos.MapID++
			case "stale-target":
				delete(s.entities, dst.ID)
			case "already-stolen":
				dst.Monster.TheftSuccesses = 1
			}
			mp := src.MP
			castTheft(t, s, src, dst)
			if allowed {
				onlyTheftDrop(t, s)
			} else if len(s.ground) != 0 || src.MP != mp || !src.Player.SkillReadyAt(10212, s.tick) {
				t.Fatal("invalid target admitted, charged or generated loot")
			}
		})
	}
}

func TestMonsterTheftMapReturnKeepsSharedAllowance(t *testing.T) {
	fixture, _, _, _ := theftTestScene(t, 10000)
	router, rig := twoScenes(t)
	first, _ := router.Ensure(domain.SceneID{MapID: 7})
	other, _ := router.Ensure(domain.SceneID{MapID: 14})
	inspect(t, first, rig, func(s *Scene) {
		s.skills, s.lootTable, s.defs = fixture.skills, fixture.lootTable, fixture.defs
		putMonster(s, 900, 100000, domain.Stats{}, 120, 100)
	})
	first.Post(Enter{ID: 1, Char: &domain.Character{ID: 101, Name: "travelling-thief", Race: domain.Assassin, Level: 100,
		SkillAllocationVersion: domain.SkillFormulaVersion, Skills: domain.Learned{10212: 1}, Pos: domain.Pos{MapID: 7, X: 100, Y: 100}}, Sink: &fakeSink{}})
	inspect(t, first, rig, func(s *Scene) {
		p := s.players[1]
		p.MP, p.MaxMP = 10000, 10000
		castTheft(t, s, p, s.entities[900])
		onlyTheftDrop(t, s)
	})
	first.Post(Teleport{ID: 1, To: other.ID(), At: domain.Pos{MapID: 14, X: 100, Y: 100}})
	inspect(t, first, rig, func(s *Scene) {
		if s.players[1] != nil || s.entities[900].Monster.TheftSuccesses != 1 {
			t.Error("outbound map transfer reset monster allowance")
		}
	})
	inspect(t, other, rig, func(s *Scene) {
		if s.players[1] == nil {
			t.Error("outbound transfer failed")
		}
	})
	other.Post(Teleport{ID: 1, To: first.ID(), At: domain.Pos{MapID: 7, X: 100, Y: 100}})
	inspect(t, other, rig, func(s *Scene) {
		if s.players[1] != nil {
			t.Error("return transfer failed")
		}
	})
	inspect(t, first, rig, func(s *Scene) {
		p := s.players[1]
		if p == nil {
			t.Error("player did not return")
			return
		}
		s.tick += 100
		mp := p.MP
		castTheft(t, s, p, s.entities[900])
		if p.MP != mp || len(s.ground) != 1 || s.entities[900].Monster.TheftSuccesses != 1 {
			t.Error("map return allowed a repeated theft")
		}
	})
}

func TestMonsterTheftSettlementRacesAreBounded(t *testing.T) {
	for _, mode := range []string{"two-casters", "death-before-settlement", "source-left", "target-moved"} {
		t.Run(mode, func(t *testing.T) {
			s, src, dst, _ := theftTestScene(t, 10000)
			s.onUseSkill(UseSkill{ID: src.ID, Target: dst.ID, Skill: 10212})
			switch mode {
			case "two-casters":
				join(t, s, 2, 102, "competitor", 100, 100)
				p := s.players[2]
				p.Player.Char.Skills = domain.Learned{10212: 1}
				p.MP, p.MaxMP = 10000, 10000
				s.onUseSkill(UseSkill{ID: p.ID, Target: dst.ID, Skill: 10212})
			case "death-before-settlement":
				dst.HP = 0
			case "source-left":
				s.onLeave(Leave{ID: src.ID})
			case "target-moved":
				dst.Pos.X += 1000
			}
			s.stepCasts()
			if mode == "two-casters" {
				onlyTheftDrop(t, s)
				if dst.Monster.TheftSuccesses != 1 {
					t.Fatal("competing casts duplicated marker")
				}
			} else if len(s.ground) != 0 || dst.Monster.TheftSuccesses != 0 {
				t.Fatal("invalidated queued cast stole from stale target")
			}
		})
	}
}

func TestMonsterTheftFullBagOwnershipAndKillRemainIndependent(t *testing.T) {
	s, src, dst, _ := theftTestScene(t, 10000)
	src.Player.Bag = domain.NewBag(1)
	src.Player.Bag.Set(0, domain.NewStack(掉落牛肉面, 1))
	castTheft(t, s, src, dst)
	drop := onlyTheftDrop(t, s)
	want := drop.Drop.Stack
	s.onPickUp(PickUp{ID: src.ID, Target: drop.ID})
	if s.ground[drop.ID] == nil || src.Player.Bag.CountOf(want.Item) != 0 || drop.Drop.Stack != want {
		t.Fatal("full bag lost or rerolled stolen item")
	}
	join(t, s, 2, 102, "competitor", 100, 100)
	s.onPickUp(PickUp{ID: 2, Target: drop.ID})
	if s.ground[drop.ID] == nil || s.players[2].Player.Bag.CountOf(want.Item) != 0 {
		t.Fatal("theft ownership protection missing")
	}
	src.Player.Bag.Set(0, domain.Stack{})
	s.onPickUp(PickUp{ID: src.ID, Target: drop.ID})
	s.onPickUp(PickUp{ID: src.ID, Target: drop.ID})
	if len(s.ground) != 0 || src.Player.Bag.CountOf(want.Item) != 1 || src.Player.Bag.At(0) != want {
		t.Fatal("pickup failed to preserve exact instance or duplicate pickup paid twice")
	}
	// Normal kill generation still reads its unchanged independent drop table.
	s.lootTable[dst.Monster.TypeID] = []domain.DropEntry{{Item: 掉落树枝.ID, RatePct: 100}}
	dst.HP = 0
	s.onDeath(dst, src.ID)
	onlyTheftDrop(t, s)
	if dst.Monster.TheftSuccesses != 1 || src.Player.Bag.CountOf(want.Item) != 1 {
		t.Fatal("normal kill changed successful theft ownership/count")
	}
}
