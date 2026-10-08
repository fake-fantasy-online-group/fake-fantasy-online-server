package net

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/wire"
)

// 网关层测试。
//
// 用 `net.Pipe()` 而不是真 socket：不占端口、不受系统 TCP 缓冲影响、
// 收发是同步的所以时序可控。真 socket 的那点额外覆盖换来的是偶发失败。
//
// 这一层最要紧的两条性质：
//
//	背压    Send 绝不阻塞调用方。阻塞了就是**一个慢客户端冻住整个世界循环**
//	韧性    单帧损坏不断线。断了的话任何一次丢包/篡改都会把玩家踢下线
//
// 两条都不是功能，是"出事时会怎样"，而那正是没人会顺手测的部分。

// testKeys 是一对固定密钥。长度必须对（AES-256 要 32 字节）。
func testKeys() wire.Keys {
	k := wire.Keys{AES: make([]byte, 32), MAC: make([]byte, 32)}
	for i := range k.AES {
		k.AES[i] = byte(i)
		k.MAC[i] = byte(255 - i)
	}
	return k
}

// fakeHandler 记下 net 层交上来的一切。
type fakeHandler struct {
	mu sync.Mutex

	hsCalls  int
	hsErr    error
	keys     wire.Keys
	pkts     []wire.Packet
	closes   []string
	onPacket func(op uint16, payload []byte) // 可选钩子
	onClose  func()                          // 可选钩子；在记下原因后、锁外调用
}

func newHandler() *fakeHandler { return &fakeHandler{keys: testKeys()} }

func (h *fakeHandler) OnHandshake(hs []byte) (wire.Keys, error) {
	h.mu.Lock()
	h.hsCalls++
	err := h.hsErr
	h.mu.Unlock()
	if err != nil {
		return wire.Keys{}, err
	}
	return h.keys, nil
}

func (h *fakeHandler) OnPacket(op uint16, payload []byte) {
	h.mu.Lock()
	h.pkts = append(h.pkts, wire.Packet{Op: op, Payload: append([]byte(nil), payload...)})
	fn := h.onPacket
	h.mu.Unlock()
	if fn != nil {
		fn(op, payload)
	}
}

func (h *fakeHandler) OnClose(reason string) {
	h.mu.Lock()
	h.closes = append(h.closes, reason)
	fn := h.onClose
	h.mu.Unlock()
	if fn != nil {
		fn()
	}
}

func (h *fakeHandler) packets() []wire.Packet {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]wire.Packet(nil), h.pkts...)
}

func (h *fakeHandler) closeReasons() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.closes...)
}

// rig 是一条跑在内存管道上的连接。
type rig struct {
	t       *testing.T
	client  net.Conn // 测试这一端，扮演客户端
	conn    *Conn
	h       *fakeHandler
	done    chan struct{}
	cancel  context.CancelFunc
	sendSeq uint64
}

func newRig(t *testing.T, opts Options) *rig {
	t.Helper()
	cli, srv := net.Pipe()
	h := newHandler()
	if opts.Log == nil {
		opts.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	c := newConn(1, srv, h, opts)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { c.serve(ctx); close(done) }()

	r := &rig{t: t, client: cli, conn: c, h: h, done: done, cancel: cancel}
	t.Cleanup(func() {
		cancel()
		cli.Close()
		c.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("serve 没退出")
		}
	})
	return r
}

// handshake 从客户端这边送一个握手包过去。内容不重要，长度重要。
func (r *rig) handshake() {
	r.t.Helper()
	hs := make([]byte, 260)
	r.writeAll(hs)
	// 等 serve 真的收下了再继续 —— 不等的话后面的帧可能跟握手挤在一起
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		r.h.mu.Lock()
		n := r.h.hsCalls
		r.h.mu.Unlock()
		if n > 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	r.t.Fatal("握手没被处理")
}

func (r *rig) writeAll(b []byte) {
	r.t.Helper()
	_ = r.client.SetWriteDeadline(time.Now().Add(2 * time.Second))
	if _, err := r.client.Write(b); err != nil {
		r.t.Fatalf("写入失败: %v", err)
	}
}

// sendPacket 以客户端身份发一个合法的上行包。
func (r *rig) sendPacket(op uint16, payload []byte) {
	r.t.Helper()
	plain := wire.EncodeSequenced(r.sendSeq, wire.Encode(op, payload))
	r.sendSeq++
	frame, err := wire.Seal(testKeys(), plain)
	if err != nil {
		r.t.Fatalf("封包: %v", err)
	}
	r.writeAll(frame)
}

// waitPackets 等到收够 n 个包。
func (r *rig) waitPackets(n int) []wire.Packet {
	r.t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got := r.h.packets(); len(got) >= n {
			return got
		}
		time.Sleep(time.Millisecond)
	}
	r.t.Fatalf("只收到 %d 个包, 期望 %d", len(r.h.packets()), n)
	return nil
}

