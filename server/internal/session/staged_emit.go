package session

import (
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/event"
)

const (
	maxStagedEventBytes  = 1 << 20
	maxStagedEventFrames = 4096
	stagedFramesPerCall  = 32
	stagedBytesPerCall   = 64 << 10
)

type batchSink interface{ TrySendBatch([][]byte) bool }
type epochBatchSink interface {
	batchSink
	PacedEpoch() uint64
	TrySendBatchEpoch([][]byte, uint64) bool
	CancelPacedEpoch(uint64) (uint64, bool)
}
type stagedEmission struct {
	packets        [][]byte
	next, bytes    int
	after          []func()
	atomic         bool
	binding        *emissionBinding
	cacheChanged   bool
	transportEpoch uint64
}

// emissionCapture belongs only to the scene-owned eventSink, not Session.out.
// Recursive inferred tutorial packets count towards the same bounded budget.
type emissionCapture struct {
	Sink
	packets  [][]byte
	bytes    int
	overflow bool
}

func (c *emissionCapture) Send(p []byte) {
	if c.overflow {
		return
	}
	if len(p) > maxStagedEventBytes-c.bytes || len(c.packets) >= maxStagedEventFrames {
		c.overflow = true
		c.packets = nil
		c.bytes = 0
		return
	}
	c.packets = append(c.packets, p)
	c.bytes += len(p)
}

// TryEmit encodes the scene queue head once and submits a bounded prefix without
// waiting for TCP. The caller MUST retry that same head until true, or call
// CancelPendingEmit before replacing it. True means all packets were accepted,
// not written. Entity spawns (including owner/stall tails) admit atomically;
// larger global snapshots retain FIFO order across calls. Only the owning scene
// actor may call these methods. The actor must retire unencoded old-map queue
// entries before handoff; the binding fence protects already captured batches,
// not arbitrary stale events supplied anew by a caller after a binding change.
func (s *eventSink) TryEmit(ev event.Event) bool {
	out, supported := s.out.(batchSink)
	if !supported {
		s.Emit(ev)
		return true
	}
	if s.Closed() {
		return false
	}
	if s.staged == nil {
		binding, current := s.emissionBinding()
		if !current {
			return false
		}
		transportEpoch := uint64(0)
		if transport, ok := s.out.(epochBatchSink); ok {
			transportEpoch = transport.PacedEpoch()
		}
		previousInventory, previousWarehouse, previousPet := s.inventory, s.warehouse, s.pet
		capture := &emissionCapture{Sink: s.out}
		original := s.out
		func() {
			complete := false
			defer func() {
				s.out, s.capturing = original, false
				if !complete {
					// Scene panic recovery must never leave a capture installed as
					// the real transport or a pre-admission delta baseline behind.
					s.afterCapture = nil
					s.inventory, s.warehouse, s.pet = nil, nil, nil
					s.Close()
				}
			}()
			s.out, s.capturing = capture, true
			s.Emit(ev)
			complete = true
		}()
		if capture.overflow {
			s.afterCapture = nil
			s.log.Error("staged event exceeds bounded encoding budget", "event", eventName(ev))
			s.Close()
			return false
		}
		s.staged = &stagedEmission{packets: capture.packets, bytes: capture.bytes, after: s.afterCapture, binding: binding, transportEpoch: transportEpoch, cacheChanged: s.inventory != previousInventory || s.warehouse != previousWarehouse || s.pet != previousPet}
		s.afterCapture = nil
		_, s.staged.atomic = ev.(event.EntitySpawned)
	}
	p := s.staged
	end, bytes := p.next, 0
	for end < len(p.packets) && end-p.next < stagedFramesPerCall {
		n := len(p.packets[end])
		if end > p.next && bytes+n > stagedBytesPerCall {
			break
		}
		// One existing global packet may exceed 64 KiB; admit it alone, <=1 MiB.
		bytes += n
		end++
	}
	if p.atomic {
		end = len(p.packets)
	}
	if end > p.next {
		// Adapt to tiny configured connection queues without a capacity API. A
		// rejected batch accepts nothing, even with concurrent ordinary producers.
		for !s.admitStagedPackets(out, p, p.packets[p.next:end]) {
			if p.atomic || end-p.next <= 1 {
				return false
			}
			end = p.next + (end-p.next)/2
		}
		for i := p.next; i < end; i++ {
			p.bytes -= len(p.packets[i])
			p.packets[i] = nil
		}
		p.next = end
	}
	if p.next != len(p.packets) {
		return false
	}
	s.staged = nil
	for _, after := range p.after {
		after()
	}
	return true
}

