package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

type legendaryChildOptions struct {
	TargetOnly, SuppressGeneration bool
	PreserveControl                bool
	SuppressVisual                 bool // synthetic payload supplies an explicit producer visual instead
	BaseDamage, ScaleBP            int32
	ApplyStatuses                  bool
}
type legendaryChildFrame struct {
	SuppressGeneration   bool
	PreserveControlSkill domain.SkillID
	Producers            map[string]bool
	Depth                int
	RootID               uint64
	Producer             string
	Source               *domain.CombatSourceSnapshotV5
	Def                  domain.SkillDef
}

// ChildSkill is one declared skill-derived payload, not another player cast.
// It inherits pure modifiers; each producer may occur only once in its lineage.
func (c *legendaryContext) ChildSkill(target domain.EntityID, def domain.SkillDef, opt legendaryChildOptions) bool {
	if c.Preview {
		return false
	}
	s := c.Scene
	dst := s.entities[target]
	rt := s.legendaryState()
	if rt.depth != 0 && rt.child == nil || rt.child != nil && (rt.child.SuppressGeneration || rt.child.Depth >= 8 || rt.child.Producers[c.Power.ID]) || dst == nil || !dst.Alive() || !s.equipmentProcEnemyTarget(c.Owner, dst) {
		return false
	}
	def.Effects = append([]domain.SkillEffect(nil), def.DirectEffects()...)
	if !opt.ApplyStatuses {
		def.Statuses = nil
		effects := def.Effects[:0]
		for _, e := range def.Effects {
			if e.Kind == domain.EffectDamage {
				effects = append(effects, e)
			}
		}
		def.Effects = effects
	}
	if opt.BaseDamage > 0 {
		for i := range def.Effects {
			e := &def.Effects[i]
			if e.Kind == domain.EffectDamage {
				e.DamagePct = 0
				e.DamagePctNumerator = 0
				e.DamagePctDenominator = 1
				e.BaseDamageFlat = opt.BaseDamage
				e.DamageFlat = 0
			}
		}
	}
	s.applyResolvedSkillModifiers(c.Owner, &def, c.Owner.Player.Char.Skills, s.rawSkillLevels(c.Owner))
	contexts := s.legendaryContextsFor(c.Owner, dst, combat.Blow{SkillID: int32(def.ID), SourceOffense: c.SourceSnapshot}, event.DamageDealt{}, true)
	legendaryOrderSkillContexts(contexts)
	for stage := 0; stage < 3; stage++ {
		for _, cc := range contexts {
			cc.IsChild = true
			cc.RootID = c.RootID
			cc.Def = def
			cc.Skill = def.ID
			h := legendaryHandlers[cc.Power.ID]
			fn := h.TransformSkill
			if stage == 1 {
				fn = h.BaseSkill
			}
			if stage == 2 {
				fn = h.PrepareSkill
			}
			if fn != nil {
				fn(&cc, &def)
			}
		}
	}
	legendaryCasterHolyStatusBonus(c.Owner, &def)
	s.legendaryFinalizeSkill(&def)
	if !opt.ApplyStatuses {
		def.Statuses = nil
		effects := def.Effects[:0]
		for _, effect := range def.DirectEffects() {
			if effect.Kind == domain.EffectDamage {
				effects = append(effects, effect)
			}
		}
		def.Effects = effects
	}
	if opt.ScaleBP > 0 && opt.ScaleBP != 10000 {
		for i := range def.Effects {
			e := &def.Effects[i]
			if e.Kind == domain.EffectDamage {
				domain.ScaleSkillDamageFraction(e, int64(opt.ScaleBP), 10000)
				e.DamageFlat = domain.SaturateInt32(int64(e.DamageFlat) * int64(opt.ScaleBP) / 10000)
			}
		}
	}
	previous := rt.child
	producers := map[string]bool{c.Power.ID: true}
	depth := 1
	if previous != nil {
		depth = previous.Depth + 1
		for id := range previous.Producers {
			producers[id] = true
		}
	}
	preserve := domain.SkillID(0)
	if opt.PreserveControl {
		preserve = c.Skill
	}
	rt.child = &legendaryChildFrame{SuppressGeneration: opt.SuppressGeneration, PreserveControlSkill: preserve, Producers: producers, Depth: depth, RootID: c.RootID, Producer: c.Power.ID, Source: c.SourceSnapshot, Def: def}
	rt.depth++
	defer func() { rt.child = previous; rt.depth-- }()
	targets := []domain.EntityID{target}
	if !opt.TargetOnly {
		targets = s.resolveTargets(c.Owner, def, target, nil)
	}
	originalTargets := append([]domain.EntityID(nil), targets...)
	if !opt.TargetOnly {
		for _, cc := range contexts {
			cc.IsChild = true
			cc.Def = def
			cc.Primary = target
			cc.Targets = targets
			cc.OriginalTargets = originalTargets
			if h := legendaryHandlers[cc.Power.ID].SelectTargets; h != nil {
				targets = h(&cc, targets)
			}
		}
	}
	if len(targets) == 0 {
		targets = []domain.EntityID{target}
	}
	// One art-only native release per admitted child. It does not create another
	// gameplay cast, change the actor animation, or schedule delayed damage.
	center := dst.Pos
	if skillCenteredOnCaster(def) || def.Area.Center == domain.SkillAreaCenterCaster {
		center = c.Owner.Pos
	}
	if !opt.SuppressVisual {
		s.emit(event.ChildSkillReleased{Caster: c.Owner.ID, Skill: def.ID, Target: target, Origin: c.Owner.Pos, Center: center})
	}
	landed := false
	seen := map[domain.EntityID]bool{}
	for _, id := range targets {
		if seen[id] {
			continue
		}
		seen[id] = true
		if len(seen) > 256 {
			break
		}
		victim := s.entities[id]
		if victim == nil || !victim.Alive() || !s.equipmentProcEnemyTarget(c.Owner, victim) {
			continue
		}
		local := def
		for _, cc := range contexts {
			cc.IsChild = true
			cc.Target = victim
			cc.Def = local
			cc.Primary = target
			cc.Targets = targets
			cc.OriginalTargets = originalTargets
			if h := legendaryHandlers[cc.Power.ID].TargetDefinition; h != nil {
				h(&cc, &local)
			}
		}
		hit := s.applySkill(c.Owner, victim, local, 0, false)
		if hit && !opt.SuppressVisual {
			if local.HitEffect != "" {
				s.emit(event.SkillHitEffect{Target: victim.ID, Effect: local.HitEffect})
			} else {
				c.NativeImpact(local.ID, victim)
			}
		}
		landed = hit || landed
	}
	return landed
}
func (s *Scene) legendarySkillResolved(src, dst *entity.Entity, def domain.SkillDef, landed bool) {
	child := s.legendaryState().child
	if child != nil && child.SuppressGeneration {
		return
	}
	for _, c := range s.legendaryContextsFor(src, dst, combat.Blow{SkillID: int32(def.ID), LegendaryChild: child != nil}, event.DamageDealt{}, child != nil) {
		if child != nil && child.Producers[c.Power.ID] {
			continue
		}
		c.Def = def
		c.IsChild = child != nil
		if h := legendaryHandlers[c.Power.ID].SkillResolved; h != nil {
			h(&c)
		}
	}
}
func (s *Scene) legendaryHealResolved(src, dst *entity.Entity, def domain.SkillDef, raw, effective, overflow int32) {
	for _, c := range s.legendaryContexts(src, dst, combat.Blow{SkillID: int32(def.ID)}, event.DamageDealt{}) {
		c.Def = def
		if h := legendaryHandlers[c.Power.ID].HealResolved; h != nil {
			h(&c, raw, effective, overflow)
		}
	}
}
func (s *Scene) legendaryImpactTargets(src *entity.Entity, def domain.SkillDef, primary domain.EntityID, targets []domain.EntityID) []domain.EntityID {
	for _, c := range s.legendaryContexts(src, s.entities[primary], combat.Blow{SkillID: int32(def.ID)}, event.DamageDealt{}) {
		c.Def = def
		c.Primary = primary
		c.Targets = targets
		if h := legendaryHandlers[c.Power.ID].ImpactTargets; h != nil {
			targets = h(&c, targets)
		}
	}
	seen := map[domain.EntityID]bool{}
	out := make([]domain.EntityID, 0, len(targets))
	for _, id := range targets {
		if !seen[id] && s.entities[id] != nil {
			seen[id] = true
			out = append(out, id)
		}
	}
	if root := s.equipmentState().root; root != nil {
		root.targets = out
	}
	return out
}

