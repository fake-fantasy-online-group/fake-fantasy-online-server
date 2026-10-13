package domain

import (
	"fmt"
	"math/big"
	"sort"
)

const SkillFormulaVersion uint32 = 5
const SkillHardPointCap int32 = 20

// SkillIdentity is frozen explicit membership, never inferred from ID or damage element.
type SkillIdentity struct {
	ID            SkillID
	Name          string
	Profession    int32
	Branch        string
	UnlockLevel   int32
	Prerequisites []SkillID
}

func V5SkillIdentity(id SkillID, c *SkillProgressionCatalog) (SkillIdentity, bool) {
	return c.Identity(id)
}
func V5SkillIDs(c *SkillProgressionCatalog) []SkillID { return c.IDs() }

type SkillRankBonus struct {
	SourceID   string  `json:"sourceId,omitempty"`
	Scope      string  `json:"scope"`
	Profession int32   `json:"profession,omitempty"`
	Branch     string  `json:"branch,omitempty"`
	School     string  `json:"school,omitempty"`
	SkillID    SkillID `json:"skillId,omitempty"`
	Levels     int64   `json:"levels"`
}
type SkillGrant struct {
	SourceID               string
	SkillID                SkillID
	Rank                   int64
	Origin                 string
	BenefitsFromSkillRanks bool
}

// SkillTrainingSynergyView is a planning row even when its source is unlearned.
// Percentage values use basis points; flat values use whole resource points.
// These are contribution coefficients, never recursively evaluated skill output.
type SkillTrainingSynergyView struct {
	SourceID           SkillID `json:"sourceId"`
	SourceName         string  `json:"sourceName"`
	Effect             string  `json:"effect"`
	EffectLabel        string  `json:"effectLabel"`
	PerPointValue      int32   `json:"perPointValue"`
	Unit               string  `json:"unit"`
	InvestedHardPoints int32   `json:"investedHardPoints"`
	ContributionValue  int32   `json:"contributionValue"`
	EquipmentApplies   bool    `json:"equipmentApplies"`
}

type SkillRankView struct {
	ContractUnavailable bool                       `json:"-"`
	Contract            *SkillBehaviorContract     `json:"contract,omitempty"`
	NextChangeLines     []string                   `json:"nextChangeLines,omitempty"`
	NextRequiredLevel   int32                      `json:"nextRequiredLevel,omitempty"`
	NextEffectiveRank   string                     `json:"nextEffectiveRank,omitempty"`
	TrainingSynergies   []SkillTrainingSynergyView `json:"trainingSynergies"`
	NativeModifierLines []string                   `json:"nativeModifierLines"`
	CurrentEffectLines  []string                   `json:"currentEffectLines"`
	NextEffectLines     []string                   `json:"nextEffectLines"`
	CanInvest           bool                       `json:"canInvest"`
	BlockedReasons      []string                   `json:"blockedReasons"`
	ID                  SkillID                    `json:"id"`
	HardRank            int32                      `json:"hardRank"`
	GrantRank           string                     `json:"grantRank"`
	SoftRank            string                     `json:"softRank"`
	EffectiveRank       string                     `json:"effectiveRank"`
	CanCast             bool                       `json:"canCast"`
	Contributions       []SkillRankBonus           `json:"contributions,omitempty"`
}
type SkillResolutionSnapshot map[SkillID]SkillRankView

