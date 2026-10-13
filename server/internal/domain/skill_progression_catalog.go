package domain

import (
	"fmt"
	"sort"
)

// SkillProgressionCatalog is an immutable, explicitly injected snapshot of the
// persisted class membership, growth policies and synergy edges. Raw ranks 1..5
// continue to live in SkillTable; no skill output is installed in a global map.
type SkillProgressionCatalog struct {
	identities map[SkillID]SkillIdentity
	policies   map[SkillID]SkillGrowthPolicy
	synergies  []SkillSynergy
}

type SkillProgressionData struct {
	ExpectedSkills, ExpectedSynergies int
	Identities                        []SkillIdentity
	Policies                          map[SkillID]SkillGrowthPolicy
	Synergies                         []SkillSynergy
}

func NewSkillProgressionCatalog(in SkillProgressionData) (*SkillProgressionCatalog, error) {
	c := &SkillProgressionCatalog{identities: map[SkillID]SkillIdentity{}, policies: map[SkillID]SkillGrowthPolicy{}}
	if in.ExpectedSkills <= 0 || in.ExpectedSynergies <= 0 || len(in.Identities) != in.ExpectedSkills || len(in.Synergies) != in.ExpectedSynergies || len(in.Policies) != len(in.Identities) {
		return nil, fmt.Errorf("skill progression identity/policy coverage missing")
	}
	for _, d := range in.Identities {
		if d.ID <= 0 || d.Name == "" || d.Profession < 1 || d.Profession > 5 || d.UnlockLevel <= 0 || d.UnlockLevel > LevelCap || (d.Branch != "left" && d.Branch != "right" && d.Branch != "aux") {
			return nil, fmt.Errorf("invalid skill progression identity %d", d.ID)
		}
		if _, exists := c.identities[d.ID]; exists {
			return nil, fmt.Errorf("duplicate skill identity %d", d.ID)
		}
		d.Prerequisites = append([]SkillID(nil), d.Prerequisites...)
		c.identities[d.ID] = d
	}
	for id, d := range c.identities {
		seen := map[SkillID]bool{}
		for _, p := range d.Prerequisites {
			other, exists := c.identities[p]
			if !exists || other.Profession != d.Profession || p == id || seen[p] {
				return nil, fmt.Errorf("invalid skill prerequisite %d -> %d", id, p)
			}
			seen[p] = true
		}
		p, exists := in.Policies[id]
		if !exists {
			return nil, fmt.Errorf("missing skill growth policy %d", id)
		}
		if p.AttributeCapRank < 0 || p.EfficiencyCapRank < 0 || p.RangeCapRank < 0 || p.ProtectionCapRank < 0 || p.AttackCapRank < 0 || (p.Attribute == 0) != (p.AttributeCapRank == 0) || (p.AttackCapRank == 0) != (len(p.AttackTargets) == 0) {
			return nil, fmt.Errorf("invalid skill growth cap %d", id)
		}
		for _, cap := range []int32{p.EfficiencyCapRank, p.RangeCapRank, p.ProtectionCapRank, p.AttackCapRank} {
			if cap != 0 && cap < OriginalSkillRankCap {
				return nil, fmt.Errorf("skill %d growth cap precedes original rank range", id)
			}
		}
		switch p.Attribute {
		case 0, AttrSTR, AttrINT, AttrVIT, AttrAGI, AttrDEX, AttrSPI:
		default:
			return nil, fmt.Errorf("invalid skill growth attribute %d/%d", id, p.Attribute)
		}
		targets := map[SkillID]bool{}
		for _, target := range p.AttackTargets {
			other, exists := c.identities[target]
			if !exists || other.Profession != d.Profession || target == id || targets[target] {
				return nil, fmt.Errorf("invalid skill growth target %d -> %d", id, target)
			}
			targets[target] = true
		}
		p.AttackTargets = append([]SkillID(nil), p.AttackTargets...)
		c.policies[id] = p
	}
	visiting, done := map[SkillID]bool{}, map[SkillID]bool{}
	var visit func(SkillID) error
	visit = func(id SkillID) error {
		if visiting[id] {
			return fmt.Errorf("cyclic skill prerequisite at %d", id)
		}
		if done[id] {
			return nil
		}
		visiting[id] = true
		for _, p := range c.identities[id].Prerequisites {
			if err := visit(p); err != nil {
				return err
			}
		}
		delete(visiting, id)
		done[id] = true
		return nil
	}
	for id := range c.identities {
		if err := visit(id); err != nil {
			return nil, err
		}
	}
	if err := ValidateSkillSynergies(in.Synergies, c); err != nil {
		return nil, err
	}
	c.synergies = append([]SkillSynergy(nil), in.Synergies...)
	return c, nil
}
func (c *SkillProgressionCatalog) Identity(id SkillID) (SkillIdentity, bool) {
	if c == nil {
		return SkillIdentity{}, false
	}
	d, ok := c.identities[id]
	d.Prerequisites = append([]SkillID(nil), d.Prerequisites...)
	return d, ok
}
func (c *SkillProgressionCatalog) IDs() []SkillID {
	if c == nil {
		return nil
	}
	out := make([]SkillID, 0, len(c.identities))
	for id := range c.identities {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
func (c *SkillProgressionCatalog) Policy(id SkillID) (SkillGrowthPolicy, bool) {
	if c == nil {
		return SkillGrowthPolicy{}, false
	}
	p, ok := c.policies[id]
	p.AttackTargets = append([]SkillID(nil), p.AttackTargets...)
	return p, ok
}
func (c *SkillProgressionCatalog) Synergies() []SkillSynergy {
	if c == nil {
		return nil
	}
	return append([]SkillSynergy(nil), c.synergies...)
}

// ProgressionCatalog returns the single snapshot attached by the loader. An
// inconsistent/missing table has no catalogue and cannot pass startup checks.
func (t SkillTable) ProgressionCatalog() *SkillProgressionCatalog {
	var out *SkillProgressionCatalog
	for _, d := range t {
		if d.Progression == nil {
			continue
		}
		if out != nil && out != d.Progression {
			return nil
		}
		out = d.Progression
	}
	return out
}
func AttachSkillProgression(t SkillTable, c *SkillProgressionCatalog) {
	for key, d := range t {
		d.Progression = c
		d.Statuses = append([]StatusApplication(nil), d.Statuses...)
		for i := range d.Statuses {
			d.Statuses[i].Progression = c
		}
		t[key] = d
	}
}
