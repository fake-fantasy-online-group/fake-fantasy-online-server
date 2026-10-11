package scene

import (
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

func TestNativeStatsHelloRefreshPrecedesEchoAndSnapshot(t *testing.T) {
	s := newTestScene(t, nil)
	sink := join(t, s, 1, 100, "甲", 0, 0)
	sink.take()
	p := s.players[1]
	s.endgamePlayer(p).loaded = true
	for _, caps := range [][]string{{domain.EndgameV1, domain.EndgameNativeStatsV1}, {domain.EndgameV1, domain.EndgameNativeStatsV1}, {domain.EndgameV1}} {
		s.dispatchEndgame(p, domain.EndgameRequest{Op: "hello", Capabilities: caps})
		s.flush()
		events := sink.take()
		inv, hello, snapshot := -1, -1, -1
		for i, e := range events {
			switch v := e.(type) {
			case event.InventorySnapshot:
				inv = i
			case event.EndgameMessage:
				if v.Message.Kind == "hello" {
					hello = i
					if !v.Message.OK {
						t.Fatal(v)
					}
				}
				if v.Message.Kind == "snapshot" {
					snapshot = i
				}
			}
		}
		if inv < 0 || hello <= inv || snapshot <= hello {
			t.Fatalf("wrong refresh/echo/snapshot order %d %d %d: %+v", inv, hello, snapshot, events)
		}
		if p.Player.EndgameNativeStats != (len(caps) == 2) {
			t.Fatal("negotiated capability drift")
		}
	}
}
func TestNativeStatsHelloFailureDoesNotEnableNewRenderer(t *testing.T) {
	s := newTestScene(t, nil)
	sink := join(t, s, 1, 100, "甲", 0, 0)
	sink.take()
	p := s.players[1]
	s.endgamePlayer(p).loaded = true
	p.Player.Bag.Set(0, domain.Stack{Item: 987654, Count: 1})
	s.dispatchEndgame(p, domain.EndgameRequest{Op: "hello", Capabilities: []string{domain.EndgameNativeStatsV1}})
	if p.Player.EndgameNativeStats {
		t.Fatal("failed refresh enabled the renderer")
	}
	messages := endgameMessages(s, sink)
	if len(messages) != 1 || messages[0].OK || len(messages[0].Capabilities) != 0 {
		t.Fatal("failure advertised native stats", messages)
	}
}
func TestNativeStatsCapabilitySurvivesTransferButNotReconnect(t *testing.T) {
	source := newTestScene(t, nil)
	sink := join(t, source, 1, 100, "甲", 0, 0)
	sink.take()
	p := source.players[1]
	p.Player.EndgameNativeStats = true
	snap := source.snapshotOf(p).Clone()
	runtime := p.CapturePlayerRuntime(source.tick)
	target := newTestScene(t, nil)
	transferred := reconnectRiftPlayer(t, target, snap, 2, 0, &runtime, &fakeSink{})
	if !target.nativeEndgameStats(transferred) {
		t.Fatal("map transfer lost renderer without client rehello")
	}
	delete(target.endgamePlayers, transferred.ID)
	if !target.nativeEndgameStats(transferred) {
		t.Fatal("scene sidecar cleanup reset session capability")
	}
	fresh := newTestScene(t, nil)
	reconnected := reconnectRiftPlayer(t, fresh, snap, 3, 0, nil, &fakeSink{})
	if fresh.nativeEndgameStats(reconnected) {
		t.Fatal("new connection inherited unnegotiated capability")
	}
}
func TestNativeSpecialStatsMergeScopesAndPreserveOrdinaryTooltips(t *testing.T) {
	rows := mergedEndgameAffixes([]domain.Affix{{Attr: domain.AttrAtk, Value: 11}, {Attr: domain.AttrAtk, Value: 5, Mode: domain.ModePercent}, {Attr: domain.AttrAtkSpeed, Value: -10, Mode: domain.ModePercent}}, []domain.Affix{{Attr: domain.AttrAtk, Value: 7}, {Attr: domain.AttrAtk, Value: 8, Mode: domain.ModePercent}, {Attr: domain.AttrAtkSpeed, Value: -5, Mode: domain.ModePercent}})
	if len(rows) != 3 || rows[0].Value != 18 || rows[1].Value != 13 || rows[2].Value != -15 {
		t.Fatal("same-scope aggregation changed signs/modes", rows)
	}
	local := localEnhancementLine(domain.ItemLocalEnhancement{Attr: domain.AttrAtk, Percent: 150}, 10000)
	if local != "攻击强化 +150%（本件）" {
		t.Fatal("local scope became global", local)
	}
	s, _, _ := astralSceneFixture(t)
	owner := s.players[1]
	def := s.items[999001]
	plain := domain.NewStack(def, 1)
	legacy := s.ownedItemTooltip(def, plain, owner)
	owner.Player.EndgameNativeStats = true
	if s.ownedItemTooltip(def, plain, owner) != legacy {
		t.Fatal("ordinary tooltip changed after negotiation")
	}
	rune, _ := domain.AstralRune(domain.FirstAstralRune)
	plain.SocketCount = 2
	plain.Sockets[0] = rune.Item
	plain.SocketUIDs[0] = 22
	plain.SocketCards[0] = s.items[rune.Item].CardDefaults
	owner.Player.EndgameNativeStats = false
	legacy = s.ownedItemTooltip(def, plain, owner)
	owner.Player.EndgameNativeStats = true
	if s.ownedItemTooltip(def, plain, owner) != legacy {
		t.Fatal("unformed rune tooltip changed after negotiation")
	}
	recipe, _ := domain.AstralRecipeByID(1)
	formed := plain
	formed.SocketCount = uint8(len(recipe.Runes))
	formed.Endgame.FormationRollBP = 12000
	for i, id := range recipe.Runes {
		formed.Sockets[i] = id
		formed.SocketUIDs[i] = int64(50 + i)
		formed.SocketCards[i] = s.items[id].CardDefaults
	}
	if _, ok := domain.MatchAstralRecipe(def, formed); !ok {
		t.Fatal("formation fixture")
	}
	// A different viewer controls formatting even when the owner is negotiated.
	viewerSink := join(t, s, 2, 101, "乙", 0, 0)
	viewerSink.take()
	viewer := s.players[2]
	owner.Player.EndgameNativeStats = true
	viewer.Player.EndgameNativeStats = false
	legacy = s.itemTooltipForViewer(def, formed, owner, viewer)
	viewer.Player.EndgameNativeStats = true
	native := s.itemTooltipForViewer(def, formed, owner, viewer)
	if native == legacy || !strings.Contains(native, "\x034孔数") {
		t.Fatal("viewer capability not respected", native, legacy)
	}
	owner.Player.Worn.Set(domain.EquipSlot(def.Equip.Slot), formed)
	broken := formed
	broken.Durability = 0
	owner.Player.Worn.Set(domain.EquipSlot(def.Equip.Slot), broken)
	// Broken/suppressed items retain potential lines; combat is still gated.
	if _, include := s.tooltipFormation(def, broken, owner); !include {
		t.Fatal("inactive preview hid its intrinsic potential")
	}
}