func ValidateSkillRankBonuses(bonuses []SkillRankBonus, c *SkillProgressionCatalog) error {
	if c == nil {
		return fmt.Errorf("skill progression catalog missing")
	}
	for _, b := range bonuses {
		if b.Levels <= 0 {
			return fmt.Errorf("skill bonus must be positive: %+v", b)
		}
		switch b.Scope {
		case "all":
			if b.Profession != 0 || b.Branch != "" || b.School != "" || b.SkillID != 0 {
				return fmt.Errorf("all skill bonus has target")
			}
		case "class":
			if b.Profession < 1 || b.Profession > 5 || b.Branch != "" || b.School != "" || b.SkillID != 0 {
				return fmt.Errorf("invalid class skill bonus")
			}
		case "branch":
			if b.Profession < 1 || b.Profession > 5 || (b.Branch != "left" && b.Branch != "right" && b.Branch != "aux") || b.School != "" || b.SkillID != 0 {
				return fmt.Errorf("invalid branch skill bonus")
			}
		case "school":
			if b.Profession != 0 || b.Branch != "" || b.SkillID != 0 || (b.School != "flame" && b.School != "frost" && b.School != "healing" && b.School != "trap") {
				return fmt.Errorf("invalid school skill bonus")
			}
		case "single":
			if _, ok := c.Identity(b.SkillID); !ok || b.Profession != 0 || b.Branch != "" || b.School != "" {
				return fmt.Errorf("invalid single skill bonus %d", b.SkillID)
			}
		default:
			return fmt.Errorf("unknown skill scope %q", b.Scope)
		}
	}
	return nil
}
func (b SkillRankBonus) Matches(d SkillIdentity, profession int32) bool {
	if d.Profession != profession {
		return false
	}
	switch b.Scope {
	case "all":
		return true
	case "class":
		return b.Profession == profession
	case "branch":
		return b.Profession == profession && b.Branch == d.Branch
	case "single":
		return b.SkillID == d.ID
	case "school":
		switch b.School {
		case "flame":
			return d.Profession == 5 && d.Branch == "left"
		case "frost":
			return d.Profession == 5 && d.Branch == "right"
		case "healing":
			return d.Profession == 4 && d.Branch == "left"
		case "trap":
			return d.ID >= 10213 && d.ID <= 10217
		}
	}
	return false
}

