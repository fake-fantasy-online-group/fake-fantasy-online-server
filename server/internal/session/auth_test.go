package session

import (
	"context"
	"encoding/binary"
	"errors"
	"log/slog"
	"sync"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/store"
)

// 登录状态机的**安全边界**测试。
//
// 这一层出问题不是"功能不好用"，是"能玩别人的号" ——
// 而且它们全是"什么都不做"的分支，功能测试永远碰不到：
// 拦住了不会有任何现象，没拦住也要等到有人拿别人的角色名进来才暴露。
//
// 覆盖的四条：
//
//	未认证就选角      account == nil     → 不进
//	角色不属于本账号   AccountID 不匹配    → 不进（这条是账号接管边界）
//	封禁账号          Banned             → 不认证
//	没进游戏就移动     stage != InGame    → 忽略

// capSink 记下所有下行包。
type capSink struct {
	mu     sync.Mutex
	pkts   [][]byte
	closed bool
}

func (c *capSink) Send(inner []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pkts = append(c.pkts, append([]byte(nil), inner...))
}
func (c *capSink) Close()           { c.mu.Lock(); c.closed = true; c.mu.Unlock() }
func (c *capSink) SessionID() int64 { return 1 }

// respCode 取最后一个登录响应(0x8002)的结果码。第二个返回值 false 表示没发过。
func (c *capSink) respCode() (int32, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := len(c.pkts) - 1; i >= 0; i-- {
		p := c.pkts[i]
		if len(p) >= 6 && binary.LittleEndian.Uint16(p) == 0x8002 {
			return int32(binary.LittleEndian.Uint32(p[2:])), true
		}
	}
	return 0, false
}

func (c *capSink) sawOpcode(op uint16) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, p := range c.pkts {
		if len(p) >= 2 && binary.LittleEndian.Uint16(p) == op {
			return true
		}
	}
	return false
}

// authRig 是一个带内存库的会话。
type authRig struct {
	t    *testing.T
	st   *store.Memory
	sink *capSink
	sess *Session
}

func newAuthRig(t *testing.T) *authRig {
	t.Helper()
	st := store.NewMemory()
	sink := &capSink{}
	s := &Session{
		deps: Deps{Store: st, Log: slog.Default()},
		sink: sink,
		log:  slog.Default(),
	}
	return &authRig{t: t, st: st, sink: sink, sess: s}
}

// mkAccountWithChar 建一个账号和它名下的一个角色。
func (r *authRig) mkAccountWithChar(accName, charName string) (*store.Account, *domain.Character) { //nolint:unparam
	r.t.Helper()
	ctx := context.Background()
	acc, err := r.st.CreateAccount(ctx, accName, "")
	if err != nil {
		r.t.Fatalf("建账号: %v", err)
	}
	c := &domain.Character{AccountID: acc.ID, Name: charName, Race: domain.Warrior,
		Level: 10, Pos: domain.Pos{MapID: 7, X: 100, Y: 100}}
	if err := r.st.CreateChar(ctx, c); err != nil {
		r.t.Fatalf("建角色: %v", err)
	}
	return acc, c
}

func (r *authRig) stage() Stage {
	r.sess.mu.Lock()
	defer r.sess.mu.Unlock()
	return r.sess.stage
}

// ── 认证 ──

func TestLoginAdvancesStage(t *testing.T) {
	r := newAuthRig(t)
	r.mkAccountWithChar("alice", "甲")

	if r.stage() != StageConnected {
		t.Fatal("初始阶段不是 Connected")
	}
	r.sess.onLogin(protocol.Request{S1: "alice", S2: "test-password"})

	if r.stage() != StageAuthed {
		t.Fatalf("登录后阶段是 %v, 该是 Authed", r.stage())
	}
	if code, ok := r.sink.respCode(); !ok || code != 0 {
		t.Fatalf("登录响应码 %d(发过没: %v), 该是 0", code, ok)
	}
	if !r.sink.sawOpcode(0x8004) {
		t.Fatal("没下发角色列表")
	}
}

// **封禁的账号不许通过认证。**
//
// 只发一个拒绝包是不够的 —— 阶段必须停在 Connected，
// 否则他接着发选角包就能进游戏（客户端不发登录包也拦不住他）。
func TestBannedAccountDoesNotAdvanceStage(t *testing.T) {
	r := newAuthRig(t)
	r.mkAccountWithChar("bad", "坏人")
	// 注意: 全项目**没有任何地方写 Banned**, 封禁靠直接改库。
	// SetBanned 只在内存测试替身上有, 见 store/memory.go 的说明
	r.st.SetBanned("bad", true)

	r.sess.onLogin(protocol.Request{S1: "bad", S2: "test-password"})

	if code, ok := r.sink.respCode(); !ok || code != 2 {
		t.Fatalf("封禁响应码 %d, 该是 2", code)
	}
	if got := r.stage(); got != StageConnected {
		t.Fatalf("封禁账号推进到了 %v —— 他接着发选角包就能进游戏", got)
	}
	if r.sink.sawOpcode(0x8004) {
		t.Fatal("给封禁账号下发了角色列表")
	}
}

