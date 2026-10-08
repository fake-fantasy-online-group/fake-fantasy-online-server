package session

import (
	"log/slog"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/scene"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/store"
)

// 跨图之后客户端还会把**已经算好的那条路**走完 —— 这是真实抓包里量到的，
// 不是假想: 2026-08-14 11:43 从 map41 走门到 map3，`0x803a` 落地之后 470ms 内
// 还收到一条 `0x100a(mapId=41, 5555,6796)` 和一条 `0x1017(5555,6796)`。
//
// 服务端当时照单全收，于是新图上的权威位置被写成了另一张图的坐标，
// 并且真的落进了 PostgreSQL(`map_id=3, pos=(5555,6796)`)。
// 落点若恰好在新图的门区里，还会当场再跨一次图。

// posOf 读某张图上某个实体的权威坐标。和 partyOf 一样必须走 Inspect ——
// 场景里的实体只有场景 goroutine 能碰。
func (r *twoMapRig) posOf(mapID int32, id domain.EntityID) domain.Pos {
	r.t.Helper()
	var got domain.Pos
	done := make(chan struct{})
	r.router.Post(domain.SceneID{MapID: mapID}, scene.Inspect{
		Fn: func(s *scene.Scene) {
			if e := s.EntityAt(id); e != nil {
				got = e.Pos
			}
		},
		Done: done,
	})
	r.frames[mapID] <- time.Now()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		r.t.Fatalf("%d 号图没响应", mapID)
	}
	return got
}

