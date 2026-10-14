package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"testing"
)

func cubeQuestProgressFixture(t *testing.T) (*Scene, *riftTestBackend, *int64, domain.QuestDef, endgameRiftPersisted) {
	t.Helper()
	s, b, now := riftFixture(t)
	activateTestRift(t, s, b, *now)
	s.stepEndgameRift() // Drain the initial active-run save before changing the fixture.
	def := domain.QuestDef{ID: 1900030001, Name: "问鼎周天", NPC: "试炼小仙", Steps: []domain.QuestStep{{Kind: domain.QuestStepRiftComplete, To: "巧匠左慈"}}}
	s.quests = domain.QuestTable{def.ID: def}
	s.players[1].Player.Char.Quests.AcceptAt(def, *now)
	d := s.rift.data.clone()
	d.Run.Phase = domain.RiftCompleted
	d.Run.CompletedMS = *now + 100
	d.Run.Members[100].Eligible = true
	d.GrantsReady = true
	d.Grants = map[domain.CharID]endgameRiftGrant{100: {}}
	return s, b, now, def, d
}

func TestRiftQuestRequiresCompletedEligibleGrant(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*endgameRiftPersisted)
	}{
		{"floor transition", func(d *endgameRiftPersisted) { d.Run.Phase = domain.RiftActive }},
		{"failed run", func(d *endgameRiftPersisted) { d.Run.Phase = domain.RiftFailed }},
		{"pre-acceptance", func(d *endgameRiftPersisted) { d.Run.CompletedMS = 1 }},
		{"no completion timestamp", func(d *endgameRiftPersisted) { d.Run.CompletedMS = 0 }},
		{"no settlement eligibility", func(d *endgameRiftPersisted) { d.Run.Members[100].Eligible = false }},
		{"no member", func(d *endgameRiftPersisted) { delete(d.Run.Members, 100) }},
		{"grants pending", func(d *endgameRiftPersisted) { d.GrantsReady = false }},
		{"no personal grant", func(d *endgameRiftPersisted) { delete(d.Grants, 100) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _, _, def, d := cubeQuestProgressFixture(t)
			tc.change(&d)
			if s.recordEndgameRiftQuestCompletion(s.players[1], d) || s.players[1].Player.Char.Quests[def.ID].RiftCompletionRunID != "" {
				t.Fatal("nonqualifying run credited")
			}
		})
	}
}

func TestRiftQuestAcceptsOvertimeAndAlreadyClaimedCompletion(t *testing.T) {
	s, _, _, def, d := cubeQuestProgressFixture(t)
	d.Run.Timely = false
	d.Run.Phase = domain.RiftClosed
	d.Run.Members[100].Claimed = true
	p := s.players[1]
	if !s.recordEndgameRiftQuestCompletion(p, d) || !p.Player.Char.Quests.DeliverableAt(def, "巧匠左慈", nil) {
		t.Fatal("untimed completed eligible run failed")
	}
	if s.recordEndgameRiftQuestCompletion(p, d) {
		t.Fatal("duplicate completion")
	}
}

func TestRiftQuestWaitsForDurableCompletion(t *testing.T) {
	s, b, now, def, d := cubeQuestProgressFixture(t)
	s.rift.data = d
	s.rift.data.Run.Revision++
	b.saveFail = true
	s.syncEndgameRiftCompletion()
	s.stepEndgameRift()
	s.stepEndgameRift()
	p := s.players[1]
	if p.Player.Char.Quests[def.ID].RiftCompletionRunID != "" {
		t.Fatal("failed persistence credited quest")
	}
	b.saveFail = false
	*now += 5000
	s.stepEndgameRift()
	s.stepEndgameRift()
	if p.Player.Char.Quests[def.ID].RiftCompletionRunID != d.Run.RunID {
		t.Fatal("saved completion did not credit quest")
	}
}

func TestRiftQuestRecoversOfflineCompletionWithoutRewardClaim(t *testing.T) {
	s, b, _, def, d := cubeQuestProgressFixture(t)
	p := s.players[1]
	s.rift = nil
	b.runs[d.Run.RunID] = d.record(2)
	state := s.endgamePlayer(p)
	state.state.Payload = withRiftProfile("{}", endgameRiftProfile{RunID: d.Run.RunID})
	s.stepEndgameRiftQuestRecoveries()
	if p.Player.Char.Quests[def.ID].RiftCompletionRunID != "" {
		t.Fatal("credited before load completion")
	}
	s.stepEndgameRiftQuestRecoveries()
	if p.Player.Char.Quests[def.ID].RiftCompletionRunID != d.Run.RunID || b.commits != 0 {
		t.Fatal("offline recovery required economic reward claim")
	}
	if len(s.riftQuestRecoveries) != 0 {
		t.Fatal("completed recovery retained")
	}
}

