package scene

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strconv"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

const (
	// Verified native source: init-0.sql, qw0007 row 83 (龙城 八仙使者),
	// sprite 630 / Nbaxian, script alias Nbaxian_ffo, portrait N630.
	endgameRiftImmortalModel int32 = 630
	endgameRiftExitName            = "归元道使"
	endgameRiftExitScript          = "endgame_rift_exit"
)

type endgameRiftGroundReward struct {
	Char  domain.CharID
	Index int
}

// Runtime-only cache: the reward roster and map mask are frozen for a completed
// run. Collected flags, ownership refreshes and run revisions do not move slots.
type endgameRiftRewardLayout struct {
	NPC       domain.Pos
	NPCReady  bool
	Slots     int
	Positions map[endgameRiftGroundReward]domain.Pos
}

func (s *Scene) cachedEndgameRiftCompletionLayout() *endgameRiftRewardLayout {
	x := s.rift
	slots := 0
	for _, grant := range x.data.Grants {
		slots += len(grant.Items)
	}
	if x.rewardLayout == nil || x.rewardLayout.Slots != slots {
		npc, ready, positions := s.endgameRiftCompletionLayout()
		x.rewardLayout = &endgameRiftRewardLayout{NPC: npc, NPCReady: ready, Slots: slots, Positions: positions}
	}
	return x.rewardLayout
}

func (s *Scene) isEndgameRiftExitNPC(e *entity.Entity) bool {
	return s.rift != nil && e != nil && e.ID == s.rift.exitNPC && e.NPC != nil && e.NPC.Script == endgameRiftExitScript
}

func (s *Scene) canUseEndgameRiftExit(p *entity.Entity) bool {
	x := s.rift
	if x == nil || p == nil || p.Player == nil || !p.Alive() || p.Player.QuestNPC != x.data.Run.Rules.ExitName {
		return false
	}
	if x.data.Run.Phase != domain.RiftCompleted && x.data.Run.Phase != domain.RiftClosed {
		return false
	}
	npc := s.entities[x.exitNPC]
	return x.data.Run.Admitted(domain.CharID(p.Player.Char.ID)) && s.isEndgameRiftExitNPC(npc) && npc.Pos.MapID == p.Pos.MapID && riftDistance2(p.Pos, npc.Pos) <= 200*200
}

func (s *Scene) endgameRiftRewardPosition() domain.Pos {
	d := s.rift.data
	if d.RewardPos.MapID == s.id.MapID {
		return d.RewardPos
	}
	if d.Floor != nil {
		return d.Floor.Exit
	}
	return s.dungeons[s.id.MapID].Enter
}

// Allocate one stable reachable slot per frozen grant across the entire roster.
// Reserve space around the exit so its collider cannot cover an item. If an
// unusually narrow map has too few safe points, keep the unplaced entitlement
// for offline recovery rather than put it inside a wall or on another player's drop.
func (s *Scene) endgameRiftCompletionLayout() (domain.Pos, bool, map[endgameRiftGroundReward]domain.Pos) {
	center := s.endgameRiftRewardPosition()
	walkable := func(pos domain.Pos) bool {
		return (s.walkable == nil || s.walkable(pos)) && s.straightPathWalkable(center, pos)
	}
	npc := center
	npc.X += 120
	npcOK := walkable(npc)
	if !npcOK {
		npc = center
		npcOK = walkable(npc)
	}
	positions := map[endgameRiftGroundReward]domain.Pos{}
	chars := make([]domain.CharID, 0, len(s.rift.data.Grants))
	for char := range s.rift.data.Grants {
		chars = append(chars, char)
	}
	sort.Slice(chars, func(i, j int) bool { return chars[i] < chars[j] })
	var refs []endgameRiftGroundReward
	for _, char := range chars {
		for index := range s.rift.data.Grants[char].Items {
			refs = append(refs, endgameRiftGroundReward{Char: char, Index: index})
		}
	}
	used := []domain.Pos{}
	next := 0
	for _, spacing := range []float64{monsterLootGridSpacing, 20} {
		maxRing := int(320 / spacing)
		for ring := 0; ring <= maxRing && next < len(refs); ring++ {
			for y := -ring; y <= ring && next < len(refs); y++ {
				for x := -ring; x <= ring && next < len(refs); x++ {
					if ring > 0 && x != -ring && x != ring && y != -ring && y != ring {
						continue
					}
					pos := center
					pos.X += float64(x) * spacing
					pos.Y += float64(y) * spacing
					if !walkable(pos) {
						continue
					}
					if !npcOK {
						npc = pos
						npcOK = true
						continue
					}
					if riftDistance2(pos, npc) < 64*64 {
						continue
					}
					occupied := false
					for _, prior := range used {
						if riftDistance2(pos, prior) < spacing*spacing {
							occupied = true
							break
						}
					}
					if occupied {
						continue
					}
					positions[refs[next]] = pos
					used = append(used, pos)
					next++
				}
			}
		}
		if next == len(refs) {
			break
		}
	}
	return npc, npcOK, positions
}

