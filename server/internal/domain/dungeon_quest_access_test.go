package domain

import "testing"

func TestQuestOnlyDungeonAdmissionAndOrdinaryFarmExclusions(t *testing.T) {
	for _, pair := range [][2]int32{{29400, 356}, {29900, 359}, {30000, 361}} {
		for _, state := range []int{-1, 0, 1, 2, 3} {
			log := QuestLog{}
			if state >= 0 {
				log[QuestID(pair[1])] = QuestEntry{State: QuestState(state)}
			}
			want := state == 0 || state == 1
			if CanEnterQuestDungeon(pair[0], log) != want {
				t.Fatalf("map%d state%d", pair[0], state)
			}
		}
		log := QuestLog{QuestID(pair[1]): {State: QuestActive}}
		if !log.Abandon(QuestID(pair[1])) || CanEnterQuestDungeon(pair[0], log) {
			t.Fatal("abandon retains admission")
		}
		log[QuestID(pair[1])] = QuestEntry{State: QuestActive}
		if !CanEnterQuestDungeon(pair[0], log) {
			t.Fatal("legitimate reaccept lost admission")
		}
		log[QuestID(pair[1])] = QuestEntry{State: QuestFinished}
		if log.Abandon(QuestID(pair[1])) || CanEnterQuestDungeon(pair[0], log) {
			t.Fatal("finished history cannot be abandoned into access")
		}
	}
	// Loot/repeatable maps must not inherit rules merely because they have quests.
	for _, mid := range []int32{20100, 20300, 20502, 20600, 20701, 20801, 20901, 21100, 21200, 21300, 21400, 21500, 22901, 23601, 23800, 24400, 24500, 26800, 26900, 30600, 36700, 40700, 40900, 41100} {
		if DungeonEntryQuest(mid) != 0 || !CanEnterQuestDungeon(mid, nil) || !CanEnterQuestDungeon(mid, QuestLog{355: {State: QuestFinished}}) {
			t.Fatal("unrelated/farm/trial map gated", mid)
		}
	}
}
