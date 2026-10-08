package scene

import (
	"math"
	"sort"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
)

// 技能释放。
//
// 与普通攻击一样, **准入判断在收到命令时做, 效果在 stepCombat 里统一结算** ——
// 同一帧内谁的包先到不影响结果。
//
// ⚠️ 只放得出**有效果的**技能。直接伤害/治疗与已闭合的状态均可成为效果；
// 被动和缺少结果参数的触发仍直接拒绝，避免白扣法力。

// onUseSkill 处理释放技能。
func (s *Scene) onUseSkill(cmd UseSkill) {
	caster, ok := s.players[cmd.ID]
	if !ok || !caster.Alive() || s.entityMapLoading(caster) {
		return
	}
	if caster.Player != nil && caster.Player.Stall != nil {
		s.stallNotice(caster, "摆摊中不能使用技能，请先结束摊位")
		return
	}
	reject := func(r event.RejectReason) {
		s.emitTo(caster.ID, event.Rejected{Who: caster.ID, Cmd: "UseSkill", Reason: r})
	}
	if s.skills == nil {
		reject(event.RejectUnknown)
		return
	}
	if caster.Player.Riding {
		reject(event.RejectRiding)
		return
	}
	ch := caster.Player.Char
	lv, grantedByTreasure := s.skillLevelForUse(caster, cmd.Skill)
	if lv == 0 {
		reject(event.RejectSkillNotLearned)
		return
	}
	def, ok := s.skills.Get(cmd.Skill, lv)
	if !ok {
		reject(event.RejectSkillNotUsable)
		return
	}
	if rule, creation := s.nianliCreation[cmd.Skill]; creation {
		s.onNianliCreation(caster, def, rule, reject)
		return
	}
	if !def.Usable() {
		reject(event.RejectSkillNotUsable)
		return
	}
	if def.BindingArm != 0 && !grantedByTreasure {
		reject(event.RejectSkillNotLearned)
		return
	}
	// 存档里即使残留了越权技能，也不能绕过当前身份直接释放。
	if !grantedByTreasure && ch.EmploymentKnown && !ch.AllowsSkill(cmd.Skill, def.Prof) {
		reject(event.RejectWrongProf)
		return
	}
	s.applyPassiveSkillModifiers(caster, &def)
	if def.RequiresInvisible && !entityInvisible(caster) {
		reject(event.RejectRequiresInvisible)
		return
	}
	if caster.MP < def.MPCost {
		reject(event.RejectNotEnoughMP)
		return
	}
	if !canAct(caster) {
		reject(event.RejectStunned)
		return
	}
	if !canCast(caster) {
		reject(event.RejectSilenced)
		return
	}
	if def.HasDamageEffect() && !canDealDamage(caster) {
		reject(event.RejectStunned)
		return
	}
	// 客户端 RequestSkill 独立于普攻节拍；技能只检查自己的冷却和施法准备态。
	// 不能用上一刀的 nextAt 拒绝客户端已经正常发出的技能请求。
	if !caster.Player.SkillReadyAt(cmd.Skill, s.tick) {
		reject(event.RejectOnCooldown)
		return
	}
	if s.playerPreparingSkill(caster.ID) {
		reject(event.RejectOnCooldown)
		return
	}
	durabilityCost, durabilityReject := s.weaponDurabilityCost(caster, def)
	if durabilityReject != event.RejectUnknown {
		reject(durabilityReject)
		return
	}

	// 指向型技能只有两种中心来源：明确坐标，或当前有效目标的位置。
	// 缺两者时拒绝；不能把“无受击者”误当成可退回施法者脚下。
	if def.Area.Valid() && def.Area.Center == domain.SkillAreaCenterTarget ||
		def.GroundEffect && !skillCenteredOnCaster(def) {
		point := cmd.At
		if point == nil {
			target := s.entities[cmd.Target]
			if target == nil || !target.Alive() || !s.validTarget(caster, target, def) {
				reject(event.RejectNoTarget)
				return
			}
			position := target.Pos
			point = &position
		}
		if math.IsNaN(point.X) || math.IsNaN(point.Y) || math.IsInf(point.X, 0) || math.IsInf(point.Y, 0) ||
			def.Dist > 0 && sqDist(caster.Pos, *point) > float64(def.Dist)*float64(def.Dist) {
			reject(event.RejectTooFar)
			return
		}
	}
	targets, movementAt := s.resolveSkillTargets(caster, def, cmd.Target, cmd.At)
	if len(targets) == 0 && !(def.Kind == domain.SkillArea && def.Area.Valid()) {
		reject(event.RejectNoTarget)
		return
	}
	visualTarget, visualAim := s.skillVisualTarget(caster, def, cmd, targets)
	statusDurationPct := s.prepareSkillStatuses(caster, &def)
	releaseAtWall := time.Time{}
	if def.PrepareMS > 0 {
		// 吟唱从 phase=0 实际广播时开始，不能用 socket 收包时间。
		// 否则 mailbox 排队耗时会吞掉客户端本应看到的吟唱。
		releaseAtWall = s.now().Add(time.Duration(def.PrepareMS) * time.Millisecond)
	}

	// 扣法力后再排队。放在这里而不是结算时, 是因为同一帧连发两个技能
	// 应该被第二次的法力检查拦住 —— 扣在结算时就会两个都放出去。
	caster.Player.StopResting()
	caster.MP -= def.MPCost
	if durabilityCost.amount > 0 {
		s.consumeSkillWeaponDurability(caster, durabilityCost, def.ID)
	}
	// 客户端有些技能会本地预扣法力，有些不会（复法术就是后者）。服务端必须
	// 在准入成功后统一回写权威 MP，不能依赖不同技能按钮各自的本地表现。
	s.emitAttributesIfPlayer(caster)
	// 受理技能不重置普攻计时，也不用吟唱时间延长普攻冷却。
	// 客户端可在吟唱表现结束前发出普攻，两条队列各自处理已受理的请求。
	skillCooldown := domain.Ticks(int(def.CooldownMS))
	caster.Player.StartSkillCooldown(def.ID, s.tick, skillCooldown)
	// phase=0 负责正式客户端的快捷栏冷却/准备态；phase=1 才创建技能特效。
	// 两段缺一不可。无吟唱技能也按同一帧的 0→1 顺序下发。
	prepareTarget, prepareAim := visualTarget, visualAim
	if def.Area.Valid() && !def.GroundEffect {
		// 非位置型群攻的鼠标点只参与服务端范围裁决。phase=0 若携带 aim，客户端
		// 会在该点播放持续/准备表现，看起来像额外的受击特效。
		prepareTarget, prepareAim = 0, nil
	}
	s.emit(event.SkillCastChanged{
		Caster: caster.ID, Skill: def.ID, Phase: event.SkillCastChant,
		ChantHoldMS: def.PrepareMS, CooldownMS: def.CooldownMS,
		Target: prepareTarget, Aim: prepareAim,
	})
	cast := pendingCast{
		src: caster.ID, def: def, targets: targets, statusDurationPct: statusDurationPct,
		cooldownMS: def.CooldownMS, movementAt: movementAt,
		visualTarget: visualTarget, visualAim: visualAim,
		releaseAtWall: releaseAtWall,
	}
	if def.PrepareMS > 0 {
		cast.windup = &castWindup{}
	}
	s.casts = append(s.casts, cast)
	s.requestNewbieTip(caster.ID, 8)
}

