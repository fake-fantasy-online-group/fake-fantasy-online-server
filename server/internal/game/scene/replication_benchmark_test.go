package scene

// Opt-in diagnostic benchmarks. Private incident data is read from an explicit
// external path and is never copied into the repository or printed in results.
// Native character/monster templates are substituted by existing test fixtures.
import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
)

type replicationMeasureSink struct {
	globalResponse                                            domain.EndgameResponse
	haveGlobal                                                bool
	frames, bytes, monsters, auras, globalFrames, globalBytes int
	closed                                                    bool
}

func (q *replicationMeasureSink) Emit(e event.Event) {
	if v, ok := e.(event.EntitySpawned); ok && v.Kind == domain.KindMonster {
		q.monsters++
	}
	global := false
	if v, ok := e.(event.EndgameMessage); ok {
		global = true
		if v.Message.Rift != nil {
			q.auras = max(q.auras, len(v.Message.Rift.Auras))
			q.globalResponse = v.Message
			q.haveGlobal = true
		}
	}
	for _, packet := range protocol.Encode(1, e) {
		q.frames++
		q.bytes += len(packet)
		if global {
			q.globalFrames++
			q.globalBytes += len(packet)
		}
	}
}

// Called outside the timed/allocated region: semantic assertions do not inflate
// the production encode benchmark.
func (q *replicationMeasureSink) globalDigest() string {
	if !q.haveGlobal {
		return ""
	}
	response := q.globalResponse
	view := *response.Rift
	view.Auras = append([]domain.EndgameRiftAuraView(nil), view.Auras...)
	sort.Slice(view.Auras, func(i, j int) bool { return view.Auras[i].EntityID < view.Auras[j].EntityID })
	response.Rift = &view
	raw, _ := json.Marshal(response)
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}

func (q *replicationMeasureSink) TryEmit(e event.Event) bool { q.Emit(e); return true }
func (q *replicationMeasureSink) CancelPendingEmit()         {}
func (q *replicationMeasureSink) Close()                     { q.closed = true }

func replicationCapturedFixture(t *testing.T) *Scene {
	t.Helper()
	path := os.Getenv("RIFT_REPLICATION_FIXTURE")
	if path == "" {
		t.Skip("set RIFT_REPLICATION_FIXTURE to an external sanitized snapshot")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot struct{ Payload endgameRiftPersisted }
	if err = json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatal(err)
	}
	d := snapshot.Payload
	if d.Run == nil || d.Run.Rules == nil || d.Floor == nil || len(d.Run.Monsters) != 1160 {
		t.Fatal("expected1160-monster persisted fixture")
	}
	s := fullCoverageSceneFixture(t, 160, 160)
	s.log = slog.New(slog.NewTextHandler(io.Discard, nil))
	p := s.players[1]
	d.Run.Phase = domain.RiftActive
	d.Run.Reason = ""
	d.Run.Monsters = map[domain.EntityID]*domain.RiftMonster{}
	d.Elite = nil
	s.rift.data = d
	s.rift.floorLoading = false
	s.rift.savedRevision = d.Run.Revision
	for id := range d.Run.Members {
		p.Player.Char.ID = int64(id)
		break
	}
	p.Player.Char.Level = 35
	s.trial.Monsters[35] = s.trial.Monsters[60]
	s.now = func() time.Time { return time.UnixMilli(d.Run.LastMS) }
	s.id.MapID = d.Floor.MapID
	p.Pos = d.Floor.Entry
	s.rift.data.MapID = s.id.MapID
	s.walkable = func(pos domain.Pos) bool { return pos.MapID == s.id.MapID }
	defs := s.defs.(spawn.MapDefs)
	base := defs[s.riftLayouts[0].Monsters[0]]
	for _, point := range d.Floor.Spawns {
		def := base
		def.ID = point.Point.Monster
		defs[def.ID] = def
	}
	for n := 0; n < 995; n++ {
		s.alloc.Monster()
	}
	s.populateEndgameRift(false)
	if len(s.monsters) != 1160 {
		t.Fatal("unexpected populated count", len(s.monsters))
	}
	s.flush()
	p.Player.Sink.(*fakeSink).take()
	s.cancelReplication(p.ID)
	s.riftVisibility = nil
	s.aoi.onChange = nil
	return s
}

