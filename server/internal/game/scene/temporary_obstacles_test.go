package scene

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func temporaryWallFixture(t *testing.T) (*Scene, *int64) {
	t.Helper()
	s := newTestScene(t, nil)
	now := int64(100000)
	s.now = func() time.Time { return time.UnixMilli(now) }
	return s, &now
}

func temporaryTestWall(id string, x, y, x2, y2 float64) temporaryWall {
	return temporaryWall{ID: id, Owner: "cast/" + id, MapID: 7, X: x, Y: y, X2: x2, Y2: y2, Width: 20, ExpiresAtMS: 115000}
}

func temporaryTestPos(x, y float64) domain.Pos { return domain.Pos{MapID: 7, X: x, Y: y} }

func TestTemporaryWallContinuousOrientedGeometry(t *testing.T) {
	wall := temporaryTestWall("a", 100, 60, 100, 140)
	cases := []struct {
		name     string
		from, to domain.Pos
		blocked  bool
	}{
		{"fast crossing", temporaryTestPos(-10000, 100), temporaryTestPos(10000, 100), true},
		{"corner grazing", temporaryTestPos(50, 12), temporaryTestPos(150, 112), true},
		{"flat end clearance", temporaryTestPos(50, 47), temporaryTestPos(150, 47), false},
		{"parallel clear", temporaryTestPos(77, 0), temporaryTestPos(77, 200), false},
		{"parallel boundary", temporaryTestPos(78, 0), temporaryTestPos(78, 200), true},
		{"inside stationary", temporaryTestPos(100, 100), temporaryTestPos(100, 100), true},
		{"outside stationary", temporaryTestPos(50, 50), temporaryTestPos(50, 50), false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := wall.pathBlocked(tt.from, tt.to, temporaryWallActorClearance); got != tt.blocked {
				t.Fatalf("blocked=%v want %v", got, tt.blocked)
			}
			if got := wall.pathBlocked(tt.to, tt.from, temporaryWallActorClearance); got != tt.blocked {
				t.Fatal("reversal changed collision")
			}
		})
	}
	diagonal := temporaryTestWall("b", 0, 0, 80, 80)
	if !diagonal.pathBlocked(temporaryTestPos(0, 80), temporaryTestPos(80, 0), 12) {
		t.Fatal("oriented crossing missed")
	}
	if diagonal.pathBlocked(temporaryTestPos(0, 150), temporaryTestPos(80, 150), 12) {
		t.Fatal("oriented AABB incorrectly blocked")
	}
}

func TestTemporaryWallAtomicPlacementSafety(t *testing.T) {
	for _, reason := range []string{"player", "spawn", "entrance", "exit", "revive", "legacy entry", "portal", "narrow corridor", "map boundary", "existing wall", "same batch", "duplicate id", "invalid geometry"} {
		t.Run(reason, func(t *testing.T) {
			s, _ := temporaryWallFixture(t)
			walls := []temporaryWall{temporaryTestWall("a", 100, 100, 180, 100), temporaryTestWall("b", 300, 100, 380, 100), temporaryTestWall("c", 500, 100, 580, 100)}
			switch reason {
			case "player":
				join(t, s, 1, 100, "player", 540, 100)
			case "spawn":
				s.rift = &endgameRiftState{data: endgameRiftPersisted{Floor: &domain.RiftFloorPlan{Spawns: []domain.RiftFloorSpawn{{Point: domain.SpawnPoint{Pos: temporaryTestPos(540, 100)}}}}}}
			case "entrance":
				s.rift = &endgameRiftState{data: endgameRiftPersisted{Floor: &domain.RiftFloorPlan{Entry: temporaryTestPos(540, 100)}}}
			case "exit":
				s.rift = &endgameRiftState{data: endgameRiftPersisted{Floor: &domain.RiftFloorPlan{Exit: temporaryTestPos(540, 100)}}}
			case "revive":
				s.revive = ReviveAt{Scene: s.id, Pos: temporaryTestPos(540, 100)}
			case "legacy entry":
				s.rift = &endgameRiftState{}
				s.dungeons = domain.DungeonTable{7: {Enter: temporaryTestPos(540, 100)}}
			case "portal":
				s.portals = []domain.Portal{domain.NewPortal(domain.Teleport{Area: domain.Polygon{{X: 530, Y: 95}, {X: 550, Y: 95}, {X: 550, Y: 105}, {X: 530, Y: 105}}})}
			case "narrow corridor":
				s.walkable = func(p domain.Pos) bool { return p.X < 480 || p.Y >= 90 && p.Y <= 110 }
			case "map boundary":
				s.walkable = func(p domain.Pos) bool { return p.X <= 600 && p.X >= 0 && p.Y >= 0 }
			case "existing wall":
				s.temporaryWalls = map[string]temporaryWall{"other": temporaryTestWall("other", 540, 120, 620, 120)}
			case "same batch":
				walls[2] = temporaryTestWall("c", 340, 120, 420, 120)
			case "duplicate id":
				walls[2].ID = walls[1].ID
			case "invalid geometry":
				walls[2].X = math.NaN()
			}
			before := len(s.temporaryWalls)
			if s.addTemporaryWalls(walls) {
				t.Fatal("unsafe batch accepted")
			}
			if len(s.temporaryWalls) != before {
				t.Fatal("failed batch left phantom obstacles")
			}
		})
	}
}