// skillImpactAt 从 phase=1 真正释放的时刻开始计时。弹道只在这一刻取
// 起点、落点与距离，之后不追踪；无弹道技能则对齐客户端 skillshow
// 自行消费的攻击/受击阶段延迟。
func (s *Scene) skillImpactAt(caster *entity.Entity, def domain.SkillDef,
	visualTarget domain.EntityID, visualAim *domain.Pos, releasedAt time.Time) time.Time {
	if caster == nil {
		return time.Time{}
	}
	if def.ProjectileSpeedPXPerSec <= 0 {
		if def.ImpactDelayMS <= 0 {
			return time.Time{}
		}
		return releasedAt.Add(time.Duration(def.ImpactDelayMS) * time.Millisecond)
	}
	var endpoint domain.Pos
	switch {
	case visualAim != nil:
		endpoint = *visualAim
		endpoint.MapID = s.id.MapID
	case visualTarget != 0 && visualTarget != caster.ID:
		target := s.entities[visualTarget]
		if target == nil {
			return time.Time{}
		}
		endpoint = target.Pos
	default:
		return time.Time{}
	}
	delay := projectileTravelDuration(caster.Pos, endpoint, def.ProjectileSpeedPXPerSec)
	if delay <= 0 {
		return time.Time{}
	}
	s.log.Debug("技能弹道排队", "char", caster.Name, "skill", def.ID,
		"speed_px_s", def.ProjectileSpeedPXPerSec, "travel_ms", delay.Milliseconds())
	return releasedAt.Add(delay)
}

func (s *Scene) playerPreparingSkill(id domain.EntityID) bool {
	for _, cast := range s.casts {
		if cast.src == id && !cast.released && !cast.releaseAtWall.IsZero() &&
			(cast.windup == nil || !cast.windup.finished) {
			return true
		}
	}
	return false
}

// skillVisualTarget 把客户端原始选中目标与场景已经闭合的范围中心投影到
// 0x800e。表现目标不能取“排序后的第一个受击者”：那会让同一次群攻因附近
// 怪物实体号不同而把特效中心随机挂到另一只怪身上。
func (s *Scene) skillVisualTarget(caster *entity.Entity, def domain.SkillDef, cmd UseSkill, targets []domain.EntityID) (domain.EntityID, *domain.Pos) {
	target := cmd.Target
	var aim *domain.Pos
	setAim := func(p domain.Pos) {
		p.MapID = s.id.MapID
		aim = &p
	}
	if cmd.At != nil {
		setAim(*cmd.At)
	}
	if def.Area.Valid() {
		switch def.Area.Center {
		case domain.SkillAreaCenterCaster:
			if def.Area.Shape == domain.SkillAreaCircle {
				target = caster.ID
				setAim(caster.Pos)
			} else if aim == nil {
				if anchor := s.entities[cmd.Target]; anchor != nil {
					setAim(anchor.Pos)
				} else if cmd.Target == 0 {
					dx, dy := skillFacingUnit(caster.Dir)
					reach := skillAreaSearchRadius(def.Area)
					setAim(domain.Pos{X: caster.Pos.X + dx*reach, Y: caster.Pos.Y + dy*reach})
				}
			}
		case domain.SkillAreaCenterTarget:
			if aim == nil {
				if anchor := s.entities[cmd.Target]; anchor != nil {
					setAim(anchor.Pos)
				}
			}
		}
	}
	if target == 0 && aim == nil && len(targets) > 0 {
		target = targets[0]
	}
	if def.GroundEffect && aim == nil {
		if skillCenteredOnCaster(def) {
			target = caster.ID
			setAim(caster.Pos)
		} else if anchor := s.entities[target]; anchor != nil {
			setAim(anchor.Pos)
		}
	}
	return target, aim
}

func skillCenteredOnCaster(def domain.SkillDef) bool {
	if def.Area.Valid() {
		return def.Area.Center == domain.SkillAreaCenterCaster
	}
	return def.TargetSelf && !def.TargetEnemy && !def.TargetTeam
}

// resolveSkillTargets 先处理位移技能的特殊目标语义，再回到普通伤害/状态目标裁决。
// 幻影必须由 UseSkillAt 给出落点；阴影跳跃只允许活着的敌怪或同队玩家。
// 陷阱机制也在这里解析，因为陷阱不是普通伤害目标。
func (s *Scene) resolveSkillTargets(caster *entity.Entity, def domain.SkillDef, target domain.EntityID, at *domain.Pos) ([]domain.EntityID, *domain.Pos) {
	effect, special := specialSkillEffect(def)
	if !special {
		return s.resolveTargets(caster, def, target, at), nil
	}
	switch effect.Kind {
	case domain.EffectTeleportPoint:
		if at == nil {
			return nil, nil
		}
		to := domain.Pos{MapID: s.id.MapID, X: at.X, Y: at.Y}
		if def.Dist > 0 && sqDist(caster.Pos, to) > float64(def.Dist)*float64(def.Dist) {
			return nil, nil
		}
		if s.walkable != nil && !s.walkable(to) {
			return nil, nil
		}
		return []domain.EntityID{caster.ID}, &to
	case domain.EffectTeleportTarget:
		anchor, ok := s.entities[target]
		if !ok || !anchor.Alive() || anchor.ID == caster.ID {
			return nil, nil
		}
		valid := anchor.Kind == domain.KindMonster && anchor.Monster != nil && anchor.Monster.Kind.Hostile()
		if anchor.Kind == domain.KindPlayer {
			valid = sameParty(caster, anchor)
		}
		if !valid || def.Dist > 0 && sqDist(caster.Pos, anchor.Pos) > float64(def.Dist)*float64(def.Dist) {
			return nil, nil
		}
		to := anchor.Pos
		// 目标实体的当前位置就是场景权威落点。静态碰撞图在旧地图边界会把
		// 合法刷怪坐标误判成不可走，不能因此让“跳到目标旁”稳定失败。
		return []domain.EntityID{anchor.ID}, &to
	case domain.EffectPlaceTrap:
		to := caster.Pos
		if at != nil {
			to.X, to.Y = at.X, at.Y
		}
		if def.Dist > 0 && sqDist(caster.Pos, to) > float64(def.Dist)*float64(def.Dist) {
			return nil, nil
		}
		// 陷阱不移动任何实体。角色能站立的位置并不保证静态碰撞图把同一像素
		// 判为可走（旧地图边界已有这种实测偏差），这里以 75 像素施法距离
		// 作为权威约束，不能把脚下的合法放置误拒为 NoTarget。
		return []domain.EntityID{caster.ID}, &to
	case domain.EffectDetectTrap:
		return []domain.EntityID{caster.ID}, nil
	case domain.EffectDisarmTrap:
		trap := s.traps[target]
		if trap == nil || trap.Trap == nil || trap.Trap.Owner == caster.ID ||
			!trap.Trap.VisibleToPlayer(caster.ID) ||
			def.Dist > 0 && sqDist(caster.Pos, trap.Pos) > float64(def.Dist)*float64(def.Dist) {
			return nil, nil
		}
		return []domain.EntityID{trap.ID}, nil
	case domain.EffectRevealInvisible:
		return []domain.EntityID{caster.ID}, nil
	default:
		return nil, nil
	}
}