// This is the pre-change full-map behavior, retained only as a measurement
// reference. It deliberately has no queue admission or staged-delivery safety.
func replicationLegacyFlush(s *Scene) {
	for _, item := range s.outbox {
		for id, p := range s.players {
			if (item.to == 0 || item.to == id) && id != item.except && s.canDeliverWorldEvent(id, item.ev) {
				p.Player.Sink.Emit(item.ev)
			}
		}
	}
	clear(s.outbox)
	s.outbox = s.outbox[:0]
}
func replicationLegacyInitial(s *Scene, p *entity.Entity) {
	s.syncEndgameRift(p)
	if s.trial.IsMap(s.id.MapID) {
		s.emitTo(p.ID, event.NewbieTextNotice{Who: p.ID, Text: "进入妖魔结界！"})
	}
	s.pushNpcChatter(p.ID)
	s.replayGroundEffects(p.ID)
	for _, o := range s.entities {
		if s.entityMapLoading(o) || o.ID == p.ID || (o.Kind == domain.KindNPC && s.skipNPC && !s.isEndgameRiftExitNPC(o)) {
			continue
		}
		if o.Kind == domain.KindPet && o.Pet != nil {
			if owner := s.players[o.Pet.Owner]; owner != nil && owner.Player.Riding {
				continue
			}
		}
		if o.Kind == domain.KindTrap && (o.Trap == nil || !o.Trap.VisibleToPlayer(p.ID)) {
			continue
		}
		s.outbox = append(s.outbox, pending{to: p.ID, ev: s.withNativeAttackRange(s.spawnEvent(o))})
		s.sendEntityStatusEffects(p.ID, o)
	}
	s.sendIdleMonsterGreetingTo(p)
	s.sendStallSigns(p.ID)
	for _, status := range s.visibleStatuses(p) {
		if status.effect.Effect != "" {
			s.emitTo(p.ID, status.effect)
		}
	}
	s.emitTo(p.ID, s.ridingEvent(p))
	s.refreshStats(p)
	s.emitTo(p.ID, event.AppearanceChanged{Who: p.ID, Appearance: p.Look.Appearance})
	s.emitTo(p.ID, s.buffSnapshot(p))
	replicationLegacyFlush(s)
}
func replicationDrainMeasured(t *testing.T, s *Scene) int {
	t.Helper()
	ticks := 0
	for {
		pending := false
		for _, r := range s.replication {
			if len(r.queue) > 0 || r.initialAt < len(r.initial) {
				pending = true
			}
			if r.closed {
				t.Fatal("measurement stream closed")
			}
		}
		if !pending {
			return ticks
		}
		if ticks > 10000 {
			t.Fatal("replication did not drain")
		}
		s.tick++
		s.flush()
		ticks++
	}
}

type replicationMeasurement struct {
	PlayerWindowChanges, MonsterBlockCrosses, MembershipChanges                                int
	Case, Mode                                                                                 string
	Players, Frames, Bytes, MonsterSpawns, Auras, GlobalFrames, GlobalBytes, Ticks, DrainTicks int
	NS, CPUNS, AllocBytes, Allocs                                                              int64
}

func replicationPrintMeasurement(r replicationMeasurement) {
	b, _ := json.Marshal(r)
	fmt.Println("REPLICATION_BENCH " + string(b))
}

