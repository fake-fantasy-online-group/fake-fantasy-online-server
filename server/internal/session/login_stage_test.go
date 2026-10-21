package session

import (
	"bytes"
	"context"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/store"
	"testing"
)

// Rejected connections must not look up accounts or reauthenticate.
type noLoginLookupStore struct {
	store.Store
	t *testing.T
}

func (s *noLoginLookupStore) AccountByName(context.Context, string) (*store.Account, error) {
	s.t.Fatal("rejected login reached account lookup")
	return nil, nil
}
func assertReconnectReply(t *testing.T, sink *capSink, start int) {
	t.Helper()
	sink.mu.Lock()
	defer sink.mu.Unlock()
	packets := sink.pkts[start:]
	want := protocol.LoginResult(1, "当前连接无法登录，请重新连接")
	if len(packets) != 1 || !bytes.Equal(packets[0], want) {
		t.Fatalf("wanted exactly one reconnect failure, got %x", packets)
	}
}
func TestLoginStageRejectionRepliesWithoutChangingIdentity(t *testing.T) {
	for _, stage := range []Stage{StageAuthed, StageInGame, StageClosed} {
		for _, name := range []string{"alice", "bob", "missing"} {
			t.Run(map[Stage]string{StageAuthed: "selection", StageInGame: "world", StageClosed: "closed"}[stage]+"/"+name, func(t *testing.T) {
				r := newAuthRig(t)
				_, char := r.mkAccountWithChar("alice", "甲")
				r.mkAccountWithChar("bob", "乙")
				registry := NewAccountSessions()
				r.sess.deps.AccountSessions = registry
				r.sess.onLogin(protocol.Request{S1: "alice", S2: "test-password"})
				if r.stage() != StageAuthed {
					t.Fatal("initial login failed")
				}
				account := r.sess.account
				if stage == StageClosed {
					r.sess.OnClose("test close")
				} else if stage == StageInGame {
					r.sess.stage = stage
					r.sess.char = char
					r.sess.entity = 77
					r.sess.scene = domain.SceneID{MapID: 7}
				}
				oldChar, oldEntity, oldScene, oldLease := r.sess.char, r.sess.entity, r.sess.scene, r.sess.lease
				start := len(r.sink.pkts)
				r.sess.deps.Store = &noLoginLookupStore{Store: r.st, t: t}
				r.sess.onLogin(protocol.Request{S1: name, S2: "wrong-password"})
				assertReconnectReply(t, r.sink, start)
				if r.stage() != stage || r.sess.account != account || r.sess.char != oldChar || r.sess.entity != oldEntity || r.sess.scene != oldScene || r.sess.lease != oldLease {
					t.Fatal("rejected login changed session identity or world binding")
				}
				owner := registry.byAccount[account.ID]
				if stage == StageClosed {
					if owner != nil {
						t.Fatal("closed session reclaimed account")
					}
				} else if owner != r.sess || len(registry.byAccount) != 1 {
					t.Fatal("rejected login changed account lease")
				}
			})
		}
	}
}
func TestLoginStageRecheckRepliesWithoutClaimingAccount(t *testing.T) {
	for _, closeSession := range []bool{false, true} {
		t.Run(map[bool]string{false: "competing_login", true: "close"}[closeSession], func(t *testing.T) {
			r := newAuthRig(t)
			alice, _ := r.mkAccountWithChar("alice", "甲")
			bob, _ := r.mkAccountWithChar("bob", "乙")
			registry := NewAccountSessions()
			r.sess.deps.AccountSessions = registry
			start := 0
			r.sess.deps.Store = &changingLoginStore{Store: r.st, afterPasswordCheck: func() {
				if closeSession {
					r.sess.OnClose("closed during login")
				} else {
					// Deterministically interleave real handlers before the stage recheck.
					r.sess.onLogin(protocol.Request{S1: "bob", S2: "test-password"})
					if r.stage() != StageAuthed || r.sess.account.ID != bob.ID {
						t.Fatal("competing login failed")
					}
				}
				start = len(r.sink.pkts)
			}}
			r.sess.onLogin(protocol.Request{S1: "alice", S2: "test-password"})
			assertReconnectReply(t, r.sink, start)
			if registry.byAccount[alice.ID] != nil {
				t.Fatal("losing login claimed account")
			}
			if closeSession {
				if r.stage() != StageClosed || r.sess.account != nil || len(registry.byAccount) != 0 {
					t.Fatal("login resurrected closed session")
				}
			} else if r.stage() != StageAuthed || r.sess.account.ID != bob.ID || registry.byAccount[bob.ID] != r.sess || len(registry.byAccount) != 1 {
				t.Fatal("losing login replaced winning identity")
			}
		})
	}
}

func TestReconnectAfterLoginStageRejectionPreservesNewLease(t *testing.T) {
	r := newAuthRig(t)
	alice, _ := r.mkAccountWithChar("alice", "甲")
	registry := NewAccountSessions()
	r.sess.deps.AccountSessions = registry
	request := protocol.Request{S1: "alice", S2: "test-password"}
	r.sess.onLogin(request)
	start := len(r.sink.pkts)
	r.sess.onLogin(request)
	assertReconnectReply(t, r.sink, start)
	freshSink := &capSink{}
	fresh := New(freshSink, r.sess.deps)
	fresh.onLogin(request)
	if code, sent := freshSink.respCode(); !sent || code != 0 || fresh.stage != StageAuthed || !freshSink.sawOpcode(0x8004) {
		t.Fatal("fresh connection did not authenticate normally")
	}
	if r.stage() != StageClosed || !r.sink.closed {
		t.Fatal("old connection was not replaced")
	}
	r.sess.OnClose("late transport close")
	if registry.byAccount[alice.ID] != fresh || len(registry.byAccount) != 1 {
		t.Fatal("late close removed new session's lease")
	}
}
