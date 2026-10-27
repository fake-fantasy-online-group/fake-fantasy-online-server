package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

func stampSkillStatusSnapshot(status *domain.StatusDef, def domain.SkillDef) {
	if status == nil {
		return
	}
	status.SkillApplied = true
	status.DamageSourceRange = def.Dist
	status.LegendarySources = append([]domain.LegendarySourceRef(nil), def.LegendarySources...)
	if def.FormulaVersion != domain.SkillFormulaVersion {
		return
	}
	status.SkillSnapshotID = def.ID
	status.SkillSnapshotRank = def.Level
	status.SkillSnapshotRaw = def.SnapshotRawRanks.Clone()
	status.SkillSnapshotHard = def.SnapshotHardRanks.Clone()
}
func lesserSkillLevels(cast, current domain.Learned) domain.Learned {
	out := cast.Clone()
	for id, rank := range out {
		if current[id] < rank {
			out[id] = current[id]
		}
	}
	return out
}
func (s *Scene) lesserPersistentSkill(source *entity.Entity, def domain.SkillDef) (domain.SkillDef, bool) {
	if def.FormulaVersion != domain.SkillFormulaVersion || source == nil || source.Player == nil {
		return def, true
	}
	current := s.rawSkillLevels(source)
	rank := min(def.Level, current[def.ID])
	if rank <= 0 {
		return domain.SkillDef{}, false
	}
	raw := lesserSkillLevels(def.SnapshotRawRanks, current)
	hard := lesserSkillLevels(def.SnapshotHardRanks, source.Player.Char.Skills)
	next, ok := s.skills.Get(def.ID, rank)
	if !ok {
		return domain.SkillDef{}, false
	}
	s.applyTrainingSynergiesWithLevels(source, &next, hard, raw)
	s.applyPassiveSkillModifiersWithLevels(source, &next, raw)
	s.applyUniqueSkillModifiers(source, &next)
	if def.SpecialWeaponBaseID != 0 && s.specialWeapon(source) == def.SpecialWeaponBaseID {
		applySpecialWeaponDefinition(def.SpecialWeaponBaseID, &next)
	}
	next.SpecialWeaponStatusPowerPct = def.SpecialWeaponStatusPowerPct
	s.legendaryPrepareSkillSources(source, &next, def.LegendarySources, true)
	// Equipment-contributed attack bases may shrink after source loss, but a
	// later gear upgrade must not strengthen an already placed native trap.
	next.Effects = append([]domain.SkillEffect(nil), next.DirectEffects()...)
	acceptedEffects := def.DirectEffects()
	for i := range next.Effects {
		if i < len(acceptedEffects) && next.Effects[i].Kind == acceptedEffects[i].Kind {
			next.Effects[i].AttackBaseFlat = min(next.Effects[i].AttackBaseFlat, acceptedEffects[i].AttackBaseFlat)
		}
	}
	// Geometric bonuses survive while equipped but cannot exceed the cast snapshot.
	if def.Radius > 0 {
		next.Radius = min(next.Radius, def.Radius)
	}
	if def.Area.Radius > 0 {
		next.Area.Radius = min(next.Area.Radius, def.Area.Radius)
	}
	if def.Area.Width > 0 {
		next.Area.Width = min(next.Area.Width, def.Area.Width)
	}
	if def.Area.Length > 0 {
		next.Area.Length = min(next.Area.Length, def.Area.Length)
	}
	next.SnapshotRawRanks = raw
	next.SnapshotHardRanks = hard
	return next, true
}
func (s *Scene) refreshSkillPersistentEffects(source *entity.Entity) {
	if s == nil || source == nil || source.Player == nil || s.skillProgressionRefreshing {
		return
	}
	// refreshStats defers this until its HP/MP growth accounting is committed.
	// Applying a native status earlier would reenter refreshStats and charge
	// the same maximum-resource delta twice.
	s.syncLegendaryPermanentBuffs(source)
	s.skillProgressionRefreshing = true
	defer func() { s.skillProgressionRefreshing = false }()
	for _, target := range s.entities {
		if target.Status == nil {
			continue
		}
		removed, changed := target.Status.ReconcileSkillSource(source.ID, func(old domain.StatusDef) (domain.StatusDef, bool, bool) {
			return s.resolvePersistentStatus(source, target, old)
		})
		if len(removed) > 0 {
			s.statusesRemoved(target, removed)
		}
		if changed {
			s.refreshEntityStats(target)
			s.emitAttributesIfPlayer(target)
		}
	}
}

