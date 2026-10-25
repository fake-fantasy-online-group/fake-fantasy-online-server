package domain

import (
	"encoding/json"
	"errors"
	"math"
)

const (
	RiftKeyItemID      ItemID = 1900028101
	RiftMaxTier        int32  = 150
	RiftProgressMax    int32  = 10000
	RiftNormalProgress int32  = 40
	RiftGlobeProgress  int32  = 200
	RiftTimedMS        int64  = 15 * 60 * 1000
	RiftHardMS         int64  = 20 * 60 * 1000
	RiftReconnectMS    int64  = 120000
	RiftDeathPenaltyMS int64  = 5000
)

type RiftPhase string

const (
	RiftReserved     RiftPhase = "reserved"
	RiftActive       RiftPhase = "active"
	RiftBoss         RiftPhase = "boss"
	RiftBossArriving RiftPhase = "boss_arriving"
	RiftCompleted    RiftPhase = "completed"
	RiftFailed       RiftPhase = "failed"
	RiftAborted      RiftPhase = "aborted"
	RiftClosed       RiftPhase = "closed"
)

type TrialKeyMember struct {
	CharID       CharID
	Level, Cycle int32
}

// TrialKeyReward freezes recipient round and opening levels. One base, up to
// four peers each worth 1+1 mentorship, and round 0..5: exactly 14 maximum.
func TrialKeyReward(recipient CharID, members []TrialKeyMember) (int32, error) {
	if len(members) < 1 || len(members) > 5 {
		return 0, errors.New("invalid roster")
	}
	seen := map[CharID]bool{}
	var own *TrialKeyMember
	for i := range members {
		m := &members[i]
		if m.CharID <= 0 || seen[m.CharID] || m.Level <= 0 || m.Cycle < 0 || m.Cycle > 5 {
			return 0, errors.New("invalid member")
		}
		seen[m.CharID] = true
		if m.CharID == recipient {
			own = m
		}
	}
	if own == nil {
		return 0, errors.New("not admitted")
	}
	n := int32(1) + own.Cycle
	for _, m := range members {
		if m.CharID != recipient {
			n++
			if own.Level-m.Level >= 10 {
				n++
			}
		}
	}
	return n, nil
}

type RiftMember struct {
	CharID                      CharID
	Level                       int32
	PresentMS                   int64
	LastSeenMS                  int64
	Departed, Eligible, Claimed bool
}
type RiftMonster struct {
	Class int32
	Dead  bool
}
type RiftGlobe struct {
	ID    EntityID
	Pos   Pos
	Value int32
}
type RiftRun struct {
	RulesVersion                                         int32
	Rules                                                *RiftRules
	RunID                                                string
	Revision                                             int64
	Phase                                                RiftPhase
	Tier                                                 int32
	Payer                                                CharID
	CreatedMS, StartedMS, CompletedMS, LastMS, PenaltyMS int64
	Progress                                             int32
	Timely                                               bool
	Reason                                               string
	Members                                              map[CharID]*RiftMember
	Monsters                                             map[EntityID]*RiftMonster
	Globes                                               map[EntityID]RiftGlobe
}

