package session

import (
	"context"
	"encoding/binary"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/scene"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
)

// 这个文件测的是**竖切的接缝**: 场景发出游戏事件 → eventSink 编码 → 连接收到字节。
//
// 上下两层各自都有测试(scene 测行为, protocol 测字节), 但那两个测试之间有条缝:
// 场景真的会发出协议层认识的那些事件吗? 只有把它们接起来跑一遍才知道。
//
// 用真的 Run 循环, 但节拍由测试给 —— 逻辑帧的好处就在这, 不用 sleep。

// byteSink 是假的网关连接, 把发出去的字节留下来。
// 场景 goroutine 写、测试 goroutine 读, 所以要加锁。
type byteSink struct {
	mu   sync.Mutex
	pkts [][]byte
}

func (b *byteSink) Send(inner []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pkts = append(b.pkts, append([]byte(nil), inner...))
}
func (b *byteSink) Close()           {}
func (b *byteSink) SessionID() int64 { return 1 }

func (b *byteSink) reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pkts = nil
}

// opcodes 返回收到的所有包的 opcode, 便于断言下发序列。
func (b *byteSink) opcodes() []uint16 {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]uint16, 0, len(b.pkts))
	for _, p := range b.pkts {
		out = append(out, binary.LittleEndian.Uint16(p))
	}
	return out
}

func (b *byteSink) find(op uint16) []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, p := range b.pkts {
		if binary.LittleEndian.Uint16(p) == op {
			return p
		}
	}
	return nil
}

// rig 是一个跑着的场景 + 手动节拍。
type rig struct {
	t      *testing.T
	scene  *scene.Scene
	frames chan time.Time
}

func newRig(t *testing.T) *rig {
	t.Helper()
	frames := make(chan time.Time)
	s := scene.New(scene.Config{ID: domain.SceneID{MapID: 7}, Frames: frames})
	ctx, cancel := context.WithCancel(context.Background())
	go s.Run(ctx)
	t.Cleanup(cancel)
	return &rig{t: t, scene: s, frames: frames}
}

// step 推进一帧, 并等这一帧真的跑完。
//
// 等待用的是 Inspect 而不是 sleep: 投一条 Inspect 再推一帧, Inspect 回来就说明
// 前面那帧的所有效果都已经发生了。这样测试既快又不会偶发失败。
func (r *rig) step() {
	r.t.Helper()
	r.frames <- time.Now()

	done := make(chan struct{})
	if !r.scene.Post(scene.Inspect{Fn: func(*scene.Scene) {}, Done: done}) {
		r.t.Fatal("投递同步点失败")
	}
	r.frames <- time.Now()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		r.t.Fatal("场景没响应 —— 循环可能卡住了")
	}
}

func (r *rig) enter(id domain.EntityID, charID int64, name string, x, y float64) *byteSink {
	out := &byteSink{}
	r.scene.Post(scene.Enter{
		ID: id,
		Char: &domain.Character{ID: charID, Name: name, Level: 3,
			Pos: domain.Pos{MapID: 7, X: x, Y: y}},
		Sink: &eventSink{out: out, log: slog.Default(), observer: id},
	})
	return out
}

// 一个玩家进图: 先进游戏确认, 再自身数据。
// 顺序是有讲究的 —— 客户端要先有"我"才能把别人摆到我周围。
func TestEnterProducesSelfPackets(t *testing.T) {
	r := newRig(t)
	out := r.enter(1, 100, "甲", 100, 100)
	r.step()

	ops := out.opcodes()
	if len(ops) < 2 {
		t.Fatalf("进图应至少下发 2 个包, 实际 %d: %#x", len(ops), ops)
	}
	if ops[0] != 0x8033 {
		t.Errorf("第一个包应是进游戏确认 0x8033, 实际 %#x", ops[0])
	}
	if ops[1] != protocol.SCSelfData {
		t.Errorf("第二个包应是自身数据 0x8003, 实际 %#x", ops[1])
	}
}

// 第二个玩家进来, 第一个人收到一个 0x8023。
// 这一条把"场景的 AOI 广播"和"协议的字节布局"串起来验了。
func TestSecondPlayerAppearsAsSpawnPacket(t *testing.T) {
	r := newRig(t)
	outA := r.enter(1, 100, "甲", 100, 100)
	r.step()
	outA.reset()

	r.enter(2, 200, "乙", 150, 150)
	r.step()

	pkt := outA.find(protocol.SCRemotePlayer)
	if pkt == nil {
		t.Fatalf("甲应收到乙的远端玩家出场包 0x8023, 实际收到 %#x", outA.opcodes())
	}
	p := pkt[2:]
	if id := binary.LittleEndian.Uint32(p); id != 2 {
		t.Errorf("出场包里应是乙的**运行时** id 2, 实际 %d —— 别把角色主键发出去", id)
	}
	nameLen := int(binary.LittleEndian.Uint16(p[4:]))
	if got := string(p[6 : 6+nameLen]); got != "乙" {
		t.Errorf("名字应为 乙, 实际 %q", got)
	}
}

// 移动: 旁观者收到 0x8018, 移动者自己**不收**。
func TestMoveProducesMovePacketForOthersOnly(t *testing.T) {
	r := newRig(t)
	outA := r.enter(1, 100, "甲", 100, 100)
	outB := r.enter(2, 200, "乙", 150, 150)
	r.step()
	outA.reset()
	outB.reset()

	r.scene.Post(scene.MoveTo{ID: 1, To: domain.Pos{MapID: 7, X: 180, Y: 190}})
	r.step()

	pkt := outB.find(protocol.SCEntityMove)
	if pkt == nil {
		t.Fatalf("乙应收到甲的移动包 0x8018, 实际 %#x", outB.opcodes())
	}
	p := pkt[2:]
	if x := int32(binary.LittleEndian.Uint32(p[9:])); x != 180 {
		t.Errorf("移动包 X 应为 180, 实际 %d", x)
	}
	if y := int32(binary.LittleEndian.Uint32(p[13:])); y != 190 {
		t.Errorf("移动包 Y 应为 190, 实际 %d", y)
	}
	if outA.find(protocol.SCEntityMove) != nil {
		t.Error("甲不该收到自己的移动包 —— 会把客户端画面拽回去")
	}
}

// 离开: 旁观者收到 0x8010。少了它, 乙屏幕上会留下一个不动的幽灵。
func TestLeaveProducesDespawnPacket(t *testing.T) {
	r := newRig(t)
	r.enter(1, 100, "甲", 100, 100)
	outB := r.enter(2, 200, "乙", 150, 150)
	r.step()
	outB.reset()

	r.scene.Post(scene.Leave{ID: 1, Reason: "登出"})
	r.step()

	pkt := outB.find(protocol.SCEntityDespawn)
	if pkt == nil {
		t.Fatalf("乙应收到甲的消失包 0x8010, 实际 %#x", outB.opcodes())
	}
	if id := binary.LittleEndian.Uint32(pkt[2:]); id != 1 {
		t.Errorf("消失的应是实体 1, 实际 %d", id)
	}
}