func (r *rig) waitClosed() string {
	r.t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got := r.h.closeReasons(); len(got) > 0 {
			return got[0]
		}
		time.Sleep(time.Millisecond)
	}
	r.t.Fatal("连接没关闭")
	return ""
}

// ── 握手 ──

func TestHandshakeThenPacket(t *testing.T) {
	r := newRig(t, Options{})
	r.handshake()
	r.sendPacket(0x1001, []byte{1, 2, 3, 4})

	got := r.waitPackets(1)
	if got[0].Op != 0x1001 {
		t.Fatalf("opcode = %#x", got[0].Op)
	}
	if string(got[0].Payload) != string([]byte{1, 2, 3, 4}) {
		t.Fatalf("载荷 = % x", got[0].Payload)
	}
}

// 握手包不够长就断开 —— 扫端口的连上来发几个字节就跑，每天一堆。
func TestShortHandshakeClosesConnection(t *testing.T) {
	r := newRig(t, Options{})
	r.writeAll([]byte{1, 2, 3}) // 只有 3 字节, 远不够 260
	r.client.Close()

	reason := r.waitClosed()
	if reason == "" {
		t.Fatal("没给出断开原因")
	}
	if r.h.packets() != nil && len(r.h.packets()) > 0 {
		t.Fatal("握手都没成就交上了包")
	}
}

