package domain

// SkillStatusTarget scopes an application to the actual target entity. The
// empty value preserves existing rules; it never changes cast target admission.
type SkillStatusTarget string

const (
	SkillStatusTargetAny       SkillStatusTarget = ""
	SkillStatusTargetPlayer    SkillStatusTarget = "player"
	SkillStatusTargetNonPlayer SkillStatusTarget = "non_player"
)

func (t SkillStatusTarget) Matches(kind EntityKind) bool {
	switch t {
	case SkillStatusTargetAny:
		return true
	case SkillStatusTargetPlayer:
		return kind == KindPlayer
	case SkillStatusTargetNonPlayer:
		return kind != KindPlayer && kind != KindUnknown
	default:
		return false
	}
}

// StatusOverlayStrength selects a single final affix for same-ID replacement.
// ValueOrder must be +1 (higher is stronger) or -1 (lower is stronger).
// The selector is immutable loaded metadata and is not a new gameplay effect.
type StatusOverlayStrength struct {
	Attr       int32
	Mode       int32
	ValueOrder int32
}

func (r StatusOverlayStrength) Valid() bool {
	return r.Attr > 0 && (r.Mode == ModeAbsolute || r.Mode == ModePercent) && (r.ValueOrder == -1 || r.ValueOrder == 1)
}

// OverlayPriority reads final, already-scaled affixes, including percentage caps.
// Legacy states retain level ordering. A malformed selector never falls back to
// levels, which would compare two different strength axes for the same status.
func (d StatusDef) OverlayPriority() (int64, bool) {
	if d.OverlayStrength == nil {
		return int64(d.Level) * 10000, true
	}
	r := *d.OverlayStrength
	if !r.Valid() {
		return 0, false
	}
	count := 0
	var value int64
	for _, a := range d.Affixes {
		if a.Attr != r.Attr {
			continue
		}
		count++
		if a.Mode != r.Mode {
			return 0, false
		}
		value = int64(a.Value) * int64(r.ValueOrder)
	}
	return value, count == 1
}

func (d StatusDef) legacyOverlayLevel() int64 {
	if d.LegacyOverlayLevelBP > 0 {
		return d.LegacyOverlayLevelBP
	}
	return int64(d.Level) * 10000
}

func overlayReplacementBlocked(old, incoming StatusDef) bool {
	// Cross-status matrix rules retain their original level comparison.
	if old.ID != incoming.ID {
		return old.legacyOverlayLevel() > incoming.legacyOverlayLevel()
	}
	if (old.OverlayStrength == nil) != (incoming.OverlayStrength == nil) {
		return true
	}
	if old.OverlayStrength != nil && *old.OverlayStrength != *incoming.OverlayStrength {
		return true
	}
	a, aOK := old.OverlayPriority()
	b, bOK := incoming.OverlayPriority()
	return !aOK || !bOK || a > b
}

// RefreshOverlayStrengths attaches current metadata to old persisted or map-
// transferred snapshots without changing their affixes, timers or DOT remainder.
func (s *StatusSet) RefreshOverlayStrengths(table StatusTable) {
	if s == nil {
		return
	}
	for id, list := range s.act {
		for i := range list {
			if canonical, ok := table.Get(id, list[i].Def.Level); ok {
				list[i].Def.OverlayStrength = canonical.OverlayStrength
				list[i].Def.LegacyOverlayLevelBP = canonical.LegacyOverlayLevelBP
			}
		}
	}
}
