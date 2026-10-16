package scene

import (
	"encoding/binary"
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"sort"
)

// Each authored budget entry has an instance-stable source identity. One item can
// intentionally carry all+single, while accidental repeated collection is deduped.
func (s *Scene) skillRankBonuses(p *entity.Entity) []domain.SkillRankBonus {
	return s.skillRankBonusesResolved(p, newEquipmentRequirementResolution())
}

func (s *Scene) skillRankBonusesResolved(p *entity.Entity, resolution *equipmentRequirementResolution) []domain.SkillRankBonus {
	if s == nil || p == nil || p.Player == nil || p.Player.Char == nil {
		return nil
	}
	worn := s.functionalWornResolved(p, resolution)
	if worn == nil {
		return nil
	}
	var out []domain.SkillRankBonus
	worn.Each(func(slot domain.EquipSlot, st domain.Stack) {
		if d, ok := domain.UniqueOf(st, s.endgameCatalog); ok {
			for i, b := range d.SkillBonuses {
				b.SourceID = fmt.Sprintf("item:%d:unique:%d", st.UID, i)
				out = append(out, b)
			}
		}
	})
	for _, a := range domain.ActiveAstralRecipes(worn, s.itemDef, p.Player.Char) {
		for i, b := range a.Recipe.SkillBonuses {
			b.SourceID = fmt.Sprintf("item:%d:formation:%d", a.UID, i)
			out = append(out, b)
		}
	}
	return out
}
func (s *Scene) skillRanks(p *entity.Entity) (domain.SkillResolutionSnapshot, error) {
	return s.skillRanksResolved(p, newEquipmentRequirementResolution())
}

func (s *Scene) skillRanksResolved(p *entity.Entity, resolution *equipmentRequirementResolution) (domain.SkillResolutionSnapshot, error) {
	if s == nil || p == nil || p.Player == nil || p.Player.Char == nil {
		return nil, fmt.Errorf("missing skill actor")
	}
	ch := p.Player.Char
	var grants []domain.SkillGrant
	if treasure := s.equippedTreasureSkill(p); treasure != nil {
		grants = append(grants, domain.SkillGrant{SourceID: "treasure", SkillID: treasure.ID, Rank: 1, Origin: "treasure_grant"})
	}
	bonuses := s.skillRankBonusesResolved(p, resolution)
	// With no equipment rank sources, every subset has the exact same permanent
	// loadout. Eligibility still runs above; only the pure catalogue resolution
	// is shared inside this one immutable actor evaluation.
	plain := resolution != nil && len(bonuses) == 0 && len(grants) == 0
	if plain && resolution.plainRanksReady {
		return resolution.plainRanks, nil
	}
	ranks, err := domain.ResolveSkillLoadout(int32(ch.Race)+1, ch.Skills, bonuses, grants, s.skills.ProgressionCatalog())
	if plain && err == nil {
		resolution.plainRanks, resolution.plainRanksReady = ranks, true
	}
	return ranks, err
}

// rawSkillLevels is the single raw, nonrecursive snapshot used by every passive consumer.
// Legacy tables keep exact legacy levels; the returned map never aliases permanent hard points.
func (s *Scene) rawSkillLevels(p *entity.Entity) domain.Learned {
	state := s.combatAttributeState(p)
	if state == nil {
		return s.rawSkillLevelsResolved(p, newEquipmentRequirementResolution())
	}
	s.prepareCombatAttributes(p, state)
	return state.values.RawSkills.Clone()
}

func (s *Scene) rawSkillLevelsResolved(p *entity.Entity, resolution *equipmentRequirementResolution) (result domain.Learned) {
	if p == nil || p.Player == nil || p.Player.Char == nil {
		return nil
	}
	// Every recursive preview in this context has the same actor inputs and
	// only removes fixed slots. Memoize each subset before the legacy full-stack
	// fingerprint; the latter otherwise serializes the same gear over and over.
	if resolution != nil {
		mask := equipmentRequirementMask(p.Player.Worn)
		if levels, ok := resolution.raw[mask]; ok {
			resolution.rawHits++
			return levels.Clone()
		}
		defer func() { resolution.raw[mask] = result.Clone() }()
	}
	out := p.Player.Char.Skills.Clone()
	hasV5 := false
	for id := range out {
		if s.skills.V5Enabled(id) {
			hasV5 = true
			break
		}
	}
	// Preserve the cheap legacy/unlearned path, but do not drop an explicit
	// equipment grant merely because this actor has no allocated class skill.
	if !hasV5 {
		if grant := s.equippedTreasureSkill(p); grant != nil {
			hasV5 = s.skills.V5Enabled(grant.ID)
		}
	}
	if !hasV5 {
		return out
	}
	key := skillRawFingerprint(p)
	if cached, ok := s.skillRawCache[p.ID]; ok && cached.key == key {
		return cached.levels.Clone()
	}
	ranks, err := s.skillRanksResolved(p, resolution)
	if err != nil { // Invalid persisted inputs fail closed, rather than granting partial soft ranks.
		for id := range out {
			if s.skills.V5Enabled(id) {
				delete(out, id)
			}
		}
		return out
	}
	// An equipment grant can be the first skill of an otherwise unlearned
	// character; preserve the nil legacy fast path but allocate before grants.
	if out == nil && len(ranks) > 0 {
		out = make(domain.Learned)
	}
	for id, r := range ranks {
		if !s.skills.V5Enabled(id) {
			continue
		}
		if !r.CanCast {
			delete(out, id)
			continue
		}
		rank, err := r.EngineRank()
		if err != nil {
			delete(out, id)
			continue
		}
		out[id] = rank
	}
	if s.skillRawCache == nil {
		s.skillRawCache = map[domain.EntityID]skillRawCacheEntry{}
	}
	s.skillRawCache[p.ID] = skillRawCacheEntry{key: key, levels: out.Clone()}
	return out
}
func (s *Scene) applyTrainingSynergies(p *entity.Entity, def *domain.SkillDef) {
	if p == nil || p.Player == nil {
		return
	}
	s.applyTrainingSynergiesWithLevels(p, def, p.Player.Char.Skills, s.rawSkillLevels(p))
}
func (s *Scene) applyTrainingSynergiesWithHard(p *entity.Entity, def *domain.SkillDef, hard domain.Learned) {
	s.applyTrainingSynergiesWithLevels(p, def, hard, s.rawSkillLevels(p))
}

