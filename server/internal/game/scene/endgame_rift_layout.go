package scene

import (
	"errors"
	"sort"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
)

func (s *Scene) planEndgameRiftFloor(runID string, floor, previousMap int32) (domain.RiftFloorPlan, error) {
	rules := s.currentRiftRules()
	if rules == nil {
		return domain.RiftFloorPlan{}, errors.New("missing rift population rules")
	}
	var plan domain.RiftFloorPlan
	var err error
	if rules.FloorLayoutVersion == domain.RiftFloorDenseCoverageVersion {
		plan, err = domain.PlanRiftDenseCoverageFloor(runID, floor, s.riftLayouts, previousMap)
	} else if rules.FloorLayoutVersion == domain.RiftFloorCoverageVersion {
		plan, err = domain.PlanRiftCoverageFloor(runID, floor, s.riftLayouts, previousMap)
	} else {
		plan, err = domain.PlanRiftFloor(runID, floor, s.riftLayouts, previousMap, rules.FloorPopulationMultiplier)
	}
	if err != nil || rules.Elite == nil {
		return plan, err
	}
	// Guardian identity is independent of this map's ordinary fallback pool.
	// Check the approved whole catalog before admitting a paid run.
	for _, g := range rules.Elite.Guardians {
		d, ok := s.riftNativeDefinition(g.MonsterID)
		if !ok || d.Kind != domain.MonsterBoss || d.NoAttack || d.Sprite <= 0 || len(d.Skills) == 0 {
			return domain.RiftFloorPlan{}, errors.New("invalid approved guardian")
		}
	}
	selected := rules.Elite.Guardians[int(plan.Seed%uint64(len(rules.Elite.Guardians)))]
	plan.Guardian.Point.Monster = selected.MonsterID
	if rules.GuardianRecentWindow == 3 && s.rift != nil && s.rift.data.Run.RunID == runID {
		// A run owns one guardian, including after floor travel or reconnect.
		plan.Guardian.Point.Monster = s.rift.data.GuardianID
	}
	return plan, nil
}

func (s *Scene) validateEndgameRiftFloor(plan domain.RiftFloorPlan) error {
	if !plan.Valid() || s.id.MapID != plan.MapID || s.walkable == nil {
		return errors.New("invalid or unverified rift floor")
	}
	if !s.walkable(plan.Entry) || !s.walkable(plan.Exit) || !s.walkable(plan.Guardian.Point.Pos) {
		return errors.New("blocked rift entrance or exit")
	}
	for _, p := range plan.Spawns {
		if !s.walkable(p.Point.Pos) {
			return errors.New("blocked rift spawn")
		}
	}
	return nil
}

