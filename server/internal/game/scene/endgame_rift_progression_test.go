package scene

import catalogfixture "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/testfixture/catalog"

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func completedProgressionRift(t *testing.T, s *Scene, b *riftTestBackend, now int64) {
	t.Helper()
	activateTestRift(t, s, b, now)
	s.stepEndgameRift()
	x := s.rift
	x.data.Run.Tier = 20
	x.data.Run.Phase = domain.RiftCompleted
	x.data.Run.CompletedMS = x.data.Run.StartedMS + domain.RiftTimedMS - 540000
	x.data.Run.Members[100].Eligible = true
	x.data.GrantsReady = true
	x.data.Grants[100] = endgameRiftGrant{}
	x.savedRevision = x.data.Run.Revision
	b.runs[x.data.Run.RunID] = x.data.record(x.revision)
}

func TestRiftCompletedSnapshotShowsFrozenUnlockWithoutClaim(t *testing.T) {
	s, b, now := riftFixture(t)
	completedProgressionRift(t, s, b, *now)
	d := s.rift.data
	d.Floor = &domain.RiftFloorPlan{FloorIndex: 8}
	p := s.players[1]
	for _, live := range []bool{true, false} {
		*now += 24 * 60 * 60 * 1000
		v := s.riftView(p, d, true, live)
		if v.UnlockedTier != 23 || v.Tier != 20 || v.FloorIndex != 8 || v.RemainingSeconds != 540 || v.MaxTier != 150 {
			t.Fatalf("live=%v: %+v", live, v)
		}
	}
	if v := s.riftView(p, d, false, true); v.UnlockedTier != 1 || v.Phase != "reward_pending" {
		t.Fatalf("undurable completion exposed new unlock: %+v", v)
	}
	s.endgamePlayer(p).state.Payload = withRiftProfile(`{"other":{"keep":true}}`, endgameRiftProfile{UnlockedTier: 99})
	if v := s.riftView(p, d, true, true); v.UnlockedTier != 99 {
		t.Fatalf("higher personal unlock regressed: %+v", v)
	}
}

func TestRiftClaimPersistsFrozenUnlockAndReceiptOnce(t *testing.T) {
	s, b, now := riftFixture(t)
	completedProgressionRift(t, s, b, *now)
	d, runRevision := s.rift.data.clone(), s.rift.revision
	s.rift = nil // Bulk claim is the offline/closed-instance recovery path.
	p := s.players[1]
	s.endgamePlayer(p).state.Payload = `{"other":{"keep":true}}`
	*now += 24 * 60 * 60 * 1000
	req := domain.EndgameRequest{Op: "rift.claim", RequestID: "tier-claim-0000001", ExpectedRevision: "0"}
	s.claimEndgameRift(p, req, d, runRevision, false)
	if riftProfile(s.endgamePlayer(p).state.Payload).UnlockedTier != 1 || riftProfile(b.plan.State.Payload).UnlockedTier != 23 {
		t.Fatal("unlock was not staged atomically")
	}
	var receipt domain.EndgameResponse
	if err := json.Unmarshal([]byte(b.plan.Receipt.Response), &receipt); err != nil || receipt.Rift.UnlockedTier != 23 || receipt.Rift.RemainingSeconds != 540 {
		t.Fatalf("incorrect persisted receipt: %+v err=%v", receipt, err)
	}
	b.finish(true)
	s.finishEndgame(false)
	if riftProfile(s.endgamePlayer(p).state.Payload).UnlockedTier != 23 {
		t.Fatal("saved profile missing unlock")
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(s.endgamePlayer(p).state.Payload), &payload); err != nil || string(payload["other"]) != `{"keep":true}` {
		t.Fatal("unrelated payload overwritten")
	}
	saved := b.runs[d.Run.RunID]
	d, _ = decodeEndgameRift(&saved, riftRulesTableFixture())
	s.claimEndgameRift(p, req, d, saved.Revision, false)
	if b.commits != 1 {
		t.Fatal("repeated settlement increased unlock")
	}
}

func TestRiftLeavePersistsCompletionWithoutClaimAndFailureRollsBack(t *testing.T) {
	for _, success := range []bool{false, true} {
		t.Run(map[bool]string{false: "storage_failure", true: "success"}[success], func(t *testing.T) {
			s, b, now := riftFixture(t)
			completedProgressionRift(t, s, b, *now)
			p := s.players[1]
			s.leaveEndgameRift(p, domain.EndgameRequest{Op: "rift.leave", RequestID: "tier-leave-0000001", ExpectedRevision: "0"})
			if b.commits != 1 || riftProfile(b.plan.State.Payload).UnlockedTier != 23 {
				t.Fatal("leave did not include earned unlock")
			}
			b.finish(success)
			s.finishEndgame(false)
			want := int32(1)
			if success {
				want = 23
			}
			if got := riftProfile(s.endgamePlayer(p).state.Payload).UnlockedTier; got != want {
				t.Fatalf("got=%d want=%d", got, want)
			}
			if s.rift.data.Run.Members[100].Claimed {
				t.Fatal("leaving implicitly consumed rewards")
			}
		})
	}
}