// Ground entities are a projection of the existing durable entitlement, never a
// second reward roll. Personal ownership is keyed by character, not a transient
// login entity ID, and persists for the whole instance rather than going public.
func (s *Scene) syncEndgameRiftCompletion() {
	x := s.rift
	if x == nil || x.retired || x.transition != nil || !x.data.GrantsReady || x.savedRevision < x.data.Run.Revision {
		return
	}
	if x.data.Run.Phase != domain.RiftCompleted && x.data.Run.Phase != domain.RiftClosed {
		return
	}
	layout := s.cachedEndgameRiftCompletionLayout()
	npcPos, npcOK, positions := layout.NPC, layout.NPCReady, layout.Positions
	if x.exitNPC == 0 && npcOK {
		pos := npcPos
		npc := &entity.Entity{ID: s.alloc.NPC(), Kind: domain.KindNPC, Name: x.data.Run.Rules.ExitName, Pos: pos, Dir: 3, Look: domain.Look{ModelID: x.data.Run.Rules.NPCModel}, NPC: &entity.NPCInfo{Sprite: x.data.Run.Rules.ExitSprite, Portrait: x.data.Run.Rules.ExitPortrait, Script: endgameRiftExitScript, Role: domain.NPCRoleGuide, Greeting: x.data.Run.Rules.ExitGreeting}}
		x.exitNPC = npc.ID
		s.entities[npc.ID] = npc
		s.aoi.Enter(npc)
		s.emit(s.spawnEvent(npc))
		for _, p := range s.players {
			s.pushNpcChatter(p.ID)
		}
	}
	if x.rewardDrops == nil {
		x.rewardDrops = map[domain.EntityID]endgameRiftGroundReward{}
	}
	active := map[endgameRiftGroundReward]bool{}
	for id, ref := range x.rewardDrops {
		grant, member := x.data.Grants[ref.Char], x.data.Run.Members[ref.Char]
		if member == nil || member.Claimed || !member.Eligible || ref.Index >= len(grant.Items) || grant.Collected[ref.Index] {
			s.removeLoot(id, event.DespawnPickedUp)
			delete(x.rewardDrops, id)
			continue
		}
		if e := s.ground[id]; e != nil {
			active[ref] = true
			if owner := s.endgameRewardPlayer(ref.Char); owner != nil && e.Drop.Owner != owner.ID {
				e.Drop.Owner = owner.ID
				s.emit(s.spawnEvent(e)) // ownership refresh, never another landing sound
			}
		} else {
			delete(x.rewardDrops, id)
		}
	}
	chars := make([]domain.CharID, 0, len(x.data.Grants))
	for char := range x.data.Grants {
		chars = append(chars, char)
	}
	sort.Slice(chars, func(i, j int) bool { return chars[i] < chars[j] })
	for _, char := range chars {
		member, owner := x.data.Run.Members[char], s.endgameRewardPlayer(char)
		if member == nil || !member.Eligible || member.Claimed || owner == nil || !x.ready[char] {
			continue
		}
		for index, stack := range x.data.Grants[char].Items {
			ref := endgameRiftGroundReward{Char: char, Index: index}
			if active[ref] || x.data.Grants[char].Collected[index] {
				continue
			}
			def, ok := s.itemDef(stack.Item)
			if !ok {
				continue
			} // preserve durable recovery if configuration is unavailable
			pos, placed := positions[ref]
			if !placed {
				continue
			}
			id := s.alloc.Drop()
			e := &entity.Entity{ID: id, Kind: domain.KindDrop, Name: itemDisplayName(def, stack), Pos: pos, Drop: &entity.Drop{Item: stack.Item, Count: stack.Count, Stack: stack, Owner: owner.ID, OwnerUntil: s.tick + 864000}}
			x.rewardDrops[id] = ref
			s.entities[id], s.ground[id] = e, e
			s.aoi.Enter(e)
			if x.rewardFresh {
				s.emit(s.freshGroundSpawnEvent(e))
			} else {
				s.emit(s.spawnEvent(e))
			}
		}
	}
	x.rewardFresh = false
	// Serialize per-member experience through the same run revision and inventory
	// lease used by pickup/claim/leave. Bag capacity cannot block boss experience.
	if x.economicPending || x.saving != nil {
		return
	}
	for _, char := range chars {
		if p := s.endgameRewardPlayer(char); p != nil && x.ready[char] && s.settleEndgameRiftExperience(p) {
			break
		}
	}
}

