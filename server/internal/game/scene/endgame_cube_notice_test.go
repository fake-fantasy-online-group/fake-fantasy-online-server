package scene

import (
	"encoding/json"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	"os"
	"testing"
)

func TestCubeFailureNativeNoticeOnce(t *testing.T) {
	for _, kind := range []string{"cube.quote", "cube.execute"} {
		for _, tc := range []struct{ code, text string }{{"no_match", "鼎中诸物，未合炼方。"}, {"partial_match", "部分材料相合，尚需调配。"}, {"insufficient_count", "材料虽合，数目尚缺。"}} {
			t.Run(kind+"/"+tc.code, func(t *testing.T) {
				s, sink, _ := cubeSceneFixture(t)
				s.endgameReply(s.players[1], domain.EndgameResponse{Kind: kind, RequestID: "notice-fixture", Error: tc.code, Message: tc.text})
				replies, notices := 0, 0
				for _, ev := range cubeTake(s, sink) {
					switch e := ev.(type) {
					case event.EndgameMessage:
						replies++
						if e.Message.Error != tc.code || e.Message.Message != tc.text {
							t.Fatal(e)
						}
					case event.NewbieTextNotice:
						notices++
						if e.Who != 1 || e.Text != tc.text {
							t.Fatal(e)
						}
						if len(protocol.Encode(2, e)) != 0 {
							t.Fatal("notice leaked to another observer")
						}
						packets := protocol.Encode(1, e)
						if len(packets) != 1 {
							t.Fatal("missing native packet")
						}
						if dir := os.Getenv("CUBE_NOTICE_FIXTURE"); dir != "" {
							if err := os.MkdirAll(dir, 0700); err != nil {
								t.Fatal(err)
							}
							if err := os.WriteFile(dir+"/"+kind+"-"+tc.code+".bin", packets[0], 0600); err != nil {
								t.Fatal(err)
							}
						}
					case event.ServerNotice:
						t.Fatal("duplicate chat notice")
					}
				}
				if replies != 1 || notices != 1 {
					t.Fatalf("replies=%d notices=%d", replies, notices)
				}
			})
		}
	}
}
func TestCubeNativeNoticeIgnoresSuccessAndOtherOperations(t *testing.T) {
	for _, r := range []domain.EndgameResponse{{Kind: "cube.quote", OK: true, Message: "ok"}, {Kind: "cube.execute", OK: true, Message: "ok"}, {Kind: "snapshot", Error: "no_match", Message: "not cube"}, {Kind: "cube.quote", Error: "no_match"}} {
		s, sink, _ := cubeSceneFixture(t)
		s.endgameReply(s.players[1], r)
		for _, ev := range cubeTake(s, sink) {
			if _, ok := ev.(event.NewbieTextNotice); ok {
				b, _ := json.Marshal(r)
				t.Fatalf("unexpected notice %s", b)
			}
		}
	}
}

func TestCubeMatcherFailureReachesNativeNotice(t *testing.T) {
	s, sink, b := cubeSceneFixture(t)
	inputCubeNPC(s)
	p := s.players[1]
	s.items[1999999999] = domain.ItemDef{ID: 1999999999, Name: "无关材料", Stackable: true, InventoryTab: 1, InventoryTabKnown: true}
	p.Player.Bag.Set(102, domain.Stack{Item: 1999999999, Count: 1})
	req := inputCubeQuoteRequest()
	req.CubeInputs = []domain.CubeInput{{ItemID: 1999999999, Count: 1}}
	s.onEndgameCube(p, req)
	replies, notices := 0, 0
	for _, ev := range cubeTake(s, sink) {
		switch e := ev.(type) {
		case event.EndgameMessage:
			replies++
			if e.Message.Error != "no_match" {
				t.Fatal(e.Message)
			}
		case event.NewbieTextNotice:
			notices++
			if e.Text != "鼎中诸物，未合炼方。" {
				t.Fatal(e)
			}
		}
	}
	if replies != 1 || notices != 1 || b.commits != 0 || p.Player.Bag.At(102).Count != 1 {
		t.Fatalf("replies=%d notices=%d commits=%d", replies, notices, b.commits)
	}
}
