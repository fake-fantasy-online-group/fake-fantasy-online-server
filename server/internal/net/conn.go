// Package net 是网关层: 管理每个 TCP 连接的生命周期, 负责分帧、加解密(复用 wire)、
// 收发与背压。它**不碰世界状态**(见 docs/架构设计.md §3.2), 只把上行包交给 handler,
// 把下行字节写出去。
package net

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/wire"
)

// Handler 处理一个连接上的事件。由 session 层实现。net 层只依赖这个接口。
type Handler interface {
	// OnHandshake 收到握手包(260B)时调用, 返回本连接的会话密钥。
	// 服务端使用私钥解握手包并返回本连接的会话密钥。
	OnHandshake(hs []byte) (wire.Keys, error)
	// OnPacket 收到一个解密后的上行内层包(opcode+载荷)。不得阻塞太久(会拖慢该连接读循环)。
	OnPacket(op uint16, payload []byte)
	// OnClose 连接关闭时调用一次, 用于清理会话/通知世界移除实体。
	OnClose(reason string)
}

type outboundPacket struct {
	inner      []byte
	pacedEpoch uint64 // zero identifies ordinary/control traffic, never purged
}

// Conn 封装一个客户端连接。实现 world.Sink(Send/Close/SessionID), 供世界循环往此连接发包。
type Conn struct {
	id         int64
	raw        net.Conn
	keys       atomic.Pointer[wire.Keys]
	out        chan outboundPacket // 下行内层包队列(未加密), 写 goroutine 消费
	outMu      sync.Mutex          // serializes producers for atomic batch admission; never held during network I/O
	outBytes   atomic.Int64
	outWake    chan struct{} // one wake token, not an additional packet queue
	pacedEpoch uint64        // protected by outMu; zero is reserved for normal traffic
	handler    Handler
	log        *slog.Logger

	closeOnce sync.Once
	closed    atomic.Bool
	closeCh   chan struct{}

	handshakeLen   int
	inbound        wire.InboundDecoder
	sendSeq        uint64
	opts           Options
	packets        tokenBucket
	bytes          tokenBucket
	authenticated  atomic.Bool
	handshakeSlots chan struct{}
}

// Options 连接参数。
type Options struct {
	HandshakeLen         int           // 握手包字节数, 默认 260
	OutBuffer            int           // 下行队列容量, 默认 1024。满则判定慢客户端并断开(背压)
	WriteWait            time.Duration // 单次写超时, 默认 10s
	Log                  *slog.Logger
	HandshakeWait        time.Duration          // default 10s, absolute handshake deadline
	AuthWait             time.Duration          // default 2m to complete account authentication
	HandshakeConcurrency int                    // default 16 RSA handshakes at once
	ReadWait             time.Duration          // default 2m, absolute deadline for each complete frame
	MaxConnections       int                    // default 4096
	MaxConnectionsPerIP  int                    // default 128 (allow shared NATs)
	PacketsPerSecond     float64                // default 100
	PacketBurst          int                    // default 200
	BytesPerSecond       float64                // default 2 MiB
	ByteBurst            int                    // default 4 MiB
	ShutdownWait         time.Duration          // default 10s
	ShutdownContext      func() context.Context // optional shared shutdown budget
}

func newConn(id int64, raw net.Conn, h Handler, o Options) *Conn {
	o = normalizeOptions(o)
	return &Conn{
		id: id, raw: raw, handler: h,
		out:          make(chan outboundPacket, o.OutBuffer),
		outWake:      make(chan struct{}, 1),
		pacedEpoch:   1,
		closeCh:      make(chan struct{}),
		log:          o.Log.With("conn", id),
		handshakeLen: o.HandshakeLen,
		opts:         o,
	}
}

// ── world.Sink 实现 ──

func (c *Conn) SessionID() int64 { return c.id }

// stagedQueueByteWatermark limits new paced presentation admission. Ordinary
// Send retains its legacy packet-count-only contract; a valid global burst can
// exceed this watermark, in which case paced traffic pauses until it drains.
const stagedQueueByteWatermark = 4 << 20

// Send 把一个下行内层包投入发送队列。非阻塞: 队列满(慢客户端)则断开该连接,
// 绝不阻塞调用方(世界循环)。这是背压的关键。
func (c *Conn) Send(inner []byte) {
	if c.closed.Load() {
		return
	}
	c.outMu.Lock()
	if c.closed.Load() {
		c.outMu.Unlock()
		return
	}
	if len(c.out) == cap(c.out) {
		c.outMu.Unlock()
		c.log.Warn("下行队列满, 判定慢客户端, 断开")
		c.Close()
		return
	}
	// Preserve normal/control delivery semantics: no new aggregate byte or
	// frame-size rejection is imposed on the pre-existing Send path. Producers
	// serialize only this nonblocking admission, never socket writes.
	c.outBytes.Add(int64(len(inner)))
	c.out <- outboundPacket{inner: inner}
	c.wakeWriter()
	c.outMu.Unlock()
}

