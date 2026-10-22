package domain

// WithDungeonDifficultyChoices adds options to the already working native
// transport UI. Original indices and unrelated destinations never move. The
// authoritative list is rebuilt identically for open and choose, so clients
// still submit only the existing list/index pair, not a difficulty or map.
func WithDungeonDifficultyChoices(list TransportList) TransportList {
	for i, d := range list.Destinations {
		// A configured malformed list or already decorated list must not be expanded.
		if d.Index != int32(i) || d.ExplicitDifficulty {
			return list
		}
	}
	out := list
	out.Destinations = append([]TransportDestination(nil), list.Destinations...)
	for i, d := range list.Destinations {
		if !d.Dungeon || d.RequireClear || d.Price != 0 || d.Pos.X <= 0 || d.Pos.Y <= 0 || !auditedDifficultyTransport(list.ID, list.SourceMapID, d.Pos.MapID) {
			continue
		}
		out.Destinations[i].Name = d.Name + "（普通）"
		out.Destinations[i].Difficulty = DungeonNormal
		out.Destinations[i].ExplicitDifficulty = true
		hard := d
		hard.Index = int32(len(out.Destinations))
		hard.Name += "（困难）"
		hard.Difficulty = DungeonHard
		hard.ExplicitDifficulty = true
		out.Destinations = append(out.Destinations, hard)
	}
	return out
}

// Exact native NPC/list/source/map bindings from the existing production seed.
// Do not infer eligibility from Dungeon=true or from a shared map filename.
func auditedDifficultyTransport(list, source, target int32) bool {
	switch list {
	case 110:
		return source == 32 && (target == 24400 || target == 26800)
	case 115:
		return source == 7 && target == 26900
	case 125:
		return source == 7 && target == 24500
	case 126:
		return source == 19 && target == 24500
	case 117, 118, 119, 120, 121, 122:
		sources := [...]int32{7, 19, 32, 58, 86, 154}
		return source == sources[list-117] && target == 30600
	case 127, 128, 129, 130, 131, 132:
		sources := [...]int32{19, 32, 37, 58, 86, 154}
		return source == sources[list-127] && target == 23800
	}
	return false
}
