package domain

import (
	"fmt"
	"testing"
)

// The existing native task wire has no raw QuestState field. Any row projected
// here is treated as Active by the real client menu, so the receipt must agree
// with admission even when a saved, defined task contains an invalid state.
func TestQuestDungeonDefinedReceiptMatchesAdmission(t *testing.T) {
	for _, pair := range [][2]int32{{29200, 355}, {29400, 356}, {29900, 359}, {30000, 361}} {
		for _, state := range []QuestState{QuestActive, QuestDeliver, QuestFinished, QuestState(3), QuestState(255)} {
			t.Run(fmt.Sprintf("map%d-state%d", pair[0], state), func(t *testing.T) {
				id := QuestID(pair[1])
				ch := &Character{Quests: QuestLog{id: {State: state}}}
				defs := QuestTable{id: {ID: id, Name: "loaded native definition", NPC: "native NPC"}}
				rows := ActiveQuestsOf(defs, ch, nil)
				if shown, allowed := len(rows) != 0, CanEnterQuestDungeon(pair[0], ch.Quests); shown != allowed {
					t.Fatalf("native receipt makes client show=%v, server admission=%v: %#v", shown, allowed, rows)
				}
			})
		}
	}
}

func TestQuestDungeonProjectionDoesNotInventMissingRecordsOrChangeOtherTasks(t *testing.T) {
	for _, id := range []QuestID{355, 356, 359, 361} {
		ch := &Character{Quests: QuestLog{}}
		if rows := ActiveQuestsOf(QuestTable{id: {ID: id}}, ch, nil); len(rows) != 0 {
			t.Fatal("missing record projected", id, rows)
		}
	}
	// This bounded fix does not redefine receipt behavior for any other task.
	const other QuestID = 999
	ch := &Character{Quests: QuestLog{other: {State: QuestState(3)}}}
	rows := ActiveQuestsOf(QuestTable{other: {ID: other}}, ch, nil)
	if len(rows) != 1 || rows[0].ID != other {
		t.Fatal("unrelated task projection changed", rows)
	}
}
