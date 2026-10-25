package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"math"
)

// replicationMovementBuffer extends the nominal 600-pixel buffer using the
// existing authoritative movement admission budget, not a guessed speed limit.
// Two tick periods additionally cover ordinary staged-delta delivery. A dense
// initial snapshot can require longer: this is not a promise that an arbitrarily
// large initial snapshot is fully visible before the client starts moving.
func (s *Scene) replicationMovementBuffer(p *entity.Entity) float64 {
	speed := float64(s.playerMoveSpeedPX(p))
	reportWindow := (clientMoveReportInterval * clientMoveWindowPeriods).Seconds()
	reach := speed*(reportWindow*clientMoveCatchUpMul+2*float64(domain.TickMS)/1000) + clientMoveLagTolerancePX
	return math.Max(0, reach-600)
}
