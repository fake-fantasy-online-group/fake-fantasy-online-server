package net

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/wire"
)

func TestReadDeadlinesReleaseSilentAndPartialPeers(t *testing.T) {
	for _, mode := range []string{"silent handshake", "partial handshake", "partial header", "partial body", "idle authenticated"} {
		t.Run(mode, func(t *testing.T) {
			r := newRig(t, Options{HandshakeWait: 30 * time.Millisecond, ReadWait: 30 * time.Millisecond})
			switch mode {
			case "partial handshake":
				r.writeAll([]byte{1})
			case "partial header":
				r.handshake()
				r.writeAll([]byte{1})
			case "partial body":
				r.handshake()
				r.writeAll(binary.LittleEndian.AppendUint32(nil, wire.MaxFrame))
			case "idle authenticated":
				r.handshake()
				r.conn.MarkAuthenticated()
			}
			select {
			case <-r.done:
			case <-time.After(time.Second):
				t.Fatal("stalled peer retained")
			}
			if len(r.h.closeReasons()) != 1 {
				t.Fatal("close handler must run exactly once")
			}
		})
	}
}
func TestDeclaredByteBudgetRejectsBeforeBodyRead(t *testing.T) {
	r := newRig(t, Options{ByteBurst: 64})
	r.handshake()
	r.writeAll(binary.LittleEndian.AppendUint32(nil, 1024))
	select {
	case <-r.done:
	case <-time.After(time.Second):
		t.Fatal("read body despite rejected declared length")
	}
}
func TestPacketFloodDisconnectsOnlyOffender(t *testing.T) {
	noisy := newRig(t, Options{PacketBurst: 2, PacketsPerSecond: 0.001})
	healthy := newRig(t, Options{PacketBurst: 2, PacketsPerSecond: 0.001})
	noisy.handshake()
	healthy.handshake()
	for i := uint64(0); i < 2; i++ {
		frame, err := wire.Seal(testKeys(), wire.EncodeSequenced(i, wire.Encode(1, nil)))
		if err != nil {
			t.Fatal(err)
		}
		noisy.writeAll(frame)
	}
	// The next prefix alone must be rejected; no need to supply an encrypted body.
	noisy.writeAll(binary.LittleEndian.AppendUint32(nil, 64))
	select {
	case <-noisy.done:
	case <-time.After(time.Second):
		t.Fatal("flood not disconnected")
	}
	frame, _ := wire.Seal(testKeys(), wire.EncodeSequenced(0, wire.Encode(1, nil)))
	healthy.writeAll(frame)
	deadline := time.Now().Add(time.Second)
	for len(healthy.h.packets()) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(healthy.h.packets()) != 1 {
		t.Fatal("healthy connection lost its independent budget")
	}
}
func TestTokenBucketRefillAndBurst(t *testing.T) {
	var b tokenBucket
	now := time.Unix(100, 0)
	if !b.allow(now, 2, 2, 2) || b.allow(now, 1, 2, 2) {
		t.Fatal("burst not bounded")
	}
	if !b.allow(now.Add(time.Second/2), 1, 2, 2) {
		t.Fatal("legitimate sustained packet denied")
	}
	if !b.allow(now.Add(time.Hour), 2, 2, 2) || b.allow(now.Add(time.Hour), 1, 2, 2) {
		t.Fatal("idle accrued unbounded credit")
	}
}
func TestAuthenticationLifetimeCannotBeExtendedByTraffic(t *testing.T) {
	r := newRig(t, Options{AuthWait: 40 * time.Millisecond, ReadWait: time.Second})
	r.handshake()
	frame, _ := wire.Seal(testKeys(), wire.EncodeSequenced(0, wire.Encode(1, nil)))
	r.writeAll(frame)
	select {
	case <-r.done:
	case <-time.After(time.Second):
		t.Fatal("pre-auth lifetime unbounded")
	}
}
func TestWriteWaitOptionIsHonored(t *testing.T) {
	r := newRig(t, Options{WriteWait: 25 * time.Millisecond})
	r.handshake()
	r.conn.Send(wire.Encode(1, nil)) // client deliberately never reads
	select {
	case <-r.done:
	case <-time.After(time.Second):
		t.Fatal("custom WriteWait ignored")
	}
}
func TestServerShutdownReportsUnfinishedHandler(t *testing.T) {
	addr := freePort(t)
	accepted := make(chan struct{}, 1)
	release := make(chan struct{})
	h := newHandler()
	h.onClose = func() { <-release }
	srv := NewServer(addr, func(Sink) Handler { accepted <- struct{}{}; return h }, Options{ShutdownWait: 30 * time.Millisecond, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer close(release)
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx) }()
	var c net.Conn
	var err error
	until := time.Now().Add(time.Second)
	for time.Now().Before(until) {
		c, err = net.Dial("tcp", addr)
		if err == nil {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	<-accepted
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected incomplete shutdown error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown hung")
	}
}

func TestServerAdmissionCapsBeforeHandlerAllocation(t *testing.T) {
	for _, mode := range []string{"global", "per IP"} {
		t.Run(mode, func(t *testing.T) {
			addr := freePort(t)
			accepted := make(chan struct{}, 4)
			opts := Options{MaxConnections: 10, MaxConnectionsPerIP: 10, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
			if mode == "global" {
				opts.MaxConnections = 1
			} else {
				opts.MaxConnectionsPerIP = 1
			}
			srv := NewServer(addr, func(Sink) Handler { accepted <- struct{}{}; return newHandler() }, opts)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- srv.Run(ctx) }()
			var first net.Conn
			var err error
			until := time.Now().Add(time.Second)
			for time.Now().Before(until) {
				first, err = net.Dial("tcp", addr)
				if err == nil {
					break
				}
				time.Sleep(time.Millisecond)
			}
			if err != nil {
				t.Fatal(err)
			}
			defer first.Close()
			<-accepted
			second, err := net.Dial("tcp", addr)
			if err != nil {
				t.Fatal(err)
			}
			defer second.Close()
			_ = second.SetReadDeadline(time.Now().Add(time.Second))
			if _, err = second.Read(make([]byte, 1)); err == nil {
				t.Fatal("over-cap connection retained")
			}
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				t.Fatal("over-cap peer was not actively closed")
			}
			select {
			case <-accepted:
				t.Fatal("over-cap handler allocated")
			default:
			}
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("server failed to stop")
			}
		})
	}
}

