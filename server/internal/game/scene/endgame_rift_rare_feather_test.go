package scene

import (
	"encoding/json"
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"testing"
)

func rareFeatherCount(g endgameRiftGrant) int32 {
	var n int32
	for _, st := range g.Items {
		if st.Item == 11682 {
			n += st.Count
		}
	}
	return n
}
func enableRareFeather(s *Scene, r *domain.RiftRules) {
	r.WingRewards.RareFeather = &domain.RiftRareFeatherRule{Item: 11682, Count: 1, ChanceBP: 50}
	s.items[11682] = domain.ItemDef{ID: 11682, Name: "暗之羽", Stackable: true, InventoryTab: 0, InventoryTabKnown: true}
	rows := []domain.RiftRules{}
	for _, version := range []int32{1, 12} {
		if old, ok := s.riftRules.Version(version); ok {
			rows = append(rows, *old)
		}
	}
	rows = append(rows, *r)
	var err error
	s.riftRules, err = domain.NewRiftRuleTable(14, rows, s.lootRules, s.items)
	if err != nil {
		panic(err)
	}

}
func findRareRun(s *Scene, char domain.CharID, want bool) string {
	for i := 0; i < 10000; i++ {
		s.rift.data.Run.RunID = fmt.Sprintf("%032x", i)
		st, _ := s.riftRareFeatherStacks(char)
		if (len(st) > 0) == want {
			return s.rift.data.Run.RunID
		}
	}
	panic("missing deterministic rare fixture")
}
func TestRiftRareFeatherOneRollIndependentMFHeightMemberAndOtherRewards(t *testing.T) {
	table, s := wingRiftRulesFixture(t)
	rules := table.Active()
	enableRareFeather(s, rules)
	prepareCumulativeRun(s, rules, "rare-find", 1, false, 60, 60)
	s.rift.data.Run.Phase = domain.RiftCompleted
	hit := findRareRun(s, 100, true)
	miss := findRareRun(s, 100, false)
	for _, id := range []string{hit, miss} {
		for _, tier := range []int32{1, 150} {
			for _, mf := range []int64{0, 1000000} {
				prepareCumulativeRun(s, rules, id, tier, false, 60, 60)
				s.rift.data.Run.Phase = domain.RiftCompleted
				expected := map[domain.CharID]int32{}
				for _, char := range []domain.CharID{100, 200} {
					x, _ := s.riftRareFeatherStacks(char)
					if len(x) > 0 {
						expected[char] = 1
					}
				}
				// Unrelated consumption cannot advance this dedicated stream.
				other := s.riftCumulativeRandom(100, 0, "values")
				for i := 0; i < 100; i++ {
					other.Intn(10000)
				}
				s.generateEndgameRiftGrantsWithContexts(map[domain.CharID]endgameRiftRewardContext{100: {Level: 60, MagicFind: mf}, 200: {Level: 60, MagicFind: mf}})
				if !s.rift.data.GrantsReady {
					t.Fatal("generation failed")
				}
				for _, char := range []domain.CharID{100, 200} {
					if got := rareFeatherCount(s.rift.data.Grants[char]); got != expected[char] {
						t.Fatal("member/rate changed", char, got, expected[char])
					}
				}
				if (rareFeatherCount(s.rift.data.Grants[100]) == 1) != (id == hit) {
					t.Fatal("tier/MF changed rare roll")
				}
				raw, _ := json.Marshal(s.rift.data.Grants)
				s.generateEndgameRiftGrantsWithContexts(nil)
				after, _ := json.Marshal(s.rift.data.Grants)
				if string(raw) != string(after) {
					t.Fatal("ready grants rerolled")
				}
			}
		}
	}
	// A single shared run can independently hit one member and miss another.
	prepareCumulativeRun(s, rules, hit, 150, false, 60, 60)
	s.rift.data.Run.Phase = domain.RiftCompleted
	independent := false
	for i := 0; i < 10000; i++ {
		s.rift.data.Run.RunID = fmt.Sprintf("%032x", i)
		a, _ := s.riftRareFeatherStacks(100)
		b, _ := s.riftRareFeatherStacks(200)
		if len(a) != len(b) {
			independent = true
			break
		}
	}
	if !independent {
		t.Fatal("party shared a rare roll")
	}
	s.rift.data.Run.Phase = domain.RiftFailed
	if x, _ := s.riftRareFeatherStacks(100); len(x) != 0 {
		t.Fatal("failed run awarded rare")
	}
	s.rift.data.Run.Phase = domain.RiftCompleted
	s.rift.data.Run.Members[100].Eligible = false
	if x, _ := s.riftRareFeatherStacks(100); len(x) != 0 {
		t.Fatal("ineligible member awarded rare")
	}
}
func TestRiftRareFeatherFrozenGrantFullBagStorageRetryAndReplay(t *testing.T) {
	s, b := completedWingRift(t, 50)
	p := s.players[1]
	enableRareFeather(s, s.rift.data.Run.Rules)
	findRareRun(s, 100, true)
	st, err := s.riftRareFeatherStacks(100)
	if err != nil || len(st) != 1 || !st[0].Bound {
		t.Fatal(st, err)
	}
	grant := s.rift.data.Grants[100]
	grant.Items = st
	s.rift.data.Grants[100] = grant
	d, rev := s.rift.data.clone(), s.rift.revision
	s.rift = nil
	p.Player.Bag = domain.NewBag(1)
	p.Player.Bag.Set(0, domain.Stack{Item: 1900028202, Count: 999})
	req := domain.EndgameRequest{Op: "rift.claim", RequestID: "rare-feather-retry", ExpectedRevision: "0"}
	s.claimEndgameRift(p, req, d, rev, false)
	if b.commits != 0 {
		t.Fatal("full bag consumed rare")
	}
	p.Player.Bag = domain.NewBag(domain.DefaultBagSlots)
	s.claimEndgameRift(p, req, d, rev, false)
	if b.commits != 1 {
		t.Fatal("missing staged reward")
	}
	b.finish(false)
	s.finishEndgame(false)
	if p.Player.Bag.CountOf(11682) != 0 || rareFeatherCount(d.Grants[100]) != 1 {
		t.Fatal("storage failure lost or published rare")
	}
	s.claimEndgameRift(p, req, d, rev, false)
	finishRiftRewardCommit(t, s, b)
	if p.Player.Bag.CountOf(11682) != 1 {
		t.Fatal("retry changed rare")
	}
	s.claimEndgameRift(p, req, d, rev, false)
	if b.commits != 2 || p.Player.Bag.CountOf(11682) != 1 {
		t.Fatal("replay duplicated rare")
	}
}

