package scene

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"testing"
	"time"
)

type riftTestBackend struct {
	runLoad  <-chan domain.EndgameRunLoadResult
	runs     map[string]domain.EndgameRunRecord
	plan     domain.EndgameCommitPlan
	commit   chan domain.EndgameCommitResult
	commits  int
	saveFail bool
}

func (b *riftTestBackend) Load(int64) <-chan domain.EndgameLoadResult {
	c := make(chan domain.EndgameLoadResult, 1)
	c <- domain.EndgameLoadResult{}
	return c
}
func (b *riftTestBackend) LoadReceipt(int64, string) <-chan domain.EndgameReceiptLoadResult {
	c := make(chan domain.EndgameReceiptLoadResult, 1)
	c <- domain.EndgameReceiptLoadResult{}
	return c
}
func (b *riftTestBackend) LoadRun(id string) <-chan domain.EndgameRunLoadResult {
	if b.runLoad != nil {
		return b.runLoad
	}
	c := make(chan domain.EndgameRunLoadResult, 1)
	if r, ok := b.runs[id]; ok {
		c <- domain.EndgameRunLoadResult{Run: &r}
	} else {
		c <- domain.EndgameRunLoadResult{}
	}
	return c
}
func (b *riftTestBackend) Commit(p domain.EndgameCommitPlan) <-chan domain.EndgameCommitResult {
	b.plan = p
	b.commits++
	b.commit = make(chan domain.EndgameCommitResult, 1)
	return b.commit
}
func (b *riftTestBackend) SaveRun(r domain.EndgameRunRecord, rev int64) <-chan domain.EndgameRunSaveResult {
	c := make(chan domain.EndgameRunSaveResult, 1)
	if b.saveFail {
		c <- domain.EndgameRunSaveResult{Err: errors.New("offline")}
	} else {
		b.runs[r.RunID] = r
		c <- domain.EndgameRunSaveResult{Run: &r}
	}
	return c
}
func (b *riftTestBackend) finish(ok bool) {
	if !ok {
		b.commit <- domain.EndgameCommitResult{Err: errors.New("failure")}
		return
	}
	if b.plan.Run != nil {
		b.runs[b.plan.Run.RunID] = *b.plan.Run
	}
	b.commit <- domain.EndgameCommitResult{State: b.plan.State, Receipt: b.plan.Receipt}
}
func riftFixture(t *testing.T) (*Scene, *riftTestBackend, *int64) {
	t.Helper()
	s := newTestScene(t, newFakeSaver())
	now := int64(1000000)
	s.now = func() time.Time { return time.UnixMilli(now) }
	s.alloc = domain.NewEntityAlloc()
	s.items = map[domain.ItemID]domain.ItemDef{}
	s.items[domain.RiftKeyItemID] = domain.ItemDef{ID: domain.RiftKeyItemID, Name: "洞天符钥", InventoryTab: 1, InventoryTabKnown: true, Stackable: true}
	s.items[1900028202] = domain.ItemDef{ID: 1900028202, Name: "净煞尘", InventoryTab: 1, InventoryTabKnown: true, Stackable: true}
	for _, d := range domain.AstralRuneItems(7) {
		s.items[d.ID] = d
	}
	var u domain.UniqueDefinition
	for _, candidate := range domain.UniqueCatalog() {
		if candidate.NativeDropEligible {
			u = candidate
			break
		}
	}
	s.items[u.BaseID] = domain.ItemDef{ID: u.BaseID, Name: "测试法器", InventoryTab: 2, InventoryTabKnown: true, InstanceKind: domain.ItemInstanceEquipment, Equip: &domain.EquipDef{Slot: 1, LevelReq: 12, Durable: 100}}
	s.trial = &domain.TrialRules{Quest: 99, MinLevel: 12, MaxLevel: 60, AnchorLevel: 50, AnchorUnits: 16, Maps: map[int32][]domain.TrialSpawn{7: {{Point: domain.SpawnPoint{ID: 1, Pos: domain.Pos{MapID: 7, X: 100, Y: 100}}, Candidates: []domain.MonsterID{1}}, {Point: domain.SpawnPoint{ID: 2, Pos: domain.Pos{MapID: 7, X: 110, Y: 110}}, Candidates: []domain.MonsterID{2}}}}, Monsters: map[int32]map[domain.MonsterID]domain.MonsterDef{60: {1: {ID: 1, Name: "妖影", Kind: domain.MonsterNormal, Level: 60, HP: 100}, 2: {ID: 2, Name: "劫影", Kind: domain.MonsterBoss, Level: 60, HP: 1000}}}, Multipliers: []int32{100, 100, 100, 100, 100, 100}}
	s.dungeons = map[int32]domain.DungeonDef{7: {Enter: domain.Pos{MapID: 7, X: 100, Y: 100}}}
	s.router = NewRouter(nil, nil)
	s.router.scenes[s.id] = s
	join(t, s, 1, 100, "甲", 100, 100)
	p := s.players[1]
	p.Player.Char.Level = 60
	p.Player.Char.BagSlots = domain.DefaultBagSlots
	p.Player.Bag = domain.NewBag(domain.DefaultBagSlots)
	p.HP = 100
	p.MaxHP = 100
	p.Player.Char.Quests = domain.QuestLog{99: {State: domain.QuestActive}}
	s.endgamePlayer(p).loaded = true
	b := &riftTestBackend{runs: map[string]domain.EndgameRunRecord{}}
	s.endgameBackend = b
	return s, b, &now
}
func activateTestRift(t *testing.T, s *Scene, b *riftTestBackend, now int64) {
	t.Helper()
	r, err := domain.NewRiftRun("test-rift", 1, 100, []domain.TrialKeyMember{{CharID: 100, Level: 60}}, now)
	if err != nil {
		t.Fatal(err)
	}
	d := endgameRiftPersisted{Version: 1, Run: r, MapID: 7, Return: domain.Pos{MapID: 8}, Names: map[domain.CharID]string{100: "甲"}, Profiles: map[domain.CharID]endgameRiftIdentity{}, Grants: map[domain.CharID]endgameRiftGrant{}}
	b.runs[r.RunID] = d.record(1)
	s.initEndgameRift(d)
	s.syncEndgameRift(s.players[1])
	s.stepEndgameRift()
}
func TestRiftNativePopulationProgressGuardianClaim(t *testing.T) {
	s, b, now := riftFixture(t)
	activateTestRift(t, s, b, *now)
	if len(s.monsters) != 104 {
		t.Fatal(len(s.monsters))
	}
	counts := map[int32]int{}
	var monsters []*entity.Entity
	for id, m := range s.monsters {
		class := s.rift.data.Run.Monsters[id].Class
		counts[class]++
		if (class == 1 || class == 2) && m.Monster.Kind != domain.MonsterElite {
			t.Fatal("elite combat context")
		}
		monsters = append(monsters, m)
	}
	if counts[0] != 80 || counts[1] != 18 || counts[2] != 6 {
		t.Fatal(counts)
	}
	for _, m := range monsters {
		if s.rift.data.Run.Phase != domain.RiftActive {
			break
		}
		*now += 100
		m.HP = 0
		s.onDeath(m, 1)
		s.stepEndgameRift()
	}
	if s.rift.data.Run.Phase != domain.RiftBoss {
		t.Fatal(s.rift.data.Run.Phase, s.rift.data.Run.Progress)
	}
	if len(s.monsters) != 1 {
		t.Fatal(len(s.monsters))
	}
	for _, m := range s.monsters {
		*now += 100
		m.HP = 0
		s.onDeath(m, 1)
	}
	s.stepEndgameRift()
	if s.riftView(s.players[1], s.rift.data, false, true).Phase != "reward_pending" {
		t.Fatal("published early")
	}
	s.stepEndgameRift()
	p := s.players[1]
	if !s.riftView(p, s.rift.data, true, true).Claimable {
		t.Fatal("no credit")
	}
	before := p.Player.Bag.CountOf(1900028202)
	req := domain.EndgameRequest{Op: "rift.claim", RequestID: "claim-natural-0001", ExpectedRevision: "0"}
	s.onEndgameRift(p, req)
	if b.commits != 1 || p.Player.Bag.CountOf(1900028202) != before {
		t.Fatal("premature grant")
	}
	b.finish(true)
	s.finishEndgame(false)
	if p.Player.Bag.CountOf(1900028202) != 10 || !s.rift.data.Run.Members[100].Claimed {
		t.Fatal("claim failed")
	}
	if s.rift.data.Run.Phase != domain.RiftClosed {
		t.Fatal("sequential run blocked")
	}
}
func TestRiftOrphanCompensationOnceAndClosedProjection(t *testing.T) {
	s, b, now := riftFixture(t)
	r, _ := domain.NewRiftRun("orphan", 1, 100, []domain.TrialKeyMember{{CharID: 100, Level: 60}}, *now)
	r.Activate(*now)
	d := endgameRiftPersisted{Version: 1, Run: r, Names: map[domain.CharID]string{100: "甲"}}
	p := s.players[1]
	view := s.riftView(p, d, true, false)
	if view.Phase != "interrupted" || !view.Claimable {
		t.Fatal(view)
	}
	s.claimEndgameRift(p, domain.EndgameRequest{Op: "rift.claim", RequestID: "compensation-001", ExpectedRevision: "0"}, d, 1, false)
	b.finish(true)
	s.finishEndgame(false)
	if p.Player.Bag.CountOf(domain.RiftKeyItemID) != 1 {
		t.Fatal("missing compensation")
	}
	saved := b.runs[r.RunID]
	d, _ = decodeEndgameRift(&saved)
	s.claimEndgameRift(p, domain.EndgameRequest{Op: "rift.claim", RequestID: "compensation-002", ExpectedRevision: "1"}, d, 2, false)
	if b.commits != 1 {
		t.Fatal("double refund")
	}
	d.Run.Phase = domain.RiftCompleted
	d.Run.Members[100].Eligible = true
	if v := s.riftView(p, d, true, false); v.Phase != "closed" || !v.Claimable {
		t.Fatal(v)
	}
}
func TestRiftTrialOfflineEntitlementAndSameTick(t *testing.T) {
	for _, instant := range []bool{true, false} {
		t.Run(map[bool]string{true: "same_tick", false: "offline"}[instant], func(t *testing.T) {
			s, b, now := riftFixture(t)
			s.trialLevel = 60
			join(t, s, 2, 200, "乙", 105, 105)
			p := s.players[2]
			p.Player.Char.Level = 50
			p.Player.Char.Quests = domain.QuestLog{99: {State: domain.QuestActive}}
			s.players[1].Player.Char.Trial.Cycle = 5
			s.beginEndgameTrial()
			if !instant {
				*now += 10000
				s.stepEndgameTrial()
				delete(s.players, 2)
				*now += 1000
			}
			s.finishEndgameTrialKeys()
			s.stepEndgameTrial()
			if !s.trialRun.Saved {
				t.Fatal("no durable completion")
			}
			if s.trialRun.Keys[100] != 8 || s.trialRun.Keys[200] != 2 {
				t.Fatal(s.trialRun.Keys)
			}
			nonce := p.Player.Char.Trial.CompletionID
			s.trialRun = nil
			s.players[2] = p
			s.recoverEndgameTrial(p)
			s.stepEndgameTrial()
			if p.Player.Char.Trial.CompletionID != nonce || p.Player.Char.Trial.KeyReward != 2 || p.Player.Char.Quests[99].State != domain.QuestDeliver {
				t.Fatal("offline claim lost")
			}
			if len(b.runs) != 1 {
				t.Fatal(b.runs)
			}
		})
	}
}
func TestRiftPreviousRewardCannotBeHidden(t *testing.T) {
	s, b, now := riftFixture(t)
	r, _ := domain.NewRiftRun("owed", 1, 100, []domain.TrialKeyMember{{CharID: 100, Level: 60}}, *now)
	r.Phase = domain.RiftCompleted
	r.Members[100].Eligible = true
	d := endgameRiftPersisted{Version: 1, Run: r}
	b.runs[r.RunID] = d.record(1)
	p := s.players[1]
	s.endgamePlayer(p).state.Payload = withRiftProfile("{}", endgameRiftProfile{RunID: r.RunID, UnlockedTier: 1})
	s.onEndgameRift(p, domain.EndgameRequest{Op: "rift.start", RequestID: "start-another-01", Tier: 1})
	s.stepEndgameRiftRequests()
	if b.commits != 0 {
		t.Fatal("overwrote entitlement")
	}
	s.flush()
	sink := p.Player.Sink.(*fakeSink)
	found := false
	for _, e := range sink.take() {
		if message, ok := e.(event.EndgameMessage); ok && message.Message.OK && message.Message.Rift != nil && message.Message.Rift.RunID == "owed" {
			found = true
		}
	}
	if !found {
		t.Fatal("old claim inaccessible")
	}
	encoded, _ := json.Marshal(d)
	if len(encoded) == 0 {
		t.Fatal("invalid record")
	}
}

