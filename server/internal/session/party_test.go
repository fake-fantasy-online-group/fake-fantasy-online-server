package session

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/online"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/party"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/scene"
)

// 这个文件测的是上一轮留下的那个缺口：**队伍变更能不能送到另一张图上的队友。**
//
// 以前 pushPartyID 只能刷新本连接自己 —— 别人在哪张图、实体 id 多少，
// 会话层手上没这张表。补上在线索引之后才有地址可投。
//
// 用真的 Router + 真的场景 goroutine 跑，节拍由测试给。

// twoMapRig 起两张图，各跑一个真场景。
type twoMapRig struct {
	t      *testing.T
	router *scene.Router
	frames map[int32]chan time.Time
}

func newTwoMapRig(t *testing.T) *twoMapRig {
	t.Helper()
	r := &twoMapRig{t: t, frames: map[int32]chan time.Time{
		1: make(chan time.Time), 285: make(chan time.Time)}}

	build := func(id domain.SceneID) (*scene.Scene, error) {
		ch, ok := r.frames[id.MapID]
		if !ok {
			t.Fatalf("测试没准备 %d 号图的节拍", id.MapID)
		}
		return scene.New(scene.Config{ID: id, Frames: ch}), nil
	}
	router := scene.NewRouter(build, slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	router.Start(ctx)
	t.Cleanup(func() { cancel(); router.Drain(time.Second) })
	r.router = router
	return r
}

// step 推一帧并等它跑完。**每张图一条独立的 channel** ——
// 共用一条的话某一帧会被随便哪个场景抢走，测试会偶发失败(以前踩过)。
func (r *twoMapRig) step(mapID int32) {
	r.t.Helper()
	sc := domain.SceneID{MapID: mapID}
	r.frames[mapID] <- time.Now()

	done := make(chan struct{})
	if !r.router.Post(sc, scene.Inspect{Fn: func(*scene.Scene) {}, Done: done}) {
		r.t.Fatalf("往 %d 号图投同步点失败", mapID)
	}
	r.frames[mapID] <- time.Now()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		r.t.Fatalf("%d 号图没响应", mapID)
	}
}

// enter 把一个角色放进某张图。
func (r *twoMapRig) enter(mapID int32, id domain.EntityID, charID int64, name string) {
	r.t.Helper()
	sc := domain.SceneID{MapID: mapID}
	if _, err := r.router.Ensure(sc); err != nil {
		r.t.Fatalf("建 %d 号图失败: %v", mapID, err)
	}
	ok := r.router.Post(sc, scene.Enter{
		ID: id,
		Char: &domain.Character{ID: charID, Name: name, Level: 10,
			Pos: domain.Pos{MapID: mapID, X: 100, Y: 100}},
		Sink: &nullSink{},
	})
	if !ok {
		r.t.Fatal("投递 Enter 失败")
	}
	r.step(mapID)
}

