package scene

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"sort"
)

func (s *Scene) onEndgameRift(p *entity.Entity, req domain.EndgameRequest) {
	if s.endgameBackend == nil || s.riftRules.Active() == nil {
		s.endgameError(p, req, "unavailable", "洞天暂不可用")
		return
	}
	if !p.Player.EndgameRiftPages && req.Op == "rift.state" {
		if _, _, _, _, waiting := s.router.PagedRiftReconnect(domain.CharID(p.Player.Char.ID)); waiting {
			s.endgameError(p, req, "update_required", "请更新客户端后自动重返洞天；原入场资格与进度保留。")
			return
		}
	}
	if s.rift != nil {
		if !s.requireRiftPages(p, req, s.rift.data.Run.Rules) {
			return
		}
		if req.RunID != "" && req.RunID != s.rift.data.Run.RunID {
			s.endgameError(p, req, "busy", "请先离开当前洞天")
			return
		}
		switch req.Op {
		case "rift.advance":
			s.advanceEndgameRift(p, req)
		case "rift.state":
			s.replyLiveRift(p, req)
		case "rift.leave":
			s.leaveEndgameRift(p, req)
		case "rift.claim":
			s.claimEndgameRift(p, req, s.rift.data, s.rift.revision, true)
		case "rift.start":
			s.replyLiveRift(p, req)
		}
		return
	}
	if req.Op == "rift.advance" {
		s.endgameError(p, req, "not_inside", "当前不在洞天内")
		return
	}
	profile := riftProfile(s.endgamePlayer(p).state.Payload)
	if (req.Op == "rift.start") && profile.RunID != "" && profile.RunID != req.RunID {
		s.loadEndgameRift(p, req, profile.RunID, true)
		return
	}
	id := req.RunID
	if id == "" {
		id = profile.RunID
	}
	if req.Op == "rift.start" && req.RunID == "" {
		s.startEndgameRift(p, req)
		return
	}
	if id == "" {
		s.endgameReply(p, domain.EndgameResponse{Kind: req.Op, RequestID: req.RequestID, OK: true, Rift: &domain.EndgameRiftView{Phase: "idle", MaxTier: s.riftRules.Active().MaxTier, UnlockedTier: profile.UnlockedTier, Name: s.riftRules.Active().Name, KeyCount: p.Player.Bag.CountOf(domain.RiftKeyItemID), KeyItemID: int32(domain.RiftKeyItemID)}})
		return
	}
	s.loadEndgameRift(p, req, id, false)
}
func (s *Scene) loadEndgameRift(p *entity.Entity, req domain.EndgameRequest, id string, prior bool) {
	if s.riftRequests == nil {
		s.riftRequests = map[domain.EntityID]*endgameRiftRequestState{}
	}
	if s.riftRequests[p.ID] != nil {
		return
	}
	s.riftRequests[p.ID] = &endgameRiftRequestState{req: req, load: s.endgameBackend.LoadRun(id), prior: prior}
}
func (s *Scene) stepEndgameRiftRequests() {
	for id, x := range s.riftRequests {
		select {
		case result := <-x.load:
			delete(s.riftRequests, id)
			p := s.players[id]
			if p == nil {
				continue
			}
			if result.Err != nil {
				s.endgameError(p, x.req, "storage", "读取洞天失败，请重试")
				continue
			}
			d, ok := decodeEndgameRift(result.Run, s.riftRules)
			if !ok {
				if result.Run != nil {
					s.endgameError(p, x.req, "configuration", "洞天存档或规则版本无法读取")
					continue
				}
				if x.prior {
					s.continueEndgameRiftStart(p, x.req)
				} else {
					s.endgameError(p, x.req, "not_found", "洞天已不存在")
				}
				continue
			}
			char := domain.CharID(p.Player.Char.ID)
			m := d.Run.Members[char]
			if m == nil {
				s.endgameError(p, x.req, "not_owner", "不在本次洞天名册")
				continue
			}
			if !s.requireRiftPages(p, x.req, d.Run.Rules) {
				continue
			}
			if validEndgameRiftQuestRecord(result.Run, d) {
				s.recordEndgameRiftQuestCompletion(p, d)
			}
			_, live := s.router.riftScene(d.Run.RunID)
			if x.prior {
				owed := (m.Eligible && !m.Claimed && (d.Run.Phase == domain.RiftCompleted || d.Run.Phase == domain.RiftClosed)) || riftNeedsCompensation(d.Run, live, char)
				if owed {
					s.endgameError(p, x.req, "pending_reward", "请先领取上次洞天奖励或中断补偿")
					s.endgameReply(p, domain.EndgameResponse{Kind: "rift.state", OK: true, Rift: s.riftView(p, d, true, live)})
					continue
				}
				if live && !d.Run.Terminal() {
					s.endgameError(p, x.req, "active_run", "上次洞天仍在进行")
					continue
				}
				if x.req.RunID == "" {
					// A restored/legacy completion may already be claimed while its
					// profile still has the old progression rule. Admit from the
					// durable entitlement and materialize it with the new run only.
					profile := riftProfileWithCompletion(s.endgamePlayer(p).state.Payload, d.Run, char)
					s.startEndgameRiftWithProfile(p, x.req, profile)
				} else {
					s.continueEndgameRiftStart(p, x.req)
				}
				continue
			}
			switch x.req.Op {
			case "rift.state":
				s.endgameReply(p, domain.EndgameResponse{Kind: x.req.Op, RequestID: x.req.RequestID, OK: true, Rift: s.riftView(p, d, true, live)})
			case "rift.start":
				s.joinEndgameRift(p, x.req, d)
			case "rift.claim":
				if live {
					s.endgameError(p, x.req, "reenter", "洞天尚未收束，可重返拾取；收束后可补领。")
					continue
				}
				s.claimEndgameRift(p, x.req, d, result.Run.Revision, false)
			case "rift.leave":
				s.endgameError(p, x.req, "not_inside", "当前不在洞天内")
			}
		default:
		}
	}
}
func (s *Scene) continueEndgameRiftStart(p *entity.Entity, req domain.EndgameRequest) {
	if req.RunID == "" {
		s.startEndgameRift(p, req)
	} else {
		s.loadEndgameRift(p, req, req.RunID, false)
	}
}
func (s *Scene) startEndgameRift(p *entity.Entity, req domain.EndgameRequest) {
	s.startEndgameRiftWithProfile(p, req, riftProfile(s.endgamePlayer(p).state.Payload))
}