func TestRiftQuestDoesNotMutateConcurrentCandidate(t *testing.T) {
	s, _, _, def, d := cubeQuestProgressFixture(t)
	p := s.players[1]
	p.Player.TradeBusy = true
	if s.recordEndgameRiftQuestCompletion(p, d) {
		t.Fatal("mutated during economic commit")
	}
	p.Player.TradeBusy = false
	if !s.recordEndgameRiftQuestCompletion(p, d) || p.Player.Char.Quests[def.ID].RiftCompletionRunID == "" {
		t.Fatal("lost deferred completion")
	}
}

func TestTrialPermanentQualificationWaitsForSaveAndSurvivesReset(t *testing.T) {
	s, b, now := riftFixture(t)
	p := s.players[1]
	s.trialLevel = 60
	b.saveFail = true
	s.finishEndgameTrialKeys()
	s.stepEndgameTrial()
	s.recoverEndgameTrial(p)
	if p.Player.Char.HasCompletedTrial() {
		t.Fatal("unsaved trial qualified")
	}
	b.saveFail = false
	*now += 2000
	s.stepEndgameTrial()
	s.stepEndgameTrial()
	if !p.Player.Char.Trial.CompletedOnce {
		t.Fatal("durable trial missing permanent qualification")
	}
	p.Player.Char.Trial.DailyCompleted = 0
	p.Player.Char.Trial.Cycle = 0
	p.Player.Char.Quests[s.trial.Quest] = domain.QuestEntry{State: domain.QuestActive}
	if !p.Player.Char.HasCompletedTrial() {
		t.Fatal("next trial reset qualification")
	}
}

func TestRiftQuestRecoveryRetriesIncompleteGrantSnapshot(t *testing.T) {
	s, b, now, def, d := cubeQuestProgressFixture(t)
	p := s.players[1]
	s.rift = nil
	s.endgamePlayer(p).state.Payload = withRiftProfile("{}", endgameRiftProfile{RunID: d.Run.RunID})
	pending := d.clone()
	pending.GrantsReady = false
	pending.Grants = nil
	b.runs[d.Run.RunID] = pending.record(2)
	s.stepEndgameRiftQuestRecoveries()
	s.stepEndgameRiftQuestRecoveries()
	if p.Player.Char.Quests[def.ID].RiftCompletionRunID != "" {
		t.Fatal("incomplete saved completion credited")
	}
	b.runs[d.Run.RunID] = d.record(3)
	*now += 5000
	s.stepEndgameRiftQuestRecoveries()
	s.stepEndgameRiftQuestRecoveries()
	if p.Player.Char.Quests[def.ID].RiftCompletionRunID != d.Run.RunID {
		t.Fatal("stopped recovery before durable grant arrived")
	}
}

func TestRiftQuestRecoveryRejectsInconsistentDurableRecord(t *testing.T) {
	s, b, _, def, d := cubeQuestProgressFixture(t)
	p := s.players[1]
	s.rift = nil
	s.endgamePlayer(p).state.Payload = withRiftProfile("{}", endgameRiftProfile{RunID: d.Run.RunID})
	record := d.record(2)
	record.State = string(domain.RiftActive)
	b.runs[d.Run.RunID] = record
	s.stepEndgameRiftQuestRecoveries()
	s.stepEndgameRiftQuestRecoveries()
	if p.Player.Char.Quests[def.ID].RiftCompletionRunID != "" {
		t.Fatal("record state did not corroborate completion")
	}
}

func TestTrialPermanentQualificationSurvivesAbandonedQuest(t *testing.T) {
	s, _, _ := riftFixture(t)
	p := s.players[1]
	p.Player.Char.Trial.CompletionID = "saved-trial"
	p.Player.Char.Quests.Abandon(s.trial.Quest)
	c := endgameTrialCompletion{Version: 2, Kind: "trial", Nonce: "saved-trial", Keys: map[domain.CharID]int32{100: 1}, Grants: map[domain.CharID]endgameTrialGrant{100: {}}}
	s.assignEndgameTrial(p, c)
	if !p.Player.Char.Trial.CompletedOnce || p.Player.Char.Quests.Active(s.trial.Quest) {
		t.Fatal("verified completion depended on retaining an abandoned repeatable quest")
	}
}