func specialSkillEffect(def domain.SkillDef) (domain.SkillEffect, bool) {
	for _, effect := range def.DirectEffects() {
		switch effect.Kind {
		case domain.EffectTeleportPoint, domain.EffectTeleportTarget,
			domain.EffectPlaceTrap, domain.EffectDetectTrap, domain.EffectDisarmTrap,
			domain.EffectRevealInvisible:
			return effect, true
		}
	}
	return domain.SkillEffect{}, false
}

// applyPassiveSkillModifiers 把当前已学的被动技能汇总到本次施法定义的副本。
// SkillTable 是全服共享只读表，修改切片前必须克隆，不能污染其他角色。
func (s *Scene) applyPassiveSkillModifiers(caster *entity.Entity, def *domain.SkillDef) {
	if s == nil || s.skills == nil || caster == nil || caster.Player == nil ||
		caster.Player.Char == nil || def == nil {
		return
	}
	var damageBonus, castReduction int32
	var stealthDuration, stealthMove, stealthDamage, preserveInvisible int32
	var durabilityReduction, statusPhysicalDamageTaken int32
	effectBonus := map[domain.EffectKind]int32{}
	var triggerStatuses []domain.StatusApplication
	var statusAffixes []domain.PassiveStatusAffix
	var triggerEffects []domain.SkillEffect
	for passiveID, level := range caster.Player.Char.Skills {
		passive, ok := s.skills.Get(passiveID, level)
		if !ok || passive.Kind != domain.SkillPassive ||
			!caster.Player.Char.AllowsSkill(passive.ID, passive.Prof) {
			continue
		}
		for _, modifier := range passive.PassiveModifiers {
			switch modifier.Kind {
			case domain.PassiveSkillDamagePct:
				if modifier.TargetSkill == def.ID {
					damageBonus += modifier.Value
				}
			case domain.PassiveEffectSourceAttackPct:
				if modifier.TargetSkill == def.ID {
					effectBonus[modifier.EffectKind] += modifier.Value
				}
			case domain.PassiveCastTimeReductionPct:
				castReduction += modifier.Value
			case domain.PassiveStealthDurationSec:
				if modifier.TargetSkill == def.ID {
					stealthDuration += modifier.Value
				}
			case domain.PassiveStealthMoveSpeedPct:
				if modifier.TargetSkill == def.ID {
					stealthMove += modifier.Value
				}
			case domain.PassiveStealthDamagePct:
				if modifier.TargetSkill == def.ID {
					stealthDamage += modifier.Value
				}
			case domain.PassivePreserveInvisibleChance:
				if modifier.TargetSkill == def.ID {
					preserveInvisible += modifier.Value * 100
				}
			case domain.PassiveSkillDurabilityReductionPct:
				if modifier.TargetSkill == def.ID {
					durabilityReduction += modifier.Value
				}
			case domain.PassiveStatusPhysicalDamageTakenPct:
				if modifier.TargetSkill == def.ID {
					statusPhysicalDamageTaken += modifier.Value
				}
			}
		}
		for _, trigger := range passive.PassiveTriggers {
			if trigger.TargetSkill != def.ID {
				continue
			}
			triggerStatuses = append(triggerStatuses, domain.StatusApplication{
				ID: trigger.Status, Level: trigger.StatusLevel,
				ChanceBP: trigger.ChanceBP, DurationSec: trigger.DurationSec,
			})
		}
		for _, modifier := range passive.PassiveStatusAffixes {
			if modifier.TargetSkill == def.ID {
				statusAffixes = append(statusAffixes, modifier)
			}
		}
		for _, trigger := range passive.PassiveTriggerEffects {
			if trigger.TargetSkill == def.ID {
				triggerEffects = append(triggerEffects, trigger.Effect)
			}
		}
	}
	if preserveInvisible > 10000 {
		preserveInvisible = 10000
	}
	def.PreserveInvisibleChanceBP = preserveInvisible
	if durabilityReduction > 100 {
		durabilityReduction = 100
	}
	def.WeaponDurabilityReductionPct = durabilityReduction
	statusesCloned := false
	if len(triggerStatuses) > 0 || len(statusAffixes) > 0 || statusPhysicalDamageTaken > 0 {
		def.Statuses = append([]domain.StatusApplication(nil), def.Statuses...)
		statusesCloned = true
		def.Statuses = append(def.Statuses, triggerStatuses...)
	}
	for _, modifier := range statusAffixes {
		for i := range def.Statuses {
			app := &def.Statuses[i]
			if app.ID != modifier.Status {
				continue
			}
			app.ExtraAffixes = append([]domain.Affix(nil), app.ExtraAffixes...)
			adjustStatusApplicationAffix(app, modifier.Affix.Attr, modifier.Affix.Value)
		}
	}
	if statusPhysicalDamageTaken > 0 {
		if statusPhysicalDamageTaken > 100 {
			statusPhysicalDamageTaken = 100
		}
		for i := range def.Statuses {
			def.Statuses[i].PhysicalDamageTakenPct += statusPhysicalDamageTaken
		}
	}
	if stealthDuration != 0 || stealthMove != 0 || stealthDamage != 0 {
		if !statusesCloned {
			def.Statuses = append([]domain.StatusApplication(nil), def.Statuses...)
		}
		for i := range def.Statuses {
			app := &def.Statuses[i]
			if app.ID != domain.StatusStealth {
				continue
			}
			app.ExtraAffixes = append([]domain.Affix(nil), app.ExtraAffixes...)
			app.DurationSec += stealthDuration
			app.StealthDamagePct += stealthDamage
			adjustStatusApplicationAffix(app, domain.AttrMoveSpeed, stealthMove)
		}
	}
	if castReduction > 100 {
		castReduction = 100
	}
	if castReduction > 0 {
		def.PrepareMS -= def.PrepareMS * castReduction / 100
	}
	if len(triggerEffects) > 0 {
		def.Effects = append(append([]domain.SkillEffect(nil), def.DirectEffects()...), triggerEffects...)
		def.Effect = def.Effects[0]
	}
	apply := func(effect *domain.SkillEffect) {
		if effect == nil {
			return
		}
		if effect.Kind == domain.EffectDamage && damageBonus > 0 {
			effect.DamagePct = boostedDamagePct(effect.DamagePct, damageBonus)
		}
		if bonus := effectBonus[effect.Kind]; bonus > 0 {
			effect.SourceAttackPct += bonus
		}
	}
	if len(def.Effects) > 0 {
		def.Effects = append([]domain.SkillEffect(nil), def.Effects...)
		for i := range def.Effects {
			apply(&def.Effects[i])
		}
		def.Effect = def.Effects[0]
		return
	}
	apply(&def.Effect)
}

