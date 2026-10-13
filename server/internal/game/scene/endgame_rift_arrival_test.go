package scene

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func arrivalRiftRulesFixture(t *testing.T) *domain.RiftRuleTable {
	t.Helper()
	prior := rebalanceRiftRulesFixture(t)
	var rows []domain.RiftRules
	for v := int32(1); v <= 4; v++ {
		r, _ := prior.Version(v)
		rows = append(rows, *r)
	}
	next := *prior.Active()
	next.Version = 5
	next.GuardianArrivalMS = 5000
	next.ExitName = "归元道使"
	rows = append(rows, next)
	table, err := domain.NewRiftRuleTable(5, rows, weightedSceneLootFixture(t), balanceItemsFixture())
	if err != nil {
		t.Fatal(err)
	}
	return table
}

func arrivalRiftFixture(t *testing.T) (*Scene, *riftFloorTestBackend, *int64) {
	t.Helper()
	s, b, now := seededRiftFloorFixture(t)
	s.riftRules = arrivalRiftRulesFixture(t)
	r := s.rift.data.Run
	r.Rules = s.riftRules.Active()
	r.RulesVersion = 5
	s.rift.data.Version = 2
	s.rift.data.Grants = map[domain.CharID]endgameRiftGrant{}
	s.rift.data.Profiles = map[domain.CharID]endgameRiftIdentity{}
	s.levels = domain.NewLevelTable(map[int32]int64{60: 100000000, 61: 200000000})
	s.players[1].Pos = domain.Pos{MapID: s.id.MapID, X: 700, Y: 700}
	s.defs = endgameRiftDefs{base: s.defs, trial: map[domain.MonsterID]domain.MonsterDef{2: {ID: 2, Name: "夜巡蛟", Sprite: 2, Kind: domain.MonsterBoss, HP: 1000}}}
	b.runs[r.RunID] = s.rift.data.record(s.rift.revision)
	return s, b, now
}

func triggerRiftArrival(t *testing.T, s *Scene, globe bool) {
	t.Helper()
	r := s.rift.data.Run
	if globe {
		r.Progress = r.Rules.ProgressMax - r.Rules.GlobeProgress
		r.Globes[999999] = domain.RiftGlobe{ID: 999999, Pos: s.players[1].Pos, Value: r.Rules.GlobeProgress}
		s.stepEndgameRift()
	} else {
		r.Progress = r.Rules.ProgressMax - r.Rules.NormalProgress
		var source *entity.Entity
		for id, m := range s.monsters {
			if r.Monsters[id].Class == 0 {
				source = m
				break
			}
		}
		if source == nil {
			t.Fatal("missing trash")
		}
		s.onEndgameRiftMonsterDeath(source, s.players[1])
		s.stepEndgameRift()
	}
	if r.Phase != domain.RiftBossArriving || s.rift.data.GuardianArrival == nil || len(s.monsters) != 0 || s.riftCombatReady() {
		t.Fatal("progress did not start noncombat arrival", r.Phase, len(s.monsters))
	}
}

