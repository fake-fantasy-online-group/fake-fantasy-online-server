package scene

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
)

func wingRiftRulesFixture(t *testing.T) (*domain.RiftRuleTable, *Scene) {
	t.Helper()
	source, old, _ := cumulativeRiftFixture(t)
	fresh := *old
	fresh.GuardianHPDivisor = 10
	fresh.Version, fresh.FloorLayoutVersion, fresh.GuardianRecentWindow = 14, 0, 0
	fresh.WingRewards = &domain.RiftWingRewardRules{
		Milestones:  []domain.RiftWingMilestone{{Tier: 30, Item: 25029, Count: 12}, {Tier: 50, Item: 25029, Count: 32}, {Tier: 70, Item: 25029, Count: 2500}},
		RepeatBands: []domain.RiftWingRepeatBand{{MinTier: 1, Item: 25029, Count: 1}, {MinTier: 50, Item: 25029, Count: 2}, {MinTier: 100, Item: 25029, Count: 3}, {MinTier: 150, Item: 25029, Count: 4}},
	}
	source.items[25029] = domain.ItemDef{ID: 25029, Name: "神奇的羽毛", Stackable: true, InventoryTab: 1, InventoryTabKnown: true}
	table, err := domain.NewRiftRuleTable(14, []domain.RiftRules{*riftRulesFixture(), *old, fresh}, source.lootRules, source.items)
	if err != nil {
		t.Fatal(err)
	}
	return table, source
}

func completedWingRift(t *testing.T, tier int32) (*Scene, *riftTestBackend) {
	t.Helper()
	s, b, now := riftFixture(t)
	completedProgressionRift(t, s, b, *now)
	table, source := wingRiftRulesFixture(t)
	s.riftRules, s.lootRules, s.items = table, source.lootRules, source.items
	s.rift.data.Run.Rules, s.rift.data.Run.RulesVersion, s.rift.data.Run.Tier = table.Active(), 14, tier
	repeat, _ := table.Active().WingRepeatReward(tier)
	stacks, err := s.riftWingStacks(repeat.Item, repeat.Count)
	if err != nil {
		t.Fatal(err)
	}
	s.rift.data.Grants[100] = endgameRiftGrant{Items: stacks}
	b.runs[s.rift.data.Run.RunID] = s.rift.data.record(s.rift.revision)
	return s, b
}

func wingGrantCount(grant endgameRiftGrant) int32 {
	var count int32
	for _, st := range grant.Items {
		if st.Item == 25029 {
			count += st.Count
		}
	}
	return count
}

func TestRiftWingGenerationRepeatsOnlyAndFrozenRules(t *testing.T) {
	table, s := wingRiftRulesFixture(t)
	rules := table.Active()
	for _, tier := range []int32{1, 29, 30, 49, 50, 69, 70, 99, 100, 149, 150} {
		for _, timely := range []bool{false, true} {
			prepareCumulativeRun(s, rules, "wing-generation", tier, timely, 60, 60)
			s.generateEndgameRiftGrantsWithContexts(nil)
			want, _ := rules.WingRepeatReward(tier)
			for _, char := range []domain.CharID{100, 200} {
				grant := s.rift.data.Grants[char]
				if !s.rift.data.GrantsReady || wingGrantCount(grant) != want.Count || grant.WingMilestonesResolved {
					t.Fatal("generation reserved first-clear or missed personal repeat", tier, grant)
				}
				for _, st := range grant.Items {
					if st.Item == 25029 && (!st.Bound || st.Count > domain.MaxStack) {
						t.Fatal("invalid material stack", st)
					}
				}
			}
			before, _ := json.Marshal(s.rift.data.Grants)
			s.generateEndgameRiftGrantsWithContexts(nil)
			after, _ := json.Marshal(s.rift.data.Grants)
			if string(before) != string(after) {
				t.Fatal("reward rerolled")
			}
		}
	}
	rules.WingRewards.Milestones[0].Count = 500
	rules.WingRewards.RepeatBands[0].Count = 500
	if table.Active().WingRewards.Milestones[0].Count != 12 || table.Active().WingRewards.RepeatBands[0].Count != 1 {
		t.Fatal("published rule mutated")
	}
	old, _ := table.Version(12)
	if old.WingRewards != nil {
		t.Fatal("historical run retrofitted")
	}
}