// Stat baselines come from the frozen roster's trial level, not from the natural
// level of a randomly chosen map. Existing asset IDs change appearance; the
// selected tier and party size remain the only additional difficulty scaling.
func (s *Scene) riftFloorBaseDefinitions(boss bool) []domain.MonsterDef {
	if s.trial == nil || s.rift == nil {
		return nil
	}
	level := int32(0)
	for _, m := range s.rift.data.Run.Members {
		if m.Level > level {
			level = m.Level
		}
	}
	defs := s.trial.Monsters[level]
	if boss && len(s.trial.Monsters[level+s.trial.BossOffset]) > 0 {
		defs = s.trial.Monsters[level+s.trial.BossOffset]
	}
	var out []domain.MonsterDef
	for _, d := range defs {
		if d.Kind.Hostile() && (d.Kind == domain.MonsterBoss) == boss && d.HP > 0 {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *Scene) populateSeededEndgameRift(boss bool) {
	x := s.rift
	plan := x.data.Floor
	fail := func() {
		x.data.Run.Phase = domain.RiftFailed
		x.data.Run.Reason = "spawn_failed"
		x.data.Run.Revision++
	}
	if plan == nil || s.validateEndgameRiftFloor(*plan) != nil || s.defs == nil {
		fail()
		return
	}
	bases := s.riftFloorBaseDefinitions(boss)
	if len(bases) == 0 {
		fail()
		return
	}
	points := plan.Spawns
	if boss {
		guardian := plan.Guardian
		if x.data.GuardianArrival != nil {
			guardian.Point.Pos = x.data.GuardianArrival.Pos
		}
		points = []domain.RiftFloorSpawn{guardian}
	}
	type prepared struct {
		point domain.RiftFloorSpawn
		def   domain.MonsterDef
	}
	population := make([]prepared, 0, len(points))
	lookup := map[domain.MonsterID]domain.MonsterDef{}
	// Validate the entire population before adding a single entity. An invalid
	// template cannot leave a partially playable, unwinnable paid instance.
	for i, p := range points {
		def, ok := s.riftNativeDefinition(p.Point.Monster)
		if boss && x.data.Run.Rules.Elite != nil {
			approved := false
			for _, g := range x.data.Run.Rules.Elite.Guardians {
				if g.MonsterID == p.Point.Monster {
					approved = true
				}
			}
			if !approved {
				fail()
				return
			}
			def, ok = s.riftNativeDefinition(p.Point.Monster)
			if !ok || def.Kind != domain.MonsterBoss || len(def.Skills) == 0 {
				fail()
				return
			}
		}
		if !ok || def.Sprite <= 0 || !def.Kind.Hostile() || def.NoAttack {
			fail()
			return
		}
		base := bases[int((plan.Seed+uint64(i))%uint64(len(bases)))]
		def.Level = base.Level
		def.HP = base.HP
		if boss && x.data.Run.Rules.Elite != nil {
			// Scale level/HP/attack separately; retain native defenses, movement,
			// original AI, spell choices, cast restrictions and immunity definitions.
			def.Stats.MaxHP = base.HP
			def.Stats.MinAtk, def.Stats.MaxAtk, def.Stats.MAtk = base.Stats.MinAtk, base.Stats.MaxAtk, base.Stats.MAtk
		} else {
			nativeSpeed := def.Stats.MoveSpeed
			def.Stats = base.Stats
			def.Stats.MoveSpeed = nativeSpeed
			def.Aggressive, def.NoAttack = true, false
		}
		def = domain.ChallengeMonsterBehavior(def)
		def.EliteID = 0
		lookup[def.ID] = def
		if boss {
			if x.data.Run.Rules.Elite == nil && x.data.Run.Rules.GuardianArrivalMS == 0 {
				def.Name = x.data.Run.Rules.BossName
			}
			def.Kind = domain.MonsterBoss
		} else if p.Class > 0 {
			def.Kind = domain.MonsterElite
		} else {
			def.Kind = domain.MonsterNormal
		}
		def.HP = domain.RiftScaledStat(def.HP, x.data.Run.Tier, len(x.data.Run.Members), p.Class, true, x.data.Run.Rules)
		def.Stats.MaxHP = def.HP
		def.Stats.MinAtk = domain.RiftScaledStat(def.Stats.MinAtk, x.data.Run.Tier, len(x.data.Run.Members), p.Class, false, x.data.Run.Rules)
		def.Stats.MaxAtk = domain.RiftScaledStat(def.Stats.MaxAtk, x.data.Run.Tier, len(x.data.Run.Members), p.Class, false, x.data.Run.Rules)
		def.Stats.MAtk = domain.RiftScaledStat(def.Stats.MAtk, x.data.Run.Tier, len(x.data.Run.Members), p.Class, false, x.data.Run.Rules)
		population = append(population, prepared{p, def})
	}
	s.defs = endgameRiftDefs{base: s.defs, trial: lookup, scaled: true}
	for i, p := range population {
		m := spawn.Encounter(s.alloc.Monster(), p.point.Point.Pos, uint8(p.point.Point.Dir), p.def)
		x.originalNames[m.ID] = lookup[p.def.ID].Name
		m.Monster.EliteRing = p.point.Class == 1 || p.point.Class == 2
		x.baseStats[m.ID] = p.def.Stats
		s.initializeMonster(m)
		s.registerEndgameRiftAbilities(m, p.point.Class, i, plan.Seed, plan.FloorIndex)
		s.entities[m.ID] = m
		s.monsters[m.ID] = m
		s.aoi.Enter(m)
		x.data.Run.RegisterMonster(m.ID, p.point.Class)
		s.emit(s.spawnEvent(m))
	}
}

// Resolve the native catalog entry, skipping generated encounter overrides.
// A catalog may itself be layered (or have no base), so the wrapper type alone
// cannot tell us whether its entries are original or instance-local scaled data.
func (s *Scene) riftNativeDefinition(id domain.MonsterID) (domain.MonsterDef, bool) {
	defs := s.defs
	for {
		overlay, ok := defs.(endgameRiftDefs)
		if !ok {
			break
		}
		if !overlay.scaled {
			if def, exists := overlay.trial[id]; exists {
				return def, true
			}
		}
		defs = overlay.base
	}
	if defs == nil {
		return domain.MonsterDef{}, false
	}
	return defs.Def(id)
}
