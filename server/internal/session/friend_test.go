package session

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/online"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/scene"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/social"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/store"
)

// 好友的会话层测试。用内存 Store，不打真库 ——
// 关系表的 SQL 语义由 store 包的集成测试守着，这里测的是**编排**：
// 谁能加谁、请求什么时候作废、该通知谁。

type friendRig struct {
	t     *testing.T
	store *store.Memory
	idx   *online.Registry
	reqs  *social.Requests
}

func newFriendRig(t *testing.T) *friendRig {
	t.Helper()
	return &friendRig{t: t, store: store.NewMemory(),
		idx: online.NewRegistry(), reqs: social.NewRequests()}
}

// login 建一个角色、开一个会话、写进在线索引。
func (r *friendRig) login(name string, ent domain.EntityID, mapID int32) *Session {
	r.t.Helper()
	ctx := context.Background()
	acc, err := r.store.CreateAccount(ctx, "acc-"+name, "h")
	if err != nil {
		r.t.Fatalf("建账号: %v", err)
	}
	c := &domain.Character{AccountID: acc.ID, Name: name, Race: domain.Warrior,
		Level: 10, Pos: domain.Pos{MapID: mapID}}
	if err := r.store.CreateChar(ctx, c); err != nil {
		r.t.Fatalf("建角色: %v", err)
	}
	s := &Session{
		deps: Deps{Store: r.store, Online: r.idx, Requests: r.reqs, Log: slog.Default()},
		log:  slog.Default(),
	}
	s.char = c
	s.entity = ent
	s.scene = domain.SceneID{MapID: mapID}
	s.stage = StageInGame
	r.idx.Enter(online.Location{Char: domain.CharID(c.ID), Name: name,
		Entity: ent, Scene: domain.SceneID{MapID: mapID}})
	return s
}

func TestRequestThenAccept(t *testing.T) {
	r := newFriendRig(t)
	ctx := context.Background()
	甲 := r.login("甲", 11, 1)
	乙 := r.login("乙", 22, 1)

	if got := 甲.requestFriend(ctx, "乙"); got != domain.FriendOK {
		t.Fatalf("发请求被拒: %v", got)
	}
	// 请求挂在**收件人**那里
	if len(r.reqs.List(domain.CharID(乙.char.ID))) != 1 {
		t.Fatal("乙没收到请求")
	}

	if got := 乙.acceptFriend(ctx, domain.CharID(甲.char.ID)); got != domain.FriendOK {
		t.Fatalf("同意被拒: %v", got)
	}
	// 双向都要有
	for _, c := range []*Session{甲, 乙} {
		list := c.friendsOf(ctx, c.char.ID)
		if len(list) != 1 {
			t.Fatalf("%s 有 %d 个好友, 该是 1", c.char.Name, len(list))
		}
	}
	// 请求用掉了
	if len(r.reqs.List(domain.CharID(乙.char.ID))) != 0 {
		t.Fatal("同意之后请求还挂着")
	}
}

// 加好友是当面同意的事：给离线的人发请求，他上线时那个请求早就没了。
func TestCannotRequestOfflinePlayer(t *testing.T) {
	r := newFriendRig(t)
	甲 := r.login("甲", 11, 1)
	if got := 甲.requestFriend(context.Background(), "查无此人"); got != domain.FriendOffline {
		t.Fatalf("给离线的人发请求该拒, 得到 %v", got)
	}
}

func TestCannotRequestYourself(t *testing.T) {
	r := newFriendRig(t)
	甲 := r.login("甲", 11, 1)
	if got := 甲.requestFriend(context.Background(), "甲"); got != domain.FriendSelf {
		t.Fatalf("加自己该拒, 得到 %v", got)
	}
}