func (s *Scene) revokeDepartingSkillSource(source *entity.Entity) {
	if source == nil {
		return
	}
	for _, target := range s.entities {
		if target.ID == source.ID || target.Status == nil {
			continue
		}
		removed, changed := target.Status.ReconcileSkillSource(source.ID, func(domain.StatusDef) (domain.StatusDef, bool, bool) { return domain.StatusDef{}, false, true })
		if len(removed) > 0 {
			s.statusesRemoved(target, removed)
		} else if changed {
			s.refreshEntityStats(target)
			s.emitAttributesIfPlayer(target)
		}
	}
}

func (s *Scene) resolvePersistentStatus(source, holder *entity.Entity, old domain.StatusDef) (domain.StatusDef, bool, bool) {
	if old.LegendaryPermanentPower != "" {
		return old, true, false // Rebuilt by the equipment/learned-skill reconciler.
	}
	current := s.rawSkillLevels(source)
	decreased := current[old.SkillSnapshotID] < old.SkillSnapshotRank
	for id, rank := range old.SkillSnapshotRaw {
		decreased = decreased || current[id] < rank
	}
	for id, rank := range old.SkillSnapshotHard {
		decreased = decreased || source.Player.Char.Skills[id] < rank
	}
	if !decreased {
		return old, true, false
	}
	template := domain.SkillDef{LegendarySources: old.LegendarySources, SpecialWeaponBaseID: old.SpecialWeaponSourceBase, SpecialWeaponStatusPowerPct: old.SpecialWeaponStatusPowerPct, ID: old.SkillSnapshotID, Level: old.SkillSnapshotRank, FormulaVersion: domain.SkillFormulaVersion, SnapshotRawRanks: old.SkillSnapshotRaw, SnapshotHardRanks: old.SkillSnapshotHard}
	def, ok := s.lesserPersistentSkill(source, template)
	if !ok {
		return domain.StatusDef{}, false, true
	}
	if old.ID == v5UtilityShield {
		next := old
		next.DamageShield = min(old.DamageShield, def.ProgressionShieldFlat)
		stampSkillStatusSnapshot(&next, def)
		next.DamageSourceRange = old.DamageSourceRange
		next.DamageSourceTrap = old.DamageSourceTrap
		return next, next.DamageShield > 0, true
	}
	for _, app := range def.Statuses {
		if app.ID != old.ID || holder == nil || !app.TargetType.Matches(holder.Kind) {
			continue
		}
		base, ok := s.resolveSkillStatusDef(def, app, 0)
		if !ok {
			return domain.StatusDef{}, false, true
		}
		// Reconciliation may lower a carrier's skill payload, never upgrade the
		// holder's effect-hit snapshot merely because equipment improved later.
		scaleClassSetTrapStatus(&base, old.SuitTrapDamageBP)
		// Preserve accepted instance identity and consumed charges during rank
		// downgrade. Recompute the smaller native base, not a fresh grant.
		base.LegendaryModifiers = old.LegendaryModifiers
		base.LegendaryModifiers.Applied = false
		base.LegendaryPoisonAmplified = old.LegendaryPoisonAmplified
		base.LegendaryPoisonCopy = old.LegendaryPoisonCopy
		base.LegendaryPoisonSpread = old.LegendaryPoisonSpread
		base.LegendaryFreezePoisonOwner = old.LegendaryFreezePoisonOwner
		base.LegendaryStealthTrapCharges = old.LegendaryStealthTrapCharges
		s.legendaryFinalizeStatus(&base)
		base.SourceEffectHitBP = old.SourceEffectHitBP
		base.SourceEffectHitKnown = old.SourceEffectHitKnown
		base.Icon = old.Icon
		base.SourceSkillID = old.SourceSkillID
		base.SpecialWeaponNextBasicPct = min(base.SpecialWeaponNextBasicPct, old.SpecialWeaponNextBasicPct)
		stampSkillStatusSnapshot(&base, def)
		// A refresh cannot change the accepted attack category, including legacy
		// unknown provenance, merely because its skill is rebuilt today.
		base.DamageSourceRange = old.DamageSourceRange
		base.DamageSourceTrap = old.DamageSourceTrap
		return base, true, true
	}
	return domain.StatusDef{}, false, true
}

func (s *Scene) reconcileEnteringSkillStatuses(p *entity.Entity) {
	if p == nil || p.Status == nil {
		return
	}
	p.Status.RefreshOverlayStrengths(s.statuses)
	p.Status.ReconcileSkillSource(p.ID, func(old domain.StatusDef) (domain.StatusDef, bool, bool) { return s.resolvePersistentStatus(p, p, old) })
}