func TestStaleMoveAfterCrossMapIsDropped(t *testing.T) {
	r := newTwoMapRig(t)
	r.enter(1, 1, 100, "片姐司马")
	sink := &capSink{}

	s := &Session{
		deps:  Deps{Router: r.router, Log: slog.Default(), Store: store.NewMemory()},
		log:   slog.Default(),
		stage: StageInGame, entity: 1, scene: domain.SceneID{MapID: 1},
		char: &domain.Character{ID: 100, Name: "片姐司马"},
		sink: sink,
	}
	// 使用真实会话出口和跨场景交接，不能只改 Session 的地图而绕过场景屏障。
	r.router.Post(domain.SceneID{MapID: 1}, scene.Inspect{Fn: func(sc *scene.Scene) {
		sc.EntityAt(1).Player.Sink = &eventSink{out: sink, log: slog.Default(), sess: s, observer: 1}
	}, Done: make(chan struct{})})
	r.step(1)
	r.router.Post(domain.SceneID{MapID: 1}, scene.Teleport{
		ID: 1, To: domain.SceneID{MapID: 285}, At: domain.Pos{MapID: 285, X: 100, Y: 100},
	})
	r.step(1)
	r.step(285)

	landing := r.posOf(285, 1)

	// ① 旧图坐标 + 旧图号(0x100a 自带图号): 对不上, 丢。
	s.onMove(protocol.Request{Kind: protocol.ReqSavePos, Flag: 1, MapID: 1, X: 5555, Y: 6796})
	r.step(285)
	if got := r.posOf(285, 1); got != landing {
		t.Fatalf("旧图号的滞后上报被应用了: %+v, 落点应保持 %+v", got, landing)
	}

	// ② 同样是旧图坐标, 但 0x1017 不带图号 —— 无法自证是新图的位置,
	//    闸门期间照样丢。这一条是上面那次真实跨图里真正把库写坏的包。
	s.onMove(protocol.Request{Kind: protocol.ReqMove, X: 5555, Y: 6796})
	r.step(285)
	if got := r.posOf(285, 1); got != landing {
		t.Fatalf("不带图号的滞后上报被应用了: %+v, 落点应保持 %+v", got, landing)
	}
	// 近坐标不能靠超速校验碰巧挡住；缺图号、错误请求种类和 entered=0
	// 都不能触发 MapReady，也不能消耗目标场景的正常移动额度。
	for _, req := range []protocol.Request{
		{Kind: protocol.ReqMove, X: 110, Y: 110},
		{Kind: protocol.ReqSavePos, Flag: 1, X: 110, Y: 110},
		{Kind: protocol.ReqMove, Flag: 1, MapID: 285, X: 110, Y: 110},
	} {
		s.onMove(req)
		r.step(285)
		s.mu.Lock()
		awaiting := s.awaitMapAck
		s.mu.Unlock()
		if !awaiting || r.posOf(285, 1) != landing {
			t.Fatalf("非明确地图确认提前开闸或移动: %+v", req)
		}
	}

	// ③ 即使地图号正确，entered=0 仍然只是存点，不能提前开闸。
	s.onMove(protocol.Request{Kind: protocol.ReqSavePos, MapID: 285, X: 120, Y: 130})
	r.step(285)
	if got := r.posOf(285, 1); got != landing {
		t.Fatalf("未完成加载的存点提前生效: %+v", got)
	}
	s.onMove(protocol.Request{Kind: protocol.ReqSavePos, Flag: 1, MapID: 285, X: 100, Y: 100})
	r.step(285)
	s.mu.Lock()
	awaiting := s.awaitMapAck
	s.mu.Unlock()
	if awaiting {
		t.Fatal("目标地图 entered=1 未完成场景确认")
	}

	// ④ 开闸之后, 不带图号的普通移动必须恢复 —— 否则玩家在新图上会一直动不了。
	s.onMove(protocol.Request{Kind: protocol.ReqMove, X: 140, Y: 150})
	r.step(285)
	if got := r.posOf(285, 1); got.X != 140 || got.Y != 150 {
		t.Fatalf("开闸后的普通移动被误挡: %+v", got)
	}

	// 等待中的新传送留在服务端；前一轮确认不能放开后一轮的同图重建。
	r.router.Post(domain.SceneID{MapID: 285}, scene.Teleport{
		ID: 1, To: domain.SceneID{MapID: 1}, At: domain.Pos{MapID: 1, X: 100, Y: 100},
	})
	r.step(285)
	r.step(1)
	s.mu.Lock()
	previousEpoch := s.mapEpoch
	s.mu.Unlock()
	r.router.Post(domain.SceneID{MapID: 1}, scene.Teleport{
		ID: 1, To: domain.SceneID{MapID: 1}, At: domain.Pos{MapID: 1, X: 300, Y: 200},
	})
	r.step(1)
	if got := r.posOf(1, 1); got.X != 100 || got.Y != 100 {
		t.Fatalf("前一次切图未确认就执行了后续传送: %+v", got)
	}
	s.onMove(protocol.Request{Kind: protocol.ReqSavePos, Flag: 1, MapID: 1, X: 100, Y: 100})
	r.step(1)
	s.mu.Lock()
	awaiting, epoch := s.awaitMapAck, s.mapEpoch
	s.mu.Unlock()
	if !awaiting || epoch <= previousEpoch {
		t.Fatal("排队的同图传送没有开启新的确认周期")
	}
	r.router.PostLifecycle(domain.SceneID{MapID: 1}, scene.MapReady{ID: 1, Scene: domain.SceneID{MapID: 1}, Epoch: previousEpoch})
	r.step(1)
	s.mu.Lock()
	awaiting = s.awaitMapAck
	s.mu.Unlock()
	if !awaiting {
		t.Fatal("旧代次的场景确认放开了新切图")
	}
	s.onMove(protocol.Request{Kind: protocol.ReqSavePos, Flag: 1, MapID: 1, X: 300, Y: 200})
	r.step(1)
	if got := r.posOf(1, 1); got.X != 300 || got.Y != 200 {
		t.Fatalf("同图传送落点错误: %+v", got)
	}
}

// 同图改绑(初次入场、实体号变化)不能拉闸 —— 那会把正常移动挡在外面。
func TestSameMapRebindDoesNotGateMoves(t *testing.T) {
	r := newTwoMapRig(t)
	r.enter(1, 1, 100, "片姐司马")

	s := &Session{
		deps:  Deps{Router: r.router, Log: slog.Default()},
		log:   slog.Default(),
		stage: StageInGame, entity: 1, scene: domain.SceneID{MapID: 1},
		char: &domain.Character{ID: 100, Name: "片姐司马"},
	}
	s.rebind(1, domain.SceneID{MapID: 1})

	s.onMove(protocol.Request{X: 160, Y: 170}) // 0x1017, 不带图号
	r.step(1)
	if got := r.posOf(1, 1); got.X != 160 || got.Y != 170 {
		t.Fatalf("同图改绑之后普通移动被误挡: %+v", got)
	}
}