func TestRiftGuardianArrivalFiveSecondDurableGate(t *testing.T) {
	for _, globe := range []bool{false, true} {
		t.Run(map[bool]string{false: "kill", true: "globe"}[globe], func(t *testing.T) {
			s, b, now := arrivalRiftFixture(t)
			triggerRiftArrival(t, s, globe)
			a := *s.rift.data.GuardianArrival
			if a.StartedAtMS != *now || a.DeadlineMS-a.StartedAtMS != 5000 || a.Anchor != s.players[1].Pos || a.AnchorCharID != 100 || riftDistance2(a.Pos, a.Anchor) < 64*64 || riftDistance2(a.Pos, a.Anchor) > 225*225 || len(b.writes) != 1 {
				t.Fatalf("incorrect plan %+v writes%d", a, len(b.writes))
			}
			frozen := s.rift.data.Floor.Guardian.Point.Pos
			b.finishFloorWrite(t, 0, true, false)
			s.stepEndgameRift()
			*now = a.DeadlineMS - 1
			s.stepEndgameRift()
			if len(s.monsters) != 0 || s.riftCombatReady() {
				t.Fatal("guardian materialized before 5000ms")
			}
			view := s.riftView(s.players[1], s.rift.data, true, true)
			if view.GuardianArrival == nil || view.GuardianArrival.RemainingMS != 1 {
				t.Fatal("missing authoritative countdown", view.GuardianArrival)
			}
			*now = a.DeadlineMS
			s.stepEndgameRift()
			if s.rift.data.Run.Phase != domain.RiftBoss || len(s.monsters) != 1 || !s.riftCombatReady() {
				t.Fatal("guardian missing after durable deadline")
			}
			var id domain.EntityID
			for key, m := range s.monsters {
				id = key
				if m.Pos != a.Pos || m.Name != "夜巡蛟" {
					t.Fatal("guardian lost chosen position or actual name", m.Pos, m.Name)
				}
			}
			for i := 0; i < 5; i++ {
				s.stepEndgameRift()
				s.populateEndgameRift(true)
			}
			if len(s.monsters) != 1 || s.monsters[id] == nil || s.rift.data.Floor.Guardian.Point.Pos != frozen {
				t.Fatal("duplicate guardian or mutated floor plan")
			}
			view = s.riftView(s.players[1], s.rift.data, true, true)
			if view.GuardianArrival != nil || len(view.Auras) != 1 || view.Auras[0].OriginalName != "夜巡蛟" {
				t.Fatal("arrival/name projection wrong", view)
			}
		})
	}
}

func TestRiftGuardianArrivalLostACKAndRetryNeverReset(t *testing.T) {
	for _, durable := range []bool{false, true} {
		t.Run(map[bool]string{false: "rollback", true: "lost_ack"}[durable], func(t *testing.T) {
			s, b, now := arrivalRiftFixture(t)
			triggerRiftArrival(t, s, false)
			a := *s.rift.data.GuardianArrival
			*now = a.DeadlineMS
			s.stepEndgameRift()
			if len(s.monsters) != 0 {
				t.Fatal("spawned without durable plan")
			}
			b.finishFloorWrite(t, 0, durable, true)
			s.stepEndgameRift()
			s.stepEndgameRift()
			if !reflect.DeepEqual(a, *s.rift.data.GuardianArrival) {
				t.Fatal("reload reset plan")
			}
			if !durable {
				if len(s.monsters) != 0 {
					t.Fatal("rollback spawned")
				}
				*now += 1000
				s.stepEndgameRift()
				if len(b.writes) != 2 || b.cas[1] != b.cas[0] {
					t.Fatal("retry missed original CAS")
				}
				b.finishFloorWrite(t, 1, true, false)
				s.stepEndgameRift()
			}
			if len(s.monsters) != 1 || s.rift.data.Run.Phase != domain.RiftBoss {
				t.Fatal("durable recovery did not materialize exactly once")
			}
		})
	}
}

func TestRiftGuardianArrivalReadyReloadAndReconnect(t *testing.T) {
	s, b, now := arrivalRiftFixture(t)
	triggerRiftArrival(t, s, false)
	b.finishFloorWrite(t, 0, true, false)
	s.stepEndgameRift()
	a := *s.rift.data.GuardianArrival
	record := b.runs[s.rift.data.Run.RunID]
	d, ok := decodeEndgameRift(&record, s.riftRules)
	if !ok {
		t.Fatal("persisted arrival failed to decode")
	}
	*now += 2000
	if !s.initEndgameRift(d) {
		t.Fatal("scene reconstruction failed")
	}
	s.stepEndgameRift()
	if !reflect.DeepEqual(a, *s.rift.data.GuardianArrival) || len(s.monsters) != 0 {
		t.Fatal("loading reset arrival")
	}
	s.syncEndgameRift(s.players[1])
	s.syncEndgameRift(s.players[1])
	s.stepEndgameRift()
	s.disconnectEndgameRift(s.players[1])
	*now = a.DeadlineMS - 1
	s.syncEndgameRift(s.players[1])
	s.stepEndgameRift()
	if len(s.monsters) != 0 || !reflect.DeepEqual(a, *s.rift.data.GuardianArrival) {
		t.Fatal("ready/reconnect reset arrival")
	}
	*now++
	s.stepEndgameRift()
	s.stepEndgameRift()
	if len(s.monsters) != 1 {
		t.Fatal("ready reconstruction duplicated/missed guardian")
	}
}