func adjustStatusApplicationAffix(app *domain.StatusApplication, attr, delta int32) {
	if app == nil || delta == 0 {
		return
	}
	for i := range app.ExtraAffixes {
		if app.ExtraAffixes[i].Attr == attr && app.ExtraAffixes[i].Mode == domain.ModePercent {
			app.ExtraAffixes[i].Value += delta
			return
		}
	}
	app.ExtraAffixes = append(app.ExtraAffixes, domain.Affix{Attr: attr, Value: delta, Mode: domain.ModePercent})
}

type skillWeaponDurabilityCost struct {
	slot   domain.EquipSlot
	amount int32
}

// weaponDurabilityCost 在排队前闭合裂空之枪的第二种施法资源。百分比以武器
// 满耐久为基数；向上取整保证客户端整数耐久至少能看见一次消耗。
func (s *Scene) weaponDurabilityCost(p *entity.Entity, def domain.SkillDef) (skillWeaponDurabilityCost, event.RejectReason) {
	pct := def.WeaponDurabilityCostPct()
	if pct <= 0 {
		return skillWeaponDurabilityCost{}, event.RejectUnknown
	}
	if p == nil || p.Player == nil || p.Player.Worn == nil {
		return skillWeaponDurabilityCost{}, event.RejectNotEquippable
	}
	for _, slot := range []domain.EquipSlot{domain.SlotWeapon, domain.SlotTwoHand} {
		st := p.Player.Worn.At(slot)
		if st.Empty() {
			continue
		}
		item, ok := s.itemDef(st.Item)
		if !ok || item.Equip == nil ||
			(def.RequiredWeaponType > 0 && item.Equip.Type != def.RequiredWeaponType) {
			return skillWeaponDurabilityCost{}, event.RejectNotEquippable
		}
		if item.Equip.Durable <= 0 || !domain.EquipmentFunctional(st, item) {
			return skillWeaponDurabilityCost{}, event.RejectNotEnough
		}
		reduction := def.WeaponDurabilityReductionPct
		if reduction > 100 {
			reduction = 100
		}
		maxDurability := domain.MaxDurabilityOf(st, item)
		amount := int32((int64(maxDurability)*int64(pct)*int64(100-reduction) + 9999) / 10000)
		if amount == 0 {
			return skillWeaponDurabilityCost{slot: slot}, event.RejectUnknown
		}
		if st.Durability < amount {
			return skillWeaponDurabilityCost{}, event.RejectNotEnough
		}
		return skillWeaponDurabilityCost{slot: slot, amount: amount}, event.RejectUnknown
	}
	return skillWeaponDurabilityCost{}, event.RejectNotEquippable
}

// equippedTreasureSkill 返回 11 号法宝槽当前临时授予的技能。
func (s *Scene) equippedTreasureSkill(p *entity.Entity) *domain.EquipmentSkill {
	if p == nil || p.Player == nil || p.Player.Worn == nil {
		return nil
	}
	st := p.Player.Worn.At(domain.SlotTreasure)
	if st.Empty() {
		return nil
	}
	def, ok := s.itemDef(st.Item)
	if !ok || def.Equip == nil || def.Equip.Skill == nil || !domain.EquipmentFunctional(st, def) {
		return nil
	}
	return def.Equip.Skill
}

// skillLevelForUse 返回客户端当前可见且服务端准许尝试的技能等级；第二个返回值
// 表示授权来自已装备法宝，用于跳过职业技能的永久学习/职业门槛。
func (s *Scene) skillLevelForUse(p *entity.Entity, id domain.SkillID) (int32, bool) {
	if skill := s.equippedTreasureSkill(p); skill != nil && skill.ID == id {
		return 1, true
	}
	if p == nil || p.Player == nil || p.Player.Char == nil {
		return 0, false
	}
	return p.Player.Char.Skills.LevelOf(id), false
}

// resolveTargets 算出这次技能打到谁身上。
//
// 单体只打指定目标；普通范围技能使用 PostgreSQL 已闭合的中心规则，以及
// ov_skilldesc 的矩形/圆形/扇形字段。陷阱、位移、侦测由 resolveSkillTargets
// 里的独立场景机制处理，不会进入这里。
func (s *Scene) resolveTargets(caster *entity.Entity, def domain.SkillDef, target domain.EntityID, at *domain.Pos) []domain.EntityID {
	canResurrect := def.HasResurrectEffect()
	if def.Kind == domain.SkillArea && def.Area.Valid() {
		return s.resolveAreaTargets(caster, def, target, at)
	}
	// 对自己放的(治疗/增益): 目标就是自己
	if def.TargetSelf && !def.TargetEnemy && target == 0 && at == nil {
		return []domain.EntityID{caster.ID}
	}

	center := caster.Pos
	// UseSkillAt 的坐标只改变范围技能的中心；单体技能仍必须由目标实体裁决。
	// 这样客户端不能靠伪造坐标绕过“目标存在、活着、可攻击”的检查。
	if at != nil && def.Kind == domain.SkillArea {
		center.X, center.Y = at.X, at.Y
		if def.Dist > 0 && sqDist(caster.Pos, center) > float64(def.Dist)*float64(def.Dist) {
			return nil
		}
	} else if target != 0 {
		t, ok := s.entities[target]
		if !ok || !t.Alive() && !(canResurrect && t.Kind == domain.KindPlayer) {
			return nil
		}
		if t.Alive() && def.ResurrectOnly() {
			return nil
		}
		if def.Dist > 0 && sqDist(caster.Pos, t.Pos) > float64(def.Dist)*float64(def.Dist) {
			return nil // 够不着
		}
		center = t.Pos
		if def.Kind == domain.SkillSingle {
			if !s.validTarget(caster, t, def) {
				return nil
			}
			return []domain.EntityID{target}
		}
	}

	return nil
}