// TrySendBatch atomically admits a bounded batch without waiting for the writer.
// False means no packet was accepted. Unlike Send, transient congestion does not
// close the connection: the scene owns the bounded retry deadline. A little room
// remains for ordinary session replies, so one joining client cannot fill its
// connection with a snapshot at the expense of its live control traffic.
func (c *Conn) TrySendBatch(packets [][]byte) bool { return c.trySendBatch(packets, 0) }

// TrySendBatchEpoch rejects an already encoded old-world batch after retirement.
func (c *Conn) TrySendBatchEpoch(packets [][]byte, epoch uint64) bool {
	if epoch == 0 {
		return false
	}
	return c.trySendBatch(packets, epoch)
}
func (c *Conn) trySendBatch(packets [][]byte, epoch uint64) bool {
	if c.closed.Load() {
		return false
	}
	n := int64(0)
	for _, p := range packets {
		if len(p) > wire.MaxFrame {
			return false
		}
		n += int64(len(p))
		if n > stagedQueueByteWatermark {
			return false
		}
	}
	c.outMu.Lock()
	defer c.outMu.Unlock()
	if c.closed.Load() || epoch != 0 && epoch != c.pacedEpoch {
		return false
	}
	limit := cap(c.out)
	byteLimit := int64(stagedQueueByteWatermark - 64<<10)
	headroom := limit / 8
	if headroom > 64 {
		headroom = 64
	}
	limit -= headroom
	if len(packets) > limit-len(c.out) || n > byteLimit-c.outBytes.Load() {
		return false
	}
	// Only the writer can remove entries and all producers hold outMu. The
	// sends below cannot block after the capacity check above.
	c.outBytes.Add(n)
	for _, p := range packets {
		c.out <- outboundPacket{inner: p, pacedEpoch: c.pacedEpoch}
	}
	c.wakeWriter()
	return true
}

// Closed lets an actor discard a pending stream promptly after transport loss.
func (c *Conn) Closed() bool { return c.closed.Load() }

// Close 幂等关闭。触发读写 goroutine 退出与 OnClose。
func (c *Conn) Close() {
	c.closeOnce.Do(func() {
		c.closed.Store(true)
		close(c.closeCh)
		_ = c.raw.Close()
	})
}

// serve 跑该连接的完整生命周期: 握手 -> 启动写 goroutine -> 读循环。阻塞直到断开。
func (c *Conn) serve(ctx context.Context) {
	reason := "正常断开"
	defer func() {
		c.Close()
		c.handler.OnClose(reason)
		c.log.Info("连接关闭", "reason", reason)
	}()

	// 1) 握手
	_ = c.raw.SetReadDeadline(time.Now().Add(c.opts.HandshakeWait))
	hs := make([]byte, c.handshakeLen)
	if _, err := io.ReadFull(c.raw, hs); err != nil {
		reason = "读握手失败: " + err.Error()
		return
	}
	if c.handshakeSlots != nil {
		select {
		case c.handshakeSlots <- struct{}{}:
		default:
			reason = "handshake capacity exhausted"
			return
		}
	}
	keys, err := func() (wire.Keys, error) {
		if c.handshakeSlots != nil {
			defer func() { <-c.handshakeSlots }()
		}
		return c.handler.OnHandshake(hs)
	}()
	if err != nil {
		reason = "握手处理失败: " + err.Error()
		return
	}
	c.SetKeys(keys)

	// 2) 写 goroutine
	go c.writeLoop(ctx)

	// 3) 读循环
	var opener wire.Opener
	authDeadline := time.Now().Add(c.opts.AuthWait)
	for {
		select {
		case <-ctx.Done():
			reason = "服务器关闭"
			return
		case <-c.closeCh:
			return
		default:
		}
		deadline := time.Now().Add(c.opts.ReadWait)
		if !c.authenticated.Load() && authDeadline.Before(deadline) {
			deadline = authDeadline
		}
		_ = c.raw.SetReadDeadline(deadline)
		body, err := wire.ReadFrameWithAdmission(c.raw, func(size uint32) error {
			now := time.Now()
			if !c.packets.allow(now, 1, c.opts.PacketsPerSecond, float64(c.opts.PacketBurst)) ||
				!c.bytes.allow(now, float64(size)+wire.LenPrefix, c.opts.BytesPerSecond, float64(c.opts.ByteBurst)) {
				return errIngressLimit
			}
			return nil
		})
		if err != nil {
			if errors.Is(err, io.EOF) {
				reason = "客户端断开"
			} else {
				reason = "读帧错误: " + err.Error()
			}
			return
		}
		k := c.keys.Load()
		if k == nil {
			reason = "无会话密钥"
			return
		}
		plain, err := opener.OpenInPlace(k, body)
		if err != nil {
			// 单帧解密失败不一定要断开(可能密钥切换), 记录并继续; 连续失败由上层策略处理
			c.log.Warn("解帧失败", "err", err, "len", len(body))
			continue
		}
		pkt, err := c.inbound.Decode(plain)
		if err != nil {
			reason = "上行序号帧无效: " + err.Error()
			return
		}
		c.handler.OnPacket(pkt.Op, pkt.Payload)
	}
}

