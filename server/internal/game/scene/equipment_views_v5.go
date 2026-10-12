package scene

import (
	"crypto/sha256"
	"encoding/json"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
)

func (s *Scene) equipmentCombatViewV5(p *entity.Entity) *domain.EndgameCombatStatsV5 {
	if p == nil || p.Player == nil || !p.Player.EndgameGrowthV5 {
		return nil
	}
	stats := s.equipmentDefense(p, false)
	out := &domain.EndgameCombatStatsV5{MagicFind: s.magicFindV5(p), PhysicalMitigationBP: stats.PhysicalMitigationBP, EquipmentAuras: s.equipmentAuraViews(p), ProcReadiness: s.equipmentProcReadiness(p)}
	for i, element := range []string{"fire", "ice", "holy", "dark"} {
		e := stats.Elements[i]
		out.Elements = append(out.Elements, domain.EndgameElementStatsV5{Element: element, DamageBP: e.DamageBP, ResistBP: e.ResistBP, PierceBP: e.PierceBP, MitigationBP: e.MitigationBP, AbsorbBP: e.AbsorbBP})
	}
	if out.EquipmentAuras == nil {
		out.EquipmentAuras = []domain.EquipmentAuraView{}
	}
	if out.ProcReadiness == nil {
		out.ProcReadiness = []domain.EquipmentProcReadinessView{}
	}
	return out
}
func (s *Scene) publishEquipmentCombatStats(p *entity.Entity) {
	view := s.equipmentCombatViewV5(p)
	if view == nil {
		return
	}
	body, err := json.Marshal(view)
	if err != nil {
		return
	}
	hash := sha256.Sum256(body)
	rt := s.equipmentState()
	if rt.observations == nil {
		rt.observations = map[domain.EntityID][32]byte{}
	}
	if old, ok := rt.observations[p.ID]; ok && old == hash {
		return
	}
	rt.observations[p.ID] = hash
	s.endgameReply(p, domain.EndgameResponse{Kind: "combat.stats", OK: true, CombatStats: view})
}
