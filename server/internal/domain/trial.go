package domain

// TrialProgress is saved with the character, experience and inventory in one transaction.
type TrialProgress struct {
	Day, DailyCompleted, Cycle int32
	MonsterLevel               int32
 KeyReward int32
 CompletionID string
}

type TrialSpawn struct {
	Point      SpawnPoint
	Candidates []MonsterID
}

type TrialEntrance struct {
	ClaimMap, EntryMap int32
	ClaimNPC, EntryNPC string
	EntryPos           Pos
	Price              int64
}

// TrialRules is loaded exclusively from PostgreSQL. Maps contain fixed positions
// and a separate candidate pool for every spawn point.
type TrialRules struct {
	Quest                                        QuestID
	NormalCount, BossCount                       int32
	MinLevel, MaxLevel, AnchorLevel, AnchorUnits int32
	BossOffset, PenaltyGap, PenaltyPerLevel      int32
	FirstPct, RewardChance, RewardMin, RewardMax int32
	Pill, Fragment                               ItemID
	Multipliers                                  []int32
	Entrances                                    []TrialEntrance
	Maps                                         map[int32][]TrialSpawn
	Monsters                                     map[int32]map[MonsterID]MonsterDef
}

func (r *TrialRules) IsMap(id int32) bool { return r != nil && len(r.Maps[id]) > 0 }

// BaseExperience is a straight line in character level. Preserve the fractional
// high anchor until the final integer division (E(50)/16 is not an integer).
func (r *TrialRules) BaseExperience(level int32, levels *LevelTable) int64 {
	if r == nil || level < r.MinLevel || level > r.MaxLevel || levels == nil {
		return 0
	}
	n, d := r.baseFraction(level, levels)
	return (n + d/2) / d
}

func (r *TrialRules) baseFraction(level int32, levels *LevelTable) (int64, int64) {
	low := levels.Need(r.MinLevel) + levels.Need(r.MinLevel+1) + levels.Need(r.MinLevel+2)
	span, units := int64(r.AnchorLevel-r.MinLevel), int64(r.AnchorUnits)
	return (low*units*span + int64(level-r.MinLevel)*(levels.Need(r.AnchorLevel)-low*units)), span * units
}

func (r *TrialRules) Experience(level, monsterLevel int32, p TrialProgress, day int32, levels *LevelTable) int64 {
	pct := r.Multipliers[int(p.Cycle)%len(r.Multipliers)]
	if p.Day != day || p.DailyCompleted == 0 {
		pct = r.FirstPct
	}
	penalty := (level - monsterLevel - r.PenaltyGap) * r.PenaltyPerLevel
	if penalty < 0 {
		penalty = 0
	}
	if penalty > 100 {
		penalty = 100
	}
	if level < r.MinLevel || level > r.MaxLevel {
		return 0
	}
	n, d := r.baseFraction(level, levels)
	n *= int64(pct) * int64(100-penalty)
	d *= 10000
	return (n + d/2) / d
}

// DungeonParty is an immutable roster projection; scenes never read peer entities.
type DungeonParty struct {
	ID       PartyID
	Leader   CharID
	MaxLevel int32
	Members  []string
}
