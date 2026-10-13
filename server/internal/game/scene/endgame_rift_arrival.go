package scene

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"strconv"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

// Arrival is part of the durable run, not a client timer or a scheduled callback.
// Its seed, anchor and deadline are frozen once, including across lost ACKs,
// ready notifications, economic reloads and reconstruction of the scene actor.
type endgameRiftGuardianArrival struct {
	ID                      string
	Seed                    uint64
	AnchorCharID            domain.CharID
	Anchor                  domain.Pos
	Pos                     domain.Pos
	StartedAtMS, DeadlineMS int64
	Revision                int64
}

func riftGuardianArrivalView(d endgameRiftPersisted, now int64) *domain.EndgameRiftGuardianArrivalView {
	a := d.GuardianArrival
	if d.Run == nil || d.Run.Phase != domain.RiftBossArriving || a == nil {
		return nil
	}
	remaining := a.DeadlineMS - now
	if remaining < 0 {
		remaining = 0
	}
	if remaining > d.Run.Rules.GuardianArrivalMS {
		remaining = d.Run.Rules.GuardianArrivalMS
	}
	return &domain.EndgameRiftGuardianArrivalView{ID: a.ID, Seed: strconv.FormatUint(a.Seed, 10), AnchorCharID: strconv.FormatInt(int64(a.AnchorCharID), 10), X: int32(a.Pos.X), Y: int32(a.Pos.Y), StartedAtMS: a.StartedAtMS, DeadlineMS: a.DeadlineMS, DurationMS: int32(d.Run.Rules.GuardianArrivalMS), RemainingMS: int32(remaining)}
}

func (s *Scene) riftMonsterOriginalName(id domain.EntityID) string {
	if s.rift != nil {
		if registered := s.rift.data.Run.Monsters[id]; registered == nil || registered.Class == 0 {
			return ""
		}
		return s.rift.originalNames[id]
	}
	return ""
}

func (s *Scene) endgameRiftGuardianAnchor() *entity.Entity {
	var selected *entity.Entity
	r := s.rift.data.Run
	for _, p := range s.players {
		if p == nil || p.Player == nil || p.Player.Char == nil || !p.Alive() || p.Pos.MapID != s.id.MapID || s.entityMapLoading(p) {
			continue
		}
		char := domain.CharID(p.Player.Char.ID)
		if !s.rift.ready[char] || !r.Admitted(char) {
			continue
		}
		if char == r.Payer {
			return p
		}
		if selected == nil || p.Player.Char.ID < selected.Player.Char.ID {
			selected = p
		}
	}
	return selected
}

// Test a footprint, connectivity and occupied characters/NPCs. Disappearing
// trash cannot influence the chosen location or the deterministic anchor.
func (s *Scene) endgameRiftGuardianPositionSafe(anchor, pos domain.Pos) bool {
	if pos.MapID != s.id.MapID || s.walkable == nil || !s.straightPathWalkable(anchor, pos) {
		return false
	}
	for _, delta := range [][2]float64{{0, 0}, {24, 0}, {-24, 0}, {0, 24}, {0, -24}, {17, 17}, {-17, 17}, {17, -17}, {-17, -17}} {
		point := pos
		point.X += delta[0]
		point.Y += delta[1]
		if !s.walkable(point) {
			return false
		}
	}
	for _, p := range s.players {
		if p.Pos.MapID == pos.MapID && riftDistance2(pos, p.Pos) < 64*64 {
			return false
		}
	}
	for _, e := range s.entities {
		if e.Kind == domain.KindNPC && e.Pos.MapID == pos.MapID && riftDistance2(pos, e.Pos) < 80*80 {
			return false
		}
	}
	return true
}

func (s *Scene) planEndgameRiftGuardianArrival(now int64) bool {
	x := s.rift
	if x.data.GuardianArrival != nil {
		return true
	}
	anchor := s.endgameRiftGuardianAnchor()
	if anchor == nil {
		return false
	} // Map readiness must never start a hidden timer.
	floor := int32(0)
	if x.data.Floor != nil {
		floor = x.data.Floor.FloorIndex
	}
	id := fmt.Sprintf("%s/%d/guardian", x.data.Run.RunID, floor)
	hash := sha256.Sum256([]byte(id))
	seed := binary.LittleEndian.Uint64(hash[:8])
	for _, radius := range []float64{96, 128, 160, 192, 224} {
		for i := 0; i < 32; i++ {
			angle := float64((i+int(seed%32))%32) * 2 * math.Pi / 32
			pos := anchor.Pos
			pos.X = math.Round(pos.X + radius*math.Cos(angle))
			pos.Y = math.Round(pos.Y + radius*math.Sin(angle))
			if !s.endgameRiftGuardianPositionSafe(anchor.Pos, pos) {
				continue
			}
			x.data.Run.Revision++
			x.data.GuardianArrival = &endgameRiftGuardianArrival{ID: id, Seed: seed, AnchorCharID: domain.CharID(anchor.Player.Char.ID), Anchor: anchor.Pos, Pos: pos, StartedAtMS: now, DeadlineMS: now + x.data.Run.Rules.GuardianArrivalMS, Revision: x.data.Run.Revision}
			x.lastBroadcast = now - 1000
			return true
		}
	}
	// A configuration with no safe nearby footprint cannot silently use the old
	// distant exit or overlap the party. Preserve the existing failure/refund path.
	x.data.Run.Phase = domain.RiftFailed
	x.data.Run.Reason = "spawn_failed"
	x.data.Run.Revision++
	return false
}

