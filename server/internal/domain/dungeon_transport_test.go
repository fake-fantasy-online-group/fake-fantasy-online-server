package domain

import (
	"reflect"
	"testing"
)

func TestDungeonTransportAuditedChoicesPreserveOriginalIndices(t *testing.T) {
	for _, tc := range []struct{ list, source, target int32 }{{110, 32, 24400}, {110, 32, 26800}, {115, 7, 26900}, {125, 7, 24500}, {126, 19, 24500}, {117, 7, 30600}, {118, 19, 30600}, {119, 32, 30600}, {120, 58, 30600}, {121, 86, 30600}, {122, 154, 30600}, {127, 19, 23800}, {128, 32, 23800}, {129, 37, 23800}, {130, 58, 23800}, {131, 86, 23800}, {132, 154, 23800}} {
		original := TransportList{ID: tc.list, SourceMapID: tc.source, Destinations: []TransportDestination{{Index: 0, Name: "verified", Pos: Pos{MapID: tc.target, X: 100, Y: 100}, Dungeon: true}, {Index: 1, Name: "婚礼场", Pos: Pos{MapID: 30800, X: 100, Y: 100}, Dungeon: true}}}
		got := WithDungeonDifficultyChoices(original)
		if len(got.Destinations) != 3 || got.Destinations[0].Name != "verified（普通）" || !got.Destinations[0].ExplicitDifficulty || got.Destinations[0].Difficulty != DungeonNormal {
			t.Fatal(tc, got)
		}
		hard := got.Destinations[2]
		if hard.Index != 2 || hard.Name != "verified（困难）" || hard.Pos.MapID != tc.target || hard.Difficulty != DungeonHard || !hard.ExplicitDifficulty {
			t.Fatal(tc, hard)
		}
		if !reflect.DeepEqual(got.Destinations[1], original.Destinations[1]) || original.Destinations[0].Name != "verified" {
			t.Fatal("mutated unrelated destination or source")
		}
		if !reflect.DeepEqual(got, WithDungeonDifficultyChoices(got)) {
			t.Fatal("decoration is not idempotent")
		}
	}
}
func TestDungeonTransportRejectsUnauditedRoutes(t *testing.T) {
	for _, tc := range []struct{ list, source, target int32 }{{127, 7, 23800}, {133, 19, 23800}, {127, 19, 23900}, {110, 32, 24800}, {110, 32, 21601}, {117, 7, 30800}, {125, 7, 21100}} {
		original := TransportList{ID: tc.list, SourceMapID: tc.source, Destinations: []TransportDestination{{Name: "unchanged", Pos: Pos{MapID: tc.target, X: 100, Y: 100}, Dungeon: true}}}
		if !reflect.DeepEqual(original, WithDungeonDifficultyChoices(original)) {
			t.Fatal("expanded unaudited route", tc)
		}
	}
}

func TestDungeonTransportMultipleOptionsAppendAfterAllOriginals(t *testing.T) {
	list := TransportList{ID: 110, SourceMapID: 32, Destinations: []TransportDestination{{Index: 0, Name: "龙", Pos: Pos{MapID: 24400, X: 100, Y: 100}, Dungeon: true}, {Index: 1, Name: "神将", Pos: Pos{MapID: 26800, X: 100, Y: 100}, Dungeon: true}}}
	got := WithDungeonDifficultyChoices(list)
	if len(got.Destinations) != 4 {
		t.Fatal(got)
	}
	for i, target := range []int32{24400, 26800, 24400, 26800} {
		if got.Destinations[i].Index != int32(i) || got.Destinations[i].Pos.MapID != target {
			t.Fatal("index reordered", got)
		}
	}
	list = TransportList{ID: 125, SourceMapID: 7}
	for i, target := range []int32{21100, 21200, 21300, 21400, 21500, 24500} {
		list.Destinations = append(list.Destinations, TransportDestination{Index: int32(i), Name: "original", Pos: Pos{MapID: target, X: 100, Y: 100}, Dungeon: true})
	}
	got = WithDungeonDifficultyChoices(list)
	if len(got.Destinations) != 7 || got.Destinations[5].Difficulty != DungeonNormal || got.Destinations[6].Difficulty != DungeonHard || got.Destinations[6].Pos.MapID != 24500 {
		t.Fatal(got)
	}
	for i := 0; i < 5; i++ {
		if !reflect.DeepEqual(got.Destinations[i], list.Destinations[i]) {
			t.Fatal("trial menu changed")
		}
	}
}
