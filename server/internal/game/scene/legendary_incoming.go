package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// A missing skill ID is not damage provenance: hazards, reflection and resource
// ticks also use zero. Only an accepted native attack may be a basic attack.
func legendaryNativeBasic(b combat.Blow) bool {
	return b.SkillID == 0 && b.Origin == domain.DamageNormalOrigin && b.ProcDepth == 0 && !b.LegendaryChild && !b.LegendaryPayload && !b.Trap && !b.SourceTrap
}

// Defensive value modifiers must still work while an attack is inside a proc or
// set callback. They use the victim's live equipment, never the attacker's
// accepted source snapshot. Generating callbacks keep their existing guards.
func (s *Scene) legendaryIncomingContexts(src, dst *entity.Entity, blow combat.Blow, hit event.DamageDealt) []legendaryContext {
	contexts := s.legendaryContextsMode(dst, src, blow, hit, true, true)
	for i := range contexts {
		contexts[i].BeforeHP = dst.HP
		contexts[i].PreviousHP = dst.HP
	}
	return contexts
}

// This is a serial post-native-mitigation, pre-shield layer. Only explicitly
// opted-in pure reductions run here, and exactly once per new damage amount.
// Settled echoes intentionally never enter it: their amount already includes
// the original victim's reductions. In particular it does not call the warrior
// additive bucket a second time or broaden explicitly direct-only defenses.
func legendaryAllDamageIncoming(contexts []legendaryContext, amount int32) int32 {
	for _, c := range contexts {
		h := legendaryHandlers[c.Power.ID]
		if h.AllDamageIncoming && h.BeforeIncoming != nil {
			amount = max(int32(0), h.BeforeIncoming(&c, amount))
		}
	}
	return amount
}

func (s *Scene) legendarySecondaryIncoming(src, dst *entity.Entity, blow combat.Blow, amount int32) int32 {
	if amount <= 0 || dst == nil {
		return amount
	}
	hit := event.DamageDealt{Dst: dst.ID, Origin: blow.Origin, SkillID: blow.SkillID, ProcDepth: blow.ProcDepth}
	if src != nil {
		hit.Src = src.ID
	}
	if blow.Magic {
		hit.Flag |= event.DamageMagic
	}
	return legendaryAllDamageIncoming(s.legendaryIncomingContexts(src, dst, blow, hit), amount)
}

// All periodic HP damage is magic. Source metadata only refines its element
// and lineage; a missing source must not turn a tick into a basic attack.
func (s *Scene) legendaryPeriodicBlow(a domain.Active) combat.Blow {
	skill := a.Def.SkillSnapshotID
	if skill == 0 {
		skill = domain.SkillID(a.Def.SourceSkillID)
	}
	b := combat.Blow{Magic: true, Origin: domain.DamageDOTOrigin, SkillID: int32(skill), SourceSkillRange: a.Def.DamageSourceRange, SourceTrap: a.Def.DamageSourceTrap}
	if a.Def.LegendaryPowerID != "" {
		b.ProcDepth = 1
	}
	if d, ok := s.skills.Get(skill, max(int32(1), a.Def.SkillSnapshotRank)); ok {
		b.HurtType = d.HurtType
	}
	return b
}

// The loader normalizes several native buffs into typed fields rather than
// Affixes. Scaling only positive Affixes silently omitted defense, reflection,
// and next-cast benefits. Keep probabilities and identity/lifetime metadata out
// of this magnitude bucket; each has its own semantics.
func legendaryScaleNativeBuffFields(d *domain.StatusDef, bp int32) {
	if d == nil || bp <= 0 || d.Harmful() {
		return
	}
	for _, value := range []*int32{
		&d.PhysicalDamageReductionPct, &d.MagicDamageReductionPct,
		&d.PhysicalReflectPct, &d.MagicReflectPct, &d.PhysicalReflectFlat, &d.MagicReflectFlat,
		&d.NextBasicAttackPct, &d.NextSkillDamagePct, &d.NextStatusDurationPct, &d.NextCastTimeReductionPct,
		&d.StealthDamagePct, &d.LifeStealPct, &d.ManaStealPct,
		&d.OutgoingMagicDamageFlat, &d.MagicDamageReductionFlat, &d.ManaToHealthRate,
	} {
		if *value > 0 {
			*value = universalScaleBP(*value, bp)
		}
	}
}
