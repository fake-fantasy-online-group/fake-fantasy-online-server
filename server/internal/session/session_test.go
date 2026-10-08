package session

import (
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
)

// 这里放会话层的**跨阶段**测试。单点测试在 auth_test / friend_test / party_test /
// sink_test 里, 按主题分开。

// 已注册的 opcode 必须都有处理分支。
//
// 表里注册了但 OnPacket 里没接的话, 表现是"服务端收到包、日志打一行警告、
// 然后什么都不发生" —— 联调时极难与"客户端没发"区分开。
func TestRegisteredOpcodesAllHandled(t *testing.T) {
	sink := &capSink{}
	s := &Session{sink: sink, log: slog.Default(), deps: Deps{Log: slog.Default()}, stage: StageInGame}

	// 只挑不需要依赖的: 有依赖的分支各自的测试文件里覆盖
	for _, c := range []struct {
		op   uint16
		name string
		pay  []byte
	}{
		{0x1001, "心跳", make([]byte, 8)},
		{0x107d, "选服状态", nil},
	} {
		before := len(sink.pkts)
		s.OnPacket(c.op, c.pay)
		if len(sink.pkts) == before {
			t.Fatalf("%s(0x%04x) 没有回包", c.name, c.op)
		}
	}
}

// 注册表里的每个 Kind 都必须在 OnPacket 里有分支。
//
// 少一个分支的表现是日志一行 "已注册的请求没有处理分支" 然后什么都不发生 ——
// 联调时与"客户端没发"分不开。实测就漏过 SavePos(0x100a):
// 我把它从 Move 拆出来单独成一个 Kind, 却忘了加分支。
//
// 这里直接对着源码检查 —— 比跑一遍所有包再看日志可靠, 也不需要造依赖。
func TestEveryRegisteredKindHasABranch(t *testing.T) {
	src, err := os.ReadFile("session.go")
	if err != nil {
		t.Fatalf("读不到 session.go: %v", err)
	}
	body := string(src)
	seen := map[protocol.ReqKind]bool{}
	// 按 RegisteredKinds 查, 不是 RegisteredKind ——
	// 子命令包(0x100b)一个号解得出好几种 Kind, 只查主 Kind 会漏掉它们。
	for _, op := range protocol.ProvenOps() {
		for _, kind := range protocol.RegisteredKinds(op) {
			if kind == protocol.ReqUnknown || seen[kind] {
				continue
			}
			seen[kind] = true
			want := "case protocol.Req" + kind.String() + ":"
			if !strings.Contains(body, want) {
				t.Fatalf("0x%04x 的 %v 注册了却没有 %q 分支 —— "+
					"收到包会只打一行警告然后什么都不做", uint16(op), kind, want)
			}
		}
	}
	if len(seen) < 10 {
		t.Fatalf("只检查到 %d 个 Kind, 注册表是不是没读到?", len(seen))
	}
}
