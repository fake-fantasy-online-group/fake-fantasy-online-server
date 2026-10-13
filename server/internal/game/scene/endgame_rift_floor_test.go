package scene

import catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"

import (
	"errors"
	"reflect"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
)

// Floor writes are controlled separately from character commits so tests can
// observe the unpaid reservation and both sides of an ambiguous acknowledgement.
type riftFloorTestBackend struct {
	*riftTestBackend
	writes []domain.EndgameRunRecord
	cas    []int64
	acks   []chan domain.EndgameRunSaveResult
	loads  int
}

func (b *riftFloorTestBackend) SaveRun(r domain.EndgameRunRecord, expected int64) <-chan domain.EndgameRunSaveResult {
	c := make(chan domain.EndgameRunSaveResult, 1)
	b.writes = append(b.writes, r)
	b.cas = append(b.cas, expected)
	b.acks = append(b.acks, c)
	return c
}

func (b *riftFloorTestBackend) LoadRun(id string) <-chan domain.EndgameRunLoadResult {
	b.loads++
	return b.riftTestBackend.LoadRun(id)
}

func (b *riftFloorTestBackend) finishFloorWrite(t *testing.T, index int, durable, lostACK bool) {
	t.Helper()
	r := b.writes[index]
	if durable {
		if old := b.runs[r.RunID]; old.Revision != b.cas[index] {
			t.Fatalf("incorrect floor CAS: durable=%d expected=%d", old.Revision, b.cas[index])
		}
		b.runs[r.RunID] = r
	}
	if !durable || lostACK {
		b.acks[index] <- domain.EndgameRunSaveResult{Err: errors.New("floor acknowledgement lost")}
	} else {
		b.acks[index] <- domain.EndgameRunSaveResult{Run: &r}
	}
}

func riftFloorTemplatesForTest() []domain.RiftFloorTemplate {
	var layouts []domain.RiftFloorTemplate
	for _, mapID := range []int32{7, 8} {
		layout := domain.RiftFloorTemplate{MapID: mapID, MapName: "测试地图", Monsters: []domain.MonsterID{1}, Guardians: []domain.MonsterID{2}}
		for i := 0; i < 40; i++ {
			layout.Anchors = append(layout.Anchors, domain.SpawnPoint{ID: int32(i + 1), Monster: 1, Pos: domain.Pos{MapID: mapID, X: 100 + float64(i%8)*300, Y: 100 + float64(i/8)*300}})
		}
		layouts = append(layouts, layout)
	}
	return layouts
}