func (s *Scene) beginEndgameRiftGuardian() {
	s.clearRiftMonsters()
	if s.rift.data.Run.Phase == domain.RiftBossArriving {
		s.planEndgameRiftGuardianArrival(s.riftMS())
	} else if s.rift.data.Run.Phase == domain.RiftBoss {
		s.populateEndgameRift(true)
	}
}

func (s *Scene) endgameRiftGuardianCanPopulate() bool {
	x := s.rift
	if x == nil || x.data.Run.Phase != domain.RiftBoss {
		return false
	}
	for id, m := range s.monsters {
		if registered := x.data.Run.Monsters[id]; registered != nil && registered.Class == 3 && !registered.Dead && m.Alive() {
			return false
		}
	}
	if x.data.Run.Rules.GuardianArrivalMS == 0 {
		return true
	}
	a := x.data.GuardianArrival
	return a != nil && !x.floorLoading && !x.economicPending && x.savedRevision >= a.Revision && s.riftMS() >= a.DeadlineMS && s.endgameRiftGuardianPositionSafe(a.Anchor, a.Pos)
}

func (s *Scene) stepEndgameRiftGuardian(now int64) {
	x := s.rift
	if x == nil || x.economicPending || x.floorLoading || x.retired || x.transition != nil || x.data.Run.Terminal() || x.data.Run.Rules.GuardianArrivalMS == 0 {
		return
	}
	r := x.data.Run
	if r.Phase == domain.RiftBossArriving {
		if !s.planEndgameRiftGuardianArrival(now) {
			return
		}
		a := x.data.GuardianArrival
		if x.savedRevision < a.Revision || now < a.DeadlineMS || !s.endgameRiftGuardianPositionSafe(a.Anchor, a.Pos) {
			return
		}
		// Deliberately no callback: timeout/abandon/completion is checked first on
		// every step, and an occupied landing site never resets or moves the timer.
		r.Phase = domain.RiftBoss
		r.Revision++
		x.lastBroadcast = now - 1000
	}
	if r.Phase == domain.RiftBoss {
		s.populateEndgameRift(true)
	}
}

func validRiftGuardianArrival(d endgameRiftPersisted) bool {
	r, a := d.Run, d.GuardianArrival
	if r.Phase == domain.RiftBossArriving && (r.Rules.GuardianArrivalMS == 0 || r.Progress != r.Rules.ProgressMax) {
		return false
	}
	if a == nil {
		return r.Rules.GuardianArrivalMS == 0 || r.Phase != domain.RiftBoss
	}
	if r.Rules.GuardianArrivalMS == 0 || a.Revision < 1 || a.Revision > r.Revision || a.StartedAtMS < r.StartedMS || a.DeadlineMS-a.StartedAtMS != r.Rules.GuardianArrivalMS || a.DeadlineMS < a.StartedAtMS || r.Members[a.AnchorCharID] == nil {
		return false
	}
	floor := int32(0)
	if d.Floor != nil {
		floor = d.Floor.FloorIndex
	}
	id := fmt.Sprintf("%s/%d/guardian", r.RunID, floor)
	hash := sha256.Sum256([]byte(id))
	if a.ID != id || a.Seed != binary.LittleEndian.Uint64(hash[:8]) {
		return false
	}
	for _, p := range []domain.Pos{a.Anchor, a.Pos} {
		if p.MapID != d.MapID || math.IsNaN(p.X) || math.IsNaN(p.Y) || math.IsInf(p.X, 0) || math.IsInf(p.Y, 0) || p.X < math.MinInt32 || p.X > math.MaxInt32 || p.Y < math.MinInt32 || p.Y > math.MaxInt32 {
			return false
		}
	}
	return riftDistance2(a.Anchor, a.Pos) <= 225*225
}
