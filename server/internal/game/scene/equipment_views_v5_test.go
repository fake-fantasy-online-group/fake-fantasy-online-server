package scene

import (
	"encoding/json"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	"strings"
	"testing"
)

func TestEquipmentV5StatusRequirementExpiryInvalidatesCache(t *testing.T) {
	s := newCombatScene(t)
	join(t, s, 1, 100, "临时门槛", 100, 100)
	p := s.players[1]
	st := wearFormationAuraTest(t, s, p, 24, 9001)
	p.Player.Char.Base.STR = 20
	def := s.items[st.Item]
	def.Equip.Need.Base.STR = 80
	s.items[st.Item] = def
	if len(s.equipmentEffects(p).effects) != 0 {
		t.Fatal("gear active below requirement")
	}
	buff := domain.StatusDef{ID: 900001, Level: 1, Name: "临时力量", DurationSec: 1, Affixes: []domain.Affix{{Attr: domain.AttrSTR, Value: 100, Mode: domain.ModeAbsolute}}}
	if !s.applyStatusDef(p, buff, p.ID) {
		t.Fatal("buff did not apply")
	}
	s.stepEquipmentAuras()
	if len(s.equipmentEffects(p).effects) == 0 || s.equipmentDefense(p, false).Elements[0].ResistBP != s.equipmentEffects(p).stats.Elements[0].ResistBP+formationAuraTest(t, 24).Value {
		t.Fatal("buff did not enable item/aura")
	}
	s.tick += domain.Ticks(1000)
	s.stepStatus()
	s.stepEquipmentAuras()
	if len(s.equipmentEffects(p).effects) != 0 || s.equipmentDefense(p, false).Elements[0].ResistBP != 0 {
		t.Fatal("expired requirement buff left cached aura active")
	}
	u := s.items[st.Item]
	if p.Player.Worn.At(domain.EquipSlot(u.Equip.Slot)).UID != st.UID {
		t.Fatal("test accidentally unequipped gear")
	}
}
func TestEquipmentV5ObservationSnapshotAndChangePush(t *testing.T) {
	s := newCombatScene(t)
	sink := join(t, s, 1, 100, "观察", 100, 100)
	p := s.players[1]
	p.Player.EndgameGrowthV5 = true
	st := wearFormationAuraTest(t, s, p, 24, 9007199254740993)
	wantResist := s.equipmentEffects(p).stats.Elements[0].ResistBP + formationAuraTest(t, 24).Value
	s.stepEquipmentAuras()
	s.flush()
	sink.take()
	state := s.endgamePlayer(p)
	state.loaded = true
	p.Player.Bag = domain.NewBag(80)
	for i := 0; i < 40; i++ {
		copy := st
		copy.UID += int64(i) + 1
		p.Player.Bag.Set(i, copy)
	}
	s.sendEndgameSnapshot(p)
	s.flush()
	snapshots, withStats := 0, 0
	for _, raw := range sink.take() {
		ev, ok := raw.(event.EndgameMessage)
		if !ok || ev.Message.Kind != "snapshot" {
			continue
		}
		snapshots++
		if ev.Message.CombatStats != nil {
			withStats++
			if ev.Message.Chunk != 0 {
				t.Fatal("combatstats repeated afterchunk0")
			}
			v := ev.Message.CombatStats
			if len(v.Elements) != 4 || v.Elements[0].ResistBP != wantResist || len(v.EquipmentAuras) != 1 || v.EquipmentAuras[0].SourceUID != st.UID {
				t.Fatalf("bad authoritative view %+v", v)
			}
			body, _ := json.Marshal(v)
			if !strings.Contains(string(body), `"sourceUID":"9007199254740993"`) {
				t.Fatal("UID lostprecision", string(body))
			}
		}
		if packets := protocol.Encode(p.ID, ev); len(packets) == 0 {
			t.Fatal("observation snapshot could notencode")
		}
	}
	if snapshots != 2 || withStats != 1 {
		t.Fatal(snapshots, withStats)
	}
	s.publishEquipmentCombatStats(p)
	s.flush()
	sink.take()
	s.publishEquipmentCombatStats(p)
	s.flush()
	if len(sink.take()) != 0 {
		t.Fatal("unchanged observation republished")
	}
	u := s.items[st.Item]
	p.Player.Worn.Set(domain.EquipSlot(u.Equip.Slot), domain.Stack{})
	s.stepEquipmentAuras()
	s.flush()
	changed := false
	for _, raw := range sink.take() {
		if ev, ok := raw.(event.EndgameMessage); ok && ev.Message.Kind == "combat.stats" {
			changed = true
			if len(ev.Message.CombatStats.EquipmentAuras) != 0 || ev.Message.CombatStats.Elements[0].ResistBP != 0 {
				t.Fatal("stale aura view")
			}
		}
	}
	if !changed {
		t.Fatal("aura removal did notpublish combat observation")
	}
}
