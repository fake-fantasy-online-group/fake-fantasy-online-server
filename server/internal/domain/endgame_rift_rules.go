package domain

import (
	"fmt"
	"reflect"
)

type RiftClassRule struct {
	Class          int32
	HPBP, DamageBP int32
}
type RiftAbilityRule struct {
	ID                          int32
	Name, Affix, Warning, Shape string
	Radius, InnerRadius, Length int32
	DelayMS                     int64
	DamagePct                   int32
}
type RiftRules struct {
	// Zero is the legacy serialized value and means one times population.
	FloorPopulationMultiplier                                                                                                            int32 `json:",omitempty"`
	Version, LootVersion                                                                                                                 int32
	MaxTier, ProgressMax, NormalProgress, GlobeProgress, BlueOrbs, GoldOrbs                                                              int32
	TimedMS, HardMS, ReconnectMS, DeathPenaltyMS                                                                                         int64
	PresenceBP, TierJumpSeconds                                                                                                          int32
	HPGrowthBP, DamageGrowthBP, PartyHPBP                                                                                                int32
	Equipment, Runes, DustBase, DustTierDivisor                                                                                          int32
	UniqueBaseChance, UniquePerTier, UniqueCap, GuaranteedUniqueTier                                                                     int32
	TimelyEquipment, TimelyRunes, TimelyDust, ExperienceDivisor                                                                          int32
	DustItem                                                                                                                             ItemID
	Name, BossName, ExitName, ExitGreeting, ExitSprite, ExitPortrait                                                                     string
	NPCModel                                                                                                                             int32
	AbilityMaxConcurrent, AbilityRange                                                                                                   int32
	AbilityRecoveryMS, AbilityStartMS, AbilityJitterMS, AbilityResetMS, AbilityUnavailableMS, AbilityCooldownMS, AbilityClassReductionMS int64
	AbilityBossDamagePct                                                                                                                 int32
	VisualSkill                                                                                                                          SkillID
	VisualSkillLevel                                                                                                                     int32
	Classes                                                                                                                              []RiftClassRule
	Abilities                                                                                                                            []RiftAbilityRule
	Loot                                                                                                                                 *LootRuleSet
}
type RiftRuleTable struct {
	active    int32
	versions  map[int32]RiftRules
	entrances []EndgameRiftEntranceView
}

// Entrances are world-service bindings, independent of frozen run balance rules.
func (t *RiftRuleTable) Entrances() []EndgameRiftEntranceView {
	if t == nil {
		return nil
	}
	return append([]EndgameRiftEntranceView(nil), t.entrances...)
}

