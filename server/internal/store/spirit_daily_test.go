package store

import (
	"context"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"reflect"
	"strings"
	"testing"
)

func TestSpiritDailyQuestSnapshotRelog(t *testing.T) {
	m, s := endgameFixture(t)
	entry := domain.QuestEntry{State: domain.QuestActive, AcceptedAtMS: 123, Kills: map[domain.MonsterID]int32{1: 1}, SpiritDaily: domain.SpiritDailyQuestState{Day: "2026-10-28", Kind: 3, Targets: [3]domain.SpiritDailyTarget{{Monster: 1, MapID: 7, Name: "甲"}, {Monster: 2, MapID: 8, Name: "乙"}, {Monster: 3, MapID: 9, Name: "丙"}}}}
	s.Char.Quests = domain.QuestLog{domain.SpiritDailyQuestID: entry}
	if err := m.SaveSnapshot(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	s.Char.Quests[domain.SpiritDailyQuestID] = domain.QuestEntry{}
	got, err := m.LoadQuests(context.Background(), s.Char.ID)
	if err != nil || !reflect.DeepEqual(got[domain.SpiritDailyQuestID], entry) {
		t.Fatal("relogin lost daily snapshot", err, got)
	}
}

func spiritDailyClaimPlan(s domain.Snapshot) domain.EndgameCommitPlan {
	after := s.Clone()
	after.Bag.Set(10, domain.Stack{Item: 36041, Count: 300})
	after.Char.Quests = domain.QuestLog{domain.SpiritDailyQuestID: {State: domain.QuestFinished, SpiritDaily: domain.SpiritDailyQuestState{Kind: 1, Day: "2026-10-28", ClaimedDay: "2026-10-29"}}}
	return domain.EndgameCommitPlan{Before: s, Snapshot: after, State: domain.EndgameState{Revision: 1, Payload: "{}"}, Receipt: domain.EndgameReceipt{CharID: s.Char.ID, RequestID: "spirit-daily-claim-2026-10-29", Operation: "spirit.daily.claim", Digest: strings.Repeat("d", 64), Revision: 1, Response: `{"ok":true}`}}
}
func TestSpiritDailyClaimDurableFaultsAndLostAck(t *testing.T) {
	for _, stage := range []string{"state", "snapshot", "receipt", "lost_ack"} {
		t.Run(stage, func(t *testing.T) {
			m, s := endgameFixture(t)
			p := spiritDailyClaimPlan(s)
			fault := &endgameFaultStore{Memory: m, stage: stage, lostAck: stage == "lost_ack"}
			backend := NewEndgameBackend(fault, nil)
			result := awaitEndgame(t, backend.Commit(p))
			after := durableSnapshot(t, m, s)
			if stage == "lost_ack" {
				if result.Err != nil || after.Bag.CountOf(36041) != 300 || after.Char.Quests[domain.SpiritDailyQuestID].SpiritDaily.ClaimedDay != "2026-10-29" {
					t.Fatal("lost ack split reward/quota", result)
				}
				again := awaitEndgame(t, backend.Commit(p))
				if again.Err != nil || !again.Replayed || durableSnapshot(t, m, s).Bag.CountOf(36041) != 300 {
					t.Fatal("retry duplicated reward", again)
				}
			} else {
				if result.Err == nil || after.Bag.CountOf(36041) != 0 || after.Char.Quests.Done(domain.SpiritDailyQuestID) {
					t.Fatal("failed transaction split reward/quota", result)
				}
			}
		})
	}
}
func TestSpiritDailyConcurrentClaimsExactlyOnce(t *testing.T) {
	m, s := endgameFixture(t)
	backend := NewEndgameBackend(m, nil)
	p := spiritDailyClaimPlan(s)
	a := backend.Commit(p)
	b := backend.Commit(p)
	one, two := awaitEndgame(t, a), awaitEndgame(t, b)
	if one.Err != nil || two.Err != nil || one.Replayed == two.Replayed {
		t.Fatal("concurrent claims", one, two)
	}
	after := durableSnapshot(t, m, s)
	if after.Bag.CountOf(36041) != 300 || !after.Char.Quests.Done(domain.SpiritDailyQuestID) {
		t.Fatal("not exactly once")
	}
}
