package scene

import (
	"sort"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// 状态运行时: 施加、到期、周期触发、控制判定。
//
// 帧序里排在**最前面**(定时器之后、AI 之前):
// 中毒这一跳该掉的血、昏迷该不该解除, 都得在这一帧的任何人行动之前定下来。
// 排在后面的话会出现"已经到期的昏迷还挡了这一帧的攻击"。

// applyStatus 给目标施加一个状态。
//
// 返回是否真的加上了 —— 被抗性挡下来时返回 false。
func (s *Scene) applyStatus(target *entity.Entity, id domain.StatusID, level int32, src domain.EntityID) bool {
	if s.statuses == nil || target == nil || !target.Alive() {
		return false
	}
	def, ok := s.statuses.Get(id, level)
	if !ok {
		return false
	}
	// Secondary poison/freeze has no direct SkillDef at this point. Carry only
	// presentation metadata from the status that caused it; mechanics stay in def.
	if source := s.entities[src]; source != nil {
		icon, skillID := triggeredStatusIcon(source.Status, id)
		applyStatusSource(&def, icon, skillID)
	}
	return s.applyStatusDef(target, def, src)
}

func triggeredStatusIcon(statuses *domain.StatusSet, id domain.StatusID) (icon, skillID int32) {
	if statuses == nil {
		return
	}
	var strongest int32
	statuses.EachEffective(func(a domain.Active) {
		var chance int32
		switch id {
		case domain.StatusPoison:
			chance = a.Def.PoisonOnHitChance
		case domain.StatusFreeze:
			chance = a.Def.FreezeAttackerChance
		}
		if chance > strongest {
			strongest, icon, skillID = chance, a.Def.Icon, a.Def.SourceSkillID
		}
	})
	return
}

// applyStatusDef 让已确认来源的调用方在不改静态表的前提下补展示元数据。
// 例如物品状态沿用状态定义的数值，但图标来自实际被使用的物品。
func (s *Scene) applyStatusDef(target *entity.Entity, def domain.StatusDef, src domain.EntityID) bool {
	if target == nil || !target.Alive() {
		return false
	}
	s.expireHarmfulSuppression(target)
	if def.Invisible && target.Status != nil && target.Status.PreventsInvisible() {
		return false
	}
	if gmGodEnabled(target) && isHarmful(def) {
		return false
	}
	// 锁灵与混沌是状态类别门禁，和逐状态叠加矩阵是两回事。
	if target.Status != nil && target.Status.BlocksStatus(def) {
		return false
	}
	// 先做确定性的矩阵预检。被已有状态挡住不是“抗性命中”，不能发抵抗提示，
	// 也不该白白消耗本场景的随机数流。
	if target.Status != nil && target.Status.BlockedByOverlay(def, s.statusOverlay) {
		return false
	}
	// Base chance and resistance share exactly one final draw. Buffs only use
	// their authored chance; immunity/overlay failures above never consume RNG.
	if !s.statusEffectLands(target, def, src) {
		return false
	}
	if (def.PoisonOnHitChance > 0 || def.FreezeAttackerChance > 0) && !def.SourceEffectHitKnown {
		def.SourceEffectHitBP = s.sourceEffectHitBP(target)
		def.SourceEffectHitKnown = true
	}
	if target.Status == nil {
		target.Status = domain.NewStatusSet()
	}
	if def.ClearHarmful {
		s.statusesRemoved(target, target.Status.RemoveWhere(func(existing domain.StatusDef) bool {
			return existing.Harmful()
		}))
	}
	wasInvisible := entityInvisible(target)
	// Periodic damage may outlive a pet/summon/trap entity. Keep its responsible
	// player's stable source ID now; MF itself is read only at the fatal tick.
	src = s.periodicDamageOwnerV5(def, src)
	poisonBurst := s.classSetPoisonBurst(target, def, src)
	result := target.Status.ApplyWithOverlay(def, s.tick, src, s.statusOverlay)
	for _, removed := range result.Removed {
		s.emit(event.StatusRemoved{Who: target.ID, Status: int32(removed)})
	}
	s.emitRemovedEntityStatusEffects(target, result.Removed)
	if !result.Applied {
		// Counteraction can remove an effective layer while retaining a
		// suppressed sibling with the same ID. Its cached stats still change.
		if result.Counteracted || len(result.Removed) > 0 {
			s.updatePlayerVisibility(target, wasInvisible)
			s.refreshEntityStats(target)
			s.emitAttributesIfPlayer(target)
			s.pushStatusSnapshots(target)
		}
		return false
	}
	if poisonBurst > 0 {
		s.dealClassSetPoisonBurst(target, src, poisonBurst)
		if !target.Alive() {
			return true
		}
	}
	s.updatePlayerVisibility(target, wasInvisible)
	s.refreshEntityStats(target)
	s.emitAttributesIfPlayer(target)
	s.pushStatusSnapshots(target)
	s.emit(event.StatusApplied{
		Who: target.ID, Status: int32(def.ID), Level: def.Level,
		DurationMS: int64(def.DurationSec) * 1000,
	})
	if target.Kind == domain.KindPlayer && isHarmful(def) {
		s.tryPetCleanse(target, def.ID)
	}
	return true
}

// applyStatusSource 仅为没有原表元数据的状态补来源；原 SmallMap=0 也不可覆盖。
// skillID=0 表示物品；
// 客户端 Buff 身份继续回退状态号，但图标取实际使用的物品。
func applyStatusSource(def *domain.StatusDef, icon, skillID int32) {
	if def == nil || def.IconKnown || def.Icon > 0 || icon <= 0 {
		return
	}
	def.Icon = icon
	def.SourceSkillID = skillID
}

func (s *Scene) passiveStatusResistPct(target *entity.Entity, status domain.StatusID) int32 {
	if s == nil || s.skills == nil || target == nil || target.Player == nil ||
		target.Player.Char == nil || status == 0 {
		return 0
	}
	ch := target.Player.Char
	var total int64
	for skill, level := range s.rawSkillLevels(target) {
		def, ok := s.skills.Get(skill, level)
		if !ok || def.Kind != domain.SkillPassive || !ch.AllowsSkill(def.ID, def.Prof) {
			continue
		}
		for _, resist := range def.PassiveResists {
			if resist.Status == status {
				total += int64(resist.Pct)
			}
		}
	}
	return domain.SaturateInt32(total)
}

// prepareBasicAttackStatuses 在一次已受理的普通攻击起手时消耗“下一击”状态，
// 并处理隐身的“任何攻击行为都会现形”。
func (s *Scene) prepareBasicAttackStatuses(e *entity.Entity, blow *combat.Blow) {
	if e == nil || blow == nil {
		return
	}

	if e.Status == nil {
		return
	}
	bonus := e.Status.StealthDamageBonus()
	blow.FlatDamage += e.Status.ProgressionAttackBonus(false, true, false)
	blow.StartedInvisible = entityInvisible(e)
	oneShotBonus, _, _, removed := e.Status.ConsumeAttackModifiers(false, true, false, false)
	bonus += oneShotBonus
	if bonus > 0 {
		blow.SkillPct = boostedDamagePct(blow.SkillPct, bonus)
	}
	removed = append(removed, e.Status.RemoveEffectiveWhere(func(d domain.StatusDef) bool {
		return d.BreakOnAttack
	})...)
	s.statusesRemoved(e, removed)
}

// prepareSkillStatuses 消耗本次技能真正用得到的释能/集中/瞬发，并返回
// 集中提供的状态时长增幅。范围技能只在整次施法上消费一次。
func (s *Scene) prepareSkillStatuses(e *entity.Entity, def *domain.SkillDef) int32 {
	if e == nil || e.Status == nil || def == nil {
		return 0
	}
	flat := e.Status.ProgressionAttackBonus(true, def.HasDamageEffect(), def.PrepareMS > 0)
	bonus, duration, castReduction, removed := e.Status.ConsumeAttackModifiers(
		true,
		def.HasDamageEffect(),
		len(def.Statuses) > 0,
		def.PrepareMS > 0,
	)
	if def.HasDamageEffect() {
		bonus += e.Status.StealthDamageBonus()
	}
	if bonus > 0 {
		if len(def.Effects) > 0 {
			for i := range def.Effects {
				if def.Effects[i].Kind == domain.EffectDamage {
					domain.BoostSkillDamage(&def.Effects[i], bonus)
				}
			}
			def.Effect = def.Effects[0]
		} else if def.Effect.Kind == domain.EffectDamage {
			domain.BoostSkillDamage(&def.Effect, bonus)
		}
	}
	if flat > 0 {
		def.Effects = append([]domain.SkillEffect(nil), def.DirectEffects()...)
		for i := range def.Effects {
			if def.Effects[i].Kind == domain.EffectDamage {
				def.Effects[i].DamageFlat += flat
			}
		}
		if len(def.Effects) > 0 {
			def.Effect = def.Effects[0]
		}
	}
	def.PrepareMS = skillPrepareAfterReduction(def.PrepareMS, castReduction)
	preserveInvisible := false
	if def.PreserveInvisibleChanceBP > 0 && e.Status.Invisible() {
		chance := def.PreserveInvisibleChanceBP
		if chance > 10000 {
			chance = 10000
		}
		preserveInvisible = chance >= 10000 || int32(s.rng.Intn(10000)) < chance
	}
	removed = append(removed, e.Status.RemoveEffectiveWhere(func(d domain.StatusDef) bool {
		return d.BreakOnAttack && !(preserveInvisible && d.Invisible)
	})...)
	s.statusesRemoved(e, removed)
	return duration
}

func boostedDamagePct(base, bonus int32) int32 {
	if base == 0 {
		base = 100
	}
	return clampI32(int64(base) * (100 + int64(bonus)) / 100)
}

// resolveStrikeStatuses 把普通战斗公式的结果再经过状态减伤/护盾。
func (s *Scene) resolveStrikeStatuses(src, dst *entity.Entity, tr *combat.HitTracker, blow combat.Blow) (event.DamageDealt, int32, int32) {
	s.expireHarmfulSuppression(src)
	s.expireHarmfulSuppression(dst)
	if !s.equipmentCombatReady(src) || !s.equipmentCombatReady(dst) {
		return event.DamageDealt{Src: src.ID, Dst: dst.ID, DstHP: dst.HP, SkillID: blow.SkillID, Flag: event.DamageMiss}, 0, 0
	}
	beforeHP := dst.HP
	blow = s.equipmentBlow(src, blow)
	ev := combat.CalculateStrike(src, dst, tr, blow, s.rng)
	rawDamage := ev.Amount
	if ev.Flag.Has(event.DamageMiss) || rawDamage <= 0 && blow.FlatDamage <= 0 {
		s.collectEquipmentHit(src, dst, blow, ev)
		return ev, rawDamage, 0
	}
	if blow.FlatDamage > 0 {
		rawDamage = clampI32(int64(rawDamage) + int64(blow.FlatDamage))
	}
	if gmGodEnabled(dst) {
		dst.HP = beforeHP
		ev.Amount, ev.DstHP = 0, beforeHP
		ev.Flag &^= event.DamageFatal
		return ev, 0, 0
	}
	magic := ev.Flag.Has(event.DamageMagic)
	rawDamage = clampI32(int64(rawDamage) + int64(s.sourcePetExtraV5(blow.SourceOffense, blow.CurrentSourceOffense, magic)))
	if blow.SourceOffense.StatusPresent || blow.CurrentSourceOffense != nil && blow.CurrentSourceOffense.StatusPresent {
		rawDamage = max(int32(1), clampI32(int64(rawDamage)+int64(sourceStatusFlatV5(blow.SourceOffense, blow.CurrentSourceOffense, magic))))
	}
	pvp := s.equipmentPVP(src, dst)
	element := domain.HurtElementIndex(blow.HurtType)
	var offense, defense domain.ElementCombatStats
	defensive := s.equipmentDefense(dst, pvp)
	if element >= 0 {
		offense = blow.EquipmentOffense.Elements[element]
		defense = defensive.Elements[element]
	}
	x := sourceUniqueEffectsV5(blow.SourceOffense, blow.CurrentSourceOffense, uniqueStrikeContext(src, dst, blow, ev, beforeHP))
	if blow.Origin == domain.DamageProcOrigin {
		x.DamageBP = 0
	}
	buckets := domain.CombatDamageBuckets{
		DamageIncreaseBP: int64(x.DamageBP), PVPScaledDamageIncreaseBP: int64(offense.DamageBP),
		ResistanceBP: defense.ResistBP, ResistancePierceBP: offense.PierceBP,
		ScaledDamageIncreaseNumerator: s.equipmentAuraDamageNumerator(src, element, pvp),
		PayloadScaleBP:                blow.PayloadScaleBP, PayloadScaleKnown: blow.PayloadScaleKnown, PVP: pvp,
	}
	rawDamage = buckets.OutgoingDamage(rawDamage)
	rawDamage = s.casterSetOutgoingDamage(src, blow.HurtType, rawDamage)
	rawDamage = s.classSetMissingHealthDamage(src, rawDamage)
	if root := s.equipmentState().root; root != nil && root.src == src.ID && root.classSetDamageScaleBP > 0 && blow.SkillID == int32(classSetWhirlwind) {
		rawDamage = clampI32(int64(rawDamage) * int64(root.classSetDamageScaleBP) / 10000)
	}
	finalDamage, fullMagicReflect := rawDamage, int32(0)
	removed := []domain.StatusID(nil)
	reflectChance := int32(0)
	if dst.Status != nil {
		reflectChance = dst.Status.MagicFullReflectChance()
	}
	if gmOneShotEnabled(src) {
		finalDamage = beforeHP
	} else if magic && reflectChance > 0 && int32(s.rng.Intn(100)) < reflectChance {
		finalDamage, fullMagicReflect = 0, rawDamage
	} else {
		guard := domain.EvaluateUniqueTraits(s.uniqueTraits(dst), domain.UniqueContext{HPPercent: uniquePercent(beforeHP, dst.MaxHP), Magic: magic})
		extraMitigation := int64(defense.MitigationBP) + int64(guard.MitigationBP)
		if !magic {
			extraMitigation += int64(defensive.PhysicalMitigationBP)
		}
		halfCount := 0
		if chance := s.trapHalfDamageChance(dst); blow.Trap && chance > 0 && int32(s.rng.Intn(10000)) < chance {
			halfCount++
		}
		if chance := s.incomingMagicHalfChance(dst); magic && chance > 0 && int32(s.rng.Intn(10000)) < chance {
			halfCount++
		}
		flatReduction := dst.Status.DamageReductionFlat(magic)
		if dst.Kind == domain.KindPlayer {
			kind := "physical_reduction"
			if magic {
				kind = "magic_reduction"
			}
			flatReduction += int64(s.petProcAmount(dst, kind)) + int64(s.petFlatReduction(dst, magic))
		}
		buckets.MitigationBP = extraMitigation
		buckets.DamageTakenIncreasePct = dst.Status.DamageTakenIncreasePct(magic)
		buckets.DamageReductionPct = dst.Status.DamageReductionPct(magic)
		buckets.DamageReductionCapPct = dst.Status.DamageReductionCapPct()
		buckets.HalfDamageLayers, buckets.FlatReduction = halfCount, flatReduction
		finalDamage = buckets.IncomingDamage(rawDamage)
		finalDamage = s.classSetMeleeIncomingDamage(dst, finalDamage)
		finalDamage = s.classSetAuraIncomingDamage(dst, magic, finalDamage)
		if dst.Status.Invulnerable() {
			finalDamage = 0
		} else {
			finalDamage, removed = dst.Status.AbsorbDamage(finalDamage)
		}
		if dst.Status != nil && dst.Player != nil {
			dst.Player.MarkDirty()
		}
		if dst.Kind == domain.KindPlayer && finalDamage > 0 {
			finalDamage = s.petGuardDamage(dst, src.ID, finalDamage)
		}
	}
	absorbedHeal := int32(0)
	if element >= 0 && fullMagicReflect == 0 && s.equipmentAbsorptionEligible(src, dst, blow) {
		absorbed := s.equipmentAbsorb(dst, finalDamage, defense.AbsorbBP, pvp)
		finalDamage = absorbed.Damage
		absorbedHeal = absorbed.Heal
		dst.HP = absorbed.HP
	} else {
		dst.HP = clampI32(max(int64(0), int64(beforeHP)-int64(finalDamage)))
	}
	if dst.HP <= 0 {
		dst.HP = 0
		ev.Flag |= event.DamageFatal
	} else {
		ev.Flag &^= event.DamageFatal
	}
	ev.Amount, ev.DstHP, ev.AbsorbHeal = finalDamage, dst.HP-absorbedHeal, absorbedHeal
	if finalDamage > 0 {
		s.beginEndgameTrial()
	}
	s.statusesRemoved(dst, removed)
	if beforeHP > 0 && (blow.Origin == domain.DamageNormalOrigin || blow.Origin == domain.DamageCounterOrigin) && blow.ProcDepth == 0 {
		s.afterUniqueStrike(src, dst, ev)
	}
	s.collectEquipmentHit(src, dst, blow, ev)
	return ev, rawDamage, fullMagicReflect
}

func (s *Scene) incomingMagicHalfChance(target *entity.Entity) int32 {
	return s.passiveGlobalChanceBP(target, domain.PassiveIncomingMagicHalfChance)
}

func (s *Scene) trapHalfDamageChance(target *entity.Entity) int32 {
	return s.passiveGlobalChanceBP(target, domain.PassiveTrapHalfDamageChance)
}

func (s *Scene) passiveGlobalChanceBP(target *entity.Entity, kind domain.PassiveModifierKind) int32 {
	if s == nil || s.skills == nil || target == nil || target.Player == nil || target.Player.Char == nil {
		return 0
	}
	var chance int32
	for passiveID, level := range s.rawSkillLevels(target) {
		passive, ok := s.skills.Get(passiveID, level)
		if !ok || passive.Kind != domain.SkillPassive ||
			!target.Player.Char.AllowsSkill(passive.ID, passive.Prof) {
			continue
		}
		for _, modifier := range passive.PassiveModifiers {
			if modifier.Kind == kind {
				chance += modifier.Value * 100
			}
		}
	}
	if chance > 10000 {
		return 10000
	}
	return chance
}

// afterStrikeStatuses 处理命中后的反射、吸血、使毒、冰盾反冻与冰冻碎裂。
// 反射伤害不再触发反射，避免两个反射状态无限递归。
func (s *Scene) afterStrikeStatuses(src, dst *entity.Entity, ev event.DamageDealt, rawDamage, fullMagicReflect int32) {
	s.afterCastDamage(ev, true)
	if ev.AbsorbHeal > 0 && dst != nil {
		s.emit(event.HealDone{Src: dst.ID, Dst: dst.ID, Amount: ev.AbsorbHeal, DstHP: dst.HP, DstKind: dst.Kind})
	}
	if src == nil || dst == nil || ev.Flag.Has(event.DamageMiss) || rawDamage <= 0 {
		return
	}
	if fullMagicReflect > 0 {
		s.emitReflectedDamage(dst, src, fullMagicReflect, true)
		return
	}

	if ev.Amount > 0 && dst.Status != nil {
		s.statusesRemoved(dst, dst.Status.RemoveEffectiveWhere(func(d domain.StatusDef) bool {
			return d.BreakOnDamage
		}))
	}

	if ev.Origin == domain.DamageProcOrigin || ev.Origin == domain.DamageAuraOrigin {
		return
	}

	s.classSetDragonSteal(src, ev)
	magic := ev.Flag.Has(event.DamageMagic)
	if !magic && ev.SkillID == 0 && src.Status != nil && dst.Alive() {
		s.applyCombatTriggeredStatus(src, dst, domain.StatusPoison)
	}
	if !magic && dst.Status != nil && src.Alive() {
		s.applyCombatTriggeredStatus(dst, src, domain.StatusFreeze)
	}

	if src.Status != nil && src.Alive() && ev.Amount > 0 {
		_, _, _, lifePct, lifeChance, manaPct, manaChance := src.Status.CombatTriggers()
		if lifePct > 0 && int32(s.rng.Intn(100)) < lifeChance {
			amount := clampI32(int64(ev.Amount) * int64(lifePct) / 100)
			if amount > 0 {
				s.emit(combat.Restore(src, src, amount, ev.SkillID))
			}
		}
		if manaPct > 0 && int32(s.rng.Intn(100)) < manaChance {
			src.MP = clampI32(int64(src.MP) + int64(ev.Amount)*int64(manaPct)/100)
			if src.MP > src.MaxMP {
				src.MP = src.MaxMP
			}
		}
		s.emitAttributesIfPlayer(src)
	}

	if dst.Status == nil || !src.Alive() {
		return
	}
	reflectPct := dst.Status.ReflectPct(magic)
	reflectFlat, reflectFlatAsMagic := dst.Status.ReflectFlat(magic)
	reflected := clampI32(int64(ev.Amount)*int64(reflectPct)/100 + int64(reflectFlat))
	s.emitReflectedDamage(dst, src, reflected, magic || reflectFlatAsMagic)
}

func (s *Scene) emitReflectedDamage(reflector, attacker *entity.Entity, amount int32, magic bool) {
	if reflector == nil || attacker == nil || amount <= 0 || !attacker.Alive() || gmGodEnabled(attacker) {
		return
	}
	s.expireHarmfulSuppression(attacker)
	amount = s.mitigateSecondaryStatusDamageKind(attacker, magic, amount, true)
	if amount <= 0 {
		return
	}
	attacker.HP -= amount
	reflectEvent := event.DamageDealt{Origin: domain.DamageReflectionOrigin, Src: reflector.ID, Dst: attacker.ID, Amount: amount, DstHP: attacker.HP}
	if magic {
		reflectEvent.Flag |= event.DamageMagic
	}
	if attacker.HP <= 0 {
		attacker.HP = 0
		reflectEvent.DstHP = 0
		reflectEvent.Flag |= event.DamageFatal
	}
	s.emit(reflectEvent)
	s.afterCastDamage(reflectEvent, false)
	if reflectEvent.Amount > 0 && attacker.Status != nil {
		s.statusesRemoved(attacker, attacker.Status.RemoveEffectiveWhere(func(d domain.StatusDef) bool {
			return d.BreakOnDamage
		}))
	}
	s.emitAttributesIfPlayer(attacker)
	if reflectEvent.Flag.Has(event.DamageFatal) {
		s.onDeath(attacker, reflector.ID)
	}
}

// mitigateSecondaryStatusDamage preserves legacy reflection/resource tick
// settlement unless a source explicitly opts into an expanded mitigation
// budget. Such a source uses the same status adapter and common formula as
// direct hits; no ability, monster or scene identity enters the calculation.
func (s *Scene) mitigateSecondaryStatusDamage(e *entity.Entity, magic bool, amount int32) int32 {
	return s.mitigateSecondaryStatusDamageKind(e, magic, amount, false)
}

// Resource/poison ticks have no physical-or-magic classification. Only callers
// with an explicit damage type (reflection) may use typed aura mitigation.
func (s *Scene) mitigateSecondaryStatusDamageKind(e *entity.Entity, magic bool, amount int32, typed bool) int32 {
	if e == nil || e.Status.DamageReductionCapPct() <= domain.DefaultDamageReductionCapPct {
		amount = s.classSetMeleeIncomingDamage(e, amount)
		if typed {
			amount = s.classSetAuraIncomingDamage(e, magic, amount)
		}
		return amount
	}
	// Match direct hits: ordinary incoming buckets (including flat reduction),
	// then independent set multipliers, then immunity and shield consumption.
	buckets := domain.CombatDamageBuckets{
		DamageTakenIncreasePct: e.Status.DamageTakenIncreasePct(magic),
		DamageReductionPct:     e.Status.DamageReductionPct(magic),
		DamageReductionCapPct:  e.Status.DamageReductionCapPct(),
		FlatReduction:          e.Status.DamageReductionFlat(magic),
	}
	amount = s.classSetMeleeIncomingDamage(e, buckets.IncomingDamage(amount))
	if typed {
		amount = s.classSetAuraIncomingDamage(e, magic, amount)
	}
	if e.Status.Invulnerable() {
		return 0
	}
	amount, removed := e.Status.AbsorbDamage(amount)
	s.statusesRemoved(e, removed)
	return amount
}

func (s *Scene) statusesRemoved(e *entity.Entity, ids []domain.StatusID) {
	if e == nil || len(ids) == 0 {
		return
	}
	seen := make(map[domain.StatusID]struct{}, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		s.emit(event.StatusRemoved{Who: e.ID, Status: int32(id)})
	}
	s.emitRemovedEntityStatusEffects(e, ids)
	if !entityInvisible(e) && s.removedStatusesIncludedInvisible(ids) {
		s.updatePlayerVisibility(e, true)
	}
	s.refreshEntityStats(e)
	s.emitAttributesIfPlayer(e)
	s.pushStatusSnapshots(e)
}

// isHarmful 使用 ov_exceptdetail_entry.except_type 的官方 BAD 分类。
// 属性符号只作为旧手工定义的兼容回退，不再裁决生产数据。
func isHarmful(d domain.StatusDef) bool {
	return d.Harmful()
}

// expireHarmfulSuppression synchronizes effective stats at the simulation
// boundary, including callers running before the usual status stage.
func (s *Scene) expireHarmfulSuppression(e *entity.Entity) {
	if e == nil || e.Status == nil {
		return
	}
	s.statusesRemoved(e, e.Status.ExpireHarmfulSuppression(s.tick))
}

// stepStatus 推进所有实体身上的状态。
func (s *Scene) stepStatus() {
	for _, e := range s.entities {
		if e.Player != nil && e.Player.ExpireExperienceBoost(s.tick) {
			e.Player.MarkDirty()
			s.emitTo(e.ID, s.buffSnapshot(e))
		}
		if e.Status == nil || e.Status.Count() == 0 {
			continue
		}
		s.expireHarmfulSuppression(e)
		// 周期触发(中毒那类)
		due := e.Status.DueTicks(s.tick)
		if len(due) > 0 && e.Player != nil {
			e.Player.MarkDirty()
		}
		for _, a := range due {
			s.tickStatus(e, a)
		}
		// 到期清除
		// Only players publish visibility edges; avoid scanning nonplayer statuses
		// for a value updatePlayerVisibility will never consume.
		wasInvisible := false
		if e.Kind == domain.KindPlayer && e.Player != nil {
			wasInvisible = entityInvisible(e)
		}
		if gone := e.Status.Expire(s.tick); len(gone) > 0 {
			for _, id := range gone {
				s.emit(event.StatusRemoved{Who: e.ID, Status: int32(id)})
			}
			s.emitRemovedEntityStatusEffects(e, gone)
			s.updatePlayerVisibility(e, wasInvisible)
			s.refreshEntityStats(e)
			s.emitAttributesIfPlayer(e)
			s.pushStatusSnapshots(e)
		}
	}
}

type statusPresentation struct {
	name, desc string
	icon       event.StatusIconView
	buff       event.BuffView
	effect     event.EntityStatusEffect
}

// statusPositionMode 优先使用客户端实机逐项验收的结果。
// 目前只覆盖验收页原先展示的 52 个状态；未展示、未确认的状态继续沿用旧分类，
// 不能把“这 52 个里的其余状态”擅自扩大成全部客户端状态。
func statusPositionMode(def domain.StatusDef) uint8 {
	switch def.ID {
	case domain.StatusPoison, // 中毒
		domain.StatusSilence,  // 封印
		domain.StatusWeaken,   // 衰弱
		domain.StatusDampen,   // 降幅
		domain.StatusBlind,    // 致盲
		domain.StatusBerserk,  // 狂暴
		1010,                  // 攻击反射
		1011,                  // 魔法反弹
		domain.StatusIronSkin, // 钢铁
		1014,                  // 强化
		1015,                  // 增幅
		1017,                  // 心眼
		1019,                  // 防御增加
		1029,                  // 混沌
		1182,                  // 双倍积分状态
		1208,                  // 大地状态
		1209,                  // 天佑状态
		1210,                  // 巨神状态
		1211,                  // 暗月状态
		1212,                  // 双倍经验状态
		1213,                  // 三倍经验状态
		1214:                  // 一倍半经验状态
		return domain.StatusPositionOrbit
	case domain.StatusStun, // 昏迷
		domain.StatusFreeze,  // 冰冻
		domain.StatusPetrify, // 石化
		domain.StatusSlow,    // 减速
		1016,                 // 加速
		1018,                 // 使毒
		1020,                 // 攻速增加
		1021,                 // 伤害吸收
		1023,                 // 补充生命
		1024,                 // 补充法力
		1025,                 // 杀意
		1026,                 // 集中
		1027,                 // 释能
		1028,                 // 瞬发
		1022,                 // 锁灵
		1032,                 // 隐身
		1034,                 // 定身
		1036,                 // 燃烧
		1037,                 // 火之盾
		1038,                 // 冰之盾
		1039,                 // 化妆状态
		1055,                 // 咒法·褪
		1068,                 // 佛光多宝舍利
		1070,                 // 始源混沌神水
		1074,                 // 狂躁
		1081,                 // 周天辰星护阵
		1095,                 // 神光璧
		1106,                 // 神粮令
		1116,                 // 命中上升
		1145,                 // 般若龙象神水
		1154,                 // 物抗上升
		1158,                 // 物爆上升
		1159,                 // 魔爆上升
		1174,                 // 玄龟之护
		1175,                 // 狮王之力
		1176,                 // 长白参
		1178,                 // 金甲之佑
		1180,                 // 吸收伤害
		1181,                 // 提升生命
		1184,                 // 瑶池琼酿
		1188,                 // 鬼泥
		1206,                 // 灵光状态
		1207:                 // 回春状态
		return 0
	default:
		if def.TypeKnown && (def.Type == domain.StatusGood || def.Type == domain.StatusBad) {
			return domain.StatusPositionOrbit
		}
		return 0
	}
}

// visibleStatuses 把运行时状态在当前帧投影成本人 Buff、实体图标和世界特效视图，
// 按状态号排序。
// no_inform_client 的状态只参与结算，不会从任何展示通道泄漏出去。
func (s *Scene) visibleStatuses(e *entity.Entity) []statusPresentation {
	if e == nil {
		return nil
	}
	out := s.equipmentAuraStatuses(e)
	out = append(out, s.classSetUnrulyPresentation(e)...)
	if e.Status == nil {
		return out
	}
	e.Status.Each(func(a domain.Active) {
		if a.Def.Hidden || (a.ExpireAt != 0 && a.ExpireAt <= s.tick) {
			return
		}
		// Saved statuses created before the native SmallMap import may contain
		// a missing or source-skill icon. Correct presentation from the exact
		// original status level, without changing saved combat/duration values.
		if native, ok := s.statuses.Get(a.Def.ID, a.Def.Level); ok && native.IconKnown {
			a.Def.Icon, a.Def.IconKnown, a.Def.SourceSkillID = native.Icon, true, 0
		}
		remainMS := int32(1<<31 - 1) // 永久状态的客户端展示哨兵
		if a.ExpireAt != 0 {
			remainMS = nonnegativeI32((a.ExpireAt - s.tick).Millis())
		}
		beneficial := !isHarmful(a.Def)
		immobile := a.Def.Immobile
		suppressed := a.Def.Harmful() && e.Status.SuppressesHarmful()
		if immobile == 0 && (a.Def.Control == domain.ControlStun || a.Def.BlockMove) {
			immobile = 1
		}
		if suppressed {
			immobile = 0
		}
		icon := event.StatusIconView{
			Status: int32(a.Def.ID), Icon: a.Def.Icon,
			Beneficial: beneficial, RemainMS: remainMS,
		}
		skillID := a.Def.SourceSkillID
		if skillID == 0 {
			skillID = int32(a.Def.ID)
		}
		effectName, buffName, positionMode := s.statusVisual(a.Def)
		out = append(out, statusPresentation{
			name: a.Def.Name, desc: a.Def.Desc, icon: icon,
			effect: event.EntityStatusEffect{
				Who: e.ID, Effect: effectName,
				PositionMode: positionMode, DurationMS: remainMS,
			},
			buff: event.BuffView{
				// 状态表没有另一个来源技能号；状态号是运行时效果的稳定身份，且避免
				// 多个系统状态都以 0 覆盖客户端同一项。
				SkillID: skillID, Icon: a.Def.Icon,
				RemainSec: int32((int64(remainMS) + 999) / 1000),
				Name:      buffName, Desc: a.Def.Desc, Beneficial: beneficial,
				PositionMode: positionMode, Invisible: a.Def.Invisible && !suppressed, Immobile: immobile,
			},
		})
	})
	sort.Slice(out, func(i, j int) bool { return out[i].icon.Status < out[j].icon.Status })
	return out
}

func (s *Scene) buffSnapshot(e *entity.Entity) event.BuffSnapshot {
	out := event.BuffSnapshot{Who: e.ID}
	if e.Player != nil {
		bonus := e.Status.ExpBonusPct()
		if itemBonus := e.Player.ExperienceBonusPct(s.tick); itemBonus > bonus {
			bonus = itemBonus
		}
		out.DoubleExp = bonus == 100
	}
	for _, st := range s.visibleStatuses(e) {
		out.Buffs = append(out.Buffs, st.buff)
	}
	return out
}

func (s *Scene) pushStatusSnapshots(e *entity.Entity) {
	if e.Player != nil {
		e.Player.MarkDirty()
	}
	statuses := s.visibleStatuses(e)
	s.reconcileStatusVisuals(e, statuses)
	icons := make([]event.StatusIconView, 0, len(statuses))
	for _, st := range statuses {
		icons = append(icons, st.icon)
	}
	s.emit(event.EntityStatusSnapshot{Who: e.ID, Icons: icons})
	if e.Kind == domain.KindPlayer && e.Player != nil {
		s.emitTo(e.ID, s.buffSnapshot(e))
	}
	// 玩家本人由 Buff 快照显示特效；怪物没有本人快照，必须广播世界特效。
	if e.Kind == domain.KindMonster || e.Kind == domain.KindPet || (e.Kind == domain.KindPlayer && e.Player != nil) {
		for _, effect := range statusWorldEffects(statuses) {
			s.emitExcept(effect, e.ID)
		}
	}
}

// sendEntityStatusEffects 紧跟怪物或远端玩家的出场包补齐当前世界特效。
// 怪物出场包的状态尾只含图标；远端玩家也收不到本人的 Buff 快照。
func (s *Scene) sendEntityStatusEffects(to domain.EntityID, e *entity.Entity) {
	if e == nil || (e.Kind != domain.KindMonster && e.Kind != domain.KindPet && (e.Kind != domain.KindPlayer || e.Player == nil)) || to == e.ID {
		return
	}
	statuses := s.visibleStatuses(e)
	for _, effect := range statusWorldEffects(statuses) {
		s.emitTo(to, effect)
	}
	if e.PublishedStatusEffects == nil {
		e.PublishedStatusEffects = statusEffectsByID(statuses)
	}
}

// emitRemovedEntityStatusEffects 负责提前移除。状态集合的删除 API 返回状态号，
// 所以从数据库加载的静态表里确定性选取最低等级定义，恢复客户端使用的视觉键；同素材的其他状态仍在时只能刷新，不能清掉它们。
func (s *Scene) emitRemovedEntityStatusEffects(e *entity.Entity, ids []domain.StatusID) {
	if e == nil || (e.Kind != domain.KindMonster && e.Kind != domain.KindPet && (e.Kind != domain.KindPlayer || e.Player == nil)) || len(ids) == 0 {
		return
	}
	remaining := make(map[string]event.EntityStatusEffect)
	for _, effect := range statusWorldEffects(s.visibleStatuses(e)) {
		remaining[effect.Effect] = effect
	}
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if s.hasStatusVisual(id) {
			// The following full snapshot reconciles only actually published keys.
			// A guessed zero-duration key would create a brief effect on old clients.
			continue
		}
		def, ok := s.statusDefByID(id)
		if !ok || def.Hidden || def.Name == "" {
			continue
		}
		name, _, position := s.statusVisual(def)
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		if effect, ok := remaining[name]; ok {
			s.emitExcept(effect, e.ID)
			continue
		}
		if s.configuredStatusKeyWasPublished(e, name) {
			// The next full snapshot owns this shared key's one final clear.
			continue
		}
		s.emitExcept(event.EntityStatusEffect{
			Who: e.ID, Effect: name,
			PositionMode: position, DurationMS: 0,
		}, e.ID)
	}
}