func (s *Scene) legendaryPassiveStats(owner *entity.Entity, stats *domain.Stats) {
	base := *stats
	var delta [20]int64
	if owner == nil || owner.Player == nil || owner.Player.Char == nil {
		return
	}
	for _, power := range domain.ActiveLegendaryPowers(s.functionalWorn(owner), s.itemDef, owner.Player.Char) {
		if !legendaryHandlerCurrent(power.Power.ID) {
			continue
		}
		c := legendaryContext{Scene: s, Owner: owner, Power: power.Power, SourceUID: power.SourceUID, NowMS: s.tick.Millis()}
		if h := legendaryHandlers[c.Power.ID].PassiveStats; h != nil {
			local := base
			h(&c, &local)
			delta[0] += int64(local.MaxHP) - int64(base.MaxHP)
			delta[1] += int64(local.MaxMP) - int64(base.MaxMP)
			delta[2] += int64(local.HPRegen) - int64(base.HPRegen)
			delta[3] += int64(local.MPRegen) - int64(base.MPRegen)
			delta[4] += int64(local.MinAtk) - int64(base.MinAtk)
			delta[5] += int64(local.MaxAtk) - int64(base.MaxAtk)
			delta[6] += int64(local.Def) - int64(base.Def)
			delta[7] += int64(local.Hit) - int64(base.Hit)
			delta[8] += int64(local.MAtk) - int64(base.MAtk)
			delta[9] += int64(local.MDef) - int64(base.MDef)
			delta[10] += int64(local.EffectHitBP) - int64(base.EffectHitBP)
			delta[11] += int64(local.CritRate) - int64(base.CritRate)
			delta[12] += int64(local.MCritRate) - int64(base.MCritRate)
			delta[13] += int64(local.AtkSpeedPct) - int64(base.AtkSpeedPct)
			delta[14] += int64(local.AtkSpeedMS) - int64(base.AtkSpeedMS)
			delta[15] += int64(local.MoveSpeed) - int64(base.MoveSpeed)
			delta[16] += int64(local.PhysResist) - int64(base.PhysResist)
			delta[17] += int64(local.MagicResist) - int64(base.MagicResist)
			delta[18] += int64(local.StatusResist) - int64(base.StatusResist)
			delta[19] += int64(local.MaxWeight) - int64(base.MaxWeight)
		}
	}
	stats.MaxHP = domain.SaturateInt32(int64(base.MaxHP) + delta[0])
	stats.MaxMP = domain.SaturateInt32(int64(base.MaxMP) + delta[1])
	stats.HPRegen = domain.SaturateInt32(int64(base.HPRegen) + delta[2])
	stats.MPRegen = domain.SaturateInt32(int64(base.MPRegen) + delta[3])
	stats.MinAtk = domain.SaturateInt32(int64(base.MinAtk) + delta[4])
	stats.MaxAtk = domain.SaturateInt32(int64(base.MaxAtk) + delta[5])
	stats.Def = domain.SaturateInt32(int64(base.Def) + delta[6])
	stats.Hit = domain.SaturateInt32(int64(base.Hit) + delta[7])
	stats.MAtk = domain.SaturateInt32(int64(base.MAtk) + delta[8])
	stats.MDef = domain.SaturateInt32(int64(base.MDef) + delta[9])
	stats.EffectHitBP = domain.SaturateInt32(int64(base.EffectHitBP) + delta[10])
	stats.CritRate = domain.SaturateInt32(int64(base.CritRate) + delta[11])
	stats.MCritRate = domain.SaturateInt32(int64(base.MCritRate) + delta[12])
	stats.AtkSpeedPct = domain.SaturateInt32(int64(base.AtkSpeedPct) + delta[13])
	stats.AtkSpeedMS = domain.SaturateInt32(int64(base.AtkSpeedMS) + delta[14])
	stats.MoveSpeed = domain.SaturateInt32(int64(base.MoveSpeed) + delta[15])
	stats.PhysResist = domain.SaturateInt32(int64(base.PhysResist) + delta[16])
	stats.MagicResist = domain.SaturateInt32(int64(base.MagicResist) + delta[17])
	stats.StatusResist = domain.SaturateInt32(int64(base.StatusResist) + delta[18])
	stats.MaxWeight = domain.SaturateInt32(int64(base.MaxWeight) + delta[19])
}
func (s *Scene) legendaryStatusReceiving(owner, source *entity.Entity, def *domain.StatusDef) bool {
	for _, c := range s.legendaryContextsFor(owner, source, combat.Blow{}, event.DamageDealt{}, true) {
		if h := legendaryHandlers[c.Power.ID].StatusReceiving; h != nil && !h(&c, def) {
			return false
		}
	}
	return true
}
func (s *Scene) legendaryStatusCommitted(src, dst *entity.Entity, def *domain.StatusDef) {
	for _, c := range s.legendaryContexts(src, dst, combat.Blow{SkillID: int32(def.SkillSnapshotID)}, event.DamageDealt{}) {
		if h := legendaryHandlers[c.Power.ID].StatusCommitted; h != nil {
			h(&c, def)
		}
	}
}
func (s *Scene) legendaryPeriodicResolved(src, dst *entity.Entity, active domain.Active, hit event.DamageDealt) {
	if hit.ProcDepth != 0 || hit.Origin == domain.DamageProcOrigin {
		return
	}
	for _, c := range s.legendaryContexts(src, dst, combat.Blow{SkillID: int32(active.Def.SkillSnapshotID), Origin: domain.DamageDOTOrigin}, hit) {
		if h := legendaryHandlers[c.Power.ID].PeriodicResolved; h != nil {
			h(&c, active)
		}
	}
}
func (s *Scene) legendaryMagicResisted(owner, attacker *entity.Entity, blow combat.Blow, amount int32) int32 {
	if blow.ProcDepth != 0 || blow.Origin == domain.DamageProcOrigin {
		return amount
	}
	for _, c := range s.legendaryContexts(owner, attacker, blow, event.DamageDealt{Src: attacker.ID, Dst: owner.ID}) {
		if h := legendaryHandlers[c.Power.ID].MagicResisted; h != nil {
			amount = max(int32(0), h(&c, amount))
		}
	}
	return amount
}