// writeLoop 从 out 队列取下行内层包, 加密后写出。独占 raw 的写端。
func (c *Conn) writeLoop(ctx context.Context) {
	var sealer wire.Sealer
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.closeCh:
			return
		default:
		}
		inner, ok := c.takeOutbound()
		if !ok {
			select {
			case <-ctx.Done():
				return
			case <-c.closeCh:
				return
			case <-c.outWake:
			}
			continue
		}
		k := c.keys.Load()
		if k == nil {
			continue
		}
		frame, err := sealer.SealSequenced(k, c.sendSeq, inner)
		c.sendSeq++
		if err != nil {
			c.log.Error("封包失败", "err", err)
			continue
		}
		_ = c.raw.SetWriteDeadline(time.Now().Add(c.opts.WriteWait))
		if _, err := c.raw.Write(frame); err != nil {
			c.log.Warn("写出失败, 断开", "err", err)
			c.Close()
			return
		}
	}
}

// takeOutbound is the writer's dequeue linearization point. Epoch retirement
// shares this mutex; after it returns at most this one in-flight old frame can
// finish, always before a subsequently enqueued native reset/control packet.
func (c *Conn) takeOutbound() ([]byte, bool) {
	c.outMu.Lock()
	defer c.outMu.Unlock()
	select {
	case p := <-c.out:
		c.outBytes.Add(-int64(len(p.inner)))
		return p.inner, true
	default:
		return nil, false
	}
}
func (c *Conn) wakeWriter() {
	select {
	case c.outWake <- struct{}{}:
	default:
	}
}

func (c *Conn) PacedEpoch() uint64 {
	c.outMu.Lock()
	defer c.outMu.Unlock()
	return c.pacedEpoch
}

// CancelPacedEpoch retires queued, unwritten presentation only. Normal/control
// packets retain their exact FIFO order. A stale caller cannot purge a newer
// generation. The fixed-capacity channel is filtered in place while the writer
// is excluded: no second packet queue, socket work, or wait for I/O is involved.
func (c *Conn) CancelPacedEpoch(epoch uint64) (uint64, bool) {
	c.outMu.Lock()
	defer c.outMu.Unlock()
	if epoch == 0 || epoch != c.pacedEpoch {
		return c.pacedEpoch, false
	}
	c.pacedEpoch++
	if c.pacedEpoch == 0 {
		c.pacedEpoch = 1
	}
	n := len(c.out)
	for i := 0; i < n; i++ {
		p := <-c.out
		if p.pacedEpoch != 0 {
			c.outBytes.Add(-int64(len(p.inner)))
		} else {
			c.out <- p
		}
	}
	if len(c.out) > 0 {
		c.wakeWriter()
	}
	return c.pacedEpoch, true
}

// SetKeys 允许上层在握手后更新本连接的会话密钥。
func (c *Conn) SetKeys(k wire.Keys) {
	// Own immutable snapshots: readers and writers cache cryptographic state
	// independently and may still be using the previous snapshot during rekey.
	k = wire.Keys{AES: append([]byte(nil), k.AES...), MAC: append([]byte(nil), k.MAC...)}
	c.keys.Store(&k)
}

// RemoteIP identifies the actual transport peer; never trust a client supplied address.
func (c *Conn) RemoteIP() string { return peerIP(c.raw.RemoteAddr()) }

// MarkAuthenticated removes only the pre-login lifetime cap; frame deadlines remain.
func (c *Conn) MarkAuthenticated() { c.authenticated.Store(true) }
