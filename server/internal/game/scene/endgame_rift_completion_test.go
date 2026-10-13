package scene

import (
	"strconv"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func completedRiftRewards(t *testing.T) (*Scene, *riftTestBackend, *int64) {
	t.Helper()
	s, b, now := riftFixture(t)
	curve := map[int32]int64{}
	for level := int32(1); level <= 100; level++ {
		curve[level] = int64(level) * int64(level) * int64(level) * 1000000
	}
	s.levels = domain.NewLevelTable(curve)
	activateTestRift(t, s, b, *now)
	r := s.rift.data.Run
	r.Phase = domain.RiftBoss
	id := s.alloc.Monster()
	r.RegisterMonster(id, 3)
	*now += 1000
	s.onEndgameRiftMonsterDeath(&entity.Entity{ID: id, Pos: s.players[1].Pos}, s.players[1])
	if r.Phase != domain.RiftCompleted {
		t.Fatal("boss did not complete")
	}
	if len(s.ground) != 0 || s.rift.exitNPC != 0 {
		t.Fatal("published before durability")
	}
	s.stepEndgameRift()
	s.stepEndgameRift()
	return s, b, now
}
func finishRiftRewardCommit(t *testing.T, s *Scene, b *riftTestBackend) {
	t.Helper()
	if b.commit == nil {
		t.Fatal("no commit")
	}
	b.finish(true)
	s.finishEndgame(false)
}
func ownedRiftDrop(s *Scene, char domain.CharID) *entity.Entity {
	for id, ref := range s.rift.rewardDrops {
		if ref.Char == char {
			return s.ground[id]
		}
	}
	return nil
}
func TestRiftCompletionGroundNPCAndExperienceExactlyOnce(t *testing.T) {
	s, b, now := completedRiftRewards(t)
	p := s.players[1]
	want := domain.RiftBossExperience(s.trial.BaseExperience(60, s.levels), 1, riftRulesFixture())
	if want <= 0 || p.Player.Char.Exp != 0 || b.commits != 1 {
		t.Fatal("XP published early", want, b.commits)
	}
	if len(s.ground) != len(s.rift.data.Grants[100].Items) || !s.isEndgameRiftExitNPC(s.entities[s.rift.exitNPC]) {
		t.Fatal("missing completion actors")
	}
	finishRiftRewardCommit(t, s, b)
	if p.Player.Char.Exp != want || !s.rift.data.Grants[100].ExperienceClaimed {
		t.Fatal("boss XP not applied once", p.Player.Char.Exp, want)
	}
	s.flush()
	xpEvents := 0
	for _, ev := range p.Player.Sink.(*fakeSink).take() {
		if xp, ok := ev.(event.ExpGained); ok {
			xpEvents++
			if xp.Delta != want {
				t.Fatal(xp)
			}
		}
	}
	if xpEvents != 1 {
		t.Fatal("XP feedback missing or duplicated", xpEvents)
	}
	initial := len(s.ground)
	for i := 0; i < 3; i++ {
		s.syncEndgameRiftCompletion()
		s.syncEndgameRift(p)
	}
	if len(s.ground) != initial || b.commits != 1 {
		t.Fatal("completion replay")
	}
	for len(s.rift.rewardDrops) > 0 {
		drop := ownedRiftDrop(s, 100)
		p.Pos = drop.Pos
		before := p.Player.Bag.CountOf(drop.Drop.Item)
		s.onPickUp(PickUp{ID: p.ID, Target: drop.ID})
		pending := b.commits
		s.onPickUp(PickUp{ID: p.ID, Target: drop.ID})
		if b.commits != pending || p.Player.Bag.CountOf(drop.Drop.Item) != before {
			t.Fatal("pickup replay/premature application")
		}
		finishRiftRewardCommit(t, s, b)
	}
	if !s.rift.data.Run.Members[100].Claimed || s.rift.data.Run.Phase != domain.RiftClosed || p.Player.Char.Exp != want {
		t.Fatal("completion did not consume exactly once")
	}
	count := b.commits
	s.claimEndgameRift(p, domain.EndgameRequest{Op: "rift.claim", RequestID: "after-ground-claim", ExpectedRevision: strconv.FormatInt(s.endgamePlayer(p).state.Revision, 10)}, s.rift.data, s.rift.revision, true)
	if b.commits != count {
		t.Fatal("claimed ground rewards twice")
	}
	*now += 2000
	s.stepEndgameRift()
	select {
	case <-s.quit:
		t.Fatal("claimed run kicked players after one second")
	default:
	}
}
func TestRiftGroundOwnershipDistanceBagAndRecovery(t *testing.T) {
	s, b, _ := completedRiftRewards(t)
	finishRiftRewardCommit(t, s, b)
	p := s.players[1]
	drop := ownedRiftDrop(s, 100)
	join(t, s, 2, 200, "旁人", drop.Pos.X, drop.Pos.Y)
	other := s.players[2]
	other.Player.Bag = domain.NewBag(domain.DefaultBagSlots)
	s.tick += 1000000 // ordinary 30s ownership must never release this entitlement
	count := b.commits
	s.onPickUp(PickUp{ID: other.ID, Target: drop.ID})
	p.Pos.X = drop.Pos.X + 1000
	s.onPickUp(PickUp{ID: p.ID, Target: drop.ID})
	if b.commits != count {
		t.Fatal("foreign/distant pickup")
	}
	p.Pos = drop.Pos
	original := p.Player.Bag
	p.Player.Bag = domain.NewBag(1)
	p.Player.Bag.Add(s.items[1900028202], domain.MaxStack)
	s.onPickUp(PickUp{ID: p.ID, Target: drop.ID})
	if b.commits != count || s.ground[drop.ID] == nil {
		t.Fatal("full bag consumed reward")
	}
	p.Player.Bag = original
	// A committed-but-lost ACK reloads the authoritative aggregate and ledger;
	// neither candidate callbacks nor experience feedback may replay.
	s.onPickUp(PickUp{ID: p.ID, Target: drop.ID})
	b.runs[b.plan.Run.RunID] = *b.plan.Run
	snapshot := b.plan.Snapshot.Clone()
	b.commit <- domain.EndgameCommitResult{Authoritative: &snapshot, State: b.plan.State, Receipt: b.plan.Receipt, Replayed: true}
	s.finishEndgame(false)
	s.pollEndgameRiftReload(s.riftMS())
	s.syncEndgameRiftCompletion()
	if s.ground[drop.ID] != nil || s.rift.economicPending {
		t.Fatal("lost ACK did not reconcile ground")
	}
	before := b.commits
	s.onPickUp(PickUp{ID: p.ID, Target: drop.ID})
	if b.commits != before {
		t.Fatal("lost ACK allowed duplicate")
	}
}
func TestRiftExitDialogueAndNativeModelScope(t *testing.T) {
	s, b, _ := completedRiftRewards(t)
	finishRiftRewardCommit(t, s, b)
	p := s.players[1]
	npc := s.entities[s.rift.exitNPC]
	if npc.Look.ModelID != s.rift.data.Run.Rules.NPCModel || npc.NPC.Sprite != s.rift.data.Run.Rules.ExitSprite || npc.NPC.Portrait != s.rift.data.Run.Rules.ExitPortrait {
		t.Fatal("exit did not use frozen rule resources")
	}
	p.Player.QuestNPC = s.rift.data.Run.Rules.ExitName
	p.Pos = npc.Pos
	if !s.canUseEndgameRiftExit(p) {
		t.Fatal("valid exit rejected")
	}
	phase := s.rift.data.Run.Phase
	for _, blocked := range []domain.RiftPhase{domain.RiftReserved, domain.RiftActive, domain.RiftBoss, domain.RiftFailed, domain.RiftAborted} {
		s.rift.data.Run.Phase = blocked
		if s.canUseEndgameRiftExit(p) {
			t.Fatalf("unfinished run admitted NPC exit: %v", blocked)
		}
	}
	s.rift.data.Run.Phase = phase
	p.Pos.X += 201
	if s.canUseEndgameRiftExit(p) {
		t.Fatal("distant exit accepted")
	}
	p.Pos = npc.Pos
	p.Player.QuestNPC = "别的NPC"
	if s.canUseEndgameRiftExit(p) {
		t.Fatal("wrong NPC accepted")
	}
}

func TestRiftPendingPickupDisconnectAndOfflineRemainder(t *testing.T) {
	s, b, _ := completedRiftRewards(t)
	finishRiftRewardCommit(t, s, b)
	p := s.players[1]
	drop := ownedRiftDrop(s, 100)
	ref := s.rift.rewardDrops[drop.ID]
	p.Pos = drop.Pos
	xp := p.Player.Char.Exp
	s.onPickUp(PickUp{ID: p.ID, Target: drop.ID})
	s.exec(Leave{ID: p.ID, Reason: "disconnect during pickup"})
	if s.players[p.ID] == nil || s.ground[drop.ID] == nil {
		t.Fatal("disconnect escaped pending aggregate commit")
	}
	finishRiftRewardCommit(t, s, b)
	if s.players[p.ID] != nil || s.ground[drop.ID] != nil {
		t.Fatal("disconnect did not wait then finish")
	}
	saved := b.runs[s.rift.data.Run.RunID]
	durable, ok := decodeEndgameRift(&saved, riftRulesTableFixture())
	if !ok || !durable.Grants[100].Collected[ref.Index] || !durable.Grants[100].ExperienceClaimed {
		t.Fatal("pickup ledger not durable")
	}
	snapshot := b.plan.Snapshot.Clone()
	// Reconnect in the public return map after the empty instance has closed.
	s.rift = nil
	p = reconnectRiftPlayer(t, s, snapshot, 900000, 0, nil, &fakeSink{})
	s.endgamePlayer(p).loaded = true
	s.endgamePlayer(p).state = b.plan.State
	before := p.Player.Bag.CountOf(drop.Drop.Item)
	req := domain.EndgameRequest{Op: "rift.claim", RequestID: "offline-remaining-001", ExpectedRevision: strconv.FormatInt(s.endgamePlayer(p).state.Revision, 10)}
	s.claimEndgameRift(p, req, durable, saved.Revision, false)
	finishRiftRewardCommit(t, s, b)
	if p.Player.Char.Exp != xp {
		t.Fatal("offline claim duplicated XP")
	}
	after := p.Player.Bag.CountOf(drop.Drop.Item)
	want := before
	for i, st := range durable.Grants[100].Items {
		if !durable.Grants[100].Collected[i] && st.Item == drop.Drop.Item {
			want += st.Count
		}
	}
	if after != want {
		t.Fatal("offline claim duplicated ground item", before, after, want)
	}
	restored, ok := decodeEndgameRift(b.plan.Run, riftRulesTableFixture())
	if !ok || !restored.Run.Members[100].Claimed {
		t.Fatal("offline remainder not consumed")
	}
}

func TestRiftRewardRejoinPublishesSilentOwnerAndNPC(t *testing.T) {
	s, b, _ := completedRiftRewards(t)
	finishRiftRewardCommit(t, s, b)
	p := s.players[1]
	p.Player.QuestNPC = s.rift.data.Run.Rules.ExitName
	p.Pos = s.entities[s.rift.exitNPC].Pos
	snapshot := s.snapshotOf(p).Clone()
	priorCount := len(s.ground)
	s.flush()
	p.Player.Sink.(*fakeSink).take()
	s.exec(Leave{ID: p.ID, Reason: "temporary disconnect"})
	sink := &fakeSink{}
	p = reconnectRiftPlayer(t, s, snapshot, 900001, 0, nil, sink)
	if p.Player.QuestNPC != "" || s.canUseEndgameRiftExit(p) {
		t.Fatal("reconnect retained a previous session's NPC dialogue authority")
	}
	before := b.commits
	s.exec(TrialAction{ID: p.ID, Action: "trialleave"})
	s.finishEndgame(false)
	if b.commits != before {
		t.Fatal("reconnect command exited without reopening the NPC")
	}
	s.flush()
	seenNPC, seenDrops := false, 0
	for _, ev := range sink.take() {
		spawn, ok := ev.(event.EntitySpawned)
		if !ok {
			continue
		}
		if spawn.ID == s.rift.exitNPC {
			seenNPC = true
		}
		if spawn.Ground != nil {
			seenDrops++
			if spawn.Ground.FreshDrop || spawn.Ground.Owner != p.ID || spawn.Ground.ProtectionMS <= 0 {
				t.Fatal("rejoin replayed landing or lost ownership", spawn.Ground)
			}
		}
	}
	if !seenNPC || seenDrops == 0 || len(s.ground) != priorCount {
		t.Fatal("rejoin failed to project completion", seenNPC, seenDrops)
	}
}

func TestRiftFrozenPersonalExperienceDoesNotUseClaimLevel(t *testing.T) {
	s, b, _ := completedRiftRewards(t)
	// Completion already froze level 60 and tier 1; changing present character
	// level while the transaction is outstanding must not change its grant.
	frozen := s.rift.data.Grants[100].Exp
	if frozen != domain.RiftBossExperience(s.trial.BaseExperience(60, s.levels), 1, riftRulesFixture()) {
		t.Fatal("wrong frozen base")
	}
	if b.plan.Snapshot.Char.Exp != frozen {
		t.Fatal("candidate did not use frozen reward")
	}
	finishRiftRewardCommit(t, s, b)
	s.players[1].Player.Char.Level = 80
	s.syncEndgameRiftCompletion()
	if b.commits != 1 || s.rift.data.Grants[100].Exp != frozen {
		t.Fatal("claim-time reroll")
	}
}

func TestRiftNativeExitDialogueCommandFlow(t *testing.T) {
	s, b, _ := completedRiftRewards(t)
	finishRiftRewardCommit(t, s, b)
	s.quests = domain.QuestTable{}
	p := s.players[1]
	npc := s.entities[s.rift.exitNPC]
	before := b.commits
	// Knowing a command without opening the real NPC does not grant its service.
	s.exec(TrialAction{ID: p.ID, Action: "trialleave"})
	s.finishEndgame(false)
	if b.commits != before {
		t.Fatal("exit without dialogue")
	}
	s.exec(NpcTasks{ID: p.ID, NPC: s.rift.data.Run.Rules.ExitName})
	p.Pos = npc.Pos
	p.HP = 0
	s.exec(TrialAction{ID: p.ID, Action: "trialleave"})
	if b.commits != before {
		t.Fatal("dead exit")
	}
	p.HP = 100
	member := s.rift.data.Run.Members[100]
	delete(s.rift.data.Run.Members, 100)
	if s.canUseEndgameRiftExit(p) {
		t.Fatal("nonmember exit")
	}
	s.rift.data.Run.Members[100] = member
	delete(s.entities, npc.ID)
	if s.canUseEndgameRiftExit(p) {
		t.Fatal("missing NPC exit")
	}
	s.entities[npc.ID] = npc
	// Full inventory is irrelevant to leaving; unpicked entitlement stays durable.
	p.Player.Bag = domain.NewBag(1)
	p.Player.Bag.Add(s.items[1900028202], domain.MaxStack)
	s.exec(TrialAction{ID: p.ID, Action: "trialleave"})
	s.finishEndgame(false)
	if b.commits != before+1 || !s.rift.economicPending {
		t.Fatal("native exit did not use authoritative transaction")
	}
	s.exec(TrialAction{ID: p.ID, Action: "trialleave"})
	s.finishEndgame(false)
	if b.commits != before+1 {
		t.Fatal("repeated exit duplicated pending commit")
	}
	finishRiftRewardCommit(t, s, b)
	s.exec(TrialAction{ID: p.ID, Action: "trialleave"})
	s.finishEndgame(false)
	if b.commits != before+1 {
		t.Fatal("late repeated exit duplicated a completed commit")
	}
	durable, ok := decodeEndgameRift(b.plan.Run, riftRulesTableFixture())
	if !ok || durable.Run.Members[100].Claimed || !durable.Grants[100].ExperienceClaimed || len(durable.Grants[100].Items) == 0 {
		t.Fatal("exit lost unpicked entitlement")
	}
}

func TestRiftLiveOutsideProjectionRequiresReentry(t *testing.T) {
	s, b, _ := completedRiftRewards(t)
	finishRiftRewardCommit(t, s, b)
	d := s.rift.data.clone()
	p := s.players[1]
	s.rift = nil // querying from the return map while a teammate still owns the scene
	if v := s.riftView(p, d, true, true); v.Claimable || v.Phase != "completed" {
		t.Fatal("live external query advertised bulk claim", v)
	}
	if v := s.riftView(p, d, true, false); !v.Claimable || v.Phase != "closed" {
		t.Fatal("closed instance lost offline remainder", v)
	}
}

func TestRiftFiveMemberGroundLayoutAndExitDoNotOverlap(t *testing.T) {
	s, b, _ := completedRiftRewards(t)
	finishRiftRewardCommit(t, s, b)
	original := s.rift.data.Grants[100]
	s.walkable = func(pos domain.Pos) bool {
		return pos.MapID == s.id.MapID && pos.X >= 0 && pos.Y >= 0 && pos.X < 1000 && pos.Y < 1000
	}
	for _, char := range []domain.CharID{200, 300, 400, 500} {
		s.rift.data.Run.Members[char] = &domain.RiftMember{CharID: char, Level: 60, Eligible: true}
		s.rift.data.Grants[char] = original
		id := domain.EntityID(char)
		join(t, s, id, int64(char), "队员", 100, 100)
		s.rift.ready[char] = true
		s.endgamePlayer(s.players[id]).loaded = true
	}
	// A real mask boundary rejects negative coordinates rather than wrapping them.
	s.walkable = func(pos domain.Pos) bool {
		return pos.MapID == s.id.MapID && pos.X >= 0 && pos.Y >= 0 && pos.X < 1000 && pos.Y < 1000
	}
	npc, ok, positions := s.endgameRiftCompletionLayout()
	if !ok || len(positions) != 5*len(original.Items) {
		t.Fatal("missing safe roster slots", ok, len(positions))
	}
	for ref, pos := range positions {
		if !s.walkable(pos) || riftDistance2(pos, npc) < 64*64 {
			t.Fatal("unsafe item or NPC collider overlap", ref, pos, npc)
		}
		for other, otherPos := range positions {
			if ref != other && riftDistance2(pos, otherPos) < monsterLootGridSpacing*monsterLootGridSpacing {
				t.Fatal("team ground overlap", ref, other, pos, otherPos)
			}
		}
	}
	s.syncEndgameRiftCompletion()
	if len(s.rift.rewardDrops) != len(positions) {
		t.Fatal("not all team entitlements projected")
	}
	for id, ref := range s.rift.rewardDrops {
		drop := s.ground[id]
		if drop.Pos != positions[ref] || drop.Drop.Owner != s.endgameRewardPlayer(ref.Char).ID {
			t.Fatal("position or ownership mismatch", ref, drop)
		}
	}
	// Taking one item never compacts the remaining roster's positions.
	grant := s.rift.data.Grants[100]
	grant.Collected = map[int]bool{0: true}
	s.rift.data.Grants[100] = grant
	_, _, again := s.endgameRiftCompletionLayout()
	for ref, pos := range positions {
		if again[ref] != pos {
			t.Fatal("pickup reshuffled team slots")
		}
	}
}

func TestRiftGroundLayoutNarrowMapPreservesUnplacedEntitlements(t *testing.T) {
	s, b, _ := completedRiftRewards(t)
	finishRiftRewardCommit(t, s, b)
	center := s.rift.data.RewardPos
	// Degenerate mask: only the guardian's exact point is reachable. No item may
	// be placed into a wall or underneath the exit merely to force a projection.
	s.walkable = func(pos domain.Pos) bool { return pos == center }
	npc, ok, positions := s.endgameRiftCompletionLayout()
	if !ok || npc != center || len(positions) != 0 {
		t.Fatal("unsafe narrow-map fallback", npc, positions)
	}
	if len(s.rift.data.Grants[100].Items) == 0 || s.rift.data.Run.Members[100].Claimed {
		t.Fatal("unplaced rewards were consumed")
	}
}

func TestRiftGroundLayoutUsesSafeDensityInNarrowCorridor(t *testing.T) {
	s, b, _ := completedRiftRewards(t)
	finishRiftRewardCommit(t, s, b)
	grant := s.rift.data.Grants[100]
	for _, char := range []domain.CharID{200, 300, 400, 500} {
		s.rift.data.Grants[char] = grant
	}
	center := s.rift.data.RewardPos
	s.walkable = func(pos domain.Pos) bool {
		return pos.MapID == center.MapID && pos.Y == center.Y && pos.X >= center.X-320 && pos.X <= center.X+320
	}
	npc, ok, positions := s.endgameRiftCompletionLayout()
	if !ok || len(positions) != 5*len(grant.Items) {
		t.Fatal("safe dense slots not used", len(positions))
	}
	for ref, pos := range positions {
		if !s.walkable(pos) || riftDistance2(pos, npc) < 64*64 {
			t.Fatal("corridor wall/NPC overlap")
		}
		for other, otherPos := range positions {
			if ref != other && riftDistance2(pos, otherPos) < 20*20 {
				t.Fatal("dense drop overlap")
			}
		}
	}
}

func TestRiftCompletionLayoutDoesNotCrossSeparatingWall(t *testing.T) {
	s, b, _ := completedRiftRewards(t)
	finishRiftRewardCommit(t, s, b)
	center := s.rift.data.RewardPos
	// The preferred NPC point and many candidate endpoints lie in a walkable
	// room behind a solid wall. An endpoint-only check would accept them.
	s.walkable = func(pos domain.Pos) bool {
		return pos.MapID == center.MapID && (pos.X <= center.X+40 || pos.X >= center.X+100)
	}
	behind := center
	behind.X += 120
	if !s.walkable(behind) || s.straightPathWalkable(center, behind) {
		t.Fatal("invalid wall fixture")
	}
	npc, ok, positions := s.endgameRiftCompletionLayout()
	if !ok || npc.X > center.X+40 || !s.straightPathWalkable(center, npc) {
		t.Fatal("NPC placed beyond wall", npc)
	}
	if len(positions) != len(s.rift.data.Grants[100].Items) {
		t.Fatal("reachable local slots were missed")
	}
	for ref, pos := range positions {
		if pos.X > center.X+40 || !s.straightPathWalkable(center, pos) {
			t.Fatal("drop placed in unreachable room", ref, pos)
		}
	}
}

func TestRiftCompletionLayoutIsCachedAcrossSettlementTicks(t *testing.T) {
	s, b, _ := completedRiftRewards(t)
	finishRiftRewardCommit(t, s, b)
	calls := 0
	s.walkable = func(domain.Pos) bool { calls++; return true }
	s.rift.rewardLayout = nil
	s.syncEndgameRiftCompletion()
	sampled := calls
	if sampled == 0 {
		t.Fatal("layout not checked against map")
	}
	for i := 0; i < 20; i++ {
		s.syncEndgameRiftCompletion()
	}
	if calls != sampled {
		t.Fatal("completed map rescanned on every tick", sampled, calls)
	}
}

// These are the visually inspected cloud-native spirit resources. The renderer
// resolves npc362 to avatar/npc/npc36201.gsn; this model has no N362 portrait.
func TestRiftSpiritExitProjectionUsesNativeResources(t *testing.T) {
	s, b, _ := completedRiftRewards(t)
	finishRiftRewardCommit(t, s, b)
	old := s.entities[s.rift.exitNPC]
	s.aoi.Leave(old)
	delete(s.entities, old.ID)
	s.rift.exitNPC = 0
	rules := *s.rift.data.Run.Rules
	rules.NPCModel, rules.ExitSprite, rules.ExitPortrait = 362, "npc362", ""
	rules.ExitName = "洞天残魂"
	rules.ExitGreeting = "好久没见到别的人了……这里的风，总算静下来了。来吧，我这就送你回去。"
	s.rift.data.Run.Rules = &rules
	s.syncEndgameRiftCompletion()
	npc := s.entities[s.rift.exitNPC]
	if npc == nil || npc.Kind != domain.KindNPC || npc.Look.ModelID != 362 || npc.Name != rules.ExitName || npc.NPC.Sprite != "npc362" || npc.NPC.Portrait != "" || npc.NPC.Greeting != rules.ExitGreeting || npc.NPC.Script != endgameRiftExitScript {
		t.Fatal("spirit projection lost native model, dialogue or script")
	}
	first := npc.ID
	s.syncEndgameRiftCompletion()
	if s.rift.exitNPC != first {
		t.Fatal("repeat synchronization duplicated the exit NPC")
	}
	p := s.players[1]
	p.Pos, p.Player.QuestNPC = npc.Pos, npc.Name
	if !s.canUseEndgameRiftExit(p) {
		t.Fatal("spirit's original NPC interaction cannot exit")
	}
}

func TestRiftNativeExitStorageFailureCanReopen(t *testing.T) {
	s, b, now := completedRiftRewards(t)
	finishRiftRewardCommit(t, s, b)
	s.quests = domain.QuestTable{}
	p := s.players[1]
	npc := s.entities[s.rift.exitNPC]
	p.Pos = npc.Pos
	s.exec(NpcTasks{ID: p.ID, NPC: npc.Name})
	before, timers := b.commits, s.timer.pending()
	s.exec(TrialAction{ID: p.ID, Action: "trialleave"})
	s.finishEndgame(false)
	if b.commits != before+1 || p.Player.QuestNPC != "" {
		t.Fatal("native leave did not consume only its current dialogue intent")
	}
	b.finish(false)
	s.finishEndgame(false)
	s.pollEndgameRiftReload(*now)
	if s.rift.economicPending || s.players[p.ID] != p || s.timer.pending() != timers {
		t.Fatal("failed leave moved player or did not reconcile")
	}
	s.exec(TrialAction{ID: p.ID, Action: "trialleave"})
	s.finishEndgame(false)
	if b.commits != before+1 {
		t.Fatal("failed action replayed without reopening the NPC")
	}
	s.exec(NpcTasks{ID: p.ID, NPC: npc.Name})
	if !s.canUseEndgameRiftExit(p) {
		t.Fatal("failed exit could not regain authority from the real NPC")
	}
	s.exec(TrialAction{ID: p.ID, Action: "trialleave"})
	s.finishEndgame(false)
	if b.commits != before+2 {
		t.Fatal("reopened NPC did not retry failed save")
	}
	finishRiftRewardCommit(t, s, b)
	if s.timer.pending() != timers+1 {
		t.Fatal("successful retry did not schedule exactly one exit")
	}
}

func TestRiftNativeExitDisconnectDuringCommit(t *testing.T) {
	s, b, _ := completedRiftRewards(t)
	finishRiftRewardCommit(t, s, b)
	s.quests = domain.QuestTable{}
	p := s.players[1]
	npc := s.entities[s.rift.exitNPC]
	p.Pos = npc.Pos
	s.exec(NpcTasks{ID: p.ID, NPC: npc.Name})
	before := b.commits
	s.exec(TrialAction{ID: p.ID, Action: "trialleave"})
	s.finishEndgame(false)
	s.exec(Leave{ID: p.ID, Reason: "disconnect during native exit"})
	if s.players[p.ID] == nil {
		t.Fatal("disconnect escaped pending leave commit")
	}
	finishRiftRewardCommit(t, s, b)
	if s.players[p.ID] != nil {
		t.Fatal("completed leave did not release disconnected player")
	}
	s.timer.advance(s.tick + 1)
	s.exec(TrialAction{ID: p.ID, Action: "trialleave"})
	s.finishEndgame(false)
	if b.commits != before+1 {
		t.Fatal("disconnected completion duplicated exit")
	}
	durable, ok := decodeEndgameRift(b.plan.Run, riftRulesTableFixture())
	if !ok || len(durable.Grants[100].Items) == 0 || !durable.Grants[100].ExperienceClaimed {
		t.Fatal("disconnected completion lost frozen rewards")
	}
}