// ResolveSkillLoadout stores integral decimal ranks without a gameplay ceiling.
// Narrow engine representations are checked explicitly by EngineRank, never capped.
func ResolveSkillLoadout(profession int32, hard Learned, bonuses []SkillRankBonus, grants []SkillGrant, c *SkillProgressionCatalog) (SkillResolutionSnapshot, error) {
	if profession < 1 || profession > 5 {
		return nil, fmt.Errorf("invalid profession %d", profession)
	}
	if err := ValidateSkillRankBonuses(bonuses, c); err != nil {
		return nil, err
	}
	unique := make([]SkillRankBonus, 0, len(bonuses))
	seen := map[string]SkillRankBonus{}
	for _, b := range bonuses {
		if b.SourceID == "" {
			return nil, fmt.Errorf("skill bonus lacks source identity")
		}
		if old, ok := seen[b.SourceID]; ok {
			if old != b {
				return nil, fmt.Errorf("conflicting skill source %q", b.SourceID)
			}
			continue
		}
		seen[b.SourceID] = b
		unique = append(unique, b)
	}
	sort.Slice(unique, func(i, j int) bool { return unique[i].SourceID < unique[j].SourceID })
	gs := map[SkillID]*big.Int{}
	softAllowed := map[SkillID]bool{}
	grantSources := map[string]SkillGrant{}
	for _, g := range grants {
		if g.Rank <= 0 || g.SourceID == "" || g.Origin == "" {
			return nil, fmt.Errorf("invalid skill grant")
		}
		if old, ok := grantSources[g.SourceID]; ok {
			if old != g {
				return nil, fmt.Errorf("conflicting grant source")
			}
			continue
		}
		grantSources[g.SourceID] = g
		// Multiple grants offer their strongest base, not additive permanent ranks.
		if gs[g.SkillID] == nil || gs[g.SkillID].Cmp(big.NewInt(g.Rank)) < 0 {
			gs[g.SkillID] = big.NewInt(g.Rank)
		}
		softAllowed[g.SkillID] = softAllowed[g.SkillID] || g.BenefitsFromSkillRanks
	}
	out := SkillResolutionSnapshot{}
	for _, id := range V5SkillIDs(c) {
		d, _ := c.Identity(id)
		if d.Profession != profession {
			if hard[id] > 0 {
				return nil, fmt.Errorf("cross-profession hard rank %d", id)
			}
			continue
		}
		h := hard[id]
		if h < 0 || h > SkillHardPointCap {
			return nil, fmt.Errorf("invalid hard rank %d/%d", id, h)
		}
		grant := new(big.Int)
		if gs[id] != nil {
			grant.Set(gs[id])
		}
		soft := new(big.Int)
		var sources []SkillRankBonus
		for _, b := range unique {
			if b.Matches(d, profession) && (h > 0 || grant.Sign() == 0 || softAllowed[id]) {
				soft.Add(soft, big.NewInt(b.Levels))
				sources = append(sources, b)
			}
		}
		can := h > 0 || grant.Sign() > 0
		effective := new(big.Int)
		if can {
			effective.SetInt64(int64(h))
			effective.Add(effective, grant)
			effective.Add(effective, soft)
		}
		out[id] = SkillRankView{ID: id, HardRank: h, GrantRank: grant.String(), SoftRank: soft.String(), EffectiveRank: effective.String(), CanCast: can, Contributions: sources}
	}
	for id, g := range gs {
		if _, ok := out[id]; !ok {
			out[id] = SkillRankView{ID: id, GrantRank: g.String(), SoftRank: "0", EffectiveRank: g.String(), CanCast: true}
		}
	}
	return out, nil
}
func (r SkillRankView) EngineRank() (int32, error) {
	n, ok := new(big.Int).SetString(r.EffectiveRank, 10)
	if !ok || n.Sign() < 0 || !n.IsInt64() || n.Int64() > 2147483647 {
		return 0, fmt.Errorf("skill %d rank exceeds engine numeric representation: %s", r.ID, r.EffectiveRank)
	}
	return int32(n.Int64()), nil
}
func SkillPointBudget(level int32) int32 {
	if level <= 1 {
		return 0
	}
	if level > 100 {
		level = 100
	}
	return level - 1
}
func SkillHardRankLimit(level int32, id SkillID, c *SkillProgressionCatalog) int32 {
	d, ok := c.Identity(id)
	if !ok || level < d.UnlockLevel {
		return 0
	}
	return min(SkillHardPointCap, level-d.UnlockLevel+1)
}
func ValidateSkillAllocation(level, profession int32, hard Learned, unspent int32, c *SkillProgressionCatalog) error {
	if c == nil {
		return fmt.Errorf("skill progression catalog missing")
	}
	var spent int64
	for id, h := range hard {
		d, ok := c.Identity(id)
		if !ok {
			continue
		}
		if h < 0 || h > SkillHardRankLimit(level, id, c) || d.Profession != profession {
			return fmt.Errorf("invalid allocation %d/%d", id, h)
		}
		if h > 0 {
			for _, dep := range d.Prerequisites {
				if hard[dep] < 1 {
					return fmt.Errorf("hard prerequisite missing: %d requires %d", id, dep)
				}
			}
		}
		spent += int64(h)
	}
	// Starter points remain part of the budget until employment refunds/removes them.
	for _, id := range []SkillID{SkillBeginnerBolt, SkillBeginnerGuard} {
		if hard[id] > 0 {
			spent += int64(hard[id])
		}
	}
	if unspent < 0 || spent+int64(unspent) != int64(SkillPointBudget(level)) {
		return fmt.Errorf("skill budget mismatch: spent=%d unspent=%d earned=%d", spent, unspent, SkillPointBudget(level))
	}
	return nil
}

type SkillSynergy struct {
	Target, Source   SkillID
	RankMode, Effect string
	PerPointBP       int32
}

