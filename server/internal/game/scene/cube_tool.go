package scene

import (
	"strconv"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func (s *Scene) cubeToolQuest() (domain.QuestDef, domain.ItemDef, bool) {
	var quest domain.QuestDef
	var item domain.ItemDef
	found := false
	for _, q := range s.quests {
		if q.RecoveryItem == 0 {
			continue
		}
		d, ok := s.itemDef(q.RecoveryItem)
		if !ok || d.UseAction != "cube" {
			continue
		}
		if found || d.Equip != nil || d.Stackable || d.ConsumeOnUse || d.CanMail || d.CanTrade || !d.InventoryTabKnown || q.Repeatable || !q.RequireTrialCompletion || len(q.Steps) != 1 || q.Steps[0].Kind != domain.QuestStepRiftComplete || q.Steps[0].To == "" || len(q.Reward.Items) != 1 || q.Reward.Items[0].Item != d.ID || q.Reward.Items[0].Qty != 1 {
			return domain.QuestDef{}, domain.ItemDef{}, false
		}
		quest, item, found = q, d, true
	}
	return quest, item, found
}

func (s *Scene) cubeToolConfiguration() map[string]string {
	q, d, ok := s.cubeToolQuest()
	if !ok {
		return nil
	}
	return map[string]string{"cubeItemId": strconv.FormatInt(int64(d.ID), 10), "cubeNPC": q.Steps[len(q.Steps)-1].To, "cubeLore": q.RecoveryLore}
}
func (s *Scene) hasCubeTool(p *entity.Entity) bool {
	if p == nil || p.Player == nil || p.Player.Char == nil || p.Player.Bag == nil || !p.Alive() {
		return false
	}
	q, d, ok := s.cubeToolQuest()
	if !ok || !p.Player.Char.Quests.Done(q.ID) {
		return false
	}
	found := false
	p.Player.Bag.Each(func(_ int, st domain.Stack) {
		if st.Item == d.ID && st.Count == 1 && st.Bound && !st.Locked {
			found = true
		}
	})
	return found
}
func cubeToolLocation(p *entity.Entity, item domain.ItemID) string {
	if p.Player.Bag.CountOf(item) > 0 {
		return "背包"
	}
	found := false
	if p.Player.Warehouse != nil {
		p.Player.Warehouse.Each(func(_, _ int, st domain.Stack) {
			if st.Item == item && st.Count > 0 {
				found = true
			}
		})
	}
	if found {
		return "仓库"
	}
	return ""
}
func (s *Scene) cubeGrant(p *entity.Entity, q domain.QuestDef, d domain.ItemDef, req domain.EndgameRequest, first bool) {
	if where := cubeToolLocation(p, d.ID); where != "" {
		s.cubeToolError(p, req, "already_owned", "周天鼎已在你的"+where+"中，无须补领。")
		return
	}
	candidate := s.snapshotOf(p).Clone()
	st := domain.NewStack(d, 1)
	st.Bound = true
	if candidate.Bag.AddStack(d, st) != 0 {
		s.cubeToolError(p, req, "bag_full", "请先腾出一个背包空位，再来领取周天鼎。")
		return
	}
	if first {
		candidate.Char.Quests.Finish(q.ID)
	}
	s.endgameCommit(p, req, candidate, domain.EndgameResponse{Kind: req.Op, RequestID: req.RequestID, OK: true, Message: "周天鼎已放入背包，右击即可问鼎，不会消耗。"}, nil, 0, func() {
		s.pushQuestLog(p)
		s.pushQuestOffers(p, p.Player.QuestNPC)
		if first {
			s.emitTo(p.ID, s.questCompletedEvent(p.ID, q, q.Reward, ""))
		}
	})
}
func (s *Scene) recoverCubeTool(p *entity.Entity, req domain.EndgameRequest) {
	q, d, ok := s.cubeToolQuest()
	if !ok || !p.Alive() || !p.Player.Char.Quests.Done(q.ID) {
		s.cubeToolError(p, req, "requirements", "请先完成领取周天鼎的任务。")
		return
	}
	npc := s.entities[req.NPCEntityID]
	target := q.Steps[len(q.Steps)-1].To
	if s.id.Instance != 0 || npc == nil || npc.Kind != domain.KindNPC || npc.NPC == nil || npc.NPC.ConfigID <= 0 || npc.Name != target || p.Player.QuestNPC != target || npc.Pos.MapID != p.Pos.MapID || riftDistance2(p.Pos, npc.Pos) > 480*480 {
		s.cubeToolError(p, req, "npc_required", "请到领取周天鼎的巧匠处当面补领。")
		return
	}
	s.cubeGrant(p, q, d, req, false)
}
func (s *Scene) completeCubeToolQuest(p *entity.Entity, q domain.QuestDef, reject func(event.RejectReason)) {
	actual, d, ok := s.cubeToolQuest()
	if !ok || actual.ID != q.ID || !p.Player.Char.Quests.DeliverableAt(q, p.Player.QuestNPC, p.Player.Bag.CountOf) || s.npcOnMap(p.Player.QuestNPC) == "" {
		reject(event.RejectQuestNotDone)
		return
	}
	x := s.endgamePlayer(p)
	if !x.loaded || s.endgameBackend == nil {
		s.onEndgame(EndgameRequest{ID: p.ID, Request: domain.EndgameRequest{Op: "snapshot"}})
		s.trialNotice(p, "修行记录读取中，请稍后领取。")
		return
	}
	if x.commit != nil || endgameEconomicBusy(p) {
		reject(event.RejectInvalid)
		return
	}
	req := domain.EndgameRequest{Op: "cube.claim", RequestID: "cube-claim-" + strconv.FormatInt(int64(q.ID), 10), ExpectedRevision: strconv.FormatInt(x.state.Revision, 10)}
	s.cubeGrant(p, q, d, req, true)
}

func (s *Scene) cubeToolError(p *entity.Entity, req domain.EndgameRequest, code, message string) {
	s.endgameError(p, req, code, message)
}
