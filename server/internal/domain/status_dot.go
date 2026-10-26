package domain

// DamageOverTime identifies periodic HP loss, not regeneration, MP drain or
// ordinary control. Inspect the same authored inputs as the scene tick handler.
func (d StatusDef) DamageOverTime() bool {
	if !d.Periodic() {
		return false
	}
	if d.PeriodicDamageFraction100 > 0 || d.ClassSetPoisonBonusTotal > 0 {
		return true
	}
	for _, a := range d.Affixes {
		if a.Attr == AttrHPRegen && a.Value < 0 {
			return true
		}
	}
	for _, c := range d.HPChanges {
		if c.Value < 0 {
			return true
		}
	}
	return false
}

// applyDOT retains one instance per (status ID, caster) on every target kind.
// Different casters and different status IDs remain independent. A recast
// refreshes duration but never restarts the pending tick or adds potency.
func (s *StatusSet) applyDOT(def StatusDef, now Tick, src EntityID) {
	incoming := newActive(def, now, src)
	list := s.act[def.ID]
	kept := list[:0]
	var previous Active
	found := false
	for _, a := range list {
		if a.sourceUnknown || a.Source != src || !a.Def.DamageOverTime() {
			kept = append(kept, a)
			continue
		}
		if a.ExpireAt != 0 && a.ExpireAt < now {
			continue
		}
		if !found {
			previous, found = a, true
		} else {
			// Repair old same-source layers without summing their tick damage.
			expiry, next := longerDOTExpiry(previous.ExpireAt, a.ExpireAt), previous.NextTickAt
			if a.NextTickAt > 0 && (next == 0 || a.NextTickAt < next) {
				next = a.NextTickAt
			}
			if !overlayReplacementBlocked(previous.Def, a.Def) {
				previous = a
			}
			previous.ExpireAt, previous.NextTickAt = expiry, next
		}
	}
	if found {
		expiry := longerDOTExpiry(incoming.ExpireAt, previous.ExpireAt)
		if overlayReplacementBlocked(previous.Def, incoming.Def) {
			incoming = previous // Preserve strength and attribution together.
		}
		incoming.dotRefreshed = true
		incoming.dotPreviousExpire = previous.ExpireAt
		incoming.ExpireAt = expiry
		incoming.NextTickAt = previous.NextTickAt
		incoming.PeriodicDamageRemainder100 = previous.PeriodicDamageRemainder100
		// Class-set poison's remainder is measured in authored tick-count units,
		// unlike the fixed hundredths above. Preserve its fractional value when
		// the refreshed duration/interval changes the denominator.
		oldTicks := previous.Def.DurationSec / previous.Def.IntervalSec
		newTicks := incoming.Def.DurationSec / incoming.Def.IntervalSec
		if oldTicks > 0 && newTicks > 0 {
			incoming.ClassSetPoisonRemainder = previous.ClassSetPoisonRemainder * int64(newTicks) / int64(oldTicks)
		}
	}
	s.act[def.ID] = append(kept, incoming)
}

func longerDOTExpiry(a, b Tick) Tick {
	if a == 0 || b == 0 { // permanent
		return 0
	}
	return max(a, b)
}
