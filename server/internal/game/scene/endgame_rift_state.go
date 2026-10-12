package scene

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"strconv"
)

type endgameRiftProfile struct {
	RunID        string `json:"runId"`
	UnlockedTier int32  `json:"unlockedTier"`
}
type endgameRiftGrant struct {
	Items              []domain.Stack
	Exp                int64
	ProgressionVersion string
	RewardVersion      string
	Pet                endgamePetGrant
}
type endgameRiftPersisted struct {
	DeathTimes      map[domain.CharID]int64
	Floor           *domain.RiftFloorPlan
	StorageRevision int64
	Version         int
	Run             *domain.RiftRun
	MapID           int32
	Return          domain.Pos
	Names           map[domain.CharID]string
	Profiles        map[domain.CharID]endgameRiftIdentity
	Grants          map[domain.CharID]endgameRiftGrant
	GrantsReady     bool
}

func (d endgameRiftPersisted) clone() endgameRiftPersisted {
	b, _ := json.Marshal(d)
	var out endgameRiftPersisted
	_ = json.Unmarshal(b, &out)
	return out
}
func (d endgameRiftPersisted) record(rev int64) domain.EndgameRunRecord {
	d.StorageRevision = rev
	b, _ := json.Marshal(d)
	return domain.EndgameRunRecord{RunID: d.Run.RunID, Revision: rev, State: string(d.Run.Phase), Payload: string(b)}
}
func decodeEndgameRift(record *domain.EndgameRunRecord) (endgameRiftPersisted, bool) {
	var d endgameRiftPersisted
	if record == nil || json.Unmarshal([]byte(record.Payload), &d) != nil || (d.Version != 1 && d.Version != 2) || d.Run == nil || d.Run.RunID != record.RunID {
		return d, false
	}
	return d, true
}
func riftNonce() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
func riftProfile(payload string) endgameRiftProfile {
	var all map[string]json.RawMessage
	_ = json.Unmarshal([]byte(payload), &all)
	var p endgameRiftProfile
	_ = json.Unmarshal(all["rift"], &p)
	if p.UnlockedTier > domain.RiftMaxTier {
		p.UnlockedTier = domain.RiftMaxTier
	}
	if p.UnlockedTier < 1 {
		p.UnlockedTier = 1
	}
	return p
}
func withRiftProfile(payload string, p endgameRiftProfile) string {
	var all map[string]json.RawMessage
	_ = json.Unmarshal([]byte(payload), &all)
	if all == nil {
		all = map[string]json.RawMessage{}
	}
	b, _ := json.Marshal(p)
	all["rift"] = b
	b, _ = json.Marshal(all)
	return string(b)
}

type endgameRiftState struct {
	abilities                                         map[domain.EntityID]*endgameRiftAbilityState
	transition                                        *endgameRiftTransition
	retired                                           bool
	floorLoading                                      bool
	baseStats                                         map[domain.EntityID]domain.Stats
	reload                                            <-chan domain.EndgameRunLoadResult
	reloadAt                                          int64
	data                                              endgameRiftPersisted
	revision, savedRevision, savingRevision           int64
	saving                                            <-chan domain.EndgameRunSaveResult
	ready                                             map[domain.CharID]bool
	deathAt                                           map[domain.CharID]int64
	loadUntil, terminalUntil, lastSave, lastBroadcast int64
	economicPending                                   bool
	delayedDeaths                                     []endgameRiftDeath
	delayedPlayerDeaths                               []domain.CharID
}
type endgameRiftRequestState struct {
	req   domain.EndgameRequest
	load  <-chan domain.EndgameRunLoadResult
	prior bool
}

func (s *Scene) riftMS() int64 { return s.now().UnixMilli() }
func riftNeedsCompensation(r *domain.RiftRun, live bool, char domain.CharID) bool {
	return r.Payer == char && ((!live && !r.Terminal()) || (r.Phase == domain.RiftFailed && (r.Reason == "loading_failed" || r.Reason == "spawn_failed")))
}
func (s *Scene) riftView(p *entity.Entity, d endgameRiftPersisted, durable, live bool) *domain.EndgameRiftView {
	r := d.Run
	v := &domain.EndgameRiftView{RunID: r.RunID, Revision: strconv.FormatInt(r.Revision, 10), Name: "演劫洞天", KeyItemID: int32(domain.RiftKeyItemID), Phase: string(r.Phase), Tier: r.Tier, Progress: r.Progress, ProgressMax: domain.RiftProgressMax, BossName: "镇境劫影"}
	v.MaxTier = domain.RiftMaxTier
	if d.Floor != nil {
		v.Seed = strconv.FormatUint(d.Floor.Seed, 10)
		v.GeneratorVersion = d.Floor.GeneratorVersion
		v.FloorIndex = d.Floor.FloorIndex
		v.MapName = d.Floor.MapName
		v.ExitX = int32(d.Floor.Exit.X)
		v.ExitY = int32(d.Floor.Exit.Y)
	}
	if p != nil {
		v.UnlockedTier = riftProfile(s.endgamePlayer(p).state.Payload).UnlockedTier
		if s.rift != nil && live {
			v.Transitioning = s.rift.transition != nil || s.rift.floorLoading || s.rift.retired
			v.CanAdvance = s.canAdvanceEndgameRift(p)
			v.Telegraphs = s.riftAbilityTelegraphs(s.riftMS())
		}
		v.KeyCount = p.Player.Bag.CountOf(domain.RiftKeyItemID)
	}
	if p != nil && live {
		if inst, ok := s.router.riftScene(r.RunID); ok && !s.router.ActiveRiftForCharacter(inst, domain.CharID(p.Player.Char.ID)) {
			v.Phase = "failed"
		}
	}
	if p != nil && riftNeedsCompensation(r, live, domain.CharID(p.Player.Char.ID)) {
		v.Phase = "interrupted"
		v.Claimable = true
	}

	if r.Phase == domain.RiftCompleted && !durable {
		v.Phase = "reward_pending"
	}
	if !live {
		if !r.Terminal() {
			if p != nil && domain.CharID(p.Player.Char.ID) == r.Payer {
				v.Phase = "interrupted"
				v.Claimable = true
			} else {
				v.Phase = "failed"
			}
		} else if r.Phase == domain.RiftCompleted {
			v.Phase = "closed"
		}
	}
	remain := (domain.RiftTimedMS - (s.riftMS() - r.StartedMS) - r.PenaltyMS) / 1000
	if remain > 0 && r.StartedMS > 0 {
		v.RemainingSeconds = int32(remain)
	}
	for id, m := range r.Members {
		v.Members = append(v.Members, domain.EndgameRiftMemberView{CharID: strconv.FormatInt(int64(id), 10), Name: d.Names[id], Level: m.Level, Eligible: m.Eligible, Claimed: m.Claimed})
		if p != nil && int64(id) == p.Player.Char.ID && m.Eligible && !m.Claimed && durable && (r.Phase == domain.RiftCompleted || r.Phase == domain.RiftClosed) {
			v.Claimable = true
		}
	}
	if live {
		for id, m := range r.Monsters {
			if !m.Dead {
				v.Auras = append(v.Auras, domain.EndgameRiftAuraView{EntityID: strconv.FormatUint(uint64(id), 10), Class: m.Class, Affixes: s.riftMonsterAffixes(id)})
			}
		}
		for id, g := range r.Globes {
			v.Globes = append(v.Globes, domain.EndgameRiftGlobeView{EntityID: strconv.FormatUint(uint64(id), 10), X: int32(g.Pos.X), Y: int32(g.Pos.Y), Value: g.Value})
		}
	}
	return v
}