// ── 选角：两条账号接管边界 ──

// 没登录就选角，必须什么都不做。
func TestEnterWithoutAuthIsRefused(t *testing.T) {
	r := newAuthRig(t)
	_, c := r.mkAccountWithChar("alice", "甲")

	r.sess.onEnter(protocol.Request{S1: "甲"})

	if got := r.stage(); got != StageConnected {
		t.Fatalf("未认证却推进到了 %v", got)
	}
	r.sess.mu.Lock()
	entered := r.sess.char != nil
	r.sess.mu.Unlock()
	if entered {
		t.Fatalf("未认证就绑上了角色 %s", c.Name)
	}
}

// **拿别人的角色名进游戏必须被拒。** 这条是账号接管边界：
// 角色名是公开的（别人站在你旁边就看得见），拦不住就等于谁都能玩你的号。
func TestEnterSomeoneElsesCharacterIsRefused(t *testing.T) {
	r := newAuthRig(t)
	r.mkAccountWithChar("alice", "甲")
	_, 别人的 := r.mkAccountWithChar("bob", "乙")

	// 用 alice 登录
	r.sess.onLogin(protocol.Request{S1: "alice", S2: "test-password"})
	if r.stage() != StageAuthed {
		t.Fatal("登录没成功, 测试前提不成立")
	}
	// 然后报 bob 的角色名
	r.sess.onEnter(protocol.Request{S1: 别人的.Name})

	r.sess.mu.Lock()
	got := r.sess.char
	stage := r.sess.stage
	r.sess.mu.Unlock()
	if got != nil {
		t.Fatalf("绑上了别人的角色 %s —— 谁都能玩你的号了", got.Name)
	}
	if stage == StageInGame {
		t.Fatal("拿别人的角色名进了游戏")
	}
}

// 角色不存在也一样什么都不做（不能因为查不到就当成"新角色"放进去）。
func TestEnterNonexistentCharacterIsRefused(t *testing.T) {
	r := newAuthRig(t)
	r.mkAccountWithChar("alice", "甲")
	r.sess.onLogin(protocol.Request{S1: "alice", S2: "test-password"})

	r.sess.onEnter(protocol.Request{S1: "查无此角色"})

	r.sess.mu.Lock()
	defer r.sess.mu.Unlock()
	if r.sess.char != nil {
		t.Fatal("凭一个不存在的名字绑上了角色")
	}
	if r.sess.stage == StageInGame {
		t.Fatal("凭一个不存在的名字进了游戏")
	}
}

// ── 阶段拦截 ──

// 同一 TCP 已经在世界里时，重复 Login/Enter 不能覆盖当前绑定。rig 每轮只退 UI
// 到登录页而未关 socket 时曾真实触发过这条路径：旧实体留在场景，新实体收到旧人的
// 0x8023。阶段机必须在访问存储或分配新 id 之前拦住。
func TestInGameRejectsSecondLoginAndEnter(t *testing.T) {
	r := newAuthRig(t)
	acc, c := r.mkAccountWithChar("alice", "甲")
	r.sess.mu.Lock()
	r.sess.account = acc
	r.sess.char = c
	r.sess.entity = 77
	r.sess.scene = domain.SceneID{MapID: 7}
	r.sess.stage = StageInGame
	r.sess.mu.Unlock()

	r.sess.onLogin(protocol.Request{S1: "另一个账号"})
	// Router 故意为 nil：若阶段守卫失效，onEnter 最终会分配实体并在这里 panic。
	r.sess.onEnter(protocol.Request{S1: c.Name})

	r.sess.mu.Lock()
	defer r.sess.mu.Unlock()
	if r.sess.stage != StageInGame || r.sess.account != acc || r.sess.char != c ||
		r.sess.entity != 77 || r.sess.scene.MapID != 7 {
		t.Fatalf("重复登录/进世界覆盖了旧绑定: stage=%v entity=%d scene=%v",
			r.sess.stage, r.sess.entity, r.sess.scene)
	}
}

// 0x1049 是不断 TCP 的主动退世界。第一次应原子解绑并回到 Authed；重复包必须
// 无副作用，随后真实断线仍能把状态推进 Closed。
func TestLeaveWorldReturnsToAuthedAndIsIdempotent(t *testing.T) {
	r := newAuthRig(t)
	acc, c := r.mkAccountWithChar("alice", "甲")
	r.sess.mu.Lock()
	r.sess.account = acc
	r.sess.char = c
	r.sess.entity = 77
	r.sess.scene = domain.SceneID{MapID: 7}
	r.sess.stage = StageInGame
	r.sess.mu.Unlock()

	r.sess.OnPacket(0x1049, nil)
	r.sess.OnPacket(0x1049, nil)
	r.sess.mu.Lock()
	if r.sess.stage != StageAuthed || r.sess.char != nil || r.sess.entity != 0 ||
		r.sess.scene != (domain.SceneID{}) {
		r.sess.mu.Unlock()
		t.Fatalf("退世界后绑定未清干净: stage=%v char=%v entity=%d scene=%v",
			r.sess.stage, r.sess.char, r.sess.entity, r.sess.scene)
	}
	r.sess.mu.Unlock()

	r.sess.OnClose("随后断线")
	if got := r.stage(); got != StageClosed {
		t.Fatalf("退世界后断线阶段 = %v, 该是 Closed", got)
	}
}

