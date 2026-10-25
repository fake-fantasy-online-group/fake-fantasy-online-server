package net

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/wire"
)

func quietOptions() Options { return Options{Log: slog.New(slog.NewTextHandler(io.Discard, nil))} }

func TestTrySendBatchAtomicBoundsAndReserve(t *testing.T) {
	cli, srv := net.Pipe()
	defer cli.Close()
	o := quietOptions()
	o.OutBuffer = 16
	c := newConn(1, srv, newHandler(), o)
	defer c.Close()
	packets := make([][]byte, 14)
	for i := range packets {
		packets[i] = []byte{1, 2}
	}
	if !c.TrySendBatch(packets) {
		t.Fatal("initial capacity rejected")
	}
	if c.TrySendBatch([][]byte{{3, 4}, {5, 6}}) {
		t.Fatal("reserve consumed")
	}
	if len(c.out) != 14 || c.outBytes.Load() != 28 {
		t.Fatal("rejected batch changed queue")
	}
	c.Send([]byte{7, 8})
	c.Send([]byte{9, 10})
	if c.Closed() || len(c.out) != 16 {
		t.Fatal("ordinary traffic cannot use reserve")
	}
	c.Send([]byte{11, 12})
	if !c.Closed() {
		t.Fatal("ordinary overflow lost slow-client protection")
	}
}

func TestTrySendBatchBytesBound(t *testing.T) {
	cli, srv := net.Pipe()
	defer cli.Close()
	c := newConn(1, srv, newHandler(), quietOptions())
	defer c.Close()
	p := make([]byte, wire.MaxFrame)
	for i := 0; i < 3; i++ {
		if !c.TrySendBatch([][]byte{p}) {
			t.Fatalf("admission %d", i)
		}
	}
	if c.TrySendBatch([][]byte{p}) || c.outBytes.Load() != 3*wire.MaxFrame || c.Closed() {
		t.Fatal("byte bound must reject atomically without closing")
	}
	c.Send(p)
	if c.Closed() || c.outBytes.Load() != stagedQueueByteWatermark {
		t.Fatal("ordinary packet could not use byte reserve")
	}
	if c.TrySendBatch([][]byte{make([]byte, wire.MaxFrame+1)}) {
		t.Fatal("oversized frame admitted")
	}
}

// Uses the real TCP writer, native protocol encoding and sequenced encryption.
// More than 3x the production queue capacity is submitted in nonblocking chunks;
// a deliberately slow reader must delay this client, not its caller/other peers.
func TestTrySendBatchTCPDenseSnapshot(t *testing.T) {
	for _, total := range []int{1160, 4097} {
		for _, slow := range []bool{false, true} {
			t.Run(fmt.Sprintf("frames=%d/slow=%v", total, slow), func(t *testing.T) {
				ln, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				defer ln.Close()
				client, err := net.Dial("tcp", ln.Addr().String())
				if err != nil {
					t.Fatal(err)
				}
				defer client.Close()
				server, err := ln.Accept()
				if err != nil {
					t.Fatal(err)
				}
				if slow {
					if err := server.(*net.TCPConn).SetWriteBuffer(4096); err != nil {
						t.Fatal(err)
					}
					if err := client.(*net.TCPConn).SetReadBuffer(4096); err != nil {
						t.Fatal(err)
					}
				}
				o := quietOptions()
				o.WriteWait = 5 * time.Second
				c := newConn(1, server, newHandler(), o)
				defer c.Close()
				keys := testKeys()
				c.SetKeys(keys)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				go c.writeLoop(ctx)
				frames := make([][]byte, total)
				for i := range frames {
					ps := protocol.Encode(1, event.EntitySpawned{ID: domain.EntityID(i + 100), Kind: domain.KindMonster, Name: "dense", HP: 100, MaxHP: 100, Pos: domain.Pos{MapID: 28603, X: 4384, Y: 6688}})
					if len(ps) != 1 {
						t.Fatalf("monster frame count %d", len(ps))
					}
					frames[i] = ps[0]
				}
				readDone := make(chan error, 1)
				go func() {
					var opener wire.Opener
					for i, p := range frames {
						if slow && i%16 == 0 {
							time.Sleep(time.Millisecond)
						}
						_ = client.SetReadDeadline(time.Now().Add(5 * time.Second))
						body, err := wire.ReadFrame(client)
						if err != nil {
							readDone <- err
							return
						}
						plain, err := opener.OpenInPlace(&keys, body)
						if err != nil {
							readDone <- err
							return
						}
						if len(plain) < 8 || binary.LittleEndian.Uint64(plain) != uint64(i) || !bytes.Equal(plain[8:], p) {
							readDone <- fmt.Errorf("frame %d sequence/payload differs", i)
							return
						}
					}
					readDone <- nil
				}()
				start := time.Now()
				accepted := 0
				rejected := 0
				for accepted < total {
					end := accepted + 32
					if end > total {
						end = total
					}
					before := time.Now()
					ok := c.TrySendBatch(frames[accepted:end])
					elapsed := time.Since(before)
					if elapsed > 100*time.Millisecond {
						t.Fatalf("nonblocking admission took %v", elapsed)
					}
					if ok {
						accepted = end
					} else {
						rejected++
						time.Sleep(100 * time.Microsecond)
					}
					if time.Since(start) > 10*time.Second {
						t.Fatal("snapshot timed out")
					}
				}
				if err := <-readDone; err != nil {
					t.Fatal(err)
				}
				if c.Closed() || c.outBytes.Load() != 0 {
					t.Fatalf("closed=%v queued bytes=%d", c.Closed(), c.outBytes.Load())
				}
				t.Logf("frames=%d bytes=%d rejected=%d duration=%v", total, total*len(frames[0]), rejected, time.Since(start))
			})
		}
	}

}

