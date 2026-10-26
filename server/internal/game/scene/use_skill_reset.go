package scene

import (
	"fmt"
	"strconv"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

// Native UseItem has no client transaction id. Bind its server receipt to the
// durable aggregate revision and exact slot, so failed/lost-ACK attempts cannot
// refund twice. The inventory lease also blocks simultaneous uses and learning.
func (s *Scene) useSkillResetCrystal(p *entity.Entity, slot int) {
	notice := func(text string) { s.emitTo(p.ID, event.ServerNotice{Who: p.ID, Text: text}) }
	x := s.endgamePlayer(p)
	if s.endgameBackend == nil || s.skills == nil || !canAct(p) || s.entityMapLoading(p) || endgameEconomicBusy(p) || x.load != nil || x.receipt != nil || x.commit != nil {
		notice("当前状态无法重置技能，请稍后再试")
		return
	}
	if !x.loaded {
		s.onEndgame(EndgameRequest{ID: p.ID, Request: domain.EndgameRequest{Op: "snapshot"}})
		notice("正在读取技能存档，请稍后再次使用")
		return
	}
	candidate := s.snapshotOf(p).Clone()
	removed, refund, err := domain.ResetAllocatedSkills(candidate.Char, s.skills.ProgressionCatalog())
	if err != nil {
		notice(err.Error())
		return
	}
	if st := candidate.Bag.At(slot); st.Item != 3380 || st.Locked || !candidate.Bag.RemoveAt(slot, 1) {
		notice("遗忘水晶已不在原位置，未重置技能")
		return
	}
	matches := func(d domain.StatusDef) bool {
		return removed[d.SkillSnapshotID] || removed[domain.SkillID(d.SourceSkillID)]
	}
	kept := candidate.Char.SavedStatuses[:0]
	for _, saved := range candidate.Char.SavedStatuses {
		if !saved.SourceSelf || !matches(saved.Def) {
			kept = append(kept, saved)
		}
	}
	candidate.Char.SavedStatuses = kept
	rev := strconv.FormatInt(x.state.Revision, 10)
	req := domain.EndgameRequest{Op: "skill.reset", RequestID: fmt.Sprintf("skill-reset-v1-%s-%d", rev, slot), ExpectedRevision: rev}
	response := domain.EndgameResponse{Kind: "skill.reset", Message: fmt.Sprintf("技能已重置，返还%d点技能点。", refund)}
	s.endgameCommit(p, req, candidate, response, nil, 0, func() {
		notice(response.Message)
	})
}

// Reconcile runtime state against the committed or recovered authoritative skills.
// This is deliberately separate from the success callback, which never replays.
func (s *Scene) reconcileSkillReset(p *entity.Entity, before domain.Learned) {
	removed := make(map[domain.SkillID]bool)
	for id, rank := range before {
		if rank > 0 && p.Player.Char.Skills[id] == 0 {
			removed[id] = true
		}
	}
	matches := func(d domain.StatusDef) bool {
		return removed[d.SkillSnapshotID] || removed[domain.SkillID(d.SourceSkillID)]
	}
	s.clearCastWindup(p.ID)
	// Remove self-cast statuses from forgotten skills, but retain cooldowns:
	// resetting skills must not be a cooldown bypass or a free resource refill.
	for _, target := range s.entities {
		if target.Status != nil {
			s.statusesRemoved(target, target.Status.RemoveSourceWhere(p.ID, matches))
		}
	}
	s.emitTo(p.ID, s.skillSnapshot(p))
	s.emitTo(p.ID, s.hotbarSnapshot(p))
	if p.Player.EndgameGrowthV5 {
		s.sendGrowthV5Skills(p, domain.EndgameRequest{Op: "skill.ranks"})
	}
}
