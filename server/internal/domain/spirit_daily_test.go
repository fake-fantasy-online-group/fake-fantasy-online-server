package domain

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestSpiritDailyStableDistinctSnapshot(t *testing.T) {
	pool := []SpiritDailyTarget{{Monster: 1, MapID: 1}, {Monster: 2, MapID: 1}, {Monster: 1, MapID: 1}, {Monster: 3, MapID: 2}, {Monster: 4, MapID: 2}}
	a, ok := SelectSpiritDailyTargets(pool, 123, "2026-10-28")
	if !ok {
		t.Fatal("missing targets")
	}
	for i := 0; i < 3; i++ {
		for j := i + 1; j < 3; j++ {
			if a[i].Monster == a[j].Monster {
				t.Fatal("duplicate boss")
			}
		}
	}
	b, _ := SelectSpiritDailyTargets(pool, 123, "2026-10-28")
	if a != b {
		t.Fatal("rerolled")
	}
	st := SpiritDailyQuestState{Day: "2026-10-28", Kind: 9, Targets: a}
	raw, _ := json.Marshal(st)
	var restored SpiritDailyQuestState
	if json.Unmarshal(raw, &restored) != nil || !reflect.DeepEqual(st, restored) {
		t.Fatal("snapshot lost")
	}
	q, ok := st.Definition()
	if !ok || len(q.Steps[0].Kill) != 3 || q.Reward.Items[0].Qty != 300 {
		t.Fatal(q)
	}
	if _, ok := SelectSpiritDailyTargets(pool[:2], 123, "2026-10-28"); ok {
		t.Fatal("incomplete pool")
	}
}
func TestSpiritDailyChinaMidnight(t *testing.T) {
	a := time.Date(2026, 10, 28, 15, 59, 59, 0, time.UTC)
	if SpiritDailyDay(a) != "2026-10-28" || SpiritDailyDay(a.Add(time.Second)) != "2026-10-29" {
		t.Fatal("calendar")
	}
}
func TestSpiritDailySnapshotCloneIsolation(t *testing.T) {
	entry := QuestEntry{SpiritDaily: SpiritDailyQuestState{Kind: 9, Targets: [3]SpiritDailyTarget{{Monster: 1, MapID: 7}, {Monster: 2, MapID: 8}, {Monster: 3, MapID: 9}}}, Kills: map[MonsterID]int32{1: 1}}
	original := QuestLog{SpiritDailyQuestID: entry}
	cloned := original.Clone()
	e := cloned[SpiritDailyQuestID]
	e.SpiritDaily.Targets[0].Monster = 99
	e.Kills[1] = 99
	cloned[SpiritDailyQuestID] = e
	if original[SpiritDailyQuestID].SpiritDaily.Targets[0].Monster != 1 || original[SpiritDailyQuestID].Kills[1] != 1 {
		t.Fatal("snapshot alias")
	}
}
