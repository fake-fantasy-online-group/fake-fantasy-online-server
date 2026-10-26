package domain

import (
	"fmt"
	"sort"
)

type LootSocketLevel struct {
	MinLevel int32
	Cap      int32
}
type LootGradeRule struct {
	Grade         int32
	RuneRateBP    int32
	SocketWeights [6]int64
}

// OutdoorEquipmentRule is an optional, versioned override for natural outdoor
// equipment. Boss/elite and normal sources have separate instances. Weights are
// conditional on not being
// promoted: 20/25/20/15/10/5 (total 95) plus 5% promotion gives the requested
// unconditional 20/25/20/15/10/5/5 baseline on fully eligible dyeable bases.
type OutdoorEquipmentRule struct {
	AffixWeights     [6]int64
	LegendaryBasePPB int64
}

func (r OutdoorEquipmentRule) Counts() []WeightedAffixCount {
	counts := make([]WeightedAffixCount, len(r.AffixWeights))
	for n, weight := range r.AffixWeights {
		counts[n] = WeightedAffixCount{Count: uint8(n), Weight: weight}
	}
	return counts
}

type LootRuleSet struct {
	Version int32
	// Zero preserves historical mixed-pool RNG. One selects class/universal 50:50.
	LegendaryPowerPolicy           int32 `json:",omitempty"`
	MinRuneLevel                   int32
	UniqueBasePPB                  int64
	UniqueMFBoost, UniqueMFDivisor int64
	RunePool                       []ItemID
	// Omitted in legacy persisted rift snapshots: every rune then has weight 1.
	// Explicit weights are aligned with RunePool and affect selection only.
	RuneWeights  []int64 `json:",omitempty"`
	SocketLevels []LootSocketLevel
	Grades       []LootGradeRule
	// Nil in historical snapshots; never changes their shared color profiles.
	OutdoorEquipment       *OutdoorEquipmentRule `json:",omitempty"`
	OutdoorNormalEquipment *OutdoorEquipmentRule `json:",omitempty"`
}
type LootRuleTable struct {
	active   int32
	versions map[int32]LootRuleSet
}