func (s *Scene) legendaryEquipmentChanged(owner *entity.Entity) {
	if owner == nil || owner.Player == nil || owner.Player.Char == nil {
		return
	}
	powers := domain.ActiveLegendaryPowers(s.functionalWorn(owner), s.itemDef, owner.Player.Char)
	signature := ""
	for _, power := range powers {
		signature += power.Power.ID + ":" + fmt.Sprint(power.SourceUID) + ";"
	}
	rt := s.legendaryState()
	if rt.equipmentSignature == nil {
		rt.equipmentSignature = map[domain.EntityID]string{}
	}
	if old, known := rt.equipmentSignature[owner.ID]; known && old == signature {
		return
	}
	// Write before callbacks: native status removal can recompute stats reentrantly.
	rt.equipmentSignature[owner.ID] = signature
	for _, power := range powers {
		if !legendaryHandlerCurrent(power.Power.ID) {
			continue
		}
		c := legendaryContext{Scene: s, Owner: owner, Power: power.Power, SourceUID: power.SourceUID, NowMS: s.tick.Millis()}
		if h := legendaryHandlers[c.Power.ID].EquipmentChanged; h != nil {
			h(&c)
		}
	}
}

// legendaryModifyBlow only changes this strike's value snapshot, never entities.
func (s *Scene) legendaryModifyBlow(src, dst *entity.Entity, blow *combat.Blow) {
	if blow == nil || blow.LegendaryPayload {
		return
	}
	child := blow.LegendaryChild
	if blow.Origin == domain.DamageProcOrigin && !child {
		return
	}
	for _, c := range s.legendaryContextsFor(src, dst, *blow, event.DamageDealt{}, child) {
		if fn := legendaryHandlers[c.Power.ID].ModifyBlow; fn != nil {
			fn(&c, blow)
		}
	}
}

func legendaryTargetUniqueDamage(source, current *domain.CombatSourceSnapshotV5, ctx domain.UniqueContext) int32 {
	filter := func(in *domain.CombatSourceSnapshotV5) *domain.CombatSourceSnapshotV5 {
		if in == nil {
			return nil
		}
		out := *in
		out.Traits = nil
		for _, t := range in.Traits {
			if t.Kind == "execution" || t.Kind == "ambush" || t.Kind == "elite_hunter" {
				out.Traits = append(out.Traits, t)
			}
		}
		return &out
	}
	if source == nil {
		return 0
	}
	return sourceUniqueEffectsV5(filter(source), filter(current), ctx).DamageBP
}

// NativeImpact reuses the client's native skill hit phases without inventing an
// effect identifier. Call only after an authoritative successful settlement.
func (c *legendaryContext) NativeImpact(skill domain.SkillID, target *entity.Entity) bool {
	if c.Preview || c.Owner == nil || target == nil || skill <= 0 {
		return false
	}
	c.Scene.emit(event.ChildSkillReleased{Caster: c.Owner.ID, Skill: skill, Target: target.ID, Origin: c.Owner.Pos, Center: target.Pos, Mode: 1})
	return true
}
