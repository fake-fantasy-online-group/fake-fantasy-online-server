package scene

import (
	"errors"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"reflect"
	"testing"
	"time"
)

func dailyFixture(t *testing.T) (*Scene, *cubeTestBackend, domain.SpiritDefinition) {
	s, _, b, d := spiritFixture(t)
	s.now = func() time.Time { return time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC) }
	s.quests = domain.QuestTable{}
	s.players[1].Player.QuestNPC = d.NPCName
	s.spiritDailyTargets = []domain.SpiritDailyTarget{{Monster: 11, MapID: 7, Name: "甲"}, {Monster: 12, MapID: 7, Name: "乙"}, {Monster: 13, MapID: 7, Name: "丙"}, {Monster: 14, MapID: 8, Name: "丁"}}
	return s, b, d
}
func dailyFinish(s *Scene, b *cubeTestBackend) {
	b.result <- domain.EndgameCommitResult{State: b.plan.State, Receipt: b.plan.Receipt}
	s.finishEndgame(false)
}
func TestSpiritDailyAcceptKillsAtomicClaimAndAllNPCQuota(t *testing.T) {
	s, b, d := dailyFixture(t)
	p := s.players[1]
	s.acceptSpiritDaily(p, d.NPCName)
	if b.commits != 1 || p.Player.Char.Quests.Active(domain.SpiritDailyQuestID) {
		t.Fatal("not atomic accept")
	}
	dailyFinish(s, b)
	e := p.Player.Char.Quests[domain.SpiritDailyQuestID]
	if e.SpiritDaily.Kind != d.Kind {
		t.Fatal("wrong reward source")
	}
	for _, target := range e.SpiritDaily.Targets {
		s.id.MapID = target.MapID
		dead := &entity.Entity{Monster: &entity.Monster{Kind: domain.MonsterBoss, TypeID: target.Monster, SpawnID: 1}}
		s.id.Instance = 1
		if s.recordSpiritDailyKill(p, dead) {
			t.Fatal("dungeon kill")
		}
		s.id.Instance = 0
		dead.Monster.Summoner = 5
		if s.recordSpiritDailyKill(p, dead) {
			t.Fatal("summon kill")
		}
		dead.Monster.Summoner = 0
		if !s.recordSpiritDailyKill(p, dead) || s.recordSpiritDailyKill(p, dead) {
			t.Fatal("bad credit")
		}
	}
	s.id.MapID = 7
	before := p.Player.Bag.CountOf(d.EssenceID)
	s.completeSpiritDaily(p)
	if b.commits != 2 || p.Player.Bag.CountOf(d.EssenceID) != before {
		t.Fatal("claim not atomic")
	}
	s.completeSpiritDaily(p)
	s.abandonSpiritDaily(p)
	if b.commits != 2 {
		t.Fatal("concurrent mutation")
	}
	dailyFinish(s, b)
	if p.Player.Bag.CountOf(d.EssenceID) != before+300 {
		t.Fatal("wrong amount")
	}
	for _, npc := range domain.SpiritDefinitions() {
		if _, ok := s.spiritDailyState(p, npc.NPCName); ok {
			t.Fatal("extra daily claim", npc.Name)
		}
	}
	s.completeSpiritDaily(p)
	if b.commits != 2 {
		t.Fatal("duplicate claim")
	}
	s.now = func() time.Time { return time.Date(2026, 10, 28, 16, 0, 0, 0, time.UTC) }
	if _, ok := s.spiritDailyState(p, d.NPCName); !ok {
		t.Fatal("no next day offer")
	}
}
func TestSpiritDailyAcceptRollbackAndAbandonNoReroll(t *testing.T) {
	s, b, d := dailyFixture(t)
	p := s.players[1]
	initial, _ := s.spiritDailyState(p, d.NPCName)
	s.acceptSpiritDaily(p, d.NPCName)
	b.result <- domain.EndgameCommitResult{Err: errors.New("offline")}
	s.finishEndgame(false)
	if p.Player.Char.Quests.Active(domain.SpiritDailyQuestID) {
		t.Fatal("failed accept persisted")
	}
	s.acceptSpiritDaily(p, d.NPCName)
	dailyFinish(s, b)
	accepted := p.Player.Char.Quests[domain.SpiritDailyQuestID]
	if !reflect.DeepEqual(initial, accepted.SpiritDaily) {
		t.Fatal("reroll on accept")
	}
	s.abandonSpiritDaily(p)
	dailyFinish(s, b)
	next, _ := s.spiritDailyState(p, d.NPCName)
	if initial.Targets != next.Targets {
		t.Fatal("abandon reroll")
	}
	s.acceptSpiritDaily(p, d.NPCName)
	dailyFinish(s, b)
	s.now = func() time.Time { return time.Date(2026, 10, 29, 16, 0, 0, 0, time.UTC) }
	next, _ = s.spiritDailyState(p, d.NPCName)
	if initial.Targets != next.Targets {
		t.Fatal("lost unfinished targets at midnight")
	}
}

func TestSpiritDailyClaimRollbackAndBagFull(t *testing.T) {
	for _, full := range []bool{false, true} {
		s, b, d := dailyFixture(t)
		p := s.players[1]
		s.acceptSpiritDaily(p, d.NPCName)
		dailyFinish(s, b)
		e := p.Player.Char.Quests[domain.SpiritDailyQuestID]
		e.Kills = map[domain.MonsterID]int32{}
		for _, target := range e.SpiritDaily.Targets {
			e.Kills[target.Monster] = 1
		}
		p.Player.Char.Quests[domain.SpiritDailyQuestID] = e
		if full {
			p.Player.Bag = domain.NewBag(1)
			p.Player.Bag.Set(0, domain.Stack{Item: 1001, Count: 1})
		}
		before := p.Player.Bag.Clone()
		s.completeSpiritDaily(p)
		if full {
			if b.commits != 1 {
				t.Fatal("full bag committed")
			}
		} else {
			if b.commits != 2 {
				t.Fatal("no claim plan")
			}
			b.result <- domain.EndgameCommitResult{Err: errors.New("transaction rollback")}
			s.finishEndgame(false)
		}
		if !reflect.DeepEqual(before, p.Player.Bag) || !p.Player.Char.Quests.Active(domain.SpiritDailyQuestID) {
			t.Fatal("failed claim lost items/progress")
		}
	}
}
func TestSpiritDailyNineNPCRewardAndOfferProjection(t *testing.T) {
	s, _, _ := dailyFixture(t)
	p := s.players[1]
	var targets [3]domain.SpiritDailyTarget
	for i, d := range domain.SpiritDefinitions() {
		st, ok := s.spiritDailyState(p, d.NPCName)
		if !ok {
			t.Fatal(d.Name)
		}
		q, _ := st.Definition()
		if q.NPC != d.NPCName || q.Reward.Items[0].Item != d.EssenceID || q.Reward.Items[0].Qty != 300 {
			t.Fatal("wrong reward")
		}
		if i == 0 {
			targets = st.Targets
		} else if targets != st.Targets {
			t.Fatal("NPC rerolled targets")
		}
		offers := s.spiritDailyOffers(p, d.NPCName, nil)
		if len(offers) != 1 || offers[0].Title != q.Name {
			t.Fatal("native offer missing")
		}
	}
	if _, ok := s.spiritDailyState(p, "戒灵仙子"); ok {
		t.Fatal("ring exposed")
	}
}