func TestTrySendBatchStalledClientWriteDeadline(t *testing.T) {
	cli, srv := net.Pipe()
	defer cli.Close()
	o := quietOptions()
	o.WriteWait = 25 * time.Millisecond
	c := newConn(1, srv, newHandler(), o)
	defer c.Close()
	c.SetKeys(testKeys())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.writeLoop(ctx)
	if !c.TrySendBatch([][]byte{{1, 2}}) {
		t.Fatal("initial admission")
	}
	select {
	case <-c.closeCh:
	case <-time.After(time.Second):
		t.Fatal("stalled writer was not closed")
	}
	if c.TrySendBatch([][]byte{{1, 2}}) {
		t.Fatal("closed transport accepted batch")
	}
}

func TestNormalGlobalBurstPreservesLegacyAdmissionAbovePacedWatermark(t *testing.T) {
	cli, srv := net.Pipe()
	defer cli.Close()
	c := newConn(1, srv, newHandler(), quietOptions())
	defer c.Close()
	// Hold one MiB of paced presentation. No writer runs: admission/backpressure
	// is deterministic and cannot hide behind the kernel's TCP send buffer.
	if !c.TrySendBatch([][]byte{make([]byte, 1<<20)}) {
		t.Fatal("paced seed refused")
	}
	response := domain.EndgameResponse{Kind: "rift.state", OK: true, Rift: &domain.EndgameRiftView{RunID: "budget", Phase: "active", Revision: "1"}}
	base, _ := json.Marshal(response)
	response.Message = strings.Repeat("x", domain.EndgameRiftSnapshotBytes-len(base)-13)
	raw, _ := json.Marshal(response)
	response.Message += strings.Repeat("x", domain.EndgameRiftSnapshotBytes-len(raw))
	pages, err := domain.PageEndgameRift(response, strings.Repeat("s", 64))
	if err != nil {
		t.Fatal(err)
	}
	packets := make([][]byte, 0, 32)
	for _, page := range pages {
		packets = append(packets, protocol.Encode(1, event.EndgameMessage{Who: 1, Message: page})...)
	}
	inv := event.InventorySnapshot{Who: 1}
	for i := 0; i < domain.DefaultBagSlots; i++ {
		inv.Items = append(inv.Items, event.InventoryItemView{BagIndex: int32(i), Item: 1, Count: 1, Name: "item", Desc: strings.Repeat("d", 2500)})
	}
	wardrobe := event.WardrobeSnapshot{Who: 1, Capacity: 200}
	for i := 0; i < 200; i++ {
		wardrobe.Items = append(wardrobe.Items, event.WardrobeItemView{Item: domain.ItemID(i + 1), Name: "item", Desc: strings.Repeat("d", 5000)})
	}
	packets = append(packets, protocol.Encode(1, inv)...)
	packets = append(packets, protocol.Encode(1, wardrobe)...)
	for _, packet := range packets {
		if len(packet) > wire.MaxFrame-64 {
			t.Fatal("fixture is not native-frame legal")
		}
		c.Send(packet)
	}
	if c.Closed() {
		t.Fatal("valid low-count control burst hit a new byte rejection")
	}
	if len(c.out) != 1+len(packets) || c.outBytes.Load() <= stagedQueueByteWatermark {
		t.Fatal("fixture did not exceed paced watermark")
	}
	if c.TrySendBatch([][]byte{{1, 2}}) {
		t.Fatal("paced entities grew queue above byte watermark")
	}
	t.Logf("globalFrames=%d retainedBytes=%d pacedPaused=true closed=false", len(packets), c.outBytes.Load())
	// Normal controls still use the original packet capacity, even above water.
	c.Send([]byte{1, 2})
	if c.Closed() {
		t.Fatal("small control rejected above paced watermark")
	}
}

