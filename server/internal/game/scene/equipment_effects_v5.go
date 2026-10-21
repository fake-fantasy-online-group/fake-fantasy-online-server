package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"slices"
)

type equipmentCached struct {
	worn            *domain.EquipSet
	revision        uint64
	base            domain.Base
	level           int32
	race            domain.Race
	employed        bool
	employmentKnown bool
	gender          uint8
	effects         []domain.ActiveEquipmentEffect
	traits          []domain.UniqueTrait
	stats           domain.EquipmentCombatStats
}
type equipmentAbsorbHeal = domain.EquipmentAbsorbRestore
type equipmentHit struct {
	src, dst domain.EntityID
	blow     combat.Blow
	ev       event.DamageDealt
}
type equipmentRoot struct {
	classSetDamageScaleBP int32
	classSetHealingStatus bool
	source, currentSource *domain.CombatSourceSnapshotV5
	attackStats           domain.Stats
	attackSnapshot        bool
	id                    uint64
	src, primary          domain.EntityID
	offense               domain.EquipmentCombatStats
	hits                  []equipmentHit
}
type equipmentRuntime struct {
	unruly            map[domain.EntityID]classSetUnruly
	classSetCasters   map[domain.EntityID]*classSetCasterState
	classSetTriggered int
	variantMends      map[domain.EntityID]variantMend
	observations      map[domain.EntityID][32]byte
	cache             map[domain.EntityID]equipmentCached
	absorption        map[domain.EntityID][]equipmentAbsorbHeal
	root              *equipmentRoot
	sequence          uint64
	auraSources       []equipmentAuraSource
	auraSchedules     map[equipmentAuraSource]equipmentAuraSchedule
	auras             map[domain.EntityID]map[string]equipmentAuraGrant
	nextAuraScan      domain.Tick
	suppressedProcs   uint64
}

func (s *Scene) equipmentState() *equipmentRuntime {
	if s.equipment == nil {
		s.equipment = &equipmentRuntime{cache: map[domain.EntityID]equipmentCached{}, absorption: map[domain.EntityID][]equipmentAbsorbHeal{}, auras: map[domain.EntityID]map[string]equipmentAuraGrant{}}
	}
	return s.equipment
}
func (s *Scene) equipmentEffects(p *entity.Entity) equipmentCached {
	if p == nil || p.Player == nil || p.Player.Char == nil {
		return equipmentCached{}
	}
	rt := s.equipmentState()
	ch := p.Player.Char
	worn := p.Player.Worn
	revision := worn.EffectsRevision()
	if old, ok := rt.cache[p.ID]; ok && old.worn == worn && old.revision == revision && old.base == ch.EffectiveBase() && old.level == ch.Level && old.race == ch.Race && old.employed == ch.Employed && old.employmentKnown == ch.EmploymentKnown && old.gender == ch.Appear.Gender {
		return old
	}
	out := equipmentCached{worn: worn, revision: revision, base: ch.EffectiveBase(), level: ch.Level, race: ch.Race, employed: ch.Employed, employmentKnown: ch.EmploymentKnown, gender: ch.Appear.Gender}
	functional := s.functionalWorn(p)
	out.effects = domain.ActiveEquipmentEffects(functional, s.itemDef, ch)
	out.traits = append(domain.ActiveUniqueTraits(functional, s.itemDef, ch), domain.AstralTraits(functional, s.itemDef, ch)...)
	out.stats = domain.AggregateEquipmentCombat(out.effects)
	rt.cache[p.ID] = out
	return out
}
func (s *Scene) equipmentOffense(p *entity.Entity) domain.EquipmentCombatStats {
	return s.equipmentEffects(p).stats
}
func (s *Scene) equipmentOwner(e *entity.Entity) domain.EntityID {
	if e == nil {
		return 0
	}
	if e.Player != nil {
		return e.ID
	}
	if e.Pet != nil {
		return e.Pet.Owner
	}
	if e.Trap != nil {
		return e.Trap.Owner
	}
	if e.Monster != nil && e.Monster.Summoner != 0 {
		if p := s.entities[e.Monster.Summoner]; p != nil && p.Player != nil {
			return p.ID
		}
	}
	return 0
}
func (s *Scene) equipmentPVP(a, b *entity.Entity) bool {
	x, y := s.equipmentOwner(a), s.equipmentOwner(b)
	return x != 0 && y != 0 && x != y
}
func (s *Scene) equipmentDefense(p *entity.Entity, pvp bool) domain.EquipmentCombatStats {
	out := s.equipmentEffects(p).stats
	for _, grant := range s.equipmentState().auras[p.ID] {
		if !s.equipmentAuraGrantValid(p, grant) {
			continue
		}
		a := grant.effect
		v := a.Value
		if pvp {
			v = int32(int64(v) * int64(a.PVPScaleBP) / 10000)
		}
		switch a.Modifier {
		case "physical_mitigation_bp":
			out.PhysicalMitigationBP = clampI32(int64(out.PhysicalMitigationBP) + int64(v))
		case "element_damage_bp":
			i := domain.ElementIndex(a.Element)
			if i >= 0 {
				out.Elements[i].DamageBP = clampI32(int64(out.Elements[i].DamageBP) + int64(v))
			}
		case "element_resist_bp":
			i := domain.ElementIndex(a.Element)
			if i >= 0 {
				out.Elements[i].ResistBP = clampI32(int64(out.Elements[i].ResistBP) + int64(v))
			}
		}
	}
	return out
}
func (s *Scene) beginEquipmentRoot(src, primary domain.EntityID, offense domain.EquipmentCombatStats) func() {
	rt := s.equipmentState()
	prev := rt.root
	rt.sequence++
	root := &equipmentRoot{id: rt.sequence, src: src, primary: primary, offense: offense}
	rt.root = root
	return func() { rt.root = prev; s.executeEquipmentRoot(root) }
}

