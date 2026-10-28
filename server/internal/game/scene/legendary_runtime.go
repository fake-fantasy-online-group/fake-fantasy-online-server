package scene

import (
	"fmt"
	"hash/fnv"
	"sort"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/combat"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// Explicit typed handlers deliberately cannot execute design prose or Lua.
type legendaryHandler struct {
	// AllDamageIncoming opts a pure BeforeIncoming modifier into secondary damage.
	// It never enables AfterIncoming, reflection, or other proc generators.
	AllDamageIncoming     bool
	ModifyAppearance      func(*legendaryContext, *domain.Appearance)
	StatusAccuracy        func(*legendaryContext, *domain.StatusDef)
	IncomingHeal          func(*legendaryContext, int32) int32
	ModifyBlow            func(*legendaryContext, *combat.Blow)
	EquipmentChanged      func(*legendaryContext)
	AbsorbIncoming        func(*legendaryContext, int32) int32
	TheftAttempt          func(*legendaryContext)
	PeriodicResolved      func(*legendaryContext, domain.Active)
	MagicResisted         func(*legendaryContext, int32) int32
	ModifyIncoming        func(*legendaryContext, int32) int32
	ShieldDamageBP        func(*legendaryContext) int32
	OwnerPoisonDeath      func(*legendaryContext, domain.Active, *entity.Entity)
	DisplacementImmune    func(*legendaryContext) bool
	StatusCommitted       func(*legendaryContext, *domain.StatusDef)
	PassiveStats          func(*legendaryContext, *domain.Stats)
	StatusReceiving       func(*legendaryContext, *domain.StatusDef) bool
	TrapPlaced            func(*legendaryContext, *entity.Entity, bool)
	revision              string
	ModifyDamage          func(*legendaryContext, int32) int32
	SkillResolved         func(*legendaryContext)
	HealResolved          func(*legendaryContext, int32, int32, int32)
	NativeRestoreAmount   func(*legendaryContext, int32, []equipmentHit) int32
	NativeRestoreResolved func(*legendaryContext, int32, int32)
	TransformSkill        func(*legendaryContext, *domain.SkillDef)
	BaseSkill             func(*legendaryContext, *domain.SkillDef)
	ComposeSkill          func(*legendaryContext, *domain.SkillDef) // Completed buckets, before final scaling.
	ImpactTargets         func(*legendaryContext, []domain.EntityID) []domain.EntityID
	CastAccepted          func(*legendaryContext)
	Resurrected           func(*legendaryContext)
	SelectTargets         func(*legendaryContext, []domain.EntityID) []domain.EntityID
	TargetDefinition      func(*legendaryContext, *domain.SkillDef)
	Moved                 func(*legendaryContext, domain.Pos, domain.Pos, bool, bool)
	Teleported            func(*legendaryContext, domain.Pos)
	Healed                func(*legendaryContext, int32, int32)
	ManaRestored          func(*legendaryContext, int32, int32)
	Dispelled             func(*legendaryContext, []domain.Active)
	StatusApplied         func(*legendaryContext, *domain.StatusDef)
	PrepareSkill          func(*legendaryContext, *domain.SkillDef)
	BeforeDamage          func(*legendaryContext, int32) int32
	BeforeIncoming        func(*legendaryContext, int32) int32
	AfterHit              func(*legendaryContext)
	AfterIncoming         func(*legendaryContext)
	CastReleased          func(*legendaryContext)
	RootFinished          func(*legendaryContext)
	Tick                  func(*legendaryContext)
}

var legendaryHandlers = map[string]legendaryHandler{}

// Revised registrations are the only implementation of rewritten powers.
// Owners remove their superseded handlers; duplicate registrations are errors.
func registerLegendarySynergyHandler(id string, h legendaryHandler) {
	h.revision = "synergy-v1"
	registerLegendaryHandler(id, h)
}
func legendaryHandlerCurrent(id string) bool {
	p, ok := domain.LegendaryPower(id)
	return ok && p.DesignStatus != "draft" && (p.DesignDecision == "keep" || legendaryHandlers[id].revision == p.DesignRevision)
}

func registerLegendaryHandler(id string, h legendaryHandler) {
	if _, ok := legendaryHandlers[id]; ok {
		panic("duplicate legendary handler " + id)
	}
	if h.ModifyAppearance == nil && h.StatusAccuracy == nil && h.IncomingHeal == nil && h.ModifyBlow == nil && h.EquipmentChanged == nil && h.AbsorbIncoming == nil && h.TheftAttempt == nil && h.PeriodicResolved == nil && h.MagicResisted == nil && h.ModifyIncoming == nil && h.ShieldDamageBP == nil && h.OwnerPoisonDeath == nil && h.DisplacementImmune == nil && h.StatusCommitted == nil && h.PassiveStats == nil && h.StatusReceiving == nil && h.TrapPlaced == nil && h.ModifyDamage == nil && h.SkillResolved == nil && h.HealResolved == nil && h.NativeRestoreAmount == nil && h.NativeRestoreResolved == nil && h.TransformSkill == nil && h.BaseSkill == nil && h.ComposeSkill == nil && h.ImpactTargets == nil && h.CastAccepted == nil && h.Resurrected == nil && h.SelectTargets == nil && h.TargetDefinition == nil && h.Moved == nil && h.Teleported == nil && h.Healed == nil && h.ManaRestored == nil && h.Dispelled == nil && h.StatusApplied == nil && h.PrepareSkill == nil && h.BeforeDamage == nil && h.BeforeIncoming == nil && h.AfterHit == nil && h.AfterIncoming == nil && h.CastReleased == nil && h.RootFinished == nil && h.Tick == nil {
		panic("empty legendary handler " + id)
	}
	legendaryHandlers[id] = bindLegendaryEquipmentHandlers(id, h)
}

type legendaryContext struct {
	PreserveControlSkill                    domain.SkillID
	CausalPath                              []string
	PresentationStage                       string // Optional impact art, emitted only after actual payload resolution.
	Preview                                 bool
	IsBasic, IsChild                        bool
	SourceSnapshot                          *domain.CombatSourceSnapshotV5
	CastPosition                            domain.Pos
	OriginalTargets                         []domain.EntityID
	Scene                                   *Scene
	Owner, Target                           *entity.Entity
	Power                                   domain.LegendaryPowerDefinition
	PowerRollBP                             int32
	SourceUID                               int64
	Skill                                   domain.SkillID
	Def                                     domain.SkillDef
	RootID                                  uint64
	Primary                                 domain.EntityID
	Center                                  domain.Pos
	Targets                                 []domain.EntityID
	Blow                                    combat.Blow
	Hit                                     event.DamageDealt
	Hits                                    []equipmentHit
	NowMS                                   int64
	RawBeforeShield, Absorbed               int32
	BeforeHP, PreviousHP, LostHP, ManaSpent int32
	PaidMP                                  int32
}
type legendaryStateKey struct {
	Owner  domain.EntityID
	UID    int64
	Power  string
	Target domain.EntityID
	Phase  string
}
type legendaryPowerState struct {
	GroundEffectKey   string // Explicit current-state presentation; never transferred across maps.
	VisualStage       string
	ValueIsTimestamp  bool
	NextMS            int64
	Stacks            int32
	Value             int64
	ExpiresMS         int64
	RootID            uint64
	Position          domain.Pos
	Name, Description string
	Unit              string
	IconID            int32
	Good              bool
}
type legendaryRefundKey struct {
	Owner domain.EntityID
	Root  uint64
}
type legendaryRefund struct {
	Amount int32
	AtMS   int64
}
type legendaryRuntime struct {
	causalPath            []string
	permanentSyncing      bool
	nativeDamageSnapshots map[uint64]*combat.NativeDamageSnapshot
	equipmentSignature    map[domain.EntityID]string
	presentation          map[domain.EntityID]string
	refunds               map[legendaryRefundKey]legendaryRefund
	states                map[legendaryStateKey]*legendaryPowerState
	cooldowns             map[legendaryStateKey]int64
	child                 *legendaryChildFrame
	depth                 int
}

func (s *Scene) legendaryState() *legendaryRuntime {
	rt := s.equipmentState()
	if rt.legendary == nil {
		rt.legendary = &legendaryRuntime{states: map[legendaryStateKey]*legendaryPowerState{}, cooldowns: map[legendaryStateKey]int64{}}
	}
	return rt.legendary
}
func (c *legendaryContext) key(target domain.EntityID, phase string) legendaryStateKey {
	return legendaryStateKey{c.Owner.ID, c.SourceUID, c.Power.ID, target, phase}
}

// Expired states remain readable for natural-expiration mechanics. Tick runs
// handlers before cleanup. Refresh semantics are owned by the approved handler.
func (c *legendaryContext) State(target domain.EntityID, phase string) *legendaryPowerState {
	if c.Preview {
		if c.Scene.equipment != nil && c.Scene.equipment.legendary != nil {
			if st := c.Scene.equipment.legendary.states[c.key(target, phase)]; st != nil {
				copy := *st
				return &copy
			}
		}
		return &legendaryPowerState{}
	}
	rt := c.Scene.legendaryState()
	key := c.key(target, phase)
	st := rt.states[key]
	if st == nil {
		st = &legendaryPowerState{}
		rt.states[key] = st
	}
	return st
}
func (c *legendaryContext) DeleteState(target domain.EntityID, phase string) {
	if c.Preview {
		return
	}
	key := c.key(target, phase)
	c.Scene.legendaryStopStateGroundEffect(key, c.Scene.legendaryState().states[key])
	delete(c.Scene.legendaryState().states, key)
}
func (c *legendaryContext) Ready(cooldownMS int64) bool {
	if c.Preview {
		return false
	}
	rt := c.Scene.legendaryState()
	key := c.key(0, "cooldown")
	expiry, known := rt.cooldowns[key]
	persistent := fmt.Sprintf("legendary:%d:%s", c.SourceUID, c.Power.ID)
	ch := c.Owner.Player.Char
	if !known && ch.EquipmentProcCooldowns[persistent] > c.Scene.now().UnixMilli() {
		expiry = c.NowMS + ch.EquipmentProcCooldowns[persistent] - c.Scene.now().UnixMilli()
		rt.cooldowns[key] = expiry
	}
	if expiry > c.NowMS {
		return false
	}
	if cooldownMS > 0 {
		rt.cooldowns[key] = c.NowMS + cooldownMS
		if ch.EquipmentProcCooldowns == nil {
			ch.EquipmentProcCooldowns = map[string]int64{}
		}
		ch.EquipmentProcCooldowns[persistent] = c.Scene.now().UnixMilli() + cooldownMS
		c.Owner.Player.MarkDirty()
	}
	return true
}

func (s *Scene) legendaryContexts(owner, target *entity.Entity, blow combat.Blow, hit event.DamageDealt) []legendaryContext {
	return s.legendaryContextsFor(owner, target, blow, hit, false)
}
func (s *Scene) legendaryContextsFor(owner, target *entity.Entity, blow combat.Blow, hit event.DamageDealt, modifiersOnly bool) []legendaryContext {
	return s.legendaryContextsMode(owner, target, blow, hit, modifiersOnly, false)
}
func (s *Scene) legendaryContextsMode(owner, target *entity.Entity, blow combat.Blow, hit event.DamageDealt, modifiersOnly, defensive bool) []legendaryContext {
	if owner == nil || owner.Player == nil || owner.Player.Char == nil || !owner.Alive() || s.entityMapLoading(owner) {
		return nil
	}
	acceptedSource := blow.SourceOffense
	if defensive || hit.Dst == owner.ID && hit.Src != owner.ID {
		acceptedSource = nil
	}
	if root := s.equipmentState().root; !defensive && acceptedSource == nil && root != nil && root.src == owner.ID {
		acceptedSource = root.source
	}
	active := domain.ActiveLegendaryPowers(s.functionalWorn(owner), s.itemDef, owner.Player.Char)
	out := make([]legendaryContext, 0, len(active))
	for _, p := range active {
		if !modifiersOnly && s.combatEffectActive(owner.ID, p.Power.ID) {
			continue
		}
		if acceptedSource != nil && owner.ID != hit.Dst {
			found := false
			for _, accepted := range acceptedSource.LegendaryPowers {
				if accepted.SourceUID == p.SourceUID && accepted.Power.ID == p.Power.ID {
					p.PowerRollBP = accepted.PowerRollBP
					p.Power, _ = domain.LegendaryPowerForRoll(p.Power.ID, p.PowerRollBP)
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if _, ok := legendaryHandlers[p.Power.ID]; !ok || !legendaryHandlerCurrent(p.Power.ID) {
			continue
		}
		c := legendaryContext{CausalPath: s.legendaryState().causalPath, IsBasic: legendaryNativeBasic(blow), IsChild: blow.LegendaryChild, CastPosition: owner.Pos, Scene: s, Owner: owner, Target: target, Power: p.Power, PowerRollBP: p.PowerRollBP, SourceUID: p.SourceUID, Skill: domain.SkillID(blow.SkillID), Blow: blow, Hit: hit, RootID: blow.RootID, NowMS: s.tick.Millis()}
		if child := s.legendaryState().child; child != nil {
			c.PreserveControlSkill = child.PreserveControlSkill
		}
		if acceptedSource != nil {
			c.CastPosition = acceptedSource.Position
			c.SourceSnapshot = acceptedSource
		}
		if rt := s.equipmentState().root; rt != nil {
			c.RootID = rt.id
			c.Primary = rt.primary
			c.Hits = rt.hits
			c.PaidMP = rt.paidMP
			c.ManaSpent = rt.paidMP
			c.Def = rt.def
			c.OriginalTargets = rt.originalTargets
			c.Targets = rt.targets
			c.Center = rt.center
		}
		if blow.PrimaryTarget != 0 {
			c.Primary = blow.PrimaryTarget
		}
		if blow.LegendaryChild {
			if child := s.legendaryState().child; child != nil {
				c.Primary = child.Primary
				c.Center = child.Center
			}
		}
		if blow.EffectiveSkillID != 0 {
			c.Skill = domain.SkillID(blow.EffectiveSkillID)
		}
		out = append(out, c)
	}
	return out
}
func (s *Scene) legendaryPrepareSkill(owner *entity.Entity, def *domain.SkillDef, targets ...domain.EntityID) {
	s.legendaryPrepareSkillSources(owner, def, nil, false, targets...)
}
func (s *Scene) legendaryPrepareSkillSources(owner *entity.Entity, def *domain.SkillDef, accepted []domain.LegendarySourceRef, bounded bool, targets ...domain.EntityID) {
	s.legendaryComposeSkill(owner, def, accepted, bounded, false, targets...)
}

// Preview runs only pure transform/base/modifier phases, never release or hit events.
func (s *Scene) legendaryPreviewSkill(owner *entity.Entity, def *domain.SkillDef) {
	s.legendaryComposeSkill(owner, def, nil, false, true)
}
func (s *Scene) legendaryComposeSkill(owner *entity.Entity, def *domain.SkillDef, accepted []domain.LegendarySourceRef, bounded, preview bool, targets ...domain.EntityID) {
	var target *entity.Entity
	if len(targets) > 0 {
		target = s.entities[targets[0]]
	}
	if def == nil {
		return
	}
	var contexts []legendaryContext
	if preview && owner != nil && owner.Player != nil && owner.Player.Char != nil {
		for _, p := range domain.ActiveLegendaryPowers(s.functionalWorn(owner), s.itemDef, owner.Player.Char) {
			if legendaryHandlerCurrent(p.Power.ID) {
				contexts = append(contexts, legendaryContext{Preview: true, Scene: s, Owner: owner, Target: target, Power: p.Power, PowerRollBP: p.PowerRollBP, SourceUID: p.SourceUID, NowMS: s.tick.Millis()})
			}
		}
	} else {
		contexts = s.legendaryContextsFor(owner, target, combat.Blow{SkillID: int32(def.ID)}, event.DamageDealt{}, true)
	}
	if bounded {
		filtered := contexts[:0]
		for _, c := range contexts {
			for _, source := range accepted {
				if source.PowerID == c.Power.ID && source.SourceUID == c.SourceUID {
					c.PowerRollBP = source.PowerRollBP
					c.Power, _ = domain.LegendaryPowerForRoll(c.Power.ID, c.PowerRollBP)
					filtered = append(filtered, c)
					break
				}
			}
		}
		contexts = filtered
	}
	legendaryOrderSkillContexts(contexts)
	var sources []domain.LegendarySourceRef
	for _, c := range contexts {
		sources = append(sources, domain.LegendarySourceRef{PowerID: c.Power.ID, SourceUID: c.SourceUID, PowerRollBP: c.PowerRollBP})
	}

	// Complete each stage for every item before proceeding. Skill replacement
	// and base additions must not depend on equipment or power-ID ordering.
	for stage := 0; stage < 4; stage++ {
		for _, c := range contexts {
			c.Skill, c.Def = def.ID, *def
			c.IsBasic = def.LegendaryBasic
			h := legendaryHandlers[c.Power.ID]
			fn := h.TransformSkill
			if stage == 1 {
				fn = h.BaseSkill
			}
			if stage == 2 {
				fn = h.PrepareSkill
			}
			if stage == 3 {
				fn = h.ComposeSkill
			}
			if fn != nil {
				fn(&c, def)
			}
		}
	}
	def.LegendarySources = sources
	legendaryCasterHolyStatusBonus(owner, def)
	s.legendaryFinalizeSkill(def)
	previewGrowthRounding(owner, def)
}
func (s *Scene) legendaryCastReleased(owner *entity.Entity, def domain.SkillDef, primary domain.EntityID, targets []domain.EntityID, paid ...int32) uint64 {
	center := owner.Pos
	if def.Area.Center != domain.SkillAreaCenterCaster {
		if target := s.entities[primary]; target != nil {
			center = target.Pos
		}
	}
	payment := int32(0)
	if len(paid) > 0 {
		payment = paid[0]
	}
	return s.legendaryCastReleasedAt(owner, def, primary, targets, payment, center)
}
func (s *Scene) legendaryCastReleasedAt(owner *entity.Entity, def domain.SkillDef, primary domain.EntityID, targets []domain.EntityID, paid int32, center domain.Pos, snapshots ...*domain.CombatSourceSnapshotV5) uint64 {
	rt := s.equipmentState()
	rt.sequence++
	rootID := rt.sequence
	var accepted *domain.CombatSourceSnapshotV5
	if len(snapshots) > 0 {
		accepted = snapshots[0]
	}
	for _, c := range s.legendaryContexts(owner, s.entities[primary], combat.Blow{SkillID: int32(def.ID), SourceOffense: accepted}, event.DamageDealt{}) {
		c.RootID = rootID
		c.Def = def
		c.PaidMP, c.ManaSpent = paid, paid
		c.Primary = primary
		c.Targets = append([]domain.EntityID(nil), targets...)
		c.Center = center
		if h := legendaryHandlers[c.Power.ID].CastReleased; h != nil {
			c.invoke(h)
		}
	}
	s.pushStatusSnapshots(owner)
	return rootID
}
func (s *Scene) legendaryBeforeDamage(src, dst *entity.Entity, blow combat.Blow, hit event.DamageDealt, amount int32, incoming bool) int32 {
	if !incoming {
		for _, c := range s.legendaryContextsFor(src, dst, blow, hit, true) {
			c.BeforeHP = dst.HP
			c.PreviousHP = dst.HP
			if child := s.legendaryState().child; child != nil {
				c.Def = child.Def
				c.Skill = child.Def.ID
				c.IsChild = true
			}
			if h := legendaryHandlers[c.Power.ID].ModifyDamage; h != nil {
				amount = max(int32(0), h(&c, amount))
			}
		}
	}
	if incoming {
		contexts := s.legendaryIncomingContexts(src, dst, blow, hit)
		base := amount
		delta := int64(0)
		for _, c := range contexts {
			c.BeforeHP = dst.HP
			c.PreviousHP = dst.HP
			if h := legendaryHandlers[c.Power.ID].ModifyIncoming; h != nil {
				delta += int64(h(&c, base)) - int64(base)
			}
		}
		amount = max(int32(0), clampI32(int64(base)+delta))
		amount = legendaryAllDamageIncoming(contexts, amount)
		{
			// Causal producer identity, not generated origin, bounds reactions.
			for _, c := range s.legendaryContextsFor(dst, src, blow, hit, true) {
				c.BeforeHP, c.PreviousHP = dst.HP, dst.HP
				h := legendaryHandlers[c.Power.ID]
				if !h.AllDamageIncoming && h.BeforeIncoming != nil {
					c.invokeModifier(func(c *legendaryContext) { amount = max(int32(0), h.BeforeIncoming(c, amount)) })
				}
			}
		}
		// Mana substitutes for already mitigated damage, not the original hit.
		// All pure reductions must finish before an absorption spends resources.
		for _, c := range contexts {
			if h := legendaryHandlers[c.Power.ID].AbsorbIncoming; h != nil {
				amount = max(int32(0), h(&c, amount))
			}
		}
		return amount
	}
	for _, c := range s.legendaryContextsFor(src, dst, blow, hit, true) {
		c.BeforeHP = dst.HP
		c.PreviousHP = dst.HP
		h := legendaryHandlers[c.Power.ID].BeforeDamage
		if h != nil {
			c.invokeModifier(func(c *legendaryContext) { amount = max(int32(0), h(c, amount)) })
		}
	}
	return amount
}
func (s *Scene) legendaryAfterHit(src, dst *entity.Entity, blow combat.Blow, hit event.DamageDealt, beforeHP int32, shieldMetrics ...int32) {
	if hit.Flag.Has(event.DamageMiss) {
		return
	}
	for _, c := range s.legendaryContexts(src, dst, blow, hit) {
		c.BeforeHP = beforeHP
		c.PreviousHP = beforeHP
		c.LostHP = min(beforeHP, hit.Amount)
		if len(shieldMetrics) > 1 {
			c.RawBeforeShield = shieldMetrics[0]
			c.Absorbed = shieldMetrics[1]
		}
		if h := legendaryHandlers[c.Power.ID].AfterHit; h != nil {
			c.invoke(h)
		}
	}
	for _, c := range s.legendaryContexts(dst, src, blow, hit) {
		c.BeforeHP = beforeHP
		c.PreviousHP = beforeHP
		c.LostHP = min(beforeHP, hit.Amount)
		if len(shieldMetrics) > 1 {
			c.RawBeforeShield = shieldMetrics[0]
			c.Absorbed = shieldMetrics[1]
		}
		if h := legendaryHandlers[c.Power.ID].AfterIncoming; h != nil {
			c.invoke(h)
		}
	}
}

type legendaryDamagePayload struct {
	Context        legendaryContext
	Target         *entity.Entity
	Amount         int32
	Magic, Settled bool
}

func (s *Scene) legendaryRootFinished(root *equipmentRoot) {
	if root == nil {
		return
	}
	if !root.generated {
		defer delete(s.legendaryState().nativeDamageSnapshots, root.id)
	}
	s.flushLegendaryBasicLeech(root)
	for _, v := range root.legendaryDamage {
		v.Context.Damage(v.Target, v.Amount, v.Magic, v.Settled)
	}
	owner := s.entities[root.src]
	var blow combat.Blow
	if len(root.hits) > 0 {
		blow = root.hits[0].blow
	}
	if root.def.ID != 0 {
		if root.def.LegendaryBasic {
			blow.SkillID = 0
			blow.EffectiveSkillID = int32(root.def.ID)
		} else {
			blow.SkillID = int32(root.def.ID)
		}
	}
	blow.SourceOffense = root.source
	blow.RootID = root.id
	for _, c := range s.legendaryContexts(owner, s.entities[root.primary], blow, event.DamageDealt{}) {
		c.RootID = root.id
		c.Hits = root.hits
		c.PaidMP = root.paidMP
		c.ManaSpent = root.paidMP
		c.Def = root.def
		c.OriginalTargets = root.originalTargets
		c.Targets = root.targets
		c.Center = root.center
		c.Primary = root.primary
		if h := legendaryHandlers[c.Power.ID].RootFinished; h != nil {
			c.invoke(h)
		}
	}
}
func (s *Scene) stepLegendaryPowers() {
	if len(s.players) == 0 {
		return
	}
	rt := s.legendaryState()
	now := s.tick.Millis()
	valid := map[legendaryStateKey]bool{}
	ids := make([]domain.EntityID, 0, len(s.players))
	for id := range s.players {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		owner := s.players[id]
		s.syncLegendaryPermanentBuffs(owner)
		for _, c := range s.legendaryContexts(owner, nil, combat.Blow{}, event.DamageDealt{}) {
			valid[c.key(0, "")] = true
			if h := legendaryHandlers[c.Power.ID].Tick; h != nil {
				c.invoke(h)
			}
		}
	}
	for k, st := range rt.states {
		base := k
		base.Target = 0
		base.Phase = ""
		if !valid[base] || st.ExpiresMS > 0 && st.ExpiresMS <= now {
			s.legendaryStopStateGroundEffect(k, st)
			delete(rt.states, k)
		}
	}
	for k, expiry := range rt.cooldowns {
		if expiry <= now {
			// Keep the local expired marker so deterministic simulation time is not
			// reset by a wall-clock persistence fallback before the next activation.
			rt.cooldowns[k] = 0
		}
	}
	s.publishLegendaryStates()

}
func (s *Scene) clearLegendaryOwner(id domain.EntityID) {
	if s.equipment == nil || s.equipment.legendary == nil {
		return
	}
	for rootID, snapshot := range s.equipment.legendary.nativeDamageSnapshots {
		if snapshot.Owner == id {
			delete(s.equipment.legendary.nativeDamageSnapshots, rootID)
		}
	}
	for k := range s.equipment.legendary.states {
		if k.Owner == id || k.Target == id {
			s.legendaryStopStateGroundEffect(k, s.equipment.legendary.states[k])
			delete(s.equipment.legendary.states, k)
		}
	}
}
func (c *legendaryContext) Attack(magic bool) int32 {
	stats := c.Scene.equipmentAttackStats(c.Owner)
	if c.SourceSnapshot != nil {
		stats = c.SourceSnapshot.Stats
	}
	if magic {
		return stats.MAtk
	}
	return clampI32((int64(stats.MinAtk) + int64(stats.MaxAtk)) / 2)
}
func (c *legendaryContext) Enemies(center domain.Pos, radius float64, limit int) []*entity.Entity {
	if radius < 0 || limit < 0 {
		return nil
	}
	var out []*entity.Entity
	for _, e := range c.Scene.entities {
		if e.Alive() && e.Pos.MapID == center.MapID && c.Scene.equipmentProcEnemyTarget(c.Owner, e) && sqDist(center, e.Pos) <= radius*radius {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := sqDist(center, out[i].Pos), sqDist(center, out[j].Pos)
		if a != b {
			return a < b
		}
		return out[i].ID < out[j].ID
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// Damage cannot recursively trigger powers, crit, or borrow client values.
// Settled echoes deliberately do not re-run armor, outgoing or incoming
// multipliers. Their copied amount has already crossed those stages.
func (c *legendaryContext) Damage(target *entity.Entity, amount int32, magic, settled bool) {
	if c.Preview {
		return
	}
	s := c.Scene
	if amount <= 0 || target == nil || !target.Alive() || !s.equipmentProcEnemyTarget(c.Owner, target) || gmGodEnabled(target) {
		return
	}
	if root := s.equipmentState().root; root != nil && root.id == c.RootID {
		// The source root and proc-depth guards bound generation; target count
		// must not silently discard otherwise eligible area victims.
		root.legendaryDamage = append(root.legendaryDamage, legendaryDamagePayload{*c, target, amount, magic, settled})
		return
	}
	rt := s.legendaryState()
	defer s.withCombatCausalPath(c.effectPath())()
	rt.depth++
	defer func() { rt.depth-- }()
	beforeHP := target.HP
	hurt := int32(domain.HurtPhysical)
	if magic {
		hurt = c.Blow.HurtType
		if hurt == domain.HurtNone || hurt == domain.HurtPhysical {
			hurt = c.Def.HurtType
		}
		if hurt == domain.HurtPhysical {
			hurt = domain.HurtNone
		}
	}
	var ev event.DamageDealt
	var reflected int32
	rawDamage := amount
	if settled {
		if target.Status != nil {
			if target.Status.Invulnerable() {
				return
			}
			var removed []domain.StatusID
			amount, removed = target.Status.AbsorbDamage(amount)
			s.statusesRemoved(target, removed)
		}
		target.HP = max(int32(0), target.HP-amount)
		ev = event.DamageDealt{Src: c.Owner.ID, Dst: target.ID, Amount: amount, DstHP: target.HP, SkillID: int32(c.Skill), Origin: domain.DamageProcOrigin, RootID: c.RootID, ProcDepth: 1}
		if magic {
			ev.Flag |= event.DamageMagic
		}
		if target.HP == 0 {
			ev.Flag |= event.DamageFatal
		}
	} else {
		blow := combat.Blow{Origin: domain.DamageProcOrigin, ProcDepth: 1, RootID: c.RootID, SkillID: int32(c.Skill), LegendaryPayload: true, PrimaryTarget: c.Primary, AttackStatsSnapshot: true, AttackStats: domain.Stats{MinAtk: amount, MaxAtk: amount, MAtk: amount}, SkillPct: 100, GuaranteedHit: true, Magic: magic, HurtType: hurt, PayloadScaleKnown: true, PayloadScaleBP: 10000}
		blow.SourceSkillRange = c.Blow.SourceSkillRange
		blow.SourceTrap = c.Blow.Trap || c.Blow.SourceTrap
		if c.Skill != 0 && c.Def.ID == c.Skill {
			blow.SourceSkillRange = c.Def.Dist
		}
		ev, rawDamage, reflected = s.resolveStrikeStatuses(c.Owner, target, &combat.HitTracker{}, blow)
	}
	if settled {
		s.collectEquipmentHit(c.Owner, target, combat.Blow{Origin: domain.DamageProcOrigin, ProcDepth: 1, SkillID: int32(c.Skill), RootID: c.RootID, Magic: magic, LegendaryPayload: true, PrimaryTarget: c.Primary}, ev)
		s.afterUniqueStrike(c.Owner, target, ev)
		s.legendaryAfterHit(c.Owner, target, combat.Blow{Origin: domain.DamageProcOrigin, ProcDepth: 1, SkillID: int32(c.Skill), RootID: c.RootID, Magic: magic, LegendaryPayload: true, PrimaryTarget: c.Primary}, ev, beforeHP)
	}
	s.emit(ev)
	s.afterStrikeStatuses(c.Owner, target, ev, rawDamage, reflected, strikeReactionContext{HurtType: hurt, PreservedControlSkill: c.PreserveControlSkill})
	s.emitAttributesIfPlayer(target)
	s.aggroAfterHit(target, c.Owner.ID, ev)
	s.legendaryCasterDamageResolved(c, target, ev)
	if c.PresentationStage != "" && !ev.Flag.Has(event.DamageMiss) {
		c.Visual(c.PresentationStage, target)
	}
	if ev.Flag.Has(event.DamageFatal) {
		s.onDeath(target, c.Owner.ID)
	}
}
func (s *Scene) legendaryStatusPresentation(e *entity.Entity) []statusPresentation {
	if e == nil || s.equipment == nil || s.equipment.legendary == nil {
		return nil
	}
	var out []statusPresentation
	for k, st := range s.equipment.legendary.states {
		st = legendaryPresentationState(k, st)
		target := k.Target
		if target == 0 {
			target = k.Owner
		}
		if target != e.ID || st.Name == "" || st.ExpiresMS > 0 && st.ExpiresMS <= s.tick.Millis() {
			continue
		}
		name := st.Name
		unit := st.Unit
		if unit == "" {
			unit = "层"
		}
		if st.Stacks > 1 {
			name = fmt.Sprintf("%s（%d%s）", name, st.Stacks, unit)
		}
		_, desc, icon, _ := legendaryStatePresentation(k.Power, k.Phase)
		if st.Description != "" {
			desc = st.Description
		}
		if st.IconID > 0 {
			icon = st.IconID
		}
		remaining := int32(1<<31 - 1)
		if st.ExpiresMS > 0 {
			remaining = nonnegativeI32(st.ExpiresMS - s.tick.Millis())
		}
		h := fnv.New32a()
		fmt.Fprintf(h, "%d:%s:%s", k.UID, k.Power, k.Phase)
		id := int32(2200000 + h.Sum32()%1000000)
		out = append(out, statusPresentation{name: name, desc: desc, icon: event.StatusIconView{Status: id, Icon: icon, Beneficial: st.Good, RemainMS: remaining}, buff: event.BuffView{SkillID: id, Icon: icon, Name: name, Desc: desc, Beneficial: st.Good, RemainSec: int32((int64(remaining) + 999) / 1000)}})
		if st.VisualStage != "" {
			if fx, mode, ok := legendaryVisualEffect(k.Power, st.VisualStage); ok {
				out[len(out)-1].effect = event.EntityStatusEffect{Who: e.ID, Effect: fx, PositionMode: uint8(mode), DurationMS: remaining}
				out[len(out)-1].buff.PositionMode = uint8(mode)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].icon.Status < out[j].icon.Status })
	return out
}

func (s *Scene) forEachLegendary(owner *entity.Entity, fn func(*legendaryContext)) {
	for _, c := range s.legendaryContextsFor(owner, nil, combat.Blow{}, event.DamageDealt{}, true) {
		fn(&c)
	}
}
func (c *legendaryContext) States(phase string) map[domain.EntityID]*legendaryPowerState {
	out := map[domain.EntityID]*legendaryPowerState{}
	for k, v := range c.Scene.legendaryState().states {
		if k.Owner == c.Owner.ID && k.UID == c.SourceUID && k.Power == c.Power.ID && k.Phase == phase {
			if c.Preview {
				copy := *v
				out[k.Target] = &copy
			} else {
				out[k.Target] = v
			}
		}
	}
	return out
}

// Certification is separate from handler presence: a partial implementation is
// testable but must not enter natural loot. Call only after whole-power checks.
func certifyLegendaryPower(id string) {
	if _, ok := legendaryHandlers[id]; !ok || !legendaryHandlerCurrent(id) {
		panic("unregistered or superseded legendary power " + id)
	}
	domain.RegisterLegendaryPowerExecutable(id)
}

// RefundMP returns only effective mana, with a shared per-cast paid-cost ceiling
// across every equipped power. Power resource restores never call heal hooks.
func (c *legendaryContext) RefundMP(amount int32) int32 {
	if c.Preview {
		return 0
	}
	if amount <= 0 || c.PaidMP <= 0 || c.RootID == 0 || c.Owner == nil || !c.Owner.Alive() {
		return 0
	}
	rt := c.Scene.legendaryState()
	if rt.refunds == nil {
		rt.refunds = map[legendaryRefundKey]legendaryRefund{}
	}
	key := legendaryRefundKey{c.Owner.ID, c.RootID}
	record := rt.refunds[key]
	amount = min(amount, max(int32(0), c.PaidMP-record.Amount), max(int32(0), c.Owner.MaxMP-c.Owner.MP))
	if amount <= 0 {
		return 0
	}
	c.Owner.MP += amount
	record.Amount += amount
	record.AtMS = c.NowMS
	rt.refunds[key] = record
	c.Scene.emitAttributesIfPlayer(c.Owner)
	for k, v := range rt.refunds {
		if v.AtMS < c.NowMS-60000 {
			delete(rt.refunds, k)
		}
	}
	return amount
}

// Visual stages are invoked explicitly by the mechanic which owns that phase;
// a catalog entry never invents an activation or changes combat timing.
func (c *legendaryContext) Visual(stage string, target *entity.Entity) bool {
	if c.Preview {
		return false
	}
	if target == nil {
		return false
	}
	effect, _, ok := legendaryVisualEffect(c.Power.ID, stage)
	if !ok {
		return false
	}
	c.Scene.emit(event.SkillHitEffect{Target: target.ID, Effect: effect})
	return true
}

func (s *Scene) publishLegendaryStates() {
	rt := s.legendaryState()
	if rt.presentation == nil {
		rt.presentation = map[domain.EntityID]string{}
	}
	recipients := map[domain.EntityID]bool{}
	for id := range rt.presentation {
		recipients[id] = true
	}
	for k, st := range rt.states {
		st = legendaryPresentationState(k, st)
		if st.Name != "" {
			id := k.Target
			if id == 0 {
				id = k.Owner
			}
			recipients[id] = true
		}
	}
	for id := range recipients {
		e := s.entities[id]
		if e == nil {
			delete(rt.presentation, id)
			continue
		}
		views := s.legendaryStatusPresentation(e)
		signature := ""
		for _, v := range views {
			signature += fmt.Sprintf("%d:%s:%s:%d:%d;", v.icon.Status, v.name, v.desc, v.icon.Icon, v.buff.RemainSec)
		}
		if old, ok := rt.presentation[id]; !ok || old != signature {
			rt.presentation[id] = signature
			s.pushStatusSnapshots(e)
		}
		if signature == "" {
			delete(rt.presentation, id)
		}
	}
}

// Called at equipment recomputation as well as the simulation boundary so
// unequipping cannot preserve a source-bound grant for another attack.
func (s *Scene) pruneLegendaryOwner(owner *entity.Entity) {
	if owner == nil || owner.Player == nil || s.equipment == nil || s.equipment.legendary == nil {
		return
	}
	active := domain.ActiveLegendaryPowers(s.functionalWorn(owner), s.itemDef, owner.Player.Char)
	valid := map[string]int64{}
	for _, a := range active {
		valid[a.Power.ID] = a.SourceUID
	}
	for k := range s.equipment.legendary.states {
		if k.Owner == owner.ID && valid[k.Power] != k.UID {
			s.legendaryStopStateGroundEffect(k, s.equipment.legendary.states[k])
			delete(s.equipment.legendary.states, k)
		}
	}
	for _, target := range s.entities {
		if target.Status == nil {
			continue
		}
		removed := target.Status.RemoveWhere(func(d domain.StatusDef) bool {
			return (target.ID == owner.ID || d.DamageShield > 0 && d.DurationSec <= 0) && d.LegendaryPowerID != "" && d.LegendarySourceUID != 0 && valid[d.LegendaryPowerID] != d.LegendarySourceUID && legendaryStatusOwnedBy(target, d, owner.ID)
		})
		if len(removed) > 0 {
			s.statusesRemoved(target, removed)
			if target != owner {
				s.refreshEntityStats(target)
			}
			s.pushStatusSnapshots(target)
		}
	}
}
func legendaryStatusOwnedBy(target *entity.Entity, d domain.StatusDef, owner domain.EntityID) bool {
	owned := false
	target.Status.Each(func(a domain.Active) {
		if a.Def.ID == d.ID && a.Source == owner {
			owned = true
		}
	})
	return owned
}

func legendaryPresentationState(k legendaryStateKey, st *legendaryPowerState) *legendaryPowerState {
	if name, desc, icon, good, ok := legendaryCasterStatePresentation(k, st); ok {
		copy := *st
		copy.Name = name
		copy.Description = desc
		copy.IconID = icon
		copy.Good = good
		return &copy
	}
	return st
}
func (s *Scene) legendarySelectTargets(owner *entity.Entity, def domain.SkillDef, primary domain.EntityID, targets []domain.EntityID) []domain.EntityID {
	originals := map[domain.EntityID]bool{}
	for _, id := range targets {
		originals[id] = true
	}
	for _, c := range s.legendaryContexts(owner, s.entities[primary], combat.Blow{SkillID: int32(def.ID)}, event.DamageDealt{}) {
		c.Def = def
		c.IsBasic = def.LegendaryBasic
		c.Primary = primary
		c.Targets = append([]domain.EntityID(nil), targets...)
		if h := legendaryHandlers[c.Power.ID].SelectTargets; h != nil {
			targets = h(&c, append([]domain.EntityID(nil), targets...))
		}
	}
	// Preserving a selected target is not permission to extend cast range.
	// Keep it first when the effect promises primary-first settlement.
	if primaryTarget := s.entities[primary]; def.LegendaryPreservePrimary && primaryTarget != nil &&
		s.validTarget(owner, primaryTarget, def) &&
		(!def.HasDamageEffect() || s.equipmentProcEnemyTarget(owner, primaryTarget)) {
		targets = append([]domain.EntityID{primary}, targets...)
	}
	out := make([]domain.EntityID, 0, len(targets))
	seen := map[domain.EntityID]bool{}
	for _, id := range targets {
		if seen[id] {
			continue
		}
		e := s.entities[id]
		if e == nil || !originals[id] && (!e.Alive() && !def.HasResurrectEffect() || !s.validTarget(owner, e, def) ||
			id == primary && def.Dist > 0 && sqDist(owner.Pos, e.Pos) > float64(def.Dist)*float64(def.Dist)) {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}
func (s *Scene) legendaryTargetDefinition(owner *entity.Entity, def domain.SkillDef, target *entity.Entity, primary domain.EntityID, targets []domain.EntityID) domain.SkillDef {
	for _, c := range s.legendaryContexts(owner, target, combat.Blow{SkillID: int32(def.ID)}, event.DamageDealt{}) {
		c.Def = def
		c.IsBasic = def.LegendaryBasic
		c.Primary = primary
		c.Targets = targets
		if h := legendaryHandlers[c.Power.ID].TargetDefinition; h != nil {
			h(&c, &def)
		}
	}
	return def
}

func (s *Scene) legendaryMitigationNeeded(e *entity.Entity) bool {
	if e == nil {
		return false
	}
	if e.Player != nil && len(domain.ActiveLegendaryPowers(s.functionalWorn(e), s.itemDef, e.Player.Char)) > 0 {
		return true
	}
	found := false
	if e.Status != nil {
		e.Status.Each(func(a domain.Active) {
			if a.Def.LegendaryPowerID != "" {
				found = true
			}
		})
	}
	return found
}

func (c *legendaryContext) DamageTyped(target *entity.Entity, amount, hurtType int32, settled bool) {
	copy := *c
	copy.Blow.HurtType = hurtType
	copy.Def.HurtType = hurtType
	copy.Damage(target, amount, hurtType != domain.HurtNone && hurtType != domain.HurtPhysical, settled)
}

// VisualAt is a fixed world coordinate effect; it never follows an actor.
// Old clients receive no new packet until the GroundFX capability is negotiated.
func (c *legendaryContext) VisualAt(stage string, pos domain.Pos, durationMS int32) bool {
	if c.Preview {
		return false
	}
	effect, _, ok := legendaryVisualEffect(c.Power.ID, stage)
	if !ok || c.Owner == nil || pos.MapID != c.Owner.Pos.MapID {
		return false
	}
	mode := uint8(0)
	if durationMS > 0 {
		mode = 1
	}
	c.Scene.emit(event.GroundEffect{Owner: c.Owner.ID, MapID: pos.MapID, Pos: pos, Key: fmt.Sprintf("%s/%d/%s", c.Power.ID, c.RootID, stage), Effect: effect, DurationMS: max(int32(0), durationMS), Mode: mode})
	return true
}

// Class replacement/geometry precedes universal final-shape transformations.
func legendaryOrderSkillContexts(contexts []legendaryContext) {
	sort.SliceStable(contexts, func(i, j int) bool {
		return contexts[i].Power.Class != "全职业" && contexts[j].Power.Class == "全职业"
	})
}