// CancelPendingEmit releases unaccepted packets. Accepted packets precede the
// subsequent native teleport/world reset in FIFO order. Call on epoch change,
// leave, and teardown; it cannot revoke bytes already accepted by the transport.
func (s *eventSink) CancelPendingEmit() {
	if s.staged != nil && s.staged.cacheChanged {
		// Encoding may have updated diff baselines before transport admission.
		// Force the next authoritative display to be a full snapshot, even if
		// the abandoned event accepted only a prefix. Never replay inventory
		// mutations or infer grants from this presentation cache.
		s.inventory, s.warehouse, s.pet = nil, nil, nil
	}
	s.staged = nil
	s.afterCapture = nil
}
func (s *eventSink) Closed() bool {
	if c, ok := s.out.(interface{ Closed() bool }); ok {
		return c.Closed()
	}
	return false
}
func (s *eventSink) afterEmit(fn func()) {
	if s.capturing {
		s.afterCapture = append(s.afterCapture, fn)
	} else {
		fn()
	}
}

// PendingEmitStarted reports whether an incomplete global event has accepted a prefix.
// EntitySpawned events always return false here: they admit atomically.
func (s *eventSink) PendingEmitStarted() bool { return s.staged != nil && s.staged.next > 0 }

// emissionBinding fences admission against concurrent leave-to-selection,
// disconnect, or map rebind. Scene-owned cancellation orders actor transfers;
// this second fence closes the race with the connection's read goroutine.
type emissionBinding struct {
	owner  *Session
	entity domain.EntityID
	scene  domain.SceneID
	epoch  uint64
}

func (s *eventSink) emissionBinding() (*emissionBinding, bool) {
	if s.sess == nil {
		return nil, true
	}
	o := s.sess
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.stage != StageInGame || o.entity != s.observer {
		return nil, false
	}
	return &emissionBinding{owner: o, entity: o.entity, scene: o.scene, epoch: o.mapEpoch}, true
}
func (s *eventSink) admitStagedPackets(out batchSink, p *stagedEmission, packets [][]byte) bool {
	b := p.binding
	if b == nil {
		return admitTransportEpoch(out, p, packets)
	}
	o := b.owner
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.stage != StageInGame || o.entity != b.entity || o.scene != b.scene || o.mapEpoch != b.epoch {
		return false
	}
	// TrySendBatch is nonblocking and has no callbacks into Session. Holding this
	// lock through admission orders accepted old bytes before the binding change
	// and its native reset/control packets; after the change, no stale batch enters.
	return admitTransportEpoch(out, p, packets)
}

func admitTransportEpoch(out batchSink, p *stagedEmission, packets [][]byte) bool {
	if transport, ok := out.(epochBatchSink); ok {
		return transport.TrySendBatchEpoch(packets, p.transportEpoch)
	}
	return out.TrySendBatch(packets)
}

// CancelReplicationEpoch is an actual world-reset boundary, not a spawn retry.
// The scene must call it before handing the sink to a new actor. It retires
// accepted-but-unwritten paced world frames while ordinary business/control
// packets retain FIFO order. At most one writer-owned old frame can finish first.
func (s *eventSink) CancelReplicationEpoch() {
	s.CancelPendingEmit()
	transport, ok := s.out.(epochBatchSink)
	if !ok {
		return
	}
	// Same lock order as admission: Session.mu then the transport queue mutex.
	if s.sess != nil {
		s.sess.mu.Lock()
		defer s.sess.mu.Unlock()
	}
	_, _ = transport.CancelPacedEpoch(transport.PacedEpoch())
}