func TestRiftGuardianArrivalAnchorDeterminismAndSafeFootprint(t *testing.T) {
	for attempt := 0; attempt < 8; attempt++ {
		s, _, _ := arrivalRiftFixture(t)
		for _, char := range []int64{300, 200} {
			id := domain.EntityID(char)
			s.rift.data.Run.Members[domain.CharID(char)] = &domain.RiftMember{CharID: domain.CharID(char), Level: 60}
			s.router.riftAdmissions[s.id].Allowed[domain.CharID(char)] = true
			join(t, s, id, char, "队员", 900, 900)
			p := s.players[id]
			p.HP = 100
			p.MaxHP = 100
			p.Pos.MapID = s.id.MapID
			s.rift.data.Run.Members[domain.CharID(char)] = &domain.RiftMember{CharID: domain.CharID(char), Level: 60}
			s.rift.ready[domain.CharID(char)] = true
		}
		s.players[1].Player.Protect(s.tick + 100)
		if s.endgameRiftGuardianAnchor() != s.players[1] {
			t.Fatal("living ready payer/leader was not anchor during revive protection")
		}
		s.rift.ready[100] = false
		if s.endgameRiftGuardianAnchor() != s.players[200] {
			t.Fatal("fallback depended on map order")
		}
		s.players[200].Pos = domain.Pos{MapID: s.id.MapID, X: 600, Y: 600}
		s.players[300].Pos = domain.Pos{MapID: s.id.MapID, X: 300, Y: 300}
		// The original guardian point can be safe, but a wall to the east must not
		// be crossed and the complete landing footprint must remain on this side.
		s.walkable = func(p domain.Pos) bool { return p.MapID == s.id.MapID && p.X >= 0 && p.Y >= 0 && p.X < 620 }
		s.rift.data.Run.Phase = domain.RiftBossArriving
		s.rift.data.Run.Progress = s.rift.data.Run.Rules.ProgressMax
		s.beginEndgameRiftGuardian()
		a := s.rift.data.GuardianArrival
		if a == nil || a.AnchorCharID != 200 || a.Pos.X+24 >= 620 || !s.endgameRiftGuardianPositionSafe(a.Anchor, a.Pos) {
			t.Fatal("unsafe or non-deterministic anchor", a)
		}
	}
}

func TestRiftGuardianArrivalOccupiedLandingAndTerminalCancel(t *testing.T) {
	s, b, now := arrivalRiftFixture(t)
	triggerRiftArrival(t, s, false)
	a := *s.rift.data.GuardianArrival
	b.finishFloorWrite(t, 0, true, false)
	s.stepEndgameRift()
	original := s.players[1].Pos
	s.players[1].Pos = a.Pos
	*now = a.DeadlineMS
	s.stepEndgameRift()
	if len(s.monsters) != 0 || !reflect.DeepEqual(a, *s.rift.data.GuardianArrival) {
		t.Fatal("overlap or timer reset")
	}
	s.players[1].Pos = original
	s.stepEndgameRift()
	if len(s.monsters) != 1 {
		t.Fatal("landing failed after obstruction moved")
	}
	for _, phase := range []domain.RiftPhase{domain.RiftCompleted, domain.RiftFailed, domain.RiftAborted, domain.RiftClosed} {
		t.Run(string(phase), func(t *testing.T) {
			s, b, now := arrivalRiftFixture(t)
			triggerRiftArrival(t, s, false)
			b.finishFloorWrite(t, 0, true, false)
			s.stepEndgameRift()
			s.rift.data.Run.Phase = phase
			s.rift.data.Run.Revision++
			s.rift.data.GrantsReady = true
			*now = s.rift.data.GuardianArrival.DeadlineMS + 10000
			s.stepEndgameRiftGuardian(*now)
			s.populateEndgameRift(true)
			if len(s.monsters) != 0 || s.riftCombatReady() || riftGuardianArrivalView(s.rift.data, *now) != nil {
				t.Fatal("terminal timer resurrected guardian")
			}
		})
	}
}