// 没进游戏就发移动包，必须忽略而不是 panic。
//
// 这条最容易被漏掉：Router 是 nil 的时候，少一个阶段判断就是空指针崩溃 ——
// 而崩的是整个网关 goroutine。
func TestMoveBeforeInGameIsIgnored(t *testing.T) {
	r := newAuthRig(t)
	// deps.Router 故意是 nil: 真被放行的话这里会崩
	r.sess.onMove(protocol.Request{X: 100, Y: 200})

	r.sess.onLogin(protocol.Request{S1: "alice", S2: "test-password"})
	r.sess.onMove(protocol.Request{X: 100, Y: 200}) // 认证了但还没进游戏
	// 不崩就算过
}

// ── 未知 opcode ──

// 认不出来的包要记下来，不能静默丢掉 —— 那张表是补协议映射的原始材料。
func TestUnknownPacketIsRecorded(t *testing.T) {
	r := newAuthRig(t)
	unknown := protocol.NewUnknownLog()
	r.sess.deps.Unknown = unknown

	r.sess.OnPacket(0x1234, []byte{1, 2, 3})
	r.sess.OnPacket(0x1234, []byte{1, 2, 3, 4})
	r.sess.OnPacket(0x5678, nil)

	if unknown.Len() != 2 {
		t.Fatalf("记了 %d 种未知 opcode, 该是 2", unknown.Len())
	}
	snap := unknown.Snapshot()
	if snap[0].Op != 0x1234 || snap[0].Count != 2 {
		t.Fatalf("0x1234 记了 %d 次", snap[0].Count)
	}
	// 长度区间是判断包结构的主要线索
	if snap[0].MinLen != 3 || snap[0].MaxLen != 4 {
		t.Fatalf("长度区间 %d~%d, 该是 3~4", snap[0].MinLen, snap[0].MaxLen)
	}
}

// 没接观测器时也不能崩 —— 测试与工具里的会话常常不带它。
func TestUnknownPacketWithoutLoggerDoesNotPanic(t *testing.T) {
	r := newAuthRig(t)
	r.sess.deps.Unknown = nil
	r.sess.OnPacket(0x1234, []byte{1})
}

// 已注册但载荷解坏的包，走的是另一条路（报"载荷解析失败"而不是"未接入"）——
// 那是我们的格式理解错了，比"不认识这个号"严重得多。
func TestMalformedKnownPacketIsNotCountedAsUnknown(t *testing.T) {
	r := newAuthRig(t)
	unknown := protocol.NewUnknownLog()
	r.sess.deps.Unknown = unknown

	// 0x100a 是移动包, 需要至少 12 字节; 给 3 字节让它解失败
	r.sess.OnPacket(0x100a, []byte{1, 2, 3})

	if unknown.Len() != 0 {
		t.Fatal("已注册的包解坏了却被记成'未接入的协议' —— 两件事的处理方式完全不同")
	}
}

// ── 存档读失败就不让进 ──

// failingStore 让某一次读操作报错，其余照常。
type failingStore struct {
	store.Store
	failBag bool
}

func (f *failingStore) LoadBag(ctx context.Context, charID int64, slots int) (*domain.Bag, error) {
	if f.failBag {
		return nil, errors.New("模拟数据库抖动")
	}
	return f.Store.LoadBag(ctx, charID, slots)
}

// 背包读不出来就不让进 —— 顶着一个空背包进游戏，玩家会以为东西丢了，
// 然后往里放新东西，那才是真的把旧的覆盖掉。
func TestEnterRefusedWhenBagLoadFails(t *testing.T) {
	r := newAuthRig(t)
	r.mkAccountWithChar("alice", "甲")
	r.sess.deps.Store = &failingStore{Store: r.st, failBag: true}

	r.sess.onLogin(protocol.Request{S1: "alice", S2: "test-password"})
	r.sess.onEnter(protocol.Request{S1: "甲"})

	r.sess.mu.Lock()
	defer r.sess.mu.Unlock()
	if r.sess.stage == StageInGame {
		t.Fatal("背包读失败还是把人放进去了 —— 他会以为东西丢了")
	}
	if code, ok := r.sink.respCode(); !ok || code != 1 {
		t.Fatalf("该回一个失败码, 得到 %d", code)
	}
}
