package scene

import (
	"fmt"
	"math"
	"sort"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

const (
	riftGoldenStatus  domain.StatusID = 1900101
	riftAgilityStatus domain.StatusID = 1900102
)

// This state is part of the run aggregate. Each successful cast owns its own
// deadline. Effect lifetime never occupies the original monster's action gate.
type endgameRiftEliteSnapshot struct {
	PoisonNext map[domain.EntityID]int64
	Sequence   uint64
	Casters    map[string]*endgameRiftEliteCaster
	Effects    map[string]*endgameRiftHazard
	Deaths     map[domain.EntityID]bool
	Clones     map[domain.EntityID]endgameRiftClone
	Walls      []temporaryWall
}
type endgameRiftEliteCaster struct {
	GoldenUntilMS int64
	Ready         map[int32]int64
	RecoverUntil  int64
}
type endgameRiftClone struct {
	Owner       domain.EntityID
	ExpiresAtMS int64
}

func (s *Scene) eliteRift() bool {
	return s.rift != nil && s.currentRiftRules() != nil && s.currentRiftRules().Elite != nil
}
func (s *Scene) eliteSnapshot() *endgameRiftEliteSnapshot {
	if s.rift.data.Elite == nil {
		s.rift.data.Elite = &endgameRiftEliteSnapshot{}
	}
	x := s.rift.data.Elite
	if x.PoisonNext == nil {
		x.PoisonNext = map[domain.EntityID]int64{}
	}
	if x.Casters == nil {
		x.Casters = map[string]*endgameRiftEliteCaster{}
	}
	if x.Effects == nil {
		x.Effects = map[string]*endgameRiftHazard{}
	}
	if x.Deaths == nil {
		x.Deaths = map[domain.EntityID]bool{}
	}
	if x.Clones == nil {
		x.Clones = map[domain.EntityID]endgameRiftClone{}
	}
	return x
}
func (s *Scene) eliteChanged() { s.rift.data.Run.Revision++; s.rift.lastBroadcast = s.riftMS() - 1000 }
func (s *Scene) registerRiftElite(m *entity.Entity, class int32, index int, seed uint64, floor int32) {
	rule := s.currentRiftRules()
	h := riftAbilitySeed(seed, class, index, s.rift.data.Run.Tier, floor)
	state := &endgameRiftAbilityState{class: class, eliteKey: fmt.Sprintf("%d/%d/%d", floor, class, index)}
	if class == 3 {
		for _, g := range rule.Elite.Guardians {
			if g.MonsterID == m.Monster.TypeID {
				for _, id := range g.Abilities {
					if !s.disabledRiftAbility(endgameRiftAbility(id)) {
						state.pattern = append(state.pattern, endgameRiftAbility(id))
					}
				}
			}
		}
	} else {
		for i := 0; i < int(class); i++ {
			a := endgameRiftAbility((int(h%8) + i*(1+2*int((h>>8)%4))) % 8)
			if !s.disabledRiftAbility(a) {
				state.pattern = append(state.pattern, a)
			}
		}
	}
	s.rift.abilities[m.ID] = state
	snapshot := s.eliteSnapshot()
	if snapshot.Casters[state.eliteKey] == nil {
		c := &endgameRiftEliteCaster{Ready: map[int32]int64{}}
		for _, id := range state.pattern {
			c.Ready[int32(id)] = s.riftMS() + rule.AbilityStartMS + int64(h>>16)%rule.AbilityJitterMS
		}
		snapshot.Casters[state.eliteKey] = c
	}
	for _, id := range state.pattern {
		if rule.Abilities[id].Trigger == "passive" {
			s.applyRiftEliteBuff(m, rule.Abilities[id])
		}
	}
}
func (s *Scene) applyRiftEliteBuff(m *entity.Entity, a domain.RiftAbilityRule) bool {
	switch a.Mechanism {
	case "golden_body":
		return s.applyStatusDef(m, domain.StatusDef{ID: riftGoldenStatus, Level: 1, Name: a.Name, Type: domain.StatusGood, TypeKnown: true, DurationSec: int32(a.DurationMS / 1000), PhysicalDamageReductionPct: a.DamageReductionPct, MagicDamageReductionPct: a.DamageReductionPct, DamageReductionCapPct: a.DamageReductionPct, BlockHarmful: true, SuppressHarmful: true}, m.ID)
	case "agility":
		return s.applyStatusDef(m, domain.StatusDef{ID: riftAgilityStatus, Level: 1, Name: a.Name, Type: domain.StatusGood, TypeKnown: true, Permanent: true, Affixes: []domain.Affix{{Attr: domain.AttrMoveSpeed, Mode: domain.ModePercent, Value: a.MoveSpeedPct}}}, m.ID)
	}
	return false
}
func (s *Scene) riftNativeCastBusy(m *entity.Entity) bool {
	if m.Monster.CastingSkill != 0 && s.tick < m.Monster.CastingUntil {
		return true
	}
	for _, c := range s.casts {
		if c.src == m.ID {
			return true
		}
	}
	return false
}
func (s *Scene) stepRiftElite(now int64) {
	x := s.eliteSnapshot()
	if s.rift.retired || s.rift.data.Run.Terminal() {
		s.clearRiftElite()
		return
	}
	if s.rift.transition != nil || s.rift.economicPending || s.rift.floorLoading {
		s.rift.eliteNeedsRearm = true
		return
	}
	if s.rift.eliteNeedsRearm {
		for _, h := range x.Effects {
			h.Viewers = map[domain.EntityID]int64{}
			h.ViewerGeneration = map[domain.EntityID]uint64{}
			h.Occupants = map[domain.EntityID]endgameRiftOccupant{}
		}
		x.PoisonNext = map[domain.EntityID]int64{}
		s.rift.eliteNeedsRearm = false
	}
	s.expireTemporaryWalls(now)
	x.Walls = s.temporaryWallSnapshot()
	s.stepRiftClones(now)
	// Effects are independent of source entities and survive guardian-arrival clearing.
	s.stepRiftHazards(now)
	if !s.riftCombatReady() {
		return
	}
	ids := make([]domain.EntityID, 0, len(s.rift.abilities))
	for id := range s.rift.abilities {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	started := 0
	for _, id := range ids {
		state := s.rift.abilities[id]
		m := s.monsters[id]
		if state == nil || !s.riftAbilitySourceReady(m) || s.riftNativeCastBusy(m) {
			continue
		}
		c := x.Casters[state.eliteKey]
		if c == nil || now < c.RecoverUntil {
			continue
		}
		if started >= int(s.currentRiftRules().AbilityMaxConcurrent) {
			break
		}
		for offset := 0; offset < len(state.pattern); offset++ {
			at := (state.cursor + offset) % len(state.pattern)
			a := s.currentRiftRules().Abilities[state.pattern[at]]
			if s.disabledRiftAbility(state.pattern[at]) || a.Trigger != "active" || now < c.Ready[a.ID] {
				continue
			}
			target := s.players[m.Monster.Target]
			if !s.riftAbilityPlayerReady(target) || entityInvisible(target) || riftDistance2(m.Pos, target.Pos) > math.Pow(float64(s.currentRiftRules().AbilityRange), 2) || !s.staticPathWalkable(m.Pos, target.Pos) {
				c.Ready[a.ID] = now + 1000
				continue
			}
			if !s.castRiftElite(m, target, a, now) {
				c.Ready[a.ID] = now + 1000
				s.eliteChanged()
				continue
			}
			if a.Mechanism == "golden_body" {
				c.GoldenUntilMS = now + a.DurationMS
			}
			c.Ready[a.ID] = now + a.CooldownMS
			c.RecoverUntil = now + s.currentRiftRules().AbilityRecoveryMS
			state.cursor = (at + 1) % len(state.pattern)
			started++
			s.eliteChanged()
			break
		}
	}
}
func (s *Scene) castRiftElite(m, target *entity.Entity, a domain.RiftAbilityRule, now int64) bool {
	switch a.Mechanism {
	case "golden_body":
		return s.applyRiftEliteBuff(m, a)
	case "clones":
		return s.spawnRiftClones(m, a, now)
	default:
		active := 0
		for _, h := range s.eliteSnapshot().Effects {
			if h.Ability.Trigger != "death" {
				active++
			}
		}
		if active >= int(s.currentRiftRules().Elite.HazardMaxConcurrent) {
			return false
		}
		return s.createRiftHazard(m, target.Pos, a, now)
	}
}
func (s *Scene) clearRiftElite() {
	if s.rift == nil {
		return
	}
	if x := s.rift.data.Elite; x != nil {
		for id := range x.Clones {
			s.removeRiftClone(id)
		}
		if len(x.Effects) > 0 || len(x.Walls) > 0 {
			s.eliteChanged()
		}
		x.Effects = map[string]*endgameRiftHazard{}
		x.Walls = nil
	}
	s.clearTemporaryWalls()
}

// Capture event-time offense before asynchronous economic reconciliation can
// replace the aggregate. Real deaths, despawns and cleanup are distinct paths.
func (s *Scene) captureRiftEliteDeath(m *entity.Entity, now int64) *endgameRiftHazard {
	if !s.eliteRift() || m == nil || m.Monster == nil || m.Alive() {
		return nil
	}
	x := s.eliteSnapshot()
	for id, c := range x.Clones {
		if c.Owner == m.ID {
			s.removeRiftClone(id)
		}
	}
	if m.Monster.Summoner != 0 || x.Deaths[m.ID] {
		return nil
	}
	registered := s.rift.data.Run.Monsters[m.ID]
	state := s.rift.abilities[m.ID]
	if registered == nil || registered.Dead || state == nil || s.rift.data.Run.Terminal() {
		return nil
	}
	for _, id := range state.pattern {
		a := s.currentRiftRules().Abilities[id]
		if a.Trigger == "death" {
			return &endgameRiftHazard{Source: m.ID, SourceLevel: m.Level, Attack: m.Stats, Pos: m.Pos, Ability: a, BornMS: now}
		}
	}
	return nil
}
func (s *Scene) applyRiftEliteDeath(death endgameRiftDeath) {
	h := death.EliteLava
	if h == nil || !s.eliteRift() || s.rift.data.Run.Terminal() {
		return
	}
	x := s.eliteSnapshot()
	if x.Deaths[death.ID] {
		return
	}
	x.Deaths[death.ID] = true
	// A delayed economic acknowledgement never shortens an unseen warning.
	// Freeze source values at death; start its three visible seconds at admission.
	m := &entity.Entity{ID: h.Source, Kind: domain.KindMonster, Level: h.SourceLevel, Stats: h.Attack, Pos: h.Pos}
	s.createRiftHazard(m, h.Pos, h.Ability, max(death.MS, s.riftMS()))
	s.eliteChanged()
}

func (s *Scene) riftMonsterAbilityIDs(id domain.EntityID) []int32 {
	if !s.eliteRift() || s.rift.abilities[id] == nil {
		return nil
	}
	var out []int32
	for _, a := range s.rift.abilities[id].pattern {
		if s.disabledRiftAbility(a) {
			continue
		}
		out = append(out, int32(a))
	}
	return out
}

// Retained for the existing wire contract; golden body is disabled in rifts.
func (s *Scene) riftGoldenUntil(id domain.EntityID, now int64) int64 { return 0 }
