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

func V5SkillIdentity(id SkillID) (SkillIdentity, bool) {
	d, ok := v5SkillIdentities[id]
	d.Prerequisites = append([]SkillID(nil), d.Prerequisites...)
	return d, ok
}
func V5SkillIDs() []SkillID {
	ids := make([]SkillID, 0, len(v5SkillIdentities))
	for id := range v5SkillIdentities {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

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
type SkillRankView struct {
	CurrentEffectLines []string         `json:"currentEffectLines"`
	NextEffectLines    []string         `json:"nextEffectLines"`
	CanInvest          bool             `json:"canInvest"`
	BlockedReasons     []string         `json:"blockedReasons"`
	ID                 SkillID          `json:"id"`
	HardRank           int32            `json:"hardRank"`
	GrantRank          string           `json:"grantRank"`
	SoftRank           string           `json:"softRank"`
	EffectiveRank      string           `json:"effectiveRank"`
	CanCast            bool             `json:"canCast"`
	Contributions      []SkillRankBonus `json:"contributions,omitempty"`
}
type SkillResolutionSnapshot map[SkillID]SkillRankView

func ValidateSkillRankBonuses(bonuses []SkillRankBonus) error {
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
			if _, ok := v5SkillIdentities[b.SkillID]; !ok || b.Profession != 0 || b.Branch != "" || b.School != "" {
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
func ResolveSkillLoadout(profession int32, hard Learned, bonuses []SkillRankBonus, grants []SkillGrant) (SkillResolutionSnapshot, error) {
	if profession < 1 || profession > 5 {
		return nil, fmt.Errorf("invalid profession %d", profession)
	}
	if err := ValidateSkillRankBonuses(bonuses); err != nil {
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
	for _, id := range V5SkillIDs() {
		d := v5SkillIdentities[id]
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
func SkillHardRankLimit(level int32, id SkillID) int32 {
	d, ok := v5SkillIdentities[id]
	if !ok || level < d.UnlockLevel {
		return 0
	}
	return min(SkillHardPointCap, level-d.UnlockLevel+1)
}
func ValidateSkillAllocation(level, profession int32, hard Learned, unspent int32) error {
	var spent int64
	for id, h := range hard {
		d, ok := v5SkillIdentities[id]
		if !ok {
			continue
		}
		if h < 0 || h > SkillHardRankLimit(level, id) || d.Profession != profession {
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

func ValidateSkillSynergies(edges []SkillSynergy) error {
	sources := map[SkillID]map[SkillID]bool{}
	exact := map[SkillSynergy]bool{}
	for _, e := range edges {
		t, tok := v5SkillIdentities[e.Target]
		s, sok := v5SkillIdentities[e.Source]
		if !tok || !sok || t.Profession != s.Profession || e.Target == e.Source {
			return fmt.Errorf("invalid synergy %d -> %d", e.Source, e.Target)
		}
		if e.RankMode != "hard" && e.RankMode != "effective_raw" {
			return fmt.Errorf("invalid synergy rank mode")
		}
		if exact[e] {
			return fmt.Errorf("duplicate synergy edge")
		}
		exact[e] = true
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
func InstallV5SkillProgression(t SkillTable) error {
	if err := ValidateSkillSynergies(V5SkillSynergies); err != nil {
		return err
	}
	for _, id := range V5SkillIDs() {
		for _, rank := range []int32{1, 5} {
			if _, ok := t[SkillKey{ID: id, Level: rank}]; !ok {
				return fmt.Errorf("v5 missing skill anchor %d/%d", id, rank)
			}
		}
	}
	for _, id := range V5SkillIDs() {
		for _, rank := range []int32{1, 20, 21, 65536} {
			if _, err := EvaluateV5Skill(t[SkillKey{ID: id, Level: 1}], t[SkillKey{ID: id, Level: 5}], rank); err != nil {
				return fmt.Errorf("v5 anchor preflight: %w", err)
			}
		}
	}
	for _, id := range V5SkillIDs() {
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