func TestReplicationBenchmarkInitial(t *testing.T) {
	if os.Getenv("RIFT_REPLICATION_BENCH") == "" {
		t.Skip("opt-in benchmark")
	}
	runtime.GOMAXPROCS(2)
	globalDigests := map[bool]string{}
	for _, cluster := range []bool{false, true} {
		for _, local := range []bool{false, true} {
			for iteration := -2; iteration < 10; iteration++ {
				s := replicationCapturedFixture(t)
				p := s.players[1]
				if cluster {
					for _, m := range s.monsters {
						s.aoi.Move(m, p.Pos)
					}
				}
				q := &replicationMeasureSink{}
				p.Player.Sink = q
				var before, after runtime.MemStats
				runtime.ReadMemStats(&before)
				cpuStart := replicationCPUTime()
				start := time.Now()
				ticks := 0
				if local {
					s.syncWorldAfterLoad(p)
					s.flush()
					ticks = replicationDrainMeasured(t, s)
				} else {
					replicationLegacyInitial(s, p)
				}
				elapsed := time.Since(start)
				cpuElapsed := replicationCPUTime() - cpuStart
				runtime.ReadMemStats(&after)
				if !local {
					globalDigests[cluster] = q.globalDigest()
				} else if q.globalDigest() != globalDigests[cluster] {
					t.Fatal("global response changed after canonical Aura ordering")
				}
				if q.auras != 1160 {
					t.Fatalf("global aura list changed: %d", q.auras)
				}
				if iteration >= 0 {
					r := replicationMeasurement{Case: "captured1160", Mode: "legacy-full", Players: 1, Frames: q.frames, Bytes: q.bytes, MonsterSpawns: q.monsters, Auras: q.auras, GlobalFrames: q.globalFrames, GlobalBytes: q.globalBytes, Ticks: ticks, NS: elapsed.Nanoseconds(), CPUNS: cpuElapsed, AllocBytes: int64(after.TotalAlloc - before.TotalAlloc), Allocs: int64(after.Mallocs - before.Mallocs)}
					if cluster {
						r.Case = "cluster1160"
					}
					if local {
						r.Mode = "production-block-paced"
					}
					replicationPrintMeasurement(r)
				}
			}
		}
	}
}