func TestHandshakeWorkAdmissionRejectsWithoutCallingHandler(t *testing.T) {
	cli, raw := net.Pipe()
	defer cli.Close()
	h := newHandler()
	c := newConn(1, raw, h, Options{Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	c.handshakeSlots = make(chan struct{}, 1)
	c.handshakeSlots <- struct{}{} // another peer owns the only expensive-work slot
	done := make(chan struct{})
	go func() { c.serve(context.Background()); close(done) }()
	if _, err := cli.Write(make([]byte, 260)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("overload queued handshake work")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.hsCalls != 0 {
		t.Fatal("handshake executed before admission")
	}
}

func TestAuthenticatedConnectionOutlivesPreloginDeadline(t *testing.T) {
	r := newRig(t, Options{AuthWait: 50 * time.Millisecond, ReadWait: time.Second})
	r.handshake()
	r.conn.MarkAuthenticated()
	// Trigger the next reader iteration after authentication, as real OnPacket does.
	frame, _ := wire.Seal(testKeys(), wire.EncodeSequenced(0, wire.Encode(1, nil)))
	r.writeAll(frame)
	time.Sleep(100 * time.Millisecond)
	frame, _ = wire.Seal(testKeys(), wire.EncodeSequenced(1, wire.Encode(1, nil)))
	r.writeAll(frame)
	until := time.Now().Add(time.Second)
	for len(r.h.packets()) < 2 && time.Now().Before(until) {
		time.Sleep(time.Millisecond)
	}
	if len(r.h.packets()) != 2 {
		t.Fatal("authenticated reader retained prelogin lifetime cap")
	}
}
