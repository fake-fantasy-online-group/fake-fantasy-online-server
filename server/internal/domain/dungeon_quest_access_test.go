package domain

import "testing"

func TestQuestOnlyDungeonAdmissionAndOrdinaryFarmExclusions(t *testing.T) {
	for _, pair := range [][2]int32{{29200, 355}, {29400, 356}, {29900, 359}, {30000, 361}} {
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

// Missing definitions must not fabricate an accepted quest, but they must not
// erase the real eligibility of a character's retained unfinished 355 record.
func TestQuestDungeonRetainedDisabled355Projection(t *testing.T) {
	for _, state := range []int{-1, 0, 1, 2, 3} {
		ch := &Character{Quests: QuestLog{999: {State: QuestActive}}}
		if state >= 0 {
			ch.Quests[355] = QuestEntry{State: QuestState(state), Progress: 2}
		}
		got := ActiveQuestsOf(QuestTable{}, ch, nil)
		want := state == 0 || state == 1
		if (len(got) == 1) != want || CanEnterQuestDungeon(29200, ch.Quests) != want {
			t.Fatalf("state%d projection/admission disagree: %#v", state, got)
		}
		if want && (got[0].ID != 355 || got[0].Text != "九转（法系）" || got[0].Step != 2 || got[0].Ready != (state == 1) || got[0].Total != 0 || got[0].To != "") {
			t.Fatalf("projection invented or lost saved/native facts: %#v", got)
		}
		if state == 2 && len(ch.Quests.FinishedIDs()) != 1 {
			t.Fatal("finished record lost")
		}
	}
	ch := &Character{Quests: QuestLog{355: {State: QuestActive}}}
	def := QuestDef{ID: 355, Name: "九转（法系）", Desc: "loaded definition", NPC: "native publisher"}
	got := ActiveQuestsOf(QuestTable{355: def}, ch, nil)
	if len(got) != 1 || got[0].Text != def.Desc || got[0].To != def.NPC {
		t.Fatal("fallback replaced a real definition", got)
	}
}
