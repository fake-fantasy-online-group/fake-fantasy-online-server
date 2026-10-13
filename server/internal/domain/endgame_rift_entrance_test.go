package domain

import (
	"reflect"
	"strings"
	"testing"
)

func TestRiftEntranceConfigurationValidationAndSnapshotIsolation(t *testing.T) {
	valid := EndgameRiftEntranceView{MapID: 7, NPCID: 83, NPC: "八仙使者", Script: "Nbaxian"}
	makeTable := func(entries ...EndgameRiftEntranceView) (*RiftRuleTable, error) {
		return NewRiftRuleTable(1, []RiftRules{*riftRulesFixture()}, lootRulesTableFixture(), balanceItemsFixture(), entries...)
	}
	entries := []EndgameRiftEntranceView{valid}
	table, err := makeTable(entries...)
	if err != nil {
		t.Fatal(err)
	}
	entries[0].NPC = "changed"
	if table.Entrances()[0] != valid {
		t.Fatal("caller mutated entrance binding")
	}
	if !reflect.DeepEqual(table.Active(), riftRulesFixture()) {
		t.Fatal("new world service changed immutable balance snapshot")
	}
	for _, change := range []func(*EndgameRiftEntranceView){
		func(e *EndgameRiftEntranceView) { e.MapID = 0 }, func(e *EndgameRiftEntranceView) { e.NPCID = 0 },
		func(e *EndgameRiftEntranceView) { e.NPC = "" }, func(e *EndgameRiftEntranceView) { e.Script = "" },
		func(e *EndgameRiftEntranceView) { e.NPC = strings.Repeat("a", 129) }, func(e *EndgameRiftEntranceView) { e.Script = strings.Repeat("a", 129) },
	} {
		bad := valid
		change(&bad)
		if _, err := makeTable(bad); err == nil {
			t.Fatalf("invalid binding accepted: %+v", bad)
		}
	}
	if _, err := makeTable(valid, valid); err == nil {
		t.Fatal("duplicate placement accepted")
	}
}
