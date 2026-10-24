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
	RecentGuardians []domain.MonsterID `json:"recentGuardians,omitempty"`
	RunID           string             `json:"runId"`
	UnlockedTier    int32              `json:"unlockedTier"`
}
type endgameRiftGrant struct {
	ExperienceLevel    int32 `json:",omitempty"`
	ExperienceBase     int64 `json:",omitempty"`
	MagicFind          int64 `json:",omitempty"`
	ExperienceClaimed  bool
	Collected          map[int]bool
	Items              []domain.Stack
	Exp                int64
	ProgressionVersion string
	RewardVersion      string
	Pet                endgamePetGrant
}
type endgameRiftPersisted struct {
	// Actual guardian encounter level, frozen before deferred grant generation.
	RewardSourceLevel int32                       `json:",omitempty"`
	GuardianID        domain.MonsterID            `json:",omitempty"`
	Elite             *endgameRiftEliteSnapshot   `json:",omitempty"`
	GuardianArrival   *endgameRiftGuardianArrival `json:",omitempty"`
	RewardPos         domain.Pos
	DeathTimes        map[domain.CharID]int64
	Floor             *domain.RiftFloorPlan
	StorageRevision   int64
	Version           int
	Run               *domain.RiftRun
	MapID             int32
	Return            domain.Pos
	Names             map[domain.CharID]string
	Profiles          map[domain.CharID]endgameRiftIdentity
	Grants            map[domain.CharID]endgameRiftGrant
	GrantsReady       bool
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
func decodeEndgameRift(record *domain.EndgameRunRecord, table *domain.RiftRuleTable) (endgameRiftPersisted, bool) {
	var d endgameRiftPersisted
	if record == nil || json.Unmarshal([]byte(record.Payload), &d) != nil || (d.Version != 1 && d.Version != 2) || d.Run == nil || d.Run.RunID != record.RunID {
		return d, false
	}
	if d.Run.BindRules(table) != nil || !validRiftGuardianArrival(d) || !validRiftEliteSnapshot(d) || !validRiftGuardianSelection(d) {
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
	p.RecentGuardians = boundedRiftGuardianHistory(p.RecentGuardians)
	return p
}
func withRiftProfile(payload string, p endgameRiftProfile) string {
	var all map[string]json.RawMessage
	_ = json.Unmarshal([]byte(payload), &all)
	if all == nil {
		all = map[string]json.RawMessage{}
	}
	p.RecentGuardians = boundedRiftGuardianHistory(p.RecentGuardians)
	b, _ := json.Marshal(p)
	all["rift"] = b
	b, _ = json.Marshal(all)
	return string(b)
}

type endgameRiftState struct {
	hazardImpacts                                     []endgameRiftImpact
	eliteNeedsRearm                                   bool
	hazardViewerGeneration                            map[domain.EntityID]uint64
	savingRecord                                      *domain.EndgameRunRecord
	reconcilingSave                                   *domain.EndgameRunRecord
	originalNames                                     map[domain.EntityID]string
	saveRetryAt                                       int64
	rewardLayout                                      *endgameRiftRewardLayout
	rewardFresh                                       bool
	rewardDrops                                       map[domain.EntityID]endgameRiftGroundReward
	exitNPC                                           domain.EntityID
	portals                                           []domain.Portal
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
	now := s.riftMS()
	r := d.Run
	v := &domain.EndgameRiftView{RunID: r.RunID, Revision: strconv.FormatInt(r.Revision, 10), Name: r.Rules.Name, KeyItemID: int32(domain.RiftKeyItemID), Phase: string(r.Phase), Tier: r.Tier, Progress: r.Progress, ProgressMax: r.Rules.ProgressMax, BossName: r.Rules.BossName}
	if r.Rules.Elite != nil {
		v.MapID = d.MapID
		v.ServerNowMS = now
	}
	v.MaxTier = r.Rules.MaxTier
	v.GuardianArrival = riftGuardianArrivalView(d, now)
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
		if durable {
			v.UnlockedTier = r.UnlockedTierFor(domain.CharID(p.Player.Char.ID), v.UnlockedTier)
		}
		if s.rift != nil && live && s.rift.data.Run.RunID == r.RunID {
			v.Transitioning = s.rift.transition != nil || s.rift.floorLoading || s.rift.retired
			v.CanAdvance = s.canAdvanceEndgameRift(p)
			v.Telegraphs = s.riftAbilityTelegraphs(now)
			v.Hazards = s.riftHazardViews(now)
			v.Impacts = s.riftImpactViews(p, now)
		}
		v.KeyCount = p.Player.Bag.CountOf(domain.RiftKeyItemID)
		char := domain.CharID(p.Player.Char.ID)
		if member := r.Members[char]; member != nil && member.Eligible && !member.Claimed {
			for i := range d.Grants[char].Items {
				if !d.Grants[char].Collected[i] {
					v.PendingLoot++
				}
			}
		}
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
	v.RemainingSeconds = r.RemainingSeconds(now)
	// A reconnect or floor change retains the original run rules, never a client default.
	v.TotalSeconds = int32(r.Rules.TimedMS / 1000)
	for id, m := range r.Members {
		v.Members = append(v.Members, domain.EndgameRiftMemberView{CharID: strconv.FormatInt(int64(id), 10), Name: d.Names[id], Level: m.Level, Eligible: m.Eligible, Claimed: m.Claimed})
		if p != nil && int64(id) == p.Player.Char.ID && m.Eligible && !m.Claimed && durable && (r.Phase == domain.RiftCompleted || r.Phase == domain.RiftClosed) {
			v.Claimable = true
		}
	}
	if live && (r.Phase == domain.RiftCompleted || r.Phase == domain.RiftClosed) {
		v.Claimable = false // live entitlements are collected from ground entities
	}
	if live {
		for id, m := range r.Monsters {
			if !m.Dead {
				abilities := s.riftMonsterAbilityIDs(id)
				goldenUntil := s.riftGoldenUntil(id, now)
				// Existing actors created while golden body was incorrectly filtered
				// can retain fewer abilities than their elite class. Legacy clients
				// accept this optional field absent, but reject a short nonempty list
				// and discard the entire progress/identity snapshot. Do not invent an
				// ability, change the elite class, or mutate a live encounter to fix UI.
				if len(abilities) < int(m.Class) {
					abilities = nil
					goldenUntil = 0 // A golden deadline requires its explicit ability ID.
				}
				v.Auras = append(v.Auras, domain.EndgameRiftAuraView{EntityID: strconv.FormatUint(uint64(id), 10), Class: m.Class, OriginalName: s.riftMonsterOriginalName(id), Affixes: s.riftMonsterAffixes(id), AbilityIDs: abilities, GoldenUntilMS: goldenUntil})
			}
		}
		if s.eliteRift() && s.rift.data.Elite != nil {
			for id, c := range s.rift.data.Elite.Clones {
				if m := s.monsters[id]; m != nil && m.Alive() {
					v.Auras = append(v.Auras, domain.EndgameRiftAuraView{EntityID: strconv.FormatUint(uint64(id), 10), Class: 0, CloneOf: strconv.FormatUint(uint64(c.Owner), 10)})
				}
			}
		}
		for id, g := range r.Globes {
			v.Globes = append(v.Globes, domain.EndgameRiftGlobeView{EntityID: strconv.FormatUint(uint64(id), 10), X: int32(g.Pos.X), Y: int32(g.Pos.Y), Value: g.Value})
		}
	}
	if v.Phase != string(domain.RiftBossArriving) {
		v.GuardianArrival = nil
	}
	return v
}

// Publish private context before, public context after the map lifecycle edge.
// The initial login message can be filtered
// before negotiation, so hello repeats it before the normal sidecar snapshots.
func (s *Scene) sendRiftSceneContext(p *entity.Entity) {
	if !p.Player.EndgameRiftPortals {
		return
	}
	inside := s.rift != nil && !s.id.Persistent()
	result := map[string]string{"inInstance": strconv.FormatBool(inside)}
	if inside && s.eliteRift() {
		floor := int32(0)
		if s.rift.data.Floor != nil {
			floor = s.rift.data.Floor.FloorIndex
		}
		result["runId"] = s.rift.data.Run.RunID
		result["floorIndex"] = strconv.FormatInt(int64(floor), 10)
		result["mapId"] = strconv.FormatInt(int64(s.id.MapID), 10)
		result["serverNowMS"] = strconv.FormatInt(s.riftMS(), 10)
	}
	s.endgameReply(p, domain.EndgameResponse{Kind: "rift.context", OK: true, Result: result})
}
