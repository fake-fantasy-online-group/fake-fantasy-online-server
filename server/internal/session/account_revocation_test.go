package session

import (
	"context"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/online"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/protocol"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/store"
)

func credentialHash(t *testing.T, value string) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(value), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	return string(hash)
}

func credentialRig(t *testing.T) *authRig {
	t.Helper()
	r := newAuthRig(t)
	_, err := r.st.CreateRegisteredAccount(context.Background(), "owner",
		credentialHash(t, "old-password"), credentialHash(t, "security-code"))
	if err != nil {
		t.Fatal(err)
	}
	r.sess.deps.AccountSessions = NewAccountSessions()
	r.sess.onLogin(protocol.Request{S1: "owner", S2: "old-password"})
	if r.stage() != StageAuthed {
		t.Fatal("login failed")
	}
	return r
}

func TestRecoveryRevokesAuthenticatedSessions(t *testing.T) {
	for _, stage := range []Stage{StageAuthed, StageInGame} {
		t.Run(map[Stage]string{StageAuthed: "selection", StageInGame: "world"}[stage], func(t *testing.T) {
			r := credentialRig(t)
			r.sess.stage = stage
			recovery := New(&capSink{}, r.sess.deps)
			recovery.onResetPassword(protocol.Request{S1: "owner", S2: "security-code", S3: "recovered-password"})
			if r.stage() != StageClosed || !r.sink.closed {
				t.Fatal("recovery left the old authenticated connection usable")
			}
			r.sess.onChangePassword(protocol.Request{S1: "old-password", S2: "attacker-password"})
			acc, _ := r.st.AccountByName(context.Background(), "owner")
			if bcrypt.CompareHashAndPassword([]byte(acc.PassHash), []byte("recovered-password")) != nil {
				t.Fatal("old connection overwrote the recovered password")
			}
		})
	}
}

func TestStaleCredentialsAreRejectedWithoutSessionRegistry(t *testing.T) {
	for _, operation := range []string{"password", "security", "enter"} {
		t.Run(operation, func(t *testing.T) {
			r := credentialRig(t)
			r.sess.deps.AccountSessions = nil
			acc, _ := r.st.AccountByName(context.Background(), "owner")
			newHash := credentialHash(t, "recovered-password")
			if err := r.st.UpdateAccountPasswordHash(context.Background(), acc.ID, newHash); err != nil {
				t.Fatal(err)
			}
			switch operation {
			case "password":
				r.sess.onChangePassword(protocol.Request{S1: "old-password", S2: "attacker-password"})
			case "security":
				r.sess.onChangeSecurityCode(protocol.Request{S1: "old-password", S2: "security-code", S3: "attacker-code"})
			case "enter":
				r.sess.onEnter(protocol.Request{S1: "any-character"})
			}
			current, _ := r.st.AccountByName(context.Background(), "owner")
			if current.PassHash != newHash || current.SecHash != acc.SecHash || r.stage() != StageClosed {
				t.Fatal("stale authentication was not revoked before the operation")
			}
		})
	}
}

// Run a competing committed update after validation but before the CAS write.
type competingCredentialStore struct {
	store.Store
	beforeWrite func()
}

func (c *competingCredentialStore) CompareAndSwapAccountCredentials(ctx context.Context, id int64,
	oldPass, oldSec, newPass, newSec string) (bool, error) {
	if c.beforeWrite != nil {
		fn := c.beforeWrite
		c.beforeWrite = nil
		fn()
	}
	return c.Store.CompareAndSwapAccountCredentials(ctx, id, oldPass, oldSec, newPass, newSec)
}

func TestCredentialUpdatesDoNotOverwriteConcurrentRecovery(t *testing.T) {
	for _, operation := range []string{"password", "security", "recovery"} {
		t.Run(operation, func(t *testing.T) {
			r := credentialRig(t)
			acc, _ := r.st.AccountByName(context.Background(), "owner")
			winningHash := credentialHash(t, "winning-password")
			r.sess.deps.Store = &competingCredentialStore{Store: r.st, beforeWrite: func() {
				if err := r.st.UpdateAccountPasswordHash(context.Background(), acc.ID, winningHash); err != nil {
					t.Fatal(err)
				}
			}}
			switch operation {
			case "password":
				r.sess.onChangePassword(protocol.Request{S1: "old-password", S2: "losing-password"})
			case "security":
				r.sess.onChangeSecurityCode(protocol.Request{S1: "old-password", S2: "security-code", S3: "losing-code"})
			case "recovery":
				recovery := New(&capSink{}, r.sess.deps)
				recovery.onResetPassword(protocol.Request{S1: "owner", S2: "security-code", S3: "losing-password"})
			}
			current, _ := r.st.AccountByName(context.Background(), "owner")
			if current.PassHash != winningHash || current.SecHash != acc.SecHash {
				t.Fatal("credential write overwrote a concurrent committed recovery")
			}
		})
	}
}