func TestAcceptWithoutRequestFails(t *testing.T) {
	r := newFriendRig(t)
	ctx := context.Background()
	甲 := r.login("甲", 11, 1)
	乙 := r.login("乙", 22, 1)

	if got := 乙.acceptFriend(ctx, domain.CharID(甲.char.ID)); got != domain.FriendNoRequest {
		t.Fatalf("没请求就同意该拒, 得到 %v", got)
	}
	if n := len(乙.friendsOf(ctx, 乙.char.ID)); n != 0 {
		t.Fatalf("凭空加上了 %d 个好友", n)
	}
}

func TestCannotAddTwice(t *testing.T) {
	r := newFriendRig(t)
	ctx := context.Background()
	甲 := r.login("甲", 11, 1)
	乙 := r.login("乙", 22, 1)

	甲.requestFriend(ctx, "乙")
	乙.acceptFriend(ctx, domain.CharID(甲.char.ID))

	if got := 甲.requestFriend(ctx, "乙"); got != domain.FriendAlready {
		t.Fatalf("重复加该拒, 得到 %v", got)
	}
}

func TestRemoveIsBidirectional(t *testing.T) {
	r := newFriendRig(t)
	ctx := context.Background()
	甲 := r.login("甲", 11, 1)
	乙 := r.login("乙", 22, 1)
	甲.requestFriend(ctx, "乙")
	乙.acceptFriend(ctx, domain.CharID(甲.char.ID))

	if got := 甲.removeFriend(ctx, domain.CharID(乙.char.ID)); got != domain.FriendOK {
		t.Fatalf("删好友被拒: %v", got)
	}
	// **两边都要没** —— 留着单边的话对方会一直收到我的上下线通知
	for _, c := range []*Session{甲, 乙} {
		if n := len(c.friendsOf(ctx, c.char.ID)); n != 0 {
			t.Fatalf("%s 删完还剩 %d 个好友", c.char.Name, n)
		}
	}
	// 不是好友就删不了
	if got := 甲.removeFriend(ctx, domain.CharID(乙.char.ID)); got != domain.FriendNotFriend {
		t.Fatalf("删非好友该拒, 得到 %v", got)
	}
}

// **该通知谁**是这条链路上容易错的那一半（通知本身等 opcode）。
func TestOnlineFriendsOnlyListsThoseActuallyOnline(t *testing.T) {
	r := newFriendRig(t)
	ctx := context.Background()
	甲 := r.login("甲", 11, 1)
	乙 := r.login("乙", 22, 285) // 在另一张图
	丙 := r.login("丙", 33, 1)

	for _, other := range []*Session{乙, 丙} {
		甲.requestFriend(ctx, other.char.Name)
		other.acceptFriend(ctx, domain.CharID(甲.char.ID))
	}

	got := 甲.onlineFriends(ctx, 甲.char)
	if len(got) != 2 {
		t.Fatalf("在线好友 %d 个, 该是 2", len(got))
	}
	// 跨图的也要通知到 —— 好友不是同场景的概念
	seen := map[int32]bool{}
	for _, l := range got {
		seen[l.Scene.MapID] = true
	}
	if !seen[1] || !seen[285] {
		t.Fatalf("只通知到了这些图: %v, 该跨图都算", seen)
	}

	// 跨图交接一成功，会话地址和全服在线索引必须在返回前一起指向
	// 目标图；不能等目标场景异步 flush Teleported，否则这个窗口里私聊/队伍
	// 消息会被投回已经没有该实体的源图。
	var logs bytes.Buffer
	甲.log = slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	from, to := domain.SceneID{MapID: 1}, domain.SceneID{MapID: 14}
	if !甲.transferScene(11, from, to, func() bool { return true }) {
		t.Fatal("目标 Enter 成功入队却交接失败")
	}
	if loc, ok := r.idx.Find(domain.CharID(甲.char.ID)); !ok || loc.Scene != to || loc.Entity != 11 {
		t.Fatalf("交接返回后在线索引仍未改绑: loc=%+v ok=%v", loc, ok)
	}
	// 目标场景随后才 flush 落地事件：rebind 只幂等确认新地址，不能把
	// 换图当成再次上线通知好友。
	甲.rebind(11, to)
	if strings.Contains(logs.String(), "好友状态通知") {
		t.Fatalf("跨图被误报成好友上线: %s", logs.String())
	}

	// 目标 Enter 拒绝时不得提前改会话或索引地址。
	failedTo := domain.SceneID{MapID: 15}
	if 甲.transferScene(11, to, failedTo, func() bool { return false }) {
		t.Fatal("目标 Enter 失败却报交接成功")
	}
	if loc, ok := r.idx.Find(domain.CharID(甲.char.ID)); !ok || loc.Scene != to {
		t.Fatalf("交接失败却改了在线索引: loc=%+v ok=%v", loc, ok)
	}
	甲.mu.Lock()
	bound := 甲.scene
	甲.mu.Unlock()
	if bound != to {
		t.Fatalf("交接失败却改了会话绑定: %+v", bound)
	}

	// 丙下线之后就不该在名单里了
	r.idx.Leave(domain.CharID(丙.char.ID), 33)
	got = 甲.onlineFriends(ctx, 甲.char)
	if len(got) != 1 || got[0].Name != "乙" {
		t.Fatalf("丙下线后名单是 %+v, 该只剩乙", got)
	}
}

