package scene

import (
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"strconv"
)

func (s *Scene) spiritDailyState(p *entity.Entity, npc string) (domain.SpiritDailyQuestState, bool) {
	day := domain.SpiritDailyDay(s.now())
	if e, ok := p.Player.Char.Quests[domain.SpiritDailyQuestID]; ok {
		if e.State != domain.QuestFinished {
			q, valid := e.SpiritDaily.Definition()
			return e.SpiritDaily, valid && q.NPC == npc
		}
		if e.SpiritDaily.ClaimedDay >= day {
			return domain.SpiritDailyQuestState{}, false
		}
	}
	for _, d := range domain.SpiritDefinitions() {
		if d.NPCName == npc {
			targets, ok := domain.SelectSpiritDailyTargets(s.spiritDailyTargets, p.Player.Char.ID, day)
			return domain.SpiritDailyQuestState{Day: day, Kind: d.Kind, Targets: targets}, ok
		}
	}
	return domain.SpiritDailyQuestState{}, false
}
func (s *Scene) spiritDailyOffers(p *entity.Entity, npc string, offers []domain.Offer) []domain.Offer {
	st, ok := s.spiritDailyState(p, npc)
	if !ok {
		return offers
	}
	q, ok := st.Definition()
	if !ok {
		return offers
	}
	ch := p.Player.Char.Clone()
	ch.Quests = ch.Quests.Clone()
	if ch.Quests.Done(q.ID) {
		delete(ch.Quests, q.ID)
	}
	return append(offers, domain.OffersOf(domain.QuestTable{q.ID: q}, npc, ch, p.Player.Bag.CountOf)...)
}
func (s *Scene) spiritDailyLog(p *entity.Entity, list []domain.ActiveQuest) []domain.ActiveQuest {
	e, ok := p.Player.Char.Quests[domain.SpiritDailyQuestID]
	if !ok || e.State == domain.QuestFinished {
		return list
	}
	q, ok := e.SpiritDaily.Definition()
	if !ok {
		return list
	}
	return append(list, domain.ActiveQuestsOf(domain.QuestTable{q.ID: q}, p.Player.Char, p.Player.Bag.CountOf)...)
}
func (s *Scene) spiritDailyReady(p *entity.Entity) bool {
	x := s.endgamePlayer(p)
	if !x.loaded || s.endgameBackend == nil {
		s.onEndgame(EndgameRequest{ID: p.ID, Request: domain.EndgameRequest{Op: "snapshot"}})
		s.trialNotice(p, "修行记录读取中，请稍后再试。")
		return false
	}
	return x.commit == nil && !endgameEconomicBusy(p)
}
func (s *Scene) acceptSpiritDaily(p *entity.Entity, npc string) {
	if npc == "" {
		npc = p.Player.QuestNPC
	}
	if s.npcOnMap(npc) == "" || !s.spiritDailyReady(p) {
		return
	}
	if p.Player.Char.Quests.Active(domain.SpiritDailyQuestID) {
		return
	}
	st, ok := s.spiritDailyState(p, npc)
	if !ok {
		return
	}
	q, ok := st.Definition()
	if !ok {
		return
	}
	candidate := s.snapshotOf(p).Clone()
	if candidate.Char.Quests == nil {
		candidate.Char.Quests = domain.QuestLog{}
	}
	candidate.Char.Quests[q.ID] = domain.QuestEntry{State: domain.QuestActive, AcceptedAtMS: s.now().UnixMilli(), SpiritDaily: st}
	x := s.endgamePlayer(p)
	req := domain.EndgameRequest{Op: "spirit.daily.accept", RequestID: fmt.Sprintf("spirit-daily-accept-%s-%d-%d", st.Day, st.Kind, x.state.Revision), ExpectedRevision: strconv.FormatInt(x.state.Revision, 10)}
	s.endgameCommit(p, req, candidate, domain.EndgameResponse{Kind: req.Op, OK: true}, nil, 0, func() {
		s.emitTo(p.ID, event.QuestAccepted{Who: p.ID, Quest: int32(q.ID), Name: q.Name})
		s.pushQuestLog(p)
		s.pushQuestOffers(p, npc)
	})
}
func (s *Scene) completeSpiritDaily(p *entity.Entity) {
	if !s.spiritDailyReady(p) {
		return
	}
	e, ok := p.Player.Char.Quests[domain.SpiritDailyQuestID]
	if !ok || e.State == domain.QuestFinished {
		return
	}
	q, ok := e.SpiritDaily.Definition()
	if !ok || s.npcOnMap(p.Player.QuestNPC) != q.NPC || !p.Player.Char.Quests.DeliverableAt(q, p.Player.QuestNPC, p.Player.Bag.CountOf) {
		return
	}
	candidate := s.snapshotOf(p).Clone()
	bag, _, ok := s.planQuestItems(candidate.Bag, nil, nil, q.Reward.Items)
	if !ok {
		s.trialNotice(p, "行囊已满，请腾出空位再来领取元神。")
		return
	}
	candidate.Bag = bag
	day := domain.SpiritDailyDay(s.now())
	e.State = domain.QuestFinished
	e.SpiritDaily.ClaimedDay = day
	e.Kills = nil
	candidate.Char.Quests[q.ID] = e
	x := s.endgamePlayer(p)
	req := domain.EndgameRequest{Op: "spirit.daily.claim", RequestID: "spirit-daily-claim-" + day, ExpectedRevision: strconv.FormatInt(x.state.Revision, 10)}
	s.endgameCommit(p, req, candidate, domain.EndgameResponse{Kind: req.Op, OK: true}, nil, 0, func() {
		s.emitTo(p.ID, s.questCompletedEvent(p.ID, q, q.Reward, ""))
		s.pushQuestLog(p)
		s.pushQuestOffers(p, p.Player.QuestNPC)
	})
}
func (s *Scene) recordSpiritDailyKill(p, dead *entity.Entity) bool {
	if dead.Monster.Kind != domain.MonsterBoss || s.id.Instance != 0 || s.dungeon != nil || s.rift != nil || s.trialRun != nil || s.trial.IsMap(s.id.MapID) || dead.Monster.SpawnID <= 0 || dead.Monster.Summoner != 0 {
		return false
	}
	e, ok := p.Player.Char.Quests[domain.SpiritDailyQuestID]
	if !ok || e.State == domain.QuestFinished {
		return false
	}
	for _, t := range e.SpiritDaily.Targets {
		if t.Monster == dead.Monster.TypeID && e.Kills[t.Monster] < 1 {
			if e.Kills == nil {
				e.Kills = map[domain.MonsterID]int32{}
			}
			e.Kills[t.Monster] = 1
			p.Player.Char.Quests[domain.SpiritDailyQuestID] = e
			return true
		}
	}
	return false
}

func (s *Scene) abandonSpiritDaily(p *entity.Entity) {
	if !s.spiritDailyReady(p) || !p.Player.Char.Quests.Active(domain.SpiritDailyQuestID) {
		return
	}
	candidate := s.snapshotOf(p).Clone()
	candidate.Char.Quests.Abandon(domain.SpiritDailyQuestID)
	x := s.endgamePlayer(p)
	req := domain.EndgameRequest{Op: "spirit.daily.abandon", RequestID: fmt.Sprintf("spirit-daily-abandon-%d", x.state.Revision), ExpectedRevision: strconv.FormatInt(x.state.Revision, 10)}
	s.endgameCommit(p, req, candidate, domain.EndgameResponse{Kind: req.Op, OK: true}, nil, 0, func() { s.pushQuestLog(p); s.pushQuestOffers(p, p.Player.QuestNPC) })
}
