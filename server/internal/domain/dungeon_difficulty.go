package domain

// DungeonDifficulty is server-owned run state, never a client-selected instance ID.
type DungeonDifficulty uint8

const (
	DungeonNormal DungeonDifficulty = iota
	DungeonHard
)

func (d DungeonDifficulty) Valid() bool { return d == DungeonNormal || d == DungeonHard }

// NativeDungeonCommand recognizes only the existing shipped client commands.
func NativeDungeonCommand(raw string) (int32, DungeonDifficulty, string, bool) {
	switch raw {
	case "taoyu":
		return 20502, DungeonNormal, "", true
	case "taoyu hard":
		return 20502, DungeonHard, "", true
	case "dungeonhard 20100":
		return 20100, DungeonHard, "病叟", true
	case "dungeonhard 20300":
		return 20300, DungeonHard, "寂仙子", true
	case "dungeonhard 20600":
		return 20600, DungeonHard, "纯阳塔神卫", true
	case "dungeonhard 20701":
		return 20701, DungeonHard, "墨玉", true
	case "dungeonhard 20801":
		return 20801, DungeonHard, "二郎神杨戬", true
	case "dungeonhard 20901":
		return 20901, DungeonHard, "斯芬尼克", true
	}
	return 0, DungeonNormal, "", false
}
