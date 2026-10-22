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
	// scaled distinguishes generated encounter overrides from native catalog
	// layers. Native resolution must skip only generated values, not catalogs.
	scaled bool
	base   spawn.Defs
	trial  map[domain.MonsterID]domain.MonsterDef
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

// Delayed economic writes must preserve authoritative event time and ordering.
// CharID identifies a player death (or an offline personal-context update);
// otherwise ID/Pos identify a monster death. Replay preserves their event order.
type endgameRiftDeath struct {
	EliteLava       *endgameRiftHazard
	ObservedContext *endgameRiftRewardContext
	RewardContexts  map[domain.CharID]endgameRiftRewardContext
	ID              domain.EntityID
	Pos             domain.Pos
	MS              int64
	CharID          domain.CharID
	Present         map[domain.CharID]bool
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
	if riftNeedsPages(d.Run.Rules) && (cmd.Runtime == nil || !cmd.Runtime.EndgameRiftPages) {
		return false
	}
	if s.rift == nil {
		if !s.initEndgameRift(d) {
			return false
		}
	}
	return true
}
func (s *Scene) initEndgameRift(d endgameRiftPersisted) bool {
	if d.Run == nil || d.Run.BindRules(s.riftRules) != nil {
		return false
	}
	// Keep only the native gate geometry for authoritative floor admission. Its
	// original destination and script must never execute in this private run.
	portals := s.portals
	if s.riftPortalAreas != nil {
		portals = make([]domain.Portal, 0, len(s.riftPortalAreas))
		for _, area := range s.riftPortalAreas {
			portals = append(portals, domain.NewPortal(domain.Teleport{Area: area}))
		}
	}
	if s.rift != nil {
		portals = s.rift.portals
	}
	s.rift = &endgameRiftState{baseStats: map[domain.EntityID]domain.Stats{}, originalNames: map[domain.EntityID]string{}, data: d, revision: 1, savedRevision: d.Run.Revision, ready: map[domain.CharID]bool{}, deathAt: map[domain.CharID]int64{}, loadUntil: s.riftMS() + 60000}
	s.rift.portals = portals
	for char, at := range d.DeathTimes {
		s.rift.deathAt[char] = at
	}
	if d.StorageRevision > 0 {
		s.rift.revision = d.StorageRevision
	}
	s.rift.floorLoading = d.Floor != nil && d.Run.Phase != domain.RiftReserved
	s.clearRiftMonsters()
	s.restoreRiftElite()
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
	return true
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
	s.syncEndgameRiftCompletion()
	s.replyLiveRift(p, domain.EndgameRequest{Op: "rift.state"})
}
func (s *Scene) disconnectEndgameRift(p *entity.Entity) {
	if s.rift != nil && p != nil && p.Player != nil {
		char := domain.CharID(p.Player.Char.ID)
		if !s.rift.data.GrantsReady && (s.rift.data.Run.Rules.NormalBossEquipment > 0 || s.rift.data.Run.Rules.CompletionLevelExperience) && s.rift.data.Run.Admitted(char) {
			context := endgameRiftRewardContext{Level: p.Player.Char.Level, MagicFind: s.magicFindV5(p)}
			s.cacheEndgameRiftRewardContext(char, context)
			if transition := s.rift.transition; transition != nil && !s.rift.retired {
				if transition.rewardContexts == nil {
					transition.rewardContexts = map[domain.CharID]endgameRiftRewardContext{}
				}
				transition.rewardContexts[char] = context
			}
			if s.rift.economicPending {
				// Another member's pending aggregate can replace data with its earlier
				// clone, including after a lost ACK. Replay this observation before any
				// subsequently queued guardian death; the immediate cache also makes
				// that death's own event-time snapshot correct.
				s.rift.delayedDeaths = append(s.rift.delayedDeaths, endgameRiftDeath{CharID: char, ObservedContext: &context})
			}
		}
		delete(s.rift.ready, char)
		s.forgetRiftHazardPlayer(p.ID)
	}
}
func (s *Scene) cacheEndgameRiftRewardContext(char domain.CharID, context endgameRiftRewardContext) {
	cacheRiftRewardContext(&s.rift.data, char, context)
}

func cacheRiftRewardContext(data *endgameRiftPersisted, char domain.CharID, context endgameRiftRewardContext) bool {
	if data.GrantsReady {
		return false
	}
	if data.Profiles == nil {
		data.Profiles = map[domain.CharID]endgameRiftIdentity{}
	}
	profile := data.Profiles[char]
	if profile.RewardLevel == context.Level && profile.MagicFind == context.MagicFind {
		return false
	}
	profile.RewardLevel, profile.MagicFind = context.Level, context.MagicFind
	data.Profiles[char] = profile
	data.Run.Revision++
	return true
}

