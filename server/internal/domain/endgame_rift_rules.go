package domain

import (
	"fmt"
	"reflect"
)

type RiftClassRule struct {
	Class          int32
	HPBP, DamageBP int32
}

// RiftEliteRules is present only in the new, opt-in rules version. Historical
// snapshots omit it and retain their exact three-ability implementation.
type RiftEliteRules struct {
	HazardMaxConcurrent int32
	Guardians           []RiftGuardianRule
}
type RiftGuardianRule struct {
	MonsterID MonsterID
	Abilities []int32
}
type RiftAbilityRule struct {
	Mechanism          string `json:",omitempty"`
	Trigger            string `json:",omitempty"`
	CooldownMS         int64  `json:",omitempty"`
	DurationMS         int64  `json:",omitempty"`
	PulseMS            int64  `json:",omitempty"`
	FirstPulseMS       int64  `json:",omitempty"`
	PropagationMS      int64  `json:",omitempty"`
	Width              int32  `json:",omitempty"`
	Count              int32  `json:",omitempty"`
	HPPct              int32  `json:",omitempty"`
	StepDamagePct      int32  `json:",omitempty"`
	MaxDamagePct       int32  `json:",omitempty"`
	FreezeMS           int64  `json:",omitempty"`
	MoveSpeedPct       int32  `json:",omitempty"`
	DamageReductionPct int32  `json:",omitempty"`
	Gap                int32  `json:",omitempty"`

	ID                          int32
	Name, Affix, Warning, Shape string
	Radius, InnerRadius, Length int32
	DelayMS                     int64
	DamagePct                   int32
}
type RiftRules struct {
	// Nil preserves historical paid runs. v14 freezes existing feather rewards.
	WingRewards *RiftWingRewardRules `json:",omitempty"`
	// Zero preserves historical floor generation and guardian selection.
	FloorLayoutVersion   int32               `json:",omitempty"`
	GuardianRecentWindow int32               `json:",omitempty"`
	EquipmentPools       []RiftEquipmentPool `json:",omitempty"`
	// Optional frozen native IDs; only v11+ fills missing face/backpack slots.
	EquipmentSlotFallbacks []ItemID        `json:",omitempty"`
	Elite                  *RiftEliteRules `json:",omitempty"`
	// Zero keeps historical immediate guardians; v5 freezes a five-second arrival.
	GuardianArrivalMS int64 `json:",omitempty"`
	// New runs freeze reward level at guardian death; old runs keep admission level.
	CompletionLevelExperience bool `json:",omitempty"`
	// Zero preserves historical guardian HP; v13 divides the final scaled HP by ten.
	GuardianHPDivisor int32 `json:",omitempty"`
	// Zero preserves historical snapshots: tier one is the unscaled anchor.
	StatAnchorTier int32 `json:",omitempty"`
	// New v12 reward policy: each 100 cumulative percent produces one Legendary.
	// Zero preserves old paid-run equipment, chance cap and timely bonus behavior.
	CumulativeLegendaryPerTier int32 `json:",omitempty"`
	// Additional fixed-count equipment uses natural boss quality, separate from special slots.
	NormalBossEquipment int32 `json:",omitempty"`
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
	if r.WingRewards != nil {
		w := *r.WingRewards
		if w.RareFeather != nil {
			rare := *w.RareFeather
			w.RareFeather = &rare
		}
		w.Milestones = append([]RiftWingMilestone(nil), w.Milestones...)
		w.RepeatBands = append([]RiftWingRepeatBand(nil), w.RepeatBands...)
		for i := range w.Milestones {
			w.Milestones[i].Materials = append([]CubeMaterialDef(nil), w.Milestones[i].Materials...)
		}
		for i := range w.RepeatBands {
			w.RepeatBands[i].Materials = append([]CubeMaterialDef(nil), w.RepeatBands[i].Materials...)
		}
		r.WingRewards = &w
	}
	if r.EquipmentSlotFallbacks != nil {
		r.EquipmentSlotFallbacks = append([]ItemID{}, r.EquipmentSlotFallbacks...)
	}
	if r.EquipmentPools != nil {
		r.EquipmentPools = append([]RiftEquipmentPool{}, r.EquipmentPools...)
		for i := range r.EquipmentPools {
			r.EquipmentPools[i].Items = append([]ItemID(nil), r.EquipmentPools[i].Items...)
			r.EquipmentPools[i].SourceMonsters = append([]MonsterID(nil), r.EquipmentPools[i].SourceMonsters...)
		}
	}
	if r.Elite != nil {
		e := *r.Elite
		e.Guardians = append([]RiftGuardianRule(nil), e.Guardians...)
		for i := range e.Guardians {
			e.Guardians[i].Abilities = append([]int32(nil), e.Guardians[i].Abilities...)
		}
		r.Elite = &e
	}
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
	if r != nil && ((r.FloorLayoutVersion == RiftFloorFullCoverageVersion && r.Version < 10) || (r.FloorLayoutVersion == RiftFloorDenseCoverageVersion && r.Version < 9) || (r.FloorLayoutVersion != 0 && r.FloorLayoutVersion != RiftFloorCoverageVersion && r.FloorLayoutVersion != RiftFloorDenseCoverageVersion && r.FloorLayoutVersion != RiftFloorFullCoverageVersion) || (r.GuardianRecentWindow != 0 && r.GuardianRecentWindow != 3) || ((r.FloorLayoutVersion != 0 || r.GuardianRecentWindow != 0) && (r.Version < 8 || r.Elite == nil))) {
		return fmt.Errorf("invalid versioned rift layout/guardian policy")
	}
	if r == nil || r.FloorPopulationMultiplier < 0 || r.FloorPopulationMultiplier > 2 || r.Version < 1 || r.Loot == nil || r.LootVersion != r.Loot.Version || r.MaxTier < 1 || r.MaxTier > 150 || r.ProgressMax < 1 || r.ProgressMax > 100000000 || r.NormalProgress < 1 || r.NormalProgress > r.ProgressMax || r.GlobeProgress < 1 || r.GlobeProgress > r.ProgressMax || r.BlueOrbs < 1 || r.BlueOrbs > 32 || r.GoldOrbs < 1 || r.GoldOrbs > 32 {
		return fmt.Errorf("invalid rift identity/progress rules")
	}
	if (r.Version < 16 && r.Loot.LegendaryPowerPolicy != 0) || (r.Version >= 16 && r.Loot.LegendaryPowerPolicy != 1) {
		return fmt.Errorf("invalid versioned rift legendary power policy")
	}
	if r.Loot.RetiredCubeMaterials != (r.Version >= 18) || (r.Version >= 18 && r.LootVersion < 6) {
		return fmt.Errorf("invalid versioned rift retired material policy")
	}
	if r.Loot.LegendaryPowerRolls != (r.Version >= 17) || (r.Version >= 17 && r.LootVersion < 5) {
		return fmt.Errorf("invalid versioned rift numerical power roll policy")
	}
	if (r.GuardianArrivalMS != 0 && r.GuardianArrivalMS != 5000) || r.TimedMS < 1000 || r.HardMS < r.TimedMS || r.HardMS > 86400000 || r.ReconnectMS < 0 || r.ReconnectMS > r.HardMS || r.DeathPenaltyMS < 0 || r.DeathPenaltyMS > r.HardMS || r.PresenceBP < 1 || r.PresenceBP > 10000 || r.TierJumpSeconds < 1 {
		return fmt.Errorf("invalid rift time rules")
	}
	if r.StatAnchorTier < 0 || r.StatAnchorTier > r.MaxTier || r.HPGrowthBP < 10000 || r.HPGrowthBP > 100000 || r.DamageGrowthBP < 10000 || r.DamageGrowthBP > 100000 || r.PartyHPBP < 0 || r.PartyHPBP > 100000 || len(r.Classes) != 4 {
		return fmt.Errorf("invalid rift scaling")
	}
	if (r.GuardianHPDivisor != 0 && (r.Version < 13 || r.GuardianHPDivisor != 10)) || (r.Version >= 13 && r.GuardianHPDivisor != 10) {
		return fmt.Errorf("invalid rift guardian HP divisor")
	}
	for i, c := range r.Classes {
		if c.Class != int32(i) || c.HPBP < 1 || c.HPBP > 10000000 || c.DamageBP < 1 || c.DamageBP > 10000000 {
			return fmt.Errorf("invalid rift class")
		}
	}
	if r.NormalBossEquipment < 0 || r.NormalBossEquipment > 20 || r.Equipment < 0 || r.Equipment > 20 || r.Runes < 0 || r.Runes > 28 || r.TimelyEquipment < 0 || r.TimelyEquipment > 20 || r.TimelyRunes < 0 || r.TimelyRunes > 28 || r.DustBase < 0 || r.DustBase > 1000000 || r.TimelyDust < 0 || r.TimelyDust > 1000000 || r.DustTierDivisor < 1 || r.UniqueBaseChance < 0 || r.UniqueBaseChance > 100 || r.UniquePerTier < 0 || r.UniquePerTier > 100 || r.UniqueCap < 0 || r.UniqueCap > 100 || r.GuaranteedUniqueTier < 1 || r.GuaranteedUniqueTier > r.MaxTier || r.ExperienceDivisor < 1 || r.ExperienceDivisor > 1000000 {
		return fmt.Errorf("invalid rift rewards")
	}
	if r.CumulativeLegendaryPerTier != 0 {
		if r.Version < 12 || r.CumulativeLegendaryPerTier != 10 || r.Equipment != 0 || r.TimelyEquipment != 0 || r.UniqueBaseChance != 0 || r.UniquePerTier != 0 || r.UniqueCap != 0 || r.NormalBossEquipment != 10 || r.Loot.OutdoorEquipment == nil {
			return fmt.Errorf("invalid cumulative rift reward policy")
		}
	} else if r.Version >= 12 {
		return fmt.Errorf("missing cumulative rift reward policy")
	}
	if r.Name == "" || r.BossName == "" || r.ExitName == "" || r.NPCModel < 1 || r.ExitSprite == "" || r.AbilityMaxConcurrent < 1 || r.AbilityMaxConcurrent > 4 || r.AbilityRange < 1 || r.AbilityRange > 10000 || r.AbilityRecoveryMS < 0 || r.AbilityJitterMS < 1 || r.AbilityJitterMS > 86400000 || r.AbilityStartMS < 1 || r.AbilityResetMS < 1 || r.AbilityUnavailableMS < 1 || r.AbilityClassReductionMS < 0 || r.AbilityRecoveryMS > 86400000 || r.AbilityStartMS > 86400000 || r.AbilityResetMS > 86400000 || r.AbilityUnavailableMS > 86400000 || r.AbilityCooldownMS > 86400000 || r.AbilityClassReductionMS > 86400000 || r.AbilityCooldownMS <= 3*r.AbilityClassReductionMS || r.AbilityBossDamagePct < 0 || r.AbilityBossDamagePct > 10000 || (r.Elite == nil && len(r.Abilities) != 3) {
		return fmt.Errorf("invalid rift presentation/abilities")
	}
	if r.Elite != nil {
		if err := r.validateEliteAbilities(); err != nil {
			return err
		}
	} else {
		for i, a := range r.Abilities {
			if a.ID != int32(i) || a.Name == "" || a.Affix == "" || a.Warning == "" || (a.Shape != "circle" && a.Shape != "ring" && a.Shape != "line") || a.Radius < 1 || a.Radius > 10000 || a.InnerRadius < 0 || a.InnerRadius >= a.Radius || a.Length < 0 || (a.Shape == "line" && a.Length == 0) || a.Length > 10000 || a.DelayMS < 1 || a.DelayMS > 60000 || a.DamagePct < 1 || a.DamagePct > 10000 {
				return fmt.Errorf("invalid rift ability %d", a.ID)
			}
		}
	}
	if err := r.validateEquipmentPools(nil); err != nil {
		return err
	}
	if err := r.validateWingRewards(nil); err != nil {
		return err
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
		if err := r.validateEquipmentPools(items); err != nil {
			return nil, err
		}
		if err := r.validateWingRewards(items); err != nil {
			return nil, err
		}
		if err := r.validateSpiritRewards(items); err != nil {
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

func (r *RiftRules) validateEliteAbilities() error {
	if r.Version < 6 || r.Elite.HazardMaxConcurrent < 1 || r.Elite.HazardMaxConcurrent > 24 || len(r.Abilities) != 8 || len(r.Elite.Guardians) != 8 {
		return fmt.Errorf("invalid elite catalog")
	}
	expected := []string{"golden_body", "clones", "death_lava", "lightning_orb", "ice_orb", "agility", "poison_pool", "walls"}
	for i, a := range r.Abilities {
		trigger := "active"
		if i == 2 {
			trigger = "death"
		}
		if i == 5 {
			trigger = "passive"
		}
		if a.ID != int32(i) || a.Mechanism != expected[i] || a.Trigger != trigger || a.Name == "" || a.Affix == "" || a.Warning == "" || a.CooldownMS < 0 || a.CooldownMS > 60000 || a.DurationMS < 0 || a.DurationMS > 60000 || a.DelayMS < 0 || a.DelayMS > 60000 || a.DamagePct < 0 || a.DamagePct > 10000 || a.Radius < 0 || a.Radius > 10000 {
			return fmt.Errorf("invalid elite ability %d", i)
		}
		if trigger == "active" && a.CooldownMS <= 0 || trigger != "active" && a.CooldownMS != 0 {
			return fmt.Errorf("invalid elite cooldown %d", i)
		}
		switch a.Mechanism {
		case "golden_body":
			if a.DurationMS != 5000 || a.DamageReductionPct != 95 || a.CooldownMS != 30000 {
				return fmt.Errorf("invalid golden body")
			}
		case "clones":
			if a.Count != 3 || a.HPPct != 10 || a.DurationMS <= 0 || a.CooldownMS != 30000 {
				return fmt.Errorf("invalid clones")
			}
		case "death_lava", "ice_orb":
			if a.DelayMS != 3000 || a.DurationMS != a.DelayMS || a.Radius <= 0 || a.DamagePct <= 0 || a.Shape != "circle" {
				return fmt.Errorf("invalid orb")
			}
			if i == 4 && (a.FreezeMS < 1 || a.FreezeMS > 10000 || a.FreezeMS%1000 != 0 || a.CooldownMS != 20000) {
				return fmt.Errorf("invalid ice freeze")
			}
		case "lightning_orb":
			if a.DurationMS != 10000 || a.CooldownMS != 20000 || a.FirstPulseMS < 1 || a.PulseMS < 1 || a.PropagationMS < 1 || a.PropagationMS > a.PulseMS || a.FirstPulseMS+a.PropagationMS > a.DurationMS || a.Width < 1 || a.Width > a.Radius || a.Radius < 1 || a.DamagePct <= 0 || a.Shape != "ring" {
				return fmt.Errorf("invalid lightning")
			}
		case "agility":
			if a.MoveSpeedPct != 100 || a.DurationMS != 0 {
				return fmt.Errorf("invalid agility")
			}
		case "poison_pool":
			if a.DurationMS != 20000 || a.CooldownMS != 30000 || a.PulseMS != 1000 || a.DamagePct <= 0 || a.StepDamagePct < 1 || a.MaxDamagePct < a.DamagePct || a.MaxDamagePct > 10000 || a.Radius <= 0 || a.Shape != "circle" {
				return fmt.Errorf("invalid poison pool")
			}
		case "walls":
			if a.Count != 3 || a.DurationMS != 15000 || a.CooldownMS != 30000 || a.Length < 1 || a.Length > 1000 || a.Width < 1 || a.Width > 200 || a.Gap < 80 || a.Gap > 1000 || a.Shape != "line" {
				return fmt.Errorf("invalid walls")
			}
		}
	}
	seen := map[MonsterID]bool{}
	allowed := map[MonsterID]bool{1011: true, 1092: true, 1060: true, 1073: true, 1085: true, 1136: true, 3067: true, 3044: true}
	for _, g := range r.Elite.Guardians {
		if !allowed[g.MonsterID] || seen[g.MonsterID] || len(g.Abilities) != 3 {
			return fmt.Errorf("invalid guardian catalog")
		}
		seen[g.MonsterID] = true
		chosen := map[int32]bool{}
		for _, id := range g.Abilities {
			if id < 0 || int(id) >= len(r.Abilities) || chosen[id] || r.Abilities[id].Trigger == "death" {
				return fmt.Errorf("invalid guardian affixes")
			}
			chosen[id] = true
		}
	}
	return nil
}