// equipmentAttackStats preserves the accepted source stats for every direct
// payload, including HP/MP conversions that do not produce a combat.Blow.
func (s *Scene) equipmentAttackStats(src *entity.Entity) domain.Stats {
	if root := s.equipmentState().root; root != nil && root.src == src.ID && root.attackSnapshot {
		return root.attackStats
	}
	return src.Stats
}

func (s *Scene) equipmentBlow(src *entity.Entity, blow combat.Blow) combat.Blow {
	rt := s.equipmentState()
	if blow.SourceOffense == nil {
		blow.SourceOffense, blow.CurrentSourceOffense = s.combatSourceV5(src)
		if rt.root == nil || rt.root.source == nil {
			blow.SourceOffense.Invisible = blow.StartedInvisible || entityInvisible(src)
		}
	}
	if rt.root != nil {
		if !blow.AttackStatsSnapshot && rt.root.src == src.ID && rt.root.attackSnapshot {
			blow.AttackStats = rt.root.attackStats
			blow.AttackStatsSnapshot = true
		}
		blow.RootID = rt.root.id
		if !blow.EquipmentSnapshot && rt.root.src == src.ID {
			blow.EquipmentOffense = rt.root.offense
			blow.EquipmentSnapshot = true
		}
	}
	if !blow.EquipmentSnapshot {
		blow.EquipmentOffense = s.equipmentOffense(src)
		blow.EquipmentSnapshot = true
	}
	// Live aura contributions are resolved after the target/PvP context is known.
	return blow
}
func (s *Scene) collectEquipmentHit(src, dst *entity.Entity, blow combat.Blow, ev event.DamageDealt) {
	if blow.Origin != domain.DamageNormalOrigin || blow.ProcDepth != 0 || blow.Trap || src.Kind == domain.KindPet || (src.Monster != nil && src.Monster.Summoner != 0) {
		return
	}
	if !s.equipmentLegalDirectPair(src, dst) {
		return
	}
	rt := s.equipmentState()
	if rt.root != nil {
		if len(rt.root.hits) < 256 {
			rt.root.hits = append(rt.root.hits, equipmentHit{src.ID, dst.ID, blow, ev})
		} else {
			rt.suppressedProcs++
			s.log.Warn("equipment root hit budget exceeded", "root", rt.root.id)
		}
	}
}
func (s *Scene) equipmentAbsorb(dst *entity.Entity, damage, absorbBP int32, pvp bool) domain.ElementAbsorbResult {
	rt := s.equipmentState()
	now := s.now().UnixMilli()
	old := rt.absorption[dst.ID]
	if dst.Player != nil {
		old = dst.Player.EquipmentAbsorbRestores
	}
	kept := old[:0]
	used := int64(0)
	for _, h := range old {
		if h.AtMS > now-1000 {
			kept = append(kept, h)
			used += int64(h.Amount)
		}
	}
	cap := int64(dst.MaxHP) * 2 / 100
	if pvp {
		cap = int64(dst.MaxHP) / 100
	}
	result := domain.ResolveElementAbsorb(dst.HP, dst.MaxHP, damage, absorbBP, clampI32(max(int64(0), cap-used)), pvp)
	if result.Heal > 0 {
		kept = append(kept, equipmentAbsorbHeal{AtMS: now, Amount: result.Heal})
	}
	if dst.Player != nil {
		dst.Player.EquipmentAbsorbRestores = kept
	} else {
		rt.absorption[dst.ID] = kept
	}
	return result
}
func sortedEquipmentEntities[T any](m map[domain.EntityID]T) []domain.EntityID {
	ids := make([]domain.EntityID, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

func (s *Scene) equipmentCombatReady(e *entity.Entity) bool {
	if e == nil {
		return false
	}
	if !s.requireGrowthV5 {
		return true
	}
	owner := s.equipmentOwner(e)
	if owner == 0 {
		return true
	}
	p := s.players[owner]
	return p != nil && p.Player.EndgameGrowthV5
}

func (s *Scene) equipmentLegalDirectPair(a, b *entity.Entity) bool {
	if a == nil || b == nil || a.ID == b.ID {
		return false
	}
	if a.Player != nil {
		return s.equipmentProcEnemyTarget(a, b)
	}
	return a.Kind == domain.KindMonster && a.Monster != nil && a.Monster.Kind.Hostile() && a.Monster.Summoner == 0 && b.Player != nil
}

func (s *Scene) equipmentAbsorptionEligible(src, dst *entity.Entity, b combat.Blow) bool {
	if src == nil || dst == nil || src.ID == dst.ID || b.Origin != domain.DamageNormalOrigin || b.ProcDepth != 0 {
		return false
	}
	a, d := s.equipmentOwner(src), s.equipmentOwner(dst)
	if a != 0 && a == d {
		return false
	}
	if src.Player != nil && dst.Player != nil && sameParty(src, dst) {
		return false
	}
	return true
}
