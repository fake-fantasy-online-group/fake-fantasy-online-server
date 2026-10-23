package event

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"

// GroundEffect is an optional fixed-map-position presentation, never entity-following.
// Mode: 0 once, 1 sustained (zero duration means until stopped), 2 stop owner/key.
type GroundEffect struct {
	Owner       domain.EntityID
	MapID       int32
	Pos         domain.Pos
	Key, Effect string
	DurationMS  int32
	Mode        uint8
}

func (e GroundEffect) Subject() domain.EntityID { return e.Owner }
