package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

// legendaryAugment layers additional typed behaviour onto an already registered
// power without replacing its original handler. Both parts run through the same
// equipment-hook binding. Void hooks run in order; value hooks chain the value;
// StatusReceiving keeps a status only if both accept it; DisplacementImmune and
// AllDamageIncoming combine by OR; ShieldDamageBP sums.
func legendaryAugment(id string, extra legendaryHandler) {
	base, ok := legendaryHandlers[id]
	if !ok {
		panic("augmenting unregistered legendary handler " + id)
	}
	extra = bindLegendaryEquipmentHandlers(id, extra)
	base.AllDamageIncoming = base.AllDamageIncoming || extra.AllDamageIncoming
	if e := extra.ModifyAppearance; e != nil {
		if b := base.ModifyAppearance; b != nil {
			base.ModifyAppearance = func(a0 *legendaryContext, a1 *domain.Appearance) {
				b(a0, a1)
				e(a0, a1)
			}
		} else {
			base.ModifyAppearance = e
		}
	}
	if e := extra.StatusAccuracy; e != nil {
		if b := base.StatusAccuracy; b != nil {
			base.StatusAccuracy = func(a0 *legendaryContext, a1 *domain.StatusDef) {
				b(a0, a1)
				e(a0, a1)
			}
		} else {
			base.StatusAccuracy = e
		}
	}
	if e := extra.IncomingHeal; e != nil {
		if b := base.IncomingHeal; b != nil {
			base.IncomingHeal = func(a0 *legendaryContext, a1 int32) int32 {
				return e(a0, b(a0, a1))
			}
		} else {
			base.IncomingHeal = e
		}
	}
	if e := extra.ModifyBlow; e != nil {
		if b := base.ModifyBlow; b != nil {
			base.ModifyBlow = func(a0 *legendaryContext, a1 *combat.Blow) {
				b(a0, a1)
				e(a0, a1)
			}
		} else {
			base.ModifyBlow = e
		}
	}
	if e := extra.EquipmentChanged; e != nil {
		if b := base.EquipmentChanged; b != nil {
			base.EquipmentChanged = func(a0 *legendaryContext) {
				b(a0)
				e(a0)
			}
		} else {
			base.EquipmentChanged = e
		}
	}
	if e := extra.AbsorbIncoming; e != nil {
		if b := base.AbsorbIncoming; b != nil {
			base.AbsorbIncoming = func(a0 *legendaryContext, a1 int32) int32 {
				return e(a0, b(a0, a1))
			}
		} else {
			base.AbsorbIncoming = e
		}
	}
	if e := extra.TheftAttempt; e != nil {
		if b := base.TheftAttempt; b != nil {
			base.TheftAttempt = func(a0 *legendaryContext) {
				b(a0)
				e(a0)
			}
		} else {
			base.TheftAttempt = e
		}
	}
	if e := extra.PeriodicResolved; e != nil {
		if b := base.PeriodicResolved; b != nil {
			base.PeriodicResolved = func(a0 *legendaryContext, a1 domain.Active) {
				b(a0, a1)
				e(a0, a1)
			}
		} else {
			base.PeriodicResolved = e
		}
	}
	if e := extra.MagicResisted; e != nil {
		if b := base.MagicResisted; b != nil {
			base.MagicResisted = func(a0 *legendaryContext, a1 int32) int32 {
				return e(a0, b(a0, a1))
			}
		} else {
			base.MagicResisted = e
		}
	}
	if e := extra.ModifyIncoming; e != nil {
		if b := base.ModifyIncoming; b != nil {
			base.ModifyIncoming = func(a0 *legendaryContext, a1 int32) int32 {
				return e(a0, b(a0, a1))
			}
		} else {
			base.ModifyIncoming = e
		}
	}
	if e := extra.ShieldDamageBP; e != nil {
		if b := base.ShieldDamageBP; b != nil {
			base.ShieldDamageBP = func(a0 *legendaryContext) int32 {
				return b(a0) + e(a0)
			}
		} else {
			base.ShieldDamageBP = e
		}
	}
	if e := extra.OwnerPoisonDeath; e != nil {
		if b := base.OwnerPoisonDeath; b != nil {
			base.OwnerPoisonDeath = func(a0 *legendaryContext, a1 domain.Active, a2 *entity.Entity) {
				b(a0, a1, a2)
				e(a0, a1, a2)
			}
		} else {
			base.OwnerPoisonDeath = e
		}
	}
	if e := extra.DisplacementImmune; e != nil {
		if b := base.DisplacementImmune; b != nil {
			base.DisplacementImmune = func(a0 *legendaryContext) bool {
				return b(a0) || e(a0)
			}
		} else {
			base.DisplacementImmune = e
		}
	}
	if e := extra.StatusCommitted; e != nil {
		if b := base.StatusCommitted; b != nil {
			base.StatusCommitted = func(a0 *legendaryContext, a1 *domain.StatusDef) {
				b(a0, a1)
				e(a0, a1)
			}
		} else {
			base.StatusCommitted = e
		}
	}
	if e := extra.PassiveStats; e != nil {
		if b := base.PassiveStats; b != nil {
			base.PassiveStats = func(a0 *legendaryContext, a1 *domain.Stats) {
				b(a0, a1)
				e(a0, a1)
			}
		} else {
			base.PassiveStats = e
		}
	}
	if e := extra.StatusReceiving; e != nil {
		if b := base.StatusReceiving; b != nil {
			base.StatusReceiving = func(a0 *legendaryContext, a1 *domain.StatusDef) bool {
				if !b(a0, a1) {
					return false
				}
				return e(a0, a1)
			}
		} else {
			base.StatusReceiving = e
		}
	}
	if e := extra.TrapPlaced; e != nil {
		if b := base.TrapPlaced; b != nil {
			base.TrapPlaced = func(a0 *legendaryContext, a1 *entity.Entity, a2 bool) {
				b(a0, a1, a2)
				e(a0, a1, a2)
			}
		} else {
			base.TrapPlaced = e
		}
	}
	if e := extra.ModifyDamage; e != nil {
		if b := base.ModifyDamage; b != nil {
			base.ModifyDamage = func(a0 *legendaryContext, a1 int32) int32 {
				return e(a0, b(a0, a1))
			}
		} else {
			base.ModifyDamage = e
		}
	}
	if e := extra.SkillResolved; e != nil {
		if b := base.SkillResolved; b != nil {
			base.SkillResolved = func(a0 *legendaryContext) {
				b(a0)
				e(a0)
			}
		} else {
			base.SkillResolved = e
		}
	}
	if e := extra.HealResolved; e != nil {
		if b := base.HealResolved; b != nil {
			base.HealResolved = func(a0 *legendaryContext, a1 int32, a2 int32, a3 int32) {
				b(a0, a1, a2, a3)
				e(a0, a1, a2, a3)
			}
		} else {
			base.HealResolved = e
		}
	}
	if e := extra.NativeRestoreAmount; e != nil {
		if b := base.NativeRestoreAmount; b != nil {
			base.NativeRestoreAmount = func(a0 *legendaryContext, a1 int32, a2 []equipmentHit) int32 {
				return e(a0, b(a0, a1, a2), a2)
			}
		} else {
			base.NativeRestoreAmount = e
		}
	}
	if e := extra.NativeRestoreResolved; e != nil {
		if b := base.NativeRestoreResolved; b != nil {
			base.NativeRestoreResolved = func(a0 *legendaryContext, a1 int32, a2 int32) {
				b(a0, a1, a2)
				e(a0, a1, a2)
			}
		} else {
			base.NativeRestoreResolved = e
		}
	}
	if e := extra.TransformSkill; e != nil {
		if b := base.TransformSkill; b != nil {
			base.TransformSkill = func(a0 *legendaryContext, a1 *domain.SkillDef) {
				b(a0, a1)
				e(a0, a1)
			}
		} else {
			base.TransformSkill = e
		}
	}
	if e := extra.BaseSkill; e != nil {
		if b := base.BaseSkill; b != nil {
			base.BaseSkill = func(a0 *legendaryContext, a1 *domain.SkillDef) {
				b(a0, a1)
				e(a0, a1)
			}
		} else {
			base.BaseSkill = e
		}
	}
	if e := extra.ComposeSkill; e != nil {
		if b := base.ComposeSkill; b != nil {
			base.ComposeSkill = func(a0 *legendaryContext, a1 *domain.SkillDef) {
				b(a0, a1)
				e(a0, a1)
			}
		} else {
			base.ComposeSkill = e
		}
	}
	if e := extra.ImpactTargets; e != nil {
		if b := base.ImpactTargets; b != nil {
			base.ImpactTargets = func(a0 *legendaryContext, a1 []domain.EntityID) []domain.EntityID {
				return e(a0, b(a0, a1))
			}
		} else {
			base.ImpactTargets = e
		}
	}
	if e := extra.CastAccepted; e != nil {
		if b := base.CastAccepted; b != nil {
			base.CastAccepted = func(a0 *legendaryContext) {
				b(a0)
				e(a0)
			}
		} else {
			base.CastAccepted = e
		}
	}
	if e := extra.Resurrected; e != nil {
		if b := base.Resurrected; b != nil {
			base.Resurrected = func(a0 *legendaryContext) {
				b(a0)
				e(a0)
			}
		} else {
			base.Resurrected = e
		}
	}
	if e := extra.SelectTargets; e != nil {
		if b := base.SelectTargets; b != nil {
			base.SelectTargets = func(a0 *legendaryContext, a1 []domain.EntityID) []domain.EntityID {
				return e(a0, b(a0, a1))
			}
		} else {
			base.SelectTargets = e
		}
	}
	if e := extra.TargetDefinition; e != nil {
		if b := base.TargetDefinition; b != nil {
			base.TargetDefinition = func(a0 *legendaryContext, a1 *domain.SkillDef) {
				b(a0, a1)
				e(a0, a1)
			}
		} else {
			base.TargetDefinition = e
		}
	}
	if e := extra.Moved; e != nil {
		if b := base.Moved; b != nil {
			base.Moved = func(a0 *legendaryContext, a1 domain.Pos, a2 domain.Pos, a3 bool, a4 bool) {
				b(a0, a1, a2, a3, a4)
				e(a0, a1, a2, a3, a4)
			}
		} else {
			base.Moved = e
		}
	}
	if e := extra.Teleported; e != nil {
		if b := base.Teleported; b != nil {
			base.Teleported = func(a0 *legendaryContext, a1 domain.Pos) {
				b(a0, a1)
				e(a0, a1)
			}
		} else {
			base.Teleported = e
		}
	}
	if e := extra.Healed; e != nil {
		if b := base.Healed; b != nil {
			base.Healed = func(a0 *legendaryContext, a1 int32, a2 int32) {
				b(a0, a1, a2)
				e(a0, a1, a2)
			}
		} else {
			base.Healed = e
		}
	}
	if e := extra.ManaRestored; e != nil {
		if b := base.ManaRestored; b != nil {
			base.ManaRestored = func(a0 *legendaryContext, a1 int32, a2 int32) {
				b(a0, a1, a2)
				e(a0, a1, a2)
			}
		} else {
			base.ManaRestored = e
		}
	}
	if e := extra.Dispelled; e != nil {
		if b := base.Dispelled; b != nil {
			base.Dispelled = func(a0 *legendaryContext, a1 []domain.Active) {
				b(a0, a1)
				e(a0, a1)
			}
		} else {
			base.Dispelled = e
		}
	}
	if e := extra.StatusApplied; e != nil {
		if b := base.StatusApplied; b != nil {
			base.StatusApplied = func(a0 *legendaryContext, a1 *domain.StatusDef) {
				b(a0, a1)
				e(a0, a1)
			}
		} else {
			base.StatusApplied = e
		}
	}
	if e := extra.PrepareSkill; e != nil {
		if b := base.PrepareSkill; b != nil {
			base.PrepareSkill = func(a0 *legendaryContext, a1 *domain.SkillDef) {
				b(a0, a1)
				e(a0, a1)
			}
		} else {
			base.PrepareSkill = e
		}
	}
	if e := extra.BeforeDamage; e != nil {
		if b := base.BeforeDamage; b != nil {
			base.BeforeDamage = func(a0 *legendaryContext, a1 int32) int32 {
				return e(a0, b(a0, a1))
			}
		} else {
			base.BeforeDamage = e
		}
	}
	if e := extra.BeforeIncoming; e != nil {
		if b := base.BeforeIncoming; b != nil {
			base.BeforeIncoming = func(a0 *legendaryContext, a1 int32) int32 {
				return e(a0, b(a0, a1))
			}
		} else {
			base.BeforeIncoming = e
		}
	}
	if e := extra.AfterHit; e != nil {
		if b := base.AfterHit; b != nil {
			base.AfterHit = func(a0 *legendaryContext) {
				b(a0)
				e(a0)
			}
		} else {
			base.AfterHit = e
		}
	}
	if e := extra.AfterIncoming; e != nil {
		if b := base.AfterIncoming; b != nil {
			base.AfterIncoming = func(a0 *legendaryContext) {
				b(a0)
				e(a0)
			}
		} else {
			base.AfterIncoming = e
		}
	}
	if e := extra.CastReleased; e != nil {
		if b := base.CastReleased; b != nil {
			base.CastReleased = func(a0 *legendaryContext) {
				b(a0)
				e(a0)
			}
		} else {
			base.CastReleased = e
		}
	}
	if e := extra.RootFinished; e != nil {
		if b := base.RootFinished; b != nil {
			base.RootFinished = func(a0 *legendaryContext) {
				b(a0)
				e(a0)
			}
		} else {
			base.RootFinished = e
		}
	}
	if e := extra.Tick; e != nil {
		if b := base.Tick; b != nil {
			base.Tick = func(a0 *legendaryContext) {
				b(a0)
				e(a0)
			}
		} else {
			base.Tick = e
		}
	}
	legendaryHandlers[id] = base
}
