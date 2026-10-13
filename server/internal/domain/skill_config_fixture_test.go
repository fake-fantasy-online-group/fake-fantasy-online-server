package domain

func testSkillGrowthPolicies() map[SkillID]SkillGrowthPolicy {
	out := map[SkillID]SkillGrowthPolicy{}
	c := testSkillProgressionCatalog()
	for _, id := range c.IDs() {
		p, _ := c.Policy(id)
		out[id] = p
	}
	return out
}