func (s *Scene) clearRiftMonsters() {
	s.clearEndgameRiftAbilities()
	for id, m := range s.monsters {
		s.releaseMonsterCarriedLoot(m)
		s.cancelPendingCombat(id)
		if s.rift != nil {
			delete(s.rift.baseStats, id)
			delete(s.rift.originalNames, id)
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
	if s.rift.data.Run.Phase == domain.RiftBossArriving {
		return
	}
	if boss && !s.endgameRiftGuardianCanPopulate() {
		return
	}
	if s.rift.data.Floor != nil {
		s.populateSeededEndgameRift(boss)
		return
	}
	r := s.rift.data.Run
	if r.Rules.Elite != nil {
		r.Phase = domain.RiftFailed
		r.Reason = "spawn_failed"
		r.Revision++
		return
	}
	level := int32(0)
	for _, m := range r.Members {
		if m.Level > level {
			level = m.Level
		}
	}
	lookup := map[domain.MonsterID]domain.MonsterDef{}
	for id, def := range s.trial.Monsters[level] {
		lookup[id] = domain.ChallengeMonsterBehavior(def)
	}
	if boss {
		for id, def := range s.trial.Monsters[level+s.trial.BossOffset] {
			if def.Kind == domain.MonsterBoss {
				lookup[id] = domain.ChallengeMonsterBehavior(def)
			}
		}
	}
	s.defs = endgameRiftDefs{base: s.defs, trial: lookup, scaled: true}
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
		originalName := def.Name
		class := int32(0)
		if boss {
			class = 3
			if r.Rules.GuardianArrivalMS == 0 {
				def.Name = r.Rules.BossName
			}
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
		def.HP = domain.RiftScaledStat(def.HP, r.Tier, len(r.Members), class, true, r.Rules)
		def.Stats.MaxHP = def.HP
		def.Stats.MinAtk = domain.RiftScaledStat(def.Stats.MinAtk, r.Tier, len(r.Members), class, false, r.Rules)
		def.Stats.MaxAtk = domain.RiftScaledStat(def.Stats.MaxAtk, r.Tier, len(r.Members), class, false, r.Rules)
		def.Stats.MAtk = domain.RiftScaledStat(def.Stats.MAtk, r.Tier, len(r.Members), class, false, r.Rules)
		pos := point.Point.Pos
		if boss && s.rift.data.GuardianArrival != nil {
			pos = s.rift.data.GuardianArrival.Pos
		}
		m := spawn.Encounter(s.alloc.Monster(), pos, uint8(point.Point.Dir), def)
		s.rift.originalNames[m.ID] = originalName
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
	present := map[domain.CharID]bool{}
	for _, p := range s.players {
		char := domain.CharID(p.Player.Char.ID)
		if x.ready[char] {
			present[char] = true
		}
	}
	death := endgameRiftDeath{ID: m.ID, Pos: m.Pos, MS: s.riftMS(), Present: present, EliteLava: s.captureRiftEliteDeath(m, s.riftMS())}
	if registered := x.data.Run.Monsters[m.ID]; registered != nil && registered.Class == 3 {
		death.RewardContexts = s.freezeEndgameRiftRewardContexts()
	}
	if x.economicPending {
		x.delayedDeaths = append(x.delayedDeaths, death)
		return true
	}
	return s.applyEndgameRiftDeath(death)
}

func (s *Scene) applyEndgameRiftDeath(death endgameRiftDeath) bool {
	x := s.rift
	if death.ObservedContext != nil {
		s.cacheEndgameRiftRewardContext(death.CharID, *death.ObservedContext)
		return true
	}
	if death.CharID != 0 {
		x.data.Run.RecordDeath(death.CharID)
		return true
	}
	s.applyRiftEliteDeath(death)
	x.data.Run.ObserveClock(death.MS, death.Present)
	before := x.data.Run.Phase
	orbs, ok := x.data.Run.RecordKill(death.ID, death.MS)
	if !ok {
		return true
	}
	if x.data.Run.Phase == domain.RiftCompleted && !x.data.GrantsReady {
		x.data.RewardPos = death.Pos
		x.rewardFresh = true
		s.generateEndgameRiftGrantsWithContexts(death.RewardContexts)
		x.data.Run.Revision++
	}
	for i := 0; i < orbs; i++ {
		id := s.alloc.Drop()
		pos := death.Pos
		pos.X += float64(i * 15)
		x.data.Run.Globes[id] = domain.RiftGlobe{ID: id, Pos: pos, Value: x.data.Run.Rules.GlobeProgress}
	}
	if before == domain.RiftActive && (x.data.Run.Phase == domain.RiftBoss || x.data.Run.Phase == domain.RiftBossArriving) {
		s.beginEndgameRiftGuardian()
	}
	return true
}
func (s *Scene) onEndgameRiftPlayerDeath(p *entity.Entity) bool {
	if s.rift == nil {
		return false
	}
	id := domain.CharID(p.Player.Char.ID)
	s.forgetRiftHazardPlayer(p.ID)
	if s.rift.economicPending {
		s.rift.delayedDeaths = append(s.rift.delayedDeaths, endgameRiftDeath{CharID: id, MS: s.riftMS()})
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
		s.replyLiveRift(p, domain.EndgameRequest{Op: "rift.state"})
	}
	return true
}
