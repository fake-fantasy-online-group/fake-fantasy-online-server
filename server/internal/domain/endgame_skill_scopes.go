package domain

import "strings"

// Verified original active-skill semantics; catalogue/skill_scopes.json records
// SQL provenance, prepare ranges, shapes, and the permitted execution channels.
type EndgameSkillScope struct {
	Name          string
	Profession    uint8
	AllowedTraits string
}

func (d EndgameSkillScope) Supports(kind string) bool {
	return strings.Contains(" "+d.AllowedTraits+" ", " "+kind+" ")
}

var endgameSkillScopes = map[SkillID]EndgameSkillScope{
	10002: {Name: "连环枪法", Profession: 1, AllowedTraits: "ambush critical_focus desperate_assault elite_hunter execution healthy_assault invisible_focus single_focus skill_economy skill_focus"},
	10021: {Name: "流虹之枪", Profession: 1, AllowedTraits: "ambush area_expansion area_focus critical_focus desperate_assault elite_hunter execution healthy_assault invisible_focus skill_economy skill_focus"},
	10022: {Name: "野战八荒", Profession: 1, AllowedTraits: "ambush area_expansion area_focus critical_focus desperate_assault elite_hunter execution healthy_assault invisible_focus skill_economy skill_focus"},
	10102: {Name: "罡气剑", Profession: 2, AllowedTraits: "ambush critical_focus desperate_assault elite_hunter execution healthy_assault invisible_focus single_focus skill_economy skill_focus"},
	10104: {Name: "饮血剑", Profession: 2, AllowedTraits: "ambush critical_focus desperate_assault elite_hunter execution healthy_assault invisible_focus single_focus skill_economy skill_focus"},
	10121: {Name: "旋风剑", Profession: 2, AllowedTraits: "ambush area_expansion area_focus critical_focus desperate_assault elite_hunter execution healthy_assault invisible_focus skill_economy skill_focus"},
	10209: {Name: "背刺", Profession: 3, AllowedTraits: "ambush critical_focus desperate_assault elite_hunter execution healthy_assault invisible_focus single_focus skill_economy skill_focus"},
	10213: {Name: "陷阱：爆弹", Profession: 3, AllowedTraits: "ambush critical_focus desperate_assault elite_hunter execution healthy_assault invisible_focus skill_economy skill_focus trap_expansion"},
	10216: {Name: "陷阱：冻气", Profession: 3, AllowedTraits: "ambush critical_focus desperate_assault elite_hunter execution healthy_assault invisible_focus skill_economy skill_focus trap_expansion"},
	10301: {Name: "治愈术", Profession: 4, AllowedTraits: "healing_cost healing_grace rescue_heal selfless_heal skill_economy spell_economy"},
	10304: {Name: "群疗术", Profession: 4, AllowedTraits: "area_expansion healing_cost healing_grace rescue_heal selfless_heal skill_economy swift_cast"},
	10319: {Name: "光弹术", Profession: 4, AllowedTraits: "ambush critical_focus desperate_assault elite_hunter execution healthy_assault holy_mastery invisible_focus single_focus skill_economy skill_focus spell_economy"},
	10401: {Name: "火弹术", Profession: 5, AllowedTraits: "ambush critical_focus desperate_assault elite_hunter execution fire_mastery healthy_assault invisible_focus single_focus skill_economy skill_focus spell_economy swift_cast"},
	10402: {Name: "火球术", Profession: 5, AllowedTraits: "ambush area_expansion area_focus critical_focus desperate_assault elite_hunter execution fire_mastery healthy_assault invisible_focus skill_economy skill_focus spell_economy swift_cast"},
	10409: {Name: "冰箭术", Profession: 5, AllowedTraits: "ambush critical_focus desperate_assault elite_hunter execution healthy_assault ice_mastery invisible_focus single_focus skill_economy skill_focus spell_economy"},
	10412: {Name: "暴雪术", Profession: 5, AllowedTraits: "ambush area_expansion area_focus critical_focus desperate_assault elite_hunter execution healthy_assault ice_mastery invisible_focus skill_economy skill_focus spell_economy"},
}

func EndgameSkillTarget(id SkillID) (EndgameSkillScope, bool) {
	d, ok := endgameSkillScopes[id]
	return d, ok
}

// Native class capability is a real applicability guard, not flavor text.
// A shared item may serve one eligible class, but a class-locked item must have
// some executable path without assuming nonexistent elemental/ritual skills.
func TraitMatchesProfessions(t UniqueTrait, mask uint8) bool {
	allowed := uint8(31)
	switch t.Kind {
	case "healing_grace", "rescue_heal", "selfless_heal", "healing_cost", "holy_mastery":
		allowed = 8
	case "fire_mastery", "ice_mastery":
		allowed = 16
	case "swift_cast", "spell_economy":
		allowed = 24
	case "trap_expansion", "invisible_focus":
		allowed = 4
	}
	if mask != 0 && mask&allowed == 0 {
		return false
	}
	if t.SkillID == 0 {
		return true
	}
	d, ok := EndgameSkillTarget(t.SkillID)
	return ok && (mask == 0 || mask&(1<<(d.Profession-1)) != 0)
}
