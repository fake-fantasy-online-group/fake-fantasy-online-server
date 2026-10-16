package session

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"time"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
)

// AuthWorkOptions bounds expensive credential work without changing password cost.
// Peer budgets are generous enough for a shared NAT. Peer+account budgets prevent
// reconnects resetting attempts without letting another IP lock out an account.
type AuthWorkOptions struct {
	Concurrent   int
	PeerRate     float64
	PeerBurst    int
	AccountRate  float64
	AccountBurst int
	MaxKeys      int
	Retention    time.Duration
}

type authBucket struct {
	at     time.Time
	tokens float64
}
type AuthWorkLimiter struct {
	mu        sync.Mutex
	opts      AuthWorkOptions
	active    int
	buckets   map[string]authBucket
	lastSweep time.Time
}

func NewAuthWorkLimiter(o AuthWorkOptions) *AuthWorkLimiter {
	if o.Concurrent <= 0 {
		o.Concurrent = 4
	}
	if o.PeerRate <= 0 {
		o.PeerRate = 2
	}
	if o.PeerBurst <= 0 {
		o.PeerBurst = 30
	}
	if o.AccountRate <= 0 {
		o.AccountRate = 0.2
	}
	if o.AccountBurst <= 0 {
		o.AccountBurst = 5
	}
	if o.MaxKeys <= 0 {
		o.MaxKeys = 8192
	}
	if o.Retention <= 0 {
		o.Retention = 10 * time.Minute
	}
	return &AuthWorkLimiter{opts: o, buckets: make(map[string]authBucket)}
}
func (l *AuthWorkLimiter) take(key string, now time.Time, rate float64, burst int) bool {
	b, exists := l.buckets[key]
	if !exists {
		if len(l.buckets) >= l.opts.MaxKeys {
			return false
		}
		b.tokens = float64(burst)
	} else if now.After(b.at) {
		b.tokens += now.Sub(b.at).Seconds() * rate
		if b.tokens > float64(burst) {
			b.tokens = float64(burst)
		}
	}
	b.at = now
	ok := b.tokens >= 1
	if ok {
		b.tokens--
	}
	l.buckets[key] = b
	return ok
}
func (l *AuthWorkLimiter) acquire(peer, account string, now time.Time) (func(), bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.lastSweep.IsZero() || now.Sub(l.lastSweep) >= time.Minute {
		for k, b := range l.buckets {
			if now.Sub(b.at) >= l.opts.Retention {
				delete(l.buckets, k)
			}
		}
		l.lastSweep = now
	}
	if l.active >= l.opts.Concurrent {
		return nil, false
	}
	// Hash keys to retain no plaintext account names, and bound attacker supplied key size.
	pair := fmt.Sprintf("account:%x", sha256.Sum256([]byte(peer+"\x00"+account)))
	ip := fmt.Sprintf("peer:%x", sha256.Sum256([]byte(peer)))
	if !l.take(ip, now, l.opts.PeerRate, l.opts.PeerBurst) ||
		!l.take(pair, now, l.opts.AccountRate, l.opts.AccountBurst) {
		return nil, false
	}
	l.active++
	var once sync.Once
	return func() { once.Do(func() { l.mu.Lock(); l.active--; l.mu.Unlock() }) }, true
}

func (s *Session) beginAuthWork(req protocol.Request) (func(), bool) {
	if s.deps.AuthWork == nil {
		return func() {}, true
	}
	switch req.Kind {
	case protocol.ReqLogin, protocol.ReqRegister, protocol.ReqResetPassword,
		protocol.ReqChangePassword, protocol.ReqChangeSecurityCode:
	case protocol.ReqSetItemLock:
		if req.Flag != 0 {
			return func() {}, true
		}
	default:
		return func() {}, true
	}
	peer := fmt.Sprintf("session:%d", s.sink.SessionID())
	if remote, ok := s.sink.(interface{ RemoteIP() string }); ok {
		peer = remote.RemoteIP()
	}
	account := req.S1
	if req.Kind != protocol.ReqLogin && req.Kind != protocol.ReqRegister && req.Kind != protocol.ReqResetPassword {
		account = "authenticated"
		s.mu.Lock()
		if s.account != nil {
			account = s.account.Username
		}
		s.mu.Unlock()
	}
	return s.deps.AuthWork.acquire(peer, account, time.Now())
}

// Authenticated users can legitimately unlock several items in a row. Budget
// rejection must not log them out; retain the normal per-operation failure UI.
func (s *Session) rejectAuthWork(req protocol.Request) {
	const message = "操作过于频繁或服务繁忙，请稍后重试"
	switch req.Kind {
	case protocol.ReqChangePassword:
		s.sendAccountResult(accountResultPassword, accountResultFailed, message)
	case protocol.ReqChangeSecurityCode:
		s.sendAccountResult(accountResultSecurity, accountResultFailed, message)
	case protocol.ReqSetItemLock:
		s.sink.Send(protocol.AccountSecurityResult(accountResultUnlock, accountResultFailed, message,
			req.Tab, req.Slot, domain.ItemID(req.ID32)))
	default:
		s.sink.Close()
	}
}
