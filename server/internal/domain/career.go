package domain

// CareerQuestDef 是五个基础职业就职任务的权威映射。
// 任务发布者使用客户端任务步骤与地图 NPC 的完整名字，不能用 game_tasks 中
// 被截短的名字，否则玩家站在真实 NPC 前也无法接取或交付。
type CareerQuestDef struct {
	Profession Race
	NPC        string
}

var careerQuests = map[QuestID]CareerQuestDef{
	400: {Profession: Healer, NPC: "铁拐门人蓟子训"},
	401: {Profession: Warlock, NPC: "果老门人寒山子"},
	402: {Profession: Assassin, NPC: "仙姑门人萼绿华"},
	403: {Profession: Warrior, NPC: "钟离门人裴谌"},
	404: {Profession: Swordsman, NPC: "吕仙门人许栖岩"},
}

var careerQuestByRoute = [...]QuestID{
	Warrior:   403,
	Swordsman: 404,
	Assassin:  402,
	Healer:    400,
	Warlock:   401,
}

// CareerOfQuest 返回完成指定就职任务后获得的职业。
func CareerOfQuest(id QuestID) (Race, bool) {
	d, ok := careerQuests[id]
	return d.Profession, ok
}

// CareerQuestNPC 返回就职任务发布者在地图中的完整名字。
func CareerQuestNPC(id QuestID) (string, bool) {
	d, ok := careerQuests[id]
	return d.NPC, ok
}

// IsCareerQuest 报告任务是否为五个基础职业就职任务之一。
func IsCareerQuest(id QuestID) bool {
	_, ok := careerQuests[id]
	return ok
}

// HasProfession 报告角色是否已从初行者就职。
func (c *Character) HasProfession() bool {
	return c != nil && (!c.EmploymentKnown || c.Employed)
}

// ClientRace 返回客户端角色外观协议使用的建角路线编号 0..4。
//
// 是否已经就职不能塞进 race：它同时驱动角色外观，并且会让未就职角色丢失
// 自己的路线。客户端技能面板通过对应就职任务是否完成来切换初心者/职业页。
func (c *Character) ClientRace() uint8 {
	if c == nil || !c.Race.Valid() {
		return 0
	}
	return uint8(c.Race)
}

// AllowsCareerQuest 报告角色能否接取或继续指定就职任务。
//
// 建角时已经选定未来职业路线：十级时只能接这条路线对应的一个就职任务。
// 完成后永久关闭全部基础职业就职任务。
func (c *Character) AllowsCareerQuest(id QuestID) bool {
	if !IsCareerQuest(id) {
		return true
	}
	if c == nil || c.HasProfession() {
		return false
	}
	if !c.Race.Valid() {
		return false
	}
	return careerQuestByRoute[c.Race] == id
}

// CompleteCareerQuest 完成一个正在进行的就职任务：切换职业、删除初行者技能，
// 并把就职前花掉的技能点和属性点全部退回。
// 调用方必须在把任务标记为完成之前调用，避免没有接任务也能直接切职业。
func (c *Character) CompleteCareerQuest(id QuestID) bool {
	profession, ok := CareerOfQuest(id)
	if !ok || c == nil || !c.AllowsCareerQuest(id) || !c.Quests.Active(id) {
		return false
	}
	if profession != c.Race {
		return false
	}
	c.Employed = true
	c.EmploymentKnown = true
	c.RemoveBeginnerSkills()
	c.resetCareerPoints()
	return true
}

// resetCareerPoints 把角色恢复到当前等级尚未分配任何自由点、技能点的状态。
// Base 同时包含自然成长与玩家加点，不能只增加 FreePoints；必须先按等级重建
// 自然六维，才能真正洗掉就职前已经分配的属性点。
func (c *Character) resetCareerPoints() {
	c.Base = StartingBase(c.Race)
	for level := int32(2); level <= c.Level; level++ {
		c.Base = c.Base.Add(GrowthAt(c.Race, level))
	}
	levelUps := max(c.Level-1, 0)
	c.FreePoints = levelUps * FreePointsPerLevel
	c.SkillPoints = levelUps * SkillPointsPerLevel
	c.SkillPointsKnown = true
}

// BeginnerSkill 返回建角时所选路线对应的初行者技能。
// 战士、剑客、刺客学守护术；药师、术士学魔弹术。
func (c *Character) BeginnerSkill() (SkillID, bool) {
	if c == nil || !c.Race.Valid() {
		return 0, false
	}
	if c.Race == Healer || c.Race == Warlock {
		return SkillBeginnerBolt, true
	}
	return SkillBeginnerGuard, true
}

// AllowsSkill 是学习与释放技能共用的职业准入规则。
// prof=0 时只允许未就职角色使用自己路线的那一项初行者技能；
// prof=1..5 必须已就职且职业吻合。
func (c *Character) AllowsSkill(id SkillID, prof int32) bool {
	if c == nil {
		return false
	}
	if prof == 0 {
		starter, ok := c.BeginnerSkill()
		return !c.HasProfession() && ok && id == starter
	}
	return c.HasProfession() && prof == int32(c.Race)+1
}
