package scene

import (
	"math"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

// Same-map dash powers (U05/U11/U22/U27; U16 is CK-U04 in the utility file).
// Every displacement goes through skillDash: no map reload, 150-250ms
// interpolation, control immunity for the dash window, and the generic
// Teleported hook so TY-U07 counts charges and lunges as displacement.

const (
	bpDashLanceRange   = 500.0
	bpDashLungeRange   = 400.0
	bpDashRescueRange  = 600.0
	bpDashContactSpace = 40.0
)

// bpLanceSkills is every warrior lance skill (枪系) that deals damage.
var bpLanceSkills = map[domain.SkillID]bool{
	10002: true, 10003: true, 10004: true, 10005: true, 10006: true, 10007: true, 10009: true,
	10013: true, 10015: true, 10016: true, 10017: true, 10020: true, 10021: true, 10023: true,
}

func bpLearned(c *legendaryContext, skill domain.SkillID) bool {
	return c != nil && c.Owner != nil && c.Owner.Alive() && c.Owner.Player != nil && c.Owner.Player.Char != nil && c.Owner.Player.Char.Skills[skill] > 0
}

// bpDashPoint is the contact point next to target on the line from owner:
// in front of it (towards owner) or behind it (away from owner).
func bpDashPoint(owner, target *entity.Entity, behind bool) (domain.Pos, bool) {
	dx, dy := target.Pos.X-owner.Pos.X, target.Pos.Y-owner.Pos.Y
	d := math.Hypot(dx, dy)
	if d < 1 {
		return owner.Pos, false
	}
	ux, uy := dx/d, dy/d
	p := target.Pos
	if behind {
		p.X, p.Y = p.X+ux*bpDashContactSpace, p.Y+uy*bpDashContactSpace
	} else {
		if d <= bpDashContactSpace {
			return owner.Pos, false // already in contact
		}
		p.X, p.Y = p.X-ux*bpDashContactSpace, p.Y-uy*bpDashContactSpace
	}
	return p, true
}

// bpDashTo moves the wearer with the shared dash channel and fires the
// displacement hooks. It returns whether the wearer actually moved.
func bpDashTo(c *legendaryContext, to domain.Pos) bool {
	if c == nil || c.Preview || c.Owner == nil || to.MapID != c.Owner.Pos.MapID {
		return false
	}
	from := c.Owner.Pos
	if !c.Scene.skillDash(c.Owner, to, c.Owner.Dir) {
		return false
	}
	c.Scene.legendaryCasterTeleported(c.Owner, from, c.Skill)
	return true
}

func bpWithin(a, b *entity.Entity, r float64) bool {
	return a != nil && b != nil && a.Pos.MapID == b.Pos.MapID && sqDist(a.Pos, b.Pos) <= r*r
}

// ZS-U03 (replacement): Flying Lance becomes a charge to the target's side
// (500), stunning it for 1s; lance skills deal +50% for the next 3s.
func bpLanceChargeHandler() legendaryHandler {
	return legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == 10017 && d.Dist < int32(bpDashLanceRange) {
			d.Dist = int32(bpDashLanceRange)
		}
		if bpLanceSkills[d.ID] && c.State(0, "lance-charge").ExpiresMS > c.NowMS {
			d.LegendaryModifiers.DamageBP += c.RollValue("damage_pct", 50) * 100
		}
	}, CastReleased: func(c *legendaryContext) {
		if c.Skill != 10017 || !bpLearned(c, 10017) || !bpHostile(c, c.Target) || !bpWithin(c.Owner, c.Target, bpDashLanceRange+bpDashContactSpace) {
			return
		}
		if to, ok := bpDashPoint(c.Owner, c.Target, false); ok {
			bpDashTo(c, to)
		}
		bpTimed(c, c.Target, "lance-stun", 1, true, func(d *domain.StatusDef) { d.Control = domain.ControlStun })
		c.State(0, "lance-charge").ExpiresMS = c.NowMS + 3000
	}}
}

// JK-U01 (replacement): Vacuum Sword becomes a lunge behind the target (400).
func bpVacuumLungeHandler() legendaryHandler {
	return legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == 10117 && d.Dist < int32(bpDashLungeRange) {
			d.Dist = int32(bpDashLungeRange)
		}
	}, CastReleased: func(c *legendaryContext) {
		if c.Skill != 10117 || !bpLearned(c, 10117) || !bpHostile(c, c.Target) || !bpWithin(c.Owner, c.Target, bpDashLungeRange+bpDashContactSpace) {
			return
		}
		if to, ok := bpDashPoint(c.Owner, c.Target, true); ok {
			bpDashTo(c, to)
		}
	}}
}

func init() {
	// SS-U01 (kept + extended): phantom distance +50%; within 2s after a
	// phantom, the next spell deals +30% damage.
	legendaryAugment("SS-U01", legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == 10432 {
			d.Dist = specialScale(d.Dist, 150)
			return
		}
		if c.State(0, "phantom-next").ExpiresMS > c.NowMS {
			d.LegendaryModifiers.DamageBP += 3000
		}
	}, CastReleased: func(c *legendaryContext) {
		st := c.State(0, "phantom-next")
		if c.Skill == 10432 {
			st.ExpiresMS = c.NowMS + 2000
			return
		}
		st.ExpiresMS = 0
	}})
	// YS-U07 (kept + extended): Purify may target a party member within 600;
	// the healer dashes beside them and the cleansed ally also takes 25% less
	// physical and magic damage for 8s.
	legendaryAugment("YS-U07", legendaryHandler{PrepareSkill: func(c *legendaryContext, d *domain.SkillDef) {
		if d.ID == 10306 && d.Dist < int32(bpDashRescueRange) {
			d.Dist = int32(bpDashRescueRange)
		}
	}, CastReleased: func(c *legendaryContext) {
		ally := c.Target
		if c.Skill != 10306 || !bpLearned(c, 10306) || ally == nil || ally.ID == c.Owner.ID || ally.Player == nil || !ally.Alive() || !sameParty(c.Owner, ally) || !bpWithin(c.Owner, ally, bpDashRescueRange+bpDashContactSpace) {
			return
		}
		if to, ok := bpDashPoint(c.Owner, ally, false); ok {
			bpDashTo(c, to)
		}
		bpTimed(c, ally, "escort-ally", 8, false, func(d *domain.StatusDef) {
			d.PhysicalDamageReductionPct = 25
			d.MagicDamageReductionPct = 25
		})
	}})
}