// Consume only the supplied raw rank snapshot. In particular, persistent effects
// pass the lesser cast/current snapshot so changing equipment cannot improve an old cast.
func (s *Scene) applyTrainingSynergiesWithLevels(p *entity.Entity, def *domain.SkillDef, hard, raw domain.Learned) {
	if def == nil || def.FormulaVersion != domain.SkillFormulaVersion || p == nil || p.Player == nil {
		return
	}
	var damage, elite, heal, flat, mana, radius, width, delay, duration int64
	for _, e := range s.skills.ProgressionCatalog().Synergies() {
		if e.Target != def.ID || !e.IsTraining() {
			continue
		}
		h := e.SourceRank(hard, raw)
		if h <= 0 {
			continue
		}
		v := int64(e.PerPointBP) * int64(h)
		switch e.Effect {
		case "damage":
			damage += v
		case "elite_damage":
			elite += v
		case "heal_power":
			heal += v
		case "heal_flat":
			flat += v / 100
		case "mana_economy":
			mana += v
		case "radius":
			radius += v
		case "width":
			width += v
		case "arm_delay_economy":
			delay += v
		case "freeze_duration":
			duration += v
		}
	}
	// Rank/coefficient multiplication is exact in int64; narrow only at the
	// existing engine payload boundary, never while selecting source ranks.
	healBP, healFlat := domain.SaturateInt32(heal), domain.SaturateInt32(flat)
	manaBP, delayBP := domain.SaturateInt32(mana), domain.SaturateInt32(delay)
	radiusBP, widthBP, durationBP := domain.SaturateInt32(radius), domain.SaturateInt32(width), domain.SaturateInt32(duration)
	def.TrainingDamageBP = domain.SaturateInt32(damage)
	def.TrainingEliteDamageBP = domain.SaturateInt32(elite)
	def.MPCost = domain.ApplyUniqueBP(def.MPCost, -min(manaBP, 8000))
	def.PrepareMS = domain.ApplyUniqueBP(def.PrepareMS, -min(delayBP, 8000))
	def.Radius = domain.ApplyUniqueBP(def.Radius, radiusBP)
	def.Area.Radius = domain.ApplyUniqueBP(def.Area.Radius, radiusBP)
	def.Area.Width = domain.ApplyUniqueBP(def.Area.Width, widthBP)
	def.Effects = append([]domain.SkillEffect(nil), def.DirectEffects()...)
	for i := range def.Effects {
		e := &def.Effects[i]
		if e.Kind == domain.EffectHeal {
			// Exact original rows use integer fields. Seed their rational form
			// before adding/scaling training bonuses, or numerator zero erases
			// the original healing when a denominator is introduced.
			if e.HealFlatDenominator <= 0 {
				e.HealFlatNumerator, e.HealFlatDenominator = int64(e.HealFlat), 1
			}
			if e.HealPctDenominator <= 0 {
				e.HealPctNumerator, e.HealPctDenominator = int64(e.HealPctOfMax), 1
			}
			e.HealFlat += healFlat
			e.HealFlatNumerator += int64(healFlat) * max(int64(1), e.HealFlatDenominator)
			if healBP != 0 {
				domain.ApplySkillHealingBP(e, healBP)
			}
		}
		e.TrapRadius = domain.ApplyUniqueBP(e.TrapRadius, radiusBP)
	}
	if len(def.Effects) > 0 {
		def.Effect = def.Effects[0]
	}
	if durationBP > 0 {
		def.Statuses = append([]domain.StatusApplication(nil), def.Statuses...)
		for i := range def.Statuses {
			app := &def.Statuses[i]
			if app.ProgressionRanks > 0 {
				resolved, ok := s.statuses.ResolveSkillApplication(*app)
				if !ok {
					continue
				}
				if app.DurationSec <= 0 {
					original, ok := s.statuses.Get(app.ID, app.Level)
					if !ok {
						continue
					}
					app.DurationSec = original.DurationSec
					if app.ProgressionBaseDurationSec > 0 {
						app.DurationSec = app.ProgressionBaseDurationSec
					}
				}
				boosted := domain.ApplyUniqueBP(resolved.DurationSec, durationBP)
				if cap := s.statuses.SkillApplicationDurationCap(*app); cap > 0 {
					boosted = min(boosted, cap)
				}
				app.DurationSec = domain.SaturateInt32(int64(app.DurationSec) + int64(boosted) - int64(resolved.DurationSec))
				continue
			}
			duration := app.DurationSec
			if duration <= 0 {
				if base, ok := s.statuses.Get(app.ID, app.Level); ok {
					duration = base.DurationSec
				}
			}
			app.DurationSec = domain.ApplyUniqueBP(duration, durationBP)
		}
	}
}