func TestRiftFailedLeaveDoesNotUnlock(t *testing.T) {
	s, b, now := riftFixture(t)
	activateTestRift(t, s, b, *now)
	s.stepEndgameRift()
	s.rift.data.Run.Tier = 20
	s.leaveEndgameRift(s.players[1], domain.EndgameRequest{Op: "rift.leave", RequestID: "tier-abandon-00001", ExpectedRevision: "0"})
	if got := riftProfile(b.plan.State.Payload).UnlockedTier; got != 1 {
		t.Fatalf("abandon granted unlock=%d", got)
	}
}

func TestRiftOfflineTeamMemberClaimsOwnProgress(t *testing.T) {
	s, b, now := riftFixture(t)
	completedProgressionRift(t, s, b, *now)
	d := s.rift.data.clone()
	d.Run.Members[200] = &domain.RiftMember{CharID: 200, Level: 60, Eligible: true}
	d.Grants[200] = endgameRiftGrant{}
	d.Run.Members[100].Claimed = true
	d.Run.Phase = domain.RiftClosed
	record := d.record(10)
	d, ok := decodeEndgameRift(&record, riftRulesTableFixture())
	if !ok {
		t.Fatal("completed record did not survive persistence")
	}
	s.rift = nil
	join(t, s, 2, 200, "乙", 105, 105)
	p := s.players[2]
	p.Player.Char.Level = 60
	s.endgamePlayer(p).loaded = true
	s.endgamePlayer(p).state.Payload = withRiftProfile("{}", endgameRiftProfile{RunID: d.Run.RunID, UnlockedTier: 1})
	*now += 24 * 60 * 60 * 1000
	s.claimEndgameRift(p, domain.EndgameRequest{Op: "rift.claim", RequestID: "tier-offline-00001", ExpectedRevision: "0"}, d, record.Revision, false)
	if b.commits != 1 || b.plan.Receipt.CharID != 200 || riftProfile(b.plan.State.Payload).UnlockedTier != 23 {
		t.Fatalf("offline member progress missing: %+v", b.plan)
	}
	b.finish(true)
	s.finishEndgame(false)
	if riftProfile(s.endgamePlayer(p).state.Payload).UnlockedTier != 23 || riftProfile(s.endgamePlayer(s.players[1]).state.Payload).UnlockedTier != 1 {
		t.Fatal("team member overwrote another member's profile")
	}
	s.flush()
	found := false
	for _, e := range p.Player.Sink.(*fakeSink).take() {
		if message, ok := e.(event.EndgameMessage); ok && message.Message.Rift != nil && message.Message.OK && message.Message.Rift.UnlockedTier == 23 {
			found = true
		}
	}
	if !found {
		t.Fatal("offline settlement did not publish unlock")
	}
}

func TestRiftDelayedGuardianDeathFreezesAuthoritativeTimeAndOrder(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		before, after, disconnect bool
		remaining                 int64
		want                      int32
	}{
		{name: "save_delay_crosses_hard_timeout", remaining: 360, want: 360},
		{name: "prior_player_death_penalty", before: true, remaining: 360, want: 355},
		{name: "later_player_death_no_penalty", after: true, remaining: 360, want: 360},
		{name: "disconnect_after_guardian", disconnect: true, remaining: 360, want: 360},
		{name: "inclusive_timed_boundary", remaining: 0, want: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, b, now := riftFixture(t)
			activateTestRift(t, s, b, *now)
			s.stepEndgameRift()
			x := s.rift
			x.data.Run.Tier = 20
			x.data.Run.Phase = domain.RiftBoss
			const boss domain.EntityID = 987654
			if !x.data.Run.RegisterMonster(boss, 3) {
				t.Fatal("boss registration")
			}
			p := s.players[1]
			*now = x.data.Run.StartedMS + domain.RiftTimedMS - tc.remaining*1000
			completed := *now
			x.economicPending = true
			if tc.before {
				s.onEndgameRiftPlayerDeath(p)
			}
			s.onEndgameRiftMonsterDeath(&entity.Entity{ID: boss, Pos: p.Pos}, p)
			if tc.after {
				s.onEndgameRiftPlayerDeath(p)
			}
			if tc.disconnect {
				delete(x.ready, 100)
			}
			*now += domain.RiftHardMS + 1000
			x.economicPending = false
			s.stepEndgameRift()
			r := x.data.Run
			if r.Phase != domain.RiftCompleted || r.CompletedMS != completed || r.RemainingSeconds(*now) != tc.want || !r.Members[100].Eligible {
				t.Fatalf("wrong delayed outcome: phase=%s at=%d remaining=%d eligible=%v", r.Phase, r.CompletedMS, r.RemainingSeconds(*now), r.Members[100].Eligible)
			}
			if got, want := r.UnlockedTierFor(100, 1), 20+domain.RiftTierJump(tc.want, riftRulesFixture()); got != want {
				t.Fatalf("unlocked=%d want=%d", got, want)
			}
			if len(x.delayedDeaths) != 0 {
				t.Fatal("delayed deaths were not drained")
			}
		})
	}
}

