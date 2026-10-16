package session

import (
	"sync"
	"testing"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
)

func TestAuthWorkBoundedConcurrencyAndRelease(t *testing.T) {
	l := NewAuthWorkLimiter(AuthWorkOptions{Concurrent: 2})
	now := time.Unix(100, 0)
	a, ok := l.acquire("one", "account", now)
	if !ok {
		t.Fatal("first denied")
	}
	b, ok := l.acquire("two", "account", now)
	if !ok {
		t.Fatal("second denied")
	}
	if _, ok = l.acquire("three", "account", now); ok {
		t.Fatal("unbounded expensive concurrency")
	}
	a()
	a() // callers cannot release another slot by mistake
	c, ok := l.acquire("three", "account", now)
	if !ok {
		t.Fatal("slot not released")
	}
	if _, ok = l.acquire("four", "account", now); ok {
		t.Fatal("double release bypassed bound")
	}
	b()
	c()
}
func TestAuthWorkAttemptsSurviveReconnectWithoutCrossPeerLockout(t *testing.T) {
	l := NewAuthWorkLimiter(AuthWorkOptions{AccountBurst: 2, AccountRate: 1})
	now := time.Unix(100, 0)
	for i := 0; i < 2; i++ {
		release, ok := l.acquire("one", "alice", now)
		if !ok {
			t.Fatal("burst denied")
		}
		release()
	}
	if _, ok := l.acquire("one", "alice", now); ok {
		t.Fatal("same peer/account can reset attempts")
	}
	release, ok := l.acquire("two", "alice", now)
	if !ok {
		t.Fatal("other peer locked out")
	}
	release()
	release, ok = l.acquire("one", "alice", now.Add(time.Second))
	if !ok {
		t.Fatal("attempt credit never refilled")
	}
	release()
}
func TestAuthWorkPeerBudgetAndBoundedKeyRetention(t *testing.T) {
	l := NewAuthWorkLimiter(AuthWorkOptions{PeerBurst: 1, MaxKeys: 2, Retention: time.Minute})
	now := time.Unix(100, 0)
	release, ok := l.acquire("one", "alice", now)
	if !ok {
		t.Fatal("first denied")
	}
	release()
	if _, ok = l.acquire("one", "bob", now); ok {
		t.Fatal("changing account bypassed IP budget")
	}
	if _, ok = l.acquire("two", "alice", now); ok {
		t.Fatal("key capacity unbounded")
	}
	if len(l.buckets) > 2 {
		t.Fatal("too many retained keys")
	}
	release, ok = l.acquire("two", "alice", now.Add(2*time.Minute))
	if !ok {
		t.Fatal("expired keys were not reclaimed")
	}
	release()
}
func TestAuthWorkConcurrentAcquisition(t *testing.T) {
	l := NewAuthWorkLimiter(AuthWorkOptions{Concurrent: 2, AccountBurst: 100, PeerBurst: 100})
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 100; n++ {
				if release, ok := l.acquire("peer", "account", time.Now()); ok {
					release()
				}
			}
		}()
	}
	wg.Wait()
	if l.active != 0 {
		t.Fatal("work slot leaked")
	}
}
func TestAuthWorkGateOnlyCredentialRequests(t *testing.T) {
	s := &Session{deps: Deps{AuthWork: NewAuthWorkLimiter(AuthWorkOptions{})}}
	// Non-auth requests must not touch even a transport, much less consume auth slots.
	for _, kind := range []protocol.ReqKind{protocol.ReqMove, protocol.ReqPing, protocol.ReqAttack} {
		release, ok := s.beginAuthWork(protocol.Request{Kind: kind})
		if !ok {
			t.Fatal("normal gameplay gated")
		}
		release()
	}
	if s.deps.AuthWork.active != 0 || len(s.deps.AuthWork.buckets) != 0 {
		t.Fatal("gameplay spent auth budget")
	}
}
func TestMovementIngressIndependentBoundedBurstAndMobileCadence(t *testing.T) {
	var flood, healthy movementIngress
	now := time.Unix(100, 0)
	for i := 0; i < 80; i++ {
		if !flood.allow(now, 0, 0) {
			t.Fatal("configured burst denied")
		}
	}
	if flood.allow(now, 0, 0) {
		t.Fatal("unbounded producer")
	}
	for i := 0; i < 1000; i++ {
		// Client combat flush may run every60ms, followed by a stop report.
		if !healthy.allow(now.Add(time.Duration(i)*60*time.Millisecond), 0, 0) ||
			!healthy.allow(now.Add(time.Duration(i)*60*time.Millisecond), 0, 0) {
			t.Fatal("normal mobile movement cadence denied")
		}
	}
	if !flood.allow(now.Add(time.Second), 0, 0) {
		t.Fatal("quota did not refill")
	}
}

func TestAuthenticatedAuthBudgetDenialDoesNotDisconnect(t *testing.T) {
	for _, kind := range []protocol.ReqKind{protocol.ReqChangePassword, protocol.ReqChangeSecurityCode, protocol.ReqSetItemLock} {
		sink := &capSink{}
		s := &Session{sink: sink}
		s.rejectAuthWork(protocol.Request{Kind: kind})
		if sink.closed || len(sink.pkts) != 1 {
			t.Fatal("legitimate credential burst logged player out", kind)
		}
	}
}