func TestRiftGuardianArrivalEarlyDeathAndLegacyContract(t *testing.T) {
	s, _, now := arrivalRiftFixture(t)
	triggerRiftArrival(t, s, false)
	r := s.rift.data.Run
	r.RegisterMonster(999998, 3)
	if _, ok := r.RecordKill(999998, *now+1); ok || r.Monsters[999998].Dead || r.Phase != domain.RiftBossArriving {
		t.Fatal("early death bypassed materialization")
	}
	for v := int32(1); v <= 4; v++ {
		legacy, _ := s.riftRules.Version(v)
		run, err := domain.NewRiftRun("legacy", 1, 100, []domain.TrialKeyMember{{CharID: 100, Level: 60}}, *now, legacy)
		if err != nil {
			t.Fatal(err)
		}
		run.Activate(*now)
		run.Progress = legacy.ProgressMax - legacy.NormalProgress
		run.RegisterMonster(1, 0)
		run.RecordKill(1, *now+1)
		raw, _ := json.Marshal(run.Rules)
		if run.Phase != domain.RiftBoss || legacy.GuardianArrivalMS != 0 || string(raw) == "" {
			t.Fatal("historical contract changed", v)
		}
	}
}

func TestRiftGuardianArrivalRejectsCorruptPersistedPlan(t *testing.T) {
	s, _, _ := arrivalRiftFixture(t)
	triggerRiftArrival(t, s, false)
	for _, change := range []func(*endgameRiftGuardianArrival){func(a *endgameRiftGuardianArrival) { a.DeadlineMS++ }, func(a *endgameRiftGuardianArrival) { a.Seed++ }, func(a *endgameRiftGuardianArrival) { a.Pos.X += 1000 }, func(a *endgameRiftGuardianArrival) { a.AnchorCharID = 999 }} {
		d := s.rift.data.clone()
		change(d.GuardianArrival)
		record := d.record(6)
		if _, ok := decodeEndgameRift(&record, s.riftRules); ok {
			t.Fatal("accepted corrupt arrival")
		}
	}
}

func TestRiftGuardianArrivalHasNoAbilityShouts(t *testing.T) {
	s, _, now := arrivalRiftFixture(t)
	var source *entity.Entity
	for id, m := range s.monsters {
		if s.rift.abilities[id] != nil {
			source = m
			break
		}
	}
	if source == nil {
		t.Fatal("missing elite")
	}
	source.Monster.Target = s.players[1].ID
	source.Pos = s.players[1].Pos
	s.rift.abilities[source.ID].nextAt = *now
	s.flush()
	s.players[1].Player.Sink.(*fakeSink).take()
	s.stepEndgameRiftAbilities(*now)
	s.flush()
	if s.rift.abilities[source.ID].pending == nil {
		t.Fatal("no test cast")
	}
	for _, ev := range s.players[1].Player.Sink.(*fakeSink).take() {
		if _, ok := ev.(event.MonsterSpoke); ok {
			t.Fatal("extra ability shout emitted")
		}
	}
}

func TestRiftGuardianArrivalPriorActiveSaveLostACKPreservesProgress(t *testing.T) {
	s, b, now := arrivalRiftFixture(t)
	s.rift.data.Run.Revision++
	*now += 5000
	s.stepEndgameRift()
	if len(b.writes) != 1 {
		t.Fatal("missing active autosave")
	}
	triggerRiftArrival(t, s, false)
	a := *s.rift.data.GuardianArrival
	b.finishFloorWrite(t, 0, true, true)
	s.stepEndgameRift()
	s.stepEndgameRift()
	if s.rift.data.Run.Phase != domain.RiftBossArriving || !reflect.DeepEqual(a, *s.rift.data.GuardianArrival) || s.rift.data.Run.Progress != s.rift.data.Run.Rules.ProgressMax {
		t.Fatal("older active snapshot replaced full progress")
	}
	if s.rift.savedRevision >= a.Revision {
		t.Fatal("arrival was incorrectly declared durable")
	}
	*now += 1000
	s.stepEndgameRift()
	if len(b.writes) != 2 || b.cas[1] != b.writes[0].Revision {
		t.Fatal("did not advance verified storage CAS")
	}
	b.finishFloorWrite(t, 1, true, false)
	s.stepEndgameRift()
	*now = a.DeadlineMS
	s.stepEndgameRift()
	if len(s.monsters) != 1 || s.rift.data.Run.Phase != domain.RiftBoss {
		t.Fatal("empty arena after active save recovery")
	}
}