func cloneLootRuleSet(r LootRuleSet) LootRuleSet {
	if r.OutdoorEquipment != nil {
		outdoor := *r.OutdoorEquipment
		r.OutdoorEquipment = &outdoor
	}
	if r.OutdoorNormalEquipment != nil {
		outdoor := *r.OutdoorNormalEquipment
		r.OutdoorNormalEquipment = &outdoor
	}
	r.RunePool = append([]ItemID(nil), r.RunePool...)
	r.RuneWeights = append([]int64(nil), r.RuneWeights...)
	r.SocketLevels = append([]LootSocketLevel(nil), r.SocketLevels...)
	r.Grades = append([]LootGradeRule(nil), r.Grades...)
	return r
}
func NewLootRuleTable(active int32, rows []LootRuleSet, items map[ItemID]ItemDef) (*LootRuleTable, error) {
	t := &LootRuleTable{active: active, versions: map[int32]LootRuleSet{}}
	for _, r := range rows {
		r = cloneLootRuleSet(r)
		if _, ok := t.versions[r.Version]; ok {
			return nil, fmt.Errorf("duplicate loot version %d", r.Version)
		}
		if r.LegendaryPowerPolicy < 0 || r.LegendaryPowerPolicy > 1 || (r.Version < 4 && r.LegendaryPowerPolicy != 0) {
			return nil, fmt.Errorf("invalid legendary power policy for loot version %d", r.Version)
		}
		if r.Version < 1 || r.MinRuneLevel < 1 || r.MinRuneLevel > 1000000 || r.UniqueBasePPB < 0 || r.UniqueBasePPB > LootProbabilityScaleV5 || r.UniqueMFBoost < 0 || r.UniqueMFBoost > 100 || r.UniqueMFDivisor < 1 || r.UniqueMFDivisor > 1000000 || r.UniqueBasePPB*(1+r.UniqueMFBoost) > LootProbabilityScaleV5 {
			return nil, fmt.Errorf("invalid loot rules version %d", r.Version)
		}
		for _, outdoor := range []*OutdoorEquipmentRule{r.OutdoorEquipment, r.OutdoorNormalEquipment} {
			if outdoor == nil {
				continue
			}
			if _, err := NaturalAffixProbabilitiesV5(outdoor.Counts(), 0); err != nil {
				return nil, fmt.Errorf("invalid outdoor equipment weights version %d: %w", r.Version, err)
			}
			if outdoor.LegendaryBasePPB < 0 || outdoor.LegendaryBasePPB > LootProbabilityScaleV5 || outdoor.LegendaryBasePPB*(1+r.UniqueMFBoost) > LootProbabilityScaleV5 {
				return nil, fmt.Errorf("invalid outdoor legendary probability version %d", r.Version)
			}
		}
		if len(r.RunePool) != 28 || len(r.Grades) != 4 || len(r.SocketLevels) == 0 {
			return nil, fmt.Errorf("incomplete loot rules version %d", r.Version)
		}
		if len(r.RuneWeights) != 0 && len(r.RuneWeights) != len(r.RunePool) {
			return nil, fmt.Errorf("incomplete loot rune weights version %d", r.Version)
		}
		for _, weight := range r.RuneWeights {
			if weight <= 0 || weight > 1000000 {
				return nil, fmt.Errorf("invalid loot rune weight version %d", r.Version)
			}
		}
		seen := map[ItemID]bool{}
		for _, id := range r.RunePool {
			d, ok := items[id]
			if !ok || seen[id] || id < FirstAstralRune || id > LastAstralRune || d.InstanceKind != ItemInstanceSocketCard || !d.CardDefaults.Initialized {
				return nil, fmt.Errorf("invalid loot rune %d", id)
			}
			seen[id] = true
		}
		sort.Slice(r.Grades, func(i, j int) bool { return r.Grades[i].Grade < r.Grades[j].Grade })
		for i, g := range r.Grades {
			if g.Grade != int32(i) || g.RuneRateBP < 0 || g.RuneRateBP > 10000 {
				return nil, fmt.Errorf("invalid loot grade")
			}
			sum := int64(0)
			for _, w := range g.SocketWeights {
				if w < 0 || w > 100 {
					return nil, fmt.Errorf("invalid socket weight")
				}
				sum += w
			}
			if sum != 100 {
				return nil, fmt.Errorf("socket weights must total 100")
			}
		}
		sort.Slice(r.SocketLevels, func(i, j int) bool { return r.SocketLevels[i].MinLevel < r.SocketLevels[j].MinLevel })
		for i, l := range r.SocketLevels {
			if l.MinLevel < 1 || l.Cap < 0 || l.Cap > 5 || i > 0 && (l.MinLevel == r.SocketLevels[i-1].MinLevel || l.Cap < r.SocketLevels[i-1].Cap) {
				return nil, fmt.Errorf("invalid socket level tier")
			}
		}
		t.versions[r.Version] = cloneLootRuleSet(r)
	}
	if _, ok := t.versions[1]; !ok {
		return nil, fmt.Errorf("missing legacy loot version 1")
	}
	if _, ok := t.versions[active]; !ok {
		return nil, fmt.Errorf("missing active loot version %d", active)
	}
	return t, nil
}
func (t *LootRuleTable) Version(v int32) (*LootRuleSet, bool) {
	if t == nil {
		return nil, false
	}
	r, ok := t.versions[v]
	if !ok {
		return nil, false
	}
	r = cloneLootRuleSet(r)
	return &r, true
}
func (t *LootRuleTable) Active() *LootRuleSet {
	if t == nil {
		return nil
	}
	r, _ := t.Version(t.active)
	return r
}
func (r *LootRuleSet) RuneDrop(grade AstralDropGrade) DropEntry {
	if r == nil || int(grade) >= len(r.Grades) {
		return DropEntry{}
	}
	return DropEntry{RatePct: float64(r.Grades[grade].RuneRateBP) / 100, Choices: append([]ItemID(nil), r.RunePool...)}
}

// RollRune chooses the rune after a successful occurrence roll (or for a
// guaranteed rift reward). Legacy snapshots retain their exact Intn sequence.
func (r *LootRuleSet) RollRune(intn func(int) int) (ItemID, bool) {
	if r == nil || len(r.RunePool) == 0 || intn == nil {
		return 0, false
	}
	if len(r.RuneWeights) == 0 {
		roll := intn(len(r.RunePool))
		if roll < 0 || roll >= len(r.RunePool) {
			return 0, false
		}
		return r.RunePool[roll], true
	}
	if len(r.RuneWeights) != len(r.RunePool) {
		return 0, false
	}
	total := int64(0)
	for _, weight := range r.RuneWeights {
		if weight <= 0 || weight > 1000000 {
			return 0, false
		}
		total += weight
	}
	roll := int64(intn(int(total)))
	if roll < 0 || roll >= total {
		return 0, false
	}
	for i, weight := range r.RuneWeights {
		if roll < weight {
			return r.RunePool[i], true
		}
		roll -= weight
	}
	return 0, false
}
func (r *LootRuleSet) SocketCapacity(level int32) int {
	if r == nil {
		return 0
	}
	cap := 0
	for _, l := range r.SocketLevels {
		if level < l.MinLevel {
			break
		}
		cap = int(l.Cap)
	}
	return cap
}
