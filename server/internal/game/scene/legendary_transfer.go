package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

func (s *Scene) captureLegendaryRuntime(owner *entity.Entity) domain.LegendaryRuntimeTransfer {
	var out domain.LegendaryRuntimeTransfer
	if owner == nil || s.equipment == nil || s.equipment.legendary == nil {
		return out
	}
	now := s.tick.Millis()
	for k, st := range s.equipment.legendary.states {
		if k.Owner != owner.ID || (k.Target != 0 && k.Target != owner.ID) || st.ExpiresMS > 0 && st.ExpiresMS <= now {
			continue
		}
		out.States = append(out.States, domain.LegendaryTransferredState{ValueIsTimestamp: st.ValueIsTimestamp, ValueAgeMS: now - st.Value, SourceUID: k.UID, PowerID: k.Power, Phase: k.Phase, SelfTarget: k.Target == owner.ID, Stacks: st.Stacks, Value: st.Value, NextInMS: max(int64(0), st.NextMS-now), RemainingMS: max(int64(0), st.ExpiresMS-now), ExpirySentinel: min(int64(0), st.ExpiresMS), Permanent: st.ExpiresMS <= 0, VisualStage: st.VisualStage, Unit: st.Unit, Name: st.Name, Description: st.Description, IconID: st.IconID, Good: st.Good})
	}
	for k, expiry := range s.equipment.legendary.cooldowns {
		if k.Owner == owner.ID && expiry > now {
			out.Cooldowns = append(out.Cooldowns, domain.LegendaryTransferredCooldown{SourceUID: k.UID, PowerID: k.Power, RemainingMS: expiry - now})
		}
	}
	return out
}
func (s *Scene) restoreLegendaryRuntime(owner *entity.Entity, r domain.LegendaryRuntimeTransfer) {
	if owner == nil || owner.Player == nil {
		return
	}
	rt := s.legendaryState()
	active := map[legendaryStateKey]bool{}
	s.forEachLegendary(owner, func(c *legendaryContext) { active[c.key(0, "")] = true })
	for _, v := range r.States {
		key := legendaryStateKey{Owner: owner.ID, UID: v.SourceUID, Power: v.PowerID}
		if !active[key] {
			continue
		}
		key.Phase = v.Phase
		if v.SelfTarget {
			key.Target = owner.ID
		}
		expiry := v.ExpirySentinel
		if !v.Permanent {
			if v.RemainingMS <= 0 {
				continue
			}
			expiry = s.tick.Millis() + v.RemainingMS
		}
		value := v.Value
		if v.ValueIsTimestamp {
			value = s.tick.Millis() - v.ValueAgeMS
		}
		rt.states[key] = &legendaryPowerState{ValueIsTimestamp: v.ValueIsTimestamp, NextMS: s.tick.Millis() + v.NextInMS, Stacks: v.Stacks, Value: value, ExpiresMS: expiry, VisualStage: v.VisualStage, Unit: v.Unit, Name: v.Name, Description: v.Description, IconID: v.IconID, Good: v.Good}
	}
	// Cooldowns follow the item/power even if currently unequipped.
	for _, v := range r.Cooldowns {
		if v.RemainingMS > 0 {
			rt.cooldowns[legendaryStateKey{Owner: owner.ID, UID: v.SourceUID, Power: v.PowerID, Phase: "cooldown"}] = s.tick.Millis() + v.RemainingMS
		}
	}
}