func NewRiftRun(id string, tier int32, payer CharID, roster []TrialKeyMember, now int64, rule *RiftRules) (*RiftRun, error) {
	if err := rule.Validate(); err != nil {
		return nil, err
	}
	if id == "" || tier < 1 || tier > rule.MaxTier {
		return nil, errors.New("invalid run")
	}
	if _, err := TrialKeyReward(payer, roster); err != nil {
		return nil, err
	}
	r := &RiftRun{RulesVersion: rule.Version, Rules: cloneRiftRulesPtr(rule), RunID: id, Revision: 1, Phase: RiftReserved, Tier: tier, Payer: payer, CreatedMS: now, LastMS: now, Members: map[CharID]*RiftMember{}, Monsters: map[EntityID]*RiftMonster{}, Globes: map[EntityID]RiftGlobe{}}
	for _, m := range roster {
		r.Members[m.CharID] = &RiftMember{CharID: m.CharID, Level: m.Level, LastSeenMS: now}
	}
	return r, nil
}
func (r *RiftRun) Clone() *RiftRun {
	b, _ := json.Marshal(r)
	var out RiftRun
	_ = json.Unmarshal(b, &out)
	return &out
}
func (r *RiftRun) Terminal() bool {
	return r.Phase == RiftCompleted || r.Phase == RiftClosed || r.Phase == RiftFailed || r.Phase == RiftAborted
}
func (r *RiftRun) Admitted(id CharID) bool { m := r.Members[id]; return m != nil && !m.Departed }
func (r *RiftRun) Activate(now int64) bool {
	if r.Rules == nil {
		return false
	}
	if r.Phase != RiftReserved {
		return false
	}
	r.Phase = RiftActive
	r.StartedMS = now
	r.LastMS = now
	r.Revision++
	return true
}
func (r *RiftRun) ObserveClock(now int64, present map[CharID]bool) {
	if r.Rules == nil {
		return
	}
	if r.Terminal() {
		return
	}
	delta := now - r.LastMS
	if delta < 0 {
		delta = 0
	}
	r.LastMS = now
	for id, m := range r.Members {
		if present[id] && !m.Departed {
			m.LastSeenMS = now
			if r.Phase != RiftReserved {
				m.PresentMS += delta
			}
		}
	}
	if r.Phase != RiftReserved && now-r.StartedMS >= r.Rules.HardMS {
		r.Phase = RiftFailed
		r.Reason = "timeout"
		r.Revision++
	}
}
func (r *RiftRun) RegisterMonster(id EntityID, class int32) bool {
	if id == 0 || class < 0 || class > 3 || r.Monsters[id] != nil {
		return false
	}
	r.Monsters[id] = &RiftMonster{Class: class}
	return true
}

// RecordKill returns required orb count. Progress only comes from registered,
// first authoritative deaths; guardian completion requires the boss phase.
func (r *RiftRun) RecordKill(id EntityID, now int64) (int, bool) {
	if r.Rules == nil {
		return 0, false
	}
	if r.Terminal() || r.Phase == RiftReserved {
		return 0, false
	}
	if now-r.StartedMS >= r.Rules.HardMS {
		r.Phase = RiftFailed
		r.Reason = "timeout"
		r.Revision++
	}
	if r.Terminal() {
		return 0, false
	}
	m := r.Monsters[id]
	if m == nil || m.Dead || (m.Class == 3 && r.Phase != RiftBoss) {
		return 0, false
	}
	m.Dead = true
	r.Revision++
	if m.Class == 3 {
		if r.Phase != RiftBoss {
			return 0, false
		}
		r.Phase = RiftCompleted
		r.CompletedMS = now
		r.Timely = now-r.StartedMS+r.PenaltyMS <= r.Rules.TimedMS
		elapsed := now - r.StartedMS
		if elapsed < 1 {
			elapsed = 1
		}
		for _, p := range r.Members {
			p.Eligible = !p.Departed && now-p.LastSeenMS <= r.Rules.ReconnectMS && p.PresentMS >= (elapsed*int64(r.Rules.PresenceBP)+9999)/10000
		}
		return 0, true
	}
	if r.Phase != RiftActive {
		return 0, false
	}
	switch m.Class {
	case 0:
		r.Progress += r.Rules.NormalProgress
	case 1:
		return int(r.Rules.BlueOrbs), true
	case 2:
		return int(r.Rules.GoldOrbs), true
	}
	r.advance()
	return 0, true
}
func (r *RiftRun) advance() {
	if r.Rules == nil {
		return
	}
	if r.Progress >= r.Rules.ProgressMax {
		r.Progress = r.Rules.ProgressMax
		r.Phase = RiftBoss
		if r.Rules.GuardianArrivalMS > 0 {
			r.Phase = RiftBossArriving
		}
		r.Globes = map[EntityID]RiftGlobe{}
	}
}
func (r *RiftRun) CollectGlobe(id EntityID, who CharID) bool {
	if r.Rules == nil {
		return false
	}
	if r.Phase != RiftActive || !r.Admitted(who) {
		return false
	}
	g, ok := r.Globes[id]
	if !ok {
		return false
	}
	delete(r.Globes, id)
	r.Progress += g.Value
	r.advance()
	r.Revision++
	return true
}
func (r *RiftRun) RecordDeath(id CharID) bool {
	if r.Rules == nil {
		return false
	}
	if !r.Admitted(id) || r.Terminal() || r.Phase == RiftReserved {
		return false
	}
	r.PenaltyMS += r.Rules.DeathPenaltyMS
	r.Revision++
	return true
}
func (r *RiftRun) Leave(id CharID) bool {
	m := r.Members[id]
	if m == nil || m.Departed {
		return false
	}
	if !r.Terminal() {
		m.Departed = true
		m.Eligible = false
		all := true
		for _, p := range r.Members {
			all = all && p.Departed
		}
		if all {
			r.Phase = RiftAborted
			r.Reason = "abandoned"
		}
	}
	r.Revision++
	return true
}
func (r *RiftRun) MarkClaimed(id CharID) bool {
	m := r.Members[id]
	if (r.Phase != RiftCompleted && r.Phase != RiftClosed) || m == nil || !m.Eligible || m.Claimed {
		return false
	}
	m.Claimed = true
	r.Revision++
	return true
}