func (s *Scene) statusDefByID(id domain.StatusID) (domain.StatusDef, bool) {
	var picked domain.StatusDef
	var pickedLevel int32
	found := false
	for key, def := range s.statuses {
		if key.ID != id || (found && key.Level >= pickedLevel) {
			continue
		}
		picked, pickedLevel, found = def, key.Level, true
	}
	return picked, found
}

// tickStatus 处理一次周期触发。
//
// attr38 与 attr446 修改当前生命，attr447 修改当前法力。Mode=1 按对应
// 上限取百分比；此前把“每两秒减少生命上限6%”错算成固定 6 点。
func (s *Scene) tickStatus(e *entity.Entity, a domain.Active) {
	s.expireHarmfulSuppression(e)
	if e == nil || a.Def.Harmful() && e.Status.SuppressesHarmfulAt(s.tick) {
		return
	}
	var hpDelta, mpDelta int64
	hpDelta -= int64(a.PeriodicDamageTickExtra)
	for _, af := range a.Def.Affixes {
		if af.Attr == domain.AttrHPRegen {
			hpDelta += int64(resourceDelta(af.Value, af.Mode, e.MaxHP))
		}
	}
	for _, change := range a.Def.HPChanges {
		hpDelta += int64(resourceDelta(change.Value, change.Mode, e.MaxHP))
	}
	for _, change := range a.Def.MPChanges {
		mpDelta += int64(resourceDelta(change.Value, change.Mode, e.MaxMP))
	}
	if a.Def.ManaToHealthRate > 0 && int64(e.MP)+mpDelta > 0 && int64(e.HP)+hpDelta < int64(e.MaxHP) {
		mpDelta--
		hpDelta += int64(a.Def.ManaToHealthRate)
	}
	if (hpDelta == 0 && mpDelta == 0) || !e.Alive() {
		return
	}
	src := e
	if o, ok := s.entities[a.Source]; ok {
		src = o
	}
	if hpDelta > 0 {
		s.casterSetCopyHeal(src, e, clampI32(hpDelta), domain.SkillID(a.Def.ID))
		s.emit(combat.Restore(src, e, clampI32(hpDelta), int32(a.Def.ID)))
	} else if hpDelta < 0 {
		// 掉血。走一条独立的路径而不是 combat.Strike ——
		// 周期伤害不判命中、不吃暴击、不触发“挨打还手”。
		if gmGodEnabled(e) {
			hpDelta = 0
		} else {
			dmg := s.mitigateSecondaryStatusDamage(e, false, clampI32(-hpDelta))
			e.HP -= dmg
			ev := event.DamageDealt{Origin: domain.DamageDOTOrigin, Src: a.Source, Dst: e.ID, Amount: dmg, DstHP: e.HP}
			if e.HP <= 0 {
				e.HP = 0
				ev.DstHP = 0
				ev.Flag |= event.DamageFatal
			}
			s.emit(ev)
			s.afterCastDamage(ev, false)
			if ev.Amount > 0 {
				s.beginEndgameTrial()
			}
			if ev.Amount > 0 && e.Status != nil {
				s.statusesRemoved(e, e.Status.RemoveEffectiveWhere(func(d domain.StatusDef) bool {
					return d.BreakOnDamage
				}))
			}
			if ev.Flag.Has(event.DamageFatal) {
				s.emitAttributesIfPlayer(e)
				s.onDeath(e, a.Source)
				return
			}
		}
	}
	if mpDelta != 0 {
		e.MP = clampI32(int64(e.MP) + mpDelta)
		if e.MP < 0 {
			e.MP = 0
		}
		if e.MP > e.MaxMP {
			e.MP = e.MaxMP
		}
	}
	s.emitAttributesIfPlayer(e)
}