func TestClientReadyTransferWaitsForTargetOwnershipThenAllowsMovement(t *testing.T) {
	r := newTwoMapRig(t)
	r.enter(1, 1, 100, "片姐司马")
	sink := &capSink{}
	s := &Session{
		deps: Deps{Router: r.router, Log: slog.Default(), Store: store.NewMemory()},
		log:  slog.Default(), sink: sink,
		stage: StageInGame, entity: 1, scene: domain.SceneID{MapID: 1},
		char: &domain.Character{ID: 100, Name: "片姐司马"},
	}
	r.router.Post(domain.SceneID{MapID: 1}, scene.Inspect{Fn: func(sc *scene.Scene) {
		sc.EntityAt(1).Player.Sink = &eventSink{out: sink, log: slog.Default(), sess: s, observer: 1}
	}, Done: make(chan struct{})})
	r.step(1)
	// ClientMapReady is supplied by an authorized client-preloaded route. The
	// target must consume Enter before the session can forward further gameplay.
	r.router.Post(domain.SceneID{MapID: 1}, scene.Teleport{
		ID: 1, To: domain.SceneID{MapID: 285}, At: domain.Pos{MapID: 285, X: 100, Y: 100},
		ClientMapReady: true,
	})
	r.step(1)
	s.mu.Lock()
	awaiting, current, epoch := s.awaitMapAck, s.scene, s.mapEpoch
	s.mu.Unlock()
	if !awaiting || current.MapID != 285 || epoch == 0 {
		t.Fatal("handoff failed to gate requests before target ownership")
	}
	s.onMove(protocol.Request{Kind: protocol.ReqMove, X: 120, Y: 120})
	r.step(285)
	s.mu.Lock()
	awaiting, afterEpoch := s.awaitMapAck, s.mapEpoch
	s.mu.Unlock()
	if awaiting || afterEpoch != epoch {
		t.Fatal("target did not complete the already-ready handoff in the same epoch")
	}
	if got := r.posOf(285, 1); got.X != 100 || got.Y != 100 {
		t.Fatalf("movement sent during handoff reached the target: %+v", got)
	}
	if sink.sawOpcode(0x803a) {
		t.Fatal("preloaded client was instructed to rebuild the target again")
	}
	s.onMove(protocol.Request{Kind: protocol.ReqMove, X: 120, Y: 120})
	r.step(285)
	if got := r.posOf(285, 1); got.X != 120 || got.Y != 120 {
		t.Fatalf("preloaded client cannot move without a redundant acknowledgment: %+v", got)
	}
}

func TestTransferFailurePreservesMapConfirmationState(t *testing.T) {
	s := &Session{
		log: slog.Default(), stage: StageInGame, entity: 1,
		scene: domain.SceneID{MapID: 1}, mapEpoch: 7,
	}
	from, to := s.scene, domain.SceneID{MapID: 285}
	if s.transferScene(1, from, to, func() bool { return false }) {
		t.Fatal("failed target handoff reported success")
	}
	if s.scene != from || s.awaitMapAck || s.mapEpoch != 7 {
		t.Fatal("failed handoff changed the active scene or confirmation epoch")
	}
	if !s.transferScene(1, from, to, func() bool { return true }) {
		t.Fatal("valid handoff failed")
	}
	if s.scene != to || !s.awaitMapAck || s.mapEpoch != 8 {
		t.Fatal("successful handoff did not close the target gate immediately")
	}
	// The later Teleported callback sees an already-rebound target. It must not
	// open a second confirmation epoch or accidentally release the existing one.
	s.rebind(1, to)
	if !s.awaitMapAck || s.mapEpoch != 8 {
		t.Fatal("late rebind changed the handoff's confirmation state")
	}
}
