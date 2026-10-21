package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"slices"
	"sort"
)

type equipmentProcCandidate struct {
	active domain.ActiveEquipmentEffect
	hit    equipmentHit
	owner  domain.EntityID
	pvp    bool
}

// Runs after all root damage/death/loot has committed. Only approved damage or
// heal payloads execute, never statuses or arbitrary side effects of a SkillDef.
func (s *Scene) executeEquipmentRoot(root *equipmentRoot) {
	if root == nil || len(root.hits) == 0 {
		return
	}
	// Most roots have few targets. Keep their endpoint list on the stack,
	// without filtering owners before earlier procs have finished executing.
	var ownerBuf [32]domain.EntityID
	for _, ownerID := range equipmentRootOwners(root.hits, ownerBuf[:0]) {
		owner := s.players[ownerID]
		if owner == nil || !owner.Alive() {
			continue
		}
		var candidates []equipmentProcCandidate
		for _, active := range s.equipmentEffects(owner).effects {
			p := active.Effect.Proc
			if active.Effect.Kind != "proc" || p == nil {
				continue
			}
			var chosen *equipmentHit
			for i := range root.hits {
				h := &root.hits[i]
				valid := false
				switch p.Trigger {
				case "on_attack":
					valid = h.src == ownerID && h.blow.SkillID == 0
				case "on_direct_hit":
					valid = h.src == ownerID && h.ev.Amount > 0 && !h.ev.Flag.Has(event.DamageMiss) && (!p.RequireMagic || h.blow.Magic)
				case "on_direct_damage_taken":
					valid = h.dst == ownerID && h.ev.Amount > 0 && !h.ev.Flag.Has(event.DamageMiss) && !h.ev.Flag.Has(event.DamageFatal)
				}
				if !valid {
					continue
				}
				if p.HPBelowPct > 0 && int64(owner.HP)*100 > int64(owner.MaxHP)*int64(p.HPBelowPct) {
					continue
				}
				if chosen == nil || (h.dst == root.primary && chosen.dst != root.primary) || (h.dst != root.primary && chosen.dst != root.primary && h.dst < chosen.dst) {
					chosen = h
				}
			}
			if chosen != nil {
				candidates = append(candidates, equipmentProcCandidate{active, *chosen, ownerID, s.equipmentPVP(s.entities[chosen.src], s.entities[chosen.dst])})
			}
		}
		sort.Slice(candidates, func(i, j int) bool {
			a, b := candidates[i].active.Effect.Proc, candidates[j].active.Effect.Proc
			if a.Priority != b.Priority {
				return a.Priority > b.Priority
			}
			if a.FixedRank != b.FixedRank {
				return a.FixedRank > b.FixedRank
			}
			if a.ID != b.ID {
				return a.ID < b.ID
			}
			return candidates[i].active.SourceUID < candidates[j].active.SourceUID
		})
		groups := map[string]bool{}
		for _, c := range candidates {
			p := c.active.Effect.Proc
			if groups[p.SharedGroup] {
				continue
			}
			groups[p.SharedGroup] = true
			if s.reserveEquipmentProc(owner, p, root.id) {
				s.executeEquipmentProc(owner, c)
				s.publishEquipmentCombatStats(owner)
				break
			}
		}
	}
}

// equipmentRootOwners returns each endpoint once in the same ascending order as
// the former map/key sort. It does not change hits or retain the caller's buffer.
func equipmentRootOwners(hits []equipmentHit, owners []domain.EntityID) []domain.EntityID {
	owners = owners[:0]
	for _, h := range hits {
		owners = append(owners, h.src, h.dst)
	}
	slices.Sort(owners)
	return slices.Compact(owners)
}