func cloneRiftRules(r RiftRules) RiftRules {
	r.Classes = append([]RiftClassRule(nil), r.Classes...)
	r.Abilities = append([]RiftAbilityRule(nil), r.Abilities...)
	if r.Loot != nil {
		l := cloneLootRuleSet(*r.Loot)
		r.Loot = &l
	}
	return r
}
func (t *RiftRuleTable) Version(v int32) (*RiftRules, bool) {
	if t == nil {
		return nil, false
	}
	r, ok := t.versions[v]
	if !ok {
		return nil, false
	}
	r = cloneRiftRules(r)
	return &r, true
}
func (t *RiftRuleTable) Active() *RiftRules {
	if t == nil {
		return nil
	}
	r, _ := t.Version(t.active)
	return r
}
func (r *RiftRules) Validate() error {
	if r == nil || r.FloorPopulationMultiplier < 0 || r.FloorPopulationMultiplier > 2 || r.Version < 1 || r.Loot == nil || r.LootVersion != r.Loot.Version || r.MaxTier < 1 || r.MaxTier > 150 || r.ProgressMax < 1 || r.ProgressMax > 100000000 || r.NormalProgress < 1 || r.NormalProgress > r.ProgressMax || r.GlobeProgress < 1 || r.GlobeProgress > r.ProgressMax || r.BlueOrbs < 1 || r.BlueOrbs > 32 || r.GoldOrbs < 1 || r.GoldOrbs > 32 {
		return fmt.Errorf("invalid rift identity/progress rules")
	}
	if r.TimedMS < 1000 || r.HardMS < r.TimedMS || r.HardMS > 86400000 || r.ReconnectMS < 0 || r.ReconnectMS > r.HardMS || r.DeathPenaltyMS < 0 || r.DeathPenaltyMS > r.HardMS || r.PresenceBP < 1 || r.PresenceBP > 10000 || r.TierJumpSeconds < 1 {
		return fmt.Errorf("invalid rift time rules")
	}
	if r.HPGrowthBP < 10000 || r.HPGrowthBP > 100000 || r.DamageGrowthBP < 10000 || r.DamageGrowthBP > 100000 || r.PartyHPBP < 0 || r.PartyHPBP > 100000 || len(r.Classes) != 4 {
		return fmt.Errorf("invalid rift scaling")
	}
	for i, c := range r.Classes {
		if c.Class != int32(i) || c.HPBP < 1 || c.HPBP > 10000000 || c.DamageBP < 1 || c.DamageBP > 10000000 {
			return fmt.Errorf("invalid rift class")
		}
	}
	if r.Equipment < 0 || r.Equipment > 20 || r.Runes < 0 || r.Runes > 28 || r.TimelyEquipment < 0 || r.TimelyEquipment > 20 || r.TimelyRunes < 0 || r.TimelyRunes > 28 || r.DustBase < 0 || r.DustBase > 1000000 || r.TimelyDust < 0 || r.TimelyDust > 1000000 || r.DustTierDivisor < 1 || r.UniqueBaseChance < 0 || r.UniqueBaseChance > 100 || r.UniquePerTier < 0 || r.UniquePerTier > 100 || r.UniqueCap < 0 || r.UniqueCap > 100 || r.GuaranteedUniqueTier < 1 || r.GuaranteedUniqueTier > r.MaxTier || r.ExperienceDivisor < 1 || r.ExperienceDivisor > 1000000 {
		return fmt.Errorf("invalid rift rewards")
	}
	if r.Name == "" || r.BossName == "" || r.ExitName == "" || r.NPCModel < 1 || r.ExitSprite == "" || r.ExitPortrait == "" || r.AbilityMaxConcurrent < 1 || r.AbilityMaxConcurrent > 4 || r.AbilityRange < 1 || r.AbilityRange > 10000 || r.AbilityRecoveryMS < 0 || r.AbilityJitterMS < 1 || r.AbilityJitterMS > 86400000 || r.AbilityStartMS < 1 || r.AbilityResetMS < 1 || r.AbilityUnavailableMS < 1 || r.AbilityClassReductionMS < 0 || r.AbilityRecoveryMS > 86400000 || r.AbilityStartMS > 86400000 || r.AbilityResetMS > 86400000 || r.AbilityUnavailableMS > 86400000 || r.AbilityCooldownMS > 86400000 || r.AbilityClassReductionMS > 86400000 || r.AbilityCooldownMS <= 3*r.AbilityClassReductionMS || r.AbilityBossDamagePct < 0 || r.AbilityBossDamagePct > 10000 || len(r.Abilities) != 3 {
		return fmt.Errorf("invalid rift presentation/abilities")
	}
	for i, a := range r.Abilities {
		if a.ID != int32(i) || a.Name == "" || a.Affix == "" || a.Warning == "" || (a.Shape != "circle" && a.Shape != "ring" && a.Shape != "line") || a.Radius < 1 || a.Radius > 10000 || a.InnerRadius < 0 || a.InnerRadius >= a.Radius || a.Length < 0 || (a.Shape == "line" && a.Length == 0) || a.Length > 10000 || a.DelayMS < 1 || a.DelayMS > 60000 || a.DamagePct < 1 || a.DamagePct > 10000 {
			return fmt.Errorf("invalid rift ability %d", a.ID)
		}
	}
	return r.validateWire()
}
func NewRiftRuleTable(active int32, rows []RiftRules, loot *LootRuleTable, items map[ItemID]ItemDef, entrances ...EndgameRiftEntranceView) (*RiftRuleTable, error) {
	t := &RiftRuleTable{active: active, versions: map[int32]RiftRules{}}
	if len(entrances) > 32 {
		return nil, fmt.Errorf("too many rift entrances")
	}
	seen := map[[2]int32]bool{}
	for _, e := range entrances {
		key := [2]int32{e.MapID, e.NPCID}
		if e.MapID < 1 || e.NPCID < 1 || e.NPC == "" || e.Script == "" || len(e.NPC) > 128 || len(e.Script) > 128 || seen[key] {
			return nil, fmt.Errorf("invalid or duplicate rift entrance")
		}
		seen[key] = true
		t.entrances = append(t.entrances, e)
	}
	for _, r := range rows {
		if _, ok := t.versions[r.Version]; ok {
			return nil, fmt.Errorf("duplicate rift version")
		}
		var ok bool
		r.Loot, ok = loot.Version(r.LootVersion)
		if !ok {
			return nil, fmt.Errorf("missing loot version %d", r.LootVersion)
		}
		if err := r.Validate(); err != nil {
			return nil, err
		}
		d, ok := items[r.DustItem]
		if !ok || !d.Stackable {
			return nil, fmt.Errorf("invalid rift dust item")
		}
		t.versions[r.Version] = cloneRiftRules(r)
	}
	if _, ok := t.versions[1]; !ok {
		return nil, fmt.Errorf("missing legacy rift version 1")
	}
	if _, ok := t.versions[active]; !ok {
		return nil, fmt.Errorf("missing active rift version")
	}
	return t, nil
}

// Resolve binds unversioned legacy runs to published version1, never active.
func (t *RiftRuleTable) Resolve(version int32, snapshot *RiftRules) (*RiftRules, error) {
	if version == 0 {
		version = 1
	}
	r, ok := t.Version(version)
	if !ok {
		return nil, fmt.Errorf("missing persisted rift rules %d", version)
	}
	if snapshot != nil && !reflect.DeepEqual(*snapshot, *r) {
		return nil, fmt.Errorf("persisted rift rule snapshot mismatch")
	}
	return r, nil
}

func cloneRiftRulesPtr(r *RiftRules) *RiftRules {
	if r == nil {
		return nil
	}
	copy := cloneRiftRules(*r)
	return &copy
}
func (r *RiftRun) BindRules(t *RiftRuleTable) error {
	if r == nil {
		return fmt.Errorf("missing rift run")
	}
	rules, err := t.Resolve(r.RulesVersion, r.Rules)
	if err != nil {
		return err
	}
	r.RulesVersion = rules.Version
	r.Rules = rules
	return nil
}