func TestRiftWingLiveReservationFullBagAndLostAck(t *testing.T) {
	s, b := completedWingRift(t, 70)
	p := s.players[1]
	p.Player.Bag = domain.NewBag(1)
	p.Player.Bag.Add(s.items[1900028202], domain.MaxStack)
	s.endgamePlayer(p).state.Payload = `{"other":{"keep":true}}`
	s.syncEndgameRiftCompletion()
	if b.commits != 1 {
		t.Fatal("full bag blocked durable entitlement", b.commits)
	}
	if len(riftProfile(s.endgamePlayer(p).state.Payload).WingMilestones) != 0 || wingGrantCount(s.rift.data.Grants[100]) != 2 {
		t.Fatal("entitlement published before commit")
	}
	var next endgameRiftPersisted
	if err := json.Unmarshal([]byte(b.plan.Run.Payload), &next); err != nil {
		t.Fatal(err)
	}
	if wingGrantCount(next.Grants[100]) != 2546 || !next.Grants[100].WingMilestonesResolved || !reflect.DeepEqual(riftProfile(b.plan.State.Payload).WingMilestones, []int32{30, 50, 70}) {
		t.Fatal("missing atomic catch-up package")
	}
	for _, st := range next.Grants[100].Items {
		if !st.Bound || st.Count > domain.MaxStack {
			t.Fatal("unbound or oversized", st)
		}
	}
	// Model a commit whose acknowledgement was lost: restore both durable sides,
	// never run publication callbacks or reserve the same milestone again.
	b.runs[b.plan.Run.RunID] = *b.plan.Run
	snapshot := b.plan.Snapshot.Clone()
	b.commit <- domain.EndgameCommitResult{Authoritative: &snapshot, State: b.plan.State, Receipt: b.plan.Receipt, Replayed: true}
	s.finishEndgame(false)
	s.pollEndgameRiftReload(s.riftMS())
	s.syncEndgameRiftCompletion()
	if wingGrantCount(s.rift.data.Grants[100]) != 2546 || b.commits != 1 || s.rift.data.Run.Members[100].Claimed {
		t.Fatal("lost ACK lost/duplicated entitlement")
	}
	drop := ownedRiftDrop(s, 100)
	if drop == nil {
		t.Fatal("no recoverable ground material")
	}
	p.Pos = drop.Pos
	s.onPickUp(PickUp{ID: p.ID, Target: drop.ID})
	if b.commits != 1 || s.ground[drop.ID] == nil {
		t.Fatal("full bag consumed feather")
	}
	// Reconnect after the instance is gone. Bulk claim uses the frozen remainder.
	d, rev := s.rift.data.clone(), s.rift.revision
	s.rift = nil
	p.Player.Bag = domain.NewBag(domain.DefaultBagSlots)
	req := domain.EndgameRequest{Op: "rift.claim", RequestID: "wing-offline-remainder", ExpectedRevision: strconv.FormatInt(s.endgamePlayer(p).state.Revision, 10)}
	s.claimEndgameRift(p, req, d, rev, false)
	finishRiftRewardCommit(t, s, b)
	if p.Player.Bag.CountOf(25029) != 2546 {
		t.Fatal("offline feather total", p.Player.Bag.CountOf(25029))
	}
	if !reflect.DeepEqual(riftProfile(s.endgamePlayer(p).state.Payload).WingMilestones, []int32{30, 50, 70}) {
		t.Fatal("ledger lost on claim")
	}
	s.claimEndgameRift(p, req, d, rev, false)
	if b.commits != 2 {
		t.Fatal("duplicate click committed")
	}
}

func TestRiftWingOfflineClaimFailureKeepsMilestones(t *testing.T) {
	s, b := completedWingRift(t, 50)
	d, rev := s.rift.data.clone(), s.rift.revision
	s.rift = nil
	p := s.players[1]
	p.Player.Bag = domain.NewBag(1)
	p.Player.Bag.Add(s.items[1900028202], domain.MaxStack)
	req := domain.EndgameRequest{Op: "rift.claim", RequestID: "wing-bag-full-claim", ExpectedRevision: "0"}
	s.claimEndgameRift(p, req, d, rev, false)
	if b.commits != 0 || len(riftProfile(s.endgamePlayer(p).state.Payload).WingMilestones) != 0 || d.Run.Members[100].Claimed {
		t.Fatal("failed claim consumed milestone")
	}
	p.Player.Bag = domain.NewBag(domain.DefaultBagSlots)
	s.claimEndgameRift(p, req, d, rev, false)
	b.finish(false)
	s.finishEndgame(false)
	if p.Player.Bag.CountOf(25029) != 0 || len(riftProfile(s.endgamePlayer(p).state.Payload).WingMilestones) != 0 {
		t.Fatal("storage failure published feather")
	}
	s.claimEndgameRift(p, req, d, rev, false)
	finishRiftRewardCommit(t, s, b)
	if p.Player.Bag.CountOf(25029) != 46 {
		t.Fatal("retry lost first-clear", p.Player.Bag.CountOf(25029))
	}
}