const v5UtilityShield domain.StatusID = 91005

func (s *Scene) applySkillProgressionShield(src, dst *entity.Entity, def domain.SkillDef) {
	if def.ProgressionShieldFlat <= 0 || src == nil {
		return
	}
	target := dst
	if def.TargetEnemy || target == nil {
		target = src
	}
	if target == nil || !target.Alive() {
		return
	}
	shield := domain.StatusDef{ID: v5UtilityShield, Level: def.ProgressionShieldFlat, Name: "护身余势", Type: domain.StatusGood, TypeKnown: true, DurationSec: 5, DamageShield: def.ProgressionShieldFlat, Icon: def.Icon, SourceSkillID: int32(def.ID)}
	stampSkillStatusSnapshot(&shield, def)
	s.applyStatusDef(target, shield, src.ID)
}
func applyProgressionStatus(app domain.StatusApplication, status *domain.StatusDef) {
	status.ProgressionAttackFlat = app.ProgressionAttackFlat
	status.Affixes = append(append([]domain.Affix(nil), status.Affixes...), app.ProgressionAffixes...)
	if app.ProgressionHPPerTick != 0 {
		status.HPChanges = append(append([]domain.ResourceChange(nil), status.HPChanges...), domain.ResourceChange{Value: app.ProgressionHPPerTick, Mode: domain.ModeAbsolute})
	}
	status.DamageShield = domain.SaturateInt32(int64(status.DamageShield) + int64(app.ProgressionDamageShield))
	if app.ProgressionReflectFlat > 0 {
		if status.PhysicalReflectPct > 0 || status.PhysicalReflectFlat > 0 || status.ReflectFlatAsMagic {
			status.PhysicalReflectFlat = domain.SaturateInt32(int64(status.PhysicalReflectFlat) + int64(app.ProgressionReflectFlat))
		}
		if status.MagicReflectPct > 0 || status.MagicReflectFlat > 0 || status.MagicFullReflectChancePct > 0 || status.ReflectFlatAsMagic {
			status.MagicReflectFlat = domain.SaturateInt32(int64(status.MagicReflectFlat) + int64(app.ProgressionReflectFlat))
		}
	}
}

// Equipment eligibility and source composition are part of the key. This caches
// only immutable raw ranks, never cast-local or synergy-enhanced definitions.
type skillRawCacheEntry struct {
	key    skillRawCacheKey
	levels domain.Learned
}

// Gear snapshots already own a revision and value-only stacks. Use their
// identity/revision instead of serializing every socket/card/affix for every
// recursive trial. A cloned preview may miss this scene memo; the evaluation's
// subset memo handles those repeats. This key never escapes runtime memory.
type skillRawCacheKey struct {
	character                 *domain.Character
	worn                      *domain.EquipSet
	revision                  uint64
	charID                    int64
	profession                domain.Race
	level                     int32
	base                      domain.Base
	hard                      string
	employed, employmentKnown bool
	gender                    uint8
}

func skillRawFingerprint(p *entity.Entity) skillRawCacheKey {
	ch := p.Player.Char
	ids := make([]domain.SkillID, 0, len(ch.Skills))
	for id := range ch.Skills {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	// Exact canonical encoding, not a probabilistic hash. Hard-point maps are
	// small, and callers can still mutate them in place without an epoch bump.
	hard := make([]byte, 8*len(ids))
	for i, id := range ids {
		binary.LittleEndian.PutUint32(hard[i*8:], uint32(id))
		binary.LittleEndian.PutUint32(hard[i*8+4:], uint32(ch.Skills[id]))
	}
	return skillRawCacheKey{character: ch, worn: p.Player.Worn, revision: p.Player.Worn.EffectsRevision(),
		charID: ch.ID, profession: ch.Race, level: ch.Level, base: ch.EffectiveBase(), hard: string(hard),
		employed: ch.Employed, employmentKnown: ch.EmploymentKnown, gender: ch.Appear.Gender}
}