func markEndgameRiftGrantClaimed(d *endgameRiftPersisted, char domain.CharID) {
	grant := d.Grants[char]
	if !grant.ExperienceClaimed {
		return
	}
	for i := range grant.Items {
		if !grant.Collected[i] {
			return
		}
	}
	d.Run.MarkClaimed(char)
	all := true
	for _, m := range d.Run.Members {
		if m.Eligible && !m.Claimed {
			all = false
		}
	}
	if all && d.Run.Phase == domain.RiftCompleted {
		d.Run.Phase = domain.RiftClosed
	}
}

func (s *Scene) endgameRiftRewardRequest(p *entity.Entity, suffix string) domain.EndgameRequest {
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s/%d/%s", s.rift.data.Run.RunID, p.Player.Char.ID, suffix)))
	return domain.EndgameRequest{Op: "rift.reward", RequestID: fmt.Sprintf("rift-reward-%x", hash[:16]), ExpectedRevision: strconv.FormatInt(s.endgamePlayer(p).state.Revision, 10), RunID: s.rift.data.Run.RunID, TargetUID: suffix}
}

func (s *Scene) endgameRiftRewardReady(p *entity.Entity) bool {
	state := s.endgamePlayer(p)
	return state.loaded && state.load == nil && state.receipt == nil && state.commit == nil && !endgameEconomicBusy(p)
}

func (s *Scene) commitEndgameRiftReward(p *entity.Entity, next endgameRiftPersisted, candidate domain.Snapshot, suffix, message string, after func()) bool {
	x := s.rift
	req := s.endgameRiftRewardRequest(p, suffix)
	record := next.record(x.revision + 1)
	profile := riftProfileWithCompletion(s.endgamePlayer(p).state.Payload, next.Run, domain.CharID(p.Player.Char.ID))
	profile.RunID = next.Run.RunID
	x.economicPending = true
	ok := s.endgameCommitPayload(p, req, candidate, domain.EndgameResponse{Kind: "rift.reward", OK: true, Message: message}, &record, x.revision, func() {
		x.data, x.revision, x.savedRevision, x.economicPending = next, record.Revision, next.Run.Revision, false
		if after != nil {
			after()
		}
		s.syncEndgameRiftCompletion()
	}, withRiftProfile(s.endgamePlayer(p).state.Payload, profile))
	if ok {
		s.endgamePlayer(p).onFailure = func() { s.reloadEndgameRift() }
	} else {
		x.economicPending = false
	}
	return ok
}