func TestRiftClaimFailureAndLostACKRunRecovery(t *testing.T) {
	s, b, now := riftFixture(t)
	activateTestRift(t, s, b, *now)
	s.stepEndgameRift()
	x := s.rift
	x.data.Run.Phase = domain.RiftCompleted
	x.data.Run.Members[100].Eligible = true
	x.data.GrantsReady = true
	x.data.Grants[100] = endgameRiftGrant{Items: []domain.Stack{{Item: 1900028202, Count: 10, Bound: true}}}
	x.savedRevision = x.data.Run.Revision
	b.runs[x.data.Run.RunID] = x.data.record(x.revision)
	p := s.players[1]
	req := domain.EndgameRequest{Op: "rift.claim", RequestID: "claim-storage-001", ExpectedRevision: "0"}
	s.claimEndgameRift(p, req, x.data, x.revision, true)
	b.finish(false)
	s.finishEndgame(false)
	if !x.economicPending {
		t.Fatal("unlocked before run reconcile")
	}
	s.pollEndgameRiftReload(*now)
	if x.economicPending || p.Player.Bag.CountOf(1900028202) != 0 || x.data.Run.Members[100].Claimed {
		t.Fatal("failed claim mutated")
	}
	s.claimEndgameRift(p, req, x.data, x.revision, true)
	plan := b.plan
	b.runs[plan.Run.RunID] = *plan.Run
	authoritative := plan.Snapshot.Clone()
	b.commit <- domain.EndgameCommitResult{State: plan.State, Receipt: plan.Receipt, Replayed: true, Authoritative: &authoritative}
	s.finishEndgame(false)
	s.pollEndgameRiftReload(*now)
	if x.economicPending || !x.data.Run.Members[100].Claimed || p.Player.Bag.CountOf(1900028202) != 10 || x.revision != plan.Run.Revision {
		t.Fatal("committed replay lost run state")
	}
}
func TestRiftLeaveDurableAndNativeExitGuard(t *testing.T) {
	s, b, now := riftFixture(t)
	activateTestRift(t, s, b, *now)
	s.stepEndgameRift()
	p := s.players[1]
	if s.onTeleport(Teleport{ID: p.ID, To: domain.SceneID{MapID: 8}, At: domain.Pos{MapID: 8}}) {
		t.Fatal("native portal bypassed voluntary-leave receipt")
	}
	req := domain.EndgameRequest{Op: "rift.leave", RequestID: "leave-durable-001", ExpectedRevision: "0"}
	s.leaveEndgameRift(p, req)
	if s.rift.data.Run.Members[100].Departed {
		t.Fatal("left before commit")
	}
	b.finish(true)
	s.finishEndgame(false)
	if !s.rift.data.Run.Members[100].Departed || s.rift.data.Run.Phase != domain.RiftAborted {
		t.Fatal("leave not committed")
	}
}
func TestRiftTrialClaimFrozenKeysAtomic(t *testing.T) {
	s, b, _ := riftFixture(t)
	p := s.players[1]
	s.trial.Entrances = []domain.TrialEntrance{{ClaimMap: 7, ClaimNPC: "试炼小仙"}}
	s.entities[77] = &entity.Entity{ID: 77, Kind: domain.KindNPC, Name: "试炼小仙"}
	p.Player.QuestNPC = "试炼小仙"
	p.Player.Char.Quests[99] = domain.QuestEntry{State: domain.QuestDeliver}
	p.Player.Char.Trial = domain.TrialProgress{MonsterLevel: 60, KeyReward: 14, CompletionID: "frozen-opening", Cycle: 5}
	s.completeTrial(p)
	if b.commits != 1 || p.Player.Bag.CountOf(domain.RiftKeyItemID) != 0 {
		t.Fatal("key published before receipt")
	}
	if b.plan.Snapshot.Bag.CountOf(domain.RiftKeyItemID) != 14 || b.plan.Snapshot.Char.Trial.Cycle != 0 {
		t.Fatal("round or frozen count changed")
	}
	b.finish(false)
	s.finishEndgame(false)
	if p.Player.Char.Quests[99].State != domain.QuestDeliver {
		t.Fatal("failure consumed completion")
	}
	s.completeTrial(p)
	b.finish(true)
	s.finishEndgame(false)
	if p.Player.Bag.CountOf(domain.RiftKeyItemID) != 14 || p.Player.Char.Quests[99].State != domain.QuestFinished {
		t.Fatal("missing atomic keys")
	}
	p.Player.Bag.Each(func(_ int, st domain.Stack) {
		if st.Item == domain.RiftKeyItemID && !st.Bound {
			t.Fatal("key not bound")
		}
	})
	s.completeTrial(p)
	if b.commits != 2 {
		t.Fatal("double native claim")
	}
}
func TestRiftStartOneKeyAndDefiniteFailureReservation(t *testing.T) {
	for _, success := range []bool{true, false} {
		t.Run(map[bool]string{true: "paid", false: "rollback"}[success], func(t *testing.T) {
			s, b, _ := riftFixture(t)
			frames := make(chan time.Time)
			router := NewRouter(func(id domain.SceneID) (*Scene, error) {
				return New(Config{ID: id, Frames: frames, Trial: s.trial, Dungeons: s.dungeons}), nil
			}, nil)
			router.Start(context.Background())
			defer router.Drain(time.Second)
			s.router = router
			var anchors []domain.SpawnPoint
			for i := 0; i < 40; i++ {
				anchors = append(anchors, domain.SpawnPoint{ID: int32(i + 1), Monster: 1, Pos: domain.Pos{MapID: 7, X: float64(100 + i*100), Y: 100}})
			}
			s.riftLayouts = []domain.RiftFloorTemplate{{MapID: 7, MapName: "测试洞天", Anchors: anchors, Monsters: []domain.MonsterID{1}, Guardians: []domain.MonsterID{2}}}
			s.trial.Entrances = []domain.TrialEntrance{{EntryMap: 7, EntryNPC: "妖魔结界守卫"}}
			s.entities[77] = &entity.Entity{ID: 77, Kind: domain.KindNPC, Name: "妖魔结界守卫"}
			p := s.players[1]
			p.Player.QuestNPC = "妖魔结界守卫"
			p.Player.Bag.AddStack(s.items[domain.RiftKeyItemID], domain.Stack{Item: domain.RiftKeyItemID, Count: 3, Bound: true})
			req := domain.EndgameRequest{Op: "rift.start", RequestID: "start-one-key-0001", ExpectedRevision: "0", Tier: 1}
			s.startEndgameRift(p, req)
			if b.commits != 1 || p.Player.Bag.CountOf(domain.RiftKeyItemID) != 3 || b.plan.Snapshot.Bag.CountOf(domain.RiftKeyItemID) != 2 {
				t.Fatal("key changed outside atomic candidate")
			}
			b.finish(success)
			s.finishEndgame(false)
			if success {
				if p.Player.Bag.CountOf(domain.RiftKeyItemID) != 2 {
					t.Fatal("wrong sponsor debit")
				}
				if _, ok := router.riftScene(b.plan.Run.RunID); !ok {
					t.Fatal("paid admission missing")
				}
			} else {
				if p.Player.Bag.CountOf(domain.RiftKeyItemID) != 3 {
					t.Fatal("rollback lost key")
				}
				router.mu.RLock()
				n := len(router.riftAdmissions)
				router.mu.RUnlock()
				if n != 0 {
					t.Fatal("unpaid reservation leaked")
				}
			}
		})
	}
}