func TestReplicationBenchmarkMovement(t *testing.T) {
	if os.Getenv("RIFT_REPLICATION_BENCH") == "" {
		t.Skip("opt-in benchmark")
	}
	runtime.GOMAXPROCS(2)
	for _, players := range []int{1, 3, 5} {
		for _, local := range []bool{false, true} {
			for iteration := -1; iteration < 5; iteration++ {
				s := replicationCapturedFixture(t)
				base := s.players[1]
				for n := 2; n <= players; n++ {
					p := *base
					pl := *base.Player
					p.ID = domain.EntityID(n)
					p.Player = &pl
					s.players[p.ID] = &p
					s.entities[p.ID] = &p
					s.aoi.Enter(&p)
				}
				sinks := make([]*replicationMeasureSink, players)
				for n := 1; n <= players; n++ {
					p := s.players[domain.EntityID(n)]
					p.Pos.X += float64(n-1) * 600
					p.Player.Char.Level = 35
					q := &replicationMeasureSink{}
					p.Player.Sink = q
					sinks[n-1] = q
					if local {
						s.syncWorldAfterLoad(p)
					} else {
						replicationLegacyInitial(s, p)
					}
				}
				if local {
					s.flush()
					replicationDrainMeasured(t, s)
				}
				for _, q := range sinks {
					*q = replicationMeasureSink{}
				}
				windowChanges, monsterCrosses, membershipChanges := 0, 0, 0
				if local {
					changed := s.riftVisibility.changed
					s.riftVisibility.changed = func(id domain.EntityID, e *entity.Entity, visible bool) { membershipChanges++; changed(id, e, visible) }
				}
				ids := make([]int, 0, len(s.monsters))
				for id := range s.monsters {
					ids = append(ids, int(id))
				}
				sort.Ints(ids)
				var before, after runtime.MemStats
				runtime.ReadMemStats(&before)
				cpuStart := replicationCPUTime()
				start := time.Now()
				// 100 position reports at100ms, synthetic400units/s straight traversal.
				// Ten distinct monster movement events each report keep the full baseline
				// under the per-observer tick allowance; no artificial over-budget workload.
				for step := 0; step < 100; step++ {
					s.tick++
					for n := 1; n <= players; n++ {
						p := s.players[domain.EntityID(n)]
						pos := p.Pos
						pos.X += 40
						beforeWindow := riftBlockWindow{}
						if local {
							beforeWindow = s.riftVisibility.windows[p.ID]
						}
						s.aoi.Move(p, pos)
						if local && beforeWindow != s.riftVisibility.windows[p.ID] {
							windowChanges++
						}
						ev := event.EntityMoved{ID: p.ID, To: pos, Speed: 400}
						if local {
							s.emitExcept(ev, p.ID)
						} else {
							s.outbox = append(s.outbox, pending{ev: s.withNativeAttackRange(ev), except: p.ID})
						}
					}
					for j := 0; j < 10; j++ {
						m := s.monsters[domain.EntityID(ids[(step*10+j)%len(ids)])]
						pos := m.Pos
						pos.X += 15
						if visibilityBlock(m.Pos) != visibilityBlock(pos) {
							monsterCrosses++
						}
						s.aoi.Move(m, pos)
						ev := event.EntityMoved{ID: m.ID, To: pos, Speed: 150}
						if local {
							s.emit(ev)
						} else {
							s.outbox = append(s.outbox, pending{ev: s.withNativeAttackRange(ev)})
						}
					}
					if local {
						s.flush()
					} else {
						replicationLegacyFlush(s)
					}
				}
				drains := 0
				if local {
					drains = replicationDrainMeasured(t, s)
				}
				elapsed := time.Since(start)
				cpuElapsed := replicationCPUTime() - cpuStart
				runtime.ReadMemStats(&after)
				if iteration >= 0 {
					r := replicationMeasurement{Case: "captured1160-synthetic-walk400", PlayerWindowChanges: windowChanges, MonsterBlockCrosses: monsterCrosses, MembershipChanges: membershipChanges, Mode: "legacy-full", Players: players, Ticks: 100, DrainTicks: drains, NS: elapsed.Nanoseconds(), CPUNS: cpuElapsed, AllocBytes: int64(after.TotalAlloc - before.TotalAlloc), Allocs: int64(after.Mallocs - before.Mallocs)}
					if local {
						r.Mode = "production-block-paced"
					}
					for _, q := range sinks {
						r.Frames += q.frames
						r.Bytes += q.bytes
						r.MonsterSpawns += q.monsters
						if q.closed {
							t.Fatal("closed")
						}
					}
					replicationPrintMeasurement(r)
				}
			}
		}
	}
}

func TestReplicationBenchmarkCluster4097(t *testing.T) {
	if os.Getenv("RIFT_REPLICATION_BENCH") == "" {
		t.Skip("opt-in benchmark")
	}
	s, sinks := replicationScene(t, 1, true)
	replicationMonsters(s, 4097, 6000, 6000)
	q := &replicationMeasureSink{}
	s.players[1].Player.Sink = q
	cpuStart := replicationCPUTime()
	start := time.Now()
	s.sendAllEntities(1)
	s.flush()
	ticks := replicationDrainMeasured(t, s)
	if q.monsters != 4097 || q.closed {
		t.Fatal("incomplete dense cluster", q.monsters)
	}
	_ = sinks
	replicationPrintMeasurement(replicationMeasurement{Case: "synthetic-cluster4097", Mode: "production-block-paced", Players: 1, Frames: q.frames, Bytes: q.bytes, MonsterSpawns: q.monsters, Ticks: ticks, NS: time.Since(start).Nanoseconds(), CPUNS: replicationCPUTime() - cpuStart})
}