func TestRiftWingPersonalCatchupAndLateDropPlacement(t *testing.T) {
	s, b := completedWingRift(t, 70)
	p := s.players[1]
	s.endgamePlayer(p).state.Payload = withRiftProfile(`{}`, endgameRiftProfile{WingMilestones: []int32{50}})
	s.syncEndgameRiftCompletion()
	first := ownedRiftDrop(s, 100)
	if first == nil {
		t.Fatal("repeat material missing")
	}
	pos := first.Pos
	finishRiftRewardCommit(t, s, b)
	if wingGrantCount(s.rift.data.Grants[100]) != 2514 || first.Pos != pos {
		t.Fatal("catch-up or existing position changed")
	}
	seen := map[domain.Pos]bool{}
	for id := range s.rift.rewardDrops {
		at := s.ground[id].Pos
		if seen[at] {
			t.Fatal("late milestone overlap", at)
		}
		seen[at] = true
	}
	// A different party member has a separate lifetime ledger and gets all tiers.
	d := s.rift.data.clone()
	d.Run.Members[200] = &domain.RiftMember{CharID: 200, Eligible: true}
	d.Grants[200] = endgameRiftGrant{}
	profile := endgameRiftProfile{}
	if err := s.resolveEndgameRiftWingMilestones(&d, 200, &profile); err != nil {
		t.Fatal(err)
	}
	if wingGrantCount(d.Grants[200]) != 2544 || !reflect.DeepEqual(profile.WingMilestones, []int32{30, 50, 70}) {
		t.Fatal("party rewards shared")
	}
	if err := s.resolveEndgameRiftWingMilestones(&d, 200, &profile); err != nil || wingGrantCount(d.Grants[200]) != 2544 {
		t.Fatal("same run re-reserved milestone")
	}
	// Eligibility is checked even on a malformed/manual recovery attempt.
	d.Grants[200] = endgameRiftGrant{}
	d.Run.Members[200].Eligible = false
	if err := s.resolveEndgameRiftWingMilestones(&d, 200, &profile); err == nil {
		t.Fatal("ineligible member rewarded")
	}
}

func TestRiftWingOfflinePartialStacksRemainClaimable(t *testing.T) {
	s, b := completedWingRift(t, 70)
	d, rev := s.rift.data.clone(), s.rift.revision
	s.rift = nil
	p := s.players[1]
	p.Player.Bag = domain.NewBag(1)
	var delivered int32
	for batch := 0; batch < 3; batch++ {
		req := domain.EndgameRequest{Op: "rift.claim", RequestID: fmt.Sprintf("wing-partial-batch-%d", batch), ExpectedRevision: strconv.FormatInt(s.endgamePlayer(p).state.Revision, 10)}
		s.claimEndgameRift(p, req, d, rev, false)
		if b.commits != batch+1 {
			t.Fatal("partial claim made no progress", batch, b.commits)
		}
		// Repeated clicks while the same aggregate is pending cannot stage again.
		s.claimEndgameRift(p, req, d, rev, false)
		if b.commits != batch+1 {
			t.Fatal("pending click created duplicate commit")
		}
		finishRiftRewardCommit(t, s, b)
		got := p.Player.Bag.CountOf(25029)
		want := int32(999)
		if batch == 0 {
			want = 548
		}
		if got != want {
			t.Fatal("partial AddStack leaked from failed probe", batch, got, want)
		}
		delivered += got
		record := b.runs[d.Run.RunID]
		var ok bool
		d, ok = decodeEndgameRift(&record, s.riftRules)
		if !ok {
			t.Fatal("partial grant cannot reload")
		}
		rev = record.Revision
		if d.Run.Members[100].Claimed != (batch == 2) || !d.Grants[100].ExperienceClaimed || !d.Grants[100].WingMilestonesResolved {
			t.Fatal("partial grant closed early or lost resolution", batch)
		}
		if !reflect.DeepEqual(riftProfile(s.endgamePlayer(p).state.Payload).WingMilestones, []int32{30, 50, 70}) {
			t.Fatal("partial claim lost first-clear ledger")
		}
		// The user moves the claimed stack out before continuing the next batch.
		p.Player.Bag = domain.NewBag(1)
	}
	if delivered != 2546 || d.Run.Phase != domain.RiftClosed {
		t.Fatal("batched delivery lost or duplicated feather", delivered, d.Run.Phase)
	}
}