func TestRiftGuardianArrivalBossSaveLostACKPreservesCompletion(t *testing.T) {
	for _, queued := range []bool{false, true} {
		t.Run(map[bool]string{false: "death_before_ack", true: "death_during_reconcile"}[queued], func(t *testing.T) {
			s, b, now := arrivalRiftFixture(t)
			triggerRiftArrival(t, s, false)
			b.finishFloorWrite(t, 0, true, false)
			s.stepEndgameRift()
			*now = s.rift.data.GuardianArrival.DeadlineMS
			s.stepEndgameRift()
			if len(b.writes) != 2 || len(s.monsters) != 1 {
				t.Fatal("missing pending boss snapshot")
			}
			var boss *entity.Entity
			for _, m := range s.monsters {
				boss = m
			}
			if queued {
				b.finishFloorWrite(t, 1, true, true)
				s.stepEndgameRift()
				if !s.rift.economicPending {
					t.Fatal("reconciliation not quarantined")
				}
			}
			boss.HP = 0
			s.onDeath(boss, s.players[1].ID)
			if !queued {
				b.finishFloorWrite(t, 1, true, true)
				s.stepEndgameRift()
			}
			s.stepEndgameRift()
			if s.rift.data.Run.Phase != domain.RiftCompleted || !s.rift.data.GrantsReady || s.rift.savedRevision >= s.rift.data.Run.Revision || b.commits != 0 {
				t.Fatal("completion replaced or published too soon", s.rift.data.Run.Phase)
			}
			grant := s.rift.data.clone().Grants
			*now += 1000
			s.stepEndgameRift()
			if len(b.writes) != 3 || b.cas[2] != b.writes[1].Revision {
				t.Fatal("completion retry did not use reconciled CAS")
			}
			for i := 0; i < 4; i++ {
				s.stepEndgameRift()
			}
			for _, m := range s.monsters {
				if m.Alive() {
					t.Fatal("guardian resurrected")
				}
			}
			if !reflect.DeepEqual(grant, s.rift.data.Grants) {
				t.Fatal("completion grant rerolled")
			}
			b.finishFloorWrite(t, 2, true, false)
			s.stepEndgameRift()
			if b.commits != 1 || s.rift.savedRevision != s.rift.data.Run.Revision {
				t.Fatal("durable completion not published exactly once")
			}
		})
	}
}