// 好友列表满了的人不该被人不停地加 —— **两边的上限都要查**。
func TestTargetListFullIsRejected(t *testing.T) {
	r := newFriendRig(t)
	ctx := context.Background()
	甲 := r.login("甲", 11, 1)
	乙 := r.login("乙", 22, 1)

	// 把乙的列表灌满(直接写库, 不走请求流程)
	for i := 0; i < domain.MaxFriends; i++ {
		c := &domain.Character{AccountID: 1, Name: "路人" + string(rune('a'+i%26)) +
			string(rune('a'+i/26)), Race: domain.Warrior, Level: 1}
		if err := r.store.CreateChar(ctx, c); err != nil {
			t.Fatalf("建路人: %v", err)
		}
		if err := r.store.AddFriend(ctx, 乙.char.ID, c.ID); err != nil {
			t.Fatalf("灌好友: %v", err)
		}
	}
	if n, _ := r.store.CountFriends(ctx, 乙.char.ID); n != domain.MaxFriends {
		t.Fatalf("乙有 %d 个好友, 该灌满 %d", n, domain.MaxFriends)
	}

	if got := 甲.requestFriend(ctx, "乙"); got != domain.FriendTargetFull {
		t.Fatalf("对方列表满了该拒, 得到 %v", got)
	}
}

// 从发请求到点同意之间隔着人的反应时间，那段时间里可能已经加满了。
func TestLimitIsRecheckedOnAccept(t *testing.T) {
	r := newFriendRig(t)
	ctx := context.Background()
	甲 := r.login("甲", 11, 1)
	乙 := r.login("乙", 22, 1)

	if got := 甲.requestFriend(ctx, "乙"); got != domain.FriendOK {
		t.Fatalf("发请求被拒: %v", got)
	}
	// 请求发出之后, 乙在别处把自己加满了
	for i := 0; i < domain.MaxFriends; i++ {
		c := &domain.Character{AccountID: 1, Name: "后来的" + string(rune('a'+i%26)) +
			string(rune('a'+i/26)), Race: domain.Warrior, Level: 1}
		if err := r.store.CreateChar(ctx, c); err != nil {
			t.Fatal(err)
		}
		if err := r.store.AddFriend(ctx, 乙.char.ID, c.ID); err != nil {
			t.Fatal(err)
		}
	}

	if got := 乙.acceptFriend(ctx, domain.CharID(甲.char.ID)); got != domain.FriendListFull {
		t.Fatalf("同意时该重查上限, 得到 %v", got)
	}
	if 甲.alreadyFriends(ctx, 甲.char.ID, domain.CharID(乙.char.ID)) {
		t.Fatal("超了上限还是加上了")
	}
}