func TestRiftFrozenReconnectACLAndEmptyGrace(t *testing.T) {
	s, b, now := riftFixture(t)
	activateTestRift(t, s, b, *now)
	s.stepEndgameRift()
	s.id = domain.SceneID{MapID: 7, Instance: 4}
	s.router.scenes[s.id] = s
	s.router.riftAdmissions = map[domain.SceneID]*endgameRiftAdmission{s.id: {Data: s.rift.data.clone(), Paid: true, Allowed: map[domain.CharID]bool{100: true}}}
	p := s.players[1]
	p.Player.Party = 777
	if !s.canStayInDungeon(p) || !s.router.ActiveRiftForCharacter(s.id, 100) || s.router.ActiveRiftForCharacter(s.id, 200) {
		t.Fatal("frozen ACL changed with party")
	}
	count := len(s.monsters)
	s.disconnectEndgameRift(p)
	delete(s.players, 1)
	*now += 60000
	s.stepEndgameRift()
	if s.rift.data.Run.Terminal() {
		t.Fatal("disconnected run lost before grace")
	}
	s.players[1] = p
	s.syncEndgameRift(p)
	s.stepEndgameRift()
	if len(s.monsters) != count {
		t.Fatal("reconnect rerolled population")
	}
	s.disconnectEndgameRift(p)
	delete(s.players, 1)
	*now += domain.RiftReconnectMS + 1
	s.stepEndgameRift()
	if s.rift.data.Run.Phase != domain.RiftAborted || s.rift.data.Run.Reason != "abandoned" {
		t.Fatal("empty grace did not expire")
	}
}
func TestRiftTrialRewardRetainsNativeLevelGrowth(t *testing.T) {
	s, b, _ := riftFixture(t)
	s.levels = domain.NewLevelTable(map[int32]int64{12: 100, 13: 100, 14: 100, 15: 100, 50: 100})
	s.trial.FirstPct = 100
	s.trial.Entrances = []domain.TrialEntrance{{ClaimMap: 7, ClaimNPC: "试炼小仙"}}
	s.entities[77] = &entity.Entity{ID: 77, Kind: domain.KindNPC, Name: "试炼小仙"}
	p := s.players[1]
	p.Player.QuestNPC = "试炼小仙"
	p.Player.Char.Level = 12
	p.Player.Char.Trial = domain.TrialProgress{MonsterLevel: 12, KeyReward: 1, CompletionID: "growth-receipt"}
	p.Player.Char.Quests[99] = domain.QuestEntry{State: domain.QuestDeliver}
	s.completeTrial(p)
	b.finish(true)
	s.finishEndgame(false)
	if p.Player.Char.Level != 15 || p.Player.Char.FreePoints != 3*domain.FreePointsPerLevel {
		t.Fatal("trial level growth lost", p.Player.Char.Level, p.Player.Char.FreePoints)
	}
}
func TestRiftStatusRefreshPreservesEntityScaling(t *testing.T) {
	s, b, now := riftFixture(t)
	def := s.trial.Monsters[60][1]
	def.Stats.MinAtk = 10
	def.Stats.MaxAtk = 20
	s.trial.Monsters[60][1] = def
	activateTestRift(t, s, b, *now)
	found := false
	for id, m := range s.monsters {
		if s.rift.data.Run.Monsters[id].Class != 2 {
			continue
		}
		found = true
		before := m.Stats
		m.Status = domain.NewStatusSet()
		s.refreshEntityStats(m)
		if m.Stats != before || m.Stats.MinAtk != 14 || m.Stats.MaxHP != 800 {
			t.Fatal("native status refresh erased elite scaling", before, m.Stats)
		}
		if base, ok := s.defs.Def(m.Monster.TypeID); !ok || base.Level != 60 {
			t.Fatal("missing trial AI definition")
		}
		break
	}
	if !found {
		t.Fatal("no yellow elite")
	}
	if s.trial.Monsters[60][1].HP != 100 || s.trial.Monsters[60][1].Stats.MinAtk != 10 {
		t.Fatal("shared template mutated")
	}
}