func TestRiftPriorClaimedCompletionAdmitsAndPersistsProjectedTierAtomically(t *testing.T) {
	for _, tc := range []struct {
		name                              string
		prior, tier                       int32
		noKey, fail, failedRun, nonMember bool
		wantError                         string
	}{
		{name: "projected_tier", prior: 21, tier: 23},
		{name: "higher_history", prior: 90, tier: 90},
		{name: "no_key", prior: 21, tier: 23, noKey: true, wantError: "key"},
		{name: "commit_failure", prior: 21, tier: 23, fail: true},
		{name: "failed_run", prior: 21, tier: 23, failedRun: true, wantError: "tier"},
		{name: "nonmember", prior: 21, tier: 23, nonMember: true, wantError: "not_owner"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, b, now := riftFixture(t)
			frames := make(chan time.Time)
			router := NewRouter(func(id domain.SceneID) (*Scene, error) {
				return New(Config{EndgameCatalog: catalogfixture.EndgameCatalog(), ID: id, Frames: frames, Trial: s.trial, Dungeons: s.dungeons}), nil
			}, nil)
			router.Start(context.Background())
			defer router.Drain(time.Second)
			s.router = router
			s.riftLayouts = riftFloorTemplatesForTest()[:1]
			// The configured rift entrance is independent from native trial guards.
			s.entities[77] = &entity.Entity{ID: 77, Kind: domain.KindNPC, Name: "八仙使者", Pos: domain.Pos{MapID: 7, X: 100, Y: 100}, NPC: &entity.NPCInfo{ConfigID: 83, Script: "Nbaxian"}}
			p := s.players[1]
			p.Player.QuestNPC = "八仙使者"
			if !tc.noKey {
				p.Player.Bag.AddStack(s.items[domain.RiftKeyItemID], domain.Stack{Item: domain.RiftKeyItemID, Count: 1, Bound: true})
			}
			r, err := domain.NewRiftRun("old-claimed-progress", 20, 100, []domain.TrialKeyMember{{CharID: 100, Level: 60}}, *now, riftRulesFixture())
			if err != nil {
				t.Fatal(err)
			}
			r.Activate(*now)
			r.CompletedMS = r.StartedMS + domain.RiftTimedMS - 540000
			r.Phase = domain.RiftClosed
			r.Members[100].Eligible, r.Members[100].Claimed = true, true
			if tc.failedRun {
				r.Phase = domain.RiftFailed
			}
			if tc.nonMember {
				delete(r.Members, 100)
			}
			d := endgameRiftPersisted{Version: 2, Run: r, GrantsReady: true}
			b.runs[r.RunID] = d.record(1)
			s.endgamePlayer(p).state.Payload = withRiftProfile("{}", endgameRiftProfile{RunID: r.RunID, UnlockedTier: tc.prior})
			s.onEndgameRift(p, domain.EndgameRequest{Op: "rift.start", NPCEntityID: 77, RequestID: "start-tier-upgrade-01", ExpectedRevision: "0", Tier: tc.tier})
			s.stepEndgameRiftRequests()
			if got := riftProfile(s.endgamePlayer(p).state.Payload).UnlockedTier; got != tc.prior {
				t.Fatalf("read/start mutated profile early: %d", got)
			}
			if tc.wantError != "" {
				if b.commits != 0 {
					t.Fatal("rejected start committed")
				}
				requireRiftFloorError(t, s, p, tc.wantError)
				return
			}
			want := max(int32(23), tc.prior)
			if b.commits != 1 || riftProfile(b.plan.State.Payload).UnlockedTier != want {
				t.Fatalf("progress not in start candidate: commits=%d payload=%s", b.commits, b.plan.State.Payload)
			}
			var receipt domain.EndgameResponse
			if json.Unmarshal([]byte(b.plan.Receipt.Response), &receipt) != nil || receipt.Rift.UnlockedTier != want || receipt.Rift.Tier != tc.tier {
				t.Fatal("start receipt lost projected tier")
			}
			b.finish(!tc.fail)
			s.finishEndgame(false)
			if tc.fail {
				want = tc.prior
			}
			if got := riftProfile(s.endgamePlayer(p).state.Payload).UnlockedTier; got != want {
				t.Fatalf("saved unlock=%d want=%d", got, want)
			}
		})
	}
}
