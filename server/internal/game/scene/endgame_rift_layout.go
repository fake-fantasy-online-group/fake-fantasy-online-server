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
	return domain.PlanRiftFloor(runID, floor, s.riftLayouts, previousMap, rules.FloorPopulationMultiplier)
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
		def, ok := s.defs.Def(p.Point.Monster)
		if !ok || def.Sprite <= 0 || !def.Kind.Hostile() || def.NoAttack {
			fail()
			return
		}
		base := bases[int((plan.Seed+uint64(i))%uint64(len(bases)))]
		def.Level = base.Level
		def.HP = base.HP
		def.Stats = base.Stats
		def.AI = base.AI
		def.ViewDist, def.TraceDist = base.ViewDist, base.TraceDist
		def.Skills, def.ImmuneSkills = base.Skills, base.ImmuneSkills
		def.Aggressive, def.NoAttack, def.NoBasicAttack = true, false, base.NoBasicAttack
		def.CallHelp = base.CallHelp
		def.EliteID = 0
		lookup[def.ID] = def
		if boss {
			if x.data.Run.Rules.GuardianArrivalMS == 0 {
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
	s.defs = endgameRiftDefs{base: s.defs, trial: lookup}
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
