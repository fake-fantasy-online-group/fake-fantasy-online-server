package net

import (
	"bytes"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/wire"
	"sync"
	"testing"
	"time"
)

func TestSetKeysOwnsSnapshot(t *testing.T) {
	c := new(Conn)
	k := testKeys()
	wantAES, wantMAC := bytes.Clone(k.AES), bytes.Clone(k.MAC)
	c.SetKeys(k)
	k.AES[0] ^= 255
	k.MAC[0] ^= 255
	old := c.keys.Load()
	if !bytes.Equal(old.AES, wantAES) || !bytes.Equal(old.MAC, wantMAC) {
		t.Fatal("keys alias caller")
	}
	c.SetKeys(k)
	if old == c.keys.Load() || !bytes.Equal(old.AES, wantAES) {
		t.Fatal("rekey mutated old snapshot")
	}
}
func TestConcurrentKeySnapshotsAndDirections(t *testing.T) {
	c := new(Conn)
	c.SetKeys(testKeys())
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		for i := 0; i < 300; i++ {
			k := testKeys()
			k.AES[0] = byte(i)
			k.MAC[0] = byte(i)
			c.SetKeys(k)
		}
	}()
	for d := 0; d < 2; d++ {
		go func() {
			defer wg.Done()
			var s wire.Sealer
			var o wire.Opener
			for i := 0; i < 300; i++ {
				k := c.keys.Load()
				f, err := s.SealSequenced(k, uint64(i), []byte("owned"))
				if err != nil {
					t.Error(err)
					return
				}
				if _, err = o.OpenInPlace(k, f[4:]); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
}

// The real read loop hands payload ownership to consumers, which may enqueue
// it for later processing. It must survive subsequent frames and rekeys.
func TestConnRetainedPayloadAcrossRekey(t *testing.T) {
	r := newRig(t, Options{})
	received := make(chan []byte, 3)
	r.h.mu.Lock()
	r.h.onPacket = func(_ uint16, p []byte) { received <- p }
	r.h.mu.Unlock()
	r.handshake()
	keys := []wire.Keys{testKeys(), testKeys(), testKeys()}
	keys[1].AES[0] ^= 7
	keys[1].MAC[0] ^= 9
	var retained [][]byte
	for i, k := range keys {
		r.conn.SetKeys(k)
		payload := bytes.Repeat([]byte{byte(i + 1)}, 100)
		frame, err := wire.Seal(k, wire.EncodeSequenced(uint64(i), wire.Encode(42, payload)))
		if err != nil {
			t.Fatal(err)
		}
		r.writeAll(frame)
		select {
		case p := <-received:
			retained = append(retained, p)
		case <-time.After(2 * time.Second):
			t.Fatal("missing inbound payload")
		}
		// The write loop independently observes the same key update and maintains
		// its sequence across key replacement.
		r.conn.Send(wire.Encode(43, payload))
		_ = r.client.SetReadDeadline(time.Now().Add(2 * time.Second))
		body, err := wire.ReadFrame(r.client)
		if err != nil {
			t.Fatal(err)
		}
		plain, err := wire.Open(k, body)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(plain, wire.EncodeSequenced(uint64(i), wire.Encode(43, payload))) {
			t.Fatal("downlink sequence or rekey mismatch")
		}
	}
	for i, p := range retained {
		if !bytes.Equal(p, bytes.Repeat([]byte{byte(i + 1)}, 100)) {
			t.Fatal("retained payload overwritten")
		}
	}
}
