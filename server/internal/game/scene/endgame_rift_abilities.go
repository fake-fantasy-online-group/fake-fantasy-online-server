package scene

import (
	"math"
	"sort"
	"strconv"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// Rift affixes are encounter-local mechanics, not changes to native monster or
// skill definitions. Their damage still goes through the ordinary authoritative
// strike/status/death pipeline. Tiers scale the entity's stats; floors only mix
// the pattern seed and never multiply its damage or shorten its warning.
type endgameRiftAbility uint8

const (
	riftEruption endgameRiftAbility = iota
	riftNova
	riftFissure
	riftMaxTelegraphs           = 4
	riftAbilityRange            = 400.0
	riftAbilityRecoveryMS int64 = 650
)

type endgameRiftAbilityState struct {
	eliteKey             string
	class                int32
	pattern              []endgameRiftAbility
	cursor               int
	sequence             uint64
	nextAt, recoverUntil int64
	pending              *endgameRiftAbilityCast
}

type endgameRiftAbilityCast struct {
	id                  string
	ability             endgameRiftAbility
	shape               string
	center, end         domain.Pos
	radius, innerRadius float64
	dueAt               int64
	damagePct           int32
	visual              domain.SkillDef
	// Only players present and ready when the tell was shown may be hit. A
	// reconnect, map acknowledgement or resurrection cannot join a hidden cast.
	warned map[domain.EntityID]bool
}

func (a endgameRiftAbility) name(rule *domain.RiftRules) string {
	if rule == nil || int(a) >= len(rule.Abilities) {
		return ""
	}
	return rule.Abilities[a].Name
}
func (a endgameRiftAbility) affix(rule *domain.RiftRules) string {
	if rule == nil || int(a) >= len(rule.Abilities) {
		return ""
	}
	return rule.Abilities[a].Affix
}
func (a endgameRiftAbility) warning(rule *domain.RiftRules) string {
	if rule == nil || int(a) >= len(rule.Abilities) {
		return ""
	}
	return rule.Abilities[a].Warning
}

func riftAbilitySeed(seed uint64, class int32, spawnIndex int, tier, floor int32) uint64 {
	// SplitMix64 finalizer gives each tuple its own deterministic stream without
	// consuming combat RNG or depending on entity allocation / map iteration.
	x := seed ^ uint64(spawnIndex+1)*0x9e3779b97f4a7c15 ^ uint64(class)*0xbf58476d1ce4e5b9 ^ uint64(tier)<<32 ^ uint64(floor)
	x = (x ^ x>>30) * 0xbf58476d1ce4e5b9
	x = (x ^ x>>27) * 0x94d049bb133111eb
	return x ^ x>>31
}

func (s *Scene) registerEndgameRiftAbilities(m *entity.Entity, class int32, spawnIndex int, seed uint64, floor int32) {
	if s.rift == nil || m == nil || m.Monster == nil || class < 1 || class > 3 {
		return
	}
	if s.rift.abilities == nil {
		s.rift.abilities = map[domain.EntityID]*endgameRiftAbilityState{}
	}
	if s.eliteRift() {
		s.registerRiftElite(m, class, spawnIndex, seed, floor)
		return
	}
	h := riftAbilitySeed(seed, class, spawnIndex, s.rift.data.Run.Tier, floor)
	count := int(class)
	state := &endgameRiftAbilityState{class: class, nextAt: s.riftMS() + s.currentRiftRules().AbilityStartMS + int64(h>>16)%s.currentRiftRules().AbilityJitterMS}
	// Gold elites alternate two distinct affixes; guardians cycle through all
	// three. The seed rotates and reverses this readable, repeating pattern.
	for i := 0; i < count; i++ {
		offset := i
		if h&8 != 0 {
			offset = 3 - i
		}
		state.pattern = append(state.pattern, endgameRiftAbility((int(h%3)+offset)%3))
	}
	s.rift.abilities[m.ID] = state
}

// cancel preserves patterns for a rolled-back floor transfer.
func (s *Scene) cancelEndgameRiftAbilities(now int64) {
	if s.rift == nil {
		return
	}
	for _, state := range s.rift.abilities {
		state.pending = nil
		state.nextAt = now + s.currentRiftRules().AbilityResetMS
		state.recoverUntil = 0
	}
	s.rift.lastBroadcast = now - 1000
}

func (s *Scene) clearEndgameRiftAbilities() {
	if s.rift != nil {
		s.rift.abilities = nil
	}
}

func (s *Scene) riftMonsterAffixes(id domain.EntityID) []string {
	if s.rift == nil || s.rift.abilities[id] == nil {
		return nil
	}
	var names []string
	for _, a := range s.rift.abilities[id].pattern {
		if s.disabledRiftAbility(a) {
			continue
		}
		names = append(names, a.affix(s.currentRiftRules()))
	}
	return names
}

func (s *Scene) riftMonsterAbilityBusy(id domain.EntityID) bool {
	if s.rift == nil || s.rift.abilities[id] == nil {
		return false
	}
	a := s.rift.abilities[id]
	if s.eliteRift() {
		c := s.eliteSnapshot().Casters[a.eliteKey]
		return c != nil && s.riftMS() < c.RecoverUntil
	}
	return a.pending != nil || s.riftMS() < a.recoverUntil
}

func (s *Scene) riftAbilityPlayerReady(p *entity.Entity) bool {
	if s.rift == nil || p == nil || p.Player == nil || p.Player.Char == nil || p.Kind != domain.KindPlayer ||
		!s.validMonsterTarget(p) || p.Pos.MapID != s.id.MapID {
		return false
	}
	id := domain.CharID(p.Player.Char.ID)
	return s.rift.ready[id] && s.rift.data.Run.Admitted(id)
}

func (s *Scene) riftAbilitySourceReady(m *entity.Entity) bool {
	if m == nil || m.Monster == nil || !m.Alive() || m.Pos.MapID != s.id.MapID ||
		!canAct(m) || !canCast(m) || !canDealDamage(m) || m.Monster.NoAttack {
		return false
	}
	registered := s.rift.data.Run.Monsters[m.ID]
	return registered != nil && !registered.Dead
}

func (s *Scene) stepEndgameRiftAbilities(now int64) {
	if s.eliteRift() {
		s.stepRiftElite(now)
		return
	}
	if s.rift == nil || len(s.rift.abilities) == 0 {
		return
	}
	changed := false
	if !s.riftCombatReady() {
		for _, state := range s.rift.abilities {
			changed = changed || state.pending != nil
			state.pending = nil
			state.nextAt = now + s.currentRiftRules().AbilityResetMS
			state.recoverUntil = 0
		}
		if changed {
			s.rift.lastBroadcast = now - 1000
		}
		return
	}
	ids := make([]domain.EntityID, 0, len(s.rift.abilities))
	active := 0
	for id, state := range s.rift.abilities {
		ids = append(ids, id)
		if state.pending != nil {
			active++
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		state := s.rift.abilities[id]
		if state == nil {
			continue
		} // Reflection may replace the entire population.
		m := s.monsters[id]
		if !s.riftAbilitySourceReady(m) {
			if state.pending != nil {
				state.pending = nil
				active--
				changed = true
				state.nextAt = now + s.currentRiftRules().AbilityUnavailableMS
			}
			if m == nil || !m.Alive() {
				delete(s.rift.abilities, id)
			}
			continue
		}
		if state.pending != nil {
			if now < state.pending.dueAt {
				continue
			}
			cast := state.pending
			state.pending = nil
			active--
			changed = true
			state.recoverUntil = now + s.currentRiftRules().AbilityRecoveryMS
			// Cooldowns start from actual resolution: a delayed scene step cannot
			// replay missed attacks or immediately start another cast.
			state.nextAt = now + s.currentRiftRules().AbilityCooldownMS - int64(state.class)*s.currentRiftRules().AbilityClassReductionMS
			s.resolveEndgameRiftAbility(m, cast)
			continue
		}
		if now < state.nextAt || active >= int(s.currentRiftRules().AbilityMaxConcurrent) || len(state.pattern) == 0 {
			continue
		}
		target := s.players[m.Monster.Target]
		if !s.riftAbilityPlayerReady(target) || entityInvisible(target) ||
			riftDistance2(m.Pos, target.Pos) > float64(s.currentRiftRules().AbilityRange)*float64(s.currentRiftRules().AbilityRange) || !s.staticPathWalkable(m.Pos, target.Pos) {
			continue
		}
		// Let already accepted native spells finish rather than overlaying an
		// unrelated spell animation and a rift warning.
		busy := m.Monster.CastingSkill != 0 && s.tick < m.Monster.CastingUntil
		for _, c := range s.casts {
			busy = busy || c.src == id
		}
		if busy {
			continue
		}
		state.sequence++
		cast := newRiftAbilityCast(state.pattern[state.cursor], m.Pos, target.Pos, state.class, now, s.currentRiftRules())
		cast.id = strconv.FormatUint(uint64(id), 10) + ":" + strconv.FormatUint(state.sequence, 10)
		cast.visual = s.riftAbilityVisual(m)
		cast.warned = map[domain.EntityID]bool{}
		for playerID, p := range s.players {
			if s.riftAbilityPlayerReady(p) {
				cast.warned[playerID] = true
			}
		}
		state.cursor = (state.cursor + 1) % len(state.pattern)
		state.pending = cast
		active++
		changed = true
		// Existing client movement / speech and (when available) skill effects
		// provide feedback; no invented resource IDs or external art are used.
		s.emit(event.EntityMoved{ID: id, To: m.Pos, Snap: true})
		if cast.visual.ID != 0 {
			center := cast.center
			s.emit(event.SkillCastChanged{Caster: id, Skill: cast.visual.ID, Phase: event.SkillCastChant,
				ChantHoldMS: int32(cast.dueAt - now), Aim: &center})
		}
	}
	if changed {
		// Called before the normal state broadcast in stepEndgameRift. Send a
		// fresh snapshot this frame so the entire warning window is visible.
		s.rift.lastBroadcast = now - 1000
	}
}

func newRiftAbilityCast(ability endgameRiftAbility, source, target domain.Pos, class int32, now int64, rule *domain.RiftRules) *endgameRiftAbilityCast {
	if rule == nil || int(ability) >= len(rule.Abilities) {
		return nil
	}
	def := rule.Abilities[ability]
	round := func(p domain.Pos) domain.Pos {
		p.X, p.Y = math.Round(p.X), math.Round(p.Y)
		return p
	}
	c := &endgameRiftAbilityCast{ability: ability, center: round(target), shape: def.Shape, radius: float64(def.Radius), innerRadius: float64(def.InnerRadius), dueAt: now + def.DelayMS, damagePct: def.DamagePct}
	if def.Shape == "ring" {
		c.center = round(source)
	}
	if def.Shape == "line" {
		c.center = round(source)
		dx, dy := target.X-source.X, target.Y-source.Y
		length := math.Hypot(dx, dy)
		if length < 1 {
			dx, dy, length = 1, 0, 1
		}
		c.end = round(domain.Pos{MapID: source.MapID, X: source.X + dx/length*float64(def.Length), Y: source.Y + dy/length*float64(def.Length)})
	}
	if class == 3 {
		c.damagePct += rule.AbilityBossDamagePct
	}
	return c
}

func (c *endgameRiftAbilityCast) contains(p domain.Pos) bool {
	if p.MapID != c.center.MapID || math.IsNaN(p.X) || math.IsNaN(p.Y) || math.IsInf(p.X, 0) || math.IsInf(p.Y, 0) {
		return false
	}
	d2 := riftDistance2(c.center, p)
	switch c.shape {
	case "circle":
		return d2 <= c.radius*c.radius
	case "ring":
		return d2 <= c.radius*c.radius && d2 >= c.innerRadius*c.innerRadius
	case "line":
		dx, dy := c.end.X-c.center.X, c.end.Y-c.center.Y
		length2 := dx*dx + dy*dy
		if length2 <= 0 {
			return false
		}
		t := ((p.X-c.center.X)*dx + (p.Y-c.center.Y)*dy) / length2
		if t < 0 || t > 1 {
			return false
		}
		x, y := p.X-c.center.X-t*dx, p.Y-c.center.Y-t*dy
		return x*x+y*y <= c.radius*c.radius
	}
	return false
}

func (s *Scene) riftAbilityVisual(m *entity.Entity) domain.SkillDef {
	var chosen domain.SkillDef
	if s.defs != nil {
		if d, ok := s.defs.Def(m.Monster.TypeID); ok {
			for _, rule := range d.Skills {
				if rule.Usable() && rule.Skill.HasDamageEffect() && (chosen.ID == 0 || rule.Skill.ID < chosen.ID) {
					chosen = rule.Skill
				}
			}
		}
	}
	// Fireball already ships in the native skill table/client. The lookup must
	// succeed; minimal fixtures and missing catalogs use only geometric tells.
	if chosen.ID == 0 && s.skills != nil {
		if def, ok := s.skills.Get(s.currentRiftRules().VisualSkill, s.currentRiftRules().VisualSkillLevel); ok && def.HasDamageEffect() {
			chosen = def
		}
	}
	return chosen
}

func (s *Scene) resolveEndgameRiftAbility(m *entity.Entity, cast *endgameRiftAbilityCast) {
	if !s.riftCombatReady() || !s.riftAbilitySourceReady(m) {
		return
	}
	if cast.visual.ID != 0 {
		center := cast.center
		s.emit(event.SkillCastChanged{Caster: m.ID, Skill: cast.visual.ID, Phase: event.SkillCastRelease, Aim: &center})
	}
	ids := make([]domain.EntityID, 0, len(cast.warned))
	for id := range cast.warned {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	finishEquipment := s.beginEquipmentRoot(m.ID, 0, s.equipmentOffense(m))
	defer finishEquipment()
	for _, id := range ids {
		// Reflection can kill the caster (and even complete the rift) on the
		// previous victim. Do not continue damaging other players afterwards.
		if !s.riftCombatReady() || !s.riftAbilitySourceReady(m) {
			break
		}
		p := s.players[id]
		if id == m.ID || !s.riftAbilityPlayerReady(p) || !cast.contains(p.Pos) || !s.staticPathWalkable(cast.center, p.Pos) {
			continue
		}
		tr := s.hits[m.ID]
		if tr == nil {
			tr = &combat.HitTracker{}
			s.hits[m.ID] = tr
		}
		// Readable, avoidable area mechanics cannot randomly miss once a player
		// remains inside. Use the stronger native attack channel so magic-only
		// templates also inherit their actual tier-scaled damage. Resistance,
		// shields, magic defense and reflection remain ordinary combat rules.
		magic := m.Stats.MAtk > m.Stats.MaxAtk
		hurtType := int32(domain.HurtPhysical)
		if magic {
			hurtType = domain.HurtFire
		}
		ev, raw, reflected := s.resolveStrikeStatuses(m, p, tr, combat.Blow{Area: true, Magic: magic,
			HurtType: hurtType, SkillID: int32(cast.visual.ID), SkillPct: cast.damagePct, GuaranteedHit: true})
		s.emit(ev)
		s.afterStrikeStatuses(m, p, ev, raw, reflected)
		s.consumeHitDurability(p, ev)
		s.emitAttributesIfPlayer(p)
		if cast.visual.HitEffect != "" && ev.Amount > 0 {
			s.emit(event.SkillHitEffect{Target: p.ID, Effect: cast.visual.HitEffect})
		}
		if ev.Flag.Has(event.DamageFatal) {
			s.onDeath(p, m.ID)
		}
	}
}

func (s *Scene) riftAbilityTelegraphs(now int64) []domain.EndgameRiftTelegraphView {
	if !s.riftCombatReady() {
		return nil
	}
	var out []domain.EndgameRiftTelegraphView
	for id, state := range s.rift.abilities {
		c := state.pending
		if c == nil || c.dueAt <= now || !s.riftAbilitySourceReady(s.monsters[id]) {
			continue
		}
		out = append(out, domain.EndgameRiftTelegraphView{ID: c.id, EntityID: strconv.FormatUint(uint64(id), 10),
			Ability: c.ability.name(s.currentRiftRules()), Shape: c.shape, X: int32(c.center.X), Y: int32(c.center.Y),
			EndX: int32(c.end.X), EndY: int32(c.end.Y), Radius: int32(c.radius), InnerRadius: int32(c.innerRadius), RemainingMS: int32(c.dueAt - now)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
