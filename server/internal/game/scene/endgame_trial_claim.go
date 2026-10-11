package scene

import (
	"crypto/sha256"
	"encoding/binary"
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
	r := s.trial
	if ch.Quests[r.Quest].State != domain.QuestDeliver || ch.Trial.MonsterLevel <= 0 {
		s.recoverEndgameTrial(p)
		s.trialNotice(p, "尚未完成试炼，已检查离线完成记录，请稍后重试。")
		return
	}
	x := s.endgamePlayer(p)
	if !x.loaded {
		s.onEndgame(EndgameRequest{ID: p.ID, Request: domain.EndgameRequest{Op: "snapshot"}})
		s.trialNotice(p, "奖励档案读取中，请稍后重试。")
		return
	}
	nonce := ch.Trial.CompletionID
	if nonce == "" {
		nonce = riftNonce()
		ch.Trial.CompletionID = nonce
		p.Player.MarkDirty()
	}
	req := domain.EndgameRequest{Op: "trial.claim", RequestID: "trial-claim-" + nonce, ExpectedRevision: strconv.FormatInt(x.state.Revision, 10)}
	day := domain.DayOf(s.now().In(time.FixedZone("Asia/Shanghai", 8*3600)))
	progress := ch.Trial
	if progress.Day != day {
		progress.Day = day
		progress.DailyCompleted = 0
	}
	special := progress.DailyCompleted == int32(len(r.Multipliers))-1
	reward := domain.QuestReward{Exp: r.Experience(ch.Level, ch.Trial.MonsterLevel, progress, day, s.levels)}
	if special {
		reward.Items = []domain.RewardItem{{Item: r.Pill, Qty: 1}}
		hash := sha256.Sum256([]byte(nonce))
		if binary.BigEndian.Uint32(hash[:4])%100 < uint32(r.RewardChance) {
			reward.Items = append(reward.Items, domain.RewardItem{Item: r.Fragment, Qty: r.RewardMin + int32(binary.BigEndian.Uint32(hash[4:8])%uint32(r.RewardMax-r.RewardMin+1))})
		}
	}
	candidate := s.snapshotOf(p).Clone()
	bag, _, ok := s.planQuestItems(candidate.Bag, nil, nil, reward.Items)
	if !ok {
		s.trialNotice(p, "请腾出奖励所需的背包空间。")
		return
	}
	candidate.Bag = bag
	keys := progress.KeyReward
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
	progress.DailyCompleted++
	progress.Cycle = (progress.Cycle + 1) % int32(len(r.Multipliers))
	progress.MonsterLevel = 0
	progress.KeyReward = 0
	progress.CompletionID = ""
	candidate.Char.Trial = progress
	candidate.Char.Quests.Finish(r.Quest)
	gain := candidate.Char.AddExp(s.levels, reward.Exp, domain.LevelCap)
	candidate.Char.ApplyLevelUps(gain.FromLevel, gain.ToLevel)
	s.endgameCommit(p, req, candidate, domain.EndgameResponse{Kind: req.Op, RequestID: req.RequestID, OK: true, Message: fmt.Sprintf("试炼完成，获得%d枚洞天符钥。", keys)}, nil, 0, func() {
		s.publishEndgameRewardExperience(p, gain.FromLevel, reward.Exp)
		s.pushQuestLog(p)
		s.pushQuestOffers(p, p.Player.QuestNPC)
		s.trialNotice(p, fmt.Sprintf("试炼完成，获得%d经验、%d枚洞天符钥。", reward.Exp, keys))
	})
}