func (s *Scene) reserveEquipmentProc(owner *entity.Entity, p *domain.EquipmentProc, root uint64) bool {
	now := s.now().UnixMilli()
	cd := owner.Player.Char.EquipmentProcCooldowns
	if cd["global"] > now || cd["group:"+p.SharedGroup] > now || cd["effect:"+p.ID] > now {
		return false
	}
	if p.ChanceBP <= 0 || p.ChanceBP < 10000 && s.equipmentProcRoll(owner.ID, p.ID, root) >= uint64(p.ChanceBP) {
		return false
	}
	if cd == nil {
		cd = map[string]int64{}
		owner.Player.Char.EquipmentProcCooldowns = cd
	}
	for key, expiry := range cd {
		if expiry <= now {
			delete(cd, key)
		}
	}
	cd["global"] = now + 1000
	cd["group:"+p.SharedGroup] = now + int64(p.ICDMS)
	cd["effect:"+p.ID] = now + int64(p.ICDMS)
	owner.Player.MarkDirty()
	return true
}
func (s *Scene) equipmentProcRoll(owner domain.EntityID, id string, root uint64) uint64 {
	// Counter-derived stream is independent of hit, crit, loot and visual RNG.
	x := s.equipmentSeed ^ uint64(owner)*0x9e3779b97f4a7c15 ^ root*0xbf58476d1ce4e5b9
	for _, b := range []byte(id) {
		x = (x ^ uint64(b)) * 0x100000001b3
	}
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	x ^= x >> 31
	return x % 10000
}
func (s *Scene) executeEquipmentProc(owner *entity.Entity, c equipmentProcCandidate) {
	if owner == nil || !owner.Alive() || s.entityMapLoading(owner) {
		return
	}
	p := c.active.Effect.Proc
	stillActive := false
	for _, a := range s.equipmentEffects(owner).effects {
		if a.SourceUID == c.active.SourceUID && a.Effect.Proc != nil && a.Effect.Proc.ID == p.ID {
			stillActive = true
			break
		}
	}
	if !stillActive {
		return
	}
	def, ok := s.skills.Get(p.SkillID, p.FixedRank)
	if !ok {
		s.equipmentState().suppressedProcs++
		s.log.Error("equipment proc fixed rank unavailable", "effect", p.ID, "skill", p.SkillID, "rank", p.FixedRank)
		return
	}
	target := s.entities[c.hit.dst]
	if p.TargetRule == "self" {
		target = owner
	}
	if target == nil || !target.Alive() || s.entityMapLoading(target) {
		return
	}
	if target != owner {
		if !s.equipmentProcEnemyTarget(owner, target) {
			return
		}
		dist := def.Dist
		if dist <= 0 {
			dist = 75
		}
		if sqDist(owner.Pos, target.Pos) > float64(dist)*float64(dist) || !s.equipmentLineClear(owner.Pos, target.Pos) {
			return
		}
	}
	scale := p.ScaleBP
	if c.pvp {
		scale = int32(int64(scale) * int64(p.PVPScaleBP) / 10000)
	}
	s.emit(event.SkillCastChanged{Caster: owner.ID, Skill: def.ID, Phase: event.SkillCastRelease, Target: target.ID})
	if target.Monster != nil && s.defs != nil {
		if monster, ok := s.defs.Def(target.Monster.TypeID); ok {
			if _, immune := monster.ImmuneSkills[def.ID]; immune {
				s.emit(event.DamageDealt{Src: owner.ID, Dst: target.ID, SkillID: int32(def.ID), DstHP: target.HP, Flag: event.DamageMiss, Origin: domain.DamageProcOrigin, RootID: c.hit.ev.RootID, ProcDepth: 1})
				return
			}
		}
	}
	for _, effect := range def.DirectEffects() {
		if p.Payload == "heal" && effect.Kind == domain.EffectHeal {
			amount := clampI32(int64(effect.HealAmount(target.MaxHP)) * int64(scale) / 10000)
			s.emit(combat.Restore(owner, target, amount, int32(def.ID)))
			s.emitAttributesIfPlayer(target)
			break
		}
		if p.Payload == "damage" && effect.Kind == domain.EffectDamage {
			tr := s.hits[owner.ID]
			if tr == nil {
				tr = &combat.HitTracker{}
				s.hits[owner.ID] = tr
			}
			blow := combat.Blow{Origin: domain.DamageProcOrigin, ProcDepth: 1, RootID: c.hit.ev.RootID, HurtType: def.HurtType, SkillID: int32(def.ID), SkillPct: effect.DamagePct, FlatDamage: effect.DamageFlat, GuaranteedHit: effect.GuaranteedHit, Magic: def.HurtType != domain.HurtNone && def.HurtType != domain.HurtPhysical, PayloadScaleBP: scale, PayloadScaleKnown: true}
			blow.SkillPctNumerator, blow.SkillPctDenominator = effect.DamagePctNumerator, effect.DamagePctDenominator
			ev, raw, reflected := s.resolveStrikeStatuses(owner, target, tr, blow)
			s.emit(ev)
			s.afterStrikeStatuses(owner, target, ev, raw, reflected)
			s.emitAttributesIfPlayer(target)
			s.aggroAfterHit(target, owner.ID, ev)
			if ev.Flag.Has(event.DamageFatal) {
				s.onDeath(target, owner.ID)
			}
			break
		}
	}
}
func (s *Scene) equipmentLineClear(a, b domain.Pos) bool {
	if s.walkable == nil {
		return true
	}
	dx, dy := b.X-a.X, b.Y-a.Y
	steps := int(max(equipmentAbsFloat(dx), equipmentAbsFloat(dy))/16) + 1
	for i := 1; i <= steps; i++ {
		t := float64(i) / float64(steps)
		if !s.walkable(domain.Pos{MapID: a.MapID, X: a.X + dx*t, Y: a.Y + dy*t}) {
			return false
		}
	}
	return true
}
func equipmentAbsFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// Preserve the current map PK policy. The coefficient supports PvP, but an
// equipment proc must never open PvP on maps advertised as safe or hit friends.
func (s *Scene) equipmentProcEnemyTarget(owner, target *entity.Entity) bool {
	if owner == nil || target == nil || owner.ID == target.ID || !s.equipmentCombatReady(target) {
		return false
	}
	if target.Kind == domain.KindPet {
		return false
	} // Current native pets are noncombat.
	if target.Player != nil {
		return owner.Player != nil && !sameParty(owner, target) && !pkStateSnapshot(owner).MapSafe && !pkStateSnapshot(target).MapSafe
	}
	if target.Kind != domain.KindMonster || (target.Monster != nil && !target.Monster.Kind.Hostile()) {
		return false
	}
	if target.Monster != nil && target.Monster.Summoner != 0 {
		summoner := s.entities[target.Monster.Summoner]
		if summoner != nil && summoner.Player != nil {
			return !sameParty(owner, summoner) && !pkStateSnapshot(owner).MapSafe
		}
	}
	return true
}