// 握手被拒（密钥解不出来）也要断开，而且**不能进读循环** ——
// 进去了就等于没有密钥也能发包。
func TestRejectedHandshakeClosesConnection(t *testing.T) {
	cli, srv := net.Pipe()
	h := newHandler()
	h.hsErr = errors.New("密钥不对")
	c := newConn(1, srv, h, Options{Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	done := make(chan struct{})
	go func() { c.serve(context.Background()); close(done) }()
	t.Cleanup(func() { cli.Close(); c.Close() })

	go func() { _, _ = cli.Write(make([]byte, 260)) }()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("握手失败后 serve 没退出")
	}
	if got := h.closeReasons(); len(got) != 1 {
		t.Fatalf("OnClose 调了 %d 次, 该正好 1 次", len(got))
	}
}

// ── 韧性：坏帧不该断线 ──

// **单帧解密失败不断开。** 断了的话任何一次丢包/篡改都会把玩家踢下线，
// 而客户端那边只会看到"莫名其妙掉线"。
func TestCorruptFrameDoesNotDropConnection(t *testing.T) {
	r := newRig(t, Options{})
	r.handshake()

	// 一个长度合法但内容是垃圾的帧: HMAC 对不上, Open 会失败
	bad, err := wire.Seal(testKeys(), wire.EncodeSequenced(r.sendSeq, wire.Encode(0x1001, []byte{9})))
	if err != nil {
		t.Fatal(err)
	}
	bad[len(bad)-1] ^= 0xff // 篡改最后一字节
	r.writeAll(bad)

	// 坏帧之后紧跟一个好帧: 好帧必须照常收到
	r.sendPacket(0x1002, []byte{7, 7})
	got := r.waitPackets(1)
	if got[0].Op != 0x1002 {
		t.Fatalf("坏帧之后收到的是 %#x, 该是 0x1002", got[0].Op)
	}
	if reasons := r.h.closeReasons(); len(reasons) > 0 {
		t.Fatalf("坏帧把连接断了: %v", reasons)
	}
}

// 合法加密但缺少 1.5.8 序号/opcode 的包必须直接断开。
func TestMalformedInnerPacketDropsConnection(t *testing.T) {
	r := newRig(t, Options{})
	r.handshake()

	// 旧 opcode|payload 虽然加密合法，但没有 sequence，必须拒绝。
	bad, err := wire.Seal(testKeys(), wire.Encode(0x1001, make([]byte, 8)))
	if err != nil {
		t.Fatal(err)
	}
	r.writeAll(bad)
	if reason := r.waitClosed(); reason == "" {
		t.Fatal("无序号包没有触发断开")
	}
	if got := r.h.packets(); len(got) != 0 {
		t.Fatalf("畸形包进入了 handler: %+v", got)
	}
}

// ── 背压：这一层最要紧的一条 ──

// **Send 绝不阻塞调用方。**
//
// 调 Send 的是场景 goroutine（世界循环）。它一旦被一个慢客户端卡住，
// 那张图上所有人都停帧 —— 一个人的烂网络冻住整个服务器。
//
// 所以队列满时的正确行为是**断开那个连接**，不是等他。
func TestSendNeverBlocksAndDropsSlowClient(t *testing.T) {
	// 队列开得很小, 而且客户端那头一个字节都不读 —— 写 goroutine 立刻堵住
	r := newRig(t, Options{OutBuffer: 4})
	r.handshake()

	done := make(chan struct{})
	go func() {
		defer close(done)
		// 远超队列容量; 只要有一次阻塞, 这个 goroutine 就再也回不来
		for i := 0; i < 200; i++ {
			r.conn.Send([]byte{0x01, 0x10})
		}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Send 阻塞了 —— 一个慢客户端会冻住整个世界循环")
	}

	// 而且慢客户端要被断开
	if !r.conn.closed.Load() {
		t.Fatal("队列灌满了却没断开慢客户端")
	}
}

// 关闭之后再 Send 不该 panic（往已关闭的 channel 写会 panic）。
//
// 关服时序上这真会发生：场景还在冲刷最后一批事件，连接已经断了。
func TestSendAfterCloseIsSafe(t *testing.T) {
	r := newRig(t, Options{})
	r.handshake()
	r.conn.Close()

	for i := 0; i < 10; i++ {
		r.conn.Send([]byte{0x01, 0x10}) // 不 panic 就算过
	}
}

// Close 幂等。读循环、写循环、世界层都可能各调一次。
func TestCloseIsIdempotent(t *testing.T) {
	r := newRig(t, Options{})
	r.handshake()

	for i := 0; i < 5; i++ {
		r.conn.Close()
	}
	// OnClose 只该报一次 —— 报多次的话会话层会重复做清理
	reason := r.waitClosed()
	time.Sleep(50 * time.Millisecond)
	if got := r.h.closeReasons(); len(got) != 1 {
		t.Fatalf("OnClose 调了 %d 次(%v), 该正好 1 次", len(got), got)
	}
	_ = reason
}

// ── 下行 ──

// 发出去的东西客户端要能解回来。这条把 Send → writeLoop → wire.Seal 串起来验了。
func TestSendReachesClientAndDecodes(t *testing.T) {
	r := newRig(t, Options{})
	r.handshake()

	inner := wire.Encode(0x8002, []byte{0, 0, 0, 0})
	r.conn.Send(inner)

	_ = r.client.SetReadDeadline(time.Now().Add(2 * time.Second))
	body, err := wire.ReadFrame(r.client)
	if err != nil {
		t.Fatalf("客户端读帧失败: %v", err)
	}
	plain, err := wire.Open(testKeys(), body)
	if err != nil {
		t.Fatalf("客户端解帧失败: %v", err)
	}
	var decoder wire.InboundDecoder
	pkt, err := decoder.Decode(plain)
	if err != nil {
		t.Fatalf("客户端解包失败: %v", err)
	}
	if pkt.Op != 0x8002 {
		t.Fatalf("收到 %#x, 该是 0x8002", pkt.Op)
	}
}

// ── 断开原因 ──

// 客户端正常断开要报得出来 —— 运维靠这个区分"他自己关的"和"我们踢的"。
func TestClientDisconnectIsReported(t *testing.T) {
	r := newRig(t, Options{})
	r.handshake()
	r.client.Close()

	reason := r.waitClosed()
	if reason == "" {
		t.Fatal("没给出断开原因")
	}
	t.Logf("断开原因: %s", reason)
}

// ctx 取消（关服）时读循环要退出。
func TestContextCancelStopsServe(t *testing.T) {
	cli, srv := net.Pipe()
	h := newHandler()
	c := newConn(1, srv, h, Options{Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { c.serve(ctx); close(done) }()
	t.Cleanup(func() { cli.Close(); c.Close() })

	go func() { _, _ = cli.Write(make([]byte, 260)) }()
	// 等握手过去
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		h.mu.Lock()
		n := h.hsCalls
		h.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}

	cancel()
	c.Close() // 读循环阻塞在 ReadFrame 上, 靠关 socket 把它踹醒

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ctx 取消后 serve 没退出")
	}
}

// ── Server：真监听 ──
//
// 这一段用**真 socket**，因为要验的正是"客户端能不能连上"——
// 用 net.Pipe 就把被测的东西绕过去了。
//
// 端口写 `:0` 让系统分配：写死端口的测试会在并行跑或端口被占时偶发失败。

// listenAddr 从跑起来的 Server 上问出实际监听的地址。
//
// Server 现在没有暴露它，所以自己先占一个端口再让给它 —— 有个小竞态窗口，
// 但比写死端口可靠得多。真要根治得给 Server 加一个 Addr() 访问器，
// 那是生产代码为测试让路，先不动。
func freePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("找不到空闲端口: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()
	return addr
}

// **客户端连上来能不能走完握手并发包。**
// conn_test 的其余部分都绕过了 accept 这一段，只有这条真的过了 socket。
func TestServerAcceptsAndServes(t *testing.T) {
	addr := freePort(t)
	h := newHandler()
	got := make(chan wire.Packet, 4)
	h.onPacket = func(op uint16, payload []byte) {
		got <- wire.Packet{Op: op, Payload: append([]byte(nil), payload...)}
	}

	srv := NewServer(addr, func(Sink) Handler { return h },
		Options{Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() { runErr <- srv.Run(ctx) }()
	t.Cleanup(cancel)

	// 等监听起来
	var cli net.Conn
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			cli = c
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if cli == nil {
		t.Fatal("连不上服务器")
	}
	defer cli.Close()

	// 握手 + 一个包
	if _, err := cli.Write(make([]byte, 260)); err != nil {
		t.Fatalf("发握手: %v", err)
	}
	frame, err := wire.Seal(testKeys(), wire.EncodeSequenced(0, wire.Encode(0x1002, []byte{5, 6})))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cli.Write(frame); err != nil {
		t.Fatalf("发包: %v", err)
	}

	select {
	case p := <-got:
		if p.Op != 0x1002 || string(p.Payload) != string([]byte{5, 6}) {
			t.Fatalf("收到 %#x / % x", p.Op, p.Payload)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("服务器没收到包 —— accept 到 OnPacket 这条链断了")
	}

	// 关服: Run 要返回 nil(正常关闭), 不能报错
	cancel()
	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("正常关服却返回了错误: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("ctx 取消后 Run 没返回 —— 关服会挂住")
	}
}

// 关服不能只关 listener：已接入的客户端可能正卡在握手/读帧上。Run 还必须
// 等每条连接的 OnClose 做完，调用方才能继续关闭会话清理所依赖的数据库。
func TestServerShutdownClosesEveryConnectionAndWaitsForOnClose(t *testing.T) {
	addr := freePort(t)
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	accepted := make(chan struct{}, 2)
	h := newHandler()
	h.onClose = func() {
		entered <- struct{}{}
		<-release
	}

	srv := NewServer(addr, func(Sink) Handler {
		accepted <- struct{}{}
		return h
	}, Options{Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() { runErr <- srv.Run(ctx) }()
	t.Cleanup(cancel)

	clients := make([]net.Conn, 0, 2)
	dialDeadline := time.Now().Add(3 * time.Second)
	for len(clients) < 2 && time.Now().Before(dialDeadline) {
		c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err != nil {
			time.Sleep(5 * time.Millisecond)
			continue
		}
		clients = append(clients, c)
	}
	if len(clients) != 2 {
		t.Fatalf("只连上 %d 条连接", len(clients))
	}
	defer func() {
		for _, c := range clients {
			_ = c.Close()
		}
	}()
	for range clients {
		select {
		case <-accepted:
		case <-time.After(2 * time.Second):
			t.Fatal("连接已建立但 factory 没被调用")
		}
	}

	// 两条连接都故意不发握手；只有 Server 主动 Close 才能踹醒 ReadFull。
	cancel()
	for range clients {
		select {
		case <-entered:
		case <-time.After(2 * time.Second):
			t.Fatal("关服没有关闭全部已接收连接")
		}
	}
	select {
	case err := <-runErr:
		t.Fatalf("OnClose 尚未完成，Run 就返回了: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("正常关服却返回了错误: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OnClose 完成后 Run 仍未返回")
	}
	if got := h.closeReasons(); len(got) != len(clients) {
		t.Fatalf("OnClose 调了 %d 次，连接数是 %d", len(got), len(clients))
	}
}

// 端口被占时 Run 要**立刻返回错误**，不能默默重试到天亮。
func TestServerReportsListenFailure(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	srv := NewServer(ln.Addr().String(), func(Sink) Handler { return newHandler() },
		Options{Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err := srv.Run(context.Background()); err == nil {
		t.Fatal("端口被占却没报错 —— 起服脚本会以为起成功了")
	}
}

// 每条连接要有各自的 id。混了的话日志里根本分不清谁是谁。
func TestEachConnectionGetsItsOwnID(t *testing.T) {
	addr := freePort(t)
	var mu sync.Mutex
	var ids []int64

	srv := NewServer(addr, func(s Sink) Handler {
		mu.Lock()
		ids = append(ids, s.SessionID())
		mu.Unlock()
		return newHandler()
	}, Options{Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = srv.Run(ctx) }()
	t.Cleanup(cancel)

	for i := 0; i < 3; i++ {
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
			if err == nil {
				defer c.Close()
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(ids)
		mu.Unlock()
		if n >= 3 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(ids) < 3 {
		t.Fatalf("只接到 %d 条连接", len(ids))
	}
	seen := map[int64]bool{}
	for _, id := range ids {
		if seen[id] {
			t.Fatalf("连接 id %d 重复了 —— 日志里分不清谁是谁", id)
		}
		seen[id] = true
	}
}