// ── 登出清理 ──
//
// OnClose 要做四件事：退队、通知好友、摘在线索引、清好友请求。
// 漏掉任何一件的表现都是"这个人断线了但全服还当他在线" ——
// 私聊发给他、队伍面板显示他、好友列表亮着，而他早就不在了。

// closeRig 在 friendRig 之上补一个真 Router（OnClose 要往场景投 Leave）。
func closeRig(t *testing.T) (*friendRig, *scene.Router, func()) {
	t.Helper()
	r := newFriendRig(t)
	frames := make(chan time.Time, 4)
	build := func(id domain.SceneID) (*scene.Scene, error) {
		return scene.New(scene.Config{ID: id, Frames: frames}), nil
	}
	router := scene.NewRouter(build, slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	router.Start(ctx)
	return r, router, func() { cancel(); router.Drain(time.Second) }
}

func TestOnCloseCleansUpEverything(t *testing.T) {
	r, router, stop := closeRig(t)
	defer stop()
	ctx := context.Background()

	甲 := r.login("甲", 11, 1)
	乙 := r.login("乙", 22, 1)
	甲.deps.Router = router
	乙.deps.Router = router

	// 甲乙成为好友, 丙给甲发了个请求(丙这里只用 id, 不必真登录)
	甲.requestFriend(ctx, "乙")
	乙.acceptFriend(ctx, domain.CharID(甲.char.ID))
	r.reqs.Add(domain.CharID(甲.char.ID), social.Request{From: 999, FromName: "丙"})
	// 甲也给乙发了个请求(用来验证"我发出去的也要撤回")
	r.reqs.Add(domain.CharID(乙.char.ID), social.Request{
		From: domain.CharID(甲.char.ID), FromName: "甲"})
	甲ID := domain.CharID(甲.char.ID)

	甲.OnClose("测试断线")

	// 1. 在线索引里没了 —— 否则私聊会一直投给一个不存在的会话
	if r.idx.Online(甲ID) {
		t.Fatal("断线了还在在线索引里")
	}
	// 乙不受影响
	if !r.idx.Online(domain.CharID(乙.char.ID)) {
		t.Fatal("顺手把别人也摘了")
	}
	// 2. 收件箱清空
	if n := len(r.reqs.List(甲ID)); n != 0 {
		t.Fatalf("断线后收件箱还剩 %d 条", n)
	}
	// 3. 自己发出去的请求要撤回 —— 留着的话乙点了同意, 而甲早就不在了
	for _, req := range r.reqs.List(domain.CharID(乙.char.ID)) {
		if req.From == 甲ID {
			t.Fatal("甲断线了, 他发给乙的请求还挂着 —— 乙点同意会什么都不发生")
		}
	}
	// 4. 好友**关系**不能被清掉 —— 那是持久数据, 断线不该删好友
	if n := len(乙.friendsOf(ctx, 乙.char.ID)); n != 1 {
		t.Fatalf("断线把好友关系也删了, 乙还剩 %d 个好友", n)
	}
}

// 没进游戏就断线（连上了没登录、或登录了没选角）不该出事。
// 扫端口的、连上就断的，每天都有一堆。
func TestOnCloseBeforeInGameIsSafe(t *testing.T) {
	r := newFriendRig(t)
	s := &Session{deps: Deps{Store: r.store, Online: r.idx, Requests: r.reqs,
		Log: slog.Default()}, log: slog.Default()}
	// stage 还是 Connected, char 是 nil, Router 也是 nil —— 真走下去就是空指针
	s.OnClose("还没登录就断了")
}

// 断线两次不该出事（网关重试关闭、或者读写两侧各报一次）。
func TestOnCloseIsIdempotent(t *testing.T) {
	r, router, stop := closeRig(t)
	defer stop()
	甲 := r.login("甲", 11, 1)
	甲.deps.Router = router
	甲ID := domain.CharID(甲.char.ID)

	甲.OnClose("第一次")
	甲.OnClose("第二次") // 这时 stage 已经是 Closed, 该直接返回
	if r.idx.Online(甲ID) {
		t.Fatal("还在索引里")
	}
}
