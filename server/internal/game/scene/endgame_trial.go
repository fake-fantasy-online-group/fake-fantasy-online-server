package scene

import (
	"encoding/json"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"sort"
	"time"
)

type endgameTrialMember struct {
	Opening               domain.TrialKeyMember
	Progress              domain.TrialProgress
	PresentMS, LastSeenMS int64
}
type endgameTrialCompletion struct {
	Version         int
	Kind, Nonce     string
	MonsterLevel    int32
	Keys            map[domain.CharID]int32
	Grants          map[domain.CharID]endgameTrialGrant
	StorageRevision int64
}
type endgameTrialGrant struct {
	Reward             domain.QuestReward
	Progress           domain.TrialProgress
	ProgressionVersion string
	RewardVersion      string
	Pet                endgamePetGrant
	Claimed            bool
}
type endgameTrialRecovery struct {
	nonce string
	load  <-chan domain.EndgameRunLoadResult
}
type endgameTrialState struct {
	Nonce             string
	StartedMS, LastMS int64
	Members           map[domain.CharID]*endgameTrialMember
	Keys              map[domain.CharID]int32
	Completion        endgameTrialCompletion
	Completions       map[string]endgameTrialCompletion
	Completed, Saved  bool
	Saving            <-chan domain.EndgameRunSaveResult
	RetryAfter        int64
	Recoveries        map[domain.EntityID]endgameTrialRecovery
	RecoveryChecked   map[domain.EntityID]string
	Claims            map[domain.EntityID]string
}

func (s *Scene) trialState() *endgameTrialState {
	if s.trialRun == nil {
		s.trialRun = &endgameTrialState{Members: map[domain.CharID]*endgameTrialMember{}, Keys: map[domain.CharID]int32{}, Recoveries: map[domain.EntityID]endgameTrialRecovery{}}
	}
	if s.trialRun.Completions == nil {
		s.trialRun.Completions = map[string]endgameTrialCompletion{}
	}
	if s.trialRun.RecoveryChecked == nil {
		s.trialRun.RecoveryChecked = map[domain.EntityID]string{}
	}
	if s.trialRun.Claims == nil {
		s.trialRun.Claims = map[domain.EntityID]string{}
	}
	return s.trialRun
}
func (s *Scene) beginEndgameTrial() {
	if s.endgameBackend == nil || s.rift != nil || !s.trial.IsMap(s.id.MapID) || s.trialCleared {
		return
	}
	x := s.trialState()
	if x.Nonce != "" {
		return
	}
	x.Nonce = riftNonce()
	x.StartedMS = s.riftMS()
	x.LastMS = x.StartedMS
	var players []*entity.Entity
	for _, p := range s.players {
		if p.Player.Char.Quests[s.trial.Quest].State == domain.QuestActive {
			players = append(players, p)
		}
	}
	sort.Slice(players, func(i, j int) bool { return players[i].Player.Char.ID < players[j].Player.Char.ID })
	if len(players) > 5 {
		players = players[:5]
	}
	for _, p := range players {
		ch := p.Player.Char
		id := domain.CharID(ch.ID)
		x.Members[id] = &endgameTrialMember{Opening: domain.TrialKeyMember{CharID: id, Level: ch.Level, Cycle: ch.Trial.Cycle}, Progress: ch.Trial, LastSeenMS: x.StartedMS}
		s.recordEndgameTrialOpening(p, x.Nonce)
	}
}