// partyOf 读某张图上某个实体的队伍号。走 Inspect —— 场景里的实体
// 只有场景 goroutine 能碰，测试线程直接读就是数据竞争。
func (r *twoMapRig) partyOf(mapID int32, id domain.EntityID) domain.PartyID {
	r.t.Helper()
	var got domain.PartyID
	done := make(chan struct{})
	r.router.Post(domain.SceneID{MapID: mapID}, scene.Inspect{
		Fn: func(s *scene.Scene) {
			if e := s.EntityAt(id); e != nil && e.Player != nil {
				got = e.Player.Party
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

// nullSink 是不关心下行字节的出口。本文件只看场景状态。
type nullSink struct{}

func (nullSink) Emit(event.Event) {}
func (nullSink) Close()           {}

// **这是本轮的核心断言**：甲在 1 号图、乙在 285 号图，
// 甲这个会话推一次队伍号，乙那张图上的实体要真的变。
func TestPartyIDReachesTeammateInAnotherMap(t *testing.T) {
	r := newTwoMapRig(t)
	idx := online.NewRegistry()

	r.enter(1, 11, 100, "甲")
	r.enter(285, 22, 200, "乙")
	idx.Enter(online.Location{Char: 100, Name: "甲", Entity: 11,
		Scene: domain.SceneID{MapID: 1}})
	idx.Enter(online.Location{Char: 200, Name: "乙", Entity: 22,
		Scene: domain.SceneID{MapID: 285}})

	s := &Session{deps: Deps{Router: r.router, Online: idx, Log: slog.Default()},
		log: slog.Default()}

	s.pushPartyID(200, 5) // 甲的会话去刷新乙的队伍号
	r.step(285)

	if got := r.partyOf(285, 22); got != 5 {
		t.Fatalf("另一张图上的队友队伍号是 %d, 该是 5 —— 跨场景没送到", got)
	}
	// 没碰甲
	if got := r.partyOf(1, 11); got != 0 {
		t.Fatalf("顺手把甲的队伍号改成了 %d", got)
	}
}

// 人下线了就查不到地址，什么都不做即可 —— 不能 panic，也不能投给 0 号场景。
func TestPushPartyIDToOfflineIsANoop(t *testing.T) {
	r := newTwoMapRig(t)
	idx := online.NewRegistry()
	r.enter(1, 11, 100, "甲")

	s := &Session{deps: Deps{Router: r.router, Online: idx, Log: slog.Default()},
		log: slog.Default()}
	s.pushPartyID(999, 5) // 999 号从没上线过

	r.step(1)
	if got := r.partyOf(1, 11); got != 0 {
		t.Fatalf("投给不存在的人却改到了别人: %d", got)
	}
}

// 退队时队伍里剩下的每个人都要收到新的队伍号，**哪怕他在别的图上**。
func TestLeavePartyRefreshesRemainingMembersAcrossMaps(t *testing.T) {
	r := newTwoMapRig(t)
	idx := online.NewRegistry()
	reg := party.NewRegistry()

	// 三个人: 甲(队长)在 1 号图, 乙丙在 285 号图
	r.enter(1, 11, 100, "甲")
	r.enter(285, 22, 200, "乙")
	r.enter(285, 33, 300, "丙")
	for _, m := range []struct {
		char domain.CharID
		name string
		ent  domain.EntityID
		mp   int32
	}{{100, "甲", 11, 1}, {200, "乙", 22, 285}, {300, "丙", 33, 285}} {
		idx.Enter(online.Location{Char: m.char, Name: m.name, Entity: m.ent,
			Scene: domain.SceneID{MapID: m.mp}})
	}
	reg.Invite(100, 200)
	reg.Accept(200, "乙", 100, "甲")
	reg.Invite(100, 300)
	p, _ := reg.Accept(300, "丙", 100, "甲")

	s := &Session{deps: Deps{Router: r.router, Online: idx, Party: reg,
		Log: slog.Default()}, log: slog.Default()}

	// 先把三个人的队伍号都推下去
	for _, m := range p.Members {
		s.pushPartyID(m.Char, p.ID)
	}
	r.step(1)
	r.step(285)
	if got := r.partyOf(285, 22); got != p.ID {
		t.Fatalf("乙的队伍号是 %d, 该是 %d", got, p.ID)
	}

	// 甲退队 —— 乙丙还在另一张图上, 他们的队伍号不该变(队伍还在)
	s.leaveParty(100)
	r.step(285)
	if got := r.partyOf(285, 22); got != p.ID {
		t.Fatalf("队长走后乙的队伍号变成了 %d, 队伍还在就不该变", got)
	}

	// 乙再退 —— 只剩丙一个, 队伍解散, 丙的号必须清零
	s.leaveParty(200)
	r.step(285)
	if got := r.partyOf(285, 33); got != 0 {
		t.Fatalf("队伍解散了丙还挂着队伍号 %d —— "+
			"他之后所有的队友判定都会失败得莫名其妙", got)
	}
}
