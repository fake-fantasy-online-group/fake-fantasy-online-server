package scene

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"strconv"
	"time"
)

func (s *Scene) completeEndgameTrial(p *entity.Entity) {
	if !s.trialClaimNPC(p) {
		s.trialNotice(p, "请回四村试炼小仙处领取奖励。")
		return
	}
	ch := p.Player.Char
	entry := ch.Quests[s.trial.Quest]
	if (entry.State != domain.QuestActive && entry.State != domain.QuestDeliver) ||
		(ch.Trial.CompletionID == "" && (entry.State != domain.QuestDeliver || ch.Trial.MonsterLevel <= 0)) {
		s.trialNotice(p, "尚未完成试炼，请消灭副本内全部怪物。")
		return
	}
	state := s.trialState()
	if _, pending := state.Claims[p.ID]; pending {
		return
	}
	if ch.Trial.CompletionID == "" {
		ch.Trial.CompletionID = riftNonce()
		p.Player.MarkDirty()
	}
	// Keep the native click across asynchronous archive and completion reads.
	// The intent belongs only to this completion, never to a future trial.
	state.Claims[p.ID] = ch.Trial.CompletionID
	s.resumeEndgameTrialClaim(p.ID)
}

func (s *Scene) cancelEndgameTrialClaim(p *entity.Entity, message string) {
	if s.trialRun != nil {
		if _, pending := s.trialRun.Claims[p.ID]; pending {
			delete(s.trialRun.Claims, p.ID)
			s.trialNotice(p, message)
		}
	}
}

func (s *Scene) resumeEndgameTrialClaim(id domain.EntityID) {
	state := s.trialState()
	nonce, pending := state.Claims[id]
	p := s.players[id]
	if !pending {
		return
	}
	if p == nil || p.Player == nil || !p.Alive() || s.trial == nil || !s.trialClaimNPC(p) ||
		s.mapLoads[id] != nil || p.Player.Char.Trial.CompletionID != nonce || !p.Player.Char.Quests.Active(s.trial.Quest) {
		delete(state.Claims, id)
		return
	}
	x := s.endgamePlayer(p)
	if endgameEconomicBusy(p) || x.commit != nil {
		s.cancelEndgameTrialClaim(p, "其他物品事务尚未结束，请稍后重试领取。")
		return
	}
	if x.load != nil || x.receipt != nil {
		return
	}
	if !x.loaded {
		s.onEndgame(EndgameRequest{ID: id, Request: domain.EndgameRequest{Op: "snapshot"}})
		return
	}
	completion, frozen := state.Completions[nonce]
	if !frozen && state.RecoveryChecked[id] != nonce {
		s.recoverEndgameTrial(p)
		return
	}
	if frozen {
		if state.Nonce == nonce && !state.Saved {
			return
		}
		s.assignEndgameTrial(p, completion)
	}
	// Consume before committing. Repeated packets cannot queue a second payout;
	// storage failures retain the entitlement but require a fresh explicit click.
	delete(state.Claims, id)
	s.settleEndgameTrial(p)
}

