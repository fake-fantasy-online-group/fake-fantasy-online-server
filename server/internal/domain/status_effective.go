package domain

// SuppressesHarmful reports an explicit, stored suppression source. Callers
// advance expiry at their simulation boundary; reads never mutate deadlines.
// BlockHarmful by itself remains admission-only and never suppresses old effects.
func (s *StatusSet) SuppressesHarmful() bool {
	active := false
	s.Each(func(a Active) { active = active || a.Def.SuppressHarmful })
	return active
}

// SuppressesHarmfulAt is a side-effect-free query for queued effects at a known
// simulation time. The end is exclusive: an immunity expiring at t does not
// suppress an effect first due at t. Past-due receipts are discarded separately.
func (s *StatusSet) SuppressesHarmfulAt(now Tick) bool {
	until, permanent := s.harmfulSuppressionEnd()
	return permanent || now < until
}

func (s *StatusSet) harmfulSuppressionEnd() (until Tick, permanent bool) {
	s.Each(func(a Active) {
		if a.Def.SuppressHarmful {
			permanent = permanent || a.ExpireAt == 0
			until = max(until, a.ExpireAt)
		}
	})
	return
}

// EachEffective leaves storage and expiration intact while hiding harmful
// effects from combat and stat adapters. Positive statuses always remain active.
func (s *StatusSet) EachEffective(fn func(Active)) {
	if fn == nil {
		return
	}
	suppress := s.SuppressesHarmful()
	s.Each(func(a Active) {
		if !suppress || !a.Def.Harmful() {
			fn(a)
		}
	})
}

// HasEffective is for behavior queries. Has retains raw storage semantics for
// persistence, deduplication, explicit removal and presentation.
func (s *StatusSet) HasEffective(id StatusID) bool {
	found := false
	s.EachEffective(func(a Active) { found = found || a.Def.ID == id })
	return found
}

// RemoveEffectiveWhere consumes reactive effects without accidentally deleting
// a suspended harmful instance. Explicit dispels should continue to use RemoveWhere.
func (s *StatusSet) RemoveEffectiveWhere(match func(StatusDef) bool) []StatusID {
	if match == nil {
		return nil
	}
	suppress := s.SuppressesHarmful()
	return s.RemoveWhere(func(d StatusDef) bool { return (!suppress || !d.Harmful()) && match(d) })
}

// DamageReductionCapPct combines explicit source ceilings by maximum. Ordinary
// states, equipment and other layers still share the default 75% final budget.
func (s *StatusSet) DamageReductionCapPct() int32 {
	capPct := DefaultDamageReductionCapPct
	s.EachEffective(func(a Active) { capPct = max(capPct, a.Def.DamageReductionCapPct) })
	return min(int32(100), capPct)
}

// discardSuppressedTicks advances pending harmful ticks instead of replaying
// them after immunity. Keep fractional carry unchanged: suppressed periods earn
// no damage credit. Existing no-catch-up cadence is retained (next = now+interval).
func (s *StatusSet) discardSuppressedTicks(now Tick) {
	if s == nil {
		return
	}
	until, permanent := s.harmfulSuppressionEnd()
	if !permanent && until == 0 {
		return
	}
	for id, list := range s.act {
		for i, a := range list {
			if a.NextTickAt == 0 || now < a.NextTickAt || !a.Def.Harmful() || !(permanent || a.NextTickAt < until) {
				continue
			}
			a.NextTickAt = now + Ticks(int(a.Def.IntervalSec)*1000)
			a.PeriodicDamageTickExtra = 0
			list[i] = a
		}
		s.act[id] = list
	}
}

// ExpireHarmfulSuppression releases immunity before this frame's periodic
// settlement and stat reads. Any expired harmful states are removed together
// with a stored suppression source, so even a late frame cannot restore an
// already-ended debuff or replay its final periodic tick.
// Other states retain their final-tick order. Refresh stats for returned IDs.
func (s *StatusSet) ExpireHarmfulSuppression(now Tick) []StatusID {
	if s == nil {
		return nil
	}
	until, permanent := s.harmfulSuppressionEnd()
	if !permanent && until == 0 {
		return nil
	}
	s.discardSuppressedTicks(now)
	// Snapshot before removal: map iteration must not make simultaneous expiry
	// depend on whether the immunity provider or its suspended state is visited first.
	hadSuppression := permanent || until > 0
	var gone []StatusID
	for id, list := range s.act {
		kept := list[:0]
		removed := false
		for _, a := range list {
			expiredHarmful := hadSuppression && a.Def.Harmful()
			if a.ExpireAt != 0 && now >= a.ExpireAt && (a.Def.SuppressHarmful || expiredHarmful) {
				removed = true
				continue
			}
			kept = append(kept, a)
		}
		if len(kept) == 0 {
			delete(s.act, id)
		} else {
			s.act[id] = kept
		}
		if removed {
			gone = append(gone, id)
		}
	}
	return gone
}
