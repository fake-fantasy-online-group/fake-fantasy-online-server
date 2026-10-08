package session

import "sync"

// AccountSessions 是全服共享的账号在线会话表。
//
// 账号认证成功时，新会话原子替换旧会话；旧会话随后被断开。Release 必须同时
// 校验会话指针，避免被顶掉的旧连接迟到执行 OnClose 时误删新连接的所有权。
type AccountSessions struct {
	mu        sync.Mutex
	byAccount map[int64]*Session
}

func NewAccountSessions() *AccountSessions {
	return &AccountSessions{byAccount: make(map[int64]*Session)}
}

// Claim 把账号绑定到 current，并返回需要断开的旧会话。同一会话重复 Claim
// 是幂等的，不会把自己当成被顶掉者。
func (r *AccountSessions) Claim(accountID int64, current *Session) (replaced *Session) {
	if r == nil || accountID <= 0 || current == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	replaced = r.byAccount[accountID]
	r.byAccount[accountID] = current
	if replaced == current {
		return nil
	}
	return replaced
}

// Release 只释放仍属于 current 的账号绑定。若新会话已经 Claim，旧会话的迟到
// 清理会在这里成为 no-op。
func (r *AccountSessions) Release(accountID int64, current *Session) bool {
	if r == nil || accountID <= 0 || current == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.byAccount[accountID] != current {
		return false
	}
	delete(r.byAccount, accountID)
	return true
}

// Revoke also covers authenticated sessions still on character selection. Take
// the binding before closing; callbacks may acquire the registry again.
func (r *AccountSessions) Revoke(accountID int64, reason string) int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	current := r.byAccount[accountID]
	delete(r.byAccount, accountID)
	r.mu.Unlock()
	if current == nil {
		return 0
	}
	current.revokeAuthentication(reason)
	return 1
}

// A login using the recovered password can race with recovery's notification.
// Keep that new authentication; only the obsolete credential snapshot is revoked.
func (r *AccountSessions) RevokeCredentials(accountID int64, oldPassHash, reason string) int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	current := r.byAccount[accountID]
	r.mu.Unlock()
	if current == nil {
		return 0
	}
	current.mu.Lock()
	stale := current.account != nil && current.account.ID == accountID &&
		current.account.PassHash == oldPassHash
	current.mu.Unlock()
	if !stale {
		return 0
	}
	r.Release(accountID, current)
	current.revokeAuthentication(reason)
	return 1
}

func (s *Session) revokeAuthentication(reason string) {
	// Close alone only wakes the socket loop; OnClose synchronously invalidates
	// Stage and orders Leave after any in-flight Enter before another request runs.
	s.OnClose(reason)
	s.sink.Close()
}

func (s *Session) revokeAccountSessions(accountID int64, reason string) int {
	n := s.deps.AccountSessions.Revoke(accountID, reason)
	// Legacy tools may omit AccountSessions; also close any remaining world entry.
	if s.deps.Online != nil {
		n += disconnectAccount(s.deps.Online, accountID)
	}
	return n
}