// EquipmentProcReadinessView is a bounded, read-only observation, not an action
// endpoint. A ready cooldown never bypasses the legal-event/target checks.
type EquipmentProcReadinessView = domain.EquipmentProcReadinessView

func (s *Scene) equipmentProcReadiness(p *entity.Entity) []EquipmentProcReadinessView {
	if p == nil || p.Player == nil || p.Player.Char == nil {
		return nil
	}
	now := s.now().UnixMilli()
	cd := p.Player.Char.EquipmentProcCooldowns
	seen := map[string]bool{}
	var out []EquipmentProcReadinessView
	for _, a := range s.equipmentEffects(p).effects {
		v := a.Effect.Proc
		if v == nil || seen[v.ID] {
			continue
		}
		seen[v.ID] = true
		remaining := max(int64(0), max(cd["effect:"+v.ID], cd["group:"+v.SharedGroup])-now)
		global := max(int64(0), cd["global"]-now)
		out = append(out, EquipmentProcReadinessView{EffectID: v.ID, SourceUID: a.SourceUID, Trigger: v.Trigger, SkillID: v.SkillID, FixedRank: v.FixedRank, ChanceBP: v.ChanceBP, ICDMS: v.ICDMS, RemainingMS: remaining, GlobalRemainingMS: global, Ready: p.Alive() && s.equipmentCombatReady(p) && remaining == 0 && global == 0})
		if len(out) >= domain.MaxEquipmentProcReadiness {
			break
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EffectID < out[j].EffectID })
	return out
}
