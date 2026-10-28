// Code generated from the typed legendaryHandler contract; no reflection at runtime.
package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

// bindLegendaryEquipmentHandlers routes every registered hook, including pure
// modifiers, through the same ingress as legacy equipment. Event invocation
// owns causal scope; these inner payload adapters must not enter it twice.
func bindLegendaryEquipmentHandlers(id string, h legendaryHandler) legendaryHandler {
	if fn := h.ModifyAppearance; fn != nil {
		h.ModifyAppearance = func(c *legendaryContext, a0 *domain.Appearance) {
			c.dispatchRegisteredEquipmentHook(id, "ModifyAppearance", false, func() { fn(c, a0) })
		}
	}
	if fn := h.StatusAccuracy; fn != nil {
		h.StatusAccuracy = func(c *legendaryContext, a0 *domain.StatusDef) {
			c.dispatchRegisteredEquipmentHook(id, "StatusAccuracy", false, func() { fn(c, a0) })
		}
	}
	if fn := h.IncomingHeal; fn != nil {
		h.IncomingHeal = func(c *legendaryContext, a0 int32) int32 {
			var result int32
			c.dispatchRegisteredEquipmentHook(id, "IncomingHeal", false, func() { result = fn(c, a0) })
			return result
		}
	}
	if fn := h.ModifyBlow; fn != nil {
		h.ModifyBlow = func(c *legendaryContext, a0 *combat.Blow) {
			c.dispatchRegisteredEquipmentHook(id, "ModifyBlow", false, func() { fn(c, a0) })
		}
	}
	if fn := h.EquipmentChanged; fn != nil {
		h.EquipmentChanged = func(c *legendaryContext) {
			c.dispatchRegisteredEquipmentHook(id, "EquipmentChanged", false, func() { fn(c) })
		}
	}
	if fn := h.AbsorbIncoming; fn != nil {
		h.AbsorbIncoming = func(c *legendaryContext, a0 int32) int32 {
			var result int32
			c.dispatchRegisteredEquipmentHook(id, "AbsorbIncoming", false, func() { result = fn(c, a0) })
			return result
		}
	}
	if fn := h.TheftAttempt; fn != nil {
		h.TheftAttempt = func(c *legendaryContext) {
			c.dispatchRegisteredEquipmentHook(id, "TheftAttempt", false, func() { fn(c) })
		}
	}
	if fn := h.PeriodicResolved; fn != nil {
		h.PeriodicResolved = func(c *legendaryContext, a0 domain.Active) {
			c.dispatchRegisteredEquipmentHook(id, "PeriodicResolved", false, func() { fn(c, a0) })
		}
	}
	if fn := h.MagicResisted; fn != nil {
		h.MagicResisted = func(c *legendaryContext, a0 int32) int32 {
			var result int32
			c.dispatchRegisteredEquipmentHook(id, "MagicResisted", false, func() { result = fn(c, a0) })
			return result
		}
	}
	if fn := h.ModifyIncoming; fn != nil {
		h.ModifyIncoming = func(c *legendaryContext, a0 int32) int32 {
			var result int32
			c.dispatchRegisteredEquipmentHook(id, "ModifyIncoming", false, func() { result = fn(c, a0) })
			return result
		}
	}
	if fn := h.ShieldDamageBP; fn != nil {
		h.ShieldDamageBP = func(c *legendaryContext) int32 {
			var result int32
			c.dispatchRegisteredEquipmentHook(id, "ShieldDamageBP", false, func() { result = fn(c) })
			return result
		}
	}
	if fn := h.OwnerPoisonDeath; fn != nil {
		h.OwnerPoisonDeath = func(c *legendaryContext, a0 domain.Active, a1 *entity.Entity) {
			c.dispatchRegisteredEquipmentHook(id, "OwnerPoisonDeath", false, func() { fn(c, a0, a1) })
		}
	}
	if fn := h.DisplacementImmune; fn != nil {
		h.DisplacementImmune = func(c *legendaryContext) bool {
			var result bool
			c.dispatchRegisteredEquipmentHook(id, "DisplacementImmune", false, func() { result = fn(c) })
			return result
		}
	}
	if fn := h.StatusCommitted; fn != nil {
		h.StatusCommitted = func(c *legendaryContext, a0 *domain.StatusDef) {
			c.dispatchRegisteredEquipmentHook(id, "StatusCommitted", false, func() { fn(c, a0) })
		}
	}
	if fn := h.PassiveStats; fn != nil {
		h.PassiveStats = func(c *legendaryContext, a0 *domain.Stats) {
			c.dispatchRegisteredEquipmentHook(id, "PassiveStats", false, func() { fn(c, a0) })
		}
	}
	if fn := h.StatusReceiving; fn != nil {
		h.StatusReceiving = func(c *legendaryContext, a0 *domain.StatusDef) bool {
			var result bool
			c.dispatchRegisteredEquipmentHook(id, "StatusReceiving", false, func() { result = fn(c, a0) })
			return result
		}
	}
	if fn := h.TrapPlaced; fn != nil {
		h.TrapPlaced = func(c *legendaryContext, a0 *entity.Entity, a1 bool) {
			c.dispatchRegisteredEquipmentHook(id, "TrapPlaced", false, func() { fn(c, a0, a1) })
		}
	}
	if fn := h.ModifyDamage; fn != nil {
		h.ModifyDamage = func(c *legendaryContext, a0 int32) int32 {
			var result int32
			c.dispatchRegisteredEquipmentHook(id, "ModifyDamage", false, func() { result = fn(c, a0) })
			return result
		}
	}
	if fn := h.SkillResolved; fn != nil {
		h.SkillResolved = func(c *legendaryContext) {
			c.dispatchRegisteredEquipmentHook(id, "SkillResolved", false, func() { fn(c) })
		}
	}
	if fn := h.HealResolved; fn != nil {
		h.HealResolved = func(c *legendaryContext, a0 int32, a1 int32, a2 int32) {
			c.dispatchRegisteredEquipmentHook(id, "HealResolved", false, func() { fn(c, a0, a1, a2) })
		}
	}
	if fn := h.NativeRestoreAmount; fn != nil {
		h.NativeRestoreAmount = func(c *legendaryContext, a0 int32, a1 []equipmentHit) int32 {
			var result int32
			c.dispatchRegisteredEquipmentHook(id, "NativeRestoreAmount", false, func() { result = fn(c, a0, a1) })
			return result
		}
	}
	if fn := h.NativeRestoreResolved; fn != nil {
		h.NativeRestoreResolved = func(c *legendaryContext, a0 int32, a1 int32) {
			c.dispatchRegisteredEquipmentHook(id, "NativeRestoreResolved", false, func() { fn(c, a0, a1) })
		}
	}
	if fn := h.TransformSkill; fn != nil {
		h.TransformSkill = func(c *legendaryContext, a0 *domain.SkillDef) {
			c.dispatchRegisteredEquipmentHook(id, "TransformSkill", false, func() { fn(c, a0) })
		}
	}
	if fn := h.BaseSkill; fn != nil {
		h.BaseSkill = func(c *legendaryContext, a0 *domain.SkillDef) {
			c.dispatchRegisteredEquipmentHook(id, "BaseSkill", false, func() { fn(c, a0) })
		}
	}
	if fn := h.ComposeSkill; fn != nil {
		h.ComposeSkill = func(c *legendaryContext, a0 *domain.SkillDef) {
			c.dispatchRegisteredEquipmentHook(id, "ComposeSkill", false, func() { fn(c, a0) })
		}
	}
	if fn := h.ImpactTargets; fn != nil {
		h.ImpactTargets = func(c *legendaryContext, a0 []domain.EntityID) []domain.EntityID {
			var result []domain.EntityID
			c.dispatchRegisteredEquipmentHook(id, "ImpactTargets", false, func() { result = fn(c, a0) })
			return result
		}
	}
	if fn := h.CastAccepted; fn != nil {
		h.CastAccepted = func(c *legendaryContext) {
			c.dispatchRegisteredEquipmentHook(id, "CastAccepted", false, func() { fn(c) })
		}
	}
	if fn := h.Resurrected; fn != nil {
		h.Resurrected = func(c *legendaryContext) {
			c.dispatchRegisteredEquipmentHook(id, "Resurrected", false, func() { fn(c) })
		}
	}
	if fn := h.SelectTargets; fn != nil {
		h.SelectTargets = func(c *legendaryContext, a0 []domain.EntityID) []domain.EntityID {
			var result []domain.EntityID
			c.dispatchRegisteredEquipmentHook(id, "SelectTargets", false, func() { result = fn(c, a0) })
			return result
		}
	}
	if fn := h.TargetDefinition; fn != nil {
		h.TargetDefinition = func(c *legendaryContext, a0 *domain.SkillDef) {
			c.dispatchRegisteredEquipmentHook(id, "TargetDefinition", false, func() { fn(c, a0) })
		}
	}
	if fn := h.Moved; fn != nil {
		h.Moved = func(c *legendaryContext, a0 domain.Pos, a1 domain.Pos, a2 bool, a3 bool) {
			c.dispatchRegisteredEquipmentHook(id, "Moved", false, func() { fn(c, a0, a1, a2, a3) })
		}
	}
	if fn := h.Teleported; fn != nil {
		h.Teleported = func(c *legendaryContext, a0 domain.Pos) {
			c.dispatchRegisteredEquipmentHook(id, "Teleported", false, func() { fn(c, a0) })
		}
	}
	if fn := h.Healed; fn != nil {
		h.Healed = func(c *legendaryContext, a0 int32, a1 int32) {
			c.dispatchRegisteredEquipmentHook(id, "Healed", false, func() { fn(c, a0, a1) })
		}
	}
	if fn := h.ManaRestored; fn != nil {
		h.ManaRestored = func(c *legendaryContext, a0 int32, a1 int32) {
			c.dispatchRegisteredEquipmentHook(id, "ManaRestored", false, func() { fn(c, a0, a1) })
		}
	}
	if fn := h.Dispelled; fn != nil {
		h.Dispelled = func(c *legendaryContext, a0 []domain.Active) {
			c.dispatchRegisteredEquipmentHook(id, "Dispelled", false, func() { fn(c, a0) })
		}
	}
	if fn := h.StatusApplied; fn != nil {
		h.StatusApplied = func(c *legendaryContext, a0 *domain.StatusDef) {
			c.dispatchRegisteredEquipmentHook(id, "StatusApplied", false, func() { fn(c, a0) })
		}
	}
	if fn := h.PrepareSkill; fn != nil {
		h.PrepareSkill = func(c *legendaryContext, a0 *domain.SkillDef) {
			c.dispatchRegisteredEquipmentHook(id, "PrepareSkill", false, func() { fn(c, a0) })
		}
	}
	if fn := h.BeforeDamage; fn != nil {
		h.BeforeDamage = func(c *legendaryContext, a0 int32) int32 {
			var result int32
			c.dispatchRegisteredEquipmentHook(id, "BeforeDamage", false, func() { result = fn(c, a0) })
			return result
		}
	}
	if fn := h.BeforeIncoming; fn != nil {
		h.BeforeIncoming = func(c *legendaryContext, a0 int32) int32 {
			var result int32
			c.dispatchRegisteredEquipmentHook(id, "BeforeIncoming", false, func() { result = fn(c, a0) })
			return result
		}
	}
	if fn := h.AfterHit; fn != nil {
		h.AfterHit = func(c *legendaryContext) {
			c.dispatchRegisteredEquipmentHook(id, "AfterHit", false, func() { fn(c) })
		}
	}
	if fn := h.AfterIncoming; fn != nil {
		h.AfterIncoming = func(c *legendaryContext) {
			c.dispatchRegisteredEquipmentHook(id, "AfterIncoming", false, func() { fn(c) })
		}
	}
	if fn := h.CastReleased; fn != nil {
		h.CastReleased = func(c *legendaryContext) {
			c.dispatchRegisteredEquipmentHook(id, "CastReleased", false, func() { fn(c) })
		}
	}
	if fn := h.RootFinished; fn != nil {
		h.RootFinished = func(c *legendaryContext) {
			c.dispatchRegisteredEquipmentHook(id, "RootFinished", false, func() { fn(c) })
		}
	}
	if fn := h.Tick; fn != nil {
		h.Tick = func(c *legendaryContext) {
			c.dispatchRegisteredEquipmentHook(id, "Tick", false, func() { fn(c) })
		}
	}
	return h
}