func TestPacedFrameReserveFitsMaximumExplicitBootstrap(t *testing.T) {
	cli, srv := net.Pipe()
	defer cli.Close()
	c := newConn(1, srv, newHandler(), quietOptions())
	defer c.Close()
	packets := make([][]byte, 960)
	for i := range packets {
		packets[i] = []byte{1, 2}
	}
	if !c.TrySendBatch(packets) {
		t.Fatal("paced queue admission")
	}
	// 20 fixed cold-login frames + maximum29Riftpages + six asynchronous social
	// snapshots. Unrelated concurrent traffic retains the original full-queue
	// protection; this is the explicit initial producer, not an unlimited promise.
	for i := 0; i < 55; i++ {
		c.Send([]byte{1, 2})
	}
	if c.Closed() || len(c.out) != 1015 {
		t.Fatal("64 reserved frames did not fit bootstrap")
	}
	if c.TrySendBatch([][]byte{{1, 2}}) {
		t.Fatal("paced traffic consumed control headroom")
	}
}

// This is a deterministic architecture boundary measurement, not an assertion
// that the current64slot policy protects arbitrary legacy control bursts. No
// writer runs; candidate headroom means the producer voluntarily stops earlier.
func TestControlBurstReserveBoundaryEvidence(t *testing.T) {
	for _, cleanup := range []int{100, 400} {
		for _, reserve := range []int{64, 256, 512} {
			t.Run(fmt.Sprintf("cleanup%d/reserve%d", cleanup, reserve), func(t *testing.T) {
				cli, srv := net.Pipe()
				defer cli.Close()
				c := newConn(1, srv, newHandler(), quietOptions())
				defer c.Close()
				queued := 1024 - reserve
				packets := make([][]byte, queued)
				for i := range packets {
					packets[i] = []byte{1, 2}
				}
				if !c.TrySendBatch(packets) {
					t.Fatal("paced occupancy fixture rejected")
				}
				firstClosed := 0
				// One native removal per source NPC/drop, followed by maximum explicit
				// nonworld bootstrap. Their semantic contents do not affect packet admission.
				for i := 0; i < cleanup+55; i++ {
					c.Send(protocol.Encode(1, event.EntityDespawned{ID: domain.EntityID(i + 1), Kind: domain.KindDrop, Reason: event.DespawnTimeout})[0])
					if c.Closed() && firstClosed == 0 {
						firstClosed = i + 1
					}
				}
				expectedClose := cleanup+55 > reserve
				if c.Closed() != expectedClose {
					t.Fatal("capacity arithmetic differs from actual Conn")
				}
				t.Logf("pacedFrames=%d controlFrames=%d retained=%d closed=%v firstCloseAtControl=%d", queued, cleanup+55, len(c.out), c.Closed(), firstClosed)
			})
		}
	}
}
