package session

import (
	"encoding/json"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	"strings"
	"testing"
)

// globalBudgetFixture uses current slot/skill counts with long, wire-legal
// descriptions. It is a transport worst-case counterexample, not a claim about
// the user's inventory or the current catalog's longest tooltip.
func globalBudgetFixture(t *testing.T) []event.Event {
	t.Helper()
	response := domain.EndgameResponse{Kind: "rift.state", OK: true, Rift: &domain.EndgameRiftView{RunID: "budget", Phase: "active", Revision: "1"}}
	base, _ := json.Marshal(response)
	response.Message = strings.Repeat("x", domain.EndgameRiftSnapshotBytes-len(base)-13)
	raw, _ := json.Marshal(response)
	// Adjust for the exact optional message key syntax rather than guessing.
	response.Message += strings.Repeat("x", domain.EndgameRiftSnapshotBytes-len(raw))
	raw, _ = json.Marshal(response)
	if len(raw) != domain.EndgameRiftSnapshotBytes {
		t.Fatalf("raw=%d", len(raw))
	}
	pages, err := domain.PageEndgameRift(response, strings.Repeat("s", 64))
	if err != nil {
		t.Fatal(err)
	}
	var events []event.Event
	bytes, maxPacket := 0, 0
	for _, p := range pages {
		events = append(events, event.EndgameMessage{Who: 1, Message: p})
		pkt := protocol.Encode(1, event.EndgameMessage{Who: 1, Message: p})[0]
		bytes += len(pkt)
		if len(pkt) > maxPacket {
			maxPacket = len(pkt)
		}
	}
	t.Logf("maxRift raw=%d pages=%d nativeInnerBytes=%d largestPacket=%d", len(raw), len(pages), bytes, maxPacket)
	inv := event.InventorySnapshot{Who: 1}
	for i := 0; i < domain.DefaultBagSlots; i++ {
		inv.Items = append(inv.Items, event.InventoryItemView{BagIndex: int32(i), Item: 1, Count: 1, Name: "item", Desc: strings.Repeat("d", 2500)})
	}
	wardrobe := event.WardrobeSnapshot{Who: 1, Capacity: 200}
	for i := 0; i < 200; i++ {
		wardrobe.Items = append(wardrobe.Items, event.WardrobeItemView{Item: domain.ItemID(i + 1), Name: "item", Desc: strings.Repeat("d", 5000)})
	}
	pet := event.PetSnapshot{Who: 1, ActiveSlot: 0, FightCap: 4, LifeCap: 8}
	for i := 0; i < 12; i++ {
		pet.Active.Skills = append(pet.Active.Skills, event.PetSkillView{ID: int32(i + 1), Name: "skill", Description: strings.Repeat("d", 60000)})
	}
	for i := 0; i < domain.MaxPets; i++ {
		slot := event.PetSlotView{Slot: int32(i), Name: "pet", Species: "species"}
		for j := 0; j < 12; j++ {
			slot.Skills = append(slot.Skills, event.PetSkillView{Name: strings.Repeat("s", 2000), Level: 1})
		}
		pet.Pets = append(pet.Pets, slot)
	}
	events = append(events, inv, wardrobe, pet)
	for _, e := range []event.Event{inv, wardrobe, pet} {
		packets := protocol.Encode(1, e)
		n := 0
		for _, p := range packets {
			n += len(p)
		}
		t.Logf("protocolValid %T frames=%d bytes=%d", e, len(packets), n)
	}
	return events
}

func TestGlobalBurstBudgetEvidence(t *testing.T) {
	events := globalBudgetFixture(t)
	if len(events) != 32 {
		t.Fatalf("global event count=%d", len(events))
	}
}
