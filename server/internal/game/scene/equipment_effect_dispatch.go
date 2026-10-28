package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

// dispatchEquipmentEffect is the shared runtime ingress. Source adapters retain
// their existing wear/requirement/snapshot gates; registration does not grant an
// effect. Only event producers enter ancestry. Pure modifiers remain applicable
// to their own children and distinct producers freely compose in either order.
func (s *Scene) dispatchEquipmentEffect(owner *entity.Entity, effect domain.RuntimeEquipmentEffect, fn func()) bool {
	if owner == nil || fn == nil || !effect.Registered() {
		return false
	}
	if effect.Producer {
		entered, restore := s.enterCombatEffect(owner.ID, effect.ID)
		if !entered {
			return false
		}
		defer restore()
	}
	return domain.DispatchEquipmentEffect(effect, fn)
}

func (c *legendaryContext) dispatchEquipmentHook(trigger string, producer bool, fn func()) bool {
	effect := domain.RuntimeLegendaryEquipmentEffect(domain.ActiveLegendaryPower{SourceUID: c.SourceUID, PowerRollBP: c.PowerRollBP, Power: c.Power}, producer)
	effect.Trigger = trigger
	// Contexts can be retained while a sibling executes. Restore their captured
	// ancestry rather than inheriting an unrelated sibling's producer chain.
	if producer {
		defer c.Scene.withCombatCausalPath(c.CausalPath)()
	}
	return c.Scene.dispatchEquipmentEffect(c.Owner, effect, fn)
}
func (c *legendaryContext) invokeModifier(fn func(*legendaryContext)) {
	c.dispatchEquipmentHook("modifier", false, func() { fn(c) })
}

// Isolated pure-handler callers need no scene or owner. Registration supplies
// identity if their context omits it, without bypassing the common registry.
func (c *legendaryContext) dispatchRegisteredEquipmentHook(id, trigger string, producer bool, fn func()) bool {
	if c.Power.ID != "" && c.Power.ID != id {
		return false
	}
	if c.Power.ID == "" {
		copy := *c
		copy.Power, _ = domain.LegendaryPower(id)
		c = &copy
	}
	if c.Scene == nil || c.Owner == nil {
		if producer || !legendaryPureEquipmentHook(trigger) {
			return false
		}
		effect := domain.RuntimeLegendaryEquipmentEffect(domain.ActiveLegendaryPower{Power: c.Power, PowerRollBP: c.PowerRollBP, SourceUID: c.SourceUID}, false)
		effect.Trigger = trigger
		return domain.DispatchEquipmentEffect(effect, fn)
	}
	return c.dispatchEquipmentHook(trigger, producer, fn)
}

// Only pure calculation hooks support scene-free callers (previews/unit models).
func legendaryPureEquipmentHook(trigger string) bool {
	switch trigger {
	case "StatusApplied", "ModifyAppearance", "StatusAccuracy", "IncomingHeal", "ModifyBlow", "ModifyIncoming", "ShieldDamageBP", "DisplacementImmune", "PassiveStats", "ModifyDamage", "NativeRestoreAmount", "TransformSkill", "BaseSkill", "ComposeSkill", "ImpactTargets", "SelectTargets", "TargetDefinition", "PrepareSkill", "BeforeDamage", "BeforeIncoming":
		return true
	}
	return false
}