func (s *Scene) settleEndgameTrial(p *entity.Entity) {
	ch := p.Player.Char
	r := s.trial
	if ch.Quests[r.Quest].State != domain.QuestDeliver || ch.Trial.MonsterLevel <= 0 {
		s.trialNotice(p, "尚未完成试炼，请消灭副本内全部怪物。")
		return
	}
	x := s.endgamePlayer(p)
	nonce := ch.Trial.CompletionID
	req := domain.EndgameRequest{Op: "trial.claim", RequestID: "trial-claim-" + nonce, ExpectedRevision: strconv.FormatInt(x.state.Revision, 10)}
	day := domain.DayOf(s.now().In(time.FixedZone("Asia/Shanghai", 8*3600)))
	progress := ch.Trial
	if progress.Day != day {
		progress.Day = day
		progress.DailyCompleted = 0
	}
	reward := s.frozenTrialReward(nonce, ch.Level, ch.Trial.MonsterLevel, progress, day)
	state := s.trialState()
	completion, frozen := state.Completions[nonce]
	var grant endgameTrialGrant
	if frozen && completion.Version >= 2 {
		var ok bool
		grant, ok = completion.Grants[domain.CharID(ch.ID)]
		if !ok || grant.Claimed {
			s.trialNotice(p, "没有可领取的完成奖励。")
			return
		}
		reward, progress = grant.Reward, grant.Progress
	}
	candidate := s.snapshotOf(p).Clone()
	bag, _, ok := s.planQuestItems(candidate.Bag, nil, nil, reward.Items)
	if !ok {
		s.trialNotice(p, "请腾出奖励所需的背包空间。")
		return
	}
	candidate.Bag = bag
	keys := progress.KeyReward
	if frozen {
		keys = completion.Keys[domain.CharID(ch.ID)]
	}
	if keys < 1 {
		keys = 1
	}
	if keys > 14 {
		keys = 14
	}
	def, exists := s.itemDef(domain.RiftKeyItemID)
	if !exists || candidate.Bag.AddStack(def, domain.Stack{Item: domain.RiftKeyItemID, Count: keys, Bound: true}) != 0 {
		s.trialNotice(p, "请腾出洞天符钥所需的背包空间。")
		return
	}
	progress.CompletedOnce = true
	progress.DailyCompleted++
	progress.Cycle = (progress.Cycle + 1) % int32(len(r.Multipliers))
	progress.MonsterLevel = 0
	progress.KeyReward = 0
	progress.CompletionID = ""
	candidate.Char.Trial = progress
	candidate.Char.Quests.Finish(r.Quest)
	gain := candidate.Char.AddExp(s.levels, reward.Exp, domain.LevelCap)
	if gain.Reason != "" {
		s.trialNotice(p, "经验数据异常，奖励已保留。")
		return
	}
	candidate.Char.ApplyLevelUps(gain.FromLevel, gain.ToLevel)
	var record *domain.EndgameRunRecord
	var revision int64
	if frozen && completion.Version >= 2 {
		if err := s.settleEndgamePetGrant(candidate.Char, &grant.Pet); err != nil {
			s.trialNotice(p, "宠物经验数据异常，奖励已保留。")
			return
		}
		// Copy the map before changing it; a rejected transaction retains the entitlement.
		body, _ := json.Marshal(completion)
		var detached endgameTrialCompletion
		_ = json.Unmarshal(body, &detached)
		completion = detached
		grant.Claimed = true
		completion.Grants[domain.CharID(ch.ID)] = grant
		revision = completion.StorageRevision
		completion.StorageRevision = revision + 1
		body, _ = json.Marshal(completion)
		rc := domain.EndgameRunRecord{RunID: "trial-" + nonce, Revision: revision + 1, State: "trial.completed", Payload: string(body)}
		record = &rc
	}
	committed := s.endgameCommit(p, req, candidate, domain.EndgameResponse{Kind: req.Op, RequestID: req.RequestID, OK: true, Message: fmt.Sprintf("试炼完成，获得%d枚洞天符钥。", keys)}, record, revision, func() {
		if frozen {
			state.Completions[nonce] = completion
		}
		s.publishEndgamePetGrant(p, grant.Pet)
		s.publishEndgameRewardExperience(p, gain.FromLevel, gain.AppliedExp)
		s.pushQuestLog(p)
		s.pushQuestOffers(p, p.Player.QuestNPC)
		s.trialNotice(p, fmt.Sprintf("试炼完成，获得%d经验、%d枚洞天符钥。", gain.AppliedExp, keys))
	})
	if committed {
		s.endgamePlayer(p).onFailure = func() { delete(state.Completions, nonce); delete(state.RecoveryChecked, p.ID) }
	}
}

func (s *Scene) frozenTrialReward(nonce string, level, monsterLevel int32, progress domain.TrialProgress, day int32) domain.QuestReward {
	r := s.trial
	reward := domain.QuestReward{Exp: r.Experience(level, monsterLevel, progress, day, s.levels)}
	if progress.DailyCompleted == int32(len(r.Multipliers))-1 {
		reward.Items = []domain.RewardItem{{Item: r.Pill, Qty: 1}}
		hash := sha256.Sum256([]byte(nonce))
		if binary.BigEndian.Uint32(hash[:4])%100 < uint32(r.RewardChance) {
			reward.Items = append(reward.Items, domain.RewardItem{Item: r.Fragment, Qty: r.RewardMin + int32(binary.BigEndian.Uint32(hash[4:8])%uint32(r.RewardMax-r.RewardMin+1))})
		}
	}
	return reward
}
