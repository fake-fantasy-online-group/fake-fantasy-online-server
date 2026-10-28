package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"strconv"
)

func (s *Scene) canUseWingNPC(p *entity.Entity, id domain.EntityID) bool {
	if s.wingRules == nil || p == nil || p.Player == nil || !p.Alive() || s.id.Instance != 0 {
		return false
	}
	c := s.wingRules.Config()
	npc := s.entities[id]
	return npc != nil && npc.NPC != nil && npc.Kind == domain.KindNPC && npc.NPC.ConfigID == c.Entrance.NPCID && npc.NPC.Script == c.Entrance.Script && npc.Name == c.Entrance.NPC && s.id.MapID == c.Entrance.MapID && npc.Pos.MapID == s.id.MapID && p.Pos.MapID == s.id.MapID && p.Player.QuestNPC == npc.Name && riftDistance2(p.Pos, npc.Pos) <= 480*480
}

// The one-time starter and completed quest are one durable aggregate mutation.
func (s *Scene) completeWingGuide(p *entity.Entity, q domain.QuestDef, reject func(event.RejectReason)) {
	c := s.wingRules.Config()
	if q.ID != c.Quest || q.Repeatable || len(q.Steps) != 1 || !p.Player.Char.Quests.DeliverableAt(q, p.Player.QuestNPC, p.Player.Bag.CountOf) {
		reject(event.RejectQuestNotDone)
		return
	}
	var id domain.EntityID
	for eid, n := range s.entities {
		if n.Name == p.Player.QuestNPC && s.canUseWingNPC(p, eid) {
			id = eid
			break
		}
	}
	if id == 0 {
		reject(event.RejectNoTarget)
		return
	}
	x := s.endgamePlayer(p)
	if !x.loaded || s.endgameBackend == nil {
		s.onEndgame(EndgameRequest{ID: p.ID, Request: domain.EndgameRequest{Op: "snapshot"}})
		s.trialNotice(p, "修行记录读取中，请稍后领取。")
		return
	}
	if endgameEconomicBusy(p) || x.commit != nil || x.receipt != nil {
		reject(event.RejectInvalid)
		return
	}
	def, ok := s.itemDef(c.Starter)
	if !ok || def.Equip == nil {
		reject(event.RejectUnknown)
		return
	}
	snap := s.snapshotOf(p).Clone()
	wing := domain.NewStack(def, 1)
	wing.Bound = true
	if snap.Bag.AddStack(def, wing) != 0 {
		reject(event.RejectBagFull)
		return
	}
	snap.Char.Quests.Finish(q.ID)
	req := domain.EndgameRequest{Op: "wing.guide", RequestID: "wing-guide-claim-" + strconv.Itoa(int(q.ID)), ExpectedRevision: strconv.FormatInt(x.state.Revision, 10)}
	s.endgameCommit(p, req, snap, domain.EndgameResponse{Kind: req.Op, Message: "光之翼已放入背包。破境或勤修皆可积羽，可用周天鼎升阶、换系，以威能卷轴融入威能。"}, nil, 0, func() {
		s.pushQuestLog(p)
		s.pushQuestOffers(p, p.Player.QuestNPC)
		s.emitTo(p.ID, s.questCompletedEvent(p.ID, q, q.Reward, ""))
	})
}