func ValidateSkillSynergies(edges []SkillSynergy, c *SkillProgressionCatalog) error {
	if c == nil {
		return fmt.Errorf("skill progression catalog missing")
	}
	sources := map[SkillID]map[SkillID]bool{}
	exact := map[SkillSynergy]bool{}
	for _, e := range edges {
		t, tok := c.Identity(e.Target)
		s, sok := c.Identity(e.Source)
		if !tok || !sok || t.Profession != s.Profession || e.Target == e.Source {
			return fmt.Errorf("invalid synergy %d -> %d", e.Source, e.Target)
		}
		if e.RankMode != "hard" && e.RankMode != "effective_raw" {
			return fmt.Errorf("invalid synergy rank mode")
		}
		if e.RankMode == "hard" {
			switch e.Effect {
			case "damage", "elite_damage", "heal_power", "heal_flat", "mana_economy", "radius", "width", "freeze_duration":
			default:
				return fmt.Errorf("unsupported training effect %q", e.Effect)
			}
			if e.PerPointBP <= 0 || e.PerPointBP > 10000 || (e.Effect == "heal_flat" && e.PerPointBP%100 != 0) {
				return fmt.Errorf("invalid training coefficient: %+v", e)
			}
		} else {
			switch e.Effect {
			case "skill_damage_pct", "slow", "knockback", "preserve_stealth", "durability_economy", "vulnerability", "damage_mp_conversion", "stun", "restore_hp_conversion", "restore_mp_conversion", "stealth_duration_speed_damage", "burn", "freeze", "magic_defense_reduction", "physical_vulnerability":
			default:
				return fmt.Errorf("unsupported native synergy effect %q", e.Effect)
			}
			if e.PerPointBP != 0 {
				return fmt.Errorf("native synergy must use original skill payload: %+v", e)
			}
		}
		key := e
		key.PerPointBP = 0
		if exact[key] {
			return fmt.Errorf("duplicate synergy edge")
		}
		exact[key] = true
		if sources[e.Target] == nil {
			sources[e.Target] = map[SkillID]bool{}
		}
		sources[e.Target][e.Source] = true
		if len(sources[e.Target]) > 3 {
			return fmt.Errorf("skill %d has more than three synergy sources", e.Target)
		}
	}
	return nil
}

// InstallV5SkillProgression verifies the entire class catalogue before activation.
// The original five rows stay immutable anchors; Get computes the requested rank.
func InstallV5SkillProgression(t SkillTable, c *SkillProgressionCatalog) error {
	if c == nil {
		return fmt.Errorf("skill progression catalog missing")
	}
	if err := ValidateSkillSynergies(c.Synergies(), c); err != nil {
		return err
	}
	for _, id := range V5SkillIDs(c) {
		for rank := int32(1); rank <= OriginalSkillRankCap; rank++ {
			if _, ok := t[SkillKey{ID: id, Level: rank}]; !ok {
				return fmt.Errorf("v5 missing skill anchor %d/%d", id, rank)
			}
		}
	}
	for _, id := range V5SkillIDs(c) {
		for _, rank := range []int32{1, 5, 6, 20, 21, 65536} {
			first, fifth := t[SkillKey{ID: id, Level: 1}], t[SkillKey{ID: id, Level: 5}]
			first.Progression, fifth.Progression = c, c
			if _, err := EvaluateV5Skill(first, fifth, rank); err != nil {
				return fmt.Errorf("v5 anchor preflight: %w", err)
			}
		}
	}
	AttachSkillProgression(t, c)
	for _, id := range V5SkillIDs(c) {
		k := SkillKey{ID: id, Level: 1}
		d := t[k]
		d.FormulaVersion = SkillFormulaVersion
		t[k] = d
	}
	return nil
}
func (t SkillTable) V5Enabled(id SkillID) bool {
	d, ok := t[SkillKey{ID: id, Level: 1}]
	return ok && d.FormulaVersion == SkillFormulaVersion
}
