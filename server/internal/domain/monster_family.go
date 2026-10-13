package domain

// MonsterFamily is a server-side taxonomy independent of combat rank (Kind).
// Unknown is deliberately distinct from a reviewed Other. Neither matches a
// positive undead/demon requirement. Names and IDs are never interpreted here.
type MonsterFamily string

const (
	MonsterFamilyUnknown MonsterFamily = "unknown"
	MonsterFamilyOther   MonsterFamily = "other"
	MonsterFamilyUndead  MonsterFamily = "undead"
	MonsterFamilyDemon   MonsterFamily = "demon"
)

func (f MonsterFamily) Valid() bool {
	switch f {
	case MonsterFamilyUnknown, MonsterFamilyOther, MonsterFamilyUndead, MonsterFamilyDemon:
		return true
	}
	return false
}

// MatchesMonsterFamily evaluates the declarative allow-list. A missing list
// leaves existing skills unrestricted; an unclassified target never satisfies
// a positive family condition, even if an invalid caller requests "unknown".
func (d SkillDef) MatchesMonsterFamily(f MonsterFamily) bool {
	if len(d.TargetFamilies) == 0 {
		return true
	}
	if !f.Valid() || f == MonsterFamilyUnknown {
		return false
	}
	for _, allowed := range d.TargetFamilies {
		if allowed == f {
			return true
		}
	}
	return false
}
