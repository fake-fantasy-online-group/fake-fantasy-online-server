package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
)

// Per-instance lookup preserves native trial skill/immune definitions without
// modifying shared global templates. Per-entity scaling lives separately.
type endgameRiftDefs struct {
	base  spawn.Defs
	trial map[domain.MonsterID]domain.MonsterDef
}

func (d endgameRiftDefs) Def(id domain.MonsterID) (domain.MonsterDef, bool) {
	if def, ok := d.trial[id]; ok {
		return def, true
	}
	if d.base != nil {
		return d.base.Def(id)
	}
	return domain.MonsterDef{}, false
}

type endgameRiftDeath struct {
	ID  domain.EntityID
	Pos domain.Pos
}

func (s *Scene) prepareEndgameRift(cmd Enter) bool {
	if s.router == nil {
		return true
	}
	d, paid := s.router.riftAdmission(s.id)
	if !paid {
		return true
	}
	if !s.router.ActiveRiftForCharacter(s.id, domain.CharID(cmd.Char.ID)) {
		return false
	}
	if s.rift == nil {
		s.initEndgameRift(d)
	}
	return true
}
func (s *Scene) initEndgameRift(d endgameRiftPersisted) {
	// Keep only the native gate geometry for authoritative floor admission. Its
	// original destination and script must never execute in this private run.
	portals := s.portals
	if s.rift != nil {
		portals = s.rift.portals
	}
	s.rift = &endgameRiftState{baseStats: map[domain.EntityID]domain.Stats{}, data: d, revision: 1, savedRevision: d.Run.Revision, ready: map[domain.CharID]bool{}, deathAt: map[domain.CharID]int64{}, loadUntil: s.riftMS() + 60000}
	s.rift.portals = portals
	for char, at := range d.DeathTimes {
		s.rift.deathAt[char] = at
	}
	if d.StorageRevision > 0 {
		s.rift.revision = d.StorageRevision
	}
	s.rift.floorLoading = d.Floor != nil && d.Run.Phase != domain.RiftReserved
	s.clearRiftMonsters()
	s.spawner = nil
	s.portals = nil
	// Reused wilderness/dungeon scenery must not carry public NPC services into
	// a private challenge, including quest claims, shops or scripted transports.
	s.skipNPC = true
	for id, e := range s.entities {
		if e.Kind == domain.KindNPC {
			s.aoi.Leave(e)
			delete(s.entities, id)
		}
	}
}
func (s *Scene) syncEndgameRift(p *entity.Entity) {
	s.recoverEndgameTrial(p)
	if s.rift == nil {
		return
	}
	char := domain.CharID(p.Player.Char.ID)
	if !s.rift.data.Run.Admitted(char) {
		return
	}
	s.rift.ready[char] = true
	s.rift.data.Run.Members[char].LastSeenMS = s.riftMS()
	s.replyLiveRift(p, domain.EndgameRequest{Op: "rift.state"})
}
func (s *Scene) disconnectEndgameRift(p *entity.Entity) {
	if s.rift != nil && p != nil && p.Player != nil {
		delete(s.rift.ready, domain.CharID(p.Player.Char.ID))
	}
}
func (s *Scene) clearRiftMonsters() {
	s.clearEndgameRiftAbilities()
	for id, m := range s.monsters {
		if s.rift != nil {
			delete(s.rift.baseStats, id)
			if registered := s.rift.data.Run.Monsters[id]; registered != nil {
				registered.Dead = true
			}
		}
		s.emit(event.EntityDespawned{ID: id, Kind: m.Kind, Reason: event.DespawnDeath})
		s.aoi.Leave(m)
		delete(s.entities, id)
		delete(s.monsters, id)
		delete(s.hits, id)
	}
}
func (s *Scene) populateEndgameRift(boss bool) {
	if s.rift.data.Floor != nil {
		s.populateSeededEndgameRift(boss)
		return
	}
	r := s.rift.data.Run
	level := int32(0)
	for _, m := range r.Members {
		if m.Level > level {
			level = m.Level
		}
	}
	lookup := map[domain.MonsterID]domain.MonsterDef{}
	for id, def := range s.trial.Monsters[level] {
		lookup[id] = def
	}
	if boss {
		for id, def := range s.trial.Monsters[level+s.trial.BossOffset] {
			if def.Kind == domain.MonsterBoss {
				lookup[id] = def
			}
		}
	}
	s.defs = endgameRiftDefs{base: s.defs, trial: lookup}
	var points []domain.TrialSpawn
	for _, p := range s.trial.Maps[s.id.MapID] {
		if len(p.Candidates) == 0 {
			continue
		}
		def := s.trial.Monsters[level][p.Candidates[0]]
		if (def.Kind == domain.MonsterBoss) == boss {
			points = append(points, p)
		}
	}
	if len(points) == 0 {
		r.Phase = domain.RiftFailed
		r.Reason = "spawn_failed"
		r.Revision++
		return
	}
	n := 104
	if boss {
		n = 1
	}
	for i := 0; i < n; i++ {
		point := points[i%len(points)]
		def, ok := lookup[point.Candidates[s.rng.Intn(len(point.Candidates))]]
		if !ok {
			continue
		}
		class := int32(0)
		if boss {
			class = 3
			def.Name = "镇境劫影"
			def.Kind = domain.MonsterBoss
		} else if i >= 98 {
			class = 2
			def.Kind = domain.MonsterElite
		} else if i >= 80 {
			class = 1
			def.Kind = domain.MonsterElite
		} else {
			def.Kind = domain.MonsterNormal
		}
		def.HP = domain.RiftScaledStat(def.HP, r.Tier, len(r.Members), class, true)
		def.Stats.MaxHP = def.HP
		def.Stats.MinAtk = domain.RiftScaledStat(def.Stats.MinAtk, r.Tier, len(r.Members), class, false)
		def.Stats.MaxAtk = domain.RiftScaledStat(def.Stats.MaxAtk, r.Tier, len(r.Members), class, false)
		def.Stats.MAtk = domain.RiftScaledStat(def.Stats.MAtk, r.Tier, len(r.Members), class, false)
		m := spawn.Encounter(s.alloc.Monster(), point.Point.Pos, uint8(point.Point.Dir), def)
		m.Monster.EliteRing = class == 1 || class == 2
		s.rift.baseStats[m.ID] = def.Stats
		s.initializeMonster(m)
		s.entities[m.ID] = m
		s.monsters[m.ID] = m
		s.aoi.Enter(m)
		r.RegisterMonster(m.ID, class)
		s.emit(s.spawnEvent(m))
	}
}
func (s *Scene) onEndgameRiftMonsterDeath(m, killer *entity.Entity) bool {
	if s.rift == nil {
		return false
	}
	x := s.rift
	if x.transition != nil || x.retired || x.floorLoading {
		return true
	}
	if x.economicPending {
		x.delayedDeaths = append(x.delayedDeaths, endgameRiftDeath{ID: m.ID, Pos: m.Pos})
		return true
	}
	present := map[domain.CharID]bool{}
	for _, p := range s.players {
		char := domain.CharID(p.Player.Char.ID)
		if x.ready[char] {
			present[char] = true
		}
	}
	x.data.Run.ObserveClock(s.riftMS(), present)
	before := x.data.Run.Phase
	orbs, ok := x.data.Run.RecordKill(m.ID, s.riftMS())
	if !ok {
		return true
	}
	if x.data.Run.Phase == domain.RiftCompleted && !x.data.GrantsReady {
		s.generateEndgameRiftGrants()
		x.data.Run.Revision++
	}
	for i := 0; i < orbs; i++ {
		id := s.alloc.Drop()
		pos := m.Pos
		pos.X += float64(i * 15)
		x.data.Run.Globes[id] = domain.RiftGlobe{ID: id, Pos: pos, Value: domain.RiftGlobeProgress}
	}
	if before == domain.RiftActive && x.data.Run.Phase == domain.RiftBoss {
		s.clearRiftMonsters()
		s.populateEndgameRift(true)
	}
	return true
}
func (s *Scene) onEndgameRiftPlayerDeath(p *entity.Entity) bool {
	if s.rift == nil {
		return false
	}
	id := domain.CharID(p.Player.Char.ID)
	if s.rift.economicPending {
		s.rift.delayedPlayerDeaths = append(s.rift.delayedPlayerDeaths, id)
	} else {
		s.rift.data.Run.RecordDeath(id)
	}
	s.rift.deathAt[id] = s.riftMS()
	return true
}
func (s *Scene) reviveEndgameRift(p *entity.Entity) bool {
	if s.rift == nil {
		return false
	}
	if !p.Alive() && s.riftMS()-s.rift.deathAt[domain.CharID(p.Player.Char.ID)] >= 5000 {
		p.Pos = s.dungeons[s.id.MapID].Enter
		if s.rift.data.Floor != nil {
			p.Pos = s.rift.data.Floor.Entry
		}
		s.doRevive(p, false)
	}
	return true
}
