package session

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/scene"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/store"
)

var nativeUnavailableHardCommands = []string{
	"taoyu hard", "dungeonhard 20100", "dungeonhard 20300", "dungeonhard 20600",
	"dungeonhard 20701", "dungeonhard 20801", "dungeonhard 20901",
}

func unavailableHardSession(t *testing.T) (*Session, *capSink) {
	t.Helper()
	out := &capSink{}
	router := scene.NewRouter(func(id domain.SceneID) (*scene.Scene, error) {
		t.Errorf("unavailable notice tried to construct scene %v", id)
		return nil, fmt.Errorf("unexpected scene construction")
	}, slog.Default())
	s := New(out, Deps{Router: router, Log: slog.Default()})
	s.stage, s.entity, s.scene, s.mapEpoch = StageInGame, 9, domain.SceneID{MapID: 507}, 4
	s.account = &store.Account{ID: 3, Username: "ordinary", GMLevel: 0, Caiyu: 71}
	s.char = &domain.Character{ID: 12, AccountID: 3, Name: "ordinary", Level: 40,
		Pos: domain.Pos{MapID: 507, X: 393, Y: 621}, Money: domain.Money{Gold: 2, Silver: 3, Copper: 4}, Caiyu: 71}
	return s, out
}

func sendUnavailableHardCommand(s *Session, raw string) {
	s.OnPacket(0x1006, protocol.NewW(0).Str(raw).Bytes()[2:])
}

func assertUnavailableHardPacket(t *testing.T, out *capSink, wantText string) {
	t.Helper()
	out.mu.Lock()
	defer out.mu.Unlock()
	if len(out.pkts) != 1 {
		t.Fatalf("got %d packets, want only the native server-text notice", len(out.pkts))
	}
	// Check the real original wire layout independently of GMCommandText:
	// 0x8008, UTF-8 byte-length-prefixed string, displayMode=1, and no tail.
	want := binary.LittleEndian.AppendUint16(nil, 0x8008)
	want = binary.LittleEndian.AppendUint16(want, uint16(len(wantText)))
	want = append(want, []byte(wantText)...)
	want = append(want, 1)
	if !bytes.Equal(out.pkts[0], want) {
		t.Fatalf("native notice=%x want=%x", out.pkts[0], want)
	}
}

func assertUnavailableHardState(t *testing.T, s *Session, before []byte) {
	t.Helper()
	after, err := json.Marshal(s.char)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("notice changed character state: err=%v before=%s after=%s", err, before, after)
	}
	if s.scene != (domain.SceneID{MapID: 507}) || s.entity != 9 || s.mapEpoch != 4 ||
		s.awaitMapAck || s.clientMapReady != nil || s.account.Caiyu != 71 || s.deps.Router.Count() != 0 {
		t.Fatal("notice changed map ownership, currency, loading state, or opened a scene")
	}
}

func TestNativeHardDungeonUnavailableCommandsUseOriginalText(t *testing.T) {
	for _, raw := range nativeUnavailableHardCommands {
		t.Run(raw, func(t *testing.T) {
			s, out := unavailableHardSession(t)
			before, err := json.Marshal(s.char)
			if err != nil {
				t.Fatal(err)
			}
			sendUnavailableHardCommand(s, raw)
			assertUnavailableHardPacket(t, out, "困难模式暂不可用")
			assertUnavailableHardState(t, s, before)
		})
	}
}

func TestNativeHardDungeonUnavailableDoesNotGrantGM(t *testing.T) {
	for _, raw := range []string{"addmoney 1", "goto 20502", "item 3381 1"} {
		t.Run(raw, func(t *testing.T) {
			s, out := unavailableHardSession(t)
			before, _ := json.Marshal(s.char)
			sendUnavailableHardCommand(s, "taoyu hard")
			assertUnavailableHardPacket(t, out, "困难模式暂不可用")
			out.mu.Lock()
			out.pkts = nil
			out.mu.Unlock()
			sendUnavailableHardCommand(s, raw)
			assertUnavailableHardPacket(t, out, "权限不足：当前账号不是 GM")
			assertUnavailableHardState(t, s, before)
		})
	}
}