func TestTemporaryWallValidBatchGapAndSnapshot(t *testing.T) {
	s, _ := temporaryWallFixture(t)
	walls := []temporaryWall{temporaryTestWall("c", 420, 200, 500, 200), temporaryTestWall("a", 100, 200, 180, 200), temporaryTestWall("b", 260, 200, 340, 200)}
	if !s.addTemporaryWalls(walls) || len(s.temporaryWalls) != 3 {
		t.Fatal("80-unit gaps should fit three whole walls")
	}
	snapshot := s.temporaryWallSnapshot()
	if snapshot[0].ID != "a" || snapshot[2].ID != "c" {
		t.Fatal("nondeterministic snapshot")
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var restored []temporaryWall
	if err = json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	other, _ := temporaryWallFixture(t)
	if !other.restoreTemporaryWalls(restored) || len(other.temporaryWalls) != 3 {
		t.Fatal("safe snapshot did not rebuild")
	}
	snapshot[0].X = -999
	if s.temporaryWalls["a"].X != 100 {
		t.Fatal("snapshot aliases collision state")
	}
	if other.temporaryPositionWalkable(temporaryTestPos(120, 200)) {
		t.Fatal("restored wall not solid")
	}
}

func TestTemporaryWallOwnerExpiryAndIdempotentCleanup(t *testing.T) {
	s, now := temporaryWallFixture(t)
	a, b := temporaryTestWall("a", 100, 60, 100, 140), temporaryTestWall("b", 100, 60, 100, 140)
	b.ExpiresAtMS += 5000
	// Even legacy/restored overlapping owners cannot erase one another's solid.
	s.temporaryWalls = map[string]temporaryWall{a.ID: a, b.ID: b}
	s.removeTemporaryWalls(a.Owner)
	s.removeTemporaryWalls(a.Owner)
	if s.temporaryPathWalkable(temporaryTestPos(0, 100), temporaryTestPos(200, 100)) {
		t.Fatal("removing one owner erased overlapping wall")
	}
	*now = 119999
	if s.temporaryPositionWalkable(temporaryTestPos(100, 100)) {
		t.Fatal("wall expired early")
	}
	*now = 120000
	if !s.temporaryPositionWalkable(temporaryTestPos(100, 100)) {
		t.Fatal("exact expiry still collides before sweep")
	}
	s.expireTemporaryWalls(*now)
	if len(s.temporaryWalls) != 0 {
		t.Fatal("expiry retained wall")
	}
	s.clearTemporaryWalls()
	s.clearTemporaryWalls()
}

func TestTemporaryWallPlayerMoveAndAuthorityTeleport(t *testing.T) {
	s, _ := temporaryWallFixture(t)
	sink := join(t, s, 1, 100, "player", 0, 100)
	p := s.players[1]
	wall := temporaryTestWall("a", 100, 60, 100, 140)
	if !s.addTemporaryWalls([]temporaryWall{wall}) {
		t.Fatal("safe wall rejected")
	}
	s.flush()
	sink.take()
	to := temporaryTestPos(200, 100)
	s.onMove(MoveTo{ID: p.ID, To: to})
	if p.Pos.X != 0 {
		t.Fatal("player walked through wall")
	}
	s.flush()
	move, ok := firstOf[event.EntityMoved](sink.take())
	if !ok || !move.Snap || move.To != p.Pos {
		t.Fatal("missing authoritative prediction correction")
	}
	s.move(MoveTo{ID: p.ID, To: to}, false, true)
	if p.Pos != to {
		t.Fatal("wall incorrectly blocked authoritative GM/exit teleport")
	}
	if !s.temporaryPositionWalkable(to) {
		t.Fatal("valid teleport destination blocked")
	}
	if s.temporaryPositionWalkable(temporaryTestPos(100, 100)) {
		t.Fatal("skill teleport landing in wall accepted")
	}
}

func TestTemporaryWallAIPathWithoutStaticMask(t *testing.T) {
	s, _ := temporaryWallFixture(t)
	wall := temporaryTestWall("a", 100, 60, 100, 140)
	if !s.addTemporaryWalls([]temporaryWall{wall}) {
		t.Fatal("wall rejected")
	}
	from, to := temporaryTestPos(0, 100), temporaryTestPos(180, 100)
	if s.straightPathWalkable(from, to) {
		t.Fatal("nil MASK bypassed obstacle")
	}
	path, ok := s.localMonsterPath(from, to, 200)
	if !ok || len(path) < 2 {
		t.Fatal("AI did not find detour around temporary wall", path)
	}
	at := from
	for _, next := range path {
		if !s.temporaryPathWalkable(at, next) {
			t.Fatal("detour crosses wall", at, next)
		}
		at = next
	}
	m := putMonster(s, 999, 100, domain.Stats{}, 0, 100)
	s.monsters[m.ID] = m
	m.Monster.ChasePath = path
	m.Monster.ChasePathRetryAt = 999
	m.Monster.Roaming = true
	s.moveEntity(m, to)
	if m.Pos != from {
		t.Fatal("direct high-speed AI move tunneled")
	}
	s.clearTemporaryWalls()
	if m.Monster.ChasePath != nil || m.Monster.ChasePathRetryAt != 0 || m.Monster.Roaming {
		t.Fatal("stale path/failed-path cache survived geometry update")
	}
	if !s.straightPathWalkable(from, to) {
		t.Fatal("wall expiry left phantom block")
	}
}

func TestTemporaryWallKnockbackAndSpawnLanding(t *testing.T) {
	s, _ := temporaryWallFixture(t)
	s.temporaryWalls = map[string]temporaryWall{"a": temporaryTestWall("a", 100, 60, 100, 140)}
	src := &entity.Entity{Pos: temporaryTestPos(0, 100)}
	target := putMonster(s, 999, 100, domain.Stats{}, 50, 100)
	if !s.knockbackMonster(src, target, 200) || target.Pos.X >= 78 || target.Pos.X <= 50 {
		t.Fatal("knockback did not stop before padded wall", target.Pos)
	}
	if s.spawnPositionWalkable(temporaryTestPos(100, 100)) {
		t.Fatal("spawn inside wall accepted")
	}
	if !s.spawnPositionWalkable(temporaryTestPos(50, 100)) {
		t.Fatal("safe spawn rejected")
	}
}

func TestTemporaryWallRestoreRejectsTrappingReconnect(t *testing.T) {
	s, now := temporaryWallFixture(t)
	join(t, s, 1, 100, "reconnected", 100, 100)
	wall := temporaryTestWall("a", 100, 60, 100, 140)
	if s.restoreTemporaryWalls([]temporaryWall{wall}) || len(s.temporaryWalls) != 0 {
		t.Fatal("recovery trapped reconnecting player")
	}
	*now = wall.ExpiresAtMS
	if !s.restoreTemporaryWalls([]temporaryWall{wall}) || len(s.temporaryWalls) != 0 {
		t.Fatal("expired wall resurrected on reconnect")
	}
}

func TestTemporaryWallCasterRemovalDoesNotRemoveWall(t *testing.T) {
	s, _ := temporaryWallFixture(t)
	m := putMonster(s, 999, 100, domain.Stats{}, 0, 100)
	wall := temporaryTestWall("a", 100, 60, 100, 140)
	if !s.addTemporaryWalls([]temporaryWall{wall}) {
		t.Fatal("wall rejected")
	}
	s.despawnMonster(m)
	if s.temporaryPositionWalkable(temporaryTestPos(100, 100)) {
		t.Fatal("caster removal erased independent wall")
	}
	s.clearTemporaryWalls()
	if !s.temporaryPositionWalkable(temporaryTestPos(100, 100)) {
		t.Fatal("phase/disconnect cleanup left stale collision")
	}
}

func TestTemporaryWallKnockbackHonorsImpactTimeImmunity(t *testing.T) {
	s, _ := temporaryWallFixture(t)
	src := &entity.Entity{Pos: temporaryTestPos(0, 100)}
	target := putMonster(s, 999, 100, domain.Stats{}, 50, 100)
	target.Status = domain.NewStatusSet()
	target.Status.Apply(domain.StatusDef{ID: 1999001, Level: 1, Type: domain.StatusGood, TypeKnown: true, DurationSec: 5, SuppressHarmful: true}, 0, target.ID)
	s.tick = 49
	if s.knockbackMonster(src, target, 50) || target.Pos.X != 50 {
		t.Fatal("queued knockback bypassed current immunity")
	}
	s.tick = 50
	if !s.knockbackMonster(src, target, 50) || target.Pos.X != 100 {
		t.Fatal("expired immunity blocked fresh impact")
	}
}

func TestTemporaryWallRestoreKeepsSafeNearbyPlayers(t *testing.T) {
	s, _ := temporaryWallFixture(t)
	join(t, s, 1, 100, "reconnected", 70, 100)
	wall := temporaryTestWall("a", 100, 60, 100, 140)
	if s.addTemporaryWalls([]temporaryWall{wall}) {
		t.Fatal("initial cast ignored 40-unit safety margin")
	}
	if !s.restoreTemporaryWalls([]temporaryWall{wall}) {
		t.Fatal("safe player near existing wall incorrectly cancelled recovery")
	}
	if !s.temporaryPositionWalkable(s.players[1].Pos) {
		t.Fatal("reconnected actor embedded in recovered wall")
	}
}

func TestTemporaryWallDirectTauntHonorsImpactTimeImmunity(t *testing.T) {
	s, _ := temporaryWallFixture(t)
	join(t, s, 1, 100, "attacker", 0, 100)
	target := putMonster(s, 999, 100, domain.Stats{}, 50, 100)
	target.Status = domain.NewStatusSet()
	target.Status.Apply(domain.StatusDef{ID: 1999001, Level: 1, Type: domain.StatusGood, TypeKnown: true, DurationSec: 5, SuppressHarmful: true}, 0, target.ID)
	s.tick = 49
	if s.taunt(target, 1, 10) || target.Monster.Target != 0 {
		t.Fatal("direct taunt bypassed current immunity")
	}
	s.tick = 50
	if !s.taunt(target, 1, 10) || target.Monster.Target != 1 {
		t.Fatal("expired immunity blocked new taunt")
	}
}

func TestTemporaryWallOldOwnerCannotRemoveNewGeneration(t *testing.T) {
	s, now := temporaryWallFixture(t)
	old := temporaryTestWall("slot", 100, 60, 100, 140)
	if !s.addTemporaryWalls([]temporaryWall{old}) {
		t.Fatal("initial wall rejected")
	}
	replacement := old
	replacement.Owner = "run2/cast2"
	replacement.ExpiresAtMS = 130000
	if s.addTemporaryWalls([]temporaryWall{replacement}) {
		t.Fatal("live wall ID silently overwritten")
	}
	*now = old.ExpiresAtMS
	if !s.addTemporaryWalls([]temporaryWall{replacement}) {
		t.Fatal("expired slot not reusable")
	}
	s.removeTemporaryWalls(old.Owner)
	s.expireTemporaryWalls(old.ExpiresAtMS)
	if s.temporaryPositionWalkable(temporaryTestPos(100, 100)) {
		t.Fatal("stale generation cleanup removed replacement")
	}
}
