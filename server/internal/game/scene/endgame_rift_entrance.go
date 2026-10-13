package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

// The client identifies the native entity it clicked, but admission is resolved
// against that entity's database placement ID and this scene's configured binding.
// A display-name copy, matching sprite, or stale entity from another map is not
// an entrance. Ordinary native quest and trial dialogue paths are unchanged.
func (s *Scene) canUseEndgameRiftEntrance(p *entity.Entity, id domain.EntityID) bool {
	if p == nil || p.Player == nil || id == 0 || s.rift != nil || s.id.Instance != 0 {
		return false
	}
	npc := s.entities[id]
	if npc == nil || npc.Kind != domain.KindNPC || npc.NPC == nil ||
		npc.Pos.MapID != s.id.MapID || p.Pos.MapID != s.id.MapID ||
		p.Player.QuestNPC != npc.Name {
		return false
	}
	for _, entry := range s.riftRules.Entrances() {
		if entry.MapID == s.id.MapID && entry.NPCID == npc.NPC.ConfigID &&
			entry.NPC == npc.Name && entry.Script == npc.NPC.Script {
			return true
		}
	}
	return false
}