func TestNativeHardDungeonUnavailableRequiresWorldContext(t *testing.T) {
	contexts := []struct {
		name   string
		change func(*Session)
	}{
		{"connected", func(s *Session) { s.stage = StageConnected }},
		{"selection", func(s *Session) { s.stage = StageAuthed }},
		{"closed", func(s *Session) { s.stage = StageClosed }},
		{"no-account", func(s *Session) { s.account = nil }},
		{"no-character", func(s *Session) { s.char = nil }},
		{"zero-character", func(s *Session) { s.char.ID = 0 }},
		{"negative-character", func(s *Session) { s.char.ID = -1 }},
		{"no-entity", func(s *Session) { s.entity = 0 }},
		{"no-map", func(s *Session) { s.scene = domain.SceneID{} }},
		{"negative-map", func(s *Session) { s.scene.MapID = -1 }},
		{"loading", func(s *Session) { s.awaitMapAck = true }},
	}
	for _, context := range contexts {
		for _, raw := range nativeUnavailableHardCommands {
			t.Run(context.name+"/"+raw, func(t *testing.T) {
				s, out := unavailableHardSession(t)
				context.change(s)
				sendUnavailableHardCommand(s, raw)
				// Also protect callers which already decoded a command, including
				// the map-load interval between OnPacket's guard and dispatch.
				s.onGMCommand(raw)
				if len(out.pkts) != 0 || s.deps.Router.Count() != 0 {
					t.Fatal("invalid world context produced a reply or opened a scene")
				}
			})
		}
	}
}

func TestNativeHardDungeonUnavailableKeepsExactWhitelist(t *testing.T) {
	for _, raw := range []string{
		"taoyu", "taoyu normal", "taoyu Hard", "Taoyu hard", "taoyu hard extra", "taoyu  hard", "taoyu\thard",
		" taoyu hard", "taoyu hard ", "/taoyu hard", "gm taoyu hard", "/gm taoyu hard", "taoyu hard\x00",
		"dungeonhard", "dungeonhard 0", "dungeonhard -1", "dungeonhard 7", "dungeonhard 20502", "dungeonhard 20802",
		"dungeonhard +20100", "dungeonhard 020100", "dungeonhard 20100.0", "dungeonhard 20100 extra",
		"dungeonhard  20100", "dungeonhard\t20100", "Dungeonhard 20100", "dungeonhard 20100 ",
		"/dungeonhard 20100", "gm dungeonhard 20100", "dungeonhard 2147483648", "", strings.Repeat("x", maxGMCommandBytes+1),
	} {
		t.Run(raw, func(t *testing.T) {
			s, out := unavailableHardSession(t)
			before, _ := json.Marshal(s.char)
			sendUnavailableHardCommand(s, raw)
			if len(out.pkts) != 0 {
				t.Fatalf("non-native command %q was newly handled", raw)
			}
			assertUnavailableHardState(t, s, before)
		})
	}
	for _, payload := range [][]byte{nil, {1}, {0xff, 0xff}, {10, 0, 't'}} {
		s, out := unavailableHardSession(t)
		s.OnPacket(0x1006, payload)
		if len(out.pkts) != 0 || s.deps.Router.Count() != 0 {
			t.Fatal("malformed native packet reached notice/admission")
		}
	}
}

func TestNativeHardDungeonUnavailableDoesNotMutateActorInventory(t *testing.T) {
	r, s, out := mapReviewSession(t)
	s.account = &store.Account{ID: 3, GMLevel: 0, Caiyu: 71}
	inspect := func(fn func(*scene.Scene)) {
		t.Helper()
		done := make(chan struct{})
		if !r.router.Post(domain.SceneID{MapID: 1}, scene.Inspect{Fn: fn, Done: done}) {
			t.Fatal("source actor rejected inspection")
		}
		r.step(1)
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("source actor inspection timed out")
		}
	}
	var before domain.Snapshot
	inspect(func(sc *scene.Scene) {
		p := sc.EntityAt(1).Player
		p.Char.Money = domain.Money{Gold: 2, Silver: 3, Copper: 4}
		p.Bag.Set(0, domain.Stack{Item: 3381, Count: 3})
		before = (domain.Snapshot{Char: p.Char, Bag: p.Bag}).Clone()
	})
	count := r.router.Count()
	for _, raw := range nativeUnavailableHardCommands {
		sendUnavailableHardCommand(s, raw)
	}
	// Consume any accidentally posted actor work before comparing actual state.
	r.step(1)
	var after domain.Snapshot
	inspect(func(sc *scene.Scene) {
		if p := sc.EntityAt(1); p != nil {
			after = (domain.Snapshot{Char: p.Player.Char, Bag: p.Player.Bag}).Clone()
		}
	})
	if !reflect.DeepEqual(before, after) {
		t.Fatal("unavailable notice changed actual actor currency or inventory")
	}
	if r.router.Count() != count || mapReviewWireCount(out, protocol.SCTeleport) != 0 ||
		mapReviewWireCount(out, protocol.SCServerText) != len(nativeUnavailableHardCommands) {
		t.Fatal("unavailable commands opened an instance, teleported, or failed to give one notice each")
	}
	assertNativeOwner(t, s, domain.SceneID{MapID: 1})
}
