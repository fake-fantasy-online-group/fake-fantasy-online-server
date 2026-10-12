package scene

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
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
		if d, ok := domain.UniqueOf(st); ok {
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
	return domain.ResolveSkillLoadout(int32(ch.Race)+1, ch.Skills, s.skillRankBonusesResolved(p, resolution), grants)
}

// rawSkillLevels is the single raw, nonrecursive snapshot used by every passive consumer.
// Legacy tables keep exact legacy levels; the returned map never aliases permanent hard points.
func (s *Scene) rawSkillLevels(p *entity.Entity) domain.Learned {
	return s.rawSkillLevelsResolved(p, newEquipmentRequirementResolution())
}

func (s *Scene) rawSkillLevelsResolved(p *entity.Entity, resolution *equipmentRequirementResolution) domain.Learned {
	if p == nil || p.Player == nil || p.Player.Char == nil {
		return nil
	}
	out := p.Player.Char.Skills.Clone()
	hasV5 := false
	for id := range out {
		if s.skills.V5Enabled(id) {
			hasV5 = true
			break
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
	s.applyTrainingSynergiesWithHard(p, def, p.Player.Char.Skills)
}
func (s *Scene) applyTrainingSynergiesWithHard(p *entity.Entity, def *domain.SkillDef, hard domain.Learned) {
	if def == nil || def.FormulaVersion != domain.SkillFormulaVersion || p == nil || p.Player == nil {
		return
	}
	var damageBP, healBP, healFlat, manaBP, radiusBP, widthBP, delayBP, durationBP int32
	for _, e := range domain.V5SkillSynergies {
		if e.Target != def.ID || e.RankMode != "hard" {
			continue
		}
		h := hard.LevelOf(e.Source)
		if h <= 0 {
			continue
		}
		v := e.PerPointBP * h
		switch e.Effect {
		case "damage":
			damageBP += v
		case "elite_damage":
			def.TrainingEliteDamageBP += v
		case "heal_power":
			healBP += v
		case "heal_flat":
			healFlat += v / 100
		case "mana_economy":
			manaBP += v
		case "radius":
			radiusBP += v
		case "width":
			widthBP += v
		case "arm_delay_economy":
			delayBP += v
		case "freeze_duration":
			durationBP += v
		}
	}
	def.TrainingDamageBP = damageBP
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
				e.HealFlat = domain.ApplyUniqueBP(e.HealFlat, healBP)
				e.HealPctOfMax = domain.ApplyUniqueBP(e.HealPctOfMax, healBP)
				e.HealFlatNumerator *= int64(10000 + healBP)
				e.HealFlatDenominator = max(int64(1), e.HealFlatDenominator) * 10000
				e.HealPctNumerator *= int64(10000 + healBP)
				e.HealPctDenominator = max(int64(1), e.HealPctDenominator) * 10000
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
	key    [32]byte
	levels domain.Learned
}

func skillRawFingerprint(p *entity.Entity) [32]byte {
	ch := p.Player.Char
	var worn []domain.Stack
	p.Player.Worn.Each(func(_ domain.EquipSlot, st domain.Stack) { worn = append(worn, st) })
	b, _ := json.Marshal(struct {
		CharID                    int64
		Profession                domain.Race
		Level                     int32
		Base                      domain.Base
		Hard                      domain.Learned
		Worn                      []domain.Stack
		Employed, EmploymentKnown bool
		Gender                    uint8
	}{ch.ID, ch.Race, ch.Level, ch.EffectiveBase(), ch.Skills, worn, ch.Employed, ch.EmploymentKnown, ch.Appear.Gender})
	return sha256.Sum256(b)
}