func (s *Scene) startEndgameRiftWithProfile(p *entity.Entity, req domain.EndgameRequest, profile endgameRiftProfile) {
	rules := s.riftRules.Active()
	if rules == nil {
		s.endgameError(p, req, "configuration", "洞天规则尚未载入")
		return
	}
	if !s.requireRiftPages(p, req, rules) {
		return
	}
	if s.router == nil || s.trial == nil || !p.Alive() || !s.canUseEndgameRiftEntrance(p, req.NPCEntityID) {
		s.endgameError(p, req, "entrance", "请与洞天入口使者交谈后开启洞天")
		return
	}
	if req.Tier < 1 || req.Tier > profile.UnlockedTier || req.Tier > rules.MaxTier {
		s.endgameError(p, req, "tier", "尚未解锁此层")
		return
	}
	if p.Player.Party != 0 && (s.dungeonParty == nil || s.dungeonParty(domain.CharID(p.Player.Char.ID)).Leader != domain.CharID(p.Player.Char.ID)) {
		s.endgameError(p, req, "leader", "请由队长开启洞天")
		return
	}
	var roster []domain.TrialKeyMember
	names := map[domain.CharID]string{}
	profiles := map[domain.CharID]endgameRiftIdentity{}
	var peers []domain.EntityID
	for _, peer := range s.players {
		if peer.ID != p.ID && (p.Player.Party == 0 || peer.Player.Party != p.Player.Party) {
			continue
		}
		if !peer.Alive() || riftDistance2(peer.Pos, p.Pos) > 500*500 {
			continue
		}
		ch := peer.Player.Char
		if ch.Level < s.trial.MinLevel || ch.Level > s.trial.MaxLevel {
			continue
		}
		if riftNeedsPages(rules) && !peer.Player.EndgameRiftPages {
			s.endgameError(p, req, "update_required", "队伍中有成员需要更新客户端，未扣除洞天符钥。")
			return
		}
		char := domain.CharID(ch.ID)
		roster = append(roster, domain.TrialKeyMember{CharID: char, Level: ch.Level, Cycle: ch.Trial.Cycle})
		names[char] = ch.Name
		profiles[char] = endgameRiftIdentity{Race: ch.Race, Gender: ch.Appear.Gender, Employed: ch.HasProfession(), MagicFind: s.magicFindV5(peer), RewardLevel: ch.Level}
		peers = append(peers, peer.ID)
	}
	sort.Slice(roster, func(i, j int) bool { return roster[i].CharID < roster[j].CharID })
	run, err := domain.NewRiftRun(riftNonce(), req.Tier, domain.CharID(p.Player.Char.ID), roster, s.riftMS(), rules)
	if err != nil {
		s.endgameError(p, req, "roster", "洞天限1至5名合适等级队员")
		return
	}
	if rules.CumulativeLegendaryPerTier > 0 {
		poolLevel := int32(0)
		for _, member := range roster {
			poolLevel = max(poolLevel, member.Level)
		}
		// Check every actual possible encounter Boss baseline. Its level may
		// differ from the frozen roster's lookup key and the named guardian's
		// original world level; neither is a substitute for the spawned level.
		bases := s.riftFloorBaseDefinitionsForRun(run, true)
		if len(bases) == 0 {
			s.endgameError(p, req, "reward_pool", "洞天守关怪物奖励来源暂不可用，未扣除洞天符钥。")
			return
		}
		for _, member := range roster {
			for _, base := range bases {
				if len(s.riftCumulativeLegendaryPool(rules, poolLevel, member.Level, base.Level, profiles[member.CharID])) == 0 {
					s.endgameError(p, req, "reward_pool", "队伍中有成员暂无合法暗金奖励底材，未扣除洞天符钥。")
					return
				}
			}
		}
	}
	plan, layoutErr := s.planEndgameRiftFloor(run.RunID, 1, 0)
	if layoutErr != nil {
		s.endgameError(p, req, "layout", "洞天地形暂不可用，请稍后重试")
		return
	}
	var guardianID domain.MonsterID
	if rules.GuardianRecentWindow == 3 {
		guardianID = selectRiftGuardian(rules, plan.Seed, profile.RecentGuardians)
		plan.Guardian.Point.Monster = guardianID
		// This candidate profile is committed atomically with the key and run.
		profile.RecentGuardians = appendRiftGuardianHistory(profile.RecentGuardians, guardianID)
	}
	mapID := plan.MapID
	floor := &plan
	d := endgameRiftPersisted{GuardianID: guardianID, Version: 2, Floor: floor, Run: run, MapID: mapID, Return: p.Pos, Names: names, Profiles: profiles, Grants: map[domain.CharID]endgameRiftGrant{}}
	candidate := s.snapshotOf(p).Clone()
	if !candidate.Bag.Remove(domain.RiftKeyItemID, 1) {
		s.endgameError(p, req, "key", "需要一枚洞天符钥")
		return
	}
	inst, err := s.router.reserveEndgameRift(mapID, d)
	if err != nil {
		s.endgameError(p, req, "instance", err.Error())
		return
	}
	record := d.record(1)
	profile.RunID = run.RunID
	view := s.riftView(p, d, true, true)
	view.UnlockedTier = profile.UnlockedTier
	ok := s.endgameCommitPayload(p, req, candidate, domain.EndgameResponse{Kind: req.Op, RequestID: req.RequestID, OK: true, Rift: view}, &record, 0, func() {
		s.router.payEndgameRift(inst)
		for _, id := range peers {
			if peer := s.players[id]; peer != nil {
				s.endgameReply(peer, domain.EndgameResponse{Kind: "rift.state", OK: true, Rift: s.riftView(peer, d, true, true)})
			}
		}
		id := p.ID
		s.timer.after(s.tick, 1, func() {
			if s.players[id] != nil {
				s.onTeleport(Teleport{RiftRunID: run.RunID, ID: id, To: inst, At: s.router.riftEntry(inst, s.dungeons[mapID].Enter)})
			}
		})
	}, withRiftProfile(s.endgamePlayer(p).state.Payload, profile))
	if ok {
		s.endgamePlayer(p).onFailure = func() { s.router.discardEndgameRift(inst) }
	} else {
		s.router.discardEndgameRift(inst)
	}
}
func (s *Scene) joinEndgameRift(p *entity.Entity, req domain.EndgameRequest, d endgameRiftPersisted) {
	if !s.requireRiftPages(p, req, d.Run.Rules) {
		return
	}
	if !p.Alive() {
		s.endgameError(p, req, "dead", "请复活后进入洞天")
		return
	}
	id, ok := s.router.riftScene(d.Run.RunID)
	char := domain.CharID(p.Player.Char.ID)
	if !ok || !s.router.ActiveRiftForCharacter(id, char) {
		s.endgameError(p, req, "not_owner", "洞天已结束或没有本次入场资格")
		return
	}
	profile := riftProfile(s.endgamePlayer(p).state.Payload)
	profile.RunID = d.Run.RunID
	s.endgameCommitPayload(p, req, s.snapshotOf(p).Clone(), domain.EndgameResponse{Kind: req.Op, RequestID: req.RequestID, OK: true, Rift: s.riftView(p, d, true, true)}, nil, 0, func() {
		entityID := p.ID
		s.timer.after(s.tick, 1, func() {
			if s.players[entityID] != nil {
				s.onTeleport(Teleport{RiftRunID: d.Run.RunID, ID: entityID, To: id, At: s.router.riftEntry(id, s.dungeons[id.MapID].Enter)})
			}
		})
	}, withRiftProfile(s.endgamePlayer(p).state.Payload, profile))
}
func (s *Scene) replyLiveRift(p *entity.Entity, req domain.EndgameRequest) {
	s.endgameReply(p, domain.EndgameResponse{Kind: req.Op, RequestID: req.RequestID, OK: true, Rift: s.riftView(p, s.rift.data, s.rift.savedRevision >= s.rift.data.Run.Revision, true)})
	s.armRiftHazards(p, s.riftMS())
}
func (s *Scene) claimEndgameRift(p *entity.Entity, req domain.EndgameRequest, d endgameRiftPersisted, revision int64, live bool) {
	if live && (d.Run.Phase == domain.RiftCompleted || d.Run.Phase == domain.RiftClosed) {
		s.endgameError(p, req, "ground_rewards", "请拾取地面上的洞天所得；洞天收束后可补领未拾之物。")
		return
	}
	if live && (s.rift.saving != nil || s.rift.economicPending || s.rift.savedRevision < d.Run.Revision) {
		s.endgameError(p, req, "pending", "结算正在保存，请稍候")
		return
	}
	next := d.clone()
	char := domain.CharID(p.Player.Char.ID)
	candidate := s.snapshotOf(p).Clone()
	profile := riftProfile(s.endgamePlayer(p).state.Payload)
	previousLevel := candidate.Char.Level
	var gainedExp int64
	var petGrant endgamePetGrant
	if riftNeedsCompensation(next.Run, live, char) {
		def, ok := s.itemDef(domain.RiftKeyItemID)
		if !ok || candidate.Bag.AddStack(def, domain.Stack{Item: def.ID, Count: 1, Bound: true}) != 0 {
			s.endgameError(p, req, "bag_full", "请腾出背包空间")
			return
		}
		next.Run.Phase = domain.RiftFailed
		next.Run.Reason = "compensated"
		next.Run.Revision++
	} else {
		if !next.GrantsReady || !next.Run.MarkClaimed(char) {
			s.endgameError(p, req, "not_claimable", "没有可领取的奖励")
			return
		}
		grant := next.Grants[char]
		for index, stack := range grant.Items {
			if grant.Collected[index] {
				continue
			}
			def, ok := s.itemDef(stack.Item)
			if ok && def.HardDungeonOnly {
				s.endgameError(p, req, "invalid_reward_source", "奖励来源无效，请联系管理员")
				return
			}
			if !ok || candidate.Bag.AddStack(def, stack) != 0 {
				s.endgameError(p, req, "bag_full", "背包空间不足，奖励将保留")
				return
			}
		}
		if !grant.ExperienceClaimed {
			gain := candidate.Char.AddExp(s.levels, grant.Exp, domain.LevelCap)
			if gain.Reason != "" {
				s.endgameError(p, req, "progression", gain.Reason)
				return
			}
			candidate.Char.ApplyLevelUps(gain.FromLevel, gain.ToLevel)
			gainedExp = gain.AppliedExp
			if err := s.settleEndgamePetGrant(candidate.Char, &grant.Pet); err != nil {
				s.endgameError(p, req, "progression", err.Error())
				return
			}
			petGrant = grant.Pet
			grant.ExperienceClaimed = true
		}
		next.Grants[char] = grant
		profile = riftProfileWithCompletion(s.endgamePlayer(p).state.Payload, next.Run, char)
	}
	profile.RunID = next.Run.RunID
	all := true
	for _, m := range next.Run.Members {
		if m.Eligible && !m.Claimed {
			all = false
		}
	}
	if all && (next.Run.Phase == domain.RiftCompleted) {
		next.Run.Phase = domain.RiftClosed
	}
	record := next.record(revision + 1)
	if live {
		s.rift.economicPending = true
	}
	ok := s.endgameCommitPayload(p, req, candidate, domain.EndgameResponse{Kind: req.Op, RequestID: req.RequestID, OK: true, Rift: s.riftView(p, next, true, live)}, &record, revision, func() {
		s.publishEndgameRewardExperience(p, previousLevel, gainedExp)
		s.publishEndgamePetGrant(p, petGrant)
		if live {
			s.rift.data = next
			s.rift.revision = record.Revision
			s.rift.savedRevision = next.Run.Revision
			s.rift.economicPending = false
			s.syncEndgameRiftCompletion()
		}
	}, withRiftProfile(s.endgamePlayer(p).state.Payload, profile))
	if live {
		if ok {
			s.endgamePlayer(p).onFailure = func() { s.reloadEndgameRift() }
		} else {
			s.rift.economicPending = false
		}
	}
}
func (s *Scene) leaveEndgameRift(p *entity.Entity, req domain.EndgameRequest) {
	x := s.rift
	if x.saving != nil || x.economicPending || x.transition != nil || x.retired {
		s.endgameError(p, req, "pending", "存档中，请稍候")
		return
	}
	next := x.data.clone()
	char := domain.CharID(p.Player.Char.ID)
	next.Run.Leave(char)
	profile := riftProfileWithCompletion(s.endgamePlayer(p).state.Payload, next.Run, char)
	profile.RunID = next.Run.RunID
	record := next.record(x.revision + 1)
	x.economicPending = true
	ok := s.endgameCommitPayload(p, req, s.snapshotOf(p).Clone(), domain.EndgameResponse{Kind: req.Op, RequestID: req.RequestID, OK: true}, &record, x.revision, func() {
		x.data = next
		x.revision = record.Revision
		x.savedRevision = next.Run.Revision
		x.economicPending = false
		if !next.Run.Admitted(char) {
			s.router.withdrawRift(s.id, char)
		}
		id := p.ID
		s.timer.after(s.tick, 1, func() {
			if s.players[id] != nil {
				s.onTeleport(Teleport{ID: id, To: domain.SceneID{MapID: next.Return.MapID}, At: next.Return})
			}
		})
	}, withRiftProfile(s.endgamePlayer(p).state.Payload, profile))
	if ok {
		s.endgamePlayer(p).onFailure = func() { s.reloadEndgameRift() }
	} else {
		x.economicPending = false
	}
}
func riftDistance2(a, b domain.Pos) float64 { x, y := a.X-b.X, a.Y-b.Y; return x*x + y*y }