func TestChangeCredentialsKeepsCurrentSessionUsable(t *testing.T) {
	r := credentialRig(t)
	r.sess.onChangePassword(protocol.Request{S1: "old-password", S2: "new-password"})
	r.sess.onChangeSecurityCode(protocol.Request{S1: "new-password", S2: "security-code", S3: "new-code"})
	current, ok := r.sess.accountForSecurity()
	if !ok || bcrypt.CompareHashAndPassword([]byte(current.PassHash), []byte("new-password")) != nil ||
		bcrypt.CompareHashAndPassword([]byte(current.SecHash), []byte("new-code")) != nil {
		t.Fatal("successful credential updates did not preserve a valid current session")
	}
}

func TestRecoveryDoesNotRevokeNewPasswordLogin(t *testing.T) {
	r := credentialRig(t)
	acc, _ := r.st.AccountByName(context.Background(), "owner")
	newHash := credentialHash(t, "recovered-password")
	if err := r.st.UpdateAccountPasswordHash(context.Background(), acc.ID, newHash); err != nil {
		t.Fatal(err)
	}
	newSink := &capSink{}
	newLogin := New(newSink, r.sess.deps)
	newLogin.onLogin(protocol.Request{S1: "owner", S2: "recovered-password"})
	if n := r.sess.deps.AccountSessions.RevokeCredentials(acc.ID, acc.PassHash, "recovery completed"); n != 0 {
		t.Fatal("recovery revoked a login using the new password")
	}
	if r.stage() != StageClosed || newLogin.stage != StageAuthed || newSink.closed {
		t.Fatal("new authentication was not preserved")
	}
}

func TestGMBanRevokesCharacterSelectionSession(t *testing.T) {
	r := credentialRig(t)
	acc, _ := r.st.AccountByName(context.Background(), "owner")
	char := &domain.Character{AccountID: acc.ID, Name: "target"}
	if err := r.st.CreateChar(context.Background(), char); err != nil {
		t.Fatal(err)
	}
	r.sess.deps.Online = online.NewRegistry() // Selection has no online character.
	gm := New(&capSink{}, r.sess.deps)
	ok, msg := gm.executeGMAdmin(parsedGMCommand{admin: gmAdminBan, target: char.Name}, nil, nil)
	if !ok {
		t.Fatal(msg)
	}
	current, _ := r.st.AccountByName(context.Background(), "owner")
	if !current.Banned || r.stage() != StageClosed || !r.sink.closed {
		t.Fatal("GM ban failed to revoke the character-selection session")
	}
	r.sess.onEnter(protocol.Request{S1: char.Name})
	if r.stage() != StageClosed {
		t.Fatal("banned connection entered the world")
	}
}

func TestEnterRechecksPermanentAndTimedBan(t *testing.T) {
	for _, timed := range []bool{false, true} {
		r := credentialRig(t)
		acc, _ := r.st.AccountByName(context.Background(), "owner")
		if timed {
			if err := r.st.SetAccountBannedUntil(context.Background(), acc.ID, time.Now().Add(time.Hour)); err != nil {
				t.Fatal(err)
			}
		} else if err := r.st.SetAccountBanned(context.Background(), acc.ID, true); err != nil {
			t.Fatal(err)
		}
		r.sess.onEnter(protocol.Request{S1: "any-character"})
		if r.stage() != StageClosed || !r.sink.closed {
			t.Fatal("database ban was not checked at world entry")
		}
	}
}

type changingLoginStore struct {
	store.Store
	afterPasswordCheck func()
}

func (c *changingLoginStore) CharsByAccount(ctx context.Context, id int64) ([]*domain.Character, error) {
	if c.afterPasswordCheck != nil {
		fn := c.afterPasswordCheck
		c.afterPasswordCheck = nil
		fn()
	}
	return c.Store.CharsByAccount(ctx, id)
}

func TestLoginRejectsAccountChangeBeforeSessionClaim(t *testing.T) {
	for _, change := range []string{"password", "ban"} {
		t.Run(change, func(t *testing.T) {
			r := credentialRig(t)
			acc, _ := r.st.AccountByName(context.Background(), "owner")
			deps := r.sess.deps
			deps.Store = &changingLoginStore{Store: r.st, afterPasswordCheck: func() {
				if change == "password" {
					_ = r.st.UpdateAccountPasswordHash(context.Background(), acc.ID, credentialHash(t, "new-password"))
				} else {
					_ = r.st.SetAccountBanned(context.Background(), acc.ID, true)
				}
				deps.AccountSessions.Revoke(acc.ID, "concurrent change")
			}}
			sink := &capSink{}
			login := New(sink, deps)
			login.onLogin(protocol.Request{S1: "owner", S2: "old-password"})
			if login.stage != StageClosed || !sink.closed {
				t.Fatal("login authenticated a credential snapshot invalidated before Claim")
			}
			if code, sent := sink.respCode(); sent && code == 0 {
				t.Fatal("stale login reported success")
			}
			if deps.AccountSessions.Revoke(acc.ID, "check") != 0 {
				t.Fatal("closed login retained the account binding")
			}
		})
	}
}