func gmGodEnabled(e *entity.Entity) bool {
	return e != nil && e.Player != nil && e.Player.GMGod
}

func gmOneShotEnabled(e *entity.Entity) bool {
	return e != nil && e.Player != nil && e.Player.GMOneShot
}

func resourceDelta(value, mode, max int32) int32 {
	if mode == domain.ModePercent {
		return domain.SaturateInt32(int64(max) * int64(value) / 100)
	}
	return value
}

// refreshEntityStats 重算某个实体的属性(含装备与状态)。
//
// 玩家走完整管线; 怪物没有装备, 只把状态叠在模板属性上。
func (s *Scene) refreshEntityStats(e *entity.Entity) {
	if e.Kind == domain.KindPlayer && e.Player != nil {
		s.refreshStats(e)
		s.applyStatusAttributeLimits(e)
		return
	}
	if e.Monster == nil || s.defs == nil {
		return
	}
	def, ok := s.defs.Def(e.Monster.TypeID)
	if !ok {
		return
	}
	// 从模板重新起算；固定 buff 与百分比 buff 分桶后一次性相加，禁止复利。
	if s.challengeMonsterScene() {
		def = domain.ChallengeMonsterBehavior(def)
	}
	base := def.Stats
	if s.rift != nil {
		if scaled, exists := s.rift.baseStats[e.ID]; exists {
			base = scaled
		}
	}
	e.Stats = domain.ApplyMonsterStatusStats(base, e.Status)
	if s.challengeMonsterScene() && base.MoveSpeed <= 0 {
		e.Stats.MoveSpeed = 0 // Native immobility is not removed by speed buffs.
	}
	applyStatusAttackCap(e)
}