func TestRiftRareFeatherDisconnectKeepsFrozenResult(t *testing.T) {
	s, b := completedWingRift(t, 50)
	p := s.players[1]
	enableRareFeather(s, s.rift.data.Run.Rules)
	findRareRun(s, 100, true)
	stacks, _ := s.riftRareFeatherStacks(100)
	grant := s.rift.data.Grants[100]
	grant.Items = stacks
	s.rift.data.Grants[100] = grant
	b.runs[s.rift.data.Run.RunID] = s.rift.data.record(s.rift.revision)
	s.syncEndgameRiftCompletion()
	if b.commits != 1 {
		t.Fatal("missing entitlement transaction")
	}
	s.exec(Leave{ID: p.ID, Reason: "rare feather reservation disconnect"})
	if s.players[p.ID] == nil {
		t.Fatal("disconnect escaped lease")
	}
	finishRiftRewardCommit(t, s, b)
	saved := b.runs[s.rift.data.Run.RunID]
	d, ok := decodeEndgameRift(&saved, s.riftRules)
	if !ok {
		t.Fatal("frozen rare grant cannot decode")
	}
	if rareFeatherCount(d.Grants[100]) != 1 {
		t.Fatal("disconnect lost frozen rare")
	}
	snapshot, state := b.plan.Snapshot.Clone(), b.plan.State
	s.rift = nil
	p = reconnectRiftPlayer(t, s, snapshot, 900000, 0, nil, &fakeSink{})
	s.endgamePlayer(p).loaded, s.endgamePlayer(p).state = true, state
	req := domain.EndgameRequest{Op: "rift.claim", RequestID: "rare-reconnect-claim", ExpectedRevision: fmt.Sprint(state.Revision)}
	s.claimEndgameRift(p, req, d, saved.Revision, false)
	finishRiftRewardCommit(t, s, b)
	if p.Player.Bag.CountOf(11682) != 1 {
		t.Fatal("reconnect changed rare result")
	}
}
