package scene

import (
	"encoding/json"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"sort"
)

type endgameTrialMember struct {
	Opening               domain.TrialKeyMember
	PresentMS, LastSeenMS int64
}
type endgameTrialCompletion struct {
	Version      int
	Kind, Nonce  string
	MonsterLevel int32
	Keys         map[domain.CharID]int32
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
	Completed, Saved  bool
	Saving            <-chan domain.EndgameRunSaveResult
	RetryAfter        int64
	Recoveries        map[domain.EntityID]endgameTrialRecovery
	RecoveryChecked   map[domain.EntityID]string
}

func (s *Scene) trialState() *endgameTrialState {
	if s.trialRun == nil {
		s.trialRun = &endgameTrialState{Members: map[domain.CharID]*endgameTrialMember{}, Keys: map[domain.CharID]int32{}, Recoveries: map[domain.EntityID]endgameTrialRecovery{}}
	}
	if s.trialRun.RecoveryChecked == nil {
		s.trialRun.RecoveryChecked = map[domain.EntityID]string{}
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
		x.Members[id] = &endgameTrialMember{Opening: domain.TrialKeyMember{CharID: id, Level: ch.Level, Cycle: ch.Trial.Cycle}, LastSeenMS: x.StartedMS}
		ch.Trial.CompletionID = x.Nonce
		ch.Trial.KeyReward = 0
		p.Player.MarkDirty()
		if s.saver != nil {
			s.saver.Save(s.snapshotOf(p).Clone())
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
				if result.Err == nil {
					x.Saved = true
				} else {
					x.RetryAfter = now + 1000
				}
			default:
			}
		}
		if !x.Saved && x.Saving == nil && now >= x.RetryAfter {
			if backend, ok := s.endgameBackend.(domain.EndgameRunBackend); ok {
				completion := endgameTrialCompletion{Version: 1, Kind: "trial", Nonce: x.Nonce, MonsterLevel: s.trialLevel, Keys: x.Keys}
				b, _ := json.Marshal(completion)
				x.Saving = backend.SaveRun(domain.EndgameRunRecord{RunID: "trial-" + x.Nonce, Revision: 1, State: "trial.completed", Payload: string(b)}, 0)
			}
		}
	}
	for id, recovery := range x.Recoveries {
		select {
		case result := <-recovery.load:
			delete(x.Recoveries, id)
			p := s.players[id]
			if p == nil || p.Player.Char.Trial.CompletionID != recovery.nonce || result.Err != nil {
				continue
			}
			if result.Run == nil {
				x.RecoveryChecked[id] = recovery.nonce
				continue
			}
			var c endgameTrialCompletion
			if json.Unmarshal([]byte(result.Run.Payload), &c) == nil && c.Version == 1 && c.Kind == "trial" && c.Nonce == recovery.nonce {
				x.RecoveryChecked[id] = recovery.nonce
				s.assignEndgameTrial(p, c)
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
	x.Completed = true
	s.stepEndgameTrial()
}
func (s *Scene) assignEndgameTrial(p *entity.Entity, c endgameTrialCompletion) {
	if s.trial == nil {
		return
	}
	ch := p.Player.Char
	q := ch.Quests[s.trial.Quest]
	n, ok := c.Keys[domain.CharID(ch.ID)]
	if !ok || q.State != domain.QuestActive || ch.Trial.CompletionID != c.Nonce {
		return
	}
	q.State = domain.QuestDeliver
	ch.Quests[s.trial.Quest] = q
	ch.Trial.MonsterLevel = c.MonsterLevel
	ch.Trial.KeyReward = n
	p.Player.MarkDirty()
	s.pushQuestLog(p)
	s.trialNotice(p, "试炼完成奖励已恢复，请回试炼小仙领取。")
}
func (s *Scene) recoverEndgameTrial(p *entity.Entity) {
	if s.endgameBackend == nil || s.trial == nil {
		return
	}
	ch := p.Player.Char
	if ch.Trial.CompletionID == "" || ch.Quests[s.trial.Quest].State != domain.QuestActive {
		return
	}
	x := s.trialState()
	if x.Nonce == ch.Trial.CompletionID && x.Completed {
		s.assignEndgameTrial(p, endgameTrialCompletion{Nonce: x.Nonce, MonsterLevel: s.trialLevel, Keys: x.Keys})
		return
	}
	if _, ok := x.Recoveries[p.ID]; !ok {
		x.Recoveries[p.ID] = endgameTrialRecovery{nonce: ch.Trial.CompletionID, load: s.endgameBackend.LoadRun("trial-" + ch.Trial.CompletionID)}
	}
}