// Membership is frozen immediately, including reserved players. Only the
// character mutation and full save wait for inventory ownership to be known.
func (s *Scene) recordEndgameTrialOpening(p *entity.Entity, nonce string) {
	if s.deferTradeMutation(p.ID, func() { s.recordEndgameTrialOpening(p, nonce) }) {
		return
	}
	p.Player.Char.Trial.CompletionID = nonce
	p.Player.Char.Trial.KeyReward = 0
	p.Player.MarkDirty()
	if s.saver != nil {
		s.saver.Save(s.snapshotOf(p).Clone())
	}
	// The trial may have completed while the mail/ trade outcome was unknown.
	// Preserve its entitlement before a deferred disconnect removes the player.
	if x := s.trialRun; x != nil {
		if completion, ok := x.Completions[nonce]; ok && (nonce != x.Nonce || x.Saved) {
			s.assignEndgameTrial(p, completion)
		}
	}
}
func (s *Scene) stepEndgameTrial() {
	if s.trialRun == nil {
		return
	}
	x := s.trialRun
	now := s.riftMS()
	if x.Nonce != "" && !x.Completed {
		delta := now - x.LastMS
		if delta < 0 {
			delta = 0
		}
		x.LastMS = now
		for _, p := range s.players {
			if m := x.Members[domain.CharID(p.Player.Char.ID)]; m != nil {
				m.PresentMS += delta
				m.LastSeenMS = now
			}
		}
	}
	if x.Completed && !x.Saved {
		if x.Saving != nil {
			select {
			case result := <-x.Saving:
				x.Saving = nil
				if result.Err == nil && result.Run != nil && result.Run.RunID == "trial-"+x.Nonce && result.Run.State == "trial.completed" {
					x.Saved = true
				} else {
					x.RetryAfter = now + 1000
				}
			default:
			}
		}
		if !x.Saved && x.Saving == nil && now >= x.RetryAfter {
			if backend, ok := s.endgameBackend.(domain.EndgameRunBackend); ok {
				completion := x.Completion
				b, _ := json.Marshal(completion)
				x.Saving = backend.SaveRun(domain.EndgameRunRecord{RunID: "trial-" + x.Nonce, Revision: 1, State: "trial.completed", Payload: string(b)}, 0)
			}
		}
	}
	for _, p := range s.players {
		nonce := p.Player.Char.Trial.CompletionID
		if c, ok := x.Completions[nonce]; ok && (nonce != x.Nonce || x.Saved) {
			s.assignEndgameTrial(p, c)
		}
	}
	for id, recovery := range x.Recoveries {
		select {
		case result := <-recovery.load:
			delete(x.Recoveries, id)
			p := s.players[id]
			if p == nil || p.Player.Char.Trial.CompletionID != recovery.nonce {
				continue
			}
			if result.Err != nil {
				s.cancelEndgameTrialClaim(p, "暂时无法读取试炼完成记录，请重试领取。")
				continue
			}
			if result.Run == nil {
				x.RecoveryChecked[id] = recovery.nonce
				continue
			}
			var c endgameTrialCompletion
			if result.Run.State == "trial.completed" && json.Unmarshal([]byte(result.Run.Payload), &c) == nil && (c.Version == 1 || c.Version == 2) && c.Kind == "trial" && c.Nonce == recovery.nonce {
				c.StorageRevision = result.Run.Revision
				x.Completions[c.Nonce] = c
				x.RecoveryChecked[id] = recovery.nonce
				s.assignEndgameTrial(p, c)
			} else {
				s.cancelEndgameTrialClaim(p, "试炼完成记录尚未就绪，请稍后重试领取。")
			}
		default:
		}
	}
}
func (s *Scene) finishEndgameTrialKeys() {
	s.beginEndgameTrial()
	x := s.trialRun
	if x == nil || x.Nonce == "" || x.Completed {
		return
	}
	s.stepEndgameTrial()
	now := s.riftMS()
	elapsed := now - x.StartedMS
	if elapsed < 1 {
		elapsed = 1
	}
	var credited []domain.TrialKeyMember
	for _, m := range x.Members {
		present := false
		for _, p := range s.players {
			if domain.CharID(p.Player.Char.ID) == m.Opening.CharID {
				present = true
				break
			}
		}
		if elapsed == 1 && present {
			m.PresentMS = 1
		}
		if m.PresentMS*2 >= elapsed && now-m.LastSeenMS <= domain.RiftReconnectMS {
			credited = append(credited, m.Opening)
		}
	}
	for _, m := range credited {
		n, err := domain.TrialKeyReward(m.CharID, credited)
		if err == nil {
			x.Keys[m.CharID] = n
		}
	}
	c := endgameTrialCompletion{Version: 2, Kind: "trial", Nonce: x.Nonce, MonsterLevel: s.trialLevel, Keys: x.Keys, StorageRevision: 1, Grants: map[domain.CharID]endgameTrialGrant{}}
	day := domain.DayOf(s.now().In(time.FixedZone("Asia/Shanghai", 8*3600)))
	for _, member := range credited {
		m := x.Members[member.CharID]
		owner := s.endgameRewardPlayer(member.CharID)
		progress, level := m.Progress, member.Level
		if owner != nil {
			progress = owner.Player.Char.Trial
			level = owner.Player.Char.Level
		}
		if progress.Day != day {
			progress.Day = day
			progress.DailyCompleted = 0
		}
		reward := s.frozenTrialReward(x.Nonce, level, s.trialLevel, progress, day)
		c.Grants[member.CharID] = endgameTrialGrant{Reward: reward, Progress: progress, ProgressionVersion: domain.ProgressionVersion, RewardVersion: domain.TrialRewardVersion, Pet: s.freezeEndgamePetGrant(owner, reward.Exp, x.Nonce, member.CharID)}
	}
	x.Completion = c
	x.Completions[c.Nonce] = c
	x.Completed = true
	s.stepEndgameTrial()
}
func (s *Scene) assignEndgameTrial(p *entity.Entity, c endgameTrialCompletion) {
	if s.trial == nil || endgameEconomicBusy(p) {
		return
	}
	ch := p.Player.Char
	q, accepted := ch.Quests[s.trial.Quest]
	n, ok := c.Keys[domain.CharID(ch.ID)]
	_, granted := c.Grants[domain.CharID(ch.ID)]
	if !ok || n <= 0 || (c.Version >= 2 && !granted) || ch.Trial.CompletionID != c.Nonce {
		return
	}
	changed := !ch.Trial.CompletedOnce
	ch.Trial.CompletedOnce = true
	if accepted && q.State == domain.QuestActive {
		q.State = domain.QuestDeliver
		ch.Quests[s.trial.Quest] = q
		ch.Trial.MonsterLevel = c.MonsterLevel
		ch.Trial.KeyReward = n
		changed = true
		s.pushQuestLog(p)
		s.trialNotice(p, "试炼完成奖励已恢复，请回试炼小仙领取。")
	}
	if changed {
		p.Player.MarkDirty()
		if s.saver != nil {
			s.saver.Save(s.snapshotOf(p).Clone())
		}
	}
}
func (s *Scene) recoverEndgameTrial(p *entity.Entity) {
	if s.endgameBackend == nil || s.trial == nil {
		return
	}
	ch := p.Player.Char
	if ch.Trial.CompletionID == "" || (ch.Quests[s.trial.Quest].State != domain.QuestActive && ch.Quests[s.trial.Quest].State != domain.QuestDeliver) {
		return
	}
	x := s.trialState()
	if c, ok := x.Completions[ch.Trial.CompletionID]; ok {
		if x.Nonce == c.Nonce && !x.Saved {
			return
		}
		s.assignEndgameTrial(p, c)
		return
	}
	if _, ok := x.Recoveries[p.ID]; !ok {
		x.Recoveries[p.ID] = endgameTrialRecovery{nonce: ch.Trial.CompletionID, load: s.endgameBackend.LoadRun("trial-" + ch.Trial.CompletionID)}
	}
}
