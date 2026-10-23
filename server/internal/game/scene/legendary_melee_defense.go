package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"hash/fnv"
)

func legendaryMeleeStatusID(c *legendaryContext, phase string) domain.StatusID {
	h := fnv.New32a()
	_, _ = h.Write([]byte(c.Power.ID + ":" + phase))
	return domain.StatusID(1900000000 + h.Sum32()%10000000)
}
func legendaryMeleeIcon(c *legendaryContext) int32 {
	_, _, icon, _ := legendaryStatePresentation(c.Power.ID, "")
	return icon
}

func legendaryMeleeRemoveSlow(c *legendaryContext) {
	if c.Owner.Status == nil {
		return
	}
	c.Scene.statusesRemoved(c.Owner, c.Owner.Status.RemoveWhere(func(d domain.StatusDef) bool {
		if d.ID == domain.StatusSlow {
			return true
		}
		for _, a := range d.Affixes {
			if a.Attr == domain.AttrMoveSpeed && a.Value < 0 {
				return true
			}
		}
		return false
	}))
}
func legendaryMeleeIncoming(c *legendaryContext) bool { return legendaryMeleeDirect(c) }