// The native pickup packet stays unchanged. Both manual and pet paths consume
// exactly one frozen grant stack transactionally; no partial candidate is applied
// when the complete stack does not fit.
func (s *Scene) pickUpEndgameRiftReward(p, drop *entity.Entity, pickupRange int32) bool {
	x := s.rift
	if x == nil || drop == nil {
		return false
	}
	ref, found := x.rewardDrops[drop.ID]
	if !found {
		return false
	}
	if p == nil || p.Player == nil || !p.Alive() {
		return true
	}
	char := domain.CharID(p.Player.Char.ID)
	member := x.data.Run.Members[char]
	if ref.Char != char || member == nil || !member.Eligible || member.Claimed || !x.ready[char] || p.Pos.MapID != drop.Pos.MapID || riftDistance2(p.Pos, drop.Pos) > float64(pickupRange)*float64(pickupRange) {
		return true
	}
	if x.economicPending || x.saving != nil || x.savedRevision < x.data.Run.Revision || !s.endgameRiftRewardReady(p) {
		return true
	}
	grant := x.data.Grants[char]
	if ref.Index < 0 || ref.Index >= len(grant.Items) || grant.Collected[ref.Index] {
		s.syncEndgameRiftCompletion()
		return true
	}
	stack := grant.Items[ref.Index]
	def, ok := s.itemDef(stack.Item)
	if !ok {
		return true
	}
	candidate := s.snapshotOf(p).Clone()
	if candidate.Bag.AddStack(def, stack) != 0 {
		s.trialNotice(p, "背包空间不足，洞天所得仍留在原处。")
		return true
	}
	next := x.data.clone()
	grant = next.Grants[char]
	if grant.Collected == nil {
		grant.Collected = map[int]bool{}
	}
	grant.Collected[ref.Index] = true
	next.Grants[char] = grant
	next.Run.Revision++
	markEndgameRiftGrantClaimed(&next, char)
	message := fmt.Sprintf("获得了 %s ×%d", itemDisplayName(def, stack), stack.Count)
	s.commitEndgameRiftReward(p, next, candidate, fmt.Sprintf("item/%d", ref.Index), message, func() { s.trialNotice(p, message) })
	return true
}

func (s *Scene) settleEndgameRiftExperience(p *entity.Entity) bool {
	x := s.rift
	char := domain.CharID(p.Player.Char.ID)
	member := x.data.Run.Members[char]
	grant := x.data.Grants[char]
	if member == nil || !member.Eligible || member.Claimed || grant.ExperienceClaimed || !s.endgameRiftRewardReady(p) {
		return false
	}
	candidate := s.snapshotOf(p).Clone()
	previousLevel := candidate.Char.Level
	gain := candidate.Char.AddExp(s.levels, grant.Exp, domain.LevelCap)
	if gain.Reason != "" {
		return false
	}
	candidate.Char.ApplyLevelUps(gain.FromLevel, gain.ToLevel)
	if err := s.settleEndgamePetGrant(candidate.Char, &grant.Pet); err != nil {
		return false
	}
	next := x.data.clone()
	grant.ExperienceClaimed = true
	next.Grants[char] = grant
	next.Run.Revision++
	markEndgameRiftGrantClaimed(&next, char)
	return s.commitEndgameRiftReward(p, next, candidate, "experience", "洞天首领修为已结算", func() {
		s.publishEndgameRewardExperience(p, previousLevel, gain.AppliedExp)
		s.publishEndgamePetGrant(p, grant.Pet)
	})
}

func (s *Scene) currentRiftRules() *domain.RiftRules {
	if s.rift != nil && s.rift.data.Run != nil {
		return s.rift.data.Run.Rules
	}
	return s.riftRules.Active()
}
