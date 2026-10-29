package domain

// DungeonEntryQuest is an explicit task-only map binding, not a name/level/loot
// heuristic. Evidence: init-32 task steps 356/359/361 and init-0 solo maps.
// Ordinary repeatable reward dungeons and the separately guarded trial/rift are
// intentionally absent. A related quest alone never makes a dungeon task-only.
// 29200/355 is deliberately held: the current missing publisher disables quest355
// in LoadQuests, so existing 0x8027 Active cannot faithfully expose saved progress.
func DungeonEntryQuest(mapID int32) QuestID {
	switch mapID {
	case 29400:
		return 356
	case 29900:
		return 359
	case 30000:
		return 361
	}
	return 0
}

// CanEnterQuestDungeon admits each character from their own persisted log.
// Active and Deliver are both unfinished; Finished is not permission to return
// to these three solo task maps. No attempt counter or party-companion rule.
func CanEnterQuestDungeon(mapID int32, log QuestLog) bool {
	quest := DungeonEntryQuest(mapID)
	if quest == 0 {
		return true
	}
	entry, ok := log[quest]
	return ok && (entry.State == QuestActive || entry.State == QuestDeliver)
}
