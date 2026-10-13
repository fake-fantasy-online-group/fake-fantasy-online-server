package scene

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/entity"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func TestRiftEntranceRequiresExactNativePlacementAndDialogue(t *testing.T) {
	s, _, _ := riftFixture(t)
	p := s.players[1]
	npc := &entity.Entity{ID: 77, Kind: domain.KindNPC, Name: "八仙使者", Pos: p.Pos,
		NPC: &entity.NPCInfo{ConfigID: 83, Script: "Nbaxian"}}
	s.entities[npc.ID] = npc
	p.Player.QuestNPC = npc.Name
	if !s.canUseEndgameRiftEntrance(p, npc.ID) {
		t.Fatal("real configured NPC rejected")
	}
	for _, tc := range []struct {
		name    string
		change  func()
		restore func()
	}{
		{"copied name with wrong placement", func() { npc.NPC.ConfigID = 84 }, func() { npc.NPC.ConfigID = 83 }},
		{"wrong script", func() { npc.NPC.Script = "Nslguard_22" }, func() { npc.NPC.Script = "Nbaxian" }},
		{"different dialog", func() { p.Player.QuestNPC = "妖魔结界守卫" }, func() { p.Player.QuestNPC = npc.Name }},
		{"wrong NPC name", func() { npc.Name = "冒名使者"; p.Player.QuestNPC = npc.Name }, func() { npc.Name = "八仙使者"; p.Player.QuestNPC = npc.Name }},
		{"wrong NPC map", func() { npc.Pos.MapID = 22 }, func() { npc.Pos.MapID = 7 }},
		{"wrong player map", func() { p.Pos.MapID = 22 }, func() { p.Pos.MapID = 7 }},
		{"wrong scene map", func() { s.id.MapID = 22 }, func() { s.id.MapID = 7 }},
		{"instance copy", func() { s.id.Instance = 1 }, func() { s.id.Instance = 0 }},
		{"non-NPC entity", func() { npc.Kind = domain.KindPlayer }, func() { npc.Kind = domain.KindNPC }},
		{"missing NPC metadata", func() { npc.NPC = nil }, func() { npc.NPC = &entity.NPCInfo{ConfigID: 83, Script: "Nbaxian"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.change()
			defer tc.restore()
			if s.canUseEndgameRiftEntrance(p, npc.ID) {
				t.Fatal("invalid entrance admitted")
			}
		})
	}
	if s.canUseEndgameRiftEntrance(p, 0) || s.canUseEndgameRiftEntrance(p, 78) {
		t.Fatal("missing/stale entity admitted")
	}
	delete(s.entities, npc.ID)
	if s.canUseEndgameRiftEntrance(p, npc.ID) {
		t.Fatal("removed entity admitted")
	}
}

func TestRiftStartWrongEntranceNeverReservesOrDebits(t *testing.T) {
	s, b, _ := riftFixture(t)
	p := s.players[1]
	s.trial.Entrances = []domain.TrialEntrance{{EntryMap: 7, EntryNPC: "妖魔结界守卫"}}
	s.entities[77] = &entity.Entity{ID: 77, Kind: domain.KindNPC, Name: "妖魔结界守卫", Pos: p.Pos, NPC: &entity.NPCInfo{ConfigID: 1, Script: "Nslguard_22"}}
	p.Player.QuestNPC = "妖魔结界守卫"
	p.Player.Bag.AddStack(s.items[domain.RiftKeyItemID], domain.Stack{Item: domain.RiftKeyItemID, Count: 1, Bound: true})
	if !s.trialEntryNPC(p) {
		t.Fatal("ordinary trial service lost")
	}
	s.startEndgameRift(p, domain.EndgameRequest{Op: "rift.start", NPCEntityID: 77, Tier: 1})
	if b.commits != 0 || p.Player.Bag.CountOf(domain.RiftKeyItemID) != 1 || len(s.router.riftAdmissions) != 0 {
		t.Fatal("invalid entrance charged or allocated run")
	}
	s.flush()
	found := false
	for _, e := range p.Player.Sink.(*fakeSink).take() {
		if m, ok := e.(event.EndgameMessage); ok && m.Message.Kind == "rift.start" {
			found = m.Message.Error == "entrance"
		}
	}
	if !found {
		t.Fatal("missing entrance rejection")
	}
}

func TestRiftHelloAdvertisesDatabaseBindings(t *testing.T) {
	s, _, _ := riftFixture(t)
	p := s.players[1]
	s.dispatchEndgame(p, domain.EndgameRequest{Op: "hello", Capabilities: []string{domain.EndgameV1, domain.EndgameRiftEntranceV1}})
	s.flush()
	for _, e := range p.Player.Sink.(*fakeSink).take() {
		if m, ok := e.(event.EndgameMessage); ok && m.Message.Kind == "hello" {
			if !reflect.DeepEqual(m.Message.RiftEntrances, s.riftRules.Entrances()) {
				t.Fatal("hello entrance configuration diverged")
			}
			copy := m.Message.RiftEntrances
			copy[0].NPC = "changed"
			if s.riftRules.Entrances()[0].NPC != "八仙使者" {
				t.Fatal("mutable entrance table leaked")
			}
			return
		}
	}
	t.Fatal("hello missing")
}

func TestRiftBindingsDoNotReskinNativeNPCs(t *testing.T) {
	s, _, _ := riftFixture(t)
	s.trial.Entrances = []domain.TrialEntrance{{EntryMap: 7, EntryNPC: "妖魔结界守卫"}}
	sources := []domain.NPCSpawn{
		{NPCID: 83, Name: "八仙使者", Sprite: 630, Script: "Nbaxian", Portrait: "N630", Sell: 10, Trans: 11, Greeting: "原有问候", Pos: domain.Pos{MapID: 7, X: 9917, Y: 4734}},
		{NPCID: 1, Name: "妖魔结界守卫", Sprite: 700, Script: "Nslguard_22", Portrait: "N700", Sell: 12, Trans: 13, Greeting: "守卫问候", Pos: domain.Pos{MapID: 7, X: 100, Y: 100}},
	}
	s.placeNPCs(sources)
	for _, source := range sources {
		found := false
		for _, e := range s.entities {
			if e.NPC != nil && e.Name == source.Name {
				found = true
				if e.NPC.ConfigID != source.NPCID || e.Look.ModelID != source.Sprite || e.NPC.Script != source.Script || e.NPC.Portrait != source.Portrait || e.NPC.Sell != source.Sell || e.NPC.Trans != source.Trans || e.NPC.Greeting != source.Greeting || e.Pos != source.Pos {
					t.Fatalf("native NPC mutated: %+v", e)
				}
			}
		}
		if !found {
			t.Fatalf("native NPC missing: %s", source.Name)
		}
	}
}

func TestRiftEntranceHelloIsOptInForStrictLegacyClients(t *testing.T) {
	for _, caps := range [][]string{{domain.EndgameV1}, {domain.EndgameRiftEntranceV1}, nil} {
		s, _, _ := riftFixture(t)
		p := s.players[1]
		s.dispatchEndgame(p, domain.EndgameRequest{Op: "hello", Capabilities: caps})
		s.flush()
		for _, ev := range p.Player.Sink.(*fakeSink).take() {
			if m, ok := ev.(event.EndgameMessage); ok && m.Message.Kind == "hello" {
				body, err := json.Marshal(m.Message)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(body), "riftEntrances") || strings.Contains(string(body), domain.EndgameRiftEntranceV1) {
					t.Fatalf("unnegotiated field/capability leaked: %s", body)
				}
			}
		}
	}
}