// RiftScaledStat scales difficulty (阶), never the run's map-floor index (层).
// Invalid inputs are clamped to tier 1..150, party 1..5 and class 0..3
// (normal, blue elite, yellow elite, guardian); nonpositive bases stay zero.
// Clamp before subtracting or exponentiating so even extreme inputs cannot
// overflow intermediates. The final wire-compatible stat saturates at int32
// before the versioned guardian-only HP reduction is applied.
func RiftScaledStat(base int32, tier int32, party int, class int32, hp bool, rule *RiftRules) int32 {
	if rule == nil {
		return 0
	}
	if base <= 0 {
		return 0
	}
	tier = clampRiftTier(tier, rule)
	party = max(1, min(party, 5))
	class = max(int32(0), min(class, int32(3)))
	v := float64(base)
	rate := float64(rule.DamageGrowthBP) / 10000
	if hp {
		rate = float64(rule.HPGrowthBP) / 10000
		v *= 1 + float64(rule.PartyHPBP)/10000*float64(party-1)
	}
	v *= math.Pow(rate, float64(tier-max(int32(1), rule.StatAnchorTier)))
	if hp {
		v *= float64(rule.Classes[class].HPBP) / 10000
	} else {
		v *= float64(rule.Classes[class].DamageBP) / 10000
	}
	result := int32(math.MaxInt32)
	if v <= math.MaxInt32 {
		result = int32(math.Ceil(v))
	}
	if hp && class == 3 && rule.GuardianHPDivisor > 0 {
		result = max(int32(1), result/rule.GuardianHPDivisor)
	}
	return result
}

// Loot timing bonuses are separate from difficulty progression; use
// RiftRun.UnlockedTierFor for the completion-time tier entitlement.
type RiftRewardPlan struct {
	Equipment, NormalBossEquipment, Rune, Dust, UniqueChance int32
	GuaranteedLegendaries, ExtraLegendaryChance              int32
}

func PlanRiftRewards(tier int32, timely bool, rule *RiftRules) RiftRewardPlan {
	if rule == nil {
		return RiftRewardPlan{}
	}
	tier = clampRiftTier(tier, rule)
	p := RiftRewardPlan{Equipment: rule.Equipment, NormalBossEquipment: rule.NormalBossEquipment, Rune: rule.Runes, Dust: rule.DustBase + (tier-1)/rule.DustTierDivisor, UniqueChance: rule.UniqueBaseChance + rule.UniquePerTier*tier}
	if p.UniqueChance > rule.UniqueCap {
		p.UniqueChance = rule.UniqueCap
	}
	if timely {
		p.Equipment += rule.TimelyEquipment
		p.Rune += rule.TimelyRunes
		p.Dust += rule.TimelyDust
	}
	if rule.CumulativeLegendaryPerTier > 0 {
		// Replace, rather than multiply, the old special slot and its timely bonus.
		p.Equipment, p.UniqueChance = 0, 0
		cumulative := tier * rule.CumulativeLegendaryPerTier
		p.GuaranteedLegendaries, p.ExtraLegendaryChance = cumulative/100, cumulative%100
	}
	return p
}

// One roll only for the remainder; multiples of 100 are deterministic.
func (p RiftRewardPlan) CumulativeLegendaryCount(roll func(int) int) int32 {
	count := p.GuaranteedLegendaries
	if p.ExtraLegendaryChance > 0 && roll(100) < int(p.ExtraLegendaryChance) {
		count++
	}
	return count
}

func clampRiftTier(tier int32, rule *RiftRules) int32 {
	if rule == nil {
		return 0
	}
	return max(int32(1), min(tier, rule.MaxTier))
}
