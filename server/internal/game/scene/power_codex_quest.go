package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"strconv"
)

const powerCodexNotice = "巧匠左慈对你刚刚分解器物时散出的威能似乎颇感兴趣，不妨前往龙城，与她一叙。"

func (s *Scene) powerCodexQuest() (domain.QuestDef, domain.ItemDef, bool) {
	q, ok := s.quests[domain.PowerCodexQuestID]
	d, found := s.itemDef(domain.PowerCodexItemID)
	return q, d, ok && found && q.RecoveryItem == d.ID && !q.Repeatable && len(q.Steps) == 1 && q.Steps[0].Kind == domain.QuestStepTalk && q.Steps[0].To == "巧匠左慈" && d.UseAction == "power_codex" && !d.Stackable && !d.ConsumeOnUse && !d.CanTrade && !d.CanMail && d.InventoryTabKnown
}

func (s *Scene) acceptPowerCodexQuest(ch *domain.Character, payload string) bool {
	q, _, ok := s.powerCodexQuest()
	if !ok || ch == nil || ch.Quests.Active(q.ID) || ch.Quests.Done(q.ID) {
		return false
	}
	book, err := domain.ReadLegendaryPowerBook(payload)
	if err != nil || len(book) == 0 {
		return false
	}
	if ch.Quests == nil {
		ch.Quests = domain.QuestLog{}
	}
	ch.Quests.AcceptAt(q, s.now().UnixMilli())
	return true
}

func (s *Scene) announcePowerCodexQuest(p *entity.Entity) {
	q, _, ok := s.powerCodexQuest()
	if !ok {
		return
	}
	s.emitTo(p.ID, event.QuestAccepted{Who: p.ID, Quest: int32(q.ID), Name: q.Name})
	s.pushQuestLog(p)
	s.pushQuestOffers(p, p.Player.QuestNPC)
	s.emitTo(p.ID, event.NewbieTextNotice{Who: p.ID, Text: powerCodexNotice})
}

// Old characters keep every record; their first loaded profile supplies the same
// one-time quest. New salvage accepts in its inventory transaction instead.
func (s *Scene) recoverPowerCodexQuest(p *entity.Entity) {
	if p == nil || p.Player == nil || p.Player.Char == nil {
		return
	}
	x := s.endgamePlayer(p)
	if !x.loaded || x.commit != nil || endgameEconomicBusy(p) || !s.acceptPowerCodexQuest(p.Player.Char, x.state.Payload) {
		return
	}
	p.Player.MarkDirty()
	if s.saver != nil {
		s.saver.Save(s.snapshotOf(p).Clone())
	}
	s.announcePowerCodexQuest(p)
}

func (s *Scene) powerCodexAtArtisan(p *entity.Entity) bool {
	if p == nil || !p.Alive() || s.id.Instance != 0 || p.Player.QuestNPC != "巧匠左慈" {
		return false
	}
	for _, n := range s.entities {
		if n.NPC != nil && n.Kind == domain.KindNPC && n.NPC.ConfigID == 73 && n.NPC.Script == "N611FY" && n.Name == "巧匠左慈" && n.Pos.MapID == 7 && p.Pos.MapID == 7 && riftDistance2(p.Pos, n.Pos) <= 480*480 {
			return true
		}
	}
	return false
}

func (s *Scene) grantPowerCodex(p *entity.Entity, req domain.EndgameRequest, first bool) {
	q, d, ok := s.powerCodexQuest()
	if !ok || !s.powerCodexAtArtisan(p) {
		s.endgameError(p, req, "npc_required", "请到龙城巧匠左慈处领取秘典。")
		return
	}
	if where := cubeToolLocation(p, d.ID); where != "" {
		s.endgameError(p, req, "already_owned", "秘典已在你的"+where+"中，无须补领。")
		return
	}
	candidate := s.snapshotOf(p).Clone()
	st := domain.NewStack(d, 1)
	if st.UID == 0 {
		st.UID = domain.NewItemInstanceID()
	}
	st.Bound = true
	if candidate.Bag.AddStack(d, st) != 0 {
		s.endgameError(p, req, "bag_full", "请先腾出一个背包空位，再来领取秘典。")
		return
	}
	if first {
		candidate.Char.Quests.Finish(q.ID)
	}
	s.endgameCommit(p, req, candidate, domain.EndgameResponse{Kind: "powerbook.result", Message: "秘典已放入背包，右击即可翻阅。"}, nil, 0, func() {
		s.pushQuestLog(p)
		s.pushQuestOffers(p, p.Player.QuestNPC)
		if first {
			s.emitTo(p.ID, s.questCompletedEvent(p.ID, q, q.Reward, ""))
		}
	})
}

func (s *Scene) completePowerCodexQuest(p *entity.Entity, q domain.QuestDef, reject func(event.RejectReason)) {
	if !p.Player.Char.Quests.DeliverableAt(q, p.Player.QuestNPC, p.Player.Bag.CountOf) || !s.powerCodexAtArtisan(p) {
		reject(event.RejectQuestNotDone)
		return
	}
	x := s.endgamePlayer(p)
	if !x.loaded || s.endgameBackend == nil {
		s.onEndgame(EndgameRequest{ID: p.ID, Request: domain.EndgameRequest{Op: "snapshot"}})
		s.trialNotice(p, "修行记录读取中，请稍后领取。")
		return
	}
	if x.commit != nil || x.receipt != nil || endgameEconomicBusy(p) {
		reject(event.RejectInvalid)
		return
	}
	s.grantPowerCodex(p, domain.EndgameRequest{Op: "powerbook.claim", RequestID: "power-codex-claim-" + strconv.Itoa(int(q.ID)), ExpectedRevision: strconv.FormatInt(x.state.Revision, 10)}, true)
}

func (s *Scene) recoverPowerCodex(p *entity.Entity, req domain.EndgameRequest) {
	if !p.Player.Char.Quests.Done(domain.PowerCodexQuestID) {
		s.endgameError(p, req, "requirements", "请先完成领取秘典的任务。")
		return
	}
	s.grantPowerCodex(p, req, false)
}

func (s *Scene) filterPowerCodexOffers(p *entity.Entity, offers []domain.Offer) []domain.Offer {
	if p.Player.Char.Quests.Active(domain.PowerCodexQuestID) || p.Player.Char.Quests.Done(domain.PowerCodexQuestID) {
		return offers
	}
	result := offers[:0]
	for _, o := range offers {
		if o.ID != domain.PowerCodexQuestID {
			result = append(result, o)
		}
	}
	return result
}