func seededRiftFloorFixture(t *testing.T, members ...domain.TrialKeyMember) (*Scene, *riftFloorTestBackend, *int64) {
	t.Helper()
	s, base, now := riftFixture(t)
	b := &riftFloorTestBackend{riftTestBackend: base}
	s.endgameBackend = b
	s.riftLayouts = riftFloorTemplatesForTest()
	s.walkable = func(domain.Pos) bool { return true }
	s.defs = endgameRiftDefs{trial: map[domain.MonsterID]domain.MonsterDef{
		1: {ID: 1, Sprite: 1, Kind: domain.MonsterNormal, Level: 60, HP: 100},
		2: {ID: 2, Sprite: 2, Kind: domain.MonsterBoss, Level: 60, HP: 1000},
	}}
	if len(members) == 0 {
		members = []domain.TrialKeyMember{{CharID: 100, Level: 60}}
	}
	r, err := domain.NewRiftRun("seeded-floor-lifecycle", 7, 100, members, *now, riftRulesFixture())
	if err != nil {
		t.Fatal(err)
	}
	r.Activate(*now)
	plan, err := s.planEndgameRiftFloor(r.RunID, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	s.id = domain.SceneID{MapID: plan.MapID, Instance: 90}
	s.router.scenes = map[domain.SceneID]*Scene{s.id: s}
	d := endgameRiftPersisted{Version: 1, StorageRevision: 5, Run: r, Floor: &plan, MapID: plan.MapID, Return: domain.Pos{MapID: 9, X: 100, Y: 100}, Names: map[domain.CharID]string{100: "甲"}}
	s.initEndgameRift(d)
	s.rift.floorLoading = false
	s.rift.lastSave = *now
	allowed := map[domain.CharID]bool{}
	for _, m := range members {
		allowed[m.CharID] = true
	}
	s.router.riftAdmissions = map[domain.SceneID]*endgameRiftAdmission{s.id: {Data: d.clone(), Paid: true, Allowed: allowed}}
	p := s.players[1]
	p.Pos, p.Player.Char.Pos = plan.Exit, plan.Exit
	p.Player.Char.SceneInstance = s.id.Instance
	p.Player.Bag.AddStack(s.items[domain.RiftKeyItemID], domain.Stack{Item: domain.RiftKeyItemID, Count: 3, Bound: true})
	s.syncEndgameRift(p)
	s.populateEndgameRift(false)
	if s.rift.data.Run.Phase != domain.RiftActive || len(s.monsters) < 8 {
		t.Fatalf("seeded fixture population failed: phase=%s monsters=%d", s.rift.data.Run.Phase, len(s.monsters))
	}
	s.rift.data.Run.Progress = 840
	s.rift.data.Run.PenaltyMS = domain.RiftDeathPenaltyMS
	s.rift.data.Run.Members[100].PresentMS = 9000
	b.runs[r.RunID] = s.rift.data.record(5)
	s.flush()
	p.Player.Sink.(*fakeSink).take()
	return s, b, now
}

// Install the next reserved actor without running a goroutine: its mailbox and
// ownership transfer remain real, while every frame is driven deterministically.
func prepareRiftFloorTarget(t *testing.T, s *Scene) *Scene {
	t.Helper()
	plan, err := s.planEndgameRiftFloor(s.rift.data.Run.RunID, s.rift.data.Floor.FloorIndex+1, s.rift.data.MapID)
	if err != nil {
		t.Fatal(err)
	}
	id := domain.SceneID{MapID: plan.MapID, Instance: s.router.nextInst.Load() + 1}
	target := New(Config{LootRules: s.lootRules, RiftRules: s.riftRules, EndgameCatalog: catalogfixture.EndgameCatalog(), ID: id, Now: s.now, Alloc: s.alloc, Trial: s.trial, Dungeons: s.dungeons, Defs: s.defs, Items: s.items, EndgameBackend: s.endgameBackend, RiftLayouts: s.riftLayouts, Walkable: s.walkable})
	target.router = s.router
	s.router.scenes[id] = target
	return target
}

func advanceRiftFloorRequest(s *Scene) domain.EndgameRequest {
	return domain.EndgameRequest{Op: "rift.advance", RequestID: "floor-transition-0001", RunID: s.rift.data.Run.RunID, FloorIndex: s.rift.data.Floor.FloorIndex}
}

func requireRiftFloorError(t *testing.T, s *Scene, p *entity.Entity, code string) {
	t.Helper()
	s.flush()
	for _, ev := range p.Player.Sink.(*fakeSink).take() {
		if reply, ok := ev.(event.EndgameMessage); ok && reply.Message.Error == code && !reply.Message.OK {
			return
		}
	}
	t.Fatalf("missing floor error %q", code)
}

func TestRiftFloorTransitionPersistsBeforePublication(t *testing.T) {
	s, b, now := seededRiftFloorFixture(t)
	target := prepareRiftFloorTarget(t, s)
	p := s.players[1]
	old := s.rift.data.clone()
	oldMonsterCount := len(s.monsters)
	s.rift.data.Run.Globes[999999] = domain.RiftGlobe{ID: 999999, Pos: p.Pos, Value: domain.RiftGlobeProgress}
	s.advanceEndgameRift(p, advanceRiftFloorRequest(s))
	if len(b.writes) != 1 || b.cas[0] != 5 || b.writes[0].Revision != 6 || s.rift.transition == nil {
		t.Fatalf("missing CAS write: writes=%d cas=%v", len(b.writes), b.cas)
	}
	if _, paid := s.router.riftAdmission(target.id); paid || s.router.ActiveRiftForCharacter(target.id, 100) {
		t.Fatal("next floor published before durability")
	}
	if current, ok := s.router.riftScene(old.Run.RunID); !ok || current != s.id || len(s.players) != 1 || len(s.monsters) != oldMonsterCount {
		t.Fatal("source ownership changed before durability")
	}
	if s.riftCombatReady() || s.riftView(p, s.rift.data, true, true).CanAdvance {
		t.Fatal("combat/advance remained enabled during floor write")
	}
	s.advanceEndgameRift(p, advanceRiftFloorRequest(s))
	if len(b.writes) != 1 {
		t.Fatal("duplicate request created a second transition write")
	}
	*now += 2300
	s.stepEndgameRift()
	if len(b.writes) != 1 || s.rift.data.Run.StartedMS != old.Run.StartedMS {
		t.Fatal("pending floor write duplicated or reset the timer")
	}
	b.finishFloorWrite(t, 0, true, false)
	s.stepEndgameRift()
	if !s.rift.retired || len(s.players) != 0 || len(s.monsters) != 0 {
		t.Fatal("durable floor did not retire and drain source")
	}
	if current, ok := s.router.riftScene(old.Run.RunID); !ok || current != target.id {
		t.Fatal("canonical run did not switch to durable target")
	}
	if s.router.ActiveRiftForCharacter(s.id, 100) || s.router.PostExistingRift(s.id, 100, MapReady{}) {
		t.Fatal("retired source still accepts reconnect/commands")
	}
	target.drain()
	target.stepEndgameRift()
	next := target.rift.data
	if next.Floor.FloorIndex != 2 || next.MapID == old.MapID || next.Run.RunID != old.Run.RunID || next.Run.Tier != old.Run.Tier || next.Run.Progress != old.Run.Progress || next.Run.StartedMS != old.Run.StartedMS || next.Run.PenaltyMS != old.Run.PenaltyMS {
		t.Fatalf("floor changed persistent run identity/progress/timer: before=%+v after=%+v", old.Run, next.Run)
	}
	if target.players[1] == nil || target.players[1].Pos != next.Floor.Entry || target.players[1].Player.Bag.CountOf(domain.RiftKeyItemID) != 3 || b.commits != 0 {
		t.Fatal("floor handoff lost player or debited another key")
	}
	if next.Run.Members[100].PresentMS != old.Run.Members[100].PresentMS+2300 {
		t.Fatal("floor write time was dropped from presence clock")
	}
	if len(next.Run.Globes) != 0 || next.Run.CollectGlobe(999999, 100) {
		t.Fatal("source floor globe carried into or could be replayed on target")
	}
	for id := range old.Run.Monsters {
		if next.Run.Monsters[id] != nil {
			t.Fatal("source monster IDs carried into new floor")
		}
		before := next.Run.Progress
		target.onEndgameRiftMonsterDeath(&entity.Entity{ID: id}, nil)
		if next.Run.Progress != before {
			t.Fatal("replayed old-floor kill earned progress")
		}
	}
}

func TestRiftFloorTransitionLostACKReadsDurableFloorOnce(t *testing.T) {
	s, b, _ := seededRiftFloorFixture(t)
	target := prepareRiftFloorTarget(t, s)
	p := s.players[1]
	req := advanceRiftFloorRequest(s)
	s.advanceEndgameRift(p, req)
	beforeLoads := b.loads
	b.finishFloorWrite(t, 0, true, true)
	s.stepEndgameRift()
	if b.loads != beforeLoads+1 || !s.rift.retired || len(b.writes) != 1 || b.commits != 0 {
		t.Fatal("lost ACK was not reconciled as the same committed floor")
	}
	target.drain()
	target.stepEndgameRift()
	p = target.players[1]
	p.Pos = target.rift.data.Floor.Exit
	target.advanceEndgameRift(p, req)
	requireRiftFloorError(t, target, p, "stale_floor")
	if len(b.writes) != 1 || target.rift.data.Floor.FloorIndex != 2 {
		t.Fatal("replayed source request advanced another floor")
	}
}

func TestRiftFloorTransitionRollbackKeepsPopulationAndRetryPlan(t *testing.T) {
	s, b, _ := seededRiftFloorFixture(t)
	target := prepareRiftFloorTarget(t, s)
	before := s.rift.data.clone()
	abilityCount := len(s.rift.abilities)
	if abilityCount == 0 {
		t.Fatal("fixture must include elite abilities")
	}
	p := s.players[1]
	req := advanceRiftFloorRequest(s)
	s.advanceEndgameRift(p, req)
	proposed := s.rift.transition.next.Floor
	b.finishFloorWrite(t, 0, false, false)
	s.stepEndgameRift()
	if s.rift.transition != nil || s.rift.retired || !s.riftCombatReady() || len(s.players) != 1 {
		t.Fatal("definite rollback did not restore source ownership")
	}
	if !reflect.DeepEqual(s.rift.data.Run, before.Run) || len(s.monsters) != len(before.Run.Monsters) || len(s.rift.abilities) != abilityCount {
		t.Fatal("rollback changed progress/population or lost monster abilities")
	}
	if _, exists := s.router.riftAdmissions[target.id]; exists {
		t.Fatal("rollback leaked unpaid next-floor admission")
	}
	requireRiftFloorError(t, s, p, "storage")
	prepareRiftFloorTarget(t, s)
	s.advanceEndgameRift(p, req)
	if len(b.writes) != 2 || !reflect.DeepEqual(s.rift.transition.next.Floor, proposed) || b.cas[1] != 5 || b.commits != 0 {
		t.Fatal("retry changed deterministic floor plan or charged a key")
	}
}

func TestRiftFloorTransitionReadFailureKeepsOwnershipQuarantined(t *testing.T) {
	s, b, now := seededRiftFloorFixture(t)
	target := prepareRiftFloorTarget(t, s)
	s.advanceEndgameRift(s.players[1], advanceRiftFloorRequest(s))
	failedRead := make(chan domain.EndgameRunLoadResult, 1)
	failedRead <- domain.EndgameRunLoadResult{Err: errors.New("database unavailable")}
	b.runLoad = failedRead
	b.finishFloorWrite(t, 0, true, true)
	s.stepEndgameRift()
	if s.rift.transition == nil || s.rift.retired || s.riftCombatReady() || len(s.players) != 1 {
		t.Fatal("ambiguous outcome released source ownership")
	}
	if _, paid := s.router.riftAdmission(target.id); paid {
		t.Fatal("ambiguous floor became payable before reconciliation")
	}
	b.runLoad = nil
	*now += 1000
	s.stepEndgameRift()
	if !s.rift.retired || len(b.writes) != 1 {
		t.Fatal("durable floor was not recovered after read service returned")
	}
}

func TestRiftFloorAdvanceAdmissionGuards(t *testing.T) {
	for _, scenario := range []string{"far_from_exit", "dead", "not_ready", "boss", "peer", "departed", "stale", "wrong_run", "pending_save"} {
		t.Run(scenario, func(t *testing.T) {
			s, b, _ := seededRiftFloorFixture(t)
			prepareRiftFloorTarget(t, s)
			p := s.players[1]
			req := advanceRiftFloorRequest(s)
			switch scenario {
			case "far_from_exit":
				p.Pos = s.rift.data.Floor.Entry
			case "dead":
				p.HP = 0
			case "not_ready":
				delete(s.rift.ready, 100)
			case "boss":
				s.rift.data.Run.Phase = domain.RiftBoss
			case "peer":
				s.rift.data.Run.Payer = 200
			case "departed":
				s.rift.data.Run.Members[100].Departed = true
			case "stale":
				req.FloorIndex--
			case "wrong_run":
				req.RunID = "different-run"
			case "pending_save":
				s.rift.saving = make(chan domain.EndgameRunSaveResult)
			}
			s.advanceEndgameRift(p, req)
			if len(b.writes) != 0 || b.commits != 0 || s.rift.transition != nil || len(s.players) != 1 {
				t.Fatal("ineligible floor advance changed state")
			}
		})
	}
}

func TestRiftFloorTargetReservationFailurePreservesSource(t *testing.T) {
	s, b, _ := seededRiftFloorFixture(t)
	// Router has no started builder and no prepared target, forcing Ensure to
	// reject without starting an actor or charging a persistent operation.
	s.advanceEndgameRift(s.players[1], advanceRiftFloorRequest(s))
	if len(b.writes) != 0 || b.commits != 0 || s.rift.transition != nil || len(s.router.riftAdmissions) != 1 || !s.riftCombatReady() {
		t.Fatal("target reservation failure changed durable/source state")
	}
	requireRiftFloorError(t, s, s.players[1], "instance")
}

func TestRiftFloorDeadTargetRetriesSameDurablePlan(t *testing.T) {
	s, b, now := seededRiftFloorFixture(t)
	failed := prepareRiftFloorTarget(t, s)
	s.advanceEndgameRift(s.players[1], advanceRiftFloorRequest(s))
	plan := *s.rift.transition.next.Floor
	failed.Stop()
	replacement := prepareRiftFloorTarget(t, s)
	b.finishFloorWrite(t, 0, true, false)
	s.stepEndgameRift()
	if s.rift.retired || s.rift.transition == nil || s.rift.transition.target != replacement.id || len(s.players) != 1 {
		t.Fatal("dead target was published or failed to reserve replacement")
	}
	if _, paid := s.router.riftAdmission(replacement.id); paid {
		t.Fatal("replacement published before retry reconciliation")
	}
	*now += 1000
	s.stepEndgameRift()
	if !s.rift.retired || len(b.writes) != 1 || b.commits != 0 {
		t.Fatal("replacement target did not complete the same durable write")
	}
	replacement.drain()
	if !reflect.DeepEqual(*replacement.rift.data.Floor, plan) {
		t.Fatal("target recovery regenerated the committed plan")
	}
}

func TestRiftFloorPartyHandoffPreservesDeadAndDisconnectedMembers(t *testing.T) {
	members := []domain.TrialKeyMember{{CharID: 100, Level: 60}, {CharID: 200, Level: 60}, {CharID: 300, Level: 60}, {CharID: 400, Level: 60}}
	s, b, now := seededRiftFloorFixture(t, members...)
	join(t, s, 2, 200, "乙", 100, 100)
	p := s.players[2]
	p.HP = 0
	p.MaxHP = 100
	s.rift.deathAt[200] = *now - 1000
	s.rift.ready[200] = true
	s.rift.data.Run.Members[400].Departed = true
	s.router.withdrawRift(s.id, 400)
	target := prepareRiftFloorTarget(t, s)
	s.advanceEndgameRift(s.players[1], advanceRiftFloorRequest(s))
	b.finishFloorWrite(t, 0, true, false)
	s.stepEndgameRift()
	target.drain()
	if target.players[1] == nil || target.players[2] == nil || target.players[2].Alive() {
		t.Fatal("party handoff dropped or auto-revived dead member")
	}
	if len(target.rift.data.Run.Members) != 4 || !target.rift.data.Run.Admitted(300) || target.rift.data.Run.Admitted(400) {
		t.Fatal("frozen roster changed across floors")
	}
	if destination, entry, ok := s.router.ActiveRiftDestination(300); !ok || destination != target.id || entry != target.rift.data.Floor.Entry {
		t.Fatal("disconnected member cannot find current floor entrance")
	}
	if _, _, ok := s.router.ActiveRiftDestination(400); ok || s.router.ActiveRiftForCharacter(target.id, 999) {
		t.Fatal("withdrawn/outsider member acquired floor admission")
	}
	dead := target.players[2]
	target.reviveEndgameRift(dead)
	if dead.Alive() {
		t.Fatal("floor transition bypassed remaining revive cooldown")
	}
	*now += 4000
	target.reviveEndgameRift(dead)
	if !dead.Alive() || dead.Pos != target.rift.data.Floor.Entry {
		t.Fatal("member did not revive at current seeded entrance")
	}
	// An absent admitted member may return after the next floor's loading window.
	*now += 60000
	target.stepEndgameRift()
	if target.rift.floorLoading || target.rift.data.Run.Terminal() || len(target.monsters) == 0 || !target.rift.data.Run.Admitted(300) {
		t.Fatal("disconnected member blocked floor forever or lost admission")
	}
}

func TestRiftFloorTransitionDoesNotResetHardDeadline(t *testing.T) {
	s, b, now := seededRiftFloorFixture(t)
	target := prepareRiftFloorTarget(t, s)
	started := s.rift.data.Run.StartedMS
	s.advanceEndgameRift(s.players[1], advanceRiftFloorRequest(s))
	*now = started + domain.RiftHardMS + 1
	b.finishFloorWrite(t, 0, true, false)
	s.stepEndgameRift()
	target.drain()
	target.stepEndgameRift()
	if target.rift.data.Run.StartedMS != started || target.rift.data.Run.Phase != domain.RiftFailed || target.rift.data.Run.Reason != "timeout" {
		t.Fatal("floor write/loading extended the run's hard deadline")
	}
}

type stoppingRiftFloorTransferSink struct {
	*fakeSink
	stopTarget func()
}

func (s *stoppingRiftFloorTransferSink) TransferScene(_ domain.EntityID, _, _ domain.SceneID, handoff func() bool) bool {
	s.stopTarget()
	return handoff()
}

func TestRiftFloorTargetStopsAfterPublicationBeforeHandoff(t *testing.T) {
	s, b, _ := seededRiftFloorFixture(t)
	target := prepareRiftFloorTarget(t, s)
	p := s.players[1]
	saver := newFakeSaver()
	p.Player.FinalSaver = saver
	sink := &stoppingRiftFloorTransferSink{fakeSink: &fakeSink{}, stopTarget: func() {
		if !s.rift.retired || !s.router.ActiveRiftForCharacter(target.id, 100) {
			t.Fatal("test must stop target after the durable publication")
		}
		target.Stop()
	}}
	p.Player.Sink = sink
	s.advanceEndgameRift(p, advanceRiftFloorRequest(s))
	b.finishFloorWrite(t, 0, true, false)
	s.stepEndgameRift()
	if len(s.players) != 0 || len(target.players) != 0 || len(target.handoffs) != 0 || !sink.isClosed() {
		t.Fatal("failed handoff retained ambiguous live ownership")
	}
	if saver.count(100) != 1 || saver.last[100].SceneInstance != target.id.Instance || saver.last[100].Pos != s.rift.data.Floor.Entry || saver.lastBag[100].CountOf(domain.RiftKeyItemID) != 3 {
		t.Fatal("failed handoff did not persist current floor and unchanged key balance")
	}
	if current, ok := s.router.riftScene(s.rift.data.Run.RunID); ok {
		t.Fatalf("stopped target is still a live rift: %v", current)
	}
	if len(s.router.scenes) != 2 || len(b.writes) != 1 || b.commits != 0 {
		t.Fatal("failed rift handoff recreated a public scene or duplicated payment")
	}
}

func TestRiftFloorSeveralAdvancesKeepCumulativeProgress(t *testing.T) {
	s, b, _ := seededRiftFloorFixture(t)
	start, tier, progress := s.rift.data.Run.StartedMS, s.rift.data.Run.Tier, s.rift.data.Run.Progress
	oldIDs := map[domain.EntityID]bool{}
	for floor := int32(1); floor <= 4; floor++ {
		for id := range s.monsters {
			if oldIDs[id] {
				t.Fatalf("floor %d reused source monster %d", floor, id)
			}
			oldIDs[id] = true
		}
		for id, monster := range s.monsters {
			if s.rift.data.Run.Monsters[id].Class == 0 {
				s.onEndgameRiftMonsterDeath(monster, s.players[1])
				progress += domain.RiftNormalProgress
				break
			}
		}
		target := prepareRiftFloorTarget(t, s)
		p := s.players[1]
		p.Pos = s.rift.data.Floor.Exit
		s.advanceEndgameRift(p, advanceRiftFloorRequest(s))
		b.finishFloorWrite(t, int(floor-1), true, false)
		s.stepEndgameRift()
		target.drain()
		target.stepEndgameRift()
		if target.rift.data.Floor.FloorIndex != floor+1 || target.rift.data.Run.Progress != progress || target.rift.data.Run.StartedMS != start || target.rift.data.Run.Tier != tier || target.rift.revision != 5+int64(floor) {
			t.Fatalf("floor %d lost cumulative identity/progress/revision", floor+1)
		}
		if target.players[1].Player.Bag.CountOf(domain.RiftKeyItemID) != 3 || b.commits != 0 {
			t.Fatal("repeated traversal incurred extra economic charge")
		}
		s = target
	}
}

func TestRiftFloorLateCharacterCommitCompletesBeforePlayerHandoff(t *testing.T) {
	s, b, _ := seededRiftFloorFixture(t)
	target := prepareRiftFloorTarget(t, s)
	p := s.players[1]
	s.advanceEndgameRift(p, advanceRiftFloorRequest(s))
	// A non-rift character operation can start while the floor write is in
	// flight. Its inventory lease must keep this actor's player ownership alive.
	candidate := s.snapshotOf(p).Clone()
	candidate.Bag.AddStack(s.items[1900028202], domain.Stack{Item: 1900028202, Count: 10, Bound: true})
	callbackRan := false
	if !s.endgameCommit(p, domain.EndgameRequest{Op: "socket.insert", RequestID: "late-character-operation", ExpectedRevision: "0"}, candidate, domain.EndgameResponse{Kind: "socket.insert"}, nil, 0, func() { callbackRan = true }) {
		t.Fatal("test character operation did not acquire its lease")
	}
	b.finishFloorWrite(t, 0, true, false)
	s.stepEndgameRift()
	if !s.rift.retired || s.players[1] == nil || len(target.handoffs) != 0 || callbackRan {
		t.Fatal("floor moved player ownership ahead of character commit")
	}
	select {
	case <-s.quit:
		t.Fatal("source stopped before pending character callback")
	default:
	}
	b.finish(true)
	s.finishEndgame(false)
	s.stepEndgameRift()
	target.drain()
	if !callbackRan || len(s.players) != 0 || target.players[1] == nil || target.players[1].Player.Bag.CountOf(1900028202) != 10 || b.commits != 1 {
		t.Fatal("late commit was lost/duplicated across floor handoff")
	}
}

func TestRiftFloorInitSuppressesReusedMapPublicServices(t *testing.T) {
	s, _, _ := seededRiftFloorFixture(t)
	npc := &entity.Entity{ID: 7654321, Kind: domain.KindNPC, Name: "公共地图商人", Pos: s.rift.data.Floor.Entry}
	s.entities[npc.ID] = npc
	s.aoi.Enter(npc)
	s.portals = []domain.Portal{{}}
	s.spawner = &spawn.Spawner{}
	s.skipNPC = false
	player := s.players[1]
	s.initEndgameRift(s.rift.data.clone())
	if !s.skipNPC || s.entities[npc.ID] != nil || s.spawner != nil || len(s.portals) != 0 || len(s.monsters) != 0 {
		t.Fatal("rift initialization retained public NPC services, portals, or native spawns")
	}
	if _, exists := s.aoi.at[npc.ID]; exists {
		t.Fatal("removed public NPC remained in spatial index")
	}
	if s.players[1] != player || s.entities[1] != player {
		t.Fatal("public-service suppression removed admitted player")
	}
}
