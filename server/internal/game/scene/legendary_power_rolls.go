package scene

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"

// RollValue is called only for explicitly authored beneficial coefficients.
// Each key has corresponding player-facing range metadata. Never use this for
// IDs, counts, trigger thresholds, free casts or a complete incoming damage.
func (c *legendaryContext) RollValue(key string, base int32) int32 {
	return domain.LegendaryPowerRollValue(c.Power.ID, key, base, c.PowerRollBP)
}