func TestRiftWingPartialClaimDoesNotChangeLegacyRuns(t *testing.T) {
	s, b, now := riftFixture(t)
	completedProgressionRift(t, s, b, *now)
	s.items[25029] = domain.ItemDef{ID: 25029, Stackable: true}
	d, rev := s.rift.data.clone(), s.rift.revision
	d.Grants[100] = endgameRiftGrant{Items: []domain.Stack{{Item: 25029, Count: 999, Bound: true}, {Item: 25029, Count: 999, Bound: true}}}
	s.rift = nil
	p := s.players[1]
	p.Player.Bag = domain.NewBag(1)
	s.claimEndgameRift(p, domain.EndgameRequest{Op: "rift.claim", RequestID: "legacy-full-claim-001", ExpectedRevision: "0"}, d, rev, false)
	if b.commits != 0 || p.Player.Bag.CountOf(25029) != 0 || d.Run.Members[100].Claimed {
		t.Fatal("legacy partial-claim semantics changed")
	}
}

func TestRiftWingDisconnectDuringReservationThenReconnect(t *testing.T) {
	s, b := completedWingRift(t, 50)
	p := s.players[1]
	s.syncEndgameRiftCompletion()
	if b.commits != 1 {
		t.Fatal("reservation not pending")
	}
	s.exec(Leave{ID: p.ID, Reason: "disconnect during wing reservation"})
	if s.players[p.ID] == nil {
		t.Fatal("disconnect escaped inventory lease")
	}
	finishRiftRewardCommit(t, s, b)
	if s.players[p.ID] != nil {
		t.Fatal("disconnect not completed after durability")
	}
	saved := b.runs[s.rift.data.Run.RunID]
	d, ok := decodeEndgameRift(&saved, s.riftRules)
	if !ok || !d.Grants[100].WingMilestonesResolved || wingGrantCount(d.Grants[100]) != 46 {
		t.Fatal("disconnect lost durable wing grant")
	}
	snapshot, state := b.plan.Snapshot.Clone(), b.plan.State
	s.rift = nil
	p = reconnectRiftPlayer(t, s, snapshot, 900000, 0, nil, &fakeSink{})
	s.endgamePlayer(p).loaded, s.endgamePlayer(p).state = true, state
	req := domain.EndgameRequest{Op: "rift.claim", RequestID: "wing-reconnect-claim", ExpectedRevision: strconv.FormatInt(state.Revision, 10)}
	s.claimEndgameRift(p, req, d, saved.Revision, false)
	finishRiftRewardCommit(t, s, b)
	if p.Player.Bag.CountOf(25029) != 46 || !reflect.DeepEqual(riftProfile(s.endgamePlayer(p).state.Payload).WingMilestones, []int32{30, 50}) {
		t.Fatal("reconnect duplicate/lost feather")
	}
}

func TestRiftWingFailedRunDoesNotAwardFeathers(t *testing.T) {
	s, b := completedWingRift(t, 70)
	d, rev := s.rift.data.clone(), s.rift.revision
	d.Run.Phase, d.Run.Reason = domain.RiftFailed, "timeout"
	s.rift = nil
	p := s.players[1]
	s.claimEndgameRift(p, domain.EndgameRequest{Op: "rift.claim", RequestID: "wing-failed-claim-01", ExpectedRevision: "0"}, d, rev, false)
	if b.commits != 0 || p.Player.Bag.CountOf(25029) != 0 || len(riftProfile(s.endgamePlayer(p).state.Payload).WingMilestones) != 0 {
		t.Fatal("failed run awarded wing material")
	}
}

func TestRiftWingExplicitLayoutRebuildDoesNotReuseCachedMask(t *testing.T) {
	s, b := completedWingRift(t, 50)
	s.syncEndgameRiftCompletion()
	finishRiftRewardCommit(t, s, b)
	if s.rift.rewardLayout == nil || len(s.rift.rewardLayout.Positions) == 0 {
		t.Fatal("initial layout missing")
	}
	center := s.endgameRiftRewardPosition()
	s.walkable = func(pos domain.Pos) bool { return pos == center }
	npc, ready, positions := s.endgameRiftCompletionLayout()
	if !ready || npc != center || len(positions) != 0 {
		t.Fatal("explicit rebuild reused cached geometry", npc, positions)
	}
}
