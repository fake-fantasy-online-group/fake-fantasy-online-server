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

// Conn 封装一个客户端连接。实现 world.Sink(Send/Close/SessionID), 供世界循环往此连接发包。
type Conn struct {
	id      int64
	raw     net.Conn
	keys    atomic.Pointer[wire.Keys]
	out     chan []byte // 下行内层包队列(未加密), 写 goroutine 消费
	handler Handler
	log     *slog.Logger

	closeOnce sync.Once
	closed    atomic.Bool
	closeCh   chan struct{}

	handshakeLen int
	inbound      wire.InboundDecoder
	sendSeq      uint64
}

// Options 连接参数。
type Options struct {
	HandshakeLen int           // 握手包字节数, 默认 260
	OutBuffer    int           // 下行队列容量, 默认 1024。满则判定慢客户端并断开(背压)
	WriteWait    time.Duration // 单次写超时, 默认 10s
	Log          *slog.Logger
}

func newConn(id int64, raw net.Conn, h Handler, o Options) *Conn {
	if o.HandshakeLen == 0 {
		o.HandshakeLen = 260
	}
	if o.OutBuffer == 0 {
		o.OutBuffer = 1024
	}
	if o.WriteWait == 0 {
		o.WriteWait = 10 * time.Second
	}
	if o.Log == nil {
		o.Log = slog.Default()
	}
	return &Conn{
		id: id, raw: raw, handler: h,
		out:          make(chan []byte, o.OutBuffer),
		closeCh:      make(chan struct{}),
		log:          o.Log.With("conn", id),
		handshakeLen: o.HandshakeLen,
	}
}

// ── world.Sink 实现 ──

func (c *Conn) SessionID() int64 { return c.id }

// Send 把一个下行内层包投入发送队列。非阻塞: 队列满(慢客户端)则断开该连接,
// 绝不阻塞调用方(世界循环)。这是背压的关键。
func (c *Conn) Send(inner []byte) {
	if c.closed.Load() {
		return
	}
	select {
	case c.out <- inner:
	default:
		c.log.Warn("下行队列满, 判定慢客户端, 断开")
		c.Close()
	}
}

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
	hs := make([]byte, c.handshakeLen)
	if _, err := io.ReadFull(c.raw, hs); err != nil {
		reason = "读握手失败: " + err.Error()
		return
	}
	keys, err := c.handler.OnHandshake(hs)
	if err != nil {
		reason = "握手处理失败: " + err.Error()
		return
	}
	c.keys.Store(&keys)

	// 2) 写 goroutine
	go c.writeLoop(ctx)

	// 3) 读循环
	for {
		select {
		case <-ctx.Done():
			reason = "服务器关闭"
			return
		case <-c.closeCh:
			return
		default:
		}
		body, err := wire.ReadFrame(c.raw)
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
		plain, err := wire.Open(*k, body)
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
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.closeCh:
			return
		case inner := <-c.out:
			k := c.keys.Load()
			if k == nil {
				continue
			}
			inner = wire.EncodeSequenced(c.sendSeq, inner)
			c.sendSeq++
			frame, err := wire.Seal(*k, inner)
			if err != nil {
				c.log.Error("封包失败", "err", err)
				continue
			}
			_ = c.raw.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if _, err := c.raw.Write(frame); err != nil {
				c.log.Warn("写出失败, 断开", "err", err)
				c.Close()
				return
			}
		}
	}
}

// SetKeys 允许上层在握手后更新本连接的会话密钥。
func (c *Conn) SetKeys(k wire.Keys) { c.keys.Store(&k) }