func (s *Scene) applyStatusAttributeLimits(e *entity.Entity) {
	if e == nil || e.Status == nil {
		return
	}
	applyStatusAttackCap(e)
	floor := e.Status.MinimumMaxHP()
	if floor <= e.MaxHP {
		return
	}
	delta := floor - e.MaxHP
	e.MaxHP, e.Stats.MaxHP = floor, floor
	e.HP += delta
	if e.HP > e.MaxHP {
		e.HP = e.MaxHP
	}
}

func applyStatusAttackCap(e *entity.Entity) {
	if e == nil || e.Status == nil {
		return
	}
	cap, ok := e.Status.MaximumAttack()
	if !ok {
		return
	}
	if e.Stats.MinAtk > cap {
		e.Stats.MinAtk = cap
	}
	if e.Stats.MaxAtk > cap {
		e.Stats.MaxAtk = cap
	}
}

// canAct reports general action admission. Movement-only locks are checked by
// canMove, leaving attacks and casts available while rooted.
func canAct(e *entity.Entity) bool {
	return (e.Player == nil || e.Player.RideAnchor == 0) && e.Status.Controlled() != domain.ControlStun
}

func canMove(e *entity.Entity) bool {
	return (e.Player == nil || e.Player.RideAnchor == 0) && !e.Status.BlocksMovement()
}

func canDealDamage(e *entity.Entity) bool {
	return canAct(e) && !e.Status.CannotDealDamage()
}

// canCast 报告实体现在能不能放技能。昏迷不能, 封印也不能。
func canCast(e *entity.Entity) bool {
	return (e.Player == nil || e.Player.RideAnchor == 0) && e.Status.Controlled() == domain.ControlNone
}