func TestRiftTrialReentryWaitsForOldEntitlement(t *testing.T) {
	s, b, _ := riftFixture(t)
	p := s.players[1]
	oldNonce := "old-offline-completion"
	p.Player.Char.Trial.CompletionID = oldNonce
	pending := make(chan domain.EndgameRunLoadResult, 1)
	b.runLoad = pending
	s.recoverEndgameTrial(p)
	def := domain.DungeonDef{Solo: true, Enter: domain.Pos{MapID: 7, X: 100, Y: 100}}
	if _, err := s.openPlayerDungeon(p, def); err == nil {
		t.Fatal("opened while old recovery pending")
	}
	if p.Player.Char.Trial.CompletionID != oldNonce || s.trialRun.Nonce != "" || len(s.router.riftAdmissions) != 0 {
		t.Fatal("old nonce replaced before recovery")
	}
	completion := endgameTrialCompletion{Version: 1, Kind: "trial", Nonce: oldNonce, MonsterLevel: 60, Keys: map[domain.CharID]int32{100: 14}}
	payload, _ := json.Marshal(completion)
	record := domain.EndgameRunRecord{RunID: "trial-" + oldNonce, Revision: 1, State: "trial.completed", Payload: string(payload)}
	pending <- domain.EndgameRunLoadResult{Run: &record}
	s.stepEndgameTrial()
	if p.Player.Char.Trial.KeyReward != 14 || p.Player.Char.Quests[99].State != domain.QuestDeliver {
		t.Fatal("old completion not restored")
	}
	if _, err := s.openPlayerDungeon(p, def); err == nil {
		t.Fatal("opened over recovered reward")
	}
	if p.Player.Char.Trial.CompletionID != oldNonce {
		t.Fatal("recovered nonce changed")
	}
}
