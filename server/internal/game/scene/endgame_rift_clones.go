package scene

import (
	"math"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/spawn"
)

func (s *Scene) spawnRiftClones(owner *entity.Entity, a domain.RiftAbilityRule, now int64) bool {
	if owner == nil || owner.Monster == nil || !owner.Alive() || s.defs == nil || s.alloc == nil {
		return false
	}
	def, ok := s.defs.Def(owner.Monster.TypeID)
	if !ok {
		return false
	}
	base, ok := s.rift.baseStats[owner.ID]
	if !ok {
		return false
	}
	x := s.eliteSnapshot()
	var old []domain.EntityID
	temporary := 0
	for id, m := range s.monsters {
		if m.Monster != nil && m.Monster.Summoner != 0 {
			temporary++
		}
		if c, ok := x.Clones[id]; ok && c.Owner == owner.ID {
			old = append(old, id)
		}
	}
	if temporary-len(old)+int(a.Count) > maxSummonsPerScene {
		return false
	}
	positions := make([]domain.Pos, 0, a.Count)
	for i := 0; i < 24 && len(positions) < int(a.Count); i++ {
		angle := float64(i) * math.Pi / 12
		pos := owner.Pos
		pos.X += 80 * math.Cos(angle)
		pos.Y += 80 * math.Sin(angle)
		if !s.spawnPositionWalkable(pos) {
			continue
		}
		occupied := false
		for _, p := range positions {
			if riftDistance2(p, pos) < 48*48 {
				occupied = true
			}
		}
		for id, e := range s.entities {
			if e.Kind != domain.KindMonster && e.Kind != domain.KindPlayer {
				continue
			}
			if c, ok := x.Clones[id]; ok && c.Owner == owner.ID {
				continue
			}
			if e.Alive() && riftDistance2(e.Pos, pos) < 32*32 {
				occupied = true
			}
		}
		if !occupied {
			positions = append(positions, pos)
		}
	}
	if len(positions) != int(a.Count) {
		return false
	}
	// Validation is complete before replacing the old group. No partial success.
	for _, id := range old {
		s.removeRiftClone(id)
	}
	def.Level = owner.Level
	def.Stats = base
	def.HP = max(int32(1), domain.SaturateInt32(int64(base.MaxHP)*int64(a.HPPct)/100))
	def.Stats.MaxHP = def.HP
	def.EliteID = 0
	for _, pos := range positions {
		m := spawn.Summon(s.alloc.Monster(), pos, def, owner.ID)
		m.Monster.Target = owner.Monster.Target
		m.Monster.EliteRing = false
		s.rift.baseStats[m.ID] = def.Stats
		s.initializeMonster(m)
		s.entities[m.ID] = m
		s.monsters[m.ID] = m
		s.aoi.Enter(m)
		s.emit(s.spawnEvent(m))
		x.Clones[m.ID] = endgameRiftClone{Owner: owner.ID, ExpiresAtMS: now + a.DurationMS}
	}
	return true
}
func (s *Scene) removeRiftClone(id domain.EntityID) {
	if s.rift == nil || s.rift.data.Elite == nil {
		return
	}
	if m := s.monsters[id]; m != nil {
		s.despawnSummonedMonster(m)
	}
	delete(s.rift.baseStats, id)
	delete(s.rift.data.Elite.Clones, id)
	// Descendants are also temporary. Remove them with their parent, without a
	// death event or any reward/progress registration.
	for child, m := range s.monsters {
		if m.Monster != nil && m.Monster.Summoner == id {
			s.removeRiftClone(child)
		}
	}
}
func (s *Scene) stepRiftClones(now int64) {
	for id, c := range s.eliteSnapshot().Clones {
		owner := s.monsters[c.Owner]
		m := s.monsters[id]
		if now >= c.ExpiresAtMS || owner == nil || !owner.Alive() || m == nil || !m.Alive() {
			s.removeRiftClone(id)
			s.eliteChanged()
		}
	}
}