// resolveAreaTargets 按技能真实几何选中一批实体。以自身为中心的圆形技能会
// 忽略客户端附带的当前选中目标；这正是环形爆发、群疗术等此前失效的根因。
func (s *Scene) resolveAreaTargets(caster *entity.Entity, def domain.SkillDef, target domain.EntityID, at *domain.Pos) []domain.EntityID {
	area := def.Area
	center := caster.Pos
	var aimX, aimY float64

	switch area.Center {
	case domain.SkillAreaCenterCaster:
		if area.Shape == domain.SkillAreaRectangle || area.Shape == domain.SkillAreaCone {
			aim := at
			if aim == nil {
				if target == 0 {
					aimX, aimY = skillFacingUnit(caster.Dir)
				} else {
					t, ok := s.entities[target]
					if !ok || !t.Alive() || !s.validTarget(caster, t, def) {
						return nil
					}
					p := t.Pos
					aim = &p
				}
			}
			if aim != nil {
				aimX, aimY = aim.X-caster.Pos.X, aim.Y-caster.Pos.Y
				length := math.Hypot(aimX, aimY)
				if length <= 0 {
					return nil
				}
				aimX, aimY = aimX/length, aimY/length
			}
		}
	case domain.SkillAreaCenterTarget:
		if at != nil {
			center.X, center.Y = at.X, at.Y
		} else if target != 0 {
			t, ok := s.entities[target]
			if !ok || !t.Alive() || !s.validTarget(caster, t, def) {
				return nil
			}
			center = t.Pos
		} else {
			return nil
		}
		if def.Dist > 0 && sqDist(caster.Pos, center) > float64(def.Dist)*float64(def.Dist) {
			return nil
		}
	default:
		return nil
	}

	out := make([]domain.EntityID, 0, 8)
	s.aoi.AroundRadius(center, skillAreaSearchRadius(area), func(o *entity.Entity) {
		if !o.Alive() || !s.validTarget(caster, o, def) ||
			!skillAreaContains(area, center, aimX, aimY, o.Pos) {
			return
		}
		out = append(out, o.ID)
	})
	// AOI 格内实体用 map 保存；排序保证同一帧的命中、状态概率与日志顺序可复现。
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// skillFacingUnit 使用客户端存储朝向：0..7 = 东、东南、南、西南、西、西北、北、东北。
func skillFacingUnit(dir uint8) (float64, float64) {
	const diagonal = 0.7071067811865476
	switch dir & 7 {
	case 0:
		return 1, 0
	case 1:
		return diagonal, diagonal
	case 2:
		return 0, 1
	case 3:
		return -diagonal, diagonal
	case 4:
		return -1, 0
	case 5:
		return -diagonal, -diagonal
	case 6:
		return 0, -1
	default:
		return diagonal, -diagonal
	}
}

func skillAreaSearchRadius(area domain.SkillAreaTargeting) float64 {
	switch area.Shape {
	case domain.SkillAreaRectangle:
		return math.Hypot(float64(area.Length), float64(area.Width)/2)
	case domain.SkillAreaCircle, domain.SkillAreaCone:
		return float64(area.Radius)
	default:
		return 0
	}
}

func skillAreaContains(area domain.SkillAreaTargeting, center domain.Pos, aimX, aimY float64, p domain.Pos) bool {
	dx, dy := p.X-center.X, p.Y-center.Y
	switch area.Shape {
	case domain.SkillAreaCircle:
		r := float64(area.Radius)
		return dx*dx+dy*dy <= r*r
	case domain.SkillAreaCone:
		r := float64(area.Radius)
		d2 := dx*dx + dy*dy
		if d2 > r*r {
			return false
		}
		forward := dx*aimX + dy*aimY
		if forward < 0 {
			return false
		}
		cosHalf := math.Cos(float64(area.Angle) * math.Pi / 360)
		return forward >= math.Sqrt(d2)*cosHalf
	case domain.SkillAreaRectangle:
		forward := dx*aimX + dy*aimY
		if forward < 0 || forward > float64(area.Length) {
			return false
		}
		lateral := math.Abs(-dx*aimY + dy*aimX)
		return lateral <= float64(area.Width)/2
	default:
		return false
	}
}

// validTarget 判断某个实体能不能被这个技能选中。
//
// 这是**防止治疗技能奶怪、伤害技能打自己**的地方。客户端说了不算。
func (s *Scene) validTarget(caster, t *entity.Entity, def domain.SkillDef) bool {
	if s.entityMapLoading(caster) || s.entityMapLoading(t) {
		return false
	}
	if t.ID == caster.ID {
		return def.TargetSelf
	}
	switch t.Kind {
	case domain.KindMonster:
		// 摆设与采集物不该被技能选中
		if t.Monster != nil && !t.Monster.Kind.Hostile() {
			return false
		}
		return def.TargetEnemy
	case domain.KindPlayer:
		if !t.Alive() && !def.HasResurrectEffect() || t.Alive() && def.ResurrectOnly() {
			return false
		}
		// **只能给队友。** 以前这里是 `def.TargetTeam || ...`, 等于"任何玩家",
		// 于是药师能给全服路人加血 —— 那不叫组队技能, 叫公共设施。
		// target_team=1 的 217 行技能全部属于药师(prof=4), 队伍是它们唯一的用武之地。
		if !sameParty(caster, t) {
			return false
		}
		// 伤害技能不许落在队友身上; PVP 还没做, 队友之间更不该有伤害
		return def.TargetTeam && !def.HasDamageEffect()
	}
	return false
}

// pendingCast 是本帧收到、等着结算的一次技能释放。
type pendingCast struct {
	src               domain.EntityID
	def               domain.SkillDef
	targets           []domain.EntityID
	statusDurationPct int32
	cooldownMS        int32
	movementAt        *domain.Pos
	visualTarget      domain.EntityID
	visualAim         *domain.Pos
	releaseAtWall     time.Time
	resolveAtWall     time.Time
	released          bool
	windup            *castWindup
	extras            []domain.MonsterSkillExtra
}

type pendingSkillTeleport struct {
	who domain.EntityID
	to  domain.Pos
}

// stepCasts 结算本帧到期的技能。技能和普攻分别排队，主帧内仍固定先结算
// 技能再结算普攻，保留同帧事件的确定顺序。
func (s *Scene) stepCasts() {
	s.stepCastsAt(s.now(), false)
}

// stepCastsAt 同时服务 100ms 主帧与主帧之间的精确吟唱结束/命中相位。
// timedOnly=true 时只处理到期的壁钟时刻，不能顺手提前执行普通瞬发技能。
func (s *Scene) stepCastsAt(now time.Time, timedOnly bool) {
	if len(s.casts) == 0 {
		return
	}
	originalLen := len(s.casts)
	waiting := s.casts[:0]
	teleports := make([]pendingSkillTeleport, 0)
	for _, c := range s.casts {
		if c.windup != nil && c.windup.interrupted {
			continue
		}
		src, ok := s.entities[c.src]
		if !ok || !src.Alive() || src.Kind == domain.KindPet {
			continue
		}
		if !c.released {
			if !c.releaseAtWall.IsZero() && now.Before(c.releaseAtWall) {
				waiting = append(waiting, c)
				continue
			}
			if timedOnly && c.releaseAtWall.IsZero() {
				waiting = append(waiting, c)
				continue
			}
		}

		// 0x1013 在点击/选点完成后立即到达。phase=0 已在受理时启动吟唱；
		// 只有等 prepare 到期后的 phase=1 才让客户端进入 SkillCaster.Cast/PlayFire。
		releaseTarget, releaseAim := c.visualTarget, c.visualAim
		if c.def.Area.Valid() && !c.def.GroundEffect {
			// 对实体受击型范围表现，phase=1 的 aim 会同时成为 phrase_hit 的落点。若继续传鼠标
			// 坐标，客户端会在空地点额外播放一次受击特效。范围技能改用一个
			// 真实目标承载首个受击表现，其余命中目标由 0x8045 分别补齐。
			releaseTarget, releaseAim = 0, nil
			for _, target := range c.targets {
				if target == c.visualTarget {
					releaseTarget = target
					break
				}
			}
			if releaseTarget == 0 && len(c.targets) > 0 {
				releaseTarget = c.targets[0]
			}
		}
		release := event.SkillCastChanged{
			Caster: src.ID, Skill: c.def.ID, Phase: event.SkillCastRelease,
			ChantHoldMS: c.def.PrepareMS, CooldownMS: 0,
			Target: releaseTarget, Aim: releaseAim,
		}
		releasedNow := false
		if !c.released {
			if c.windup != nil {
				c.windup.finished = true
			}
			s.emit(release)
			c.released = true
			c.releaseAtWall = time.Time{}
			c.resolveAtWall = s.skillImpactAt(src, c.def, releaseTarget, releaseAim, now)
			releasedNow = true
		}
		if !c.resolveAtWall.IsZero() && now.Before(c.resolveAtWall) {
			waiting = append(waiting, c)
			continue
		}
		if !c.resolveAtWall.IsZero() {
			s.log.Debug("技能命中时刻到达", "char", src.Name, "skill", c.def.ID,
				"targets", len(c.targets))
		}
		if timedOnly && !releasedNow && c.resolveAtWall.IsZero() {
			waiting = append(waiting, c)
			continue
		}
		if effect, ok := specialSkillEffect(c.def); ok {
			switch effect.Kind {
			case domain.EffectTeleportPoint, domain.EffectTeleportTarget:
				if c.movementAt != nil && (effect.ChanceBP >= 10000 || int32(s.rng.Intn(10000)) < effect.ChanceBP) {
					teleports = append(teleports, pendingSkillTeleport{who: src.ID, to: *c.movementAt})
				}
			case domain.EffectPlaceTrap:
				if c.movementAt != nil {
					s.placeTrap(src, c.def, *c.movementAt, effect.TrapRadius, c.statusDurationPct)
				}
			case domain.EffectDetectTrap:
				s.detectTraps(src, c.def.Radius, effect.ChanceBP)
			case domain.EffectDisarmTrap:
				s.disarmTrap(src, c.targets[0], effect.ChanceBP)
			case domain.EffectRevealInvisible:
				s.revealInvisible(src, c.def, effect.RevealRadius, c.statusDurationPct)
			}
			continue
		}
		for _, tid := range c.targets {
			dst, ok := s.entities[tid]
			if !ok || !dst.Alive() && !c.def.HasResurrectEffect() || dst.Kind == domain.KindPet {
				continue // 这半帧里已经死了
			}
			if s.applySkill(src, dst, c.def, c.statusDurationPct, false) && c.def.HitEffect != "" &&
				dst.ID != releaseTarget {
				s.emit(event.SkillHitEffect{Target: dst.ID, Effect: c.def.HitEffect})
			}
		}
		if c.def.HasDamageEffect() {
			s.consumeAttackDurability(src)
		}
		s.applyMonsterSkillExtras(src, c.def, c.targets, c.extras)
	}
	for i := len(waiting); i < originalLen; i++ {
		s.casts[i] = pendingCast{}
	}
	s.casts = waiting
	for _, teleport := range teleports {
		player := s.players[teleport.who]
		if player == nil || !player.Alive() || s.walkable != nil && !s.walkable(teleport.to) {
			continue
		}
		s.onTeleport(Teleport{ID: player.ID, To: s.id, At: teleport.to, Dir: player.Dir})
	}
}

// applySkill 把一个技能的效果落到一个目标身上。
func (s *Scene) applySkill(src, dst *entity.Entity, def domain.SkillDef, statusDurationPct int32, triggeredByTrap bool) bool {
	if dst.Monster != nil && s.defs != nil {
		if monsterDef, ok := s.defs.Def(dst.Monster.TypeID); ok {
			if _, immune := monsterDef.ImmuneSkills[def.ID]; immune {
				if def.HasDamageEffect() {
					s.emit(event.DamageDealt{
						Src: src.ID, Dst: dst.ID, DstHP: dst.HP,
						SkillID: int32(def.ID), Flag: event.DamageMiss,
					})
				}
				return false
			}
		}
	}
	landed := len(def.DirectEffects()) == 0
	previousLanded := true
	targetDied := false
	startedDead := !dst.Alive()
	for _, effect := range def.DirectEffects() {
		if effect.RequiresPreviousLanded && !previousLanded {
			continue
		}
		// 光之奇迹对尸体只执行复活段；普通治疗不能在没有 0x801a 的情况下
		// 直接把 HP=0 的玩家抬活。存活目标则照常净化、治疗，复活段为空操作。
		if startedDead && effect.Kind != domain.EffectResurrect {
			continue
		}
		effectTarget := dst
		if effect.Target == domain.EffectTargetCaster {
			effectTarget = src
		}
		if targetDied && effectTarget == dst {
			previousLanded = false
			continue
		}
		effectLanded := false
		if len(effect.RemoveStatusIDs) > 0 {
			dispelLanded := effect.DispelChanceBP >= 10000 ||
				int32(s.rng.Intn(10000)) < effect.DispelChanceBP
			if dispelLanded {
				if effectTarget.Status != nil {
					s.statusesRemoved(effectTarget, effectTarget.Status.RemoveWhere(func(existing domain.StatusDef) bool {
						for _, id := range effect.RemoveStatusIDs {
							if existing.ID == id {
								return true
							}
						}
						return false
					}))
				}
				effectLanded = true
			}
		}
		if effect.ClearHarmful && effectTarget.Status != nil {
			s.statusesRemoved(effectTarget, effectTarget.Status.RemoveWhere(func(existing domain.StatusDef) bool {
				return existing.Harmful()
			}))
			effectLanded = true
		}
		switch effect.Kind {
		case domain.EffectHeal:
			amount := effect.HealAmount(effectTarget.MaxHP)
			ev := combat.Restore(src, effectTarget, amount, int32(def.ID))
			s.emit(ev)
			s.emitAttributesIfPlayer(effectTarget)
			effectLanded = true

		case domain.EffectRestoreMP:
			amount := effect.RestoreMPFlat + src.Stats.MaxAtk*effect.SourceAttackPct/100
			effectTarget.MP += amount
			if effectTarget.MP > effectTarget.MaxMP {
				effectTarget.MP = effectTarget.MaxMP
			}
			s.emitAttributesIfPlayer(effectTarget)
			effectLanded = true

		case domain.EffectRestoreHP:
			amount := src.Stats.MaxAtk * effect.SourceAttackPct / 100
			ev := combat.Restore(src, effectTarget, amount, int32(def.ID))
			s.emit(ev)
			s.emitAttributesIfPlayer(effectTarget)
			effectLanded = true

		case domain.EffectDamageMP:
			amount := src.Stats.MaxAtk * effect.SourceAttackPct / 100
			effectTarget.MP -= amount
			if effectTarget.MP < 0 {
				effectTarget.MP = 0
			}
			s.emitAttributesIfPlayer(effectTarget)
			effectLanded = true

		case domain.EffectDamageMPMax:
			amount := effectTarget.MaxMP * effect.MaxResourcePct / 100
			effectTarget.MP -= amount
			if effectTarget.MP < 0 {
				effectTarget.MP = 0
			}
			s.emitAttributesIfPlayer(effectTarget)
			effectLanded = true

		case domain.EffectTaunt:
			effectLanded = s.taunt(effectTarget, src.ID, effect.TauntPower)

		case domain.EffectResurrect:
			effectLanded = s.reviveBySkill(src, effectTarget, def, effect)

		case domain.EffectKnockback:
			if effect.ChanceBP >= 10000 || int32(s.rng.Intn(10000)) < effect.ChanceBP {
				effectLanded = s.knockbackMonster(src, effectTarget, effect.KnockbackDistance)
			}

		case domain.EffectInstantDeath:
			if effectTarget.Alive() &&
				(effect.ChanceBP >= 10000 || int32(s.rng.Intn(10000)) < effect.ChanceBP) &&
				!gmGodEnabled(effectTarget) &&
				(effectTarget.Status == nil || !effectTarget.Status.InstantDeathImmune()) {
				amount := effectTarget.HP
				effectTarget.HP = 0
				flag := event.DamageFatal
				if def.HurtType != domain.HurtPhysical && def.HurtType != domain.HurtNone {
					flag |= event.DamageMagic
				}
				dealt := event.DamageDealt{
					Src: src.ID, Dst: effectTarget.ID, Amount: amount, DstHP: 0,
					SkillID: int32(def.ID), Flag: flag,
				}
				s.emit(dealt)
				s.consumeHitDurability(effectTarget, dealt)
				s.emitAttributesIfPlayer(effectTarget)
				s.onDeath(effectTarget, src.ID)
				effectLanded, targetDied = true, true
			}

		case domain.EffectDamage:
			tr := s.hits[src.ID]
			if tr == nil {
				tr = &combat.HitTracker{}
				s.hits[src.ID] = tr
			}
			blow := combat.Blow{
				SkillID:       int32(def.ID),
				SkillPct:      effect.DamagePct,
				FlatDamage:    effect.DamageFlat,
				Trap:          triggeredByTrap,
				GuaranteedHit: effect.GuaranteedHit,
				// 法系技能 100% 命中(docs/战斗公式.md 第一节)。
				// 物理系技能照常判命中 —— 「法杖平砍算物理, 照样 miss」。
				Magic: def.HurtType != domain.HurtPhysical && def.HurtType != domain.HurtNone,
			}
			ev, rawDamage, fullMagicReflect := s.resolveStrikeStatuses(src, dst, tr, blow)
			s.emit(ev)
			if ev.Flag.Has(event.DamageMiss) {
				if counter, ok := s.passiveCounterPending(dst, src); ok {
					s.attacks = append(s.attacks, counter)
				}
			}
			s.afterStrikeStatuses(src, dst, ev, rawDamage, fullMagicReflect)
			s.consumeHitDurability(dst, ev)
			s.emitAttributesIfPlayer(dst)
			s.aggro(dst, src.ID)
			effectLanded = !ev.Flag.Has(event.DamageMiss)
			if ev.Flag.Has(event.DamageFatal) {
				s.onDeath(dst, src.ID)
				targetDied = true
			}
		}
		previousLanded = effectLanded
		landed = landed || effectLanded
	}
	if targetDied {
		return landed
	}
	if !landed {
		return false
	}
	for _, app := range def.Statuses {
		chanceBP := app.ChanceBP
		if chanceBP <= 0 {
			chanceBP = 10000
		}
		if chanceBP < 10000 && int32(s.rng.Intn(10000)) >= chanceBP {
			continue
		}
		statusDef, ok := s.statuses.Get(app.ID, app.Level)
		if !ok {
			continue
		}
		if app.DurationSec > 0 {
			statusDef.DurationSec = app.DurationSec
		}
		statusDef.Affixes = overrideStatusAffixes(statusDef.Affixes, app.ExtraAffixes)
		statusDef.StealthDamagePct = app.StealthDamagePct
		statusDef.PhysicalDamageTakenPct += app.PhysicalDamageTakenPct
		if statusDef.PhysicalDamageTakenPct > 100 {
			statusDef.PhysicalDamageTakenPct = 100
		}
		if app.Description != "" {
			statusDef.Desc = app.Description
		}
		statusDef.InstantDeathImmune = app.InstantDeathImmune
		statusDef.BlockHarmful = statusDef.BlockHarmful || app.BlockHarmful
		if app.Control != domain.ControlNone {
			statusDef.Control = app.Control
		}
		if statusDurationPct > 0 {
			statusDef.DurationSec += statusDef.DurationSec * statusDurationPct / 100
		}
		if app.ID == 1032 {
			statusDef.Invisible = true
		}
		applyStatusSource(&statusDef, def.Icon, int32(def.ID))
		s.applyStatusDef(dst, statusDef, src.ID)
	}
	return true
}

// overrideStatusAffixes 让更具体的技能说明覆盖共用状态表里的同属性残值。
// 隐身术的 1032 状态表移速比技能说明整体高 5%，必须以施法来源为准；不同
// 属性仍正常并存，祝福术等已有的 ExtraAffixes 行为不受影响。
func overrideStatusAffixes(base, overrides []domain.Affix) []domain.Affix {
	out := append([]domain.Affix(nil), base...)
	for _, override := range overrides {
		kept := out[:0]
		for _, existing := range out {
			if existing.Attr != override.Attr {
				kept = append(kept, existing)
			}
		}
		out = append(kept, override)
	}
	return out
}

// knockbackMonster 沿施法者到目标的方向推动活着的敌怪。逐段检查 MASK，不能
// 为了找到一个可走终点就穿过中间障碍；距离不足时落在最后一个合法采样点。
func (s *Scene) knockbackMonster(src, target *entity.Entity, distance int32) bool {
	if src == nil || target == nil || target.Monster == nil || !target.Alive() || distance <= 0 {
		return false
	}
	dx, dy := target.Pos.X-src.Pos.X, target.Pos.Y-src.Pos.Y
	length := math.Hypot(dx, dy)
	if length == 0 {
		return false
	}
	ux, uy := dx/length, dy/length
	from, to := target.Pos, target.Pos
	const sample = 5.0
	steps := int(math.Ceil(float64(distance) / sample))
	for i := 1; i <= steps; i++ {
		pushed := math.Min(float64(distance), float64(i)*sample)
		candidate := domain.Pos{
			MapID: from.MapID,
			X:     from.X + ux*pushed,
			Y:     from.Y + uy*pushed,
		}
		if s.walkable != nil && !s.walkable(candidate) {
			break
		}
		to = candidate
	}
	if to == from {
		return false
	}
	s.aoi.Move(target, to)
	// 击退必须立即覆盖怪物此前的追击路径；普通追击的 500ms 节流不能吞掉它。
	s.emit(event.EntityMoved{ID: target.ID, To: to, Dir: target.Dir, Snap: true})
	target.Monster.CombatPathActive = false
	target.Monster.Roaming = false
	return true
}

// applyMonsterSkillExtras 结算必须由场景参与的怪物技能结果。直接伤害、治疗与
// 状态已经走统一的 applySkill；这里不再重复一套战斗公式。
func (s *Scene) applyMonsterSkillExtras(src *entity.Entity, skill domain.SkillDef, targets []domain.EntityID, extras []domain.MonsterSkillExtra) {
	if src == nil || len(extras) == 0 {
		return
	}
	for _, extra := range extras {
		switch extra.Kind {
		case domain.MonsterExtraDispelBeneficial, domain.MonsterExtraDispelHarmful:
			for _, targetID := range targets {
				target := s.entities[targetID]
				if target == nil || target.Status == nil {
					continue
				}
				removeHarmful := extra.Kind == domain.MonsterExtraDispelHarmful
				s.statusesRemoved(target, target.Status.RemoveWhere(func(def domain.StatusDef) bool {
					return def.Harmful() == removeHarmful
				}))
			}
		case domain.MonsterExtraClearAggro:
			if src.Monster != nil {
				s.dropTarget(src)
			}
		case domain.MonsterExtraSummon:
			s.summonMonsters(src, extra)
		case domain.MonsterExtraRandomStatus:
			if len(extra.Statuses) == 0 {
				continue
			}
			for _, targetID := range targets {
				target := s.entities[targetID]
				if target == nil || !target.Alive() {
					continue
				}
				app := extra.Statuses[s.aiRng.Intn(len(extra.Statuses))]
				chance := app.ChanceBP
				if chance <= 0 {
					chance = 10000
				}
				if chance < 10000 && int32(s.aiRng.Intn(10000)) >= chance {
					continue
				}
				statusDef, ok := s.statuses.Get(app.ID, app.Level)
				if !ok {
					continue
				}
				if app.DurationSec > 0 {
					statusDef.DurationSec = app.DurationSec
				}
				applyStatusSource(&statusDef, skill.Icon, int32(skill.ID))
				s.applyStatusDef(target, statusDef, src.ID)
			}
		case domain.MonsterExtraDespawn:
			s.despawnMonster(src)
		}
	}
}

const (
	maxSummonsPerCaster = 16
	maxSummonsPerScene  = 128
)

var summonOffsets = [...]struct{ x, y float64 }{
	{35, 0}, {-35, 0}, {0, 35}, {0, -35},
	{25, 25}, {-25, 25}, {25, -25}, {-25, -25},
}

func (s *Scene) summonMonsters(caster *entity.Entity, extra domain.MonsterSkillExtra) {
	if caster == nil || caster.Monster == nil || s.defs == nil || s.alloc == nil ||
		extra.SummonMonster == 0 || extra.Count <= 0 {
		return
	}
	def, ok := s.defs.Def(extra.SummonMonster)
	if !ok || !def.Kind.Hostile() {
		return
	}
	owned, temporary := 0, 0
	for _, monster := range s.monsters {
		if monster.Monster == nil || monster.Monster.Summoner == 0 {
			continue
		}
		temporary++
		if monster.Monster.Summoner == caster.ID {
			owned++
		}
	}
	limit := int(extra.Count)
	if remain := maxSummonsPerCaster - owned; limit > remain {
		limit = remain
	}
	if remain := maxSummonsPerScene - temporary; limit > remain {
		limit = remain
	}
	for i := 0; i < limit; i++ {
		offset := summonOffsets[(owned+i)%len(summonOffsets)]
		pos := caster.Pos
		pos.X += offset.x
		pos.Y += offset.y
		if s.walkable != nil && !s.walkable(pos) {
			pos = caster.Pos
		}
		monster := spawn.Summon(s.alloc.Monster(), pos, def, caster.ID)
		monster.Monster.Target = caster.Monster.Target
		if extra.Lifetime > 0 {
			monster.Monster.ExpireAt = s.tick + extra.Lifetime
		}
		s.initializeMonster(monster)
		s.entities[monster.ID] = monster
		s.monsters[monster.ID] = monster
		s.aoi.Enter(monster)
		s.emit(s.spawnEvent(monster))
	}
}

// despawnMonster 是技能消失与召唤到期的共同出口：没有死亡事件、击杀奖励或
// 掉落。普通刷怪点上的特殊消失仍会按模板重刷，但不累计精英击杀保底。
func (s *Scene) despawnMonster(monster *entity.Entity) {
	if monster == nil || monster.Monster == nil {
		return
	}
	s.emit(event.EntityDespawned{ID: monster.ID, Kind: monster.Kind, Reason: event.DespawnRemoved})
	s.aoi.Leave(monster)
	delete(s.entities, monster.ID)
	delete(s.monsters, monster.ID)
	delete(s.hits, monster.ID)
	s.scheduleRespawn(monster, false)
}

func (s *Scene) despawnSummonedMonster(monster *entity.Entity) {
	if monster == nil || monster.Monster == nil || monster.Monster.Summoner == 0 {
		return
	}
	s.despawnMonster(monster)
}

// sortEntityIDs 让范围技能的结算顺序不依赖 AOI 内部遍历顺序。
func sortEntityIDs(ids []domain.EntityID) []domain.EntityID {
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}
