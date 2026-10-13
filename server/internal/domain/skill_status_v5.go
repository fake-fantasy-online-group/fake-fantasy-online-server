package domain

// ReconcileSkillSource downgrades persistent payloads without refreshing time,
// refunding a consumed shield, or increasing a previously cast instance.
func (s *StatusSet) ReconcileSkillSource(source EntityID, resolve func(StatusDef) (StatusDef, bool, bool)) (removed []StatusID, changed bool) {
	if s == nil {
		return nil, false
	}
	for id, list := range s.act {
		kept := list[:0]
		for _, active := range list {
			if active.Source != source || active.Def.SkillSnapshotID == 0 {
				kept = append(kept, active)
				continue
			}
			next, ok, update := resolve(active.Def)
			if !ok {
				changed = true
				continue
			}
			if !update {
				kept = append(kept, active)
				continue
			}
			spent := int64(active.Def.DamageShield) - int64(active.ShieldLeft)
			active.ShieldLeft = min(active.ShieldLeft, max(int32(0), SaturateInt32(int64(next.DamageShield)-spent)))
			started := active.ExpireAt - Ticks(int(active.Def.DurationSec)*1000)
			if next.DurationSec > 0 {
				active.ExpireAt = min(active.ExpireAt, started+Ticks(int(next.DurationSec)*1000))
			}
			active.Def = next
			kept = append(kept, active)
			changed = true
		}
		if len(kept) == 0 {
			delete(s.act, id)
			removed = append(removed, id)
		} else {
			s.act[id] = kept
		}
	}
	return removed, changed
}

// ProgressionAttackBonus snapshots authored first-hit/next-spell flat damage
// before its status is consumed and actor stats are refreshed.
func (s *StatusSet) ProgressionAttackBonus(skill, hasDamage, hasCast bool) int32 {
	if s == nil || !hasDamage {
		return 0
	}
	var total int64
	s.Each(func(a Active) {
		d := a.Def
		if d.ProgressionAttackFlat <= 0 {
			return
		}
		eligible := d.Invisible && d.BreakOnAttack || (!skill && d.NextBasicAttackPct > 0) || (skill && (d.NextSkillDamagePct > 0 || (hasCast && d.NextCastTimeReductionPct > 0)))
		if eligible {
			total += int64(d.ProgressionAttackFlat)
		}
	})
	return SaturateInt32(total)
}

// RebindSkillOwner preserves only self-cast V5 statuses at a scene boundary.
// External source handles cannot prove current ranks after separation, so their
// V5 effects are explicitly revoked. Legacy effects retain their prior behavior.
func (s *StatusSet) RebindSkillOwner(from, to EntityID) []StatusID {
	if s == nil {
		return nil
	}
	var removed []StatusID
	for _, id := range s.sortedIDs() {
		list := s.act[id]
		kept := list[:0]
		for _, a := range list {
			if a.Def.SkillSnapshotID != 0 {
				if from == 0 || a.Source != from {
					continue
				}
				a.Source = to
			}
			kept = append(kept, a)
		}
		if len(kept) == 0 {
			delete(s.act, id)
			removed = append(removed, id)
		} else {
			s.act[id] = kept
		}
	}
	return removed
}
