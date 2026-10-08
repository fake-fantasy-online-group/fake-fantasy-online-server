package net

import (
	"context"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
)

// HandlerFactory 为每个新连接创建一个 Handler。参数 conn 用作 world.Sink(发包/关闭)。
// 由 session 层提供实现。
type HandlerFactory func(sink Sink) Handler

// Sink 是网关暴露给上层的连接出口(与 world.Sink 同形)。session 拿到它去驱动世界。
type Sink interface {
	Send(inner []byte)
	Close()
	SessionID() int64
}

// Server 监听并接受连接, 为每个连接建立生命周期。
type Server struct {
	addr    string
	factory HandlerFactory
	opts    Options
	log     *slog.Logger
	nextID  atomic.Int64
}

func NewServer(addr string, factory HandlerFactory, opts Options) *Server {
	if opts.Log == nil {
		opts.Log = slog.Default()
	}
	return &Server{addr: addr, factory: factory, opts: opts, log: opts.Log}
}

// Run 阻塞监听直到 ctx 取消。返回前会关闭所有已接受的连接，并等待它们的
// serve（包括 Handler.OnClose）全部结束；调用方因此可以在 Run 返回后安全地
// 停场景、排空写回队列，最后关闭存储。
func (s *Server) Run(ctx context.Context) error {
	lc := net.ListenConfig{}
	ln, err := lc.Listen(ctx, "tcp", s.addr)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	defer ln.Close()
	s.log.Info("网关监听", "addr", s.addr)

	// accepted 只记录已经由本轮 Run 接管的连接。Accept 与登记都在当前
	// goroutine 串行发生；关监听后不会再 Add，所以随后 Wait 不会与 Add 竞态。
	var (
		mu       sync.Mutex
		accepted = make(map[*Conn]struct{})
		serving  sync.WaitGroup
	)
	closeAccepted := func() {
		mu.Lock()
		all := make([]*Conn, 0, len(accepted))
		for c := range accepted {
			all = append(all, c)
		}
		mu.Unlock()
		for _, c := range all {
			// 读循环可能正阻塞在握手或 ReadFrame；只取消 ctx 踹不醒 socket，
			// 必须关连接才能保证 serve 进入 defer 并执行 OnClose。
			c.Close()
		}
		serving.Wait()
	}

	watchDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = ln.Close()
		case <-watchDone:
		}
	}()
	defer close(watchDone)

	for {
		raw, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				closeAccepted()
				return nil // 正常关闭；全部 OnClose 已完成
			}
			s.log.Warn("accept 出错", "err", err)
			continue
		}
		// 取消可能和一次成功的 Accept 同时发生。不要把这条漏在关服快照外，
		// 更不能让它阻塞在尚未发送的握手上。
		if ctx.Err() != nil {
			_ = raw.Close()
			closeAccepted()
			return nil
		}
		id := s.nextID.Add(1)
		conn := newConn(id, raw, nil, s.opts)
		conn.handler = s.factory(conn) // 把 conn 作为 Sink 交给 session

		mu.Lock()
		accepted[conn] = struct{}{}
		serving.Add(1)
		mu.Unlock()
		go func(c *Conn) {
			defer serving.Done()
			defer func() {
				mu.Lock()
				delete(accepted, c)
				mu.Unlock()
			}()
			c.serve(ctx)
		}(conn)
	}
}
