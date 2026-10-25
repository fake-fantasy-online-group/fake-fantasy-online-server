package net

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/wire"
)

func TestPacedEpochPurgePreservesControlsAndAccounting(t *testing.T) {
	cli, srv := net.Pipe()
	defer cli.Close()
	c := newConn(1, srv, newHandler(), quietOptions())
	defer c.Close()
	a, b, newWorld := []byte{1, 2, 3}, []byte{4, 5, 6, 7}, []byte{8, 9}
	old := c.PacedEpoch()
	c.Send(a)
	if !c.TrySendBatchEpoch([][]byte{{11, 12}, {13, 14, 15}}, old) {
		t.Fatal("old paced admission")
	}
	c.Send(b)
	next, ok := c.CancelPacedEpoch(old)
	if !ok || next == old {
		t.Fatal("epoch was not retired")
	}
	if len(c.out) != 2 || c.outBytes.Load() != int64(len(a)+len(b)) {
		t.Fatal("purge changed controls or retained old bytes")
	}
	if c.TrySendBatchEpoch([][]byte{{99, 99}}, old) {
		t.Fatal("stale encoded batch admitted")
	}
	if !c.TrySendBatchEpoch([][]byte{newWorld}, next) {
		t.Fatal("new epoch rejected")
	}
	if got, ok := c.CancelPacedEpoch(old); ok || got != next {
		t.Fatal("stale cancellation retired new epoch")
	}
	for i, want := range [][]byte{a, b, newWorld} {
		got, ok := c.takeOutbound()
		if !ok || !bytes.Equal(got, want) {
			t.Fatalf("FIFO row%d changed", i)
		}
	}
	if c.outBytes.Load() != 0 || len(c.out) != 0 {
		t.Fatal("dequeue/purge double counted bytes")
	}
}

func TestPacedEpochPurgeAllowsLegacyMaplessCleanup(t *testing.T) {
	for _, count := range []int{100, 400, 1000} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			cli, srv := net.Pipe()
			defer cli.Close()
			c := newConn(1, srv, newHandler(), quietOptions())
			defer c.Close()
			epoch := c.PacedEpoch()
			packets := make([][]byte, 960)
			for i := range packets {
				packets[i] = []byte{15, 128, 1, 0, 0, 0}
			}
			if !c.TrySendBatchEpoch(packets, epoch) {
				t.Fatal("full paced occupancy rejected")
			}
			before, after := []byte{1, 16}, []byte{2, 16}
			c.Send(before)
			c.Send(after)
			if _, ok := c.CancelPacedEpoch(epoch); !ok {
				t.Fatal("reset purge failed")
			}
			for i := 0; i < count; i++ {
				c.Send(wire.Encode(0x8010, []byte{byte(i), byte(i >> 8), 0, 0, 0}))
			}
			c.Send(wire.Encode(0x803a, make([]byte, 12)))
			if c.Closed() || len(c.out) != count+3 || cap(c.out) != 1024 {
				t.Fatalf("legacy cleanup closed=%v queued=%d", c.Closed(), len(c.out))
			}
			a, _ := c.takeOutbound()
			b, _ := c.takeOutbound()
			if !bytes.Equal(a, before) || !bytes.Equal(b, after) {
				t.Fatal("normal controls moved at reset")
			}
			total := int64(0)
			for {
				p, ok := c.takeOutbound()
				if !ok {
					break
				}
				total += int64(len(p))
			}
			if c.outBytes.Load() != 0 {
				t.Fatal("cleanup accounting did not drain")
			}
			t.Logf("oldPaced=960 cleanup=%d controlsRetained=2 reset=1 drainedBytes=%d capacity=%d", count, total, cap(c.out))
		})
	}
}

type signaledWriter struct {
	net.Conn
	once    sync.Once
	entered chan struct{}
}

func (c *signaledWriter) Write(p []byte) (int, error) {
	c.once.Do(func() { close(c.entered) })
	return c.Conn.Write(p)
}

func TestPacedEpochInFlightPrecedesResetAndKeepsSequence(t *testing.T) {
	cli, srv := net.Pipe()
	defer cli.Close()
	raw := &signaledWriter{Conn: srv, entered: make(chan struct{})}
	c := newConn(1, raw, newHandler(), quietOptions())
	defer c.Close()
	keys := testKeys()
	c.SetKeys(keys)
	epoch := c.PacedEpoch()
	inflight, discard, control, reset := wire.Encode(0x800f, []byte{1}), wire.Encode(0x800f, []byte{2}), wire.Encode(0x8007, []byte{3}), wire.Encode(0x803a, []byte{4})
	if !c.TrySendBatchEpoch([][]byte{inflight, discard}, epoch) {
		t.Fatal("admit")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.writeLoop(ctx)
	select {
	case <-raw.entered:
	case <-time.After(time.Second):
		t.Fatal("writer did not take first frame")
	}
	c.Send(control)
	if _, ok := c.CancelPacedEpoch(epoch); !ok {
		t.Fatal("cancel")
	}
	c.Send(reset)
	var opener wire.Opener
	for i, want := range [][]byte{inflight, control, reset} {
		_ = cli.SetReadDeadline(time.Now().Add(time.Second))
		body, err := wire.ReadFrame(cli)
		if err != nil {
			t.Fatal(err)
		}
		plain, err := opener.OpenInPlace(&keys, body)
		if err != nil {
			t.Fatal(err)
		}
		if binary.LittleEndian.Uint64(plain) != uint64(i) || !bytes.Equal(plain[8:], want) {
			t.Fatalf("frame%d reordered or skipped sequence", i)
		}
	}
	if c.outBytes.Load() != 0 {
		t.Fatal("in-flight bytes decremented twice or leaked")
	}
}

func TestPacedEpochConcurrentAdmissionCancellationAndClose(t *testing.T) {
	cli, srv := net.Pipe()
	defer cli.Close()
	c := newConn(1, srv, newHandler(), quietOptions())
	defer c.Close()
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			c.Send(wire.Encode(0x8007, []byte{byte(i)}))
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			epoch := c.PacedEpoch()
			c.TrySendBatchEpoch([][]byte{{15, 128, 1}, {15, 128, 2}}, epoch)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			c.CancelPacedEpoch(c.PacedEpoch())
		}
	}()
	wg.Wait()
	c.CancelPacedEpoch(c.PacedEpoch())
	if c.Closed() || len(c.out) != 100 || c.outBytes.Load() != 300 {
		t.Fatal("concurrent purge corrupted normal queue")
	}
	for i := 0; i < 100; i++ {
		p, ok := c.takeOutbound()
		if !ok || len(p) != 3 || p[2] != byte(i) {
			t.Fatal("normal producer FIFO changed")
		}
	}
	if c.outBytes.Load() != 0 {
		t.Fatal("bytes not released")
	}
	wg.Add(3)
	go func() { defer wg.Done(); c.Close() }()
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			c.TrySendBatchEpoch([][]byte{{1, 2}}, c.PacedEpoch())
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			c.CancelPacedEpoch(c.PacedEpoch())
		}
	}()
	wg.Wait()
	if !c.Closed() || c.outBytes.Load() < 0 {
		t.Fatal("close accounting")
	}
}