func TestRiftGuardianArrivalQuietWireFixtures(t *testing.T) {
	s, _, _ := arrivalRiftFixture(t)
	triggerRiftArrival(t, s, false)
	arrival := domain.EndgameResponse{Kind: "rift.state", OK: true, Rift: s.riftView(s.players[1], s.rift.data, false, true)}
	interrupted := domain.EndgameResponse{Kind: "rift.state", OK: true, Rift: s.riftView(s.players[1], s.rift.data, true, false)}
	if interrupted.Rift.Phase != "interrupted" || interrupted.Rift.GuardianArrival != nil || !interrupted.Rift.Claimable {
		t.Fatal("orphaned arrival broke compensation projection")
	}
	s, b, _ := completedRiftRewards(t)
	finishRiftRewardCommit(t, s, b)
	p := s.players[1]
	drop := ownedRiftDrop(s, 100)
	p.Pos = drop.Pos
	originalBag := p.Player.Bag
	p.Player.Bag = domain.NewBag(1)
	p.Player.Bag.Add(s.items[1900028202], domain.MaxStack)
	s.flush()
	p.Player.Sink.(*fakeSink).take()
	s.onPickUp(PickUp{ID: p.ID, Target: drop.ID})
	s.flush()
	var bagfull domain.EndgameResponse
	for _, ev := range p.Player.Sink.(*fakeSink).take() {
		if reply, ok := ev.(event.EndgameMessage); ok {
			bagfull = reply.Message
			continue
		}
		t.Fatalf("unexpected player-facing bag-full event %T", ev)
	}
	if bagfull.Error != "bag_full" || bagfull.Message != "" || bagfull.Rift == nil || bagfull.Rift.PendingLoot < 1 || s.ground[drop.ID] == nil {
		t.Fatal("quiet bag-full response lost observable entitlement", bagfull)
	}
	p.Player.Bag = originalBag
	s.onPickUp(PickUp{ID: p.ID, Target: drop.ID})
	finishRiftRewardCommit(t, s, b)
	s.flush()
	for _, ev := range p.Player.Sink.(*fakeSink).take() {
		if reply, ok := ev.(event.EndgameMessage); ok && reply.Message.Kind == "rift.reward" && reply.Message.Message != "" {
			t.Fatal("per-item success generated chat text")
		}
	}
	if path := os.Getenv("FANTASY_RIFT_WIRE_FIXTURE"); path != "" {
		raw, err := json.MarshalIndent(map[string]domain.EndgameResponse{"arrival": arrival, "bagFull": bagfull, "interrupted": interrupted}, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

type riftArrivalShutdownBackend struct {
	*riftTestBackend
	lostACKs int
}

func (b *riftArrivalShutdownBackend) SaveRun(record domain.EndgameRunRecord, expected int64) <-chan domain.EndgameRunSaveResult {
	result := <-b.riftTestBackend.SaveRun(record, expected)
	if b.lostACKs > 0 {
		b.lostACKs--
		result.Err = fmt.Errorf("committed shutdown ACK lost")
	}
	ch := make(chan domain.EndgameRunSaveResult, 1)
	ch <- result
	return ch
}

func TestRiftGuardianArrivalShutdownPreservesNewerAuthoritativeState(t *testing.T) {
	for _, stage := range []string{"active_to_arrival", "boss_death_before_ack", "boss_death_during_reconcile"} {
		t.Run(stage, func(t *testing.T) {
			s, b, now := arrivalRiftFixture(t)
			if stage == "active_to_arrival" {
				s.rift.data.Run.Revision++
				*now += 5000
				s.stepEndgameRift()
				triggerRiftArrival(t, s, false)
				b.finishFloorWrite(t, 0, true, true)
			} else {
				triggerRiftArrival(t, s, false)
				b.finishFloorWrite(t, 0, true, false)
				s.stepEndgameRift()
				*now = s.rift.data.GuardianArrival.DeadlineMS
				s.stepEndgameRift()
				b.finishFloorWrite(t, 1, true, true)
				if stage == "boss_death_during_reconcile" {
					s.stepEndgameRift()
				}
				var boss *entity.Entity
				for _, m := range s.monsters {
					boss = m
				}
				boss.HP = 0
				s.onDeath(boss, s.players[1].ID)
			}
			arrival := *s.rift.data.GuardianArrival
			// Shutdown's final snapshot itself loses an ACK after committing too.
			s.endgameBackend = &riftArrivalShutdownBackend{riftTestBackend: b.riftTestBackend, lostACKs: 1}
			s.finishEndgameRunWrites()
			record := b.runs[s.rift.data.Run.RunID]
			durable, ok := decodeEndgameRift(&record, s.riftRules)
			if !ok || !reflect.DeepEqual(arrival, *durable.GuardianArrival) || s.rift.savedRevision != s.rift.data.Run.Revision || s.rift.reconcilingSave != nil || len(s.rift.delayedDeaths) != 0 {
				t.Fatal("shutdown failed durable reconciliation")
			}
			if stage == "active_to_arrival" {
				if durable.Run.Phase != domain.RiftBossArriving || durable.Run.Progress != durable.Run.Rules.ProgressMax {
					t.Fatal("shutdown discarded full progress")
				}
			} else if durable.Run.Phase != domain.RiftCompleted || !durable.GrantsReady || !reflect.DeepEqual(durable.Grants, s.rift.data.Grants) {
				t.Fatal("shutdown discarded completion grants")
			}
		})
	}
}

func TestRiftGuardianArrivalTimeoutBeforeMaterialization(t *testing.T) {
	s, b, now := arrivalRiftFixture(t)
	triggerRiftArrival(t, s, false)
	b.finishFloorWrite(t, 0, true, false)
	s.stepEndgameRift()
	*now = s.rift.data.Run.StartedMS + s.rift.data.Run.Rules.HardMS
	s.stepEndgameRift()
	if s.rift.data.Run.Phase != domain.RiftFailed || s.rift.data.Run.Reason != "timeout" || len(s.monsters) != 0 || s.riftCombatReady() {
		t.Fatal("arrival bypassed hard timeout")
	}
}
