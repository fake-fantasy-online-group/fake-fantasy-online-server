package scene

import (
	"math"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

// Buff pass (2026-11-16) additions to caster/healer attack legendaries that
// are layered on the existing handlers instead of rewriting them.
func init() {
	// SS-A20: blizzard drags movable non-boss enemies toward its centre.
	legendaryAugment("SS-A20", legendaryHandler{SkillResolved: func(c *legendaryContext) {
		if c.Skill != legendaryBlizzard || c.Target == nil || c.Target.ID == c.Owner.ID || sameParty(c.Owner, c.Target) {
			return
		}
		if c.Target.Monster != nil && c.Target.Monster.Kind == domain.MonsterBoss {
			return
		}
		if distance := math.Sqrt(sqDist(c.Center, c.Target.Pos)); distance > bpBlizzardPullStop {
			legendaryCasterPull(c, c.Target, c.Center, distance-bpBlizzardPullStop)
		}
	}})
	// YS-A19: shackled enemies take +20% physical and magic damage from
	// everyone for as long as the shackle status lasts.
	legendaryAugment("YS-A19", legendaryHandler{StatusApplied: func(c *legendaryContext, d *domain.StatusDef) {
		if c.Skill != 10325 || c.Target == nil || c.Target.ID == c.Owner.ID || sameParty(c.Owner, c.Target) {
			return
		}
		d.PhysicalDamageTakenPct += bpShackleVulnerablePct
		d.MagicDamageTakenPct += bpShackleVulnerablePct
	}})
}

const (
	bpBlizzardPullStop     = 80
	bpShackleVulnerablePct = 20
)
